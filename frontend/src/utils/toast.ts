import { create } from "zustand";
import { friendlyError } from "./errorMessage";

export type ToastKind = "success" | "error" | "info" | "warning";

export interface ToastItem {
  id: number;
  message: string;
  kind: ToastKind;
}

const MAX_TOAST_QUEUE = 6;
/** 同 (kind, message) 在该窗口内只显示一次。避免连续点同一按钮失败叠出 5+ 条 toast。 */
const TOAST_DEDUP_MS = 1500;
const lastShownAt = new Map<string, number>();

interface ToastState {
  queue: ToastItem[];
  push: (item: ToastItem) => void;
  remove: (id: number) => void;
}

const useToastStore = create<ToastState>((set) => ({
  queue: [],
  push: (item) =>
    set((s) => {
      const next = [...s.queue, item];
      return {
        queue: next.length > MAX_TOAST_QUEUE ? next.slice(-MAX_TOAST_QUEUE) : next,
      };
    }),
  remove: (id) => set((s) => ({ queue: s.queue.filter((t) => t.id !== id) })),
}));

let toastSeq = 0;

/** Hook：在 React 组件内订阅 toast 队列以渲染 IToast。 */
export function useToastQueue(): ToastItem[] {
  return useToastStore((s) => s.queue);
}

/** 非阻塞提示；支持 message 内换行（white-space: pre-line）。
 *  内置 1.5s dedup —— 短时间内重复触发同 (kind, message) 不会叠加。
 */
export function showToast(
  message: string,
  kind: ToastKind = "info",
  durationMs = 4800,
): void {
  const dedupKey = `${kind}::${message}`;
  const now = Date.now();
  const last = lastShownAt.get(dedupKey) ?? 0;
  if (now - last < TOAST_DEDUP_MS) {
    return;
  }
  lastShownAt.set(dedupKey, now);
  // 简单清理：避免长会话下 map 一直增长（保留最近 64 条）
  if (lastShownAt.size > 64) {
    const stale = now - TOAST_DEDUP_MS * 4;
    for (const [k, t] of lastShownAt) {
      if (t < stale) lastShownAt.delete(k);
    }
  }

  const id = ++toastSeq;
  useToastStore.getState().push({ id, message, kind });
  window.setTimeout(() => {
    useToastStore.getState().remove(id);
  }, durationMs);
}

/** 把任意错误（含 Go rpc error 字符串）翻译成中文短句后弹 error toast。
 *  用法：try { await api.xxx() } catch (e) { showErrorToast(e, '保存设置失败') }
 */
export function showErrorToast(err: unknown, fallback: string): void {
  showToast(friendlyError(err, fallback), "error", 5400);
}

/** 手动关闭 toast（用户点击或 swipe 触发，提前于 auto-dismiss） */
export function dismissToast(id: number): void {
  useToastStore.getState().remove(id);
}

// ── Confirm Dialog ──────────────────────────────────────────────────────

export interface ConfirmState {
  visible: boolean;
  message: string;
  confirmText: string;
  cancelText: string;
  destructive: boolean;
  resolve: ((value: boolean) => void) | null;
}

interface ConfirmStoreState extends ConfirmState {
  open: (
    next: Omit<ConfirmState, "visible" | "resolve"> & {
      resolve: (value: boolean) => void;
    },
  ) => void;
  close: () => ConfirmState["resolve"];
}

const useConfirmStore = create<ConfirmStoreState>((set, get) => ({
  visible: false,
  message: "",
  confirmText: "确定",
  cancelText: "取消",
  destructive: false,
  resolve: null,
  open: (next) =>
    set({
      visible: true,
      message: next.message,
      confirmText: next.confirmText,
      cancelText: next.cancelText,
      destructive: next.destructive,
      resolve: next.resolve,
    }),
  close: () => {
    const r = get().resolve;
    set({ visible: false, resolve: null });
    return r;
  },
}));

export function useConfirmState(): ConfirmState {
  return useConfirmStore((s) => ({
    visible: s.visible,
    message: s.message,
    confirmText: s.confirmText,
    cancelText: s.cancelText,
    destructive: s.destructive,
    resolve: s.resolve,
  }));
}

export function resolveConfirm(value: boolean): void {
  const r = useConfirmStore.getState().close();
  r?.(value);
}

export function confirmDialog(
  message: string,
  options?: { confirmText?: string; cancelText?: string; destructive?: boolean },
): Promise<boolean> {
  const prev = useConfirmStore.getState().resolve;
  if (prev) {
    prev(false);
  }
  return new Promise((resolve) => {
    useConfirmStore.getState().open({
      message,
      confirmText: options?.confirmText ?? "确定",
      cancelText: options?.cancelText ?? "取消",
      destructive: options?.destructive ?? false,
      resolve,
    });
  });
}
