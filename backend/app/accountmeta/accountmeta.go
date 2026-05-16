// Package accountmeta ── 账号元信息（套餐、订阅到期、状态、额度百分比）的纯函数集合。
//
// 设计动机：
//
//	这一坨「从 JWT / Profile 推导套餐名 / 订阅到期 / 状态」的逻辑原本散落在
//	app_enrich.go，被 app_accounts / app_quota / app_mitm / app_status_test
//	多处调用。集中到独立子包后：
//	- 单一归属：normalize / parseEnd / choosePreferredExpiry 等只此一份；
//	- 纯函数无副作用：不依赖 App / store / context，便于单测与替身；
//	- 上游 enrich/quota/switch 等模块要读账号语义时统一走这里。
//
// 全部函数都是 package-level 纯函数（无内部状态），调用方既可以在 main 包里
// 用 var alias 兼容旧名（示意：var normalizeAccountPlanAndStatus = accountmeta.NormalizeAccount）。
package accountmeta

import (
	"fmt"
	"strings"
	"time"

	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// AsiaShanghai 用于解析用户在 Remark / Nickname 中手填的日期 hint
// (默认按东八区 23:59:59 当日截止)。
var AsiaShanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

// FormatQuotaPercent 把 0~100 浮点数渲染成 "%.2f%%" 字符串。
func FormatQuotaPercent(value float64) string {
	return fmt.Sprintf("%.2f%%", value)
}

// ParseSubscriptionEnd 解析订阅 / 试用截止时间。优先 RFC3339(Nano)，
// 退化到 time.DateTime；解析失败返回 ok=false。
func ParseSubscriptionEnd(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	s = strings.Trim(s, `"`)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, time.DateTime} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// LooksLikeDatePrefix 快速排除非日期字符串：必须以 4 位数字 + 分隔符开头。
// 提前剪枝可以跳过 99% 的备注/昵称，避免每条都跑一遍 21 种 layout 解析。
func LooksLikeDatePrefix(s string) bool {
	if len(s) < 8 { // "2026/1/2" 最短 8 字符
		return false
	}
	for i := 0; i < 4; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s[4] == '/' || s[4] == '-' || s[4] == '.'
}

// ParseManualExpiryHint 把用户手填的 "2026/3/26" / "2026-3-26 12:34" 等
// 多种格式日期解析为 Asia/Shanghai 时区的 time。仅日期（无时间）的会
// 落到当日 23:59:59，与「截止当天仍可用」的语义对齐。
func ParseManualExpiryHint(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(strings.Trim(raw, `"`))
	if raw == "" || !LooksLikeDatePrefix(raw) {
		return time.Time{}, false
	}

	layouts := []struct {
		layout   string
		endOfDay bool
	}{
		{layout: "2006/1/2", endOfDay: true},
		{layout: "2006-1-2", endOfDay: true},
		{layout: "2006.1.2", endOfDay: true},
		{layout: "2006/01/02", endOfDay: true},
		{layout: "2006-01-02", endOfDay: true},
		{layout: "2006.01.02", endOfDay: true},
		{layout: "2006/1/2 15:04"},
		{layout: "2006-1-2 15:04"},
		{layout: "2006.1.2 15:04"},
		{layout: "2006/01/02 15:04"},
		{layout: "2006-01-02 15:04"},
		{layout: "2006.01.02 15:04"},
		{layout: "2006/1/2 15:04:05"},
		{layout: "2006-1-2 15:04:05"},
		{layout: "2006.1.2 15:04:05"},
		{layout: "2006/01/02 15:04:05"},
		{layout: "2006-01-02 15:04:05"},
		{layout: "2006.01.02 15:04:05"},
		{layout: "2006-1-2T15:04"},
		{layout: "2006-01-02T15:04"},
		{layout: "2006-1-2T15:04:05"},
		{layout: "2006-01-02T15:04:05"},
	}
	for _, item := range layouts {
		t, err := time.ParseInLocation(item.layout, raw, AsiaShanghai)
		if err != nil {
			continue
		}
		if item.endOfDay {
			t = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, AsiaShanghai)
		}
		return t, true
	}
	return time.Time{}, false
}

// ManualExpiryHint 从账号 Remark / Nickname 中尝试提取手填的截止日期，
// 返回 RFC3339 字符串（已转成 UTC）。任何字段都没填时返回空串。
func ManualExpiryHint(acc *models.Account) string {
	if acc == nil {
		return ""
	}
	for _, raw := range []string{acc.Remark, acc.Nickname} {
		if ts, ok := ParseManualExpiryHint(raw); ok {
			return ts.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

// SubscriptionEndBeforeAccountCreated 判定服务端返回的 end 是否早于账号创建
// 时间。GetPlanStatus.planEnd 偶尔返回的是「周期开始」而非「订阅结束」，
// 这种情况下 end 会落在 createdAt 之前，调用方据此忽略它。
func SubscriptionEndBeforeAccountCreated(acc *models.Account, value string) bool {
	if acc == nil {
		return false
	}
	tEnd, ok := ParseSubscriptionEnd(value)
	if !ok {
		return false
	}
	tCreated, ok := ParseSubscriptionEnd(strings.TrimSpace(acc.CreatedAt))
	if !ok {
		return false
	}
	return tEnd.Before(tCreated)
}

// SubscriptionEndLooksLikeStalePlanStart：同步到的「到期」早于账号写入本工具
// 的时间，且日/周额度显示仍有剩余时，多为 GetPlanStatus.planEnd 表示周期
// 开始而非订阅结束，调用方应忽略。
func SubscriptionEndLooksLikeStalePlanStart(acc *models.Account, profileEnd string) bool {
	if acc == nil {
		return false
	}
	if !SubscriptionEndBeforeAccountCreated(acc, profileEnd) {
		return false
	}
	d, dOk := utils.ParseQuotaPercentString(acc.DailyRemaining)
	w, wOk := utils.ParseQuotaPercentString(acc.WeeklyRemaining)
	hasQuota := (dOk && d > 0.0001) || (wOk && w > 0.0001)
	return hasQuota
}

// ChoosePreferredExpiry 在「服务端候选 / 现存值 / 用户手填 hint」三者中
// 挑出最合理的订阅到期时间字符串：
//   - 服务端 candidate 不空且不早于账号创建 → 用 candidate
//   - 否则若现存值合法（不早于创建）→ 沿用
//   - 否则若用户手填了 hint → 用 hint
//   - 都没有 → 返回空串
func ChoosePreferredExpiry(acc *models.Account, candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if acc == nil {
		return candidate
	}

	current := strings.TrimSpace(acc.SubscriptionExpiresAt)
	hint := ManualExpiryHint(acc)

	if candidate != "" && !SubscriptionEndBeforeAccountCreated(acc, candidate) {
		return candidate
	}
	if current != "" && !SubscriptionEndBeforeAccountCreated(acc, current) {
		return current
	}
	if hint != "" {
		return hint
	}
	return ""
}

// NormalizeAccount 同步 PlanName/Status 与订阅到期时间：
//   - 强制把 SubscriptionExpiresAt 走一遍 ChoosePreferredExpiry
//   - 状态 disabled 保持不变
//   - 到期已过：状态 → expired；非 free 套餐降级为 Free
//   - 否则状态 → active
func NormalizeAccount(acc *models.Account) {
	if acc == nil {
		return
	}
	acc.SubscriptionExpiresAt = ChoosePreferredExpiry(acc, "")
	status := strings.TrimSpace(strings.ToLower(acc.Status))
	if status == "" {
		status = "active"
	}
	if status == "disabled" {
		acc.Status = "disabled"
		return
	}
	if acc.SubscriptionExpiresAt == "" {
		acc.Status = status
		return
	}
	t, ok := ParseSubscriptionEnd(acc.SubscriptionExpiresAt)
	if !ok {
		acc.Status = status
		return
	}
	if !t.After(time.Now()) {
		acc.Status = "expired"
		if utils.PlanTone(acc.PlanName) != "free" {
			acc.PlanName = "Free"
		}
		return
	}
	acc.Status = "active"
}

// ApplyJWTClaims 把 JWT 解出来的 Email/Name/PlanName/TrialEnd 落到 acc 上，
// 末尾会调用 NormalizeAccount 同步状态。
func ApplyJWTClaims(acc *models.Account, claims *services.JWTClaims) {
	if claims == nil {
		return
	}
	if claims.Email != "" {
		acc.Email = claims.Email
	}
	if acc.Nickname == "" && claims.Name != "" {
		acc.Nickname = claims.Name
	}
	// 每次根据 JWT + 本地记录的到期时间重算套餐；到期后不再沿用缓存的 Pro/Trial（后续 GetPlanStatus 可覆盖）
	if plan := DerivePlanNameFromClaims(claims, ChoosePreferredExpiry(acc, "")); plan != "" {
		acc.PlanName = plan
	}
	if claims.TrialEnd != "" {
		acc.SubscriptionExpiresAt = ChoosePreferredExpiry(acc, claims.TrialEnd)
	}
	NormalizeAccount(acc)
}

// ApplyProfile 把 services.AccountProfile 的字段平移到 acc，并 Normalize 一次。
func ApplyProfile(acc *models.Account, profile *services.AccountProfile) {
	if profile == nil {
		return
	}
	if profile.Email != "" {
		acc.Email = profile.Email
	}
	if profile.Name != "" && (acc.Nickname == "" || acc.Nickname == strings.Split(acc.Email, "@")[0]) {
		acc.Nickname = profile.Name
	}
	if profile.PlanName != "" {
		acc.PlanName = profile.PlanName
	}
	// 额度字段完全以本次官方响应为准，避免沿用旧快照。
	acc.TotalQuota = profile.TotalCredits
	acc.UsedQuota = profile.UsedCredits
	if profile.DailyQuotaRemaining != nil {
		acc.DailyRemaining = FormatQuotaPercent(*profile.DailyQuotaRemaining)
	} else {
		acc.DailyRemaining = ""
	}
	if profile.WeeklyQuotaRemaining != nil {
		acc.WeeklyRemaining = FormatQuotaPercent(*profile.WeeklyQuotaRemaining)
	} else {
		acc.WeeklyRemaining = ""
	}
	// 优先使用官方接口返回的 resetAt；缺失时保持为空，不再伪造周额度/周重置时间。
	acc.DailyResetAt = strings.TrimSpace(profile.DailyResetAt)
	acc.WeeklyResetAt = strings.TrimSpace(profile.WeeklyResetAt)
	if preferred := ChoosePreferredExpiry(acc, profile.SubscriptionExpiresAt); preferred != "" {
		acc.SubscriptionExpiresAt = preferred
	} else {
		acc.SubscriptionExpiresAt = ""
	}
	NormalizeAccount(acc)
}

// ApplyAccessErrorStatus 把 grpc 报错文本翻译成账号 Status：
//   - "user is disabled in windsurf team" → disabled
//   - "subscription is not active" → expired (+ Free)
//   - "permission denied" / `"code":"permission_denied"` → disabled
//
// 其它错误不动 acc.Status。
func ApplyAccessErrorStatus(acc *models.Account, err error) {
	if acc == nil || err == nil {
		return
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(lower, "user is disabled in windsurf team"):
		acc.Status = "disabled"
	case strings.Contains(lower, "subscription is not active"):
		acc.Status = "expired"
		if utils.PlanTone(acc.PlanName) != "free" {
			acc.PlanName = "Free"
		}
	case strings.Contains(lower, `"code":"permission_denied"`), strings.Contains(lower, "permission denied"):
		acc.Status = "disabled"
	}
}

// DerivePlanNameFromClaims 从 JWT claims 推导套餐：
//
//	storedSubEnd 为 accounts.json 里已记的 SubscriptionExpiresAt
//	(JWT 无 trial_end 时参与判断是否已到期)。
//	已到期 → 直接返回 "Free"；
//	否则按 Pro / TeamsTier / TrialEnd 顺序推断；
//	无法判定时返回 ""。
func DerivePlanNameFromClaims(claims *services.JWTClaims, storedSubEnd string) string {
	if claims == nil {
		return ""
	}
	end := strings.TrimSpace(claims.TrialEnd)
	if end == "" {
		end = strings.TrimSpace(storedSubEnd)
	}
	if end != "" {
		if t, ok := ParseSubscriptionEnd(end); ok && !t.After(time.Now()) {
			return "Free"
		}
	}
	if claims.Pro {
		return "Pro"
	}
	teamsTier := strings.ToUpper(claims.TeamsTier)
	switch teamsTier {
	case "TEAMS_TIER_PRO":
		return "Pro"
	case "TEAMS_TIER_MAX", "TEAMS_TIER_PRO_MAX", "TEAMS_TIER_ULTIMATE":
		return "Max"
	case "TEAMS_TIER_ENTERPRISE":
		return "Enterprise"
	case "TEAMS_TIER_TEAMS":
		return "Teams"
	case "TEAMS_TIER_TRIAL":
		return "Trial"
	case "TEAMS_TIER_FREE":
		return "Free"
	}
	if strings.Contains(teamsTier, "TRIAL") {
		return "Trial"
	}
	if strings.Contains(teamsTier, "MAX") || strings.Contains(teamsTier, "ULTIMATE") {
		return "Max"
	}
	if strings.Contains(teamsTier, "ENTERPRISE") {
		return "Enterprise"
	}
	if teamsTier == "TEAMS_TIER_TEAMS" || (strings.Contains(teamsTier, "TEAMS") && !strings.Contains(teamsTier, "TIER_FREE") && !strings.Contains(teamsTier, "TIER_PRO") && !strings.Contains(teamsTier, "TIER_TRIAL")) {
		return "Teams"
	}
	if strings.Contains(teamsTier, "PRO") {
		return "Pro"
	}
	if claims.TrialEnd != "" {
		if t, ok := ParseSubscriptionEnd(claims.TrialEnd); ok && t.After(time.Now()) {
			return "Trial"
		}
	}
	return ""
}
