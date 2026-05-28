package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/paths"
)

type Store struct {
	dataDir      string
	accountsFile string
	settingsFile string
	mu           sync.RWMutex
	accounts     []models.Account
	settings     models.Settings
}

// DataDir 返回号池与 settings.json 所在目录（跨平台统一为 UserConfigDir/WindsurfTools）。
func (s *Store) DataDir() string {
	return s.dataDir
}

// NewStoreInPaths 在指定目录创建/加载账号与设置文件（accounts.json、settings.json）。
func NewStoreInPaths(appDir string) (*Store, error) {
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	s := &Store{
		dataDir:      appDir,
		accountsFile: filepath.Join(appDir, "accounts.json"),
		settingsFile: filepath.Join(appDir, "settings.json"),
		accounts:     make([]models.Account, 0),
		settings:     models.DefaultSettings(),
	}

	s.load()
	return s, nil
}

func NewStore() (*Store, error) {
	dir, err := paths.ResolveAppConfigDir()
	if err != nil {
		return nil, err
	}
	return NewStoreInPaths(dir)
}

func (s *Store) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, err := os.ReadFile(s.accountsFile); err == nil {
		if json.Valid(b) {
			_ = json.Unmarshal(b, &s.accounts)
		} else {
			// accounts.json 损坏，尝试从 .bak 恢复
			bakPath := s.accountsFile + ".bak"
			if bakData, bakErr := os.ReadFile(bakPath); bakErr == nil && json.Valid(bakData) {
				_ = json.Unmarshal(bakData, &s.accounts)
				// 恢复成功，覆盖损坏的文件
				_ = os.WriteFile(s.accountsFile, bakData, 0644)
				fmt.Printf("[Store] accounts.json 已损坏，已从 .bak 恢复 (%d bytes)\n", len(bakData))
			} else {
				fmt.Printf("[Store] ⚠ accounts.json 已损坏且无有效 .bak 可恢复 (%d bytes)\n", len(b))
			}
		}
	}
	if b, err := os.ReadFile(s.settingsFile); err == nil {
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(b, &raw)
		_ = json.Unmarshal(b, &s.settings)
		// 旧版 settings.json 无此字段时默认开启（与 models.DefaultSettings 一致）
		if _, ok := raw["auto_switch_on_quota_exhausted"]; !ok {
			s.settings.AutoSwitchOnQuotaExhausted = true
		}
		if _, ok := raw["quota_hot_poll_seconds"]; !ok {
			s.settings.QuotaHotPollSeconds = 12
		}
		// 旧版无 mitm_route_mode 字段时默认 "pool"(沿用历史行为)
		if _, ok := raw["mitm_route_mode"]; !ok {
			s.settings.MitmRouteMode = "pool"
		}
	}
}

func (s *Store) saveAccounts() error {
	b, err := json.MarshalIndent(s.accounts, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.accountsFile, b)
}

func (s *Store) saveSettings() error {
	b, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.settingsFile, b)
}

// atomicWriteFile 原子写入：先写临时文件再 rename，防止进程崩溃时损坏 JSON。
// 加固措施：写入前校验 JSON 有效性、先备份旧文件、fsync 刷盘再 rename。
func atomicWriteFile(filePath string, data []byte) error {
	// 1. 写入前校验 JSON 合法性，防止写入损坏数据
	if !json.Valid(data) {
		return fmt.Errorf("atomicWriteFile: 拒绝写入非法 JSON 到 %s (%d bytes)", filepath.Base(filePath), len(data))
	}

	// 2. 如果目标文件存在，先创建 .bak 备份
	if _, err := os.Stat(filePath); err == nil {
		bakPath := filePath + ".bak"
		// 静默失败: 备份不是关键路径
		if bakData, readErr := os.ReadFile(filePath); readErr == nil && json.Valid(bakData) {
			_ = os.WriteFile(bakPath, bakData, 0644)
		}
	}

	// 3. 使用带 pid 的临时文件名，避免并发/残留冲突
	tmpPath := fmt.Sprintf("%s.tmp.%d", filePath, os.Getpid())
	_ = os.Remove(tmpPath) // 清理可能的残留

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		// 创建 tmp 失败，回退直接写
		return os.WriteFile(filePath, data, 0644)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return os.WriteFile(filePath, data, 0644)
	}

	// 4. fsync 确保数据落盘
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return os.WriteFile(filePath, data, 0644)
	}
	f.Close()

	// 5. rename 原子替换
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return os.WriteFile(filePath, data, 0644)
	}
	return nil
}

// ── Account Operations ──

func (s *Store) AddAccount(acc models.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if AccountsConflict(s.accounts[i], acc) {
			return fmt.Errorf("账号已存在，不可重复导入")
		}
	}
	s.accounts = append(s.accounts, acc)
	return s.saveAccounts()
}

func (s *Store) GetAllAccounts() []models.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copied := make([]models.Account, len(s.accounts))
	copy(copied, s.accounts)
	return copied
}

// AccountCount 返回号池总数（轻量，不拷贝切片）。
func (s *Store) AccountCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.accounts)
}

// AddAccountsBatch 批量添加账号，仅在所有写入完成后执行一次持久化；返回每条记录的错误（nil 表示成功）。
func (s *Store) AddAccountsBatch(accs []models.Account) []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := make([]error, len(accs))
	added := false
	for i, acc := range accs {
		dup := false
		for j := range s.accounts {
			if AccountsConflict(s.accounts[j], acc) {
				errs[i] = fmt.Errorf("账号已存在，不可重复导入")
				dup = true
				break
			}
		}
		if !dup {
			s.accounts = append(s.accounts, acc)
			added = true
		}
	}
	if added {
		if err := s.saveAccounts(); err != nil {
			for i := range errs {
				if errs[i] == nil {
					errs[i] = err
				}
			}
		}
	}
	return errs
}

// GetAccount 返回账号值的拷贝，避免调用方持有指向内部切片的指针导致数据竞争。
func (s *Store) GetAccount(id string) (models.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			return s.accounts[i], nil
		}
	}
	return models.Account{}, fmt.Errorf("account not found")
}

func (s *Store) UpdateAccount(acc models.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if s.accounts[i].ID == acc.ID {
			s.accounts[i] = acc
			return s.saveAccounts()
		}
	}
	return fmt.Errorf("account not found")
}

func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			s.accounts = append(s.accounts[:i], s.accounts[i+1:]...)
			return s.saveAccounts()
		}
	}
	return fmt.Errorf("account not found")
}

// ── Settings Operations ──

func (s *Store) GetSettings() models.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *Store) UpdateSettings(st models.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = st
	return s.saveSettings()
}

// MutateSettings 在持锁状态下对当前 settings 做读-改-写,消除多入口并发 RMW 丢更新。
//
//	mutate 收到指向当前 settings 副本的指针,直接修改其字段即可;返回后原子持久化。
//	所有"只改少数字段"的入口(Pin/Unpin、各 Toggle 开关)都应走这里,而不是
//	GetSettings()→改→UpdateSettings()——后者会用读时的旧快照覆盖期间别处的修改。
func (s *Store) MutateSettings(mutate func(*models.Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.settings
	mutate(&cur)
	s.settings = cur
	return s.saveSettings()
}

// ════════════════════════════════════════════════════════════════
// ProviderAccountStore — 第三方 LLM 提供商账号(OpenAI/Anthropic/...)
//
// 与 Windsurf Account 物理隔离:独立文件 provider_accounts.json,
// 独立锁、独立冲突判定。复用同包的 atomicWriteFile + .bak 备份。
// ════════════════════════════════════════════════════════════════

type ProviderAccountStore struct {
	mu       sync.RWMutex
	file     string
	accounts []models.ProviderAccount
}

func NewProviderAccountStore(dataDir string) (*ProviderAccountStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("provider store: failed to create data dir: %w", err)
	}
	s := &ProviderAccountStore{
		file:     filepath.Join(dataDir, "provider_accounts.json"),
		accounts: make([]models.ProviderAccount, 0),
	}
	s.loadProvider()
	return s, nil
}

func (s *ProviderAccountStore) loadProvider() {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	if json.Valid(b) {
		_ = json.Unmarshal(b, &s.accounts)
		return
	}
	bakPath := s.file + ".bak"
	if bakData, bakErr := os.ReadFile(bakPath); bakErr == nil && json.Valid(bakData) {
		_ = json.Unmarshal(bakData, &s.accounts)
		_ = os.WriteFile(s.file, bakData, 0644)
		fmt.Printf("[ProviderStore] provider_accounts.json 已损坏,已从 .bak 恢复 (%d bytes)\n", len(bakData))
	} else {
		fmt.Printf("[ProviderStore] ⚠ provider_accounts.json 已损坏且无有效 .bak (%d bytes)\n", len(b))
	}
}

func (s *ProviderAccountStore) saveProvider() error {
	b, err := json.MarshalIndent(s.accounts, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.file, b)
}

func (s *ProviderAccountStore) GetAll() []models.ProviderAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.ProviderAccount, len(s.accounts))
	copy(out, s.accounts)
	return out
}

func (s *ProviderAccountStore) Get(id string) (models.ProviderAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			return s.accounts[i], nil
		}
	}
	return models.ProviderAccount{}, fmt.Errorf("provider account not found")
}

func (s *ProviderAccountStore) GetByProvider(provider string) []models.ProviderAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	target := strings.TrimSpace(strings.ToLower(provider))
	out := make([]models.ProviderAccount, 0)
	for i := range s.accounts {
		if strings.ToLower(s.accounts[i].Provider) == target {
			out = append(out, s.accounts[i])
		}
	}
	return out
}

func (s *ProviderAccountStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.accounts)
}

// AddProviderBatch 批量添加,返回每条的错误(nil=成功);全部加完后只持久化一次。
// 重复判定:provider + auth_token 联合唯一。
func (s *ProviderAccountStore) AddProviderBatch(accs []models.ProviderAccount) []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := make([]error, len(accs))
	added := false
	for i, acc := range accs {
		dup := false
		for j := range s.accounts {
			if ProviderAccountsConflict(s.accounts[j], acc) {
				errs[i] = fmt.Errorf("提供商账号已存在,不可重复导入")
				dup = true
				break
			}
		}
		if !dup {
			s.accounts = append(s.accounts, acc)
			added = true
		}
	}
	if added {
		if err := s.saveProvider(); err != nil {
			for i := range errs {
				if errs[i] == nil {
					errs[i] = err
				}
			}
		}
	}
	return errs
}

func (s *ProviderAccountStore) UpdateProvider(acc models.ProviderAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if s.accounts[i].ID == acc.ID {
			s.accounts[i] = acc
			// ★ 全局唯一 activated:此处置为 activated=true 时,自动把其它卡的
			// activated 置 false,保证整库永远只有一张当前激活卡
			if acc.Activated {
				for j := range s.accounts {
					if j != i && s.accounts[j].Activated {
						s.accounts[j].Activated = false
					}
				}
			}
			return s.saveProvider()
		}
	}
	return fmt.Errorf("provider account not found")
}

func (s *ProviderAccountStore) DeleteProvider(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			s.accounts = append(s.accounts[:i], s.accounts[i+1:]...)
			return s.saveProvider()
		}
	}
	return fmt.Errorf("provider account not found")
}

// ProviderAccountsConflict 判定两条 ProviderAccount 是否视为重复
// (provider + auth_token 联合唯一)。
func ProviderAccountsConflict(a, b models.ProviderAccount) bool {
	pa := strings.TrimSpace(strings.ToLower(a.Provider))
	pb := strings.TrimSpace(strings.ToLower(b.Provider))
	if pa == "" || pb == "" || pa != pb {
		return false
	}
	ta := strings.TrimSpace(a.AuthToken)
	tb := strings.TrimSpace(b.AuthToken)
	if ta == "" || tb == "" {
		return false
	}
	return ta == tb
}

// ── 阶段 3 路由调度专用 helpers ──

// GetActivated 返回当前全局唯一激活的 ProviderAccount(activated=true && status!=disabled
// && 配置完整)。整库无激活卡时返回 (zero, false)。
func (s *ProviderAccountStore) GetActivated() (models.ProviderAccount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.accounts {
		acc := s.accounts[i]
		if !acc.Activated {
			continue
		}
		if strings.ToLower(strings.TrimSpace(acc.Status)) == "disabled" {
			continue
		}
		if strings.TrimSpace(acc.BaseURL) == "" || strings.TrimSpace(acc.AuthToken) == "" {
			continue
		}
		return acc, true
	}
	return models.ProviderAccount{}, false
}

// NextActivated 在 同 active_model + status=active + 配置完整 的候选里
// 把当前 activated 卡翻到下一张。返回新激活卡的副本与 ok 状态。
//
// 行为:
//   - 当前没激活卡 → 候选[0] 设为 activated 返回
//   - 候选只有自己 → 不动,ok=false 错误信息 "only_one"
//   - 候选 0 张(无同 model 卡) → ok=false 错误信息 "no_candidates"
//
// 候选按 ID 排序保证翻动节奏稳定。
func (s *ProviderAccountStore) NextActivated() (models.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 找到当前激活卡(若有)
	currentIdx := -1
	for i := range s.accounts {
		acc := s.accounts[i]
		if acc.Activated &&
			strings.ToLower(strings.TrimSpace(acc.Status)) != "disabled" &&
			strings.TrimSpace(acc.BaseURL) != "" &&
			strings.TrimSpace(acc.AuthToken) != "" {
			currentIdx = i
			break
		}
	}

	// 候选过滤:同 active_model + status=active + 配置完整。如果当前没激活卡,
	// 用所有 active 且 active_model 非空的卡作为候选(从第一张开始)。
	targetModel := ""
	if currentIdx >= 0 {
		targetModel = strings.TrimSpace(s.accounts[currentIdx].ActiveModel)
	}

	candidateIdx := make([]int, 0)
	for i := range s.accounts {
		acc := s.accounts[i]
		if strings.ToLower(strings.TrimSpace(acc.Status)) == "disabled" {
			continue
		}
		if strings.TrimSpace(acc.BaseURL) == "" || strings.TrimSpace(acc.AuthToken) == "" {
			continue
		}
		if strings.TrimSpace(acc.ActiveModel) == "" {
			continue
		}
		// 当前没激活卡 → 任何 active_model 非空的都可作起点
		// 当前有激活卡 → 必须 active_model 完全相同
		if targetModel != "" && strings.TrimSpace(acc.ActiveModel) != targetModel {
			continue
		}
		candidateIdx = append(candidateIdx, i)
	}

	// ID 排序保证跨进程顺序一致
	for i := 0; i < len(candidateIdx); i++ {
		for j := i + 1; j < len(candidateIdx); j++ {
			if s.accounts[candidateIdx[i]].ID > s.accounts[candidateIdx[j]].ID {
				candidateIdx[i], candidateIdx[j] = candidateIdx[j], candidateIdx[i]
			}
		}
	}

	if len(candidateIdx) == 0 {
		return models.ProviderAccount{}, fmt.Errorf("no_candidates")
	}
	if len(candidateIdx) == 1 && candidateIdx[0] == currentIdx {
		return models.ProviderAccount{}, fmt.Errorf("only_one")
	}

	// 找到当前在候选列表中的位置,翻到下一张(环绕)
	nextPos := 0
	if currentIdx >= 0 {
		for k, idx := range candidateIdx {
			if idx == currentIdx {
				nextPos = (k + 1) % len(candidateIdx)
				break
			}
		}
	}
	nextIdx := candidateIdx[nextPos]

	// 切换:其它所有 activated 置 false,新激活卡置 true
	for i := range s.accounts {
		if i == nextIdx {
			s.accounts[i].Activated = true
		} else if s.accounts[i].Activated {
			s.accounts[i].Activated = false
		}
	}
	if err := s.saveProvider(); err != nil {
		return models.ProviderAccount{}, err
	}
	return s.accounts[nextIdx], nil
}

// CandidatesForActive 返回当前激活卡 + 同 active_model 的其它可用卡,用于上游
// 请求失败时的故障切换重试。顺序:激活卡排第一,其余按 ID 升序。
//
// 过滤条件与 NextActivated 一致:status≠disabled、base_url/auth_token 非空、
// active_model 非空且与激活卡相同。无激活卡时返回 nil(由上层回落号池处理)。
func (s *ProviderAccountStore) CandidatesForActive() []models.ProviderAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()

	usable := func(acc models.ProviderAccount) bool {
		if strings.ToLower(strings.TrimSpace(acc.Status)) == "disabled" {
			return false
		}
		if strings.TrimSpace(acc.BaseURL) == "" || strings.TrimSpace(acc.AuthToken) == "" {
			return false
		}
		return true
	}

	activeIdx := -1
	for i := range s.accounts {
		if s.accounts[i].Activated && usable(s.accounts[i]) {
			activeIdx = i
			break
		}
	}
	if activeIdx < 0 {
		return nil
	}

	targetModel := strings.TrimSpace(s.accounts[activeIdx].ActiveModel)

	// 收集其它同 model 候选(排除激活卡自身)。targetModel 为空时无可靠的同组
	// 概念,只返回激活卡自己,避免把不相干的卡也拿来重试。
	others := make([]models.ProviderAccount, 0)
	if targetModel != "" {
		for i := range s.accounts {
			if i == activeIdx || !usable(s.accounts[i]) {
				continue
			}
			if strings.TrimSpace(s.accounts[i].ActiveModel) == targetModel {
				others = append(others, s.accounts[i])
			}
		}
		sort.Slice(others, func(a, b int) bool { return others[a].ID < others[b].ID })
	}

	out := make([]models.ProviderAccount, 0, len(others)+1)
	out = append(out, s.accounts[activeIdx])
	out = append(out, others...)
	return out
}

// SetProviderModels 写回某账号的 /v1/models 拉取结果。
// 失败也要写：用 errMsg 记下来供 UI 展示，避免用户不知道为啥列表是空。
func (s *ProviderAccountStore) SetProviderModels(id string, models []string, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			s.accounts[i].Models = models
			s.accounts[i].ModelsError = errMsg
			s.accounts[i].ModelsRefreshedAt = time.Now().Format(time.RFC3339)
			return s.saveProvider()
		}
	}
	return fmt.Errorf("provider account not found")
}
