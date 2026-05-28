package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return ts
}

// TestUsageTrackerAppendOnlyPersistsAcrossRestart —— C-1 回归
//
// 旧实现每条 Record 全量重写整个 JSON 文件，崩溃中途文件损坏 → 下次
// load 全部丢失，用户感知为「昨天的数据第二天变少」。新实现 append-only
// + atomic compact，记录跨进程重启必须 100% 保留。
func TestUsageTrackerAppendOnlyPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	tracker := NewUsageTracker(dir)

	for i := 0; i < 50; i++ {
		tracker.Record(UsageRecord{
			ID:           fmt.Sprintf("rec-%d", i),
			At:           "2026-05-17T10:00:00+08:00",
			Model:        "claude-sonnet-4",
			PromptTokens: 100,
			TotalTokens:  100,
			Status:       "ok",
		})
	}

	// 模拟进程重启 → 重新加载
	reloaded := NewUsageTracker(dir)
	if got := reloaded.Count(); got != 50 {
		t.Fatalf("expected 50 records after restart, got %d", got)
	}
	summary := reloaded.GetSummary()
	if summary.TotalRequests != 50 {
		t.Fatalf("expected 50 total requests, got %d", summary.TotalRequests)
	}
	if summary.ByDate["2026-05-17"] != 50 {
		t.Fatalf("expected 50 requests on 2026-05-17, got %d", summary.ByDate["2026-05-17"])
	}
}

// TestUsageTrackerLoadSkipsCorruptedLines —— C-1 回归
//
// 单行损坏不能让整个文件作废；旧实现 json.Unmarshal 整体 fail 后
// records=nil，所有历史数据丢失。新实现逐行解析，损坏行跳过。
func TestUsageTrackerLoadSkipsCorruptedLines(t *testing.T) {
	dir := t.TempDir()

	good1, _ := json.Marshal(UsageRecord{
		ID: "rec-1", At: "2026-05-17T08:00:00+08:00", Model: "m", TotalTokens: 100, Status: "ok",
	})
	good2, _ := json.Marshal(UsageRecord{
		ID: "rec-2", At: "2026-05-17T09:00:00+08:00", Model: "m", TotalTokens: 200, Status: "ok",
	})
	content := strings.Join([]string{
		string(good1),
		`{"id":"rec-broken","at":"...truncated`, // 损坏行
		"",                                       // 空行
		string(good2),
		`not even json`, // 完全垃圾
	}, "\n")

	if err := os.WriteFile(filepath.Join(dir, "relay_usage.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tracker := NewUsageTracker(dir)
	if got := tracker.Count(); got != 2 {
		t.Fatalf("expected 2 valid records (skipping 2 corrupted), got %d", got)
	}
	summary := tracker.GetSummary()
	if summary.TotalTokens != 300 {
		t.Fatalf("expected 300 total tokens, got %d", summary.TotalTokens)
	}
}

// TestUsageTrackerMigratesLegacyJSON —— C-1 回归
//
// 旧版 .json 数据需要在第一次加载时透明迁移到 .jsonl，迁移完成后
// 旧文件应被删除，避免下次启动重复迁移。
func TestUsageTrackerMigratesLegacyJSON(t *testing.T) {
	dir := t.TempDir()

	legacy := []UsageRecord{
		{ID: "old-1", At: "2026-05-15T10:00:00+08:00", Model: "claude-opus-4", TotalTokens: 500, Status: "ok"},
		{ID: "old-2", At: "2026-05-16T10:00:00+08:00", Model: "gpt-4o-mini", TotalTokens: 200, Status: "ok"},
	}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, "relay_usage.json"), data, 0644); err != nil {
		t.Fatalf("WriteFile legacy: %v", err)
	}

	tracker := NewUsageTracker(dir)
	if got := tracker.Count(); got != 2 {
		t.Fatalf("expected 2 records migrated, got %d", got)
	}

	// 旧文件应已被删除
	if _, err := os.Stat(filepath.Join(dir, "relay_usage.json")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy .json to be removed after migration, stat err=%v", err)
	}
	// 新文件应存在
	if _, err := os.Stat(filepath.Join(dir, "relay_usage.jsonl")); err != nil {
		t.Fatalf("expected .jsonl to exist after migration, err=%v", err)
	}

	// 重启后还是 2 条（不会重复读旧文件）
	reloaded := NewUsageTracker(dir)
	if got := reloaded.Count(); got != 2 {
		t.Fatalf("expected 2 records after restart, got %d", got)
	}
}

// TestUsageTrackerSlidingWindowCompactsAtomically —— C-1 回归
//
// 超过 maxStore 触发滑窗时，必须用 atomic rename 重写整个文件，
// 不能让中途的 partial write 留下损坏的 .jsonl。
func TestUsageTrackerSlidingWindowCompactsAtomically(t *testing.T) {
	dir := t.TempDir()
	tracker := NewUsageTracker(dir)
	tracker.maxStore = 10 // 测试用小上限

	// 写入 15 条 → 滑窗保留最新 10 条
	for i := 0; i < 15; i++ {
		tracker.Record(UsageRecord{
			ID:           fmt.Sprintf("rec-%d", i),
			At:           fmt.Sprintf("2026-05-17T10:%02d:00+08:00", i),
			Model:        "m",
			TotalTokens:  100,
			Status:       "ok",
		})
	}

	if got := tracker.Count(); got != 10 {
		t.Fatalf("expected sliding window to keep 10 records, got %d", got)
	}

	// .tmp 不应残留
	if _, err := os.Stat(filepath.Join(dir, "relay_usage.jsonl.tmp")); !os.IsNotExist(err) {
		t.Fatalf("expected .tmp to be cleaned after rename, stat err=%v", err)
	}

	// 重启后仍是 10 条 + 是最新 10 条（rec-5..rec-14）
	reloaded := NewUsageTracker(dir)
	records := reloaded.GetRecords(0)
	if len(records) != 10 {
		t.Fatalf("expected 10 records after restart, got %d", len(records))
	}
	// 最新的（GetRecords 倒序）第一条应该是 rec-14
	if records[0].ID != "rec-14" {
		t.Fatalf("expected first record to be rec-14, got %q", records[0].ID)
	}
	// 最旧的应该是 rec-5（rec-0..rec-4 被滑窗丢弃）
	if records[9].ID != "rec-5" {
		t.Fatalf("expected last record to be rec-5, got %q", records[9].ID)
	}
}

// TestUsageTrackerDeleteBeforeAtomicallyRewrites —— C-1 回归
//
// DeleteBefore 必须 atomic 重写文件，且重启后能正确加载剩余数据。
func TestUsageTrackerDeleteBeforeAtomicallyRewrites(t *testing.T) {
	dir := t.TempDir()
	tracker := NewUsageTracker(dir)

	// 5 条不同日期
	dates := []string{
		"2026-05-10T10:00:00+08:00",
		"2026-05-11T10:00:00+08:00",
		"2026-05-12T10:00:00+08:00",
		"2026-05-13T10:00:00+08:00",
		"2026-05-14T10:00:00+08:00",
	}
	for i, at := range dates {
		tracker.Record(UsageRecord{
			ID:           fmt.Sprintf("rec-%d", i),
			At:           at,
			Model:        "m",
			TotalTokens:  100,
			Status:       "ok",
		})
	}

	// 删除 5/13 之前 → 剩 5/13 + 5/14 两条
	cutoff := mustParseTime(t, "2026-05-13T00:00:00+08:00")
	deleted := tracker.DeleteBefore(cutoff)
	if deleted != 3 {
		t.Fatalf("expected 3 deletions, got %d", deleted)
	}

	// 重启验证
	reloaded := NewUsageTracker(dir)
	if got := reloaded.Count(); got != 2 {
		t.Fatalf("expected 2 records after restart, got %d", got)
	}
}
