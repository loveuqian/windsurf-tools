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

// SendDesktopNotification 让前端能主动触发桌面通知。
//
// 入参 kind: "info" | "warn" | "success" | "error"。
// eventKey: 60s 去重键，避免连续触发同事件刷屏。
// 在 settings.desktop_notifications=false 时静默吃掉调用。
//
// 用法（前端）：
//
//	APIInfo.sendDesktopNotification("success", "switch-mitm",
//	  "MITM 切号成功", "已切到 user@example.com")
func (a *App) SendDesktopNotification(kind, eventKey, title, body string) {
	if a == nil || a.notifier == nil {
		return
	}
	var k NotifyKind
	switch kind {
	case "warn", "warning":
		k = NotifyKindWarn
	case "success":
		k = NotifyKindSuccess
	case "error":
		k = NotifyKindError
	default:
		k = NotifyKindInfo
	}
	a.notifier.Send(k, eventKey, title, body)
}
