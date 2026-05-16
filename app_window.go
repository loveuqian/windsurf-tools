package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// onBeforeClose 关闭窗口时：
//   - 已进入 shuttingDown（托盘菜单「退出并恢复环境」/ runtime.Quit）→ 让出，
//     直接走 OnShutdown，不再隐藏到托盘；
//   - MinimizeToTray=true 且托盘可用 → 隐藏窗口到托盘（保持后台运行）；
//   - 否则 → 让窗口立刻消失给用户即时反馈，cleanup 留给 OnShutdown 异步跑。
func (a *App) onBeforeClose(ctx context.Context) bool {
	// 主动退出流程优先：runtime.Quit 必然要让进程关掉，禁止再隐藏到托盘。
	if a.isShuttingDown() {
		return false
	}
	if a.store == nil {
		return false // 允许关闭
	}
	if a.store.GetSettings().MinimizeToTray && a.supportsTray() {
		runtime.WindowHide(ctx)
		return true // 阻止关闭，隐藏到托盘
	}
	// 用户在没有托盘的环境直接关窗：不再同步阻塞跑 cleanup（osascript 弹密码
	// 会让窗口卡顿数秒甚至超时失败），改为「先标记进入退出流程 → 允许 wails
	// 关窗 → OnShutdown 里再异步执行 cleanup 并通过桌面通知反馈」。
	a.shuttingDown.Store(true)
	return false // 允许关闭
}
