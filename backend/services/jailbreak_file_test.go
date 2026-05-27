package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveJailbreakOverrideFilePath_EmptyUsesDefault(t *testing.T) {
	got := ResolveJailbreakOverrideFilePath("")
	if got == "" {
		t.Fatal("expected default path, got empty")
	}
	// 跨平台 separator：Windows 是 \，*nix 是 /，所以用 filepath.Join 拼期望后缀
	want := filepath.Join(".claude", "override.md")
	if !strings.HasSuffix(got, want) {
		t.Errorf("default path should end with %q, got %q", want, got)
	}
}

func TestResolveJailbreakOverrideFilePath_TildeExpansion(t *testing.T) {
	got := ResolveJailbreakOverrideFilePath("~/my-override.md")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "my-override.md")
	if got != want {
		t.Errorf("tilde expansion: got %q, want %q", got, want)
	}
}

func TestResolveJailbreakOverrideFilePath_AbsolutePathPassThrough(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "override.md")
	got := ResolveJailbreakOverrideFilePath(abs)
	if got != abs {
		t.Errorf("absolute path mangled: got %q, want %q", got, abs)
	}
}

func TestSaveAndLoadJailbreakOverrideFile_Roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "override.md")
	text := "  OVERRIDE TEXT for test  \n"

	resolved, err := SaveJailbreakOverrideFile(path, text)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if resolved != path {
		t.Errorf("resolved path mismatch: got %q, want %q", resolved, path)
	}

	// SaveJailbreakOverrideFile 应该创建父目录
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("parent dir not created: %v", err)
	}

	loaded, loadedPath, err := LoadJailbreakOverrideFile(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loadedPath != path {
		t.Errorf("loaded path mismatch: got %q, want %q", loadedPath, path)
	}
	// LoadJailbreakOverrideFile 会 TrimSpace
	if loaded != strings.TrimSpace(text) {
		t.Errorf("loaded content mismatch: got %q, want %q", loaded, strings.TrimSpace(text))
	}
}

func TestLoadJailbreakOverrideFile_NonexistentReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.md")
	_, _, err := LoadJailbreakOverrideFile(path)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected IsNotExist error, got %v", err)
	}
}

func TestLoadJailbreakOverrideFile_DirReturnsError(t *testing.T) {
	dir := t.TempDir() // 是个目录而不是文件
	_, _, err := LoadJailbreakOverrideFile(dir)
	if err == nil {
		t.Error("expected error when loading from a directory")
	}
}

func TestLoadJailbreakOverrideFile_TruncatesOversized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.md")
	big := strings.Repeat("A", jailbreakFileMaxBytes+1024)
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := LoadJailbreakOverrideFile(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded) > jailbreakFileMaxBytes {
		t.Errorf("oversized file should be truncated to %d bytes, got %d",
			jailbreakFileMaxBytes, len(loaded))
	}
}

func TestJailbreakOverrideFileExists(t *testing.T) {
	dir := t.TempDir()
	exists := filepath.Join(dir, "exists.md")
	_ = os.WriteFile(exists, []byte("x"), 0o644)
	missing := filepath.Join(dir, "missing.md")

	if !JailbreakOverrideFileExists(exists) {
		t.Error("existing file should return true")
	}
	if JailbreakOverrideFileExists(missing) {
		t.Error("missing file should return false")
	}
	if JailbreakOverrideFileExists(dir) {
		t.Error("directory should return false")
	}
}

func TestInspectJailbreakOverrideFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "override.md")
	_ = os.WriteFile(path, []byte("hello override"), 0o644)

	st := InspectJailbreakOverrideFile(path)
	if !st.Exists {
		t.Errorf("Exists should be true, status=%+v", st)
	}
	if st.Charset != "utf-8" {
		t.Errorf("charset should be utf-8, got %q", st.Charset)
	}
	if !strings.Contains(st.Excerpt, "hello override") {
		t.Errorf("excerpt should contain content, got %q", st.Excerpt)
	}

	// 缺失文件
	st2 := InspectJailbreakOverrideFile(filepath.Join(dir, "missing.md"))
	if st2.Exists {
		t.Error("missing file Exists should be false")
	}
	if st2.Error == "" {
		t.Error("missing file should have Error set")
	}
}

func TestInspectJailbreakOverrideFile_BinaryDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	// 90% 二进制字节
	binData := make([]byte, 100)
	for i := range binData {
		if i%10 == 0 {
			binData[i] = 'A'
		} else {
			binData[i] = 0x01
		}
	}
	_ = os.WriteFile(path, binData, 0o644)

	st := InspectJailbreakOverrideFile(path)
	if st.Charset != "binary" {
		t.Errorf("binary content should be detected, got charset=%q", st.Charset)
	}
}

func TestIsMostlyText_PureASCII(t *testing.T) {
	if !isMostlyText([]byte("Hello, World!\nThis is plain text.")) {
		t.Error("ASCII should be detected as text")
	}
}

func TestIsMostlyText_UTF8Chinese(t *testing.T) {
	if !isMostlyText([]byte("你好世界，这是中文测试")) {
		t.Error("UTF-8 Chinese should be detected as text")
	}
}

func TestIsMostlyText_Empty(t *testing.T) {
	if !isMostlyText(nil) {
		t.Error("empty should be considered text (vacuously)")
	}
}
