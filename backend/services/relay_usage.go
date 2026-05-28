package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// 用量追踪 — 记录每次 relay 请求的模型、token 消耗、时间等
// 存储为本地 JSONL 文件，支持查询与删除
// ═══════════════════════════════════════════════════════════════

// UsageRecord 单次请求的用量记录
type UsageRecord struct {
	ID               string `json:"id"`
	At               string `json:"at"` // RFC3339
	Model            string `json:"model"`
	RequestModel     string `json:"request_model"` // 用户请求的原始模型名
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	DurationMs       int64  `json:"duration_ms"`
	APIKeyShort      string `json:"api_key_short"`
	Status           string `json:"status"` // "ok" / "error"
	ErrorDetail      string `json:"error_detail,omitempty"`
	Format           string `json:"format"` // "openai" / "anthropic"
}

// UsageSummary 用量汇总（按天/按模型）
type UsageSummary struct {
	TotalRequests    int            `json:"total_requests"`
	TotalPrompt      int            `json:"total_prompt_tokens"`
	TotalCompletion  int            `json:"total_completion_tokens"`
	TotalTokens      int            `json:"total_tokens"`
	ByModel          map[string]int `json:"by_model"`        // model → request count
	ByModelTokens    map[string]int `json:"by_model_tokens"` // model → total tokens
	ByDate           map[string]int `json:"by_date"`         // YYYY-MM-DD → request count
	ByDateTokens     map[string]int `json:"by_date_tokens"`  // YYYY-MM-DD → total tokens
	ErrorCount       int            `json:"error_count"`
	EstimatedCostUSD float64        `json:"estimated_cost_usd"`
}

// UsageTracker 管理用量追踪
type UsageTracker struct {
	mu       sync.Mutex
	dataDir  string
	records  []UsageRecord
	loaded   bool
	maxStore int // 最大存储条数（0=无限制）
	summary  UsageSummary
	dirty    bool
}

// NewUsageTracker 创建用量追踪器
//
// maxStore=50000 — 旧版 10000 在重度使用下一天就能打爆（每条 chat 一条
// record），导致昨天的数据被滑动窗口挤出，前端「by_date」分组里看到的
// 当天总数比当时小很多。50000 配合 60 天保留窗口能覆盖绝大多数用户。
func NewUsageTracker(dataDir string) *UsageTracker {
	return &UsageTracker{
		dataDir:  dataDir,
		maxStore: 50000,
		dirty:    true,
	}
}

// filePath 是新版 JSONL append-only 持久化文件路径。
func (t *UsageTracker) filePath() string {
	return filepath.Join(t.dataDir, "relay_usage.jsonl")
}

// legacyFilePath 是旧版 JSON array 文件路径，启动时一次性迁移。
func (t *UsageTracker) legacyFilePath() string {
	return filepath.Join(t.dataDir, "relay_usage.json")
}

// Record 记录一次用量
//
// 持久化策略：
//   - 正常路径：单行 JSON append 到 .jsonl（O(1) 写一行，不再全量重写）。
//   - 滑窗触发：当 maxStore 超过时滑窗 + 一次性 compact 重写整个文件
//     （atomic rename）。
//
// 旧实现每条 Record 都 marshal + WriteFile 整个 records，并发 + 高频聊天
// 场景下 IO 压力很大，且非原子写崩溃中途易损坏文件 → 下次 load 全部丢失，
// 用户感知为「昨天的数据第二天变少」。
func (t *UsageTracker) Record(rec UsageRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.loaded {
		t.loadLocked()
	}
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("u-%d", time.Now().UnixNano())
	}
	if rec.At == "" {
		rec.At = time.Now().Format(time.RFC3339)
	}
	t.records = append(t.records, rec)
	t.markSummaryDirtyLocked()

	// 超过上限：滑窗 + 全量 compact（atomic rename，不会丢数据）
	if t.maxStore > 0 && len(t.records) > t.maxStore {
		t.records = t.records[len(t.records)-t.maxStore:]
		t.compactLocked()
		return
	}
	// 正常路径：append 一行到 JSONL
	t.appendLocked(rec)
}

// GetRecords 返回所有记录（最近的在前）
func (t *UsageTracker) GetRecords(limit int) []UsageRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	n := len(t.records)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]UsageRecord, limit)
	for i := 0; i < limit; i++ {
		out[i] = t.records[n-1-i] // 最近的在前
	}
	return out
}

// GetSummary 返回用量汇总
func (t *UsageTracker) GetSummary() UsageSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	if t.dirty {
		t.summary = t.computeSummaryLocked()
		t.dirty = false
	}
	return cloneUsageSummary(t.summary)
}

func (t *UsageTracker) computeSummaryLocked() UsageSummary {
	s := UsageSummary{
		ByModel:       make(map[string]int),
		ByModelTokens: make(map[string]int),
		ByDate:        make(map[string]int),
		ByDateTokens:  make(map[string]int),
	}
	for _, r := range t.records {
		s.TotalRequests++
		s.TotalPrompt += r.PromptTokens
		s.TotalCompletion += r.CompletionTokens
		s.TotalTokens += r.TotalTokens
		model := r.Model
		if model == "" {
			model = r.RequestModel
		}
		s.ByModel[model]++
		s.ByModelTokens[model] += r.TotalTokens
		// 按日期分组
		ts, err := time.Parse(time.RFC3339, r.At)
		if err == nil {
			date := ts.Format("2006-01-02")
			s.ByDate[date]++
			s.ByDateTokens[date] += r.TotalTokens
		}
		if r.Status == "error" {
			s.ErrorCount++
		}

		// 精准计算定价 (根据模型)
		var pPrice, cPrice float64
		mL := strings.ToLower(model)
		if strings.Contains(mL, "opus") {
			pPrice, cPrice = 5.00, 25.00 // Opus 4.6 / Opus API
		} else if strings.Contains(mL, "sonnet") {
			pPrice, cPrice = 3.00, 15.00 // Sonnet 4.6 / 3.5
		} else if strings.Contains(mL, "haiku") {
			pPrice, cPrice = 1.00, 5.00
		} else if strings.Contains(mL, "o1") {
			pPrice, cPrice = 15.00, 60.00
		} else if strings.Contains(mL, "gpt-4o-mini") || strings.Contains(mL, "gpt-3.5") {
			pPrice, cPrice = 0.15, 0.60
		} else if strings.Contains(mL, "gpt-4") {
			pPrice, cPrice = 2.50, 10.00
		} else {
			// 默认按 Opus 定价估算（用户常用）
			pPrice, cPrice = 5.00, 25.00
		}

		cost := (float64(r.PromptTokens) / 1000000.0 * pPrice) + (float64(r.CompletionTokens) / 1000000.0 * cPrice)
		s.EstimatedCostUSD += cost
	}
	return s
}

// DeleteAll 清空所有记录
func (t *UsageTracker) DeleteAll() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	n := len(t.records)
	t.records = nil
	t.markSummaryDirtyLocked()
	// 全清空：直接删文件（DeleteAll 后再启动应该是干净状态）+ 兜底删旧 .json
	_ = os.Remove(t.filePath())
	_ = os.Remove(t.legacyFilePath())
	return n
}

// DeleteBefore 删除指定日期之前的记录
func (t *UsageTracker) DeleteBefore(before time.Time) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	var kept []UsageRecord
	deleted := 0
	for _, r := range t.records {
		ts, err := time.Parse(time.RFC3339, r.At)
		if err != nil || ts.Before(before) {
			deleted++
			continue
		}
		kept = append(kept, r)
	}
	t.records = kept
	if deleted > 0 {
		t.markSummaryDirtyLocked()
		_ = t.compactLocked()
	}
	return deleted
}

// Count 返回记录总数
func (t *UsageTracker) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	return len(t.records)
}

// ── 持久化 ──

// loadLocked 加载持久化数据。
//
// 优先级：
//  1. 优先读取 .jsonl（新格式，append-only）；
//  2. .jsonl 不存在但有 .json（旧格式 JSON array）→ 一次性迁移；
//  3. 都没有 → records 置空。
//
// 容错：单行解析失败时跳过该行而非整体 fail，避免「某行损坏导致历史
// 数据全丢」（旧实现 json.Unmarshal 整体 fail 就清空）。
func (t *UsageTracker) loadLocked() {
	t.loaded = true
	t.records = nil

	// 1) 新格式 JSONL
	if records, ok := t.readJSONLLocked(); ok {
		sort.Slice(records, func(i, j int) bool {
			return records[i].At < records[j].At
		})
		t.records = records
		t.markSummaryDirtyLocked()
		return
	}

	// 2) 旧格式 JSON array → 迁移
	if records, ok := t.readLegacyJSONLocked(); ok {
		sort.Slice(records, func(i, j int) bool {
			return records[i].At < records[j].At
		})
		t.records = records
		// 写入新格式（atomic），成功后删除旧文件
		if err := t.compactLocked(); err == nil {
			_ = os.Remove(t.legacyFilePath())
		}
		t.markSummaryDirtyLocked()
		return
	}

	t.markSummaryDirtyLocked()
}

// readJSONLLocked 读取新格式文件；返回 ok=false 表示文件不存在或不是 JSONL。
func (t *UsageTracker) readJSONLLocked() ([]UsageRecord, bool) {
	f, err := os.Open(t.filePath())
	if err != nil {
		return nil, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// 单条记录可能 > 64KB（极端 prompt），把上限拉到 1MB。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	records := make([]UsageRecord, 0, 128)
	corrupted := 0
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec UsageRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			corrupted++
			continue // 容错：跳过单行损坏
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		// 文件中途出错也不丢已读出的部分
		_ = err
	}
	if corrupted > 0 {
		// 启动一次性提示；不打到 trafficLog 避免循环依赖
		fmt.Fprintf(os.Stderr, "[usage] 跳过 %d 行损坏数据（保留 %d 条）\n", corrupted, len(records))
	}
	return records, true
}

// readLegacyJSONLocked 读取旧格式 JSON array；返回 ok=false 表示文件不存在或解析失败。
func (t *UsageTracker) readLegacyJSONLocked() ([]UsageRecord, bool) {
	data, err := os.ReadFile(t.legacyFilePath())
	if err != nil {
		return nil, false
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, true // 空文件视为已迁移
	}
	var records []UsageRecord
	if err := json.Unmarshal(data, &records); err != nil {
		// 旧文件损坏 — 备份再清空，避免下次启动反复尝试解析
		_ = os.Rename(t.legacyFilePath(), t.legacyFilePath()+".corrupted")
		return nil, false
	}
	return records, true
}

// appendLocked 把单条记录 O(1) 写入 .jsonl 末尾。
//
// 不做 fsync —— 操作系统 page cache 通常 30s 内 flush，正常关机下不会
// 丢；崩溃极端场景下最多丢最近几秒的记录，不会影响历史数据。
// 比起旧实现每条全量重写整个文件，append-only 既快又不会损坏历史。
func (t *UsageTracker) appendLocked(rec UsageRecord) {
	if err := os.MkdirAll(t.dataDir, 0755); err != nil {
		return
	}
	f, err := os.OpenFile(t.filePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = f.Write(data)
}

// compactLocked 全量重写整个 .jsonl 文件（atomic rename）。
//
// 用于 DeleteAll / DeleteBefore / 滑窗触发等需要丢弃部分记录的场景，
// 以及旧格式迁移。先写入 .tmp 再 Rename — 崩溃中途也不会损坏目标文件。
func (t *UsageTracker) compactLocked() error {
	if err := os.MkdirAll(t.dataDir, 0755); err != nil {
		return err
	}
	tmp := t.filePath() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, rec := range t.records {
		data, err := json.Marshal(rec)
		if err != nil {
			continue // 单条 marshal 失败不影响整体
		}
		_, _ = w.Write(data)
		_ = w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, t.filePath())
}

// estimateTokens 粗略估算 token 数
// 按字符类型分桶：ASCII ~4 字符/token, CJK ~1.5 字符/token, 空白 ~6 字符/token
func estimateTokens(text string) int {
	var acc tokenAccumulator
	acc.Add(text)
	return acc.Total()
}

// tokenAccumulator 增量 token 计数器（P7 优化）。
//
// 旧实现 ConnectStreamWatcher 用 strings.Builder 累积所有 chunk 文本，
// finalize 一次性 estimateTokens。长对话累 1-5MB string 占内存且 finalize
// 还要再 range 一次 — 无谓 2× CPU + 内存峰值。
//
// 新实现：每帧 Add(text) 直接 range rune 增量累计 ascii/cjk/space 三类，
// Total() 一次除法收尾。完全省掉 string 累积，CPU 一遍扫完。
type tokenAccumulator struct {
	ascii int
	cjk   int
	space int
}

// Add 把 text 的 rune 分类统计到累加器（不保留 text 引用）。
func (a *tokenAccumulator) Add(text string) {
	for _, r := range text {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			a.space++
		case r < 0x80:
			a.ascii++
		default:
			a.cjk++
		}
	}
}

// Total 返回当前累计的近似 token 数（与 estimateTokens 等价公式）。
func (a *tokenAccumulator) Total() int {
	if a.ascii+a.cjk+a.space == 0 {
		return 0
	}
	// tokens ≈ ascii/4 + cjk/1.5 + space/6
	// 公分母 12: (ascii*3 + cjk*8 + space*2) / 12，+11 做向上取整
	n := (a.ascii*3 + a.cjk*8 + a.space*2 + 11) / 12
	if n == 0 {
		n = 1
	}
	return n
}

func (t *UsageTracker) markSummaryDirtyLocked() {
	t.dirty = true
	t.summary = UsageSummary{}
}

func cloneUsageSummary(in UsageSummary) UsageSummary {
	out := in
	out.ByModel = cloneStringIntMap(in.ByModel)
	out.ByModelTokens = cloneStringIntMap(in.ByModelTokens)
	out.ByDate = cloneStringIntMap(in.ByDate)
	out.ByDateTokens = cloneStringIntMap(in.ByDateTokens)
	return out
}

func cloneStringIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return map[string]int{}
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
