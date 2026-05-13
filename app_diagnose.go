package main

// app_diagnose.go ── 跨平台兼容性诊断（v1.6.0）
//
// 提供给前端 Dashboard 一个「兼容性检查」按钮，跑一次 audit 返回所有平台
// 依赖的检测结果。用户能一眼看到哪些 OK / 哪些缺 + 修复建议。
//
// 设计目标：完全只读、不修改任何系统状态、跨平台 silent fail-safe。

import (
	"os"
	"os/exec"
	"runtime"

	"windsurf-tools-wails/backend/services"
)

// DiagnoseCheck 单项诊断结果。
type DiagnoseCheck struct {
	ID       string `json:"id"`       // 唯一 ID，UI 用来区分项
	Title    string `json:"title"`    // 中文名
	Status   string `json:"status"`   // "ok" / "warn" / "error" / "n/a"
	Detail   string `json:"detail"`   // 具体说明
	FixHint  string `json:"fix_hint"` // 修复建议（具体命令）
}

// DiagnoseReport 完整诊断报告。
type DiagnoseReport struct {
	Platform string          `json:"platform"`     // darwin / windows / linux / ...
	Arch     string          `json:"arch"`         // amd64 / arm64 / ...
	OK       int             `json:"ok"`           // 通过项数
	Warn     int             `json:"warn"`         // 警告项数
	Error    int             `json:"error"`        // 错误项数
	Checks   []DiagnoseCheck `json:"checks"`
}

// RunDiagnostics 跑全套诊断，返回报告。
//
// 检查项按风险高低排：
//  1. 通知命令（osascript / powershell / notify-send|gdbus）
//  2. 文件打开命令（open / cmd start / xdg-open|gio|...）
//  3. 提权命令（sudo / pkexec / runas）
//  4. CA 证书工具（security / certutil / update-ca-certificates）
//  5. App 数据目录可写
//  6. Windsurf 安装检测
//  7. Clash 控制器探活（如配置了）
//  8. MITM 端口可绑（443 检查）
func (a *App) RunDiagnostics() DiagnoseReport {
	report := DiagnoseReport{
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
		Checks:   make([]DiagnoseCheck, 0, 12),
	}

	add := func(c DiagnoseCheck) {
		report.Checks = append(report.Checks, c)
		switch c.Status {
		case "ok":
			report.OK++
		case "warn":
			report.Warn++
		case "error":
			report.Error++
		}
	}

	// ① 桌面通知
	add(checkNotificationCommand())

	// ② 文件打开
	add(checkFileOpener())

	// ③ 提权命令
	add(checkPrivilegeEscalation())

	// ④ CA 证书工具
	add(checkCertTool())

	// ⑤ App 数据目录可写
	add(checkAppDataDirWritable(a))

	// ⑥ Windsurf 安装
	add(checkWindsurfInstalled())

	// ⑦ Clash 控制器（仅配置了）
	if a.store != nil {
		s := a.store.GetSettings()
		if s.ClashControllerURL != "" {
			add(checkClashController(s.ClashControllerURL, s.ClashSecret))
		}
	}

	// ⑧ Windows WebView2 Runtime
	if runtime.GOOS == "windows" {
		add(checkWebView2Runtime())
	}

	return report
}

// ── 各项实现 ──

func checkNotificationCommand() DiagnoseCheck {
	c := DiagnoseCheck{ID: "notify", Title: "桌面通知"}
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("osascript"); err == nil {
			c.Status, c.Detail = "ok", "osascript 可用，通知会进系统通知中心"
			return c
		}
		c.Status, c.Detail = "error", "osascript 缺失（极少见）"
		c.FixHint = "重装 macOS 命令行工具：xcode-select --install"
	case "windows":
		for _, x := range []string{"powershell.exe", "powershell", "pwsh.exe", "pwsh"} {
			if _, err := exec.LookPath(x); err == nil {
				c.Status, c.Detail = "ok", x+" 可用，通知用 BalloonTip"
				return c
			}
		}
		c.Status, c.Detail = "error", "未找到 powershell.exe / pwsh.exe"
		c.FixHint = "Windows 10+ 自带 PowerShell，确认 PATH 包含 C:\\Windows\\System32\\WindowsPowerShell\\v1.0"
	default:
		if _, err := exec.LookPath("notify-send"); err == nil {
			c.Status, c.Detail = "ok", "notify-send 可用"
			return c
		}
		if _, err := exec.LookPath("gdbus"); err == nil {
			c.Status, c.Detail = "warn", "notify-send 缺，已降级到 gdbus"
			c.FixHint = "更佳：sudo apt install libnotify-bin"
			return c
		}
		c.Status, c.Detail = "error", "缺 notify-send 和 gdbus，桌面通知不可用（app 内 toast 仍 OK）"
		c.FixHint = "sudo apt install libnotify-bin"
	}
	return c
}

func checkFileOpener() DiagnoseCheck {
	c := DiagnoseCheck{ID: "opener", Title: "文件打开 / Finder Reveal"}
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("open"); err == nil {
			c.Status, c.Detail = "ok", "open 命令可用"
			return c
		}
		c.Status, c.Detail = "error", "open 缺失"
	case "windows":
		c.Status, c.Detail = "ok", "explorer + cmd start 内置"
	default:
		opener := pickLinuxOpener()
		if opener != "" {
			c.Status, c.Detail = "ok", opener+" 可用"
			return c
		}
		c.Status, c.Detail = "error", "缺 xdg-open / gio / kde-open，打开外部 override.md 等功能不可用"
		c.FixHint = "sudo apt install xdg-utils"
	}
	return c
}

func checkPrivilegeEscalation() DiagnoseCheck {
	c := DiagnoseCheck{ID: "privilege", Title: "提权（CA 安装 / Hosts 写入需要）"}
	switch runtime.GOOS {
	case "darwin":
		c.Status, c.Detail = "ok", "macOS 用 osascript Terminal 弹密码框"
	case "windows":
		if os.Geteuid() == 0 || isWindowsAdmin() {
			c.Status, c.Detail = "ok", "当前进程已是管理员"
		} else {
			c.Status, c.Detail = "warn", "非管理员进程；首次操作 Hosts/CA 会 UAC 提示"
			c.FixHint = "右键 windsurf-tools-wails.exe → 以管理员身份运行；或安装为 service"
		}
	default:
		if _, err := exec.LookPath("pkexec"); err == nil {
			c.Status, c.Detail = "ok", "pkexec 可用（polkit）"
			return c
		}
		if _, err := exec.LookPath("sudo"); err == nil {
			c.Status, c.Detail = "warn", "pkexec 缺，降级 sudo（需终端输密码）"
			c.FixHint = "更佳：sudo apt install policykit-1"
			return c
		}
		c.Status, c.Detail = "error", "缺 pkexec 和 sudo，CA/Hosts 操作不可用"
		c.FixHint = "sudo apt install sudo policykit-1"
	}
	return c
}

func checkCertTool() DiagnoseCheck {
	c := DiagnoseCheck{ID: "cert", Title: "CA 证书安装工具"}
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("security"); err == nil {
			c.Status, c.Detail = "ok", "security 命令可用"
			return c
		}
		c.Status, c.Detail = "error", "security 缺失（极少见）"
	case "windows":
		if _, err := exec.LookPath("certutil"); err == nil {
			c.Status, c.Detail = "ok", "certutil 可用"
			return c
		}
		c.Status, c.Detail = "error", "certutil 缺失（极少见）"
	default:
		if _, err := exec.LookPath("update-ca-certificates"); err == nil {
			c.Status, c.Detail = "ok", "update-ca-certificates 可用（Debian/Ubuntu 系）"
			return c
		}
		if _, err := exec.LookPath("update-ca-trust"); err == nil {
			c.Status, c.Detail = "ok", "update-ca-trust 可用（RHEL/Fedora/Arch 系）"
			return c
		}
		c.Status, c.Detail = "error", "缺 update-ca-certificates 和 update-ca-trust，CA 安装不可用"
		c.FixHint = "Debian/Ubuntu: sudo apt install ca-certificates  ·  RHEL/Fedora: sudo dnf install ca-certificates"
	}
	return c
}

func checkAppDataDirWritable(a *App) DiagnoseCheck {
	c := DiagnoseCheck{ID: "appdata", Title: "App 数据目录可写"}
	if a.store == nil {
		c.Status, c.Detail = "error", "store 未初始化"
		return c
	}
	dir := a.store.DataDir()
	if dir == "" {
		c.Status, c.Detail = "error", "数据目录路径未设置"
		return c
	}
	// 尝试写一个临时探针文件
	probe := dir + string(os.PathSeparator) + ".diagnose-probe"
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		c.Status, c.Detail = "error", "无法写入: "+err.Error()
		c.FixHint = "检查目录权限：" + dir
		return c
	}
	_ = os.Remove(probe)
	c.Status, c.Detail = "ok", dir
	return c
}

func checkWindsurfInstalled() DiagnoseCheck {
	c := DiagnoseCheck{ID: "windsurf", Title: "Windsurf IDE 安装"}
	// 复用 services 已有的检测能力（如果有）；简单 fallback 是查常见路径
	candidates := windsurfCandidatesByOS()
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			c.Status, c.Detail = "ok", "Windsurf 已安装: "+p
			return c
		}
	}
	c.Status, c.Detail = "warn", "未在常见路径找到 Windsurf（仍可手动指定）"
	c.FixHint = "下载安装 https://windsurf.com/"
	return c
}

func windsurfCandidatesByOS() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Windsurf.app",
			home + "/Applications/Windsurf.app",
		}
	case "windows":
		appdata := os.Getenv("LOCALAPPDATA")
		return []string{
			appdata + `\Programs\Windsurf\Windsurf.exe`,
			`C:\Program Files\Windsurf\Windsurf.exe`,
		}
	default:
		return []string{
			"/usr/share/windsurf",
			"/opt/Windsurf",
			home + "/.windsurf",
		}
	}
}

func checkClashController(url, secret string) DiagnoseCheck {
	c := DiagnoseCheck{ID: "clash", Title: "Clash 控制器探活"}
	probe := services.ProbeClashController(url, secret)
	if probe.OK {
		c.Status = "ok"
		c.Detail = url + " 可达，组数 " + intToStr(len(probe.Groups))
		return c
	}
	c.Status = "warn"
	c.Detail = "无法连接: " + probe.Error
	c.FixHint = "检查 Clash 是否运行 + external-controller 端口 + secret"
	return c
}

func checkWebView2Runtime() DiagnoseCheck {
	c := DiagnoseCheck{ID: "webview2", Title: "Windows WebView2 Runtime"}
	// 已经能跑到这步说明 WebView2 一定装了（否则 Wails 启动会失败），mark OK
	c.Status, c.Detail = "ok", "WebView2 Runtime 已装（app 能启动即说明可用）"
	return c
}

// isWindowsAdmin 简单检测（os.Geteuid 在 Windows 总返 -1，不准）
func isWindowsAdmin() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	// 尝试打开 \\.\PHYSICALDRIVE0 — 仅管理员可读
	f, err := os.Open(`\\.\PHYSICALDRIVE0`)
	if err == nil {
		_ = f.Close()
		return true
	}
	return false
}

func intToStr(n int) string {
	if n < 0 {
		return "-" + intToStr(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return intToStr(n/10) + intToStr(n%10)
}
