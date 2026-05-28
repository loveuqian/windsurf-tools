package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestSwitchHistoryGCClearsAllExpiredRecords(t *testing.T) {
	dir := t.TempDir()
	store := newSwitchHistoryStore(dir)
	old := SwitchEvent{
		At:     time.Now().Add(-switchHistoryMaxAge - time.Hour).Format(time.RFC3339),
		Email:  "old@example.com",
		Reason: "quota_exhausted",
	}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(store.filePath(), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store.gc()

	got, err := os.ReadFile(store.filePath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("gc file length = %d, want 0; content=%q", len(got), string(got))
	}
}
