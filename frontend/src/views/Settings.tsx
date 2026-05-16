/** Settings view — React 重构占位（Day 3 完整迁移） */
export default function Settings() {
  return (
    <div className="p-6 md:p-8 flex flex-1 flex-col max-w-6xl mx-auto w-full min-h-0">
      <h1 className="text-[28px] sm:text-[32px] font-bold tracking-tight">
        设置
      </h1>
      <p className="mt-2 text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
        前端已切换到 React。该 view 计划在 Day 3 完整迁移。
      </p>
    </div>
  );
}
