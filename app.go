package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/store"
	"windsurf-tools-wails/backend/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx                    context.Context
	store                  *store.Store
	windsurfSvc            *services.WindsurfService
	mitmProxy              *services.MitmProxy
	openaiRelay            *services.OpenAIRelay
	clashRotator           *services.ClashRotator
	rotationPool           *rotationPoolState
	usageTracker           *services.UsageTracker
	cancelAutoRefresh      context.CancelFunc
	cancelAutoQuotaRefresh context.CancelFunc
	cancelQuotaHotPoll     context.CancelFunc
	lastQuotaHotSwitch     time.Time
	lastQuotaHotSwitchMu   sync.Mutex
	tokenRefreshRunMu      sync.Mutex
	quotaRefreshRunMu      sync.Mutex
	mu                     sync.Mutex
	cleanupMitmOnExitFn    func() error
	activateExistingAppFn  func()
	traySupportedFn        func() bool
	// silentFromFlag 由 main 在解析到 --silent 时设置，与 settings.silent_start 二选一即可触发静默启动
	silentFromFlag bool
	// applyProxyMu / applyProxyEpoch 串行化 applyUpstreamProxy：
	// 防止 settings 连点保存 / 多个入口同时触发时，多个 goroutine 并发探活后旧 stale 结果覆盖新结果。
	// epoch 每次 +1，goroutine 拿锁后检查 epoch 不是最新则丢弃。
	applyProxyMu    sync.Mutex
	applyProxyEpoch atomic.Uint64
	// lastProxyStatus 保存最近一次 applyUpstreamProxy 的下发结果，
	// 供前端 Dashboard 角标 / GetUpstreamProxyStatus 查询（排障神器）。
	lastProxyStatus atomic.Pointer[UpstreamProxyStatus]
	// shutdownOnce 防 wails 在 panic / second-instance / 快速退出路径上重入 OnShutdown，
	// 避免 cleanupMitmEnvironment（写 hosts / 卸 CA）重入报错。
	shutdownOnce sync.Once
}

func NewApp() *App { return &App{} }

// SetSilentFromFlag 由 main 在 wails.Run 前设置（--silent / --silent-start）。
func (a *App) SetSilentFromFlag(v bool) { a.silentFromFlag = v }

func (a *App) initBackend() error {
	s, err := store.NewStore()
	if err != nil {
		return fmt.Errorf("存储初始化失败: %w", err)
	}
	a.store = s
	a.windsurfSvc = services.NewWindsurfService("")
	// ── 调试日志 ──
	settings := a.store.GetSettings()
	utils.InitDebugLogger(s.DataDir(), settings.DebugLog)
	// ── 创建跨服务的用量跟踪器 ──
	a.usageTracker = services.NewUsageTracker(s.DataDir())

	a.mitmProxy = services.NewMitmProxy(a.windsurfSvc, func(msg string) {
		utils.DLog("%s", msg)
	}, "", a.usageTracker)
	a.mitmProxy.SetOnKeyExhausted(func(apiKey string) {
		utils.DLog("[回调] onKeyExhausted 触发: key=%s...", apiKey[:min(12, len(apiKey))])
		accID := findAccountIDForMITMAPIKey(a.store.GetAllAccounts(), apiKey)
		if accID == "" {
			utils.DLog("[回调] onKeyExhausted: 在号池中未找到匹配 key，跳过")
			return
		}
		utils.DLog("[回调] onKeyExhausted: 匹配到账号 id=%s，刷新额度...", accID[:min(8, len(accID))])
		_ = a.RefreshAccountQuota(accID)
		// ★ 立即触发切号（之前只刷额度不切号，导致 IDE 继续用耗尽账号）
		s := a.store.GetSettings()
		// Pin 优先：手动锁定时跳过所有自动切（用户 100% 控制）
		if s.ManualPinEnabled {
			utils.DLog("[回调] onKeyExhausted: ManualPin 生效 (pin=%s)，跳过自动切", s.ManualPinAccountID[:min(8, len(s.ManualPinAccountID))])
			return
		}
		if s.AutoSwitchOnQuotaExhausted {
			utils.DLog("[回调] onKeyExhausted: AutoSwitch=true → 立即MITM轮换")
			if next, err := a.rotateMitmToNextAvailable(accID, s.AutoSwitchPlanFilter); err != nil {
				utils.DLog("[回调] onKeyExhausted: MITM轮换失败: %v", err)
				// 关键事件：额度耗尽 + 没切到下一个 → 用户需要立刻知道
				a.notify(NotifyKindError, "rotate-failed",
					"额度耗尽但无可用账号",
					"当前账号 quota 用尽，自动切换失败: "+err.Error())
			} else {
				utils.DLog("[回调] onKeyExhausted: MITM轮换成功 → %s", next.Email)
			}
		} else {
			utils.DLog("[回调] onKeyExhausted: AutoSwitchOnQuotaExhausted=false，不切号")
		}
	})
	a.mitmProxy.SetOnKeyAccessDenied(func(apiKey, detail string) {
		utils.DLog("[回调] onKeyAccessDenied 触发: key=%s...", apiKey[:min(12, len(apiKey))])
		a.handleMitmKeyAccessDenied(apiKey, detail)
	})
	a.mitmProxy.SetOnCurrentKeyChanged(func(apiKey, reason string) {
		utils.DLog("[回调] onCurrentKeyChanged 触发: key=%s... reason=%s", apiKey[:min(12, len(apiKey))], reason)
		a.handleMitmCurrentKeyChanged(apiKey, reason)
	})
	a.openaiRelay = services.NewOpenAIRelay(a.mitmProxy, func(msg string) {
		utils.DLog("%s", msg)
	}, "", a.usageTracker)
	a.openaiRelay.SetOnSuccess(func(apiKey string) {
		accounts := a.store.GetAllAccounts()
		accID := findAccountIDForMITMAPIKey(accounts, apiKey)
		if accID == "" {
			return
		}
		_ = a.RefreshAccountQuota(accID)
	})
	a.syncMitmPoolKeys()
	a.syncForgeConfig()
	a.syncStaticCacheConfig()
	a.syncJailbreakConfig()
	if settings.AutoRefreshTokens {
		a.startAutoRefresh()
	}
	if settings.AutoRefreshQuotas {
		a.startAutoQuotaRefresh()
	}
	a.restartQuotaHotPollIfNeeded()
	a.applyClashRotatorSettings()
	a.applyRotationPoolSettings()
	a.applyUpstreamProxy()
	return nil
}

func (a *App) shouldStartHidden() bool {
	if a.store == nil {
		return a.silentFromFlag && a.supportsTray()
	}
	settings := a.store.GetSettings()
	if !a.supportsTray() {
		return false
	}
	return a.silentFromFlag || settings.SilentStart
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.initBackend(); err != nil {
		log.Printf("[WindsurfTools] desktop init failed: %v", err)
		// 交互模式弹原生错误对话框；silent / 服务化模式只记日志。
		// 用 runtime.Quit 而非 log.Fatalf，让 OnShutdown 跑完（托盘 / hosts / CA / 443 listener 能被清理）。
		if !a.silentFromFlag {
			_, _ = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
				Type:    runtime.ErrorDialog,
				Title:   "Windsurf Tools 初始化失败",
				Message: err.Error(),
			})
		}
		runtime.Quit(a.ctx)
		return
	}
	log.Printf("[WindsurfTools] desktop backend initialized")
	if a.supportsTray() {
		a.startTray()
		log.Printf("[WindsurfTools] tray initialized")
	} else {
		log.Printf("[WindsurfTools] tray unsupported on current platform")
	}
	if a.shouldStartHidden() {
		log.Printf("[WindsurfTools] desktop start hidden")
		go func() {
			time.Sleep(280 * time.Millisecond)
			runtime.WindowHide(a.ctx)
		}()
	} else {
		log.Printf("[WindsurfTools] desktop main window visible")
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.shutdownOnce.Do(func() {
		log.Printf("[WindsurfTools] desktop shutdown requested")
		if a.cancelAutoRefresh != nil {
			a.cancelAutoRefresh()
		}
		if a.cancelAutoQuotaRefresh != nil {
			a.cancelAutoQuotaRefresh()
		}
		a.stopQuotaHotPoll()
		if a.openaiRelay != nil {
			a.openaiRelay.Stop()
		}
		// 显式停 MITM listener，跟 openaiRelay 对称（之前只有当
		// shouldCleanupMitmEnvironment 触发时才会经 TeardownMitm 间接 Stop，
		// 未启动 / 无 hosts / 无 CA 三条都不满足时 listener 可能漏关）。
		// Stop 是幂等的：未运行直接 return nil，已被 cleanup 调过也 OK。
		if a.mitmProxy != nil {
			_ = a.mitmProxy.Stop()
		}
		a.stopClashRotator()
		a.stopRotationPool()
		// cleanupMitmEnvironment 必须同步等到完成 —— 写 hosts / 卸 CA / macOS
		// osascript sudo 弹窗都要走完，绝不超时强退。残留 hosts / CA 会污染用户
		// 系统流量（IDE 之外的请求都被劫持到 127.0.0.1:443），比 wails 多卡几秒
		// 严重得多。如果用户彻底不在键盘前，他必然会强杀进程，那是用户主动选择
		// 的结果，不应该由代码替他决定放弃清理。
		log.Printf("[WindsurfTools] 正在清理 MITM 环境（可能弹密码框，请耐心等待）...")
		a.cleanupMitmEnvironment()
		log.Printf("[WindsurfTools] MITM 环境清理完成")
	})
}
