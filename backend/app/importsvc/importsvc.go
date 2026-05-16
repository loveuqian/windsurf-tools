// Package importsvc ── 批量导入 + 单个添加账号的并发框架与 5 种导入方式。
//
// 设计要点：
//
//   - main 包保留 6 个 wails binding 类型（ImportResult / EmailPasswordItem
//     / TokenItem / APIKeyItem / JWTItem / EmailAPIKeyItem）以维持 binding
//     路径不变；本子包定义同字段的独立 struct，App thin wrapper 显式拷贝。
//   - enrich / syncMitmPoolKeys / 并发数读取 都通过 Deps 注入函数指针，
//     避免子包反向 import main。
//   - 导入流程内部沿用 sync.WaitGroup + 信号量并发 + 一次性
//     store.AddAccountsBatch 的旧策略，行为完全等价。
package importsvc

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// Result 单条导入结果。
type Result struct {
	Email   string
	Success bool
	Error   string
}

// EmailPasswordItem 邮箱密码登录条目。
type EmailPasswordItem struct {
	Email       string
	Password    string
	AltPassword string
	Remark      string
}

// TokenItem RefreshToken 导入条目。
type TokenItem struct {
	Token  string
	Remark string
}

// APIKeyItem API Key 导入条目。
type APIKeyItem struct {
	APIKey string
	Remark string
}

// JWTItem JWT 直接导入条目。
type JWTItem struct {
	JWT    string
	Remark string
}

// EmailAPIKeyItem 邮箱 + API Key 组合导入条目。
type EmailAPIKeyItem struct {
	Email  string
	APIKey string
	Remark string
}

// AccountStore 描述导入流程对 store 的最小依赖。
type AccountStore interface {
	GetSettings() models.Settings
	AddAccountsBatch(accounts []models.Account) []error
}

// Deps 注入跨域协作函数。
type Deps struct {
	Store          AccountStore
	WindsurfSvc    *services.WindsurfService
	EnrichFull     func(*models.Account) // = a.enrichAccountInfo
	EnrichLite     func(*models.Account) // = a.enrichAccountInfoLite
	SyncMitmPool   func()                // = a.syncMitmPoolKeys
}

// Module 持有依赖；自身无内部状态，所有并发参数来自 settings。
type Module struct {
	deps Deps
}

// New 构造导入模块。
func New(deps Deps) *Module {
	return &Module{deps: deps}
}

// importSlot 内部导入结果（携带准备好的 Account）
type importSlot struct {
	index  int
	result Result
	acc    *models.Account // nil 表示失败
}

// concurrency 返回导入并发数（钳位 1～20）。
func (m *Module) concurrency() int {
	if m == nil || m.deps.Store == nil {
		return 3
	}
	c := m.deps.Store.GetSettings().ImportConcurrency
	if c < 1 {
		c = 3
	}
	if c > 20 {
		c = 20
	}
	return c
}

// runConcurrent 通用并发导入框架：对 items 并行执行 processFn，然后批量写入 store。
func (m *Module) runConcurrent(n int, processFn func(idx int) importSlot) []Result {
	if m == nil {
		return make([]Result, n)
	}
	defer func() {
		if m.deps.SyncMitmPool != nil {
			m.deps.SyncMitmPool()
		}
	}()

	concurrency := m.concurrency()
	utils.DLog("[导入] 开始导入 %d 条，并发=%d", n, concurrency)

	slots := make([]importSlot, n)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			slots[idx] = processFn(idx)
		}(i)
	}
	wg.Wait()

	// 收集成功的账号，批量写入 store（单次持久化）
	var accs []models.Account
	accIdxMap := make([]int, 0, n)
	for i, s := range slots {
		if s.acc != nil {
			accs = append(accs, *s.acc)
			accIdxMap = append(accIdxMap, i)
		}
	}
	if len(accs) > 0 && m.deps.Store != nil {
		errs := m.deps.Store.AddAccountsBatch(accs)
		for j, err := range errs {
			si := accIdxMap[j]
			if err != nil {
				slots[si].result.Success = false
				slots[si].result.Error = err.Error()
			}
		}
	}

	results := make([]Result, n)
	ok, fail := 0, 0
	for i, s := range slots {
		results[i] = s.result
		if s.result.Success {
			ok++
		} else {
			fail++
		}
	}
	utils.DLog("[导入] 完成: 成功=%d 失败=%d", ok, fail)
	return results
}

// FriendlyLoginError 把 Firebase/auth1 的原始英文错误映射成可读中文。
// 命中关键字时返回中文短句；未命中保留原错误避免吞掉信息。
func FriendlyLoginError(rawErr error) string {
	if rawErr == nil {
		return ""
	}
	s := strings.ToUpper(rawErr.Error())
	switch {
	case strings.Contains(s, "INVALID_LOGIN_CREDENTIALS"),
		strings.Contains(s, "INVALID_PASSWORD"),
		strings.Contains(s, "INVALID_EMAIL"):
		return "邮箱或密码错误"
	case strings.Contains(s, "EMAIL_NOT_FOUND"):
		return "账号不存在"
	case strings.Contains(s, "USER_DISABLED"):
		return "账号已被禁用"
	case strings.Contains(s, "TOO_MANY_ATTEMPTS_TRY_LATER"),
		strings.Contains(s, "429"):
		return "登录请求过于频繁，请稍后重试（建议把并发调到 1-2）"
	case strings.Contains(s, "MISSING_PASSWORD"):
		return "未填写密码"
	case strings.Contains(s, "OPERATION_NOT_ALLOWED"):
		return "Firebase 项目未启用邮箱登录"
	case strings.Contains(s, "网络"), strings.Contains(s, "NO SUCH HOST"),
		strings.Contains(s, "CONNECTION REFUSED"), strings.Contains(s, "TIMEOUT"):
		return "网络连接失败 — 请确认能访问 windsurf.com / firebase"
	}
	return rawErr.Error()
}

// ByEmailPassword 邮箱密码登录批量导入。
func (m *Module) ByEmailPassword(items []EmailPasswordItem) []Result {
	return m.runConcurrent(len(items), func(idx int) importSlot {
		item := items[idx]
		passwords := []string{item.Password}
		if item.AltPassword != "" && item.AltPassword != item.Password {
			passwords = append(passwords, item.AltPassword)
		}
		var resp *services.FirebaseSignInResp
		var err error
		var usedPassword string
		for _, pw := range passwords {
			if pw == "" {
				continue
			}
			resp, err = m.deps.WindsurfSvc.LoginWithEmail(item.Email, pw)
			if err == nil {
				usedPassword = pw
				break
			}
		}
		if err != nil {
			return importSlot{index: idx, result: Result{
				Email: item.Email, Success: false, Error: FriendlyLoginError(err),
			}}
		}
		nickname := item.Remark
		if nickname == "" {
			nickname = strings.Split(item.Email, "@")[0]
		}
		acc := models.NewAccount(item.Email, usedPassword, nickname)
		acc.Token = resp.IDToken
		acc.RefreshToken = resp.RefreshToken
		acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
		acc.Remark = item.Remark
		if m.deps.EnrichFull != nil {
			m.deps.EnrichFull(acc)
		}
		return importSlot{index: idx, result: Result{Email: item.Email, Success: true}, acc: acc}
	})
}

// ByRefreshToken RefreshToken 批量导入。
func (m *Module) ByRefreshToken(items []TokenItem) []Result {
	return m.runConcurrent(len(items), func(idx int) importSlot {
		item := items[idx]
		resp, err := m.deps.WindsurfSvc.RefreshToken(item.Token)
		if err != nil {
			return importSlot{index: idx, result: Result{
				Email: fmt.Sprintf("Token #%d", idx+1), Success: false, Error: err.Error(),
			}}
		}
		email, _ := m.deps.WindsurfSvc.GetAccountInfo(resp.IDToken)
		if email == "" {
			email = fmt.Sprintf("user_%s", resp.UserID[:minInt(8, len(resp.UserID))])
		}
		nickname := item.Remark
		if nickname == "" {
			nickname = strings.Split(email, "@")[0]
		}
		acc := models.NewAccount(email, "", nickname)
		acc.Token = resp.IDToken
		acc.RefreshToken = resp.RefreshToken
		acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
		acc.Remark = item.Remark
		if m.deps.EnrichFull != nil {
			m.deps.EnrichFull(acc)
		}
		return importSlot{index: idx, result: Result{Email: email, Success: true}, acc: acc}
	})
}

// ByAPIKey API Key 批量导入。
func (m *Module) ByAPIKey(items []APIKeyItem) []Result {
	return m.runConcurrent(len(items), func(idx int) importSlot {
		item := items[idx]
		jwt, err := m.deps.WindsurfSvc.GetJWTByAPIKey(item.APIKey)
		if err != nil {
			return importSlot{index: idx, result: Result{
				Email: fmt.Sprintf("Key #%d", idx+1), Success: false, Error: err.Error(),
			}}
		}

		email := fmt.Sprintf("%s...%s", item.APIKey[:minInt(12, len(item.APIKey))],
			item.APIKey[maxInt(0, len(item.APIKey)-6):])

		acc := models.NewAccount(email, "", item.Remark)
		acc.Token = jwt
		acc.WindsurfAPIKey = item.APIKey
		acc.Remark = item.Remark
		if m.deps.EnrichLite != nil {
			m.deps.EnrichLite(acc)
		}
		if item.Remark == "" {
			acc.Nickname = strings.Split(acc.Email, "@")[0]
		}
		return importSlot{index: idx, result: Result{Email: acc.Email, Success: true}, acc: acc}
	})
}

// ByJWT 直接 JWT 批量导入。
func (m *Module) ByJWT(items []JWTItem) []Result {
	return m.runConcurrent(len(items), func(idx int) importSlot {
		item := items[idx]
		email := fmt.Sprintf("JWT #%d", idx+1)
		acc := models.NewAccount(email, "", item.Remark)
		acc.Token = item.JWT
		acc.Remark = item.Remark
		if m.deps.EnrichLite != nil {
			m.deps.EnrichLite(acc)
		}
		// 尝试通过 RegisterUser 获取 API Key，使账号后续可通过 GetJWTByAPIKey 持续刷新凭证
		if acc.WindsurfAPIKey == "" && acc.Token != "" && m.deps.WindsurfSvc != nil {
			if reg, err := m.deps.WindsurfSvc.RegisterUser(acc.Token); err == nil && reg != nil && reg.APIKey != "" {
				acc.WindsurfAPIKey = reg.APIKey
			}
		}
		if item.Remark == "" {
			acc.Nickname = strings.Split(acc.Email, "@")[0]
		}
		return importSlot{index: idx, result: Result{Email: acc.Email, Success: true}, acc: acc}
	})
}

// ByEmailAPIKey 邮箱 + API Key 组合批量导入。
func (m *Module) ByEmailAPIKey(items []EmailAPIKeyItem) []Result {
	return m.runConcurrent(len(items), func(idx int) importSlot {
		item := items[idx]
		email := strings.TrimSpace(item.Email)
		apiKey := strings.TrimSpace(item.APIKey)
		if email == "" || apiKey == "" {
			return importSlot{index: idx, result: Result{
				Email: email, Success: false, Error: "邮箱或 Token 为空",
			}}
		}
		nickname := item.Remark
		if nickname == "" {
			nickname = strings.Split(email, "@")[0]
		}
		acc := models.NewAccount(email, "", nickname)
		acc.WindsurfAPIKey = apiKey
		acc.Remark = item.Remark
		if m.deps.EnrichLite != nil {
			m.deps.EnrichLite(acc)
		}
		if acc.Nickname == "" {
			acc.Nickname = strings.Split(email, "@")[0]
		}
		return importSlot{index: idx, result: Result{Email: email, Success: true}, acc: acc}
	})
}

// AddSingle 单个添加 —— mode 决定走哪条 ByXxx 路径。
//
//	value 在 mode="password" 时是 JSON {email, password, alt_password}，
//	否则就是凭证字符串本身。
func (m *Module) AddSingle(mode, value, remark string) Result {
	switch mode {
	case "api_key":
		r := m.ByAPIKey([]APIKeyItem{{APIKey: value, Remark: remark}})
		if len(r) > 0 {
			return r[0]
		}
	case "jwt":
		r := m.ByJWT([]JWTItem{{JWT: value, Remark: remark}})
		if len(r) > 0 {
			return r[0]
		}
	case "refresh_token":
		r := m.ByRefreshToken([]TokenItem{{Token: value, Remark: remark}})
		if len(r) > 0 {
			return r[0]
		}
	case "password":
		var cred struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			AltPassword string `json:"alt_password"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &cred); err != nil {
			return Result{Email: "?", Success: false, Error: "邮箱密码格式错误"}
		}
		if cred.Email == "" || cred.Password == "" {
			return Result{Email: "?", Success: false, Error: "请填写邮箱与密码"}
		}
		r := m.ByEmailPassword([]EmailPasswordItem{{
			Email: cred.Email, Password: cred.Password, AltPassword: cred.AltPassword, Remark: remark,
		}})
		if len(r) > 0 {
			return r[0]
		}
	}
	return Result{Email: "?", Success: false, Error: "无效的导入类型"}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
