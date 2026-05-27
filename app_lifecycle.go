package main

import (
	"log"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"windsurf-tools-wails/backend/services"
)

func (a *App) cleanupMitmEnvironment() {
	if !a.shouldCleanupMitmEnvironment() {
		return
	}
	cleanup := a.cleanupMitmOnExitFn
	if cleanup == nil {
		cleanup = a.TeardownMitm
	}
	if err := cleanup(); err != nil {
		log.Printf("[WindsurfTools] MITM cleanup: %v", err)
	}
}

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
	return services.IsCAInstalled()
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
	// Windows 11 防偷焦：其他窗口在前台时 WindowShow 只闪任务栏不抢焦。
	// 短暂置顶 → 让 WM 把窗口放最上层 → 再取消置顶恢复正常 Z-order。
	// 120ms 是经验值：足够 WM 处理 paint，又不会让用户感觉「窗口卡在最上」。
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	go func() {
		time.Sleep(120 * time.Millisecond)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
	}()
}

func (a *App) onSecondInstanceLaunch(options.SecondInstanceData) {
	go a.activateExistingWindow()
}
