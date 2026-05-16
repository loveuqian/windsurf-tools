import { CheckCircle2, XCircle, Info, AlertTriangle, X } from "lucide-react";
import { dismissToast, useToastQueue, type ToastKind } from "../../utils/toast";

const KIND_ICON: Record<ToastKind, typeof CheckCircle2> = {
  success: CheckCircle2,
  error: XCircle,
  warning: AlertTriangle,
  info: Info,
};

const KIND_TONE: Record<ToastKind, string> = {
  success: "border-emerald-500/30 bg-emerald-500/[0.10] text-emerald-700 dark:text-emerald-300",
  error: "border-rose-500/30 bg-rose-500/[0.10] text-rose-700 dark:text-rose-300",
  warning: "border-amber-500/30 bg-amber-500/[0.10] text-amber-700 dark:text-amber-300",
  info: "border-ios-blue/30 bg-ios-blue/[0.10] text-ios-blue dark:text-blue-300",
};

/**
 * IToast — React 重构占位（Day 2 完整迁移到与 Vue 版动效一致）。
 * 当前最小可用：底部右侧堆叠列表 + 双击/X 关闭。
 */
export default function IToast() {
  const queue = useToastQueue();
  if (queue.length === 0) {
    return null;
  }
  return (
    <div className="fixed bottom-6 right-6 z-[120] flex flex-col gap-2 pointer-events-none">
      {queue.map((t) => {
        const Icon = KIND_ICON[t.kind];
        return (
          <div
            key={t.id}
            className={[
              "pointer-events-auto",
              "min-w-[260px] max-w-[420px] rounded-[14px] border px-4 py-2.5 backdrop-blur-md shadow-lg",
              "flex items-start gap-2.5 ios-page-enter whitespace-pre-line text-[13px] font-semibold",
              KIND_TONE[t.kind],
            ].join(" ")}
          >
            <Icon className="h-[18px] w-[18px] shrink-0 mt-0.5" strokeWidth={2.4} />
            <span className="flex-1 leading-relaxed">{t.message}</span>
            <button
              type="button"
              className="no-drag-region opacity-60 hover:opacity-100 transition-opacity"
              onClick={() => dismissToast(t.id)}
              aria-label="关闭"
            >
              <X className="h-3.5 w-3.5" strokeWidth={2.4} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
