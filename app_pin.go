package main

// app_pin.go ── 薄壳。真正实现已迁到 backend/app/pin。
//   - 保留 Wails 暴露面：GetManualPinStatus / UnpinManualAccount，签名不变。
//   - 保留内部辅助：setManualPin / isManuallyPinned 给 mitm 切号 hook 复用。

// ManualPinStatus 给前端 UI 显示。字段必须与 Wails binding 中的
// main.ManualPinStatus 一致，因此保留在 main 包。
type ManualPinStatus struct {
	Enabled   bool   `json:"enabled"`
	AccountID string `json:"account_id,omitempty"`
	Email     string `json:"email,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
}

// GetManualPinStatus 返回当前锁定状态 + 锁定账号的 email/nickname（便利字段）。
func (a *App) GetManualPinStatus() ManualPinStatus {
	if a == nil || a.pinMod == nil {
		return ManualPinStatus{}
	}
	st := a.pinMod.Get()
	return ManualPinStatus{
		Enabled:   st.Enabled,
		AccountID: st.AccountID,
		Email:     st.Email,
		Nickname:  st.Nickname,
	}
}

// UnpinManualAccount 解除手动锁定，恢复自动切换行为。
func (a *App) UnpinManualAccount() error {
	if a == nil || a.pinMod == nil {
		return nil
	}
	err := a.pinMod.Unpin()
	// ★ Pin 解除时同步清除 MITM 粘性 key —— 新对话恢复 leastConnections 分散
	if err == nil && a.mitmProxy != nil {
		a.mitmProxy.SetStickyKey("")
	}
	return err
}

// setManualPin 内部使用：手动切号成功后调用，锁定当前账号。
// 返回 false 表示 store 不可用或 ID 为空（一般不该发生）。
func (a *App) setManualPin(accountID string) bool {
	if a == nil || a.pinMod == nil {
		return false
	}
	if !a.pinMod.Set(accountID) {
		return false
	}
	// ★ Pin 成功后把该账号的 apiKey 推到 MITM 作为 stickyKey ——
	// 之后所有新对话都强制走该 key，避免 leastConnections 把请求分散到其他号。
	a.syncMitmStickyFromPin()
	return true
}

// syncMitmStickyFromPin 从当前 settings 读 ManualPin 状态并把对应 apiKey 推给 MitmProxy。
// 启动时调用一次（恢复重启前的 pin 状态），setManualPin / Unpin 也调用。
func (a *App) syncMitmStickyFromPin() {
	if a == nil || a.mitmProxy == nil || a.store == nil {
		return
	}
	s := a.store.GetSettings()
	if !s.ManualPinEnabled || s.ManualPinAccountID == "" {
		a.mitmProxy.SetStickyKey("")
		return
	}
	acc, err := a.store.GetAccount(s.ManualPinAccountID)
	if err != nil {
		a.mitmProxy.SetStickyKey("")
		return
	}
	a.mitmProxy.SetStickyKey(acc.WindsurfAPIKey)
}

// isManuallyPinned 返回当前是否处于 Pin 状态。3 个自动切号 hook 用它做 guard。
func (a *App) isManuallyPinned() bool {
	if a == nil || a.pinMod == nil {
		return false
	}
	return a.pinMod.IsPinned()
}
