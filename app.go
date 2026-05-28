package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
	"windsurf-tools-wails/backend/app/clash"
	"windsurf-tools-wails/backend/app/importsvc"
	"windsurf-tools-wails/backend/app/notify"
	"windsurf-tools-wails/backend/app/pin"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/store"
	"windsurf-tools-wails/backend/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx                    context.Context
	store                  *store.Store
	providerStore          *store.ProviderAccountStore
	transportPool          *services.TransportPool
	windsurfSvc            *services.WindsurfService
	mitmProxy              *services.MitmProxy
	openaiRelay            *services.OpenAIRelay
	rotationPool           *rotationPoolState
	usageTracker           *services.UsageTracker
	notifier               *notify.Module
	pinMod                 *pin.Module
	clashMod               *clash.Module
	importMod              *importsvc.Module
	cancelAutoRefresh      context.CancelFunc
	cancelAutoQuotaRefresh context.CancelFunc
	cancelQuotaHotPoll     context.CancelFunc
	lastQuotaHotSwitch     time.Time
	lastQuotaHotSwitchMu   sync.Mutex
	// switchMu 串行化所有 app 级自动切号入口(onKeyExhausted / hot-poll /
	// refreshDueQuotas / rotation-pool),配合 lastAutoSwitchAt 去重:同一次
	// 耗尽事件经多条路径触发时,只切一次,避免账号在 1~2s 内连跳 2~3 个号。
	switchMu              sync.Mutex
	lastAutoSwitchAt      time.Time
	tokenRefreshRunMu     sync.Mutex
	quotaRefreshRunMu     sync.Mutex
	tasks                 *TaskRegistry       // F1: 批量任务进度面板
	switchHistory         *switchHistoryStore // F2: 切号历史持久化
	mu                    sync.Mutex
	cleanupMitmOnExitFn   func() error
	activateExistingAppFn func()
	traySupportedFn       func() bool
	// silentFromFlag 由 main 在解析到 --silent 时设置，与 settings.silent_start 二选一即可触发静默启动
	silentFromFlag bool
	// shuttingDown 在主动退出流程开始时被原子置 true。onBeforeClose 看到此标志
	// 后跳过「隐藏到托盘」逻辑，确保托盘菜单「退出」/ runtime.Quit 真的能让进程关掉。
	shuttingDown atomic.Bool
	// cleanupOnce 保证整个进程生命周期里 TeardownMitm 只跑一次，
	// 即便 onBeforeClose 与 OnShutdown 都触发也只是单次清理。
	cleanupOnce sync.Once
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
	ps, err := store.NewProviderAccountStore(s.DataDir())
	if err != nil {
		return fmt.Errorf("提供商账号存储初始化失败: %w", err)
	}
	a.providerStore = ps
	// ── 全局出站 transport 池 ──
	a.transportPool = services.NewTransportPool(func() services.ProxyConfig {
		s := a.store.GetSettings()
		return services.ProxyConfig{
			ProxyURL:           s.ProxyURL,
			ClashControllerURL: s.ClashControllerURL,
			ClashSecret:        s.ClashSecret,
			ClashRotateEnabled: s.ClashRotateEnabled,
		}
	})
	a.windsurfSvc = services.NewWindsurfService("")
	a.tasks = NewTaskRegistry()
	a.switchHistory = newSwitchHistoryStore(s.DataDir())
	// ── 调试日志 ──
	settings := a.store.GetSettings()
	utils.InitDebugLogger(s.DataDir(), settings.DebugLog)
	// ── 桌面通知 ──
	a.notifier = notify.New(func() bool {
		if a.store == nil {
			return false
		}
		return a.store.GetSettings().DesktopNotifications
	})
	// ── 手动 Pin ──
	a.pinMod = pin.New(a.store, func(eventKey, title, body string) {
		a.notify(NotifyKindInfo, eventKey, title, body)
	})
	// ── 创建跨服务的用量跟踪器 ──
	a.usageTracker = services.NewUsageTracker(s.DataDir())

	// ── MITM Proxy & OpenAI Relay（回调钩子统一在 wireProxyCallbacks 注册）──
	a.mitmProxy = services.NewMitmProxy(a.windsurfSvc, func(msg string) {
		utils.DLog("%s", msg)
	}, a.transportPool.RawProxyURL(), a.usageTracker)
	a.openaiRelay = services.NewOpenAIRelay(a.mitmProxy, func(msg string) {
		utils.DLog("%s", msg)
	}, a.transportPool.RawProxyURL(), a.usageTracker)
	a.wireProxyCallbacks()
	// 阶段 2 提供商路由注入: 总览胶囊点亮"提供商" + chat path 时,
	// MITM 走 cascade↔OpenAI/Anthropic/Gemini 翻译流水, 而非号池。
	a.mitmProxy.SetRouter(a)
	a.mitmProxy.SetTransportPool(a.transportPool)
	// ── 导入流水线 ──（必须在 mitmProxy 创建后，因为 syncMitmPoolKeys 依赖它）
	// enrichAccountInfo 返回 bool，子包不关心结果只用副作用，这里用 closure 包裹丢弃返回值。
	a.importMod = importsvc.New(importsvc.Deps{
		Store:        a.store,
		WindsurfSvc:  a.windsurfSvc,
		EnrichFull:   func(acc *models.Account) { _ = a.enrichAccountInfo(acc) },
		EnrichLite:   a.enrichAccountInfoLite,
		SyncMitmPool: a.syncMitmPoolKeys,
	})
	// ── Clash IP 轮换 ──
	a.clashMod = clash.New(a.store, a.mitmProxy)
	a.syncMitmPoolKeys()
	a.syncForgeConfig()
	a.syncStaticCacheConfig()
	a.syncJailbreakConfig()
	// ★ 启动时若 settings 显示已 Pin → 把 stickyKey 推给 MITM，避免重启后新对话又被分散
	a.syncMitmStickyFromPin()
	a.syncMitmAutoSwitchOnQuotaExhausted()
	// F7-REMOVAL: 下一行删除
	a.syncSmartFriendConfig()
	if settings.AutoRefreshTokens {
		a.startAutoRefresh()
	}
	if settings.AutoRefreshQuotas {
		a.startAutoQuotaRefresh()
	}
	a.restartQuotaHotPollIfNeeded()
	a.applyOpenAIRelaySettings()
	// ★ 在启动 rotator 之前,先把上次未正常退出残留的 Clash 节点切回原节点
	//   (kill -9/崩溃后的兜底,详见 clash_rotator_state.go)。
	a.clashMod.Recover()
	a.applyClashRotatorSettings()
	a.applyRotationPoolSettings()
	return nil
}

func (a *App) shouldStartHidden() bool {
	// 命令行 --silent 是显式意图(开机自启后台跑),无条件生效——即使无托盘。
	// 无托盘平台(macOS/Linux)虽无图标可点,但单实例锁的 onSecondInstanceLaunch
	// → activateExistingWindow(WindowShow/Unminimise,跨平台 Wails runtime)
	// 能在再次启动 app 时唤出窗口,所以不会把用户彻底锁死。
	if a.silentFromFlag {
		return true
	}
	if a.store == nil {
		return false
	}
	// settings.SilentStart 是 UI 隐式开关:仅在有托盘(可点图标唤出)时生效,
	// 避免无托盘平台用户开了开关后每次开机窗口都不见、又不知道再启一次能唤出。
	if !a.supportsTray() {
		return false
	}
	return a.store.GetSettings().SilentStart
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
	log.Printf("[WindsurfTools] desktop shutdown requested")
	a.mu.Lock()
	cancelRefresh := a.cancelAutoRefresh
	cancelQuota := a.cancelAutoQuotaRefresh
	a.cancelAutoRefresh = nil
	a.cancelAutoQuotaRefresh = nil
	a.mu.Unlock()
	if cancelRefresh != nil {
		cancelRefresh()
	}
	if cancelQuota != nil {
		cancelQuota()
	}
	a.stopQuotaHotPoll()
	if a.openaiRelay != nil {
		a.openaiRelay.Stop()
	}
	a.stopClashRotator()
	a.stopRotationPool()
	a.cleanupMitmEnvironment()
}
