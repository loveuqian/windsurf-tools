package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// InjectCodeiumConfig 写入 ~/.codeium/config.json 注入 API Key。
// 兼容不同 Windsurf/Codeium 版本，同时写入 snake_case 与 camelCase。
func InjectCodeiumConfig(apiKey string) error {
	if apiKey == "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return injectCodeiumConfigWithHomeDir(home, apiKey)
}

func InjectCodeiumConfigAtHome(homeDir, apiKey string) error {
	if apiKey == "" {
		return nil
	}
	return injectCodeiumConfigWithHomeDir(homeDir, apiKey)
}

func injectCodeiumConfigWithHomeDir(homeDir, apiKey string) error {
	dir, err := codeiumConfigDirFromHome(homeDir)
	if err != nil {
		return err
	}
	configPath := filepath.Join(dir, "config.json")
	backupPath := filepath.Join(dir, "config.json.bak")

	// 备份原始文件 —— 仅当 backup 不存在时才备份。
	// 重要：本工具会随切号反复 inject，第二次 inject 时 configPath 已经
	// 是「带上次注入的 key」的状态，**不能**用它覆盖 backup。这样
	// RestoreCodeiumConfig 才能把用户的真实原始配置还原回来。
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		if data, err := os.ReadFile(configPath); err == nil {
			_ = os.WriteFile(backupPath, data, 0644)
		}
	}

	// 读取或创建新的配置
	config := make(map[string]interface{})
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &config)
	}

	config["api_key"] = apiKey
	config["apiKey"] = apiKey

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 codeium config: %w", err)
	}
	return robustWriteFile(configPath, data)
}

// robustWriteFile 兼容管理员 Windsurf 锁定文件：直写 → 临时文件+rename → PowerShell。
func robustWriteFile(filePath string, data []byte) error {
	if err := os.WriteFile(filePath, data, 0644); err == nil {
		return nil
	} else {
		log.Printf("[写入] 直写 %s 失败(%v)，尝试备选方案", filepath.Base(filePath), err)
	}
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err == nil {
		if err := os.Rename(tmpPath, filePath); err == nil {
			return nil
		}
		_ = os.Remove(tmpPath)
	}
	if runtime.GOOS == "windows" {
		return writeFileViaPowerShell(filePath, data)
	}
	return fmt.Errorf("写入 %s 失败（所有方式均失败）", filepath.Base(filePath))
}

// HasCodeiumConfigBackup 检查 ~/.codeium/config.json.bak 是否存在。
// shouldCleanupMitmEnvironment 用它判断是否需要在退出时跑 RestoreCodeiumConfig：
// 即使 hosts/CA 都被外部清理了，只要 backup 还在就说明 ~/.codeium/config.json
// 仍是被注入过的状态，必须恢复。
func HasCodeiumConfigBackup() bool {
	dir, err := codeiumConfigDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, "config.json.bak"))
	return err == nil
}

// RestoreCodeiumConfig 恢复 ~/.codeium/config.json
func RestoreCodeiumConfig() error {
	dir, err := codeiumConfigDir()
	if err != nil {
		return nil
	}
	configPath := filepath.Join(dir, "config.json")
	backupPath := filepath.Join(dir, "config.json.bak")

	if backupData, err := os.ReadFile(backupPath); err == nil {
		// 用 robustWriteFile 写回（直写→临时→PowerShell 三级降级），兼容
		// Windsurf 以管理员锁定 config.json 的情况。
		if err := robustWriteFile(configPath, backupData); err != nil {
			// 写入失败：**不要**删除备份，否则原始 config 会永久丢失。返回错误，
			// 让上层知道恢复未完成、备份仍在，可重试。
			return fmt.Errorf("恢复 codeium config 失败（备份已保留 %s）: %w", backupPath, err)
		}
		// 仅在写入成功后才删除备份。
		_ = os.Remove(backupPath)
		return nil
	}

	// 无备份时清除注入过的 key 字段
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	config := make(map[string]interface{})
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	delete(config, "api_key")
	delete(config, "apiKey")
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 codeium config: %w", err)
	}
	if err := robustWriteFile(configPath, newData); err != nil {
		return fmt.Errorf("清除 codeium config 注入字段失败: %w", err)
	}
	return nil
}

func codeiumConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录: %w", err)
	}
	return codeiumConfigDirFromHome(home)
}

func codeiumConfigDirFromHome(home string) (string, error) {
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("获取用户目录: 为空")
	}
	dir := filepath.Join(home, ".codeium")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建 .codeium 目录: %w", err)
	}
	return dir, nil
}
