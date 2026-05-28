package main

import (
	"strings"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// applyOpenAIRelaySettings 按 settings.OpenAIRelayEnabled / Port / Secret 决定
// 启动 / 停止 / 重启 relay server。
//
// 调用时机：
//   - app.go 启动结束（恢复用户上次的开关状态）
//   - app_settings.go UpdateSettings（前端 Settings 改开关时立即生效）
//
// 行为矩阵：
//   - enabled=false + running     → Stop
//   - enabled=true  + not running → Start(port, secret)
//   - enabled=true  + running 但参数变了 → Stop + Start
//   - 其余情况 → no-op
//
// 错误仅记日志、不中断 — relay 端口冲突等不应阻塞 UpdateSettings 流程。
func (a *App) applyOpenAIRelaySettings() {
	if a.openaiRelay == nil || a.store == nil {
		return
	}
	s := a.store.GetSettings()
	port := s.OpenAIRelayPort
	if port <= 0 {
		port = 8787
	}
	secret := strings.TrimSpace(s.OpenAIRelaySecret)
	running, curPort, curSecret := a.openaiRelay.RuntimeConfig()

	switch {
	case !s.OpenAIRelayEnabled && running:
		if err := a.openaiRelay.Stop(); err != nil {
			utils.DLog("[OpenAI Relay] Stop 失败: %v", err)
		} else {
			utils.DLog("[OpenAI Relay] 按 settings 停止")
		}
	case s.OpenAIRelayEnabled && !running:
		if err := a.openaiRelay.Start(port, secret); err != nil {
			utils.DLog("[OpenAI Relay] Start 失败 port=%d: %v", port, err)
		} else {
			utils.DLog("[OpenAI Relay] 按 settings 启动 port=%d", port)
		}
	case s.OpenAIRelayEnabled && running && (curPort != port || curSecret != secret):
		if err := a.openaiRelay.Stop(); err != nil {
			utils.DLog("[OpenAI Relay] 重启-Stop 失败: %v", err)
			return
		}
		if err := a.openaiRelay.Start(port, secret); err != nil {
			utils.DLog("[OpenAI Relay] 重启-Start 失败 port=%d: %v", port, err)
		} else {
			utils.DLog("[OpenAI Relay] 参数变更，已重启 port=%d", port)
		}
	}
}

// StartOpenAIRelay 启动 OpenAI 兼容中转服务器。
// 同时把 settings.OpenAIRelayEnabled 置 true 并持久化，保证：
//  1. Settings UI 上「OpenAI 兼容中转」开关与 Relay UI 上的开关状态一致
//  2. 重启 app 后 applyOpenAIRelaySettings 能自动恢复 server
func (a *App) StartOpenAIRelay(port int, secret string) error {
	if a.openaiRelay == nil {
		return nil
	}
	if err := a.openaiRelay.Start(port, secret); err != nil {
		return err
	}
	if a.store != nil {
		s := a.store.GetSettings()
		changed := false
		if !s.OpenAIRelayEnabled {
			s.OpenAIRelayEnabled = true
			changed = true
		}
		if port > 0 && s.OpenAIRelayPort != port {
			s.OpenAIRelayPort = port
			changed = true
		}
		if s.OpenAIRelaySecret != secret {
			s.OpenAIRelaySecret = secret
			changed = true
		}
		if changed {
			_ = a.store.UpdateSettings(s)
		}
	}
	return nil
}

// StopOpenAIRelay 停止中转服务器。
// 同时把 settings.OpenAIRelayEnabled 置 false，避免重启 app 后又被自动拉起。
func (a *App) StopOpenAIRelay() error {
	if a.openaiRelay == nil {
		return nil
	}
	if err := a.openaiRelay.Stop(); err != nil {
		return err
	}
	if a.store != nil {
		s := a.store.GetSettings()
		if s.OpenAIRelayEnabled {
			s.OpenAIRelayEnabled = false
			_ = a.store.UpdateSettings(s)
		}
	}
	return nil
}

// GetOpenAIRelayStatus 获取中转服务器状态
func (a *App) GetOpenAIRelayStatus() services.OpenAIRelayStatus {
	if a.openaiRelay == nil {
		return services.OpenAIRelayStatus{}
	}
	return a.openaiRelay.Status()
}

// GetUsageRecords 获取全局调用记录
func (a *App) GetUsageRecords(limit int) []services.UsageRecord {
	if a.usageTracker == nil {
		return nil
	}
	return a.usageTracker.GetRecords(limit)
}

// GetUsageSummary 获取全局调用统计汇总
func (a *App) GetUsageSummary() services.UsageSummary {
	if a.usageTracker == nil {
		return services.UsageSummary{}
	}
	return a.usageTracker.GetSummary()
}

// DeleteAllUsage 清空所有调用记录
func (a *App) DeleteAllUsage() int {
	if a.usageTracker == nil {
		return 0
	}
	// Note: usageTracker now handles the DB truncation.
	return a.usageTracker.DeleteAll()
}
