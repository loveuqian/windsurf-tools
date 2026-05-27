package main

import (
	"strings"
	"time"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// UpstreamProxyStatus 给前端展示当前上游代理走哪条出口。
// 是排障神器：用户能直接看到现在走的是 clash / 系统代理 / 直连，
// 不用再翻日志去猜「为啥我开了 clash 但请求还在被风控」。
type UpstreamProxyStatus struct {
	// Source 取自 services.ProxySource，可能值:
	//   "direct" / "clash+nodes" / "clash" / "system" / "unknown"
	Source string `json:"source"`
	// URL 已 redact 掉 userinfo（http://user:pass@host:port → http://***@host:port）
	// 空字符串 = 直连。
	URL string `json:"url"`
	// LastAppliedAt 是上次成功下发到 MITM / Relay 的时间（RFC3339）。
	// 空 = 还没触发过 applyUpstreamProxy。
	LastAppliedAt string `json:"last_applied_at"`
}

// applyUpstreamProxy 按优先级解析上游代理并推到 MITM / OpenAI Relay。
//
// 优先级（高 → 低）：
//  1. clash + 节点轮换：ClashRotateEnabled=true 且 Clash controller 探到入口端口
//  2. clash：仅 ClashControllerURL 非空且 controller 通
//  3. 系统代理：HTTPS_PROXY/HTTP_PROXY 或 Windows 注册表
//  4. 直连
//
// 解析过程会发起 HTTP 探活（最多 3s 超时），异步执行避免阻塞 UpdateSettings。
// 在 initBackend、UpdateSettings、AutoSetupClash 之后调用。
//
// 串行化 + epoch 校验：防止用户连点保存或多入口并发触发时，
//   - 多个 goroutine 同时跑 ResolveUpstreamProxy（重复 3s 探活）
//   - 旧 stale 探活结果（用旧 settings 算出的）覆盖新一轮的下发
//
// epoch 每次入口 +1，goroutine 拿锁后比对 epoch，不是最新就丢弃。
func (a *App) applyUpstreamProxy() {
	if a.store == nil {
		return
	}
	s := a.store.GetSettings()
	controllerURL := strings.TrimSpace(s.ClashControllerURL)
	secret := strings.TrimSpace(s.ClashSecret)
	clashRotateEnabled := s.ClashRotateEnabled

	// 抢一个 epoch 序号；后面 goroutine 用它判断自己是不是已经被新一轮覆盖
	epoch := a.applyProxyEpoch.Add(1)

	go func() {
		// 串行化：保证最多一个 ResolveUpstreamProxy 在跑（探活成本高、3s 超时）
		a.applyProxyMu.Lock()
		defer a.applyProxyMu.Unlock()

		// 拿到锁的瞬间复查：在排队期间被新一轮覆盖了就跳过
		if latest := a.applyProxyEpoch.Load(); latest != epoch {
			utils.DLog("[Proxy] 跳过 stale apply (epoch=%d, latest=%d)", epoch, latest)
			return
		}

		proxyURL, source := services.ResolveUpstreamProxy(controllerURL, secret, clashRotateEnabled)

		// 探活 3s 期间又被新一轮覆盖也丢弃，不要把过时结果下发
		if latest := a.applyProxyEpoch.Load(); latest != epoch {
			utils.DLog("[Proxy] 探活后再次 stale，丢弃结果 (epoch=%d, latest=%d, source=%s)", epoch, latest, source)
			return
		}

		redacted := redactProxyURL(proxyURL)
		utils.DLog("[Proxy] 上游代理选择: source=%s url=%s", source, redacted)
		if a.mitmProxy != nil {
			a.mitmProxy.SetUpstreamProxy(proxyURL)
		}
		if a.openaiRelay != nil {
			a.openaiRelay.SetUpstreamProxy(proxyURL)
		}
		// 写入最近一次状态供 GetUpstreamProxyStatus / Dashboard 角标查询
		a.lastProxyStatus.Store(&UpstreamProxyStatus{
			Source:        string(source),
			URL:           redacted,
			LastAppliedAt: time.Now().Format(time.RFC3339),
		})
	}()
}

// GetUpstreamProxyStatus 返回最近一次 applyUpstreamProxy 的下发结果。
// 前端 Dashboard 用它展示当前上游走哪条出口（clash / 系统 / 直连）。
// 未触发过应用时返回 Source="unknown"。
func (a *App) GetUpstreamProxyStatus() UpstreamProxyStatus {
	if p := a.lastProxyStatus.Load(); p != nil {
		return *p
	}
	return UpstreamProxyStatus{Source: "unknown"}
}

// redactProxyURL 隐藏代理 URL 里的 userinfo（http://user:pass@host:port）。
func redactProxyURL(s string) string {
	if s == "" {
		return "<direct>"
	}
	if i := strings.Index(s, "@"); i >= 0 {
		if j := strings.Index(s, "://"); j >= 0 && j < i {
			return s[:j+3] + "***@" + s[i+1:]
		}
	}
	return s
}
