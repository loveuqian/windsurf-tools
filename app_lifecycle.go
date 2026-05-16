package main

import (
	"log"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"windsurf-tools-wails/backend/services"
)

// cleanupMitmEnvironment 走一次性 cleanup（sync.Once 保证）。
// 失败时通过桌面通知告诉用户「hosts/CA 没还原成功」，避免静默残留。
//
// onBeforeClose 与 OnShutdown 都会调用此函数；第二次进来 sync.Once
// 直接 no-op，不重复弹密码框。
func (a *App) cleanupMitmEnvironment() {
	a.cleanupOnce.Do(func() {
		if !a.shouldCleanupMitmEnvironment() {
			return
		}
		cleanup := a.cleanupMitmOnExitFn
		if cleanup == nil {
			cleanup = a.TeardownMitm
		}
		if err := cleanup(); err != nil {
			log.Printf("[WindsurfTools] MITM cleanup: %v", err)
			a.notify(NotifyKindError, "cleanup-failed",
				"MITM 环境未完全恢复",
				"关闭时未能完整还原 hosts/CA/Codeium 配置: "+err.Error()+
					"\n请手动运行：管理员身份 → 「卸载 MITM」按钮 / 编辑 hosts。")
		}
	})
}

// requestExit 由托盘菜单「退出并恢复环境」调用：先标记关闭流程，再让
// runtime.Quit 走 wails 的 OnBeforeClose → OnShutdown，期间 sync.Once
// 保证 cleanup 只执行一次。
//
// 标记 shuttingDown=true 后，onBeforeClose 不再走「隐藏到托盘」分支，
// 避免 runtime.Quit 被误判为关窗 → 隐藏 → 进程不退出。
func (a *App) requestExit() {
	if a == nil {
		return
	}
	a.shuttingDown.Store(true)
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// isShuttingDown 由 onBeforeClose / shutdown 等读取关闭流程标志位。
func (a *App) isShuttingDown() bool {
	if a == nil {
		return false
	}
	return a.shuttingDown.Load()
}

// shouldCleanupMitmEnvironment 判断退出时是否需要跑一遍 TeardownMitm。
// 任何「启动改、关闭恢复」的痕迹都要触发清理：MITM 进程在跑、hosts 标记
// 还在、CA 在系统钥匙串、Windows ProxyOverride 名单含我们的域名、~/.codeium
// 还有备份文件都视作未清理状态。
func (a *App) shouldCleanupMitmEnvironment() bool {
	if a.cleanupMitmOnExitFn != nil {
		return true
	}
	if a.mitmProxy != nil && a.mitmProxy.Status().Running {
		return true
	}
	if services.IsHostsMapped(services.TargetDomain) {
		return true
	}
	if services.IsCAInstalled() {
		return true
	}
	if services.HasProxyOverride() {
		return true
	}
	return services.HasCodeiumConfigBackup()
}

func (a *App) activateExistingWindow() {
	if a.activateExistingAppFn != nil {
		a.activateExistingAppFn()
		return
	}
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
}

func (a *App) onSecondInstanceLaunch(options.SecondInstanceData) {
	go a.activateExistingWindow()
}
