//go:build !windows && !darwin

package main

// startTray 在 Linux / 其它平台禁用：依赖 dbus + libappindicator，
// 发布构建复杂，且当前用户预期主要是 Windows + macOS。
func (a *App) startTray() {}

func traySupported() bool { return false }
