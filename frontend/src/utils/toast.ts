import { ref } from 'vue'
import { friendlyError } from './errorMessage'

export type ToastKind = 'success' | 'error' | 'info' | 'warning'

export interface ToastItem {
  id: number
  message: string
  kind: ToastKind
}

export const toastQueue = ref<ToastItem[]>([])

let toastSeq = 0

const MAX_TOAST_QUEUE = 6
/** 同 (kind, message) 在该窗口内只显示一次。避免连续点同一按钮失败叠出 5+ 条 toast。 */
const TOAST_DEDUP_MS = 1500
const lastShownAt = new Map<string, number>()

/** 非阻塞提示；支持 message 内换行（white-space: pre-line）。
 *  内置 1.5s dedup —— 短时间内重复触发同 (kind, message) 不会叠加。
 */
export function showToast(message: string, kind: ToastKind = 'info', durationMs = 4800): void {
  const dedupKey = `${kind}::${message}`
  const now = Date.now()
  const last = lastShownAt.get(dedupKey) ?? 0
  if (now - last < TOAST_DEDUP_MS) {
    return
  }
  lastShownAt.set(dedupKey, now)
  // 简单清理：避免长会话下 map 一直增长（保留最近 64 条）
  if (lastShownAt.size > 64) {
    const stale = now - TOAST_DEDUP_MS * 4
    for (const [k, t] of lastShownAt) {
      if (t < stale) lastShownAt.delete(k)
    }
  }

  const id = ++toastSeq
  const next = [...toastQueue.value, { id, message, kind }]
  toastQueue.value = next.length > MAX_TOAST_QUEUE ? next.slice(-MAX_TOAST_QUEUE) : next
  window.setTimeout(() => {
    toastQueue.value = toastQueue.value.filter((t) => t.id !== id)
  }, durationMs)
}

/** 把任意错误（含 Go rpc error 字符串）翻译成中文短句后弹 error toast。
 *  用法：try { await api.xxx() } catch (e) { showErrorToast(e, '保存设置失败') }
 */
export function showErrorToast(err: unknown, fallback: string): void {
  showToast(friendlyError(err, fallback), 'error', 5400)
}

/** 手动关闭 toast（用户点击或 swipe 触发，提前于 auto-dismiss） */
export function dismissToast(id: number): void {
  toastQueue.value = toastQueue.value.filter((t) => t.id !== id)
}

export interface ConfirmState {
  visible: boolean
  message: string
  confirmText: string
  cancelText: string
  destructive: boolean
  resolve: ((value: boolean) => void) | null
}

export const confirmState = ref<ConfirmState>({
  visible: false,
  message: '',
  confirmText: '确定',
  cancelText: '取消',
  destructive: false,
  resolve: null,
})

export function resolveConfirm(value: boolean): void {
  const r = confirmState.value.resolve
  confirmState.value.visible = false
  confirmState.value.resolve = null
  r?.(value)
}

export function confirmDialog(
  message: string,
  options?: { confirmText?: string; cancelText?: string; destructive?: boolean },
): Promise<boolean> {
  const prev = confirmState.value.resolve
  if (prev) {
    prev(false)
  }
  return new Promise((resolve) => {
    confirmState.value = {
      visible: true,
      message,
      confirmText: options?.confirmText ?? '确定',
      cancelText: options?.cancelText ?? '取消',
      destructive: options?.destructive ?? false,
      resolve,
    }
  })
}
