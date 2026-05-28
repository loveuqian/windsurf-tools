package main

// app_rotation_pool.go ── 轮换池 (Rotation Pool)
//
// 设计动机：
//   用户从号池里选 N 个账号进「轮换池」。轮换池启用后：
//     1. 定时切换 (默认 5 分钟一次) — 让多个账号错开负载，单号被服务端
//        画像 / 限速概率降低
//     2. 额度耗尽 / 限速 时只在池内挑下一个 — 池外账号不参与自动轮换
//     3. 池内账号每 1 分钟刷一次额度 — 让 UI 实时显示池内成员状态
//
// 与 ManualPin 的关系：
//   Pin 优先级最高 — pin 时连轮换池的定时切换也跳过，保持纯手动状态。
//   Pin 解除后定时切换会在下一个 tick 重新激活（不需要重启 manager）。
//
// 跨平台 thread-safe：mu + context.cancel + ticker drain。

import (
	"context"
	"fmt"
	"sync"
	"time"

	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/utils"
)

// RotationPoolStatus 给前端状态面板用。
type RotationPoolStatus struct {
	Enabled             bool   `json:"enabled"`
	MemberCount         int    `json:"member_count"`
	IntervalMin         int    `json:"interval_min"`
	QuotaRefreshMin     int    `json:"quota_refresh_min"`
	NextSwitchAt        string `json:"next_switch_at,omitempty"`   // RFC3339；未启用为空
	LastSwitchedTo      string `json:"last_switched_to,omitempty"` // 账号 email/key 简介
	LastSwitchedAt      string `json:"last_switched_at,omitempty"`
	LastQuotaRefreshAt  string `json:"last_quota_refresh_at,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	TotalSwitches       int    `json:"total_switches"`
	TotalQuotaRefreshes int    `json:"total_quota_refreshes"`
	// PausedByPin 表示当前因 ManualPin 暂停定时切，但 manager 仍在运行
	PausedByPin bool `json:"paused_by_pin"`
}

// rotationPoolState 内部统计 + ticker 引用，受 mu 保护。
type rotationPoolState struct {
	mu                  sync.RWMutex
	enabled             bool
	memberIDs           []string
	intervalMin         int
	quotaRefreshMin     int
	switchTicker        *time.Ticker
	quotaTicker         *time.Ticker
	cancel              context.CancelFunc
	lastSwitchedTo      string
	lastSwitchedAt      time.Time
	lastQuotaRefreshAt  time.Time
	lastError           string
	totalSwitches       int
	totalQuotaRefreshes int
	nextSwitchAt        time.Time
}

// applyRotationPoolSettings 根据 settings 启停 RotationPool。在 initBackend /
// UpdateSettings 中调用，应通过 a.mu 串行化（同 applyClashRotatorSettings）。
func (a *App) applyRotationPoolSettings() {
	if a.store == nil {
		return
	}
	if a.rotationPool == nil {
		a.rotationPool = &rotationPoolState{}
	}
	s := a.store.GetSettings()
	rp := a.rotationPool

	rp.mu.Lock()
	defer rp.mu.Unlock()

	// 钳制间隔到合法范围
	interval := s.RotationPoolIntervalMin
	if interval < 1 {
		interval = 5
	} else if interval > 60 {
		interval = 60
	}
	quotaRefresh := s.RotationPoolQuotaRefreshMin
	if quotaRefresh < 1 {
		quotaRefresh = 1
	} else if quotaRefresh > 10 {
		quotaRefresh = 10
	}

	// 标准化成员 ID 列表（去空 + 去重）
	members := dedupNonEmpty(s.RotationPoolAccountIDs)

	configChanged := rp.enabled != s.RotationPoolEnabled ||
		rp.intervalMin != interval ||
		rp.quotaRefreshMin != quotaRefresh ||
		!stringSliceEqual(rp.memberIDs, members)

	// 关闭：从 enabled → !enabled，或参数变了
	if !configChanged && rp.enabled == s.RotationPoolEnabled {
		return
	}

	// 停旧 ticker（如果有）
	if rp.cancel != nil {
		rp.cancel()
		rp.cancel = nil
	}
	if rp.switchTicker != nil {
		rp.switchTicker.Stop()
		rp.switchTicker = nil
	}
	if rp.quotaTicker != nil {
		rp.quotaTicker.Stop()
		rp.quotaTicker = nil
	}

	rp.enabled = s.RotationPoolEnabled
	rp.memberIDs = members
	rp.intervalMin = interval
	rp.quotaRefreshMin = quotaRefresh

	if !rp.enabled || len(members) == 0 {
		rp.nextSwitchAt = time.Time{}
		utils.DLog("[RotationPool] 已停止 (enabled=%v members=%d)", rp.enabled, len(members))
		return
	}

	ctx, cancel := context.WithCancel(a.ctx)
	rp.cancel = cancel
	rp.switchTicker = time.NewTicker(time.Duration(interval) * time.Minute)
	rp.quotaTicker = time.NewTicker(time.Duration(quotaRefresh) * time.Minute)
	rp.nextSwitchAt = time.Now().Add(time.Duration(interval) * time.Minute)

	utils.DLog("[RotationPool] 已启动: members=%d interval=%dmin quotaRefresh=%dmin",
		len(members), interval, quotaRefresh)

	switchTicker := rp.switchTicker
	quotaTicker := rp.quotaTicker

	// 定时切号 goroutine
	go a.rotationPoolSwitchLoop(ctx, switchTicker)
	// 额度刷新 goroutine
	go a.rotationPoolQuotaLoop(ctx, quotaTicker)
}

// stopRotationPool 在 app 退出时调用清理。
func (a *App) stopRotationPool() {
	if a.rotationPool == nil {
		return
	}
	a.rotationPool.mu.Lock()
	defer a.rotationPool.mu.Unlock()
	if a.rotationPool.cancel != nil {
		a.rotationPool.cancel()
		a.rotationPool.cancel = nil
	}
	if a.rotationPool.switchTicker != nil {
		a.rotationPool.switchTicker.Stop()
		a.rotationPool.switchTicker = nil
	}
	if a.rotationPool.quotaTicker != nil {
		a.rotationPool.quotaTicker.Stop()
		a.rotationPool.quotaTicker = nil
	}
	a.rotationPool.enabled = false
}

func (a *App) rotationPoolSwitchLoop(ctx context.Context, ticker *time.Ticker) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.rotationPoolSwitchOnce("定时")
		}
	}
}

func (a *App) rotationPoolQuotaLoop(ctx context.Context, ticker *time.Ticker) {
	// 启动后立即刷一次，让用户进入设置页时已经有数据
	go a.rotationPoolRefreshQuotas()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.rotationPoolRefreshQuotas()
		}
	}
}

// rotationPoolSwitchOnce 切到池内下一个账号。reason 用于日志区分定时切 / 立即切。
// Pin 时跳过，但更新 nextSwitchAt 让 UI 不至于卡在过去时间。
func (a *App) rotationPoolSwitchOnce(reason string) (string, error) {
	if a.rotationPool == nil || a.store == nil {
		return "", fmt.Errorf("rotation pool not initialized")
	}
	// 与其它自动切号入口共用 switchMu 串行化:定时 tick 与用户「立即切换」
	// 撞在同一秒时不会并发选号/并发 SwitchToKey 争抢 currentIdx。
	a.switchMu.Lock()
	defer a.switchMu.Unlock()
	s := a.store.GetSettings()
	if !s.RotationPoolEnabled {
		return "", fmt.Errorf("rotation pool disabled")
	}
	if s.ManualPinEnabled {
		utils.DLog("[RotationPool] %s切换跳过: ManualPin 生效", reason)
		a.rotationPool.mu.Lock()
		a.rotationPool.nextSwitchAt = time.Now().Add(time.Duration(a.rotationPool.intervalMin) * time.Minute)
		a.rotationPool.mu.Unlock()
		return "", nil
	}
	members := dedupNonEmpty(s.RotationPoolAccountIDs)
	if len(members) < 2 {
		return "", fmt.Errorf("rotation pool 至少需要 2 个成员")
	}

	currentID := findAccountIDForMITMAPIKey(a.store.GetAllAccounts(), a.mitmProxy.CurrentAPIKey())
	target := pickNextRotationPoolMember(a.store.GetAllAccounts(), members, currentID, a.shouldBypassQuotaCheck())
	if target == nil {
		// 候选全部按缓存额度判为耗尽。可能是跨日/跨周后官方额度已重置但本地缓存
		// 未刷新 → 主动拉一次池内额度再重试一次,避免把可恢复的号永久判死。
		utils.DLog("[RotationPool] %s切换: 候选按缓存全耗尽,主动刷新额度后重试", reason)
		a.rotationPoolRefreshQuotas()
		target = pickNextRotationPoolMember(a.store.GetAllAccounts(), members, currentID, a.shouldBypassQuotaCheck())
	}
	if target == nil {
		// ★ 即便失败也推进 nextSwitchAt,否则 UI「下次切换时间」永远停在过去,
		//   且 ticker 下个周期还会再触发(届时额度可能已恢复)。
		a.rotationPool.mu.Lock()
		a.rotationPool.nextSwitchAt = time.Now().Add(time.Duration(a.rotationPool.intervalMin) * time.Minute)
		a.rotationPool.mu.Unlock()
		err := fmt.Errorf("池内没有可切的可用账号 (当前=%s 成员=%d)", currentID[:min(8, len(currentID))], len(members))
		a.recordRotationPoolError(err.Error())
		return "", err
	}

	res, err := a.switchMitmAccountAndSyncLocalSession(*target)
	if err == nil {
		a.lastAutoSwitchAt = time.Now() // 与其它入口共用去重时间戳(已持 switchMu)
	}
	a.rotationPool.mu.Lock()
	a.rotationPool.nextSwitchAt = time.Now().Add(time.Duration(a.rotationPool.intervalMin) * time.Minute)
	if err == nil {
		a.rotationPool.lastSwitchedTo = target.Email
		a.rotationPool.lastSwitchedAt = time.Now()
		a.rotationPool.totalSwitches++
		a.rotationPool.lastError = ""
		utils.DLog("[RotationPool] %s切到 %s (total=%d)", reason, target.Email, a.rotationPool.totalSwitches)
	} else {
		a.rotationPool.lastError = err.Error()
		utils.DLog("[RotationPool] %s切到 %s 失败: %v", reason, target.Email, err)
	}
	a.rotationPool.mu.Unlock()

	// ★ 切号是「显式自动行为」不应触发 ManualPin（不同于用户手动 SwitchMitmToAccount）
	// 由于 switchMitmAccountAndSyncLocalSession 不会自动 pin（pin 是
	// SwitchMitmToAccount 包装层做的），这里无需额外解除 pin。
	return res, err
}

// rotationPoolRefreshQuotas 拉池内每个账号的额度（并发，限制并发数避免一次冲过头）。
func (a *App) rotationPoolRefreshQuotas() {
	if a.store == nil || a.rotationPool == nil {
		return
	}
	s := a.store.GetSettings()
	if !s.RotationPoolEnabled {
		return
	}
	members := dedupNonEmpty(s.RotationPoolAccountIDs)
	if len(members) == 0 {
		return
	}
	utils.DLog("[RotationPool] 刷新池内 %d 个账号额度", len(members))

	// 并发限制 4 个 goroutine，对小池子（10 个以内）足够快又不会瞬间冲爆 API
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, id := range members {
		wg.Add(1)
		go func(accID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := a.RefreshAccountQuota(accID); err != nil {
				utils.DLog("[RotationPool] 刷新 %s 额度失败: %v", accID[:min(8, len(accID))], err)
			}
		}(id)
	}
	wg.Wait()

	a.rotationPool.mu.Lock()
	a.rotationPool.lastQuotaRefreshAt = time.Now()
	a.rotationPool.totalQuotaRefreshes++
	a.rotationPool.mu.Unlock()
}

func (a *App) recordRotationPoolError(msg string) {
	if a.rotationPool == nil {
		return
	}
	a.rotationPool.mu.Lock()
	a.rotationPool.lastError = msg
	a.rotationPool.mu.Unlock()
}

// pickNextRotationPoolMember 从 memberIDs 里挑一个能用的、不是 currentID 的账号。
// 按 ID 列表顺序找 currentID 的下一个；找不到就取第一个可用的；都不可用返回 nil。
// 可用 = 有 凭证 + 未 disabled/expired + (非 SmartFriend 时) 未额度耗尽。
// bypassQuota=true 时跳过额度过滤——SmartFriend(F7) 模式下「显示耗尽」实际仍可用。
func pickNextRotationPoolMember(all []models.Account, memberIDs []string, currentID string, bypassQuota bool) *models.Account {
	byID := make(map[string]models.Account, len(all))
	for _, a := range all {
		byID[a.ID] = a
	}
	curIdx := -1
	for i, id := range memberIDs {
		if id == currentID {
			curIdx = i
			break
		}
	}
	tryIdx := func(idx int) *models.Account {
		acc, ok := byID[memberIDs[idx]]
		if !ok {
			return nil
		}
		if !rotationPoolMemberUsable(&acc, bypassQuota) {
			return nil
		}
		return &acc
	}
	// 从 currentID 的下一个开始扫一圈
	for offset := 1; offset <= len(memberIDs); offset++ {
		idx := (curIdx + offset) % len(memberIDs)
		if acc := tryIdx(idx); acc != nil && acc.ID != currentID {
			return acc
		}
	}
	return nil
}

// rotationPoolMemberUsable 判断轮换池成员是否可参与下一次定时切号。
// bypassQuota=true 时跳过额度耗尽过滤——SmartFriend(F7) 模式下服务端按
// SMART_FRIEND 计费、绕过日/周限额，「显示耗尽」实际仍可用。
func rotationPoolMemberUsable(acc *models.Account, bypassQuota bool) bool {
	if acc == nil {
		return false
	}
	if !hasSwitchCredentials(acc) {
		return false
	}
	status := acc.Status
	if status == "disabled" || status == "expired" {
		return false
	}
	if !bypassQuota && utils.AccountQuotaExhausted(acc) {
		return false
	}
	return true
}

// GetRotationPoolStatus 给前端状态面板用。
func (a *App) GetRotationPoolStatus() RotationPoolStatus {
	if a.rotationPool == nil || a.store == nil {
		return RotationPoolStatus{}
	}
	s := a.store.GetSettings()
	rp := a.rotationPool
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	st := RotationPoolStatus{
		Enabled:             rp.enabled,
		MemberCount:         len(rp.memberIDs),
		IntervalMin:         rp.intervalMin,
		QuotaRefreshMin:     rp.quotaRefreshMin,
		LastSwitchedTo:      rp.lastSwitchedTo,
		LastError:           rp.lastError,
		TotalSwitches:       rp.totalSwitches,
		TotalQuotaRefreshes: rp.totalQuotaRefreshes,
		PausedByPin:         s.ManualPinEnabled,
	}
	if !rp.nextSwitchAt.IsZero() {
		st.NextSwitchAt = rp.nextSwitchAt.Format(time.RFC3339)
	}
	if !rp.lastSwitchedAt.IsZero() {
		st.LastSwitchedAt = rp.lastSwitchedAt.Format(time.RFC3339)
	}
	if !rp.lastQuotaRefreshAt.IsZero() {
		st.LastQuotaRefreshAt = rp.lastQuotaRefreshAt.Format(time.RFC3339)
	}
	return st
}

// RotationPoolSwitchNow 立即触发一次切换（调试 + 用户手动加速）。
func (a *App) RotationPoolSwitchNow() (string, error) {
	return a.rotationPoolSwitchOnce("手动")
}

// RotationPoolRefreshQuotasNow 立即刷一次池内额度。
func (a *App) RotationPoolRefreshQuotasNow() {
	go a.rotationPoolRefreshQuotas()
}

func dedupNonEmpty(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
