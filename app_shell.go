package main

// app_shell.go ── 薄壳。真正实现已迁到 backend/app/desktop。
//   - 旧调用入口 openPathWithSystem / revealPathInFileManager / pickLinuxOpener
//     保留为薄包装，签名不变。

import "windsurf-tools-wails/backend/app/desktop"

func openPathWithSystem(path string) error      { return desktop.OpenPath(path) }
func revealPathInFileManager(path string) error { return desktop.RevealPath(path) }
func pickLinuxOpener() string                   { return desktop.PickLinuxOpener() }
