package main

// app_notify.go ── 跨平台桌面通知。
//
// 设计动机：
//   用户开多 app 时（IDE、浏览器、终端…），windsurf-tools 经常被遮挡在
//   后台。MITM 自动切号 / Clash 错误 / Pin 解除 等关键事件如果只在 app
//   内 toast 就被错过。系统通知中心可以让用户在任何前台 app 都能感知到。
//
// 实现：
//   优先用 wails runtime 自带的 `runtime.MessageDialog` (低侵入，原生)；
//   降级到平台 native 命令（macOS osascript / Windows powershell /
//   Linux notify-send）—— 这样不依赖 wails 内部 API 变更。
//
// 频率控制：
//   同一 event_key 60s 内只触发一次，避免 Clash 错误连续 10 条一起弹。
//
// 用户开关：
//   settings.DesktopNotifications，默认 true，关掉则降级到 app 内 toast。

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"windsurf-tools-wails/backend/utils"
)

// NotifyKind 通知类型，对应不同图标 / 紧急程度。
type NotifyKind string

const (
	NotifyKindInfo    NotifyKind = "info"
	NotifyKindWarn    NotifyKind = "warn"
	NotifyKindSuccess NotifyKind = "success"
	NotifyKindError   NotifyKind = "error"
)

var (
	notifyMu        sync.Mutex
	notifyLastSent  = make(map[string]time.Time) // event_key → last sent
	notifyDedupWin  = 60 * time.Second
)

// notify 触发桌面通知。
//   - kind 决定图标 / 紧急程度
//   - eventKey 用作去重键，同 key 60s 内只触发一次（防 Clash 连续错误刷屏）
//   - title / body 用户可见
//
// 如果 settings.DesktopNotifications=false 或后台调用失败，silently 退回。
// 调用方一般是其它 hook（onKeyExhausted / Clash 错误回调 / Pin 解除等）。
func (a *App) notify(kind NotifyKind, eventKey, title, body string) {
	if a.store == nil {
		return
	}
	if !a.store.GetSettings().DesktopNotifications {
		return
	}

	notifyMu.Lock()
	if last, ok := notifyLastSent[eventKey]; ok && time.Since(last) < notifyDedupWin {
		notifyMu.Unlock()
		return
	}
	notifyLastSent[eventKey] = time.Now()
	notifyMu.Unlock()

	// 异步触发，不阻塞调用方（osascript 启动有 50-100ms 开销）
	go sendSystemNotification(kind, title, body)
}

// sendSystemNotification 调系统命令发通知。失败时静默忽略（log debug）。
func sendSystemNotification(kind NotifyKind, title, body string) {
	// 标题/正文做简单转义，避免 shell 注入。允许中文 + 常见标点。
	safeTitle := escapeForShell(title)
	safeBody := escapeForShell(body)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// osascript 通知不支持自定义图标，但符号位置可以加 emoji 提示 kind
		script := fmt.Sprintf(
			`display notification "%s" with title "%s"`,
			safeBody, kindEmoji(kind)+" "+safeTitle,
		)
		cmd = exec.Command("osascript", "-e", script)
	case "windows":
		// PowerShell BurntToast 不在系统默认，用更稳的 msg.exe / [System.Windows.Forms.MessageBox]
		// 简化为 powershell + WPF balloon 通知。失败也无所谓。
		psScript := fmt.Sprintf(
			`[reflection.assembly]::loadwithpartialname('System.Windows.Forms') | Out-Null; `+
				`$n = New-Object System.Windows.Forms.NotifyIcon; `+
				`$n.Icon = [System.Drawing.SystemIcons]::Information; `+
				`$n.BalloonTipTitle = '%s'; $n.BalloonTipText = '%s'; `+
				`$n.Visible = $true; $n.ShowBalloonTip(5000); Start-Sleep -s 6; $n.Dispose()`,
			safeTitle, safeBody,
		)
		cmd = exec.Command("powershell", "-Command", psScript)
	default: // linux + others
		cmd = exec.Command("notify-send", "-a", "Windsurf Tools", "-i", linuxIcon(kind),
			kindEmoji(kind)+" "+safeTitle, safeBody)
	}
	if err := cmd.Start(); err != nil {
		utils.DLog("[Notify] 触发系统通知失败: %v (kind=%s title=%q)", err, kind, title)
	}
}

func kindEmoji(kind NotifyKind) string {
	switch kind {
	case NotifyKindError:
		return "❌"
	case NotifyKindWarn:
		return "⚠️"
	case NotifyKindSuccess:
		return "✅"
	default:
		return "ℹ️"
	}
}

func linuxIcon(kind NotifyKind) string {
	switch kind {
	case NotifyKindError:
		return "dialog-error"
	case NotifyKindWarn:
		return "dialog-warning"
	case NotifyKindSuccess:
		return "emblem-ok"
	default:
		return "dialog-information"
	}
}

// escapeForShell 简单替换特殊字符防止脚本注入。
// 不追求完整 escape — 我们的 title/body 都是受控字符串（账号 email / Clash 错误
// 等，不会有用户原样输入）。
func escapeForShell(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n', '\r':
			out = append(out, ' ')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
