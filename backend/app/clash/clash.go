// Package clash ── Clash IP 轮换的 App 层封装。
//
// 设计动机：
//
//	真正与 Clash controller 对接的逻辑（probe / list / rotate）已在
//	backend/services 实现；这里只负责按 settings 启停 ClashRotator、
//	注册 MITM 上游限速回调、给 UI 暴露「立即换 IP」/「智能启用」入口。
//
// 锁策略：
//
//	Module 内部用独立的 sync.Mutex 保护 rotator 字段，与 App.mu 解耦。
package clash

import (
	"strings"
	"sync"
	"time"

	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// SettingsStore 描述 clash 模块对 store 的最小依赖。
type SettingsStore interface {
	GetSettings() models.Settings
	UpdateSettings(models.Settings) error
}

// AutoSetupResult 「智能启用」按钮的反馈，给前端弹 toast 用。
//
//	main 包对外暴露同字段的 AutoSetupClashResult struct（保留 wails binding），
//	App 在 thin wrapper 里做一次显式拷贝。
type AutoSetupResult struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Group     string `json:"group,omitempty"`
	NodeCount int    `json:"node_count,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
}

// Module 持有 ClashRotator 实例。proxy 由调用方在 New 时注入。
type Module struct {
	store SettingsStore
	proxy *services.MitmProxy

	mu      sync.Mutex
	rotator *services.ClashRotator
}

// New 构造 clash 模块。proxy 必须先于 New 创建。
func New(store SettingsStore, proxy *services.MitmProxy) *Module {
	return &Module{store: store, proxy: proxy}
}

// BuildConfig 把 settings 翻译成 ClashRotatorConfig（导出便于单测）。
func BuildConfig(s models.Settings) services.ClashRotatorConfig {
	whitelist := make([]string, 0)
	for _, n := range strings.Split(s.ClashNodes, ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			whitelist = append(whitelist, n)
		}
	}
	interval := time.Duration(s.ClashIntervalMinutes) * time.Minute
	if s.ClashIntervalMinutes <= 0 {
		interval = 8 * time.Minute
	}
	return services.ClashRotatorConfig{
		ControllerURL:  strings.TrimSpace(s.ClashControllerURL),
		Secret:         strings.TrimSpace(s.ClashSecret),
		Group:          strings.TrimSpace(s.ClashGroup),
		Whitelist:      whitelist,
		Interval:       interval,
		RotateOnRL:     s.ClashRotateOnRateLimit,
		LatencyTestURL: strings.TrimSpace(s.ClashLatencyTestURL),
		LatencyMaxMs:   s.ClashLatencyMaxMs,
	}
}

// ApplySettings 根据当前 settings 启停 / 重建 ClashRotator。
// 在 initBackend / UpdateSettings 中调用。
func (m *Module) ApplySettings() {
	if m == nil || m.store == nil || m.proxy == nil {
		return
	}
	settings := m.store.GetSettings()

	m.mu.Lock()
	defer m.mu.Unlock()

	// 若已有实例，先停掉再决定是否重启
	if m.rotator != nil {
		m.rotator.Stop()
		m.rotator = nil
	}

	if !settings.ClashRotateEnabled {
		m.proxy.SetOnUpstreamRateLimit(nil)
		return
	}
	cfg := BuildConfig(settings)
	if cfg.ControllerURL == "" || cfg.Group == "" {
		utils.DLog("[Clash] 启用但 controller_url 或 group 为空，跳过启动")
		m.proxy.SetOnUpstreamRateLimit(nil)
		return
	}
	r := services.NewClashRotator(cfg, m.proxy, func(msg string) {
		utils.DLog("%s", msg)
	})
	r.Start()
	m.rotator = r
	m.proxy.SetOnUpstreamRateLimit(func(detail string) {
		r.TriggerRotate("rate_limit")
	})
}

// Stop 关停（在 shutdown 中调用）。
func (m *Module) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rotator != nil {
		m.rotator.Stop()
		m.rotator = nil
	}
}

// TriggerRotate UI「立即换 IP」按钮。返回 false 表示当前未启动 rotator。
func (m *Module) TriggerRotate() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	r := m.rotator
	m.mu.Unlock()
	if r == nil {
		return false
	}
	r.TriggerRotate("manual")
	return true
}

// IsRunning rotator 是否在跑（Settings UI 状态展示）。
func (m *Module) IsRunning() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rotator != nil
}

// TestController 透传探活。
func (m *Module) TestController(controllerURL, secret string) services.ClashProbeResult {
	return services.ProbeClashController(controllerURL, secret)
}

// ListGroupNodes 透传组内节点列表（不含 DIRECT/REJECT/GLOBAL/组自身）。
func (m *Module) ListGroupNodes(controllerURL, secret, group string) ([]string, error) {
	return services.ListClashGroupNodes(controllerURL, secret, group)
}

// AutoDetectGroup 仅做检测、不写设置 —— 用于 UI 「自动检测」按钮预览。
func (m *Module) AutoDetectGroup() services.AutoDetectClashGroupResult {
	if m == nil || m.store == nil {
		return services.AutoDetectClashGroupResult{Error: "store 未初始化"}
	}
	s := m.store.GetSettings()
	return services.AutoDetectClashGroup(
		strings.TrimSpace(s.ClashControllerURL),
		strings.TrimSpace(s.ClashSecret),
	)
}

// AutoSetup 一键智能启用 Clash IP 轮换：
//  1. 探活控制器
//  2. 自动挑选节点最多的 selector group
//  3. 写回 settings 并重启 rotator
//  4. 立即触发一次切换，验证端到端有效
//  5. 返回切换前后节点 + 真节点数
func (m *Module) AutoSetup() AutoSetupResult {
	if m == nil || m.store == nil {
		return AutoSetupResult{Error: "store 未初始化"}
	}
	settings := m.store.GetSettings()
	url := strings.TrimSpace(settings.ClashControllerURL)
	if url == "" {
		return AutoSetupResult{
			Error: "请先在「Clash IP 轮换」面板里填写控制器地址（如 http://127.0.0.1:9097）",
			Hint:  "Verge 默认 9097；Mihomo 默认 9090；ClashX 默认 9090。",
		}
	}

	// ① 探活
	probe := services.ProbeClashController(url, strings.TrimSpace(settings.ClashSecret))
	if !probe.OK {
		return AutoSetupResult{
			Error: "控制器探活失败: " + probe.Error,
			Hint:  "检查 1) Clash 是否运行；2) external-controller 端口是否对；3) secret 是否对；4) 防火墙是否拦了。",
		}
	}

	// ② 自动挑组
	auto := services.AutoDetectClashGroup(url, strings.TrimSpace(settings.ClashSecret))
	if !auto.OK {
		hint := "请在 Clash 配置里增加一个 type=selector 的代理组。"
		if len(auto.AllGroups) > 0 {
			hint += " 或者手动在面板填以下其中一个：" + strings.Join(auto.AllGroups, " / ")
		}
		return AutoSetupResult{Error: auto.Error, Hint: hint}
	}

	// ③ 写回 settings —— 注意保留用户已配置的其它字段
	settings.ClashRotateEnabled = true
	settings.ClashGroup = auto.Group
	// ★ 强制开启「限速自动切」：用户期望「智能启用 = 一切自动」，
	// 之前若显式关过这个开关也会被覆盖。否则 rate-limit 触发时不切，
	// 用户会困惑「为什么 IDE 报 rate limit 但 IP 没变」。
	settings.ClashRotateOnRateLimit = true
	if err := m.store.UpdateSettings(settings); err != nil {
		return AutoSetupResult{Error: "保存设置失败: " + err.Error()}
	}

	// ④ 重启 rotator
	m.ApplySettings()

	// ⑤ 立即触发一次切换并捕获 from→to
	m.mu.Lock()
	r := m.rotator
	m.mu.Unlock()
	if r == nil {
		return AutoSetupResult{
			Error: "rotator 启动失败（ApplySettings 未创建实例）",
			Group: auto.Group, NodeCount: auto.NodeCount,
		}
	}

	_, _, fromBefore := r.Stats()
	r.TriggerRotate("manual")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, _, last := r.Stats()
		if last != "" && last != fromBefore {
			fromBefore = last
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	_, _, to := r.Stats()

	return AutoSetupResult{
		OK:        true,
		Group:     auto.Group,
		NodeCount: auto.NodeCount,
		From:      fromBefore,
		To:        to,
	}
}
