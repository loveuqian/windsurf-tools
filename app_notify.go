package main

// app_notify.go ── 薄壳。真正实现已迁到 backend/app/notify。
//   - 兼容旧调用：保留 NotifyKindXxx 常量与 a.notify(...) 方法，签名不变。
//   - Module 状态由 App 持有（initBackend 中构造），不再使用包级全局变量。

import (
	"windsurf-tools-wails/backend/app/notify"
)

// NotifyKind 类型别名，沿用旧名以避免散落各处的调用点都跟着改。
type NotifyKind = notify.Kind

const (
	NotifyKindInfo    = notify.KindInfo
	NotifyKindWarn    = notify.KindWarn
	NotifyKindSuccess = notify.KindSuccess
	NotifyKindError   = notify.KindError
)

// notify 触发桌面通知；调用面与拆分前一致。
func (a *App) notify(kind NotifyKind, eventKey, title, body string) {
	if a == nil || a.notifier == nil {
		return
	}
	a.notifier.Send(kind, eventKey, title, body)
}
