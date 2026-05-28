package models

import (
	"time"

	"github.com/google/uuid"
)

// Account 账号信息
type Account struct {
	ID                    string `json:"id"`
	Email                 string `json:"email"`
	Password              string `json:"password,omitempty"`
	Nickname              string `json:"nickname"`
	Token                 string `json:"token,omitempty"`
	RefreshToken          string `json:"refresh_token,omitempty"`
	WindsurfAPIKey        string `json:"windsurf_api_key,omitempty"`
	PlanName              string `json:"plan_name"`
	UsedQuota             int    `json:"used_quota"`
	TotalQuota            int    `json:"total_quota"`
	DailyRemaining        string `json:"daily_remaining"`  // 例如 "85.3%"
	WeeklyRemaining       string `json:"weekly_remaining"` // 例如 "72.1%"
	DailyResetAt          string `json:"daily_reset_at"`
	WeeklyResetAt         string `json:"weekly_reset_at"`
	// ExtraUsageBalanceMicros 额外用量余额(Extra usage balance),单位 micros(百万分之一美元)。
	// 来自 GetPlanStatus 的 overageBalanceMicros 字段。正数=还有预付余额可用;
	// 负数=已用超/欠费;0 或字段缺失=未开通 extra usage。
	// HasExtraUsageBalance 标记本账号是否真的带回了该字段(区分"余额=0"与"没这字段")。
	ExtraUsageBalanceMicros int64 `json:"extra_usage_balance_micros"`
	HasExtraUsageBalance    bool  `json:"has_extra_usage_balance"`
	SubscriptionExpiresAt string `json:"subscription_expires_at"`
	TokenExpiresAt        string `json:"token_expires_at"`
	Status                string `json:"status"`
	Tags                  string `json:"tags"`
	Remark                string `json:"remark"`
	LastLoginAt           string `json:"last_login_at"`
	LastQuotaUpdate       string `json:"last_quota_update"`
	CreatedAt             string `json:"created_at"`
}

func NewAccount(email, password, nickname string) *Account {
	return &Account{
		ID:        uuid.New().String(),
		Email:     email,
		Password:  password,
		Nickname:  nickname,
		PlanName:  "unknown",
		Status:    "active",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
}

// ProviderAccount 第三方 LLM 提供商账号(OpenAI / Anthropic / DeepSeek 等)。
//
// 与 Account(Windsurf 专用)物理隔离:独立 schema、独立存储文件
// (provider_accounts.json)、独立 Store 方法。当前阶段只做 CRUD,
// Relay 层接入留待后续 phase。
type ProviderAccount struct {
	ID         string `json:"id"`
	Provider   string `json:"provider"` // openai / anthropic / deepseek / moonshot / qwen / google / xai / ...
	BaseURL    string `json:"base_url"`
	AuthToken  string `json:"auth_token"`
	Nickname   string `json:"nickname,omitempty"`
	Remark     string `json:"remark,omitempty"`
	Status     string `json:"status"` // active / disabled
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	UsedQuota  int    `json:"used_quota,omitempty"`
	TotalQuota int    `json:"total_quota,omitempty"`

	// ── 阶段 2 路由调度字段 ──
	// Activated 卡片是否参与 MITM 提供商分流。同一 provider 内多张 activated
	// 会被轮询挑选;false 时该卡片只是号池里挂着的存档，不接流量。
	Activated bool `json:"activated,omitempty"`
	// ActiveModel 强制重写 IDE 进来的 model 为此值(空则用 IDE 原值)。
	// 用户从 Models 下拉里选定。
	ActiveModel string `json:"active_model,omitempty"`
	// Models 上次拉 {base_url}/v1/models 的结果。空数组 + 非空
	// ModelsError 表示已尝试但失败。
	Models []string `json:"models,omitempty"`
	// ModelsRefreshedAt RFC3339;ModelsError 非空 = 上次拉取失败原因。
	ModelsRefreshedAt string `json:"models_refreshed_at,omitempty"`
	ModelsError       string `json:"models_error,omitempty"`
}

func NewProviderAccount(provider, baseURL, token, remark string) *ProviderAccount {
	return &ProviderAccount{
		ID:        uuid.New().String(),
		Provider:  provider,
		BaseURL:   baseURL,
		AuthToken: token,
		Remark:    remark,
		Status:    "active",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
}
