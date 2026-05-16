import { Sun } from "lucide-react";
import { APP_PRODUCT_NAME, APP_PRODUCT_TAGLINE } from "../../utils/appMode";
import { themeLabel, useThemeStore } from "../../utils/theme";

/**
 * Header — React 重构占位（Day 2 完整迁移）
 *
 * 当前最小可用：产品名、副标题、主题切换按钮。
 */
export default function Header() {
  const themeMode = useThemeStore((s) => s.mode);
  const cycleTheme = useThemeStore((s) => s.cycle);

  return (
    <header className="drag-region flex items-center gap-4 px-5 py-3 ios-glass border-b border-ios-divider dark:border-ios-dividerDark shrink-0">
      <div className="flex items-center gap-3 min-w-0">
        <div className="w-9 h-9 rounded-2xl bg-gradient-to-b from-[#3b82f6] to-ios-blue flex items-center justify-center text-white font-bold text-[17px] shadow-md shadow-ios-blue/25">
          W
        </div>
        <div className="min-w-0">
          <div className="text-[15px] font-bold text-ios-text dark:text-ios-textDark leading-tight">
            {APP_PRODUCT_NAME}
          </div>
          <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark leading-tight">
            {APP_PRODUCT_TAGLINE}
          </div>
        </div>
      </div>

      <div className="ml-auto flex items-center gap-2">
        <button
          type="button"
          className="no-drag-region inline-flex items-center gap-1.5 rounded-full bg-black/[0.04] dark:bg-white/[0.06] px-3 py-1.5 text-[12px] font-bold text-ios-text dark:text-ios-textDark hover:bg-black/[0.08] dark:hover:bg-white/[0.1] transition-colors ios-btn"
          title={themeLabel(themeMode)}
          onClick={cycleTheme}
        >
          <Sun className="h-3.5 w-3.5" strokeWidth={2.4} />
          {themeLabel(themeMode).replace("主题：", "")}
        </button>
      </div>
    </header>
  );
}
