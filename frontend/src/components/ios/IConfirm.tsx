import { useEffect } from "react";
import { resolveConfirm, useConfirmState } from "../../utils/toast";

/**
 * IConfirm — 全局确认对话框。Esc 取消 / Enter 确认 / 背景遮罩 + 主按钮，
 * destructive 操作用红色主按钮。
 */
export default function IConfirm() {
  const state = useConfirmState();

  useEffect(() => {
    if (!state.visible) {
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        resolveConfirm(false);
      } else if (e.key === "Enter") {
        e.preventDefault();
        resolveConfirm(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [state.visible]);

  if (!state.visible) {
    return null;
  }

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-md"
      onClick={(e) => {
        if (e.target === e.currentTarget) resolveConfirm(false);
      }}
    >
      <div className="w-full sm:max-w-[420px] mx-auto bg-white dark:bg-[#1C1C1E] rounded-ios-card shadow-ios-sheet ring-1 ring-white/50 dark:ring-white/10 overflow-hidden">
        <div className="px-6 pt-6 pb-4">
          <p className="text-[15px] leading-relaxed text-ios-text dark:text-ios-textDark whitespace-pre-line">
            {state.message}
          </p>
        </div>
        <div className="grid grid-cols-2 border-t border-black/[0.06] dark:border-white/[0.08]">
          <button
            type="button"
            className="no-drag-region py-3 text-[15px] font-bold text-ios-blue ios-btn hover:bg-black/[0.04] dark:hover:bg-white/[0.06]"
            onClick={() => resolveConfirm(false)}
          >
            {state.cancelText}
          </button>
          <button
            type="button"
            className={`no-drag-region py-3 text-[15px] font-bold ios-btn border-l border-black/[0.06] dark:border-white/[0.08] ${
              state.destructive
                ? "text-rose-600 dark:text-rose-400 hover:bg-rose-500/[0.06]"
                : "text-ios-blue hover:bg-black/[0.04] dark:hover:bg-white/[0.06]"
            }`}
            onClick={() => resolveConfirm(true)}
          >
            {state.confirmText}
          </button>
        </div>
      </div>
    </div>
  );
}
