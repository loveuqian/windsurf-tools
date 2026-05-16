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
	tokenRefreshRunMu      sync.Mutex
	quotaRefreshRunMu      sync.Mutex
	mu                     sync.Mutex
	cleanupMitmOnExitFn    func() error
	activateExistingAppFn  func()
	traySupportedFn        func() bool
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
	a.windsurfSvc = services.NewWindsurfService("")
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
	}, "", a.usageTracker)
	a.openaiRelay = services.NewOpenAIRelay(a.mitmProxy, func(msg string) {
		utils.DLog("%s", msg)
	}, "", a.usageTracker)
	a.wireProxyCallbacks()
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
	// F7-REMOVAL: 下一行删除
	a.syncSmartFriendConfig()
	if settings.AutoRefreshTokens {
		a.startAutoRefresh()
	}
	if settings.AutoRefreshQuotas {
		a.startAutoQuotaRefresh()
	}
	a.restartQuotaHotPollIfNeeded()
	a.applyClashRotatorSettings()
	a.applyRotationPoolSettings()
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
		log.Printf("[WindsurfTools] desktop init: %v", err)
		log.Fatalf("%v", err)
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
	a.stopClashRotator()
	a.stopRotationPool()
	a.cleanupMitmEnvironment()
}
