/** AppFooter — React 重构占位（Day 2 完整迁移） */
export default function AppFooter() {
  const version = import.meta.env.VITE_APP_VERSION ?? "dev";
  return (
    <footer className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark text-center py-2 shrink-0 select-none">
      v{version} · React 重构进行中
    </footer>
  );
}
