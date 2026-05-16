package main

// app_clash.go ── 薄壳。真正实现已迁到 backend/app/clash。
//   - 保留 Wails 暴露面：TestClashController / ListClashGroupNodes /
//     TriggerClashRotate / GetClashRotatorRunning / AutoDetectClashGroup /
//     AutoSetupClash，签名不变。
//   - AutoSetupClashResult 类型仍由 main 包持有，避免 wails binding 路径变化。

import "windsurf-tools-wails/backend/services"

// AutoSetupClashResult 「智能启用」按钮的反馈，给前端弹 toast 用。
// 字段必须与 wails binding main.AutoSetupClashResult 保持一致。
type AutoSetupClashResult struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Group     string `json:"group,omitempty"`
	NodeCount int    `json:"node_count,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
}

// applyClashRotatorSettings 在 initBackend / UpdateSettings 中调用。
func (a *App) applyClashRotatorSettings() {
	if a == nil || a.clashMod == nil {
		return
	}
	a.clashMod.ApplySettings()
}

// stopClashRotator 关停（在 shutdown 中调用）。
func (a *App) stopClashRotator() {
	if a == nil || a.clashMod == nil {
		return
	}
	a.clashMod.Stop()
}

// TestClashController 探活并返回 selector 组列表。
func (a *App) TestClashController(controllerURL, secret string) services.ClashProbeResult {
	if a == nil || a.clashMod == nil {
		return services.ProbeClashController(controllerURL, secret)
	}
	return a.clashMod.TestController(controllerURL, secret)
}

// ListClashGroupNodes 列出指定组内的节点（不含 DIRECT/REJECT/GLOBAL/组自身）。
func (a *App) ListClashGroupNodes(controllerURL, secret, group string) ([]string, error) {
	if a == nil || a.clashMod == nil {
		return services.ListClashGroupNodes(controllerURL, secret, group)
	}
	return a.clashMod.ListGroupNodes(controllerURL, secret, group)
}

// TriggerClashRotate UI「立即换 IP」按钮。
func (a *App) TriggerClashRotate() bool {
	if a == nil || a.clashMod == nil {
		return false
	}
	return a.clashMod.TriggerRotate()
}

// GetClashRotatorRunning rotator 是否在跑（Settings UI 状态展示）。
func (a *App) GetClashRotatorRunning() bool {
	if a == nil || a.clashMod == nil {
		return false
	}
	return a.clashMod.IsRunning()
}

// AutoDetectClashGroup 仅做检测、不写设置 —— 用于 UI 「自动检测」按钮预览。
func (a *App) AutoDetectClashGroup() services.AutoDetectClashGroupResult {
	if a == nil || a.clashMod == nil {
		return services.AutoDetectClashGroupResult{Error: "store 未初始化"}
	}
	return a.clashMod.AutoDetectGroup()
}

// AutoSetupClash 一键智能启用 Clash IP 轮换。
func (a *App) AutoSetupClash() AutoSetupClashResult {
	if a == nil || a.clashMod == nil {
		return AutoSetupClashResult{Error: "store 未初始化"}
	}
	r := a.clashMod.AutoSetup()
	return AutoSetupClashResult{
		OK:        r.OK,
		Error:     r.Error,
		Hint:      r.Hint,
		Group:     r.Group,
		NodeCount: r.NodeCount,
		From:      r.From,
		To:        r.To,
	}
}
