package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// app_provider.go ── 第三方 LLM 提供商账号 (OpenAI / Anthropic / DeepSeek / ...)
//                    Wails binding 暴露面，与 Windsurf 号池物理隔离。
//
// 与 Windsurf Account 的差异：
//   - 不调 GetJWTByAPIKey / RegisterUser / enrichAccountInfoLite，第三方 token
//     不需要也不能换 Windsurf JWT
//   - 落库到 provider_accounts.json，与 accounts.json 不混
//   - Provider/AuthToken 联合唯一去重

// ProviderKeyItem 前端 Provider 模式批量导入的单条数据。
type ProviderKeyItem struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Token    string `json:"token"`
	Remark   string `json:"remark"`
	Nickname string `json:"nickname"`
}

// ImportByProvider 批量导入第三方提供商账号。
//
// 不调 Windsurf 的 GetJWTByAPIKey / RegisterUser / enrichAccountInfoLite —
// 第三方 token 不需要也不能换 Windsurf JWT。校验 → 落库 → 返回每条结果。
func (a *App) ImportByProvider(items []ProviderKeyItem) []ImportResult {
	if len(items) == 0 {
		return nil
	}
	results := make([]ImportResult, len(items))
	accounts := make([]models.ProviderAccount, 0, len(items))
	indexMap := make([]int, 0, len(items)) // accounts[k] → results[indexMap[k]]

	for i, item := range items {
		provider := strings.TrimSpace(strings.ToLower(item.Provider))
		token := strings.TrimSpace(item.Token)
		baseURL := strings.TrimRight(strings.TrimSpace(item.BaseURL), "/")
		emailLike := providerAccountDisplayName(provider, token)

		if provider == "" {
			results[i] = ImportResult{Email: emailLike, Success: false, Error: "provider 不能为空"}
			continue
		}
		if token == "" {
			results[i] = ImportResult{Email: emailLike, Success: false, Error: "token 不能为空"}
			continue
		}
		if baseURL == "" {
			results[i] = ImportResult{Email: emailLike, Success: false, Error: "base_url 不能为空"}
			continue
		}

		acc := models.NewProviderAccount(provider, baseURL, token, item.Remark)
		acc.Nickname = strings.TrimSpace(item.Nickname)
		accounts = append(accounts, *acc)
		indexMap = append(indexMap, i)
		// 占位结果,落库失败再覆盖
		results[i] = ImportResult{Email: emailLike, Success: true}
	}

	if len(accounts) == 0 {
		return results
	}

	errs := a.providerStore.AddProviderBatch(accounts)
	for k, err := range errs {
		idx := indexMap[k]
		if err != nil {
			results[idx].Success = false
			results[idx].Error = err.Error()
		}
	}
	// 入库成功的账号异步触发 model 拉取 — 不阻塞批量导入响应；
	// 失败原因写到 ModelsError 让 UI 显示。
	for k, err := range errs {
		if err != nil {
			continue
		}
		idx := indexMap[k]
		if !results[idx].Success {
			continue
		}
		acc := accounts[k]
		go a.refreshProviderModelsAsync(acc.ID, acc.Provider, acc.BaseURL, acc.AuthToken)
	}
	return results
}

// RefreshProviderModels 手动重新拉取 model 列表(UI 卡片刷新按钮)。
// 同步等结果返回，前端能直接显示新列表。
func (a *App) RefreshProviderModels(id string) error {
	if a.providerStore == nil {
		return fmt.Errorf("provider store 未初始化")
	}
	acc, err := a.providerStore.Get(id)
	if err != nil {
		return err
	}
	return a.fetchAndPersistProviderModels(acc.ID, acc.Provider, acc.BaseURL, acc.AuthToken)
}

// NextActiveAccount 总览「下一席位」按钮入口。
// 在 同 active_model + status=active 候选里翻到下一张, 把它置 activated。
//
// 返回新激活卡(供前端显示);整库无候选 / 候选只有一张时返回 error,
// 错误消息分别为 "no_candidates" / "only_one"。
func (a *App) NextActiveAccount() (models.ProviderAccount, error) {
	if a.providerStore == nil {
		return models.ProviderAccount{}, fmt.Errorf("provider store 未初始化")
	}
	return a.providerStore.NextActivated()
}

// GetActiveAccount 返回当前全局唯一激活的 provider 账号。
// 没有激活卡时返回 zero 值;前端用此查询 Sidebar / Dashboard 当前活跃显示。
func (a *App) GetActiveAccount() models.ProviderAccount {
	if a.providerStore == nil {
		return models.ProviderAccount{}
	}
	acc, _ := a.providerStore.GetActivated()
	return acc
}

// refreshProviderModelsAsync goroutine 入口：忽略 error，已写到 store 字段。
// 内含 recover：批量导入会并发起多个此 goroutine,任一 panic(如解析异常)
// 不应连带崩掉整个进程。
func (a *App) refreshProviderModelsAsync(id, provider, baseURL, token string) {
	defer func() {
		if r := recover(); r != nil {
			utils.DLog("[Provider] refreshProviderModelsAsync panic recovered: %v", r)
		}
	}()
	_ = a.fetchAndPersistProviderModels(id, provider, baseURL, token)
}

// fetchAndPersistProviderModels 拉 + 持久化的核心。出错时也写一次
// (空 list + errMsg)，让 UI 上能看到失败原因不再瞎猜。
func (a *App) fetchAndPersistProviderModels(id, provider, baseURL, token string) error {
	if a.providerStore == nil {
		return fmt.Errorf("provider store 未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var httpClient *http.Client
	if a.transportPool != nil {
		httpClient = a.transportPool.Client()
	}
	list, err := services.FetchProviderModels(ctx, httpClient, provider, baseURL, token)
	if err != nil {
		_ = a.providerStore.SetProviderModels(id, nil, err.Error())
		return err
	}
	return a.providerStore.SetProviderModels(id, list, "")
}

// GetAllProviderAccounts 返回全部提供商账号(含已禁用)。
func (a *App) GetAllProviderAccounts() []models.ProviderAccount {
	if a.providerStore == nil {
		return nil
	}
	return a.providerStore.GetAll()
}

// GetProviderAccount 按 ID 取单条。
func (a *App) GetProviderAccount(id string) (models.ProviderAccount, error) {
	if a.providerStore == nil {
		return models.ProviderAccount{}, fmt.Errorf("provider store 未初始化")
	}
	return a.providerStore.Get(id)
}

// UpdateProviderAccount 替换整条记录(前端先 Get,改字段,再回传)。
func (a *App) UpdateProviderAccount(acc models.ProviderAccount) error {
	if a.providerStore == nil {
		return fmt.Errorf("provider store 未初始化")
	}
	return a.providerStore.UpdateProvider(acc)
}

// DeleteProviderAccount 按 ID 删除。
func (a *App) DeleteProviderAccount(id string) error {
	if a.providerStore == nil {
		return fmt.Errorf("provider store 未初始化")
	}
	return a.providerStore.DeleteProvider(id)
}

// providerAccountDisplayName 给 ImportResult.Email 拼一个人类可读的占位名,
// 风格同 ImportByAPIKey(token 前 12 + 后 6)。
func providerAccountDisplayName(provider, token string) string {
	if token == "" {
		return fmt.Sprintf("%s|<empty>", provider)
	}
	head := minInt(12, len(token))
	tail := maxInt(0, len(token)-6)
	return fmt.Sprintf("%s|%s...%s", provider, token[:head], token[tail:])
}
