package main

import (
	"context"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"

	"windsurf-tools-wails/backend/store"
)

func TestShutdownCleansMitmEnvironment(t *testing.T) {
	app := NewApp()
	called := 0
	app.cleanupMitmOnExitFn = func() error {
		called++
		return nil
	}

	app.shutdown(context.Background())

	if called != 1 {
		t.Fatalf("shutdown() cleanup calls = %d, want 1", called)
	}
}

func TestActivateExistingWindowCallsHook(t *testing.T) {
	app := NewApp()

	called := make(chan struct{}, 1)
	app.activateExistingAppFn = func() {
		called <- struct{}{}
	}

	app.onSecondInstanceLaunch(options.SecondInstanceData{})

	select {
	case <-called:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("onSecondInstanceLaunch() did not trigger activation hook")
	}
}

func TestShouldStartHiddenSilentFlagWorksWithoutTray(t *testing.T) {
	// 命令行 --silent 是显式后台意图,应无条件隐藏——即使无托盘。
	// 无托盘平台仍可通过单实例锁(再次启动 → onSecondInstanceLaunch)唤出窗口。
	app := NewApp()
	app.silentFromFlag = true
	app.traySupportedFn = func() bool { return false }

	if !app.shouldStartHidden() {
		t.Fatal("shouldStartHidden() should honor --silent flag even when tray is unavailable")
	}
}

func TestShouldStartHiddenIgnoresSettingToggleWithoutTray(t *testing.T) {
	// settings.SilentStart(UI 隐式开关)在无托盘平台不生效,避免用户锁死自己。
	s, err := store.NewStoreInPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreInPaths() error = %v", err)
	}
	settings := s.GetSettings()
	settings.SilentStart = true
	if err := s.UpdateSettings(settings); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	app := NewApp()
	app.store = s
	app.silentFromFlag = false
	app.traySupportedFn = func() bool { return false }

	if app.shouldStartHidden() {
		t.Fatal("shouldStartHidden() should ignore SilentStart setting when tray is unavailable")
	}
}

func TestOnBeforeCloseIgnoresMinimizeToTrayWhenTrayUnavailable(t *testing.T) {
	s, err := store.NewStoreInPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreInPaths() error = %v", err)
	}
	settings := s.GetSettings()
	settings.MinimizeToTray = true
	if err := s.UpdateSettings(settings); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	app := NewApp()
	app.store = s
	app.traySupportedFn = func() bool { return false }

	if app.onBeforeClose(context.Background()) {
		t.Fatal("onBeforeClose() should not hide window when tray is unavailable")
	}
}
