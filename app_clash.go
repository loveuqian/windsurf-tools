package main

import (
	"strings"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// ── Clash IP 轮换：App 层封装 ──

// buildClashConfig 把 settings 翻译成 ClashRotatorConfig。
func buildClashConfig(s models.Settings) services.ClashRotatorConfig {
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

// applyClashRotatorSettings 根据当前 settings 启停 / 重建 ClashRotator。
// 在 initBackend / UpdateSettings 中调用，应通过 a.mu 串行化。
func (a *App) applyClashRotatorSettings() {
	if a.mitmProxy == nil {
		return
	}
	settings := a.store.GetSettings()

	a.mu.Lock()
	defer a.mu.Unlock()

	// 若已有实例，先停掉再决定是否重启
	if a.clashRotator != nil {
		a.clashRotator.Stop()
		a.clashRotator = nil
	}

	if !settings.ClashRotateEnabled {
		a.mitmProxy.SetOnUpstreamRateLimit(nil)
		return
	}
	cfg := buildClashConfig(settings)
	if cfg.ControllerURL == "" || cfg.Group == "" {
		utils.DLog("[Clash] 启用但 controller_url 或 group 为空，跳过启动")
		a.mitmProxy.SetOnUpstreamRateLimit(nil)
		return
	}
	r := services.NewClashRotator(cfg, a.mitmProxy, func(msg string) {
		utils.DLog("%s", msg)
	})
	r.Start()
	a.clashRotator = r
	a.mitmProxy.SetOnUpstreamRateLimit(func(detail string) {
		r.TriggerRotate("rate_limit")
	})
}

// stopClashRotator 关停（在 shutdown 中调用）。
func (a *App) stopClashRotator() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.clashRotator != nil {
		a.clashRotator.Stop()
		a.clashRotator = nil
	}
}

// ── 暴露给前端的方法 ──

// TestClashController 探活并返回 selector 组列表。
func (a *App) TestClashController(controllerURL, secret string) services.ClashProbeResult {
	return services.ProbeClashController(controllerURL, secret)
}

// ListClashGroupNodes 列出指定组内的节点（不含 DIRECT/REJECT/GLOBAL/组自身）。
func (a *App) ListClashGroupNodes(controllerURL, secret, group string) ([]string, error) {
	return services.ListClashGroupNodes(controllerURL, secret, group)
}

// TriggerClashRotate UI 「立即换 IP」按钮。
func (a *App) TriggerClashRotate() bool {
	a.mu.Lock()
	r := a.clashRotator
	a.mu.Unlock()
	if r == nil {
		return false
	}
	r.TriggerRotate("manual")
	return true
}

// GetClashRotatorRunning 是否在运行（Settings UI 可用来显示状态）。
func (a *App) GetClashRotatorRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.clashRotator != nil
}

// AutoSetupClashResult 「智能启用」按钮的反馈，给前端弹 toast 用。
type AutoSetupClashResult struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Group     string `json:"group,omitempty"`     // 自动选中的组名
	NodeCount int    `json:"node_count,omitempty"` // 真节点数
	From      string `json:"from,omitempty"`       // 切换前节点
	To        string `json:"to,omitempty"`         // 立即切换后的节点
}

// AutoSetupClash 一键智能启用 Clash IP 轮换：
//  1. 探活控制器（要求用户已在设置里填了 controller_url + secret）
//  2. 自动挑选节点最多的 selector group
//  3. 写回 settings 并重启 rotator
//  4. 立即触发一次切换，验证端到端有效
//  5. 返回切换前后节点 + 真节点数，给 UI 显示
//
// 这是用户「点一下就好」的入口；用户不需要懂 group / 白名单 / type 的概念。
func (a *App) AutoSetupClash() AutoSetupClashResult {
	if a.store == nil {
		return AutoSetupClashResult{Error: "store 未初始化"}
	}
	settings := a.store.GetSettings()
	url := strings.TrimSpace(settings.ClashControllerURL)
	if url == "" {
		return AutoSetupClashResult{
			Error: "请先在「Clash IP 轮换」面板里填写控制器地址（如 http://127.0.0.1:9097）",
			Hint:  "Verge 默认 9097；Mihomo 默认 9090；ClashX 默认 9090。",
		}
	}

	// ① 探活
	probe := services.ProbeClashController(url, strings.TrimSpace(settings.ClashSecret))
	if !probe.OK {
		return AutoSetupClashResult{
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
		return AutoSetupClashResult{Error: auto.Error, Hint: hint}
	}

	// ③ 写回 settings —— 注意保留用户已配置的其它字段
	settings.ClashRotateEnabled = true
	settings.ClashGroup = auto.Group
	// ★ 强制开启「限速自动切」：用户期望「智能启用 = 一切自动」，
	// 之前若显式关过这个开关也会被覆盖。否则 rate-limit 触发时不切，
	// 用户会困惑「为什么 IDE 报 rate limit 但 IP 没变」。
	settings.ClashRotateOnRateLimit = true
	// 不主动写白名单：让 type-aware 过滤兜底，避免覆盖用户现有白名单
	if err := a.store.UpdateSettings(settings); err != nil {
		return AutoSetupClashResult{Error: "保存设置失败: " + err.Error()}
	}

	// ④ 重启 rotator（applyClashRotatorSettings 会读最新 settings）
	a.applyClashRotatorSettings()

	// ⑤ 立即触发一次切换并捕获 from→to
	a.mu.Lock()
	r := a.clashRotator
	a.mu.Unlock()
	if r == nil {
		return AutoSetupClashResult{
			Error: "rotator 启动失败（applyClashRotatorSettings 未创建实例）",
			Group: auto.Group, NodeCount: auto.NodeCount,
		}
	}

	// 先记下当前节点
	_, _, fromBefore := r.Stats()
	r.TriggerRotate("manual")

	// 等最多 3s 让 rotateOnce 完成（loop 协程异步执行）
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

	return AutoSetupClashResult{
		OK:        true,
		Group:     auto.Group,
		NodeCount: auto.NodeCount,
		From:      fromBefore,
		To:        to,
	}
}

// AutoDetectClashGroup 仅做检测，不写设置 —— 用于 UI 的「自动检测」按钮预览。
func (a *App) AutoDetectClashGroup() services.AutoDetectClashGroupResult {
	if a.store == nil {
		return services.AutoDetectClashGroupResult{Error: "store 未初始化"}
	}
	s := a.store.GetSettings()
	return services.AutoDetectClashGroup(
		strings.TrimSpace(s.ClashControllerURL),
		strings.TrimSpace(s.ClashSecret),
	)
}
