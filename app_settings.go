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

func (a *App) UpdateSettings(settings models.Settings) error {
	prev := a.store.GetSettings()
	if err := a.store.UpdateSettings(settings); err != nil {
		return err
	}
	if a.mitmProxy != nil {
		a.mitmProxy.SetWindsurfService(a.windsurfSvc)
	}
	if settings.AutoRefreshTokens {
		if a.cancelAutoRefresh == nil {
			a.startAutoRefresh()
		}
	} else {
		if a.cancelAutoRefresh != nil {
			a.cancelAutoRefresh()
			a.cancelAutoRefresh = nil
		}
	}
	if settings.AutoRefreshQuotas {
		if a.cancelAutoQuotaRefresh == nil {
			a.startAutoQuotaRefresh()
		}
	} else {
		if a.cancelAutoQuotaRefresh != nil {
			a.cancelAutoQuotaRefresh()
			a.cancelAutoQuotaRefresh = nil
		}
	}
	a.restartQuotaHotPollIfNeeded()
	a.syncMitmPoolKeys()
	a.syncForgeConfig()
	a.syncStaticCacheConfig()
	a.syncJailbreakConfig()
	// F7-REMOVAL: 下一行删除
	a.syncSmartFriendConfig()
	a.applyClashRotatorSettings()
	a.applyRotationPoolSettings()
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
