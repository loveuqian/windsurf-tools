/**
 * Dashboard view — React 重构占位
 *
 * 完整迁移见 Day 3。当前显示重构进度提示，避免主区空白。
 */
export default function Dashboard() {
  return (
    <div className="p-6 md:p-8 flex flex-1 flex-col max-w-6xl mx-auto w-full min-h-0">
      <h1 className="text-[28px] sm:text-[32px] font-bold tracking-tight">
        总览
      </h1>
      <p className="mt-2 text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
        前端已切换到 React。该 view 计划在 Day 3 完整迁移。
      </p>
    </div>
  );
}
