package main

// app_pin.go ── 手动锁定（Manual Pin）相关 API。
//
// 设计动机：
//   用户手动切到 account-A 后，3 个自动切号触发点（onKeyExhausted /
//   hot-poll / 定期同步 quota）经常又把账号换走。用户失去对当前激活账号
//   的控制感。
//
//   解法：手动切号成功后，自动 set ManualPinEnabled=true + ManualPinAccountID=id。
//   后续 3 个自动切的入口都先检查 pin 状态，pin 中直接 skip + 日志。
//   只能 UnpinManualAccount() 主动解除。这样用户 100% 控制。
//
// 与轮换池的关系：
//   Pin 优先级最高 — pin 时连轮换池的定时切换也跳过，保持纯手动状态。

import (
	"strings"
	"windsurf-tools-wails/backend/utils"
)

// ManualPinStatus 给前端 UI 显示。
type ManualPinStatus struct {
	Enabled   bool   `json:"enabled"`
	AccountID string `json:"account_id,omitempty"`
	Email     string `json:"email,omitempty"`     // 便利字段，方便 UI 直显
	Nickname  string `json:"nickname,omitempty"`  // 便利字段
}

// GetManualPinStatus 返回当前锁定状态 + 锁定账号的 email/nickname（便利字段）。
func (a *App) GetManualPinStatus() ManualPinStatus {
	if a.store == nil {
		return ManualPinStatus{}
	}
	s := a.store.GetSettings()
	st := ManualPinStatus{
		Enabled:   s.ManualPinEnabled,
		AccountID: s.ManualPinAccountID,
	}
	if !st.Enabled || st.AccountID == "" {
		return st
	}
	if acc, err := a.store.GetAccount(st.AccountID); err == nil {
		st.Email = acc.Email
		st.Nickname = acc.Nickname
	}
	return st
}

// UnpinManualAccount 解除手动锁定，恢复自动切换行为。
func (a *App) UnpinManualAccount() error {
	if a.store == nil {
		return nil
	}
	s := a.store.GetSettings()
	if !s.ManualPinEnabled {
		return nil // idempotent
	}
	pinnedID := s.ManualPinAccountID
	s.ManualPinEnabled = false
	s.ManualPinAccountID = ""
	if err := a.store.UpdateSettings(s); err != nil {
		return err
	}
	utils.DLog("[Pin] 手动解锁完成，所有自动切换已恢复")
	// 桌面通知：用户在 Settings 解锁可能是显式动作不需通知；但在
	// Account 卡片解锁时多数没看 toast。统一发，频控 60s 防误触。
	pinnedLabel := pinnedID[:min(8, len(pinnedID))]
	if acc, err := a.store.GetAccount(pinnedID); err == nil && acc.Email != "" {
		pinnedLabel = acc.Email
	}
	a.notify(NotifyKindInfo, "pin-unlocked",
		"账号锁定已解除",
		"已解锁 "+pinnedLabel+"，自动切换通道恢复")
	return nil
}

// setManualPin 内部使用：手动切号成功后调用，锁定当前账号。
// 返回 false 表示 store 不可用（一般不该发生）。
func (a *App) setManualPin(accountID string) bool {
	if a.store == nil {
		return false
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return false
	}
	s := a.store.GetSettings()
	if s.ManualPinEnabled && s.ManualPinAccountID == accountID {
		return true // 已 pin 同一账号，no-op
	}
	s.ManualPinEnabled = true
	s.ManualPinAccountID = accountID
	if err := a.store.UpdateSettings(s); err != nil {
		utils.DLog("[Pin] setManualPin 写 settings 失败: %v", err)
		return false
	}
	utils.DLog("[Pin] 已锁定到 account=%s，3 个自动切换通道暂停", accountID[:min(8, len(accountID))])
	return true
}

// isManuallyPinned 返回当前是否处于 Pin 状态。3 个自动切号 hook 用它做 guard。
func (a *App) isManuallyPinned() bool {
	if a.store == nil {
		return false
	}
	return a.store.GetSettings().ManualPinEnabled
}
