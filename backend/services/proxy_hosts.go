package services

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const hostsMarker = "# windsurf-tools-mitm"

// hostsTargets 需要劫持的所有域名 (与 kaifa 一致)
var hostsTargets = []string{
	"server.self-serve.windsurf.com",
	"server.codeium.com",
}

// GetHostsFilePath returns the system hosts file path.
func GetHostsFilePath() string {
	if runtime.GOOS == "windows" {
		return `C:\Windows\System32\drivers\etc\hosts`
	}
	return "/etc/hosts"
}

func hostsBackupPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, caDirName, caSubDir)
	_ = os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "hosts.bak")
}

// AddHostsEntry adds 127.0.0.1 mapping for domain to the system hosts file.
// When domain is empty or matches TargetDomain, all hostsTargets are added.
func AddHostsEntry(domain string) error {
	path := GetHostsFilePath()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 hosts 文件失败: %w", err)
	}
	content := string(data)

	// 如果已经有标记行，说明已劫持
	if strings.Contains(content, hostsMarker) {
		return nil
	}

	// 备份原始 hosts
	_ = os.WriteFile(hostsBackupPath(), data, 0644)

	// 添加所有目标域名
	var lines []string
	for _, target := range hostsTargets {
		lines = append(lines, fmt.Sprintf("127.0.0.1 %s %s", target, hostsMarker))
	}
	addition := "\n" + strings.Join(lines, "\n") + "\n"
	newContent := content + addition

	if err := writeSystemFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("写入 hosts 文件失败（Linux 会尝试 pkexec/sudo 提权）: %w", err)
	}

	return flushDNS()
}

// RemoveHostsEntry removes our hosts entries.
// When domain is empty or matches TargetDomain, all hostsTargets marker lines are removed.
//
// 恢复策略：**优先基于 marker 逐行清理**，只移除本程序加的行（含 hostsMarker），
// 保留劫持期间用户或其它软件对 /etc/hosts 的合法新增条目。整份 .bak 还原只作为
// 逐行清理结果异常（清理后文件为空/读取失败）时的兜底，避免把用户的修改一并覆盖。
func RemoveHostsEntry(domain string) error {
	path := GetHostsFilePath()
	backup := hostsBackupPath()

	// 优先：按 marker 逐行清理，只删本程序加的行，保留用户的其它修改。
	data, err := os.ReadFile(path)
	if err == nil {
		var newLines []string
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, hostsMarker) {
				continue // skip our lines
			}
			newLines = append(newLines, line)
		}

		newContent := strings.Join(newLines, "\n")
		if !strings.HasSuffix(newContent, "\n") {
			newContent += "\n"
		}

		// 逐行清理结果看起来正常（非空白）时写回并完成。
		if strings.TrimSpace(newContent) != "" {
			if err := writeSystemFile(path, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("写入 hosts 文件失败（Linux 会尝试 pkexec/sudo 提权）: %w", err)
			}
			_ = os.Remove(backup) // 备份文件保留无害，但既已成功清理即可移除
			return flushDNS()
		}
	}

	// 兜底：逐行清理读取失败或结果异常（被清空）时，才用整份 .bak 还原。
	if backupData, e := os.ReadFile(backup); e == nil && len(backupData) > 0 {
		if e := writeSystemFile(path, backupData, 0644); e == nil {
			_ = os.Remove(backup)
			return flushDNS()
		}
	}

	if err != nil {
		return fmt.Errorf("读取 hosts 文件失败: %w", err)
	}
	return flushDNS()
}

// IsHostsMapped checks if the hosts hijacking is active (marker present).
func IsHostsMapped(domain string) bool {
	data, err := os.ReadFile(GetHostsFilePath())
	if err != nil {
		return false
	}
	return strings.Contains(string(data), hostsMarker)
}

// flushDNS clears the system DNS cache so hosts changes take effect immediately.
func flushDNS() error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("ipconfig", "/flushdns")
		hideWindow(cmd)
		return cmd.Run()
	case "darwin":
		_ = exec.Command("dscacheutil", "-flushcache").Run()
		_ = exec.Command("killall", "-HUP", "mDNSResponder").Run()
		return nil
	default:
		_ = exec.Command("systemd-resolve", "--flush-caches").Run()
		return nil
	}
}
