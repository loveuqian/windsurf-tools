// Package pin ── 手动锁定（Manual Pin）逻辑。
//
// 设计动机：
//
//	用户手动切到 account-A 后，3 个自动切号触发点（onKeyExhausted /
//	hot-poll / 定期同步 quota）经常又把账号换走。用户失去对当前激活账号
//	的控制感。
//
//	解法：手动切号成功后，自动 set ManualPinEnabled=true + ManualPinAccountID=id。
//	后续 3 个自动切的入口都先检查 pin 状态，pin 中直接 skip + 日志。
//	只能 Unpin 主动解除。这样用户 100% 控制。
package pin

import (
	"strings"

	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/utils"
)

// SettingsStore 描述 pin 模块需要的 store 能力（GetSettings/UpdateSettings/GetAccount）。
// 真实实现是 *store.Store；用接口仅是为了便于单测替身。
type SettingsStore interface {
	GetSettings() models.Settings
	UpdateSettings(models.Settings) error
	GetAccount(id string) (models.Account, error)
}

// NotifyFn 解锁通知回调；nil 时不发通知。
//
//	参数：(eventKey, title, body)
//	真实场景：App 注入 a.notify(NotifyKindInfo, ...) 包装。
type NotifyFn func(eventKey, title, body string)

// Status 给 App 透传给 UI 用；与 main 包对外暴露的 ManualPinStatus 字段一一对应。
type Status struct {
	Enabled   bool
	AccountID string
	Email     string
	Nickname  string
}

// Module 持有依赖；自身无运行时状态，所有锁定信息都落在 settings 中。
type Module struct {
	store  SettingsStore
	notify NotifyFn
}

// New 构造 pin 模块。store 必传，notify 可为 nil（解锁时不弹桌面通知）。
func New(s SettingsStore, n NotifyFn) *Module {
	return &Module{store: s, notify: n}
}

// Get 返回当前锁定状态 + 锁定账号的 email/nickname（便利字段）。
func (m *Module) Get() Status {
	if m == nil || m.store == nil {
		return Status{}
	}
	s := m.store.GetSettings()
	st := Status{
		Enabled:   s.ManualPinEnabled,
		AccountID: s.ManualPinAccountID,
	}
	if !st.Enabled || st.AccountID == "" {
		return st
	}
	if acc, err := m.store.GetAccount(st.AccountID); err == nil {
		st.Email = acc.Email
		st.Nickname = acc.Nickname
	}
	return st
}

// IsPinned 返回当前是否处于 Pin 状态。3 个自动切号 hook 用它做 guard。
func (m *Module) IsPinned() bool {
	if m == nil || m.store == nil {
		return false
	}
	return m.store.GetSettings().ManualPinEnabled
}

// Set 锁定到指定账号。手动切号成功后调用。
// 返回 false 表示 store 未初始化或 ID 为空，调用方一般忽略。
func (m *Module) Set(accountID string) bool {
	if m == nil || m.store == nil {
		return false
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return false
	}
	s := m.store.GetSettings()
	if s.ManualPinEnabled && s.ManualPinAccountID == accountID {
		return true // 已 pin 同一账号，no-op
	}
	s.ManualPinEnabled = true
	s.ManualPinAccountID = accountID
	if err := m.store.UpdateSettings(s); err != nil {
		utils.DLog("[Pin] setManualPin 写 settings 失败: %v", err)
		return false
	}
	utils.DLog("[Pin] 已锁定到 account=%s，3 个自动切换通道暂停", accountID[:minStr(8, len(accountID))])
	return true
}

// Unpin 解除手动锁定，恢复自动切换行为。
//
//	不在 Pin 状态时是 no-op，返回 nil。
//	如果传入了 NotifyFn，会发一条「账号锁定已解除」桌面通知。
func (m *Module) Unpin() error {
	if m == nil || m.store == nil {
		return nil
	}
	s := m.store.GetSettings()
	if !s.ManualPinEnabled {
		return nil // idempotent
	}
	pinnedID := s.ManualPinAccountID
	s.ManualPinEnabled = false
	s.ManualPinAccountID = ""
	if err := m.store.UpdateSettings(s); err != nil {
		return err
	}
	utils.DLog("[Pin] 手动解锁完成，所有自动切换已恢复")

	if m.notify != nil {
		pinnedLabel := pinnedID[:minStr(8, len(pinnedID))]
		if acc, err := m.store.GetAccount(pinnedID); err == nil && acc.Email != "" {
			pinnedLabel = acc.Email
		}
		m.notify("pin-unlocked",
			"账号锁定已解除",
			"已解锁 "+pinnedLabel+"，自动切换通道恢复")
	}
	return nil
}

func minStr(a, b int) int {
	if a < b {
		return a
	}
	return b
}
