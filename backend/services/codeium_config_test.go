package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInjectCodeiumConfigWithHomeDirWritesBothKeyVariants(t *testing.T) {
	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".codeium", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"theme":"dark"}`), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := injectCodeiumConfigWithHomeDir(homeDir, "sk-ws-test"); err != nil {
		t.Fatalf("injectCodeiumConfigWithHomeDir() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got["api_key"] != "sk-ws-test" {
		t.Fatalf("api_key = %#v, want sk-ws-test", got["api_key"])
	}
	if got["apiKey"] != "sk-ws-test" {
		t.Fatalf("apiKey = %#v, want sk-ws-test", got["apiKey"])
	}
	if got["theme"] != "dark" {
		t.Fatalf("theme = %#v, want dark", got["theme"])
	}
}

// TestInjectCodeiumConfigPreservesOriginalBackupAcrossInjects 验证：
// 多次注入（模拟切号）后，backup 仍指向用户启动前的真实原始配置，
// RestoreCodeiumConfig 能把状态恢复到「未注入任何 key」的原状。
//
// 历史 bug：旧版每次 inject 都覆盖 backup → 第二次 inject 时 configPath
// 已是「带 key-A 的 config」，被当成「原始」存进 backup → Restore 把用户
// 还原成「带 key-A 的状态」而不是真正未注入的初始状态。
func TestInjectCodeiumConfigPreservesOriginalBackupAcrossInjects(t *testing.T) {
	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".codeium", "config.json")
	backupPath := filepath.Join(homeDir, ".codeium", "config.json.bak")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := []byte(`{"theme":"dark","editor":"vim"}`)
	if err := os.WriteFile(configPath, original, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// 连续 3 次注入不同 key，模拟切号反复 inject。
	for _, key := range []string{"sk-ws-A", "sk-ws-B", "sk-ws-C"} {
		if err := injectCodeiumConfigWithHomeDir(homeDir, key); err != nil {
			t.Fatalf("injectCodeiumConfigWithHomeDir(%s) error = %v", key, err)
		}
	}

	gotBackup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile(backupPath) error = %v", err)
	}
	if string(gotBackup) != string(original) {
		t.Fatalf("backup contents = %q, want unchanged original %q\n→ backup 被多次 inject 覆盖了，Restore 后用户拿到的不是真正原始状态。",
			string(gotBackup), string(original))
	}

	// 真实落盘配置应是最后一次 inject 的 key
	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(configPath) error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(cfgData, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got["api_key"] != "sk-ws-C" {
		t.Fatalf("api_key = %#v, want sk-ws-C", got["api_key"])
	}

	// Restore 后应回到完全没注入的原始状态
	t.Setenv("HOME", homeDir)
	if err := RestoreCodeiumConfig(); err != nil {
		t.Fatalf("RestoreCodeiumConfig() error = %v", err)
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(restored) error = %v", err)
	}
	if string(restored) != string(original) {
		t.Fatalf("restored config = %q, want %q", string(restored), string(original))
	}
}
