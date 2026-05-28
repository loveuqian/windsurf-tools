package main

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/utils"
)

// F3: 切号调度策略
//
// 原 orderedSwitchCandidates / orderedMitmCandidates 仅按凭证优先级排序，
// 这里在不破坏向后兼容的前提下叠加：
//
//   1. **strategy** 模式
//      - "fcfs"（默认）  完全保留旧行为
//      - "priority"      先消耗 trial（用完即弃），再用 pro/max；按 plan 类型优先级排
//      - "balanced"      按额度剩余 % 倒序：剩余多的先用，让所有号月底分布更均匀
//
//   2. **cooldown 惩罚**
//      - 因 quota_exhausted 被踢的号 → cooldownBaseSec 秒内不再被选回
//      - 因 rate_limited / access_denied 被踢的号 → 同上，但同一号连续命中倍增
//      - 重启即失（in-memory map）
//
// 不影响：
//   - 用户手动 SwitchToAccount（明确意图，不被冷却阻挡）
//   - Pin / RotationPool 的成员准入（仍按现有 settings）

// ── plan tone 优先级（priority 模式默认顺序） ─────────────────────────────
// 思路：trial 短期内必失效，先用；pro/max 是付费长期资源，留到 trial 用完
var defaultPlanPriority = []string{
	"trial",
	"pro",
	"max",
	"team",
	"enterprise",
	"free",
	"unknown",
}

// ── cooldown 状态机 ────────────────────────────────────────────────────────

const (
	defaultCooldownBaseSec = 300
	maxCooldownBaseSec     = 3600
	minCooldownBaseSec     = 30
	maxCooldownStreak      = 4 // 连续命中翻倍上限：base*2^4 = base*16
	// maxCooldownDurationSec 对最终冷却时长(base * 2^streak)的硬上限。
	// 没有它时 base=3600 + streak=4 → 16h,账号被长时间排除出自动选号。
	// 钳到 1h,与 maxCooldownBaseSec 同量级,作为兜底防御。
	maxCooldownDurationSec = 3600
)

type cooldownEntry struct {
	until    time.Time
	streak   int
	lastKind string // quota | ratelimit | access_denied
}

// switchCooldownState 进程级冷却记录（无持久化）
type switchCooldownState struct {
	mu      sync.RWMutex
	entries map[string]cooldownEntry // accountID → 状态
}

var switchCooldown = &switchCooldownState{entries: make(map[string]cooldownEntry)}

// applyCooldown 给 accountID 标记冷却。kind 影响连续 streak 翻倍：同 kind 连发翻倍，
// 切换 kind 重置 streak。
func (s *switchCooldownState) apply(accountID, kind string, baseSec int) {
	if accountID == "" {
		return
	}
	if baseSec <= 0 {
		baseSec = defaultCooldownBaseSec
	}
	if baseSec > maxCooldownBaseSec {
		baseSec = maxCooldownBaseSec
	}
	if baseSec < minCooldownBaseSec {
		baseSec = minCooldownBaseSec
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.entries[accountID]
	streak := 0
	if prev.lastKind == kind && time.Now().Before(prev.until.Add(time.Duration(baseSec)*time.Second)) {
		streak = prev.streak + 1
		if streak > maxCooldownStreak {
			streak = maxCooldownStreak
		}
	}
	mult := 1 << streak // 2^streak
	dur := time.Duration(baseSec) * time.Duration(mult) * time.Second
	// 最终时长硬上限,防止 base*2^streak 累计出 16h 这种离谱冷却。
	if dur > maxCooldownDurationSec*time.Second {
		dur = maxCooldownDurationSec * time.Second
	}
	s.entries[accountID] = cooldownEntry{
		until:    time.Now().Add(dur),
		streak:   streak,
		lastKind: kind,
	}
	utils.DLog("[调度] 冷却 %s → %v（kind=%s streak=%d）", accountID[:minInt(8, len(accountID))], dur, kind, streak)
}

// clear 显式清掉某账号的冷却（用户手动切到该账号时调用）
func (s *switchCooldownState) clear(accountID string) {
	if accountID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, accountID)
}

// isInCooldown 当前是否仍在冷却中
func (s *switchCooldownState) isInCooldown(accountID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[accountID]
	if !ok {
		return false
	}
	return time.Now().Before(e.until)
}

// gc 删除已过期的 entry（小集合，O(N) 一次走全表）
func (s *switchCooldownState) gc() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, e := range s.entries {
		if now.After(e.until.Add(time.Hour)) { // 过期 1h 后也删，避免无限增长
			delete(s.entries, k)
		}
	}
}

// ── strategy 排序辅助 ──────────────────────────────────────────────────────

// applySchedStrategy 按 settings.SwitchStrategy 重排候选；fcfs 模式直接 return。
// 调用前 candidates 应已经按凭证优先级排好。
func applySchedStrategy(candidates []models.Account, strategy string) []models.Account {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	switch strategy {
	case "priority":
		return sortByPlanPriority(candidates)
	case "balanced":
		return sortByQuotaRemaining(candidates)
	}
	return candidates
}

func sortByPlanPriority(in []models.Account) []models.Account {
	out := append([]models.Account(nil), in...)
	rank := make(map[string]int, len(defaultPlanPriority))
	for i, p := range defaultPlanPriority {
		rank[p] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri := planRank(out[i].PlanName, rank)
		rj := planRank(out[j].PlanName, rank)
		if ri != rj {
			return ri < rj
		}
		// 平局：保留原先（凭证优先级）顺序
		return false
	})
	return out
}

func planRank(planName string, rank map[string]int) int {
	tone := plantTone(planName)
	if r, ok := rank[tone]; ok {
		return r
	}
	return len(rank) // 未识别排最后
}

// plantTone 与 frontend account.ts getPlanTone 行为对齐
func plantTone(plan string) string {
	p := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(plan), "_", " "))
	if p == "" || p == "unknown" {
		return "unknown"
	}
	switch {
	case strings.Contains(p, "trial"):
		return "trial"
	case strings.Contains(p, "max") || strings.Contains(p, "ultimate"):
		return "max"
	case strings.Contains(p, "enterprise"):
		return "enterprise"
	case strings.Contains(p, "team"):
		return "team"
	case strings.Contains(p, "pro"):
		return "pro"
	case strings.Contains(p, "free") || strings.Contains(p, "basic"):
		return "free"
	}
	return "unknown"
}

// sortByQuotaRemaining 按账号「最低维度剩余 %」desc 排（剩余多的优先用）
// 数据缺失的账号视为剩余 100%（默认会被先尝试，预热阶段实测）
func sortByQuotaRemaining(in []models.Account) []models.Account {
	out := append([]models.Account(nil), in...)
	score := make(map[string]float64, len(out))
	for _, a := range out {
		score[a.ID] = quotaRemainingPct(a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return score[out[i].ID] > score[out[j].ID]
	})
	return out
}

func quotaRemainingPct(a models.Account) float64 {
	d := parsePctSafe(a.DailyRemaining)
	w := parsePctSafe(a.WeeklyRemaining)
	// monthly：用 total/used 推
	monthly := -1.0
	if a.TotalQuota > 0 {
		used := a.UsedQuota
		if used < 0 {
			used = 0
		}
		remain := a.TotalQuota - used
		if remain < 0 {
			remain = 0
		}
		monthly = float64(remain) / float64(a.TotalQuota) * 100
	}
	// 取最低维度（即「最先见底的瓶颈」）
	candidates := []float64{}
	if d >= 0 {
		candidates = append(candidates, d)
	}
	if w >= 0 {
		candidates = append(candidates, w)
	}
	if monthly >= 0 {
		candidates = append(candidates, monthly)
	}
	if len(candidates) == 0 {
		return 100.0
	}
	min := candidates[0]
	for _, v := range candidates[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func parsePctSafe(s string) float64 {
	v := strings.TrimSpace(s)
	if v == "" {
		return -1
	}
	v = strings.TrimSuffix(v, "%")
	v = strings.TrimSpace(v)
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return -1
	}
	return f
}
