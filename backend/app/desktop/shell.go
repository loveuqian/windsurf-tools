// Package desktop ── 跨平台调起系统默认应用打开文件 / 在 Finder/Explorer
// 中显示文件位置。给 OpenJailbreakOverrideFile / RevealJailbreakOverrideFolder
// 等 App 方法使用，并被 RunDiagnostics 检测 Linux 文件打开器是否可用。
package desktop

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenPath 用系统默认应用打开文件。
//   - macOS: `open <path>` → 用 Finder 注册的默认 app（.md 一般是 TextEdit）
//   - Windows: `cmd /c start "" <path>`
//   - Linux: 优先 xdg-open → gio open → kde-open5 → exo-open（每个发行版默认不同）
//
// 不阻塞等待编辑器退出（用 Start 而非 Run），即返回。
func OpenPath(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		opener := PickLinuxOpener()
		if opener == "" {
			return fmt.Errorf("Linux 没有可用的文件打开器（缺 xdg-open / gio / kde-open5 / exo-open）。装一个：sudo apt install xdg-utils")
		}
		cmd = exec.Command(opener, path)
		// gio 调用方式是 `gio open <path>`
		if opener == "gio" {
			cmd = exec.Command(opener, "open", path)
		}
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open failed: %w", err)
	}
	return nil
}

// RevealPath 在文件管理器中选中并显示文件位置。
//   - macOS: `open -R <path>` → Finder 高亮该文件
//   - Windows: `explorer /select,<path>`
//   - Linux: 没有标准 reveal 命令，统一退化为打开父目录
func RevealPath(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	case "windows":
		cmd = exec.Command("explorer", "/select,"+path)
	default:
		dir := path
		if i := lastSepIndex(path); i > 0 {
			dir = path[:i]
		}
		opener := PickLinuxOpener()
		if opener == "" {
			return fmt.Errorf("Linux 没有可用的文件管理器命令")
		}
		cmd = exec.Command(opener, dir)
		if opener == "gio" {
			cmd = exec.Command(opener, "open", dir)
		}
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("reveal failed: %w", err)
	}
	return nil
}

// PickLinuxOpener 按 Linux 发行版常见优先级挑一个可用的文件打开器。
// xdg-utils 在大多数桌面发行版默认装；服务器/极简发行版 / WSL 可能没装。
// 返回空字符串表示一个都没找到，调用方应给用户友好提示。
func PickLinuxOpener() string {
	for _, c := range []string{"xdg-open", "gio", "kde-open5", "kde-open", "exo-open", "gnome-open"} {
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	return ""
}

func lastSepIndex(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' || s[i] == '\\' {
			return i
		}
	}
	return -1
}
