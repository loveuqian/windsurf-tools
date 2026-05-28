/** AppFooter — 底部版本信息条。 */
export default function AppFooter() {
  const version = import.meta.env.VITE_APP_VERSION ?? "dev";
  return (
    <footer className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark text-center py-2 shrink-0 select-none">
      Windsurf Tools v{version} · 本地运行 · MIT 开源
    </footer>
  );
}
