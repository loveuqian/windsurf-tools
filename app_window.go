package main

import (
	"context"

	"windsurf-tools-wails/backend/models"

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

// ── 2.4: 窗口尺寸 / 位置记忆 ────────────────────────────────────────────────

// SaveWindowGeometry 把当前窗口尺寸 + 位置 + 最大化状态写入 settings。
// 由前端 resize/move 防抖 1.5s 触发，仅持久化非最大化的「正常状态尺寸」，
// 这样下次启动用户能恢复到自己拖拽好的窗口大小。
//
// 入参合法性：宽高 >= 600/400 才写盘（防止极端值把窗口卡到看不见）；
// X/Y 任意，但完全不可见的位置（< -1000）忽略。
func (a *App) SaveWindowGeometry(width, height, x, y int, maximized bool) error {
	if a.store == nil {
		return nil
	}
	// 钳制极端值;非最大化时窗口过小直接忽略。
	if !maximized && (width < 600 || height < 400) {
		return nil
	}
	return a.store.MutateSettings(func(s *models.Settings) {
		if !maximized {
			s.WindowWidth = width
			s.WindowHeight = height
			if x > -10000 && y > -10000 {
				s.WindowX = x
				s.WindowY = y
			}
		}
		s.WindowMaximized = maximized
	})
}

// RestoreWindowGeometry 启动时由前端调用，从 settings 还原窗口几何。
// 因 Wails options.Width/Height 不能动态读取 store，只能在 DOM 就绪后通过
// runtime.WindowSetSize / WindowSetPosition / WindowMaximise 调整。
//
// 返回应用的几何（前端可用作 sanity check）；首次启动 / 无保存值 → 返回 (0,0,-1,-1,false)。
func (a *App) RestoreWindowGeometry() map[string]any {
	if a.store == nil || a.ctx == nil {
		return map[string]any{"applied": false}
	}
	s := a.store.GetSettings()
	if s.WindowMaximized {
		runtime.WindowMaximise(a.ctx)
		return map[string]any{
			"applied":   true,
			"maximized": true,
		}
	}
	w, h := s.WindowWidth, s.WindowHeight
	x, y := s.WindowX, s.WindowY
	applied := false
	if w >= 600 && h >= 400 {
		runtime.WindowSetSize(a.ctx, w, h)
		applied = true
	}
	if x > -10000 && y > -10000 && (x != -1 || y != -1) {
		runtime.WindowSetPosition(a.ctx, x, y)
		applied = true
	}
	return map[string]any{
		"applied":   applied,
		"width":     w,
		"height":    h,
		"x":         x,
		"y":         y,
		"maximized": false,
	}
}
