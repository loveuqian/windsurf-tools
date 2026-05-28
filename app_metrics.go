package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// F2: Dashboard 历史趋势数据
//
// 现有 MitmProxy.recentEvents 是 in-memory 限长 ringbuffer，重启即失。
// 加这层轻量 append-only JSONL 持久化，让用户能看到 24h / 7d 的切号趋势。
//
// 设计原则：
//   - 文件路径：<DataDir>/switch_history.jsonl
//   - 单行 JSON，崩溃半行可被解析器跳过
//   - 不读全量文件，按时间窗口流式扫描尾部
//   - 自动按 30 天 retention，避免无限增长（启动时一次性 GC）
//   - 写入有锁，但用 buffered + lazy fsync（traffic_log 同款思路）

const (
	switchHistoryFileName = "switch_history.jsonl"
	switchHistoryMaxAge   = 30 * 24 * time.Hour
)

// SwitchEvent 一次切号事件
type SwitchEvent struct {
	At       string `json:"at"`        // RFC3339
	Email    string `json:"email"`     // 切到的账号
	KeyShort string `json:"key_short"` // 12 hex
	Reason   string `json:"reason"`    // manual | next | quota_exhausted | rate_limited | startup | unknown
}

// HourBucket 24h 时间桶（前端折线图用）
type HourBucket struct {
	HourStart string `json:"hour_start"` // RFC3339, 整点
	Count     int    `json:"count"`
}

// TopAccount 切号热度排行
type TopAccount struct {
	Email string `json:"email"`
	Count int    `json:"count"`
}

// DayBucket 单日切号统计 — 用于 30 天热力图日历
type DayBucket struct {
	Date  string `json:"date"` // YYYY-MM-DD（本地时区）
	Count int    `json:"count"`
}

// DashboardMetrics 面向 Dashboard 的聚合视图
type DashboardMetrics struct {
	SwitchTotal24h    int            `json:"switch_total_24h"`
	SwitchTotal7d     int            `json:"switch_total_7d"`
	SwitchTotal30d    int            `json:"switch_total_30d"`
	SwitchHourly      []HourBucket   `json:"switch_hourly"`       // 最近 24 个整点桶
	SwitchDaily30d    []DayBucket    `json:"switch_daily_30d"`    // 最近 30 天日桶（前面是过去）
	SwitchTopAccounts []TopAccount   `json:"switch_top_accounts"` // 24h 内最多的 5 个
	ReasonBreakdown   map[string]int `json:"reason_breakdown"`    // 24h 内各 reason 的出现次数
}

// switchHistoryStore append-only JSONL 写入（线程安全）
type switchHistoryStore struct {
	mu      sync.Mutex
	dataDir string
	gcOnce  sync.Once
}

func newSwitchHistoryStore(dataDir string) *switchHistoryStore {
	return &switchHistoryStore{dataDir: dataDir}
}

func (s *switchHistoryStore) filePath() string {
	return filepath.Join(s.dataDir, switchHistoryFileName)
}

// Append 追加一条事件（不阻塞调用方过久；遇错只静默吃掉避免污染切号路径）
func (s *switchHistoryStore) Append(ev SwitchEvent) {
	if s == nil || s.dataDir == "" {
		return
	}
	if ev.At == "" {
		ev.At = time.Now().Format(time.RFC3339)
	}
	if ev.Reason == "" {
		ev.Reason = "unknown"
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.MkdirAll(s.dataDir, 0o755)
	f, err := os.OpenFile(s.filePath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
	// 启动时清理过期数据
	s.gcOnce.Do(func() {
		go s.gc()
	})
}

// gc 整理文件：移除超过 retention 的旧记录（启动时一次）。
//
// 三段式实现，让 scanner / json.Unmarshal 不持 s.mu：
//
//  1. 持锁 → 读快照路径 + 把整文件 read 进 byte slice（快 IO）→ 释锁
//  2. 锁外扫描 + 过滤 + 写 tmp 文件 → 期间 record() 可继续 append 到原文件
//  3. 持锁 → 把 gc 期间新 append 的部分（snapshot 之后的字节）追加到 tmp →
//     rename tmp → path → 释锁
//
// 第 3 步通过 `os.Stat` 在持锁期间检查文件大小，若大于快照大小说明 gc 期间
// 有新 append，把新增部分拼到 tmp 末尾后再 rename，避免丢失 gc 期间产生的事件。
func (s *switchHistoryStore) gc() {
	// 第 ① 段：持锁拿快照
	s.mu.Lock()
	path := s.filePath()
	snapshot, err := os.ReadFile(path)
	if err != nil {
		s.mu.Unlock()
		return
	}
	snapSize := int64(len(snapshot))
	s.mu.Unlock()

	// 第 ② 段：锁外扫描 + 过滤 + 写 tmp（不阻塞 record append）
	cutoff := time.Now().Add(-switchHistoryMaxAge)
	tmp := path + ".gc"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(snapshot))
	scanner.Buffer(make([]byte, 1024), 64*1024)
	kept := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		var ev SwitchEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, ev.At)
		if err != nil || t.Before(cutoff) {
			continue
		}
		_, _ = out.Write(append([]byte(nil), line...))
		_, _ = out.Write([]byte("\n"))
		kept++
	}

	// 第 ③ 段：持锁补 tail + rename
	s.mu.Lock()
	defer s.mu.Unlock()

	// 把 gc 期间新 append 到原文件的部分（snapshot 之后的字节）拼到 tmp 末尾。
	if info, statErr := os.Stat(path); statErr == nil && info.Size() > snapSize {
		if cur, openErr := os.Open(path); openErr == nil {
			if _, seekErr := cur.Seek(snapSize, 0); seekErr == nil {
				if tail, readErr := io.ReadAll(cur); readErr == nil {
					_, _ = out.Write(tail)
				}
			}
			cur.Close()
		}
	}

	_ = out.Sync()
	_ = out.Close()

	if kept == 0 {
		_ = os.Rename(tmp, path)
		return
	}
	_ = os.Rename(tmp, path)
}

// loadEvents 读取所有事件（仅 24h+ 窗口外不直接读全文件，按需返回）
func (s *switchHistoryStore) loadEventsAfter(after time.Time) []SwitchEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.filePath())
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	out := make([]SwitchEvent, 0, 64)
	for scanner.Scan() {
		var ev SwitchEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, ev.At)
		if err != nil {
			continue
		}
		if t.Before(after) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// shortHexKey 把 apiKey 缩成短哈希（取末 12 位 hex 字符）—— 与 PoolKeyInfo.KeyHash 对齐
func shortHexKey(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}
	// 简单版：取字符串后 12 位（避免 sha256 依赖）；对短 key 直接返回
	if len(apiKey) <= 12 {
		return apiKey
	}
	return apiKey[len(apiKey)-12:]
}

// normalizeSwitchReason 把 MitmProxy 的 reason 规整为前端友好的枚举
func normalizeSwitchReason(raw string) string {
	r := strings.ToLower(strings.TrimSpace(raw))
	if r == "" {
		return "unknown"
	}
	switch {
	case strings.Contains(r, "manual"):
		return "manual"
	case strings.Contains(r, "next"):
		return "next"
	case strings.Contains(r, "exhaust") || strings.Contains(r, "quota"):
		return "quota_exhausted"
	case strings.Contains(r, "rate") || strings.Contains(r, "429"):
		return "rate_limited"
	case strings.Contains(r, "startup") || strings.Contains(r, "init"):
		return "startup"
	}
	return r
}

// ── Wails bindings ─────────────────────────────────────────────────────────

// GetDashboardMetrics 返回 Dashboard 历史趋势聚合数据
func (a *App) GetDashboardMetrics() DashboardMetrics {
	now := time.Now()
	if a.switchHistory == nil {
		return DashboardMetrics{
			SwitchHourly:      make([]HourBucket, 0),
			SwitchDaily30d:    make([]DayBucket, 0),
			SwitchTopAccounts: make([]TopAccount, 0),
			ReasonBreakdown:   make(map[string]int),
		}
	}
	// 3.3: 一次读 30 天数据，覆盖 day/week/30d 三个时间窗口
	month := now.Add(-30 * 24 * time.Hour)
	all30 := a.switchHistory.loadEventsAfter(month)

	week := now.Add(-7 * 24 * time.Hour)
	weekEvents := make([]SwitchEvent, 0, len(all30))
	for _, ev := range all30 {
		t, _ := time.Parse(time.RFC3339, ev.At)
		if t.After(week) {
			weekEvents = append(weekEvents, ev)
		}
	}
	all := weekEvents // 兼容下面变量名

	day := now.Add(-24 * time.Hour)
	dayEvents := make([]SwitchEvent, 0, len(all))
	for _, ev := range all {
		t, _ := time.Parse(time.RFC3339, ev.At)
		if t.After(day) {
			dayEvents = append(dayEvents, ev)
		}
	}

	// 24h hourly buckets：从当前整点向前 24 个，时间正序
	buckets := make([]HourBucket, 24)
	hourBase := now.Truncate(time.Hour).Add(-23 * time.Hour)
	for i := 0; i < 24; i++ {
		buckets[i] = HourBucket{
			HourStart: hourBase.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
		}
	}
	for _, ev := range dayEvents {
		t, _ := time.Parse(time.RFC3339, ev.At)
		t = t.Truncate(time.Hour)
		idx := int(t.Sub(hourBase) / time.Hour)
		if idx >= 0 && idx < len(buckets) {
			buckets[idx].Count++
		}
	}

	// Top accounts (24h)
	emailCount := make(map[string]int)
	reasonCount := make(map[string]int)
	for _, ev := range dayEvents {
		key := strings.TrimSpace(ev.Email)
		if key == "" {
			key = ev.KeyShort
		}
		if key != "" {
			emailCount[key]++
		}
		reasonCount[ev.Reason]++
	}
	tops := make([]TopAccount, 0, len(emailCount))
	for k, v := range emailCount {
		tops = append(tops, TopAccount{Email: k, Count: v})
	}
	// 简单排序：按 Count desc，平局按 email asc
	for i := 0; i < len(tops); i++ {
		for j := i + 1; j < len(tops); j++ {
			if tops[j].Count > tops[i].Count ||
				(tops[j].Count == tops[i].Count && tops[j].Email < tops[i].Email) {
				tops[i], tops[j] = tops[j], tops[i]
			}
		}
	}
	if len(tops) > 5 {
		tops = tops[:5]
	}

	// 3.3: 30 天 daily 桶（本地时区 YYYY-MM-DD），从 29 天前到今天，正序
	loc := time.Local
	dayBuckets := make([]DayBucket, 30)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dayBase := today.Add(-29 * 24 * time.Hour)
	dateIdx := make(map[string]int, 30)
	for i := 0; i < 30; i++ {
		d := dayBase.Add(time.Duration(i) * 24 * time.Hour)
		date := d.Format("2006-01-02")
		dayBuckets[i] = DayBucket{Date: date}
		dateIdx[date] = i
	}
	for _, ev := range all30 {
		t, _ := time.Parse(time.RFC3339, ev.At)
		date := t.In(loc).Format("2006-01-02")
		if idx, ok := dateIdx[date]; ok {
			dayBuckets[idx].Count++
		}
	}

	return DashboardMetrics{
		SwitchTotal24h:    len(dayEvents),
		SwitchTotal7d:     len(all),
		SwitchTotal30d:    len(all30),
		SwitchHourly:      buckets,
		SwitchDaily30d:    dayBuckets,
		SwitchTopAccounts: tops,
		ReasonBreakdown:   reasonCount,
	}
}
