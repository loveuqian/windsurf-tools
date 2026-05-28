package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/utils"
)

// ═══════════════════════════════════════
// 设置与代理
// ═══════════════════════════════════════

func (a *App) GetSettings() models.Settings { return a.store.GetSettings() }

// clampSettings 对所有有明确数值语义的字段做范围钳制，防止前端或导入的非法值
// （如 OpenAIRelayPort=70000、负间隔、超大并发）原样落库导致监听失败 / 死循环等。
// 必须在 store.UpdateSettings 之前调用，确保钳制后的值既下发又持久化。
// 范围依据 backend/models/settings.go 各字段注释 + 已有 clamp 常量。
func clampSettings(s *models.Settings) {
	if s == nil {
		return
	}

	// ── 端口 ── OpenAI 中转监听端口，必须在合法 TCP 端口区间，非法/<=0 回默认 8787
	if s.OpenAIRelayPort <= 0 || s.OpenAIRelayPort > 65535 {
		s.OpenAIRelayPort = 8787
	}

	// ── 并发 ──
	// ConcurrentLimit 通用批量并发，[1,20]
	if s.ConcurrentLimit < 1 {
		s.ConcurrentLimit = 1
	} else if s.ConcurrentLimit > 20 {
		s.ConcurrentLimit = 20
	}
	// ImportConcurrency 导入并发，[1,20]（默认 3）
	if s.ImportConcurrency < 1 {
		s.ImportConcurrency = 1
	} else if s.ImportConcurrency > 20 {
		s.ImportConcurrency = 20
	}

	// ── 额度热轮询 ── 复用 app_quota.go 已有 clamp（[5,60]），保持一致
	s.QuotaHotPollSeconds = clampQuotaHotPollSeconds(s.QuotaHotPollSeconds)

	// ── 自定义同步间隔 ── 复用 utils 已有 clamp（[5,10080]，非法回默认 360）
	s.QuotaCustomIntervalMinutes = utils.ClampQuotaCustomIntervalMinutes(s.QuotaCustomIntervalMinutes)

	// ── 切号冷却 ── SwitchCooldownBaseSec 注释范围 [30,3600]，默认 300
	if s.SwitchCooldownBaseSec < 30 {
		s.SwitchCooldownBaseSec = 30
	} else if s.SwitchCooldownBaseSec > 3600 {
		s.SwitchCooldownBaseSec = 3600
	}

	// ── 轮换池 ──
	// RotationPoolIntervalMin 定时切间隔，注释范围 [1,60]，默认 5
	if s.RotationPoolIntervalMin < 1 {
		s.RotationPoolIntervalMin = 1
	} else if s.RotationPoolIntervalMin > 60 {
		s.RotationPoolIntervalMin = 60
	}
	// RotationPoolQuotaRefreshMin 额度刷新间隔，注释范围 [1,10]，默认 1
	if s.RotationPoolQuotaRefreshMin < 1 {
		s.RotationPoolQuotaRefreshMin = 1
	} else if s.RotationPoolQuotaRefreshMin > 10 {
		s.RotationPoolQuotaRefreshMin = 10
	}

	// ── Clash 轮换 ──
	// ClashIntervalMinutes 轮换间隔（分钟），注释范围 [2,60]，默认 8
	if s.ClashIntervalMinutes < 2 {
		s.ClashIntervalMinutes = 2
	} else if s.ClashIntervalMinutes > 60 {
		s.ClashIntervalMinutes = 60
	}
	// ClashLatencyMaxMs 延迟上限毫秒，0=跳过测速；负值无意义归 0
	if s.ClashLatencyMaxMs < 0 {
		s.ClashLatencyMaxMs = 0
	}

	// ── 窗口尺寸 ── 宽/高为负无意义归 0（0 表示走内置默认）；X/Y 允许 -1（居中）
	if s.WindowWidth < 0 {
		s.WindowWidth = 0
	}
	if s.WindowHeight < 0 {
		s.WindowHeight = 0
	}
}

func (a *App) UpdateSettings(settings models.Settings) error {
	prev := a.store.GetSettings()
	clampSettings(&settings)
	if err := a.store.UpdateSettings(settings); err != nil {
		return err
	}
	if a.mitmProxy != nil {
		a.mitmProxy.SetWindsurfService(a.windsurfSvc)
	}
	if settings.AutoRefreshTokens {
		a.mu.Lock()
		running := a.cancelAutoRefresh != nil
		a.mu.Unlock()
		if !running {
			a.startAutoRefresh()
		}
	} else {
		a.mu.Lock()
		cancel := a.cancelAutoRefresh
		a.cancelAutoRefresh = nil
		a.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	if settings.AutoRefreshQuotas {
		a.mu.Lock()
		running := a.cancelAutoQuotaRefresh != nil
		a.mu.Unlock()
		if !running {
			a.startAutoQuotaRefresh()
		}
	} else {
		a.mu.Lock()
		cancel := a.cancelAutoQuotaRefresh
		a.cancelAutoQuotaRefresh = nil
		a.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	a.restartQuotaHotPollIfNeeded()
	a.syncMitmPoolKeys()
	a.syncForgeConfig()
	a.syncStaticCacheConfig()
	a.syncJailbreakConfig()
	a.syncMitmStickyFromPin()
	a.syncMitmAutoSwitchOnQuotaExhausted()
	a.syncMitmDebugAndCapture()
	// F7-REMOVAL: 下一行删除
	a.syncSmartFriendConfig()
	a.applyOpenAIRelaySettings()
	a.applyClashRotatorSettings()
	a.applyRotationPoolSettings()
	// 代理配置变更时刷新 transport 池 + 同步代理 URL 给 MITM / Relay
	a.refreshTransportPool()
	// 动态切换调试日志
	if prev.DebugLog != settings.DebugLog {
		utils.InitDebugLogger(a.store.DataDir(), settings.DebugLog)
		if settings.DebugLog {
			utils.DLog("[设置] 调试日志已开启")
		}
	}
	return nil
}

// ExportSettings 返回当前 settings 的 JSON 字符串，前端走 SaveDialog 写文件。
//
// 自动剔除敏感信息（OpenAIRelaySecret / ClashSecret / ManualPinAccountID /
// RotationPoolAccountIDs），避免用户误把含敏感凭证的配置分享出去。
// 用户在新机器恢复时需要单独填这些字段。
func (a *App) ExportSettings() (string, error) {
	if a.store == nil {
		return "", fmt.Errorf("store 未初始化")
	}
	s := a.store.GetSettings()
	// 拷贝一份，剥敏感
	exported := s
	exported.OpenAIRelaySecret = ""
	exported.ClashSecret = ""
	exported.ManualPinEnabled = false
	exported.ManualPinAccountID = ""
	exported.RotationPoolAccountIDs = nil
	exported.MitmJailbreakOverride = "" // 用户自定义文本可能含个人化指令，剥掉
	data, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化失败: %w", err)
	}
	return string(data), nil
}

// ImportSettings 从 JSON 字符串导入 settings 并覆盖当前值。
//
// 安全策略：
//   - JSON parse 失败 → 返回 error 不动 store
//   - 保留当前 OpenAIRelaySecret / ClashSecret / ManualPin / RotationPool
//     等敏感 / 运行时字段（导入只覆盖偏好类设置，不动凭证类）
//   - 导入完成后走 UpdateSettings 触发所有 sync hook
func (a *App) ImportSettings(jsonText string) error {
	if a.store == nil {
		return fmt.Errorf("store 未初始化")
	}
	jsonText = strings.TrimSpace(jsonText)
	if jsonText == "" {
		return fmt.Errorf("文件内容为空")
	}
	var imported models.Settings
	if err := json.Unmarshal([]byte(jsonText), &imported); err != nil {
		return fmt.Errorf("解析失败 — 请确认是 settings.json 格式: %w", err)
	}
	// 保留当前敏感字段
	current := a.store.GetSettings()
	imported.OpenAIRelaySecret = current.OpenAIRelaySecret
	imported.ClashSecret = current.ClashSecret
	imported.ManualPinEnabled = current.ManualPinEnabled
	imported.ManualPinAccountID = current.ManualPinAccountID
	imported.RotationPoolAccountIDs = current.RotationPoolAccountIDs
	if imported.MitmJailbreakOverride == "" {
		imported.MitmJailbreakOverride = current.MitmJailbreakOverride
	}
	return a.UpdateSettings(imported)
}

// refreshTransportPool 刷新全局 transport 池 + 同步代理 URL 给 MITM / Relay。
func (a *App) refreshTransportPool() {
	if a.transportPool == nil {
		return
	}
	a.transportPool.Refresh()
	proxyURL := a.transportPool.RawProxyURL()
	if a.mitmProxy != nil {
		a.mitmProxy.SetUpstreamProxy(proxyURL)
	}
	if a.openaiRelay != nil {
		a.openaiRelay.SetUpstreamProxy(proxyURL)
	}
}
