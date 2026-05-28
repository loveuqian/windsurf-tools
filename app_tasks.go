package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// F1: 批量任务进度跟踪（最小可行版）
//
// 后端 in-memory TaskRegistry —— 给「全量刷新 token / 全量同步额度」这两类
// 长耗时批量操作提供细粒度进度可视化，避免用户只能呆看一个 spinner。
//
// 设计原则：
//   - in-memory only（重启即失，对调试任务足够）
//   - 进程级单例，自动清理 30 分钟前已完成的任务
//   - 最多保留 50 条历史，避免长会话内存膨胀
//   - 线程安全，被 RefreshAllTokens / RefreshAllQuotas 的并发 worker 调用
//   - JSON tag 与前端 zustand store 一一对应
//
// 不做的：
//   - 不持久化（短任务无意义）
//   - 不做服务端推送（前端 1s 轮询已够）
//   - 不做断点续传 / 取消（语义复杂，先观测再说）

const (
	maxTaskHistory   = 50
	taskRetentionTTL = 30 * time.Minute
)

// TaskItemStatus 单条 item 的最终状态
type TaskItemStatus string

const (
	TaskItemPending TaskItemStatus = "pending"
	TaskItemOK      TaskItemStatus = "ok"
	TaskItemFailed  TaskItemStatus = "failed"
)

// TaskItem 任务里的一条子项（如「user@example.com 刷新成功」）
type TaskItem struct {
	Name   string         `json:"name"`
	Status TaskItemStatus `json:"status"`
	Detail string         `json:"detail"`
}

// Task 一次批量操作的全貌
type Task struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`  // refresh_tokens / refresh_quotas / import / cleanup
	Title      string     `json:"title"` // 显示用，如「全量刷新 Token (45)」
	Total      int        `json:"total"`
	Completed  int        `json:"completed"`
	Succeeded  int        `json:"succeeded"`
	Failed     int        `json:"failed"`
	Items      []TaskItem `json:"items"`
	StartedAt  string     `json:"started_at"`
	FinishedAt string     `json:"finished_at,omitempty"`
	Running    bool       `json:"running"`
}

// TaskRegistry 进程级 in-mem task 跟踪器
type TaskRegistry struct {
	mu     sync.Mutex
	tasks  []*Task
	nextID atomic.Int64
}

// NewTaskRegistry 新建一个空注册器
func NewTaskRegistry() *TaskRegistry {
	return &TaskRegistry{}
}

// Start 注册新任务，返回 taskID。total 为预估总条数。
func (r *TaskRegistry) Start(kind, title string, total int) string {
	id := r.nextID.Add(1)
	taskID := taskIDFromInt(id)
	t := &Task{
		ID:        taskID,
		Kind:      kind,
		Title:     title,
		Total:     total,
		Items:     make([]TaskItem, 0, total),
		StartedAt: time.Now().Format(time.RFC3339),
		Running:   true,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks = append(r.tasks, t)
	r.gcLocked()
	return taskID
}

// Add 给任务追加一条 item 结果，自动累计 Completed/Succeeded/Failed
func (r *TaskRegistry) Add(taskID, name string, ok bool, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.findLocked(taskID)
	if t == nil {
		return
	}
	status := TaskItemOK
	if !ok {
		status = TaskItemFailed
	}
	t.Items = append(t.Items, TaskItem{
		Name:   name,
		Status: status,
		Detail: detail,
	})
	t.Completed++
	if ok {
		t.Succeeded++
	} else {
		t.Failed++
	}
	if t.Completed >= t.Total {
		t.Running = false
		t.FinishedAt = time.Now().Format(time.RFC3339)
	}
}

// Finish 强制结束任务（如某些 task total 是估算或外部提前完成）
func (r *TaskRegistry) Finish(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.findLocked(taskID)
	if t == nil {
		return
	}
	t.Running = false
	if t.FinishedAt == "" {
		t.FinishedAt = time.Now().Format(time.RFC3339)
	}
	if t.Completed < t.Total {
		t.Total = t.Completed
	}
}

// Snapshot 返回当前所有任务的快照（深拷贝，前端拿到后无锁可读）
func (r *TaskRegistry) Snapshot() []Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gcLocked()
	out := make([]Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		// items 单独拷贝，避免 slice 共享
		items := make([]TaskItem, len(t.Items))
		copy(items, t.Items)
		copy := *t
		copy.Items = items
		out = append(out, copy)
	}
	return out
}

// ClearFinished 清掉所有 Running=false 的历史任务
func (r *TaskRegistry) ClearFinished() {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.tasks[:0]
	for _, t := range r.tasks {
		if t.Running {
			kept = append(kept, t)
		}
	}
	r.tasks = kept
}

// gcLocked 调用方需持有 mu。清掉超龄完成任务 + 超额 history
func (r *TaskRegistry) gcLocked() {
	if len(r.tasks) == 0 {
		return
	}
	cutoff := time.Now().Add(-taskRetentionTTL)
	kept := r.tasks[:0]
	for _, t := range r.tasks {
		if t.Running {
			kept = append(kept, t)
			continue
		}
		if t.FinishedAt == "" {
			kept = append(kept, t)
			continue
		}
		fin, err := time.Parse(time.RFC3339, t.FinishedAt)
		if err == nil && fin.Before(cutoff) {
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) > maxTaskHistory {
		// 先按时间淘汰已完成；保留 running 任务 + 最近的若干已完成
		running := make([]*Task, 0)
		finished := make([]*Task, 0, len(kept))
		for _, t := range kept {
			if t.Running {
				running = append(running, t)
			} else {
				finished = append(finished, t)
			}
		}
		// 保留 finished 的最后 (maxTaskHistory - len(running)) 条
		room := maxTaskHistory - len(running)
		if room < 0 {
			room = 0
		}
		if len(finished) > room {
			finished = finished[len(finished)-room:]
		}
		kept = append(running, finished...)
	}
	r.tasks = kept
}

func (r *TaskRegistry) findLocked(taskID string) *Task {
	for _, t := range r.tasks {
		if t.ID == taskID {
			return t
		}
	}
	return nil
}

func taskIDFromInt(id int64) string {
	return "task-" + time.Now().Format("20060102-150405") + "-" + atomicIDStr(id)
}

func atomicIDStr(id int64) string {
	// 简化：直接十进制字符串
	if id == 0 {
		return "0"
	}
	digits := make([]byte, 0, 20)
	x := id
	if x < 0 {
		x = -x
	}
	for x > 0 {
		digits = append([]byte{byte('0' + x%10)}, digits...)
		x /= 10
	}
	return string(digits)
}

// ── Wails bindings ─────────────────────────────────────────────────────────

// GetTasks 前端轮询用：返回当前所有任务快照（含 running + 最近完成）
func (a *App) GetTasks() []Task {
	if a.tasks == nil {
		return nil
	}
	return a.tasks.Snapshot()
}

// ClearFinishedTasks 前端「清空已完成」按钮使用
func (a *App) ClearFinishedTasks() {
	if a.tasks == nil {
		return
	}
	a.tasks.ClearFinished()
}
