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
