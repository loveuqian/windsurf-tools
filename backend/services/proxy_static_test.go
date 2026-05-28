package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadStaticFileCacheKeyIncludesDirectory(t *testing.T) {
	staticCache.mu.Lock()
	staticCache.entries = make(map[string]*staticEntry)
	staticCache.mu.Unlock()

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	name := "GetUserStatus.bin"
	path1 := filepath.Join(dir1, name)
	path2 := filepath.Join(dir2, name)
	modTime := time.Unix(1700000000, 0)

	if err := os.WriteFile(path1, []byte("from-dir-1"), 0o644); err != nil {
		t.Fatalf("WriteFile(path1) error = %v", err)
	}
	if err := os.WriteFile(path2, []byte("from-dir-2"), 0o644); err != nil {
		t.Fatalf("WriteFile(path2) error = %v", err)
	}
	if err := os.Chtimes(path1, modTime, modTime); err != nil {
		t.Fatalf("Chtimes(path1) error = %v", err)
	}
	if err := os.Chtimes(path2, modTime, modTime); err != nil {
		t.Fatalf("Chtimes(path2) error = %v", err)
	}

	got1, ok := loadStaticFile(dir1, name)
	if !ok || string(got1) != "from-dir-1" {
		t.Fatalf("loadStaticFile(dir1) = %q, %v", string(got1), ok)
	}
	got2, ok := loadStaticFile(dir2, name)
	if !ok || string(got2) != "from-dir-2" {
		t.Fatalf("loadStaticFile(dir2) = %q, %v", string(got2), ok)
	}
}
