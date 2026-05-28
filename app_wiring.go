package main

// app_wiring.go ── 装配阶段的回调钩子。
//
// 设计动机：
//
//	mitmProxy / openaiRelay 启动后会通过 SetOnXxx 回调把「key 耗尽 / 上游
//	拒绝 / current key 切换 / Relay 调用成功」等事件回到 App 层。原来这一坨
//	注册逻辑挤在 initBackend 里，让本就 100+ 行的 init 越发难读。
//
//	抽到这里之后：
//	- initBackend 只负责构造服务 / 装配模块，回调注册一行调用 wireProxyCallbacks；
//	- 后续如要新增 / 删除某个回调，只需在本文件一处改动；
//	- 跨域协作（onKeyExhausted 触发 quota 刷新 + auto rotate）的策略集中可见。
//
// 行为不变：每个闭包对应注释保留原始上下文，逻辑与抽离前 1:1 等价。

import "windsurf-tools-wails/backend/utils"

// wireProxyCallbacks 注册 MITM Proxy 与 OpenAIRelay 的事件回调。
// 在 initBackend 中所有服务实例创建完成后调用一次。
func (a *App) wireProxyCallbacks() {
	if a == nil || a.mitmProxy == nil {
		return
	}
	a.mitmProxy.SetOnKeyExhausted(a.onMitmKeyExhausted)
	a.mitmProxy.SetOnKeyAccessDenied(func(apiKey, detail string) {
		utils.DLog("[回调] onKeyAccessDenied 触发: key=%s...", apiKey[:min(12, len(apiKey))])
		a.handleMitmKeyAccessDenied(apiKey, detail)
	})
	a.mitmProxy.SetOnCurrentKeyChanged(func(apiKey, reason string) {
		utils.DLog("[回调] onCurrentKeyChanged 触发: key=%s... reason=%s", apiKey[:min(12, len(apiKey))], reason)
		a.handleMitmCurrentKeyChanged(apiKey, reason)
	})

	if a.openaiRelay != nil {
		a.openaiRelay.SetOnSuccess(func(apiKey string) {
			accounts := a.store.GetAllAccounts()
			accID := findAccountIDForMITMAPIKey(accounts, apiKey)
			if accID == "" {
				return
			}
			_ = a.RefreshAccountQuota(accID)
		})
	}
}

// onMitmKeyExhausted 处理 MITM key 耗尽事件：
//  1. 在号池中匹配回账号 ID；
//  2. 立即刷新该账号的额度快照；
//  3. 按 Pin / SmartFriend / AutoSwitch 优先级决定是否触发自动切号；
//  4. 切号失败时弹桌面通知（rate-limited 60s）让用户立即感知。
func (a *App) onMitmKeyExhausted(apiKey string) {
	utils.DLog("[回调] onKeyExhausted 触发: key=%s...", apiKey[:min(12, len(apiKey))])
	accID := findAccountIDForMITMAPIKey(a.store.GetAllAccounts(), apiKey)
	if accID == "" {
		utils.DLog("[回调] onKeyExhausted: 在号池中未找到匹配 key，跳过")
		return
	}
	utils.DLog("[回调] onKeyExhausted: 匹配到账号 id=%s，刷新额度...", accID[:min(8, len(accID))])
	_ = a.RefreshAccountQuota(accID)

	s := a.store.GetSettings()
	// F3: 触发冷却（quota 类）—— 后续自动选号会跳过此账号 cooldown 时间内
	if s.SwitchCooldownEnabled {
		switchCooldown.apply(accID, "quota", s.SwitchCooldownBaseSec)
	}
	// Pin 优先：手动锁定时跳过所有自动切（用户 100% 控制）
	if s.ManualPinEnabled {
		utils.DLog("[回调] onKeyExhausted: ManualPin 生效 (pin=%s)，跳过自动切", s.ManualPinAccountID[:min(8, len(s.ManualPinAccountID))])
		return
	}
	// F7-REMOVAL: 整个 if SmartFriendEnabled 分支删除，onKeyExhausted 会回到「AutoSwitch 环境」
	if s.SmartFriendEnabled {
		utils.DLog("[回调] onKeyExhausted: SmartFriend 模式已开启，跳过自动切")
		return
	}
	if !s.AutoSwitchOnQuotaExhausted {
		utils.DLog("[回调] onKeyExhausted: AutoSwitchOnQuotaExhausted=false，不切号")
		return
	}
	utils.DLog("[回调] onKeyExhausted: AutoSwitch=true → 立即MITM轮换")
	next, err := a.rotateMitmToNextAvailable(accID, s.AutoSwitchPlanFilter)
	if err != nil {
		utils.DLog("[回调] onKeyExhausted: MITM轮换失败: %v", err)
		// 关键事件：额度耗尽 + 没切到下一个 → 用户需要立刻知道
		a.notify(NotifyKindError, "rotate-failed",
			"额度耗尽但无可用账号",
			"当前账号 quota 用尽，自动切换失败: "+err.Error())
		return
	}
	utils.DLog("[回调] onKeyExhausted: MITM轮换成功 → %s", next.Email)
}
