// Package notify ── 跨平台桌面通知。
//
// 设计动机：
//
//	用户开多 app 时（IDE、浏览器、终端…），windsurf-tools 经常被遮挡在
//	后台。MITM 自动切号 / Clash 错误 / Pin 解除 等关键事件如果只在 app
//	内 toast 就被错过。系统通知中心可以让用户在任何前台 app 都能感知到。
//
// 实现：
//
//	macOS osascript / Windows powershell / Linux notify-send，每平台
//	独立 fallback。失败静默 + DLog，不阻塞调用方。
//
// 频率控制：
//
//	同一 event_key 60s 内只触发一次（默认 dedupWin），避免 Clash 错误
//	连续 10 条一起弹。
//
// 用户开关：
//
//	由调用方注入的 EnabledFn 决定是否真的发；典型实现是
//	settings.DesktopNotifications。
package notify

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"windsurf-tools-wails/backend/utils"
)

// Kind 通知类型，对应不同图标 / 紧急程度。
type Kind string

const (
	KindInfo    Kind = "info"
	KindWarn    Kind = "warn"
	KindSuccess Kind = "success"
	KindError   Kind = "error"
)

// EnabledFn 由调用方注入：返回当前 settings 中是否开启桌面通知。
type EnabledFn func() bool

// Module 持有去重状态，调用方通过 New 注入「是否开启」判定函数。
type Module struct {
	enabled  EnabledFn
	mu       sync.Mutex
	lastSent map[string]time.Time
	dedupWin time.Duration
}

// New 构造一个通知模块。enabled 为 nil 时所有 Send 都会被忽略（视作关闭）。
func New(enabled EnabledFn) *Module {
	return &Module{
		enabled:  enabled,
		lastSent: make(map[string]time.Time),
		dedupWin: 60 * time.Second,
	}
}

// Send 触发桌面通知。
//   - kind 决定图标 / 紧急程度
//   - eventKey 用作去重键，同 key dedupWin 内只触发一次
//   - title / body 用户可见
//
// 如果 enabled 返回 false 或后台调用失败，silently 退回。
func (m *Module) Send(kind Kind, eventKey, title, body string) {
	if m == nil {
		return
	}
	if m.enabled == nil || !m.enabled() {
		return
	}

	m.mu.Lock()
	if last, ok := m.lastSent[eventKey]; ok && time.Since(last) < m.dedupWin {
		m.mu.Unlock()
		return
	}
	m.lastSent[eventKey] = time.Now()
	m.mu.Unlock()

	// 异步触发，不阻塞调用方（osascript 启动有 50-100ms 开销）
	go sendSystemNotification(kind, title, body)
}

// sendSystemNotification 调系统命令发通知。失败时静默忽略（log debug）。
func sendSystemNotification(kind Kind, title, body string) {
	safeTitle := escapeForShell(title)
	safeBody := escapeForShell(body)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(
			`display notification "%s" with title "%s"`,
			safeBody, kindEmoji(kind)+" "+safeTitle,
		)
		cmd = exec.Command("osascript", "-e", script)
	case "windows":
		cmd = buildWindowsNotifyCommand(kind, safeTitle, safeBody)
		if cmd == nil {
			utils.DLog("[Notify] Windows 未找到 powershell/pwsh，已降级到 app 内 toast")
			return
		}
	default: // linux + bsd + others
		cmd = buildLinuxNotifyCommand(kind, safeTitle, safeBody)
		if cmd == nil {
			utils.DLog("[Notify] Linux 未找到 notify-send/gdbus —— 执行 `sudo apt install libnotify-bin` 启用桌面通知")
			return
		}
	}
	if err := cmd.Start(); err != nil {
		utils.DLog("[Notify] 触发系统通知失败: %v (kind=%s title=%q)", err, kind, title)
	}
}

// buildWindowsNotifyCommand 选 powershell 或 pwsh，构造 BalloonTip 命令。
// 中文标题/正文用单引号包裹避免双引号转义带来的 PowerShell parser 问题。
func buildWindowsNotifyCommand(kind Kind, title, body string) *exec.Cmd {
	psTitle := strings.ReplaceAll(title, "'", "''")
	psBody := strings.ReplaceAll(body, "'", "''")
	iconType := "Information"
	switch kind {
	case KindError:
		iconType = "Error"
	case KindWarn:
		iconType = "Warning"
	}
	psScript := fmt.Sprintf(
		`Add-Type -AssemblyName System.Windows.Forms;`+
			`$n = New-Object System.Windows.Forms.NotifyIcon;`+
			`$n.Icon = [System.Drawing.SystemIcons]::%s;`+
			`$n.BalloonTipTitle = '%s';`+
			`$n.BalloonTipText = '%s';`+
			`$n.Visible = $true;`+
			`$n.ShowBalloonTip(5000);`+
			`Start-Sleep -s 6;`+
			`$n.Dispose()`,
		iconType, psTitle, psBody,
	)
	for _, candidate := range []string{"powershell.exe", "powershell", "pwsh.exe", "pwsh"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return exec.Command(candidate, "-NoProfile", "-NonInteractive", "-Command", psScript)
		}
	}
	return nil
}

// buildLinuxNotifyCommand 选 notify-send 或 gdbus fallback。
func buildLinuxNotifyCommand(kind Kind, title, body string) *exec.Cmd {
	if _, err := exec.LookPath("notify-send"); err == nil {
		return exec.Command("notify-send", "-a", "Windsurf Tools",
			"-i", linuxIcon(kind),
			kindEmoji(kind)+" "+title, body)
	}
	if _, err := exec.LookPath("gdbus"); err == nil {
		return exec.Command("gdbus", "call",
			"--session", "--dest", "org.freedesktop.Notifications",
			"--object-path", "/org/freedesktop/Notifications",
			"--method", "org.freedesktop.Notifications.Notify",
			"Windsurf Tools", "0", linuxIcon(kind),
			kindEmoji(kind)+" "+title, body,
			"[]", "{}", "5000")
	}
	return nil
}

func kindEmoji(kind Kind) string {
	switch kind {
	case KindError:
		return "❌"
	case KindWarn:
		return "⚠️"
	case KindSuccess:
		return "✅"
	default:
		return "ℹ️"
	}
}

func linuxIcon(kind Kind) string {
	switch kind {
	case KindError:
		return "dialog-error"
	case KindWarn:
		return "dialog-warning"
	case KindSuccess:
		return "emblem-ok"
	default:
		return "dialog-information"
	}
}

// escapeForShell 简单替换特殊字符防止脚本注入。
// 不追求完整 escape — title/body 都是受控字符串（账号 email / Clash 错误
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
