package models

// Settings 全局设置
type Settings struct {
	ConcurrentLimit            int    `json:"concurrent_limit"`
	AutoRefreshTokens          bool   `json:"auto_refresh_tokens"`
	AutoRefreshQuotas          bool   `json:"auto_refresh_quotas"`
	QuotaRefreshPolicy         string `json:"quota_refresh_policy"`          // hybrid | interval_* | us_calendar | local_calendar | custom
	QuotaCustomIntervalMinutes int    `json:"quota_custom_interval_minutes"` // 仅 policy=custom 时使用，默认由后端钳制
	// AutoSwitchPlanFilter 无感「下一席位」计划池：all 不限制；否则逗号分隔多选，如 trial,pro（与 PlanTone 一致）
	AutoSwitchPlanFilter string `json:"auto_switch_plan_filter"`
	// AutoSwitchOnQuotaExhausted 在自动同步额度后，若当前 Windsurf 登录账号额度用尽则尝试切到下一席（依赖 windsurf_auth 与号池匹配）
	AutoSwitchOnQuotaExhausted bool `json:"auto_switch_on_quota_exhausted"`
	// QuotaHotPollSeconds 开启「用尽切号」时，仅对当前 Windsurf 会话高频拉额度（秒）；号池其余账号只走 QuotaRefreshPolicy 的定期同步，不在此轮询。范围 5～60
	QuotaHotPollSeconds int `json:"quota_hot_poll_seconds"`
	// MinimizeToTray 点击关闭时最小化到系统托盘而不退出（需系统支持托盘图标）
	MinimizeToTray bool `json:"minimize_to_tray"`
	// SilentStart 启动时不显示主窗口（仍可在托盘打开；也可用命令行 --silent）
	SilentStart bool `json:"silent_start"`

	// ── MITM 代理 ──
	// MitmDebugDump 开启后，MITM 拦截 GetChatMessage 时将请求/响应的 protobuf 字段树写入 proto_dumps/ 目录
	MitmDebugDump bool `json:"mitm_debug_dump"`
	// MitmFullCapture 开启后，全量记录 MITM 代理的所有请求/响应到 capture/ 目录（JSONL + body 文件）
	MitmFullCapture bool `json:"mitm_full_capture"`

	// ── 静态响应缓存 ──
	StaticCacheIntercept bool `json:"static_cache_intercept"`

	// ── 破限注入（chat system prompt 末尾追加 override 文本） ──
	// MitmJailbreakEnabled 开启后，所有 GetChatMessage / GetCompletions 请求
	// 在 F2 顶层 system prompt 末尾追加 MitmJailbreakOverride 文本。覆盖
	// alignment / 拒绝模板，等效于 patch-claude-v2.py 的 `--append-system-
	// prompt-file override.md`，但走协议层、IDE 升级不受影响。
	MitmJailbreakEnabled  bool   `json:"mitm_jailbreak_enabled"`
	MitmJailbreakOverride string `json:"mitm_jailbreak_override"`

	// ── GetUserStatus 伪造 ──
	ForgeEnabled           bool   `json:"forge_enabled"`
	FakeCredits            int    `json:"fake_credits"`
	FakeCreditsPremium     int    `json:"fake_credits_premium"`
	FakeCreditsOther       int    `json:"fake_credits_other"`
	FakeCreditsUsed        int    `json:"fake_credits_used"`
	FakeSubscriptionType   string `json:"fake_subscription_type"`
	FakeBillingExtendYears int    `json:"fake_billing_extend_years"`

	// DebugLog 开启后将切号/代理/额度判定等关键日志写入文件 debug.log
	DebugLog bool `json:"debug_log"`
	// ImportConcurrency 导入时最大并发数（默认 3）
	ImportConcurrency int `json:"import_concurrency"`

	// ── OpenAI 中转 ──
	// OpenAIRelayEnabled 启用本地 OpenAI 兼容 API 中转服务器
	OpenAIRelayEnabled bool `json:"openai_relay_enabled"`
	// OpenAIRelayPort 中转服务器监听端口（默认 8787）
	OpenAIRelayPort int `json:"openai_relay_port"`
	// OpenAIRelaySecret Bearer token 鉴权密钥（空则不鉴权）
	OpenAIRelaySecret string `json:"openai_relay_secret"`

	// ── Clash IP 轮换 ──
	// ClashRotateEnabled 通过 Clash/Mihomo external-controller 周期性切换出站节点（换 IP 防限速）
	ClashRotateEnabled bool `json:"clash_rotate_enabled"`
	// ClashControllerURL Clash 外部控制器地址，如 http://127.0.0.1:9097 (Verge) 或 :9090 (Mihomo)
	ClashControllerURL string `json:"clash_controller_url"`
	// ClashSecret 外部控制器 secret（可空）
	ClashSecret string `json:"clash_secret"`
	// ClashGroup selector 类型的代理组名，例如 "PROXY" 或 "🚀 节点选择"
	ClashGroup string `json:"clash_group"`
	// ClashNodes 白名单节点名（逗号分隔）；为空则使用组内全部节点
	ClashNodes string `json:"clash_nodes"`
	// ClashIntervalMinutes 轮换间隔（分钟），范围 [2,60]，默认 8
	ClashIntervalMinutes int `json:"clash_interval_minutes"`
	// ClashRotateOnRateLimit 检测到上游 rate-limit 时立即切换节点
	ClashRotateOnRateLimit bool `json:"clash_rotate_on_rate_limit"`
	// ClashLatencyTestURL 测速用 URL，默认 http://www.gstatic.com/generate_204
	ClashLatencyTestURL string `json:"clash_latency_test_url"`
	// ClashLatencyMaxMs 仅保留延迟 <= 该值的节点（>0 生效；0=跳过测速）
	ClashLatencyMaxMs int `json:"clash_latency_max_ms"`
}

func DefaultSettings() Settings {
	return Settings{
		ConcurrentLimit:            5,
		AutoRefreshTokens:          false,
		AutoRefreshQuotas:          false,
		QuotaRefreshPolicy:         "hybrid",
		QuotaCustomIntervalMinutes: 360,
		AutoSwitchPlanFilter:       "all",
		AutoSwitchOnQuotaExhausted: true,
		QuotaHotPollSeconds:        12,
		MinimizeToTray:             false,
		SilentStart:                false,
		MitmDebugDump:              false,
		MitmFullCapture:            false,
		StaticCacheIntercept:       true,
		MitmJailbreakEnabled:       false,
		MitmJailbreakOverride:      "", // 空表示用 services.DefaultJailbreakOverride
		ForgeEnabled:               false,
		FakeCredits:                10000000,
		FakeCreditsPremium:         150000,
		FakeCreditsOther:           25000,
		FakeCreditsUsed:            0,
		FakeSubscriptionType:       "Enterprise",
		FakeBillingExtendYears:     10,
		DebugLog:                   false,
		ImportConcurrency:          3,
		OpenAIRelayEnabled:         false,
		OpenAIRelayPort:            8787,
		OpenAIRelaySecret:          "",
		ClashRotateEnabled:         false,
		ClashControllerURL:         "http://127.0.0.1:9097",
		ClashSecret:                "",
		ClashGroup:                 "",
		ClashNodes:                 "",
		ClashIntervalMinutes:       8,
		ClashRotateOnRateLimit:     true,
		ClashLatencyTestURL:        "http://www.gstatic.com/generate_204",
		ClashLatencyMaxMs:          800,
	}
}
