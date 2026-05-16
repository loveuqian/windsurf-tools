package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/utils"
)

// prewarmCandidates 并行预热 top N 候选账号：刷新JWT + 实时查额度，将结果写入 store。
func (a *App) prewarmCandidates(candidates []models.Account, maxN int) {
	n := len(candidates)
	if n > maxN {
		n = maxN
	}
	if n == 0 {
		return
	}
	utils.DLog("[切号] 预热 %d 个候选账号...", n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(acc models.Account) {
			defer wg.Done()
			copy := acc
			a.syncAccountCredentials(&copy)
			if a.enrichAccountQuotaOnly(&copy) {
				copy.LastQuotaUpdate = time.Now().Format(time.RFC3339)
			}
			_ = a.store.UpdateAccount(copy)
			if utils.AccountQuotaExhausted(&copy) {
				utils.DLog("[切号] 预热: %s 额度已耗尽 (daily=%s weekly=%s)", copy.Email, copy.DailyRemaining, copy.WeeklyRemaining)
			} else {
				utils.DLog("[切号] 预热: %s 额度OK (daily=%s weekly=%s)", copy.Email, copy.DailyRemaining, copy.WeeklyRemaining)
			}
		}(candidates[i])
	}
	wg.Wait()
}

func hasSwitchCredentials(acc *models.Account) bool {
	if acc == nil {
		return false
	}
	if strings.TrimSpace(acc.Token) != "" {
		return true
	}
	if strings.TrimSpace(acc.WindsurfAPIKey) != "" {
		return true
	}
	if strings.TrimSpace(acc.RefreshToken) != "" {
		return true
	}
	return strings.TrimSpace(acc.Email) != "" && strings.TrimSpace(acc.Password) != ""
}

// accountEligibleForUsage 判定账号是否可参与「自动切号 / 加入号池」的候选。
// bypassQuota=true 时跳过额度耗尽过滤——SmartFriend(F7) 模式下服务端按
// SMART_FRIEND 计费、绕过日/周限额，账号「显示耗尽」实际仍可用，必须放行；
// 否则 MITM 号池会把这些账号剔除，手动切号会因「号池找不到 key」失败。
func accountEligibleForUsage(acc *models.Account, planFilter string, requireAPIKey bool, bypassQuota bool) bool {
	if acc == nil {
		return false
	}
	status := strings.TrimSpace(strings.ToLower(acc.Status))
	if status == "disabled" || status == "expired" {
		return false
	}
	if requireAPIKey && strings.TrimSpace(acc.WindsurfAPIKey) == "" {
		return false
	}
	if !hasSwitchCredentials(acc) {
		return false
	}
	if !utils.PlanFilterMatch(planFilter, acc.PlanName) {
		return false
	}
	if bypassQuota {
		return true
	}
	return !utils.AccountQuotaExhausted(acc)
}

func orderedSwitchCandidates(accounts []models.Account, currentID string, planFilter string, bypassQuota bool) []models.Account {
	var fresh, stale []models.Account
	for _, acc := range accounts {
		if acc.ID == currentID {
			continue
		}
		if accountEligibleForUsage(&acc, planFilter, false, bypassQuota) {
			fresh = append(fresh, acc)
			continue
		}
		// 额度数据过期的账号也纳入候选（额度可能已重置），预热阶段会刷新。
		// SmartFriend bypass 时已通过 fresh 全量纳入，无需再走 stale 分支。
		if !bypassQuota && quotaDataIsStale(&acc) && hasSwitchCredentials(&acc) && utils.PlanFilterMatch(planFilter, acc.PlanName) {
			status := strings.TrimSpace(strings.ToLower(acc.Status))
			if status != "disabled" && status != "expired" {
				stale = append(stale, acc)
			}
		}
	}
	sort.SliceStable(fresh, func(i, j int) bool {
		return switchCredentialPriority(fresh[i]) < switchCredentialPriority(fresh[j])
	})
	sort.SliceStable(stale, func(i, j int) bool {
		return switchCredentialPriority(stale[i]) < switchCredentialPriority(stale[j])
	})
	// 新鲜的优先，过期数据的排后面
	return append(fresh, stale...)
}

// quotaDataIsStale 检查额度数据是否过期（超过重置周期），过期的「已耗尽」账号应参与预热重新检查。
func quotaDataIsStale(acc *models.Account) bool {
	if acc == nil {
		return false
	}
	if !utils.AccountQuotaExhausted(acc) {
		return false // 未耗尽的不算过期
	}
	if utils.QuotaRefreshDueAfterOfficialReset(*acc, time.Now()) {
		return true
	}
	raw := strings.TrimSpace(acc.LastQuotaUpdate)
	if raw == "" {
		return true // 从未同步过
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true
	}
	// 额度数据超过 4 小时视为过期（日额度每天重置，4h 足以覆盖跨日场景）
	return time.Since(t) > 4*time.Hour
}

// intersectByID 保留 source 中 ID 在 idsAllowed 里的项，顺序按 source 原顺序。
// 用于把 candidates 收窄到 RotationPool 成员。
func intersectByID(source []models.Account, idsAllowed []string) []models.Account {
	if len(idsAllowed) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(idsAllowed))
	for _, id := range idsAllowed {
		if id != "" {
			allowed[id] = true
		}
	}
	out := make([]models.Account, 0, len(source))
	for _, a := range source {
		if allowed[a.ID] {
			out = append(out, a)
		}
	}
	return out
}

func orderedMitmCandidates(accounts []models.Account, currentID string, planFilter string, bypassQuota bool) []models.Account {
	var fresh, stale []models.Account
	for _, acc := range accounts {
		if acc.ID == currentID {
			continue
		}
		if accountEligibleForUsage(&acc, planFilter, true, bypassQuota) {
			fresh = append(fresh, acc)
			continue
		}
		// SmartFriend bypass 已经把所有非 disabled/expired 的耗尽账号纳入 fresh，
		// 这里只在非 bypass 时补充「数据过期可能已重置」的候选。
		if !bypassQuota && quotaDataIsStale(&acc) && hasSwitchCredentials(&acc) && utils.PlanFilterMatch(planFilter, acc.PlanName) &&
			strings.TrimSpace(acc.WindsurfAPIKey) != "" {
			status := strings.TrimSpace(strings.ToLower(acc.Status))
			if status != "disabled" && status != "expired" {
				stale = append(stale, acc)
			}
		}
	}
	sort.SliceStable(fresh, func(i, j int) bool {
		return switchCredentialPriority(fresh[i]) < switchCredentialPriority(fresh[j])
	})
	sort.SliceStable(stale, func(i, j int) bool {
		return switchCredentialPriority(stale[i]) < switchCredentialPriority(stale[j])
	})
	return append(fresh, stale...)
}

func switchCredentialPriority(acc models.Account) int {
	switch {
	case strings.TrimSpace(acc.Token) != "":
		return 0
	case strings.TrimSpace(acc.WindsurfAPIKey) != "":
		return 1
	case strings.TrimSpace(acc.RefreshToken) != "":
		return 2
	case strings.TrimSpace(acc.Email) != "" && strings.TrimSpace(acc.Password) != "":
		return 3
	default:
		return 4
	}
}

func pickNextSwitchableAccount(accounts []models.Account, currentID string, planFilter string, bypassQuota bool) (models.Account, error) {
	candidates := orderedSwitchCandidates(accounts, currentID, planFilter, bypassQuota)
	if len(candidates) == 0 {
		return models.Account{}, fmt.Errorf("no switchable account")
	}
	return candidates[0], nil
}

func pickNextMitmSwitchableAccount(accounts []models.Account, currentID string, planFilter string, bypassQuota bool) (models.Account, error) {
	candidates := orderedMitmCandidates(accounts, currentID, planFilter, bypassQuota)
	if len(candidates) == 0 {
		return models.Account{}, fmt.Errorf("no mitm switchable account")
	}
	return candidates[0], nil
}

// F7-REMOVAL: 整函数删除。同步修改调用点 — 全部改为传入字面量 false 或移除 bypassQuota 参数。
// shouldBypassQuotaCheck 当 SmartFriend(F7) 模式开启时，服务端按 SMART_FRIEND
// 计费、绕过日/周限额，所有「额度耗尽」的账号实际仍可用，号池过滤、候选筛选、
// prepareAccount 的额度校验都应放行。
func (a *App) shouldBypassQuotaCheck() bool {
	if a == nil || a.store == nil {
		return false
	}
	return a.store.GetSettings().SmartFriendEnabled
}

// prepareAccountForUsage 自动切号路径使用，SmartFriend 开启时跳过额度耗尽校验。
func (a *App) prepareAccountForUsage(acc models.Account) (models.Account, error) {
	return a.prepareAccount(acc, a.shouldBypassQuotaCheck())
}

// prepareAccountForUsageManual 用户在 UI 上明确选定账号的手动场景使用
// （如「写入本地登录态」「手动切到这个 MITM 账号」按钮）：用户的意图就是
// 「把这个账号准备好」，即使额度已耗尽也允许通过——只校验凭证可用性。
func (a *App) prepareAccountForUsageManual(acc models.Account) (models.Account, error) {
	return a.prepareAccount(acc, true)
}

func (a *App) prepareAccount(acc models.Account, allowExhausted bool) (models.Account, error) {
	utils.DLog("[切号] prepareAccount: %s status=%s hasKey=%v hasToken=%v hasRefresh=%v allowExhausted=%v",
		acc.Email, acc.Status, acc.WindsurfAPIKey != "", acc.Token != "", acc.RefreshToken != "", allowExhausted)
	if !hasSwitchCredentials(&acc) {
		return models.Account{}, fmt.Errorf("该账号没有可用凭证")
	}
	status := strings.TrimSpace(strings.ToLower(acc.Status))
	if status == "disabled" || status == "expired" {
		return models.Account{}, fmt.Errorf("该账号状态为 %s，已跳过", status)
	}

	// ★ 如果刚预热过（30 秒内），跳过重复 API 调用，直接用缓存数据校验
	recentlyWarmed := false
	if t, err := time.Parse(time.RFC3339, acc.LastQuotaUpdate); err == nil && time.Since(t) < 30*time.Second {
		recentlyWarmed = true
	}
	utils.DLog("[切号] prepareAccount: recentlyWarmed=%v", recentlyWarmed)

	before := acc
	if !recentlyWarmed {
		a.syncAccountCredentials(&acc)
		if a.enrichAccountQuotaOnly(&acc) {
			acc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
		}
	}

	if strings.TrimSpace(acc.Token) == "" {
		utils.DLog("[切号] %s Token为空，凭证同步可能失败", acc.Email)
		return models.Account{}, fmt.Errorf("该账号无法准备有效 Token（JWT/登录均失败）")
	}
	if !allowExhausted && utils.AccountQuotaExhausted(&acc) {
		_ = a.store.UpdateAccount(acc)
		a.syncMitmPoolKeys()
		utils.DLog("[切号] %s 实时额度已耗尽 (daily=%s weekly=%s)", acc.Email, acc.DailyRemaining, acc.WeeklyRemaining)
		return models.Account{}, fmt.Errorf("该账号已无可用额度（日=%s 周=%s），已跳过", acc.DailyRemaining, acc.WeeklyRemaining)
	}
	utils.DLog("[切号] prepareAccount OK: %s (daily=%s weekly=%s tokenLen=%d allowExhausted=%v)",
		acc.Email, acc.DailyRemaining, acc.WeeklyRemaining, len(acc.Token), allowExhausted)
	if acc != before {
		_ = a.store.UpdateAccount(acc)
	}
	return acc, nil
}

func (a *App) rotateMitmToNextAvailable(currentID string, planFilter string) (models.Account, error) {
	bypassQuota := a.shouldBypassQuotaCheck()
	candidates := orderedMitmCandidates(a.store.GetAllAccounts(), currentID, planFilter, bypassQuota)
	// ★ Rotation Pool 启用时把候选收窄到池内成员。池外账号完全不参与
	// 自动轮换 —— 这是用户「选这几个号来回切」的明确意图。
	settings := a.store.GetSettings()
	if settings.RotationPoolEnabled && len(settings.RotationPoolAccountIDs) > 0 {
		candidates = intersectByID(candidates, settings.RotationPoolAccountIDs)
		utils.DLog("[切号] rotateMitm: RotationPool 启用，候选收窄到池内 %d 个", len(candidates))
	}
	utils.DLog("[切号] rotateMitm: currentID=%s filter=%s 候选=%d bypassQuota=%v",
		currentID[:min(8, len(currentID))], planFilter, len(candidates), bypassQuota)
	if len(candidates) == 0 {
		return models.Account{}, fmt.Errorf("无可用 MITM 候选账号")
	}

	// ★ 预热 top N 候选：刷新 JWT + 实时查额度，防止切到实际已耗尽的账号
	a.prewarmCandidates(candidates, 2)

	// 预热后重读 store，仅保留仍有额度的（SmartFriend 时全员保留）
	freshCandidates := orderedMitmCandidates(a.store.GetAllAccounts(), currentID, planFilter, bypassQuota)
	if settings.RotationPoolEnabled && len(settings.RotationPoolAccountIDs) > 0 {
		freshCandidates = intersectByID(freshCandidates, settings.RotationPoolAccountIDs)
	}
	utils.DLog("[切号] rotateMitm: 预热后候选=%d", len(freshCandidates))
	if len(freshCandidates) == 0 {
		return models.Account{}, fmt.Errorf("预热后无可用 MITM 候选账号（候选均已耗尽）")
	}

	var lastErr error
	for _, acc := range freshCandidates {
		prepared, err := a.prepareAccountForUsage(acc)
		if err != nil {
			utils.DLog("[切号] rotateMitm 跳过 %s: %v", acc.Email, err)
			lastErr = err
			continue
		}
		apiKey := strings.TrimSpace(prepared.WindsurfAPIKey)
		if apiKey == "" {
			lastErr = fmt.Errorf("该账号没有 API Key，已跳过")
			continue
		}
		utils.DLog("[切号] rotateMitm: 切换到 %s (key=%s...)", prepared.Email, apiKey[:min(12, len(apiKey))])
		if !a.mitmProxy.SwitchToKey(apiKey) {
			lastErr = fmt.Errorf("MITM 代理未找到目标 API Key")
			continue
		}
		utils.DLog("[切号] rotateMitm 成功切换到 %s", prepared.Email)
		return prepared, nil
	}
	if lastErr != nil {
		return models.Account{}, lastErr
	}
	return models.Account{}, fmt.Errorf("无可用 MITM 候选账号")
}
