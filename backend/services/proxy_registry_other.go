//go:build !windows

package services

// AddProxyOverride 非 Windows 无需注册表操作
func AddProxyOverride() error { return nil }

// RemoveProxyOverride 非 Windows 无需清理
func RemoveProxyOverride() error { return nil }

// HasProxyOverride 非 Windows 上 ProxyOverride 概念不存在，固定返回 false。
func HasProxyOverride() bool { return false }
