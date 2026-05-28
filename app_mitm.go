package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// ═══════════════════════════════════════
// MITM 代理
// ═══════════════════════════════════════

// syncMitmPoolKeys syncs pool keys from store accounts that have WindsurfAPIKey.
// ★ 遵守 AutoSwitchPlanFilter 设置：只有计划匹配的账号才加入 MITM 号池
// ★ 传入 Plan 信息，使 MITM 代理能区分 Pro/Trial key（用于全局限速退避时优先 Pro）
// F7-REMOVAL: 下面 SmartFriend 三行注释 + 下面 settings.SmartFriendEnabled 传参一并删除
// ★ SmartFriend(F7) 开启时，服务端按 SMART_FRIEND 计费、绕过日/周限额，
//
//	「显示已耗尽」的账号实际仍可用，必须保留在号池里——否则手动切号会
//	因「号池找不到 key」而失败。
func (a *App) syncMitmPoolKeys() {
	accounts := a.store.GetAllAccounts()
	settings := a.store.GetSettings()
	filter := settings.AutoSwitchPlanFilter

	infos := collectEligibleMitmPoolKeyInfos(accounts, filter, settings.SmartFriendEnabled)
	a.mitmProxy.SetPoolKeysWithPlan(infos)
}

func collectEligibleMitmPoolKeyInfos(accounts []models.Account, planFilter string, bypassQuota bool) []services.PoolKeyInput {
	var infos []services.PoolKeyInput
	seen := make(map[string]struct{})
	for _, acc := range accounts {
		if !accountEligibleForUsage(&acc, planFilter, true, bypassQuota) {
			continue
		}
		key := strings.TrimSpace(acc.WindsurfAPIKey)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		infos = append(infos, services.PoolKeyInput{
			APIKey: key,
			Plan:   acc.PlanName,
		})
	}
	return infos
}

// collectEligibleMitmAPIKeys returns just the API key strings (backward compat wrapper).
func collectEligibleMitmAPIKeys(accounts []models.Account, planFilter string, bypassQuota bool) []string {
	infos := collectEligibleMitmPoolKeyInfos(accounts, planFilter, bypassQuota)
	keys := make([]string, 0, len(infos))
	for _, info := range infos {
		keys = append(keys, info.APIKey)
	}
	return keys
}

// StartMitmProxy starts the MITM reverse proxy with full system setup.
func (a *App) StartMitmProxy() error {
	a.syncMitmPoolKeys()

	// 生成 CA（如果不存在）
	if _, err := services.EnsureCA(services.TargetDomain); err != nil {
		return err
	}

	// macOS: CA 信任必须走 Terminal.app（osascript admin 无法设置信任）。
	// 如果尚未真正信任，先弹出 Terminal 引导用户输密码装信任。
	if runtime.GOOS == "darwin" && !services.IsCAInstalled() {
		if err := services.InstallCA(); err != nil {
			return fmt.Errorf("CA 信任安装失败: %w", err)
		}
	}

	// macOS: 把 hosts + DNS 刷新合并到 porthelper 的 osascript（单次密码）
	if runtime.GOOS == "darwin" {
		if err := services.DarwinBatchSetup(); err != nil {
			return err
		}
	}

	if err := a.mitmProxy.Start(); err != nil {
		return err
	}

	// 非 macOS 走原有逐步设置；macOS batch 已处理过的会跳过（幂等检查）
	a.applyMitmSystemSetup()
	return nil
}

// StopMitmProxy stops the MITM reverse proxy.
func (a *App) StopMitmProxy() error {
	return a.mitmProxy.Stop()
}

// SwitchMitmToNext 手动切到 MITM 号池中的下一席位。
func (a *App) SwitchMitmToNext() (string, error) {
	a.syncMitmPoolKeys()
	accounts := a.store.GetAllAccounts()
	currentID := findAccountIDForMITMAPIKey(accounts, a.mitmProxy.CurrentAPIKey())
	nextAcc, err := pickNextMitmSwitchableAccount(accounts, currentID, a.store.GetSettings().AutoSwitchPlanFilter, a.shouldBypassQuotaCheck())
	if err != nil {
		return "", fmt.Errorf("MITM 号池为空，或当前没有可切换的席位")
	}
	return a.switchMitmAccountAndSyncLocalSession(nextAcc)
}

// SwitchMitmToAccount 手动切到指定账号对应的 MITM API Key。
//
// 副作用：成功后自动 setManualPin(id) 锁定该账号，所有自动切换通道暂停。
// 这是用户「手动切到 X 就是想用 X」的明确意图表达，避免随后被 auto-rotate
// 又换走。用户需要恢复自动行为时点 Header 的「解锁」按钮即可。
func (a *App) SwitchMitmToAccount(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("账号 ID 不能为空")
	}
	acc, err := a.store.GetAccount(id)
	if err != nil {
		return "", err
	}
	apiKey := strings.TrimSpace(acc.WindsurfAPIKey)
	if apiKey == "" {
		return "", fmt.Errorf("该账号没有 API Key，无法用于 MITM 手动切号")
	}
	// F3: 用户手动切到此账号 → 明确意图，清掉其冷却（如果之前因 quota/rate_limited 被冷却）
	switchCooldown.clear(id)
	res, err := a.switchMitmAccountAndSyncLocalSession(acc)
	if err == nil {
		a.setManualPin(id)
	}
	return res, err
}

// GetMitmProxyStatus returns the current proxy status.
// 在 MitmProxy.Status() 基础上，用号池账号信息填充 PoolKeyInfo 的 Email/Nickname。
//
// ★ 必须用 KeyHash 严格匹配，不能再用 KeyShort 前缀。
// 历史 bug：KeyShort 取 full key 前 16 字符，对 devin-session-token$<JWT> 这种
// 共享 "devin-session-to" 前缀的账号会全部撞车 → Go map 随机迭代取首个匹配 →
// 所有账号都被贴上同一个 pool 入口的 Email/IsCurrent → 前端显示 "全部账号都
// 当前活跃"。
func (a *App) GetMitmProxyStatus() services.MitmProxyStatus {
	st := a.mitmProxy.Status()
	if len(st.PoolStatus) > 0 && a.store != nil {
		accounts := a.store.GetAllAccounts()
		// 按 KeyHash 索引（与 PoolKeyInfo.KeyHash 同一函数生成）
		hashToAccount := make(map[string]models.Account, len(accounts))
		for _, acc := range accounts {
			k := strings.TrimSpace(acc.WindsurfAPIKey)
			if k == "" {
				continue
			}
			hashToAccount[services.HashPoolKey(k)] = acc
		}
		for i := range st.PoolStatus {
			if acc, ok := hashToAccount[st.PoolStatus[i].KeyHash]; ok {
				st.PoolStatus[i].Email = acc.Email
				st.PoolStatus[i].Nickname = acc.Nickname
			}
		}
	}
	return st
}

// GetMitmSessionBindings returns all active session bindings for the frontend.
func (a *App) GetMitmSessionBindings() []services.SessionBindingInfo {
	return a.mitmProxy.GetSessionBindings()
}

// UnbindMitmSession removes a session binding by conversation ID prefix.
func (a *App) UnbindMitmSession(convIDPrefix string) bool {
	return a.mitmProxy.UnbindSession(convIDPrefix)
}

// SetupMitmCA generates and installs the CA certificate.
func (a *App) SetupMitmCA() error {
	if _, err := services.EnsureCA(services.TargetDomain); err != nil {
		return err
	}
	err := services.InstallCA()
	services.InvalidateCACache()
	return err
}

// SetupMitmHosts adds hosts file entries for all target domains.
func (a *App) SetupMitmHosts() error {
	if err := services.AddHostsEntry(services.TargetDomain); err != nil {
		return err
	}
	_ = services.AddProxyOverride()
	a.injectFirstPoolKeyToCodeiumConfig()
	return nil
}

// PrereqStepResult 描述前置条件单步的执行结果（CA / Hosts）。
type PrereqStepResult struct {
	Step    string `json:"step"`            // "ca" | "hosts"
	Title   string `json:"title"`           // 中文步骤名
	OK      bool   `json:"ok"`              // 是否就绪（成功安装或已就绪）
	Skipped bool   `json:"skipped"`         // true 表示已就绪未做修改
	Error   string `json:"error,omitempty"` // 失败原因（原始 message）
	Hint    string `json:"hint,omitempty"`  // 给用户看的修复指引
}

// SetupMitmAll 顺序执行 CA + Hosts 安装，返回逐步结果。
// 已就绪的步骤会被标记 Skipped=true，避免重复弹密码框。
// CA 失败时会跳过 Hosts —— 没有信任的 CA，hosts 劫持没有意义。
func (a *App) SetupMitmAll() []PrereqStepResult {
	results := make([]PrereqStepResult, 0, 2)

	// ① CA
	caStep := PrereqStepResult{Step: "ca", Title: "CA 证书"}
	if services.IsCAInstalled() {
		caStep.OK = true
		caStep.Skipped = true
	} else {
		if err := a.SetupMitmCA(); err != nil {
			caStep.Error = err.Error()
			caStep.Hint = prereqCAHint(err)
		} else {
			caStep.OK = true
		}
	}
	results = append(results, caStep)

	// ② Hosts —— CA 失败时直接跳过
	hostsStep := PrereqStepResult{Step: "hosts", Title: "Hosts 劫持"}
	if !caStep.OK {
		hostsStep.Skipped = true
		hostsStep.Hint = "CA 未就绪，已跳过 Hosts 配置（先解决 CA 问题再重试）"
		results = append(results, hostsStep)
		return results
	}
	if services.IsHostsMapped(services.TargetDomain) {
		hostsStep.OK = true
		hostsStep.Skipped = true
	} else {
		if err := a.SetupMitmHosts(); err != nil {
			hostsStep.Error = err.Error()
			hostsStep.Hint = prereqHostsHint(err)
		} else {
			hostsStep.OK = true
		}
	}
	results = append(results, hostsStep)
	return results
}

// UninstallMitmCA 仅卸载 CA 信任，不动 hosts/proxy。
func (a *App) UninstallMitmCA() error {
	err := services.UninstallCA()
	services.InvalidateCACache()
	return err
}

// UninstallMitmHosts 仅移除 hosts 劫持和 ProxyOverride（Windows），不动 CA。
func (a *App) UninstallMitmHosts() error {
	if err := services.RemoveHostsEntry(services.TargetDomain); err != nil {
		return err
	}
	_ = services.RemoveProxyOverride()
	return nil
}

// prereqCAHint 根据 CA 安装错误生成可操作的修复指引。
func prereqCAHint(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch runtime.GOOS {
	case "darwin":
		if strings.Contains(msg, "timeout") || strings.Contains(msg, "未在终端窗口完成信任安装") {
			return "终端密码超时未输入。请重试，并在弹出的 Terminal 窗口里输入登录密码后回车。"
		}
		if strings.Contains(msg, "user cancelled") || strings.Contains(msg, "用户取消") {
			return "你取消了 Terminal 授权弹窗。重试并允许 Terminal.app 打开。"
		}
		return "如需手动信任：双击 ~/.windsurf-tools/ca.crt → 钥匙串访问 → 信任 → 始终信任。"
	case "windows":
		if strings.Contains(msg, "denied") || strings.Contains(msg, "拒绝") {
			return "需要管理员权限。请用「以管理员身份运行」启动 Windsurf Tools。"
		}
		return "可以手动用 certutil -addstore Root <ca 路径> 安装到「受信任的根证书颁发机构」。"
	default:
		return "需要 sudo 权限。请确保 pkexec/sudo 可用，或将 ca.crt 复制到 /usr/local/share/ca-certificates/ 后执行 update-ca-certificates。"
	}
}

// prereqHostsHint 根据 hosts 写入错误生成修复指引。
func prereqHostsHint(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch runtime.GOOS {
	case "darwin", "linux":
		if strings.Contains(msg, "permission") || strings.Contains(msg, "denied") {
			return "/etc/hosts 写入需要管理员密码。重试并在弹出的密码框里输入登录密码。"
		}
	case "windows":
		if strings.Contains(msg, "denied") || strings.Contains(msg, "拒绝") {
			return "C:\\Windows\\System32\\drivers\\etc\\hosts 需要管理员权限。请「以管理员身份运行」。"
		}
	}
	return "可手动编辑系统 hosts 文件，添加：127.0.0.1 server.self-serve.windsurf.com"
}

// TeardownMitm removes hosts entry, cleans ProxyOverride, restores Codeium config, and uninstalls CA.
//
// 子步骤幂等：每步先检测状态再决定是否执行，避免重复关闭软件 / 重复
// onBeforeClose+OnShutdown 时多弹一次密码框 / 多吃一次 "not found" 错误。
func (a *App) TeardownMitm() error {
	var errs []error

	// 停止 MITM 代理（macOS: 此步骤内含 DisablePFRedirect，跳过以避免额外密码弹窗）。
	// 已停止的代理重复 Stop 是 no-op，安全。
	if a.mitmProxy != nil {
		if err := a.mitmProxy.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("停止 MITM 代理: %w", err))
		}
	}

	// 非特权操作先做（每个都自带 "已经清理过" 的快速返回）
	if services.HasProxyOverride() {
		if err := services.RemoveProxyOverride(); err != nil {
			errs = append(errs, fmt.Errorf("恢复 ProxyOverride: %w", err))
		}
	}
	if services.HasCodeiumConfigBackup() {
		if err := services.RestoreCodeiumConfig(); err != nil {
			errs = append(errs, fmt.Errorf("恢复 Codeium 配置: %w", err))
		}
	}

	// macOS: 合并所有特权操作为一次密码弹窗（恢复 hosts + 卸载 CA）。
	// 当 hosts 与 CA 都已被外部清理时跳过，避免无意义弹窗。
	if runtime.GOOS == "darwin" {
		if services.IsHostsMapped(services.TargetDomain) || services.IsCAInstalled() {
			if err := services.DarwinBatchTeardown(); err != nil {
				errs = append(errs, err)
			}
		}
	} else {
		if services.IsHostsMapped(services.TargetDomain) {
			if err := services.RemoveHostsEntry(services.TargetDomain); err != nil {
				errs = append(errs, fmt.Errorf("恢复 hosts: %w", err))
			}
		}
		if services.IsCAInstalled() {
			if err := services.UninstallCA(); err != nil {
				errs = append(errs, fmt.Errorf("卸载 CA: %w", err))
			}
		}
	}

	services.InvalidateCACache()
	return errors.Join(errs...)
}

// applyMitmSystemSetup 一键应用所有系统修改 (MITM 启动时调用)
func (a *App) applyMitmSystemSetup() {
	_ = services.AddHostsEntry(services.TargetDomain)
	_ = services.AddProxyOverride()
	a.injectFirstPoolKeyToCodeiumConfig()
	// 恢复持久化的 dump/抓包设置
	settings := a.store.GetSettings()
	a.mitmProxy.SetDebugDump(settings.MitmDebugDump)
	a.mitmProxy.SetFullCapture(settings.MitmFullCapture)
}

// injectFirstPoolKeyToCodeiumConfig 将号池中第一个可用 API Key 写入 Codeium config
func (a *App) injectFirstPoolKeyToCodeiumConfig() {
	keys := collectEligibleMitmAPIKeys(a.store.GetAllAccounts(), a.store.GetSettings().AutoSwitchPlanFilter, a.shouldBypassQuotaCheck())
	if len(keys) == 0 {
		return
	}
	_ = services.InjectCodeiumConfig(keys[0])
}

// GetMitmCAPath returns the CA certificate file path.
func (a *App) GetMitmCAPath() string {
	return services.GetCACertPath()
}

// ClearMitmKeyExhausted 手动解除单个 key 的「额度耗尽」锁定。
// 用户在「自动切下一席」关闭后或误判锁定时使用。
// 返回 true 表示真的解锁了一个号。
func (a *App) ClearMitmKeyExhausted(apiKey string) bool {
	if a.mitmProxy == nil {
		return false
	}
	return a.mitmProxy.ClearKeyExhausted(apiKey)
}

// ClearAllMitmExhausted 批量解除号池中所有「额度耗尽」锁定。返回解锁个数。
func (a *App) ClearAllMitmExhausted() int {
	if a.mitmProxy == nil {
		return 0
	}
	return a.mitmProxy.ClearAllExhausted()
}

// ToggleMitmDebugDump 开启/关闭 MITM proto dump
func (a *App) ToggleMitmDebugDump(enabled bool) {
	a.mitmProxy.SetDebugDump(enabled)
	settings := a.store.GetSettings()
	settings.MitmDebugDump = enabled
	a.store.UpdateSettings(settings)
}

// 注：原 GetMitmDebugDumpEnabled / GetProtoDumpDir 已删除：
//   - 前端直接读 settings.mitm_debug_dump 即可，不需要单独查 proxy 内存状态
//   - 用户访问 dump 目录走 RevealProtoDumpDir（带 Finder 打开）；裸路径不再单独暴露

// ToggleMitmFullCapture 开启/关闭全量抓包
func (a *App) ToggleMitmFullCapture(enabled bool) {
	a.mitmProxy.SetFullCapture(enabled)
	settings := a.store.GetSettings()
	settings.MitmFullCapture = enabled
	a.store.UpdateSettings(settings)
}

// GetMitmFullCaptureEnabled 返回全量抓包是否开启
func (a *App) GetMitmFullCaptureEnabled() bool {
	return a.mitmProxy.FullCaptureEnabled()
}

// 注：原 GetCaptureDir 已删除 —— 用户访问抓包目录走 RevealCaptureDir（带 Finder 打开）；
// 裸路径不再单独暴露给前端。

// RevealCaptureDir 在系统文件管理器中打开全量抓包目录。
// 目录不存在时先创建（开关刚打开还没有请求落盘时）。
// 返回路径以便前端显示；error 表示 Finder/Explorer 调用失败。
func (a *App) RevealCaptureDir() (string, error) {
	dir := services.CaptureDir()
	if dir == "" {
		return "", fmt.Errorf("capture dir 未配置")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return dir, fmt.Errorf("创建目录失败: %w", err)
	}
	return dir, revealPathInFileManager(dir)
}

// RevealProtoDumpDir 在系统文件管理器中打开 protobuf dump 目录。
func (a *App) RevealProtoDumpDir() (string, error) {
	dir := services.ProtoDumpDir()
	if dir == "" {
		return "", fmt.Errorf("proto dump dir 未配置")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return dir, fmt.Errorf("创建目录失败: %w", err)
	}
	return dir, revealPathInFileManager(dir)
}

func (a *App) describeMitmKey(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}
	for _, acc := range a.store.GetAllAccounts() {
		if strings.TrimSpace(acc.WindsurfAPIKey) != apiKey {
			continue
		}
		if email := strings.TrimSpace(acc.Email); email != "" {
			return email
		}
		if nickname := strings.TrimSpace(acc.Nickname); nickname != "" {
			return nickname
		}
		break
	}
	if len(apiKey) > 16 {
		return apiKey[:12] + "..."
	}
	return apiKey
}

func (a *App) handleMitmKeyAccessDenied(apiKey, detail string) {
	apiKey = strings.TrimSpace(apiKey)
	detail = strings.TrimSpace(detail)
	if a == nil || a.store == nil || apiKey == "" {
		return
	}

	accID := findAccountIDForMITMAPIKey(a.store.GetAllAccounts(), apiKey)
	if accID == "" {
		utils.DLog("[回调] onKeyAccessDenied: 未找到匹配 key=%s...", apiKey[:minInt(12, len(apiKey))])
		return
	}

	acc, err := a.store.GetAccount(accID)
	if err != nil {
		utils.DLog("[回调] onKeyAccessDenied: 读取账号失败 id=%s err=%v", accID[:minInt(8, len(accID))], err)
		return
	}

	before := acc
	applyAccessErrorStatus(&acc, fmt.Errorf("%s", detail))
	if acc == before {
		utils.DLog("[回调] onKeyAccessDenied: 未命中降权规则，保持原状态 id=%s", accID[:minInt(8, len(accID))])
		return
	}

	if err := a.store.UpdateAccount(acc); err != nil {
		utils.DLog("[回调] onKeyAccessDenied: 保存账号失败 id=%s err=%v", accID[:minInt(8, len(accID))], err)
		return
	}
	a.syncMitmPoolKeys()
	// F3: 触发冷却（ratelimit 类）—— 同账号连续被拒会指数退避
	if s := a.store.GetSettings(); s.SwitchCooldownEnabled {
		switchCooldown.apply(accID, "ratelimit", s.SwitchCooldownBaseSec)
	}
	utils.DLog("[回调] onKeyAccessDenied: 已持久化 %s status=%s plan=%s", labelAccountResult(acc), acc.Status, acc.PlanName)
}

func shouldSyncMitmLocalSessionOnKeyChange(reason string) bool {
	// ★ MITM 按 conversation_id 路由，自动轮转不修改本地登录态（保持 Pro 身份）
	// 只有用户手动切号 (switchMitmAccountAndSyncLocalSession) 才同步本地 auth
	return false
}

func (a *App) handleMitmCurrentKeyChanged(apiKey, reason string) {
	apiKey = strings.TrimSpace(apiKey)
	reason = strings.TrimSpace(reason)
	if a == nil || a.store == nil || apiKey == "" {
		return
	}
	// F2: 记录到 switch_history.jsonl（无论后面是否同步本地登录态都记，
	// Dashboard 折线图覆盖所有切号路径）
	if a.switchHistory != nil {
		ev := SwitchEvent{
			At:       time.Now().Format(time.RFC3339),
			KeyShort: shortHexKey(apiKey),
			Reason:   normalizeSwitchReason(reason),
		}
		if accID := findAccountIDForMITMAPIKey(a.store.GetAllAccounts(), apiKey); accID != "" {
			if acc, err := a.store.GetAccount(accID); err == nil {
				ev.Email = acc.Email
			}
		}
		go a.switchHistory.Append(ev)
	}
	if !shouldSyncMitmLocalSessionOnKeyChange(reason) {
		return
	}

	accID := findAccountIDForMITMAPIKey(a.store.GetAllAccounts(), apiKey)
	if accID == "" {
		utils.DLog("[回调] onCurrentKeyChanged: 未找到匹配 key=%s... reason=%s", apiKey[:minInt(12, len(apiKey))], reason)
		return
	}
	acc, err := a.store.GetAccount(accID)
	if err != nil {
		utils.DLog("[回调] onCurrentKeyChanged: 读取账号失败 id=%s err=%v", accID[:minInt(8, len(accID))], err)
		return
	}
	if strings.TrimSpace(acc.Token) == "" {
		utils.DLog("[回调] onCurrentKeyChanged: %s 缺少 Token，跳过本地 auth 同步 reason=%s", labelAccountResult(acc), reason)
		return
	}
	utils.DLog("[回调] onCurrentKeyChanged: MITM key 已切换 -> %s reason=%s", labelAccountResult(acc), reason)
}

func (a *App) switchMitmAccountAndSyncLocalSession(acc models.Account) (string, error) {
	// 用户在 UI 上明确选定该账号 → 即使额度耗尽也允许切过去。
	// SmartFriend 模式下「耗尽」其实仍可用；非 SmartFriend 下用户也可能想
	// 提前预定该号(等额度重置)或忽略我们的快照判断。
	prepared, err := a.prepareAccountForUsageManual(acc)
	if err != nil {
		return "", err
	}
	apiKey := strings.TrimSpace(prepared.WindsurfAPIKey)
	if apiKey == "" {
		return "", fmt.Errorf("该账号没有 API Key，无法用于 MITM 手动切号")
	}

	a.syncMitmPoolKeys()
	if !a.mitmProxy.SwitchToKey(apiKey) {
		return "", fmt.Errorf("该账号当前未加入 MITM 号池，请检查套餐筛选、额度状态或 API Key 是否可用")
	}
	utils.DLog("[MITM] 手动切号成功: %s", prepared.Email)
	return a.describeMitmKey(apiKey), nil
}

// F7-REMOVAL: 整函数删除。调用点在 app.go / app_settings.go 中同步去掉。
func (a *App) syncSmartFriendConfig() {
	if a.mitmProxy == nil || a.store == nil {
		return
	}
	s := a.store.GetSettings()
	a.mitmProxy.SetSmartFriendEnabled(s.SmartFriendEnabled)
}

// syncMitmAutoSwitchOnQuotaExhausted 把用户「额度耗尽时自动切下一席」开关推给 MITM。
// 关闭后：MITM 收到 quota_exhausted 不再锁号 / 切号 / 进冷却，错误透传给 IDE。
// 启动时（app.go）和 settings 更新时（app_settings.go）都会调用。
func (a *App) syncMitmAutoSwitchOnQuotaExhausted() {
	if a.mitmProxy == nil || a.store == nil {
		return
	}
	s := a.store.GetSettings()
	a.mitmProxy.SetAutoSwitchOnQuotaExhausted(s.AutoSwitchOnQuotaExhausted)
}

// syncMitmDebugAndCapture 把 MitmDebugDump / MitmFullCapture 同步到 proxy。
// 启动时由 attachMitmRuntimeSwitches 处理；这里覆盖 UpdateSettings / ImportSettings
// 路径 —— 没有这个 sync，导入配置后这两项要重启才生效。
func (a *App) syncMitmDebugAndCapture() {
	if a.mitmProxy == nil || a.store == nil {
		return
	}
	s := a.store.GetSettings()
	a.mitmProxy.SetDebugDump(s.MitmDebugDump)
	a.mitmProxy.SetFullCapture(s.MitmFullCapture)
}

func (a *App) syncForgeConfig() {
	if a.mitmProxy == nil {
		return
	}
	s := a.store.GetSettings()
	a.mitmProxy.SetForgeConfig(services.ForgeConfig{
		Enabled:            s.ForgeEnabled,
		FakeCredits:        s.FakeCredits,
		FakeCreditsPremium: s.FakeCreditsPremium,
		FakeCreditsOther:   s.FakeCreditsOther,
		FakeCreditsUsed:    s.FakeCreditsUsed,
		FakeSubType:        s.FakeSubscriptionType,
		ExtendYears:        s.FakeBillingExtendYears,
	})
}

// resolveActiveJailbreakOverride 根据 settings 计算实际生效的 override 文本。
// 顺序：preset 显式选中 ≠ custom → 用 preset 文本；否则 → 看 Source：
//   - file → 读 OverrideFile，失败降级到 inline
//   - inline / 空 → 用 MitmJailbreakOverride textarea；为空再回退默认
//
// 返回 (text, source, filePath)。filePath 仅 Source=file 时填实际路径。
func (a *App) resolveActiveJailbreakOverride() (string, string, string) {
	if a.store == nil {
		return services.DefaultJailbreakOverride, "inline", ""
	}
	s := a.store.GetSettings()

	// 1) preset 优先
	if pid := strings.TrimSpace(s.MitmJailbreakPresetID); pid != "" && pid != services.JailbreakPresetIDCustom {
		if p := services.GetJailbreakPresetByID(pid); p != nil && p.Text != "" {
			return p.Text, "preset:" + pid, ""
		}
	}

	source := strings.TrimSpace(s.MitmJailbreakOverrideSource)
	if source == "" {
		source = services.JailbreakOverrideSourceInline
	}

	// 2) file 来源
	if source == services.JailbreakOverrideSourceFile {
		text, resolved, err := services.LoadJailbreakOverrideFile(s.MitmJailbreakOverrideFile)
		if err == nil && strings.TrimSpace(text) != "" {
			return text, services.JailbreakOverrideSourceFile, resolved
		}
		utils.DLog("[Jailbreak] file 源读取失败 (%s)，降级到 inline: %v", resolved, err)
	}

	// 3) inline 来源（默认）
	override := strings.TrimSpace(s.MitmJailbreakOverride)
	if override == "" {
		override = services.DefaultJailbreakOverride
	}
	return override, services.JailbreakOverrideSourceInline, ""
}

// syncJailbreakConfig 把当前 settings 里的破限配置推到 MitmProxy。
// 文本来源按 resolveActiveJailbreakOverride 的优先级解析。
func (a *App) syncJailbreakConfig() {
	if a.mitmProxy == nil || a.store == nil {
		return
	}
	s := a.store.GetSettings()
	text, source, filePath := a.resolveActiveJailbreakOverride()
	a.mitmProxy.SetJailbreakConfig(services.JailbreakConfig{
		Enabled:  s.MitmJailbreakEnabled,
		Override: text,
		PresetID: strings.TrimSpace(s.MitmJailbreakPresetID),
		Source:   source,
		FilePath: filePath,
	})
}

// GetJailbreakDefaultOverride 暴露默认破限文本给前端，供「恢复默认」按钮使用。
func (a *App) GetJailbreakDefaultOverride() string {
	return services.DefaultJailbreakOverride
}

// ── 破限增强 API（v1.2.0）──

// ListJailbreakPresets 返回预设列表供前端下拉。
func (a *App) ListJailbreakPresets() []services.JailbreakPreset {
	return services.ListJailbreakPresets()
}

// GetJailbreakRuntime 一次性返回当前生效状态 + 注入统计 + 文件信息，
// 给 UI 状态面板用。比让前端调 3 个 API 拼接更优。
type JailbreakRuntime struct {
	Enabled       bool                            `json:"enabled"`
	PresetID      string                          `json:"preset_id"`
	Source        string                          `json:"source"`        // inline / file / preset:xxx
	ActiveText    string                          `json:"active_text"`   // 当前生效的完整文本
	ActiveLength  int                             `json:"active_length"` // 字符数
	FilePath      string                          `json:"file_path,omitempty"`
	FileStatus    *services.JailbreakFileStatus   `json:"file_status,omitempty"`
	Stats         services.JailbreakStatsSnapshot `json:"stats"`
	WarnAnthropic bool                            `json:"warn_anthropic"` // 检测到 cyber 雷词
}

func (a *App) GetJailbreakRuntime() JailbreakRuntime {
	if a.mitmProxy == nil || a.store == nil {
		return JailbreakRuntime{}
	}
	s := a.store.GetSettings()
	text, source, filePath := a.resolveActiveJailbreakOverride()
	rt := JailbreakRuntime{
		Enabled:       s.MitmJailbreakEnabled,
		PresetID:      strings.TrimSpace(s.MitmJailbreakPresetID),
		Source:        source,
		ActiveText:    text,
		ActiveLength:  len([]rune(text)),
		FilePath:      filePath,
		Stats:         a.mitmProxy.GetJailbreakStats(),
		WarnAnthropic: services.JailbreakTextHasCyberHazardWords(text),
	}
	if source == services.JailbreakOverrideSourceFile || filePath != "" {
		st := services.InspectJailbreakOverrideFile(s.MitmJailbreakOverrideFile)
		rt.FileStatus = &st
	}
	return rt
}

// SaveJailbreakOverrideFile 把当前 settings 里的 textarea 文本写到 file。
// 用于「保存到文件」按钮：把 inline 文本沉淀为外部文件后切到 file 源。
func (a *App) SaveJailbreakOverrideFile(text string) (string, error) {
	if a.store == nil {
		return "", nil
	}
	s := a.store.GetSettings()
	return services.SaveJailbreakOverrideFile(s.MitmJailbreakOverrideFile, text)
}

// OpenJailbreakOverrideFile 用系统默认编辑器打开 override 文件。
// 路径不存在时先用当前 settings 里的 textarea 文本（或默认 fallback）
// 创建文件再打开，避免用户点了 "编辑" 弹空白。
func (a *App) OpenJailbreakOverrideFile() (string, error) {
	if a.store == nil {
		return "", nil
	}
	s := a.store.GetSettings()
	resolved := services.ResolveJailbreakOverrideFilePath(s.MitmJailbreakOverrideFile)
	if !services.JailbreakOverrideFileExists(resolved) {
		seed := strings.TrimSpace(s.MitmJailbreakOverride)
		if seed == "" {
			seed = services.DefaultJailbreakOverride
		}
		if _, err := services.SaveJailbreakOverrideFile(resolved, seed); err != nil {
			return resolved, err
		}
	}
	return resolved, openPathWithSystem(resolved)
}

// RevealJailbreakOverrideFolder 在 Finder/Explorer 打开 override 文件所在目录。
func (a *App) RevealJailbreakOverrideFolder() (string, error) {
	if a.store == nil {
		return "", nil
	}
	s := a.store.GetSettings()
	resolved := services.ResolveJailbreakOverrideFilePath(s.MitmJailbreakOverrideFile)
	return resolved, revealPathInFileManager(resolved)
}

// ResetJailbreakStats 清零注入计数（UI debug 用）。
func (a *App) ResetJailbreakStats() {
	if a.mitmProxy == nil {
		return
	}
	a.mitmProxy.ResetJailbreakStats()
}

func (a *App) syncStaticCacheConfig() {
	if a.mitmProxy == nil || a.store == nil {
		return
	}
	s := a.store.GetSettings()
	a.mitmProxy.SetStaticCacheConfig(services.StaticCacheConfig{
		Enabled:  s.StaticCacheIntercept,
		CacheDir: a.staticCacheDir(),
	})
}

func (a *App) staticCacheDir() string {
	if a.store == nil {
		return ""
	}
	return a.store.DataDir() + string(os.PathSeparator) + "static"
}

// 注：原 GetStaticCacheDir 已删除 —— Static cache 是 .bin 文件直返的性能优化，
// 用户层面不需要查看。目录创建由 syncStaticCacheConfig 在 cache 路径首次使用前完成。
