import { useEffect, type ComponentType } from "react";
import Header from "./components/layout/Header";
import Sidebar from "./components/layout/Sidebar";
import AppFooter from "./components/layout/AppFooter";
import IConfirm from "./components/ios/IConfirm";
import IToast from "./components/ios/IToast";
import ImportModal from "./components/accounts/ImportModal";
import CommandPalette from "./components/CommandPalette";
import HotkeysCheatSheet from "./components/HotkeysCheatSheet";
import IContextMenuPortal from "./components/ios/IContextMenu";
import TaskDrawer from "./components/TaskDrawer";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { useDockBadge } from "./hooks/useDockBadge";
import { useGlobalHotkeys } from "./hooks/useGlobalHotkeys";
import { useWindowGeometryMemory } from "./hooks/useWindowGeometryMemory";
import { useMainViewStore } from "./stores/useMainViewStore";
import { useAccountStore } from "./stores/useAccountStore";
import { useSettingsStore } from "./stores/useSettingsStore";
import { useMitmStatusStore } from "./stores/useMitmStatusStore";
import { startTaskPolling } from "./stores/useTaskStore";
import {
  DEFAULT_MAIN_VIEW,
  type ShellViewTab,
} from "./utils/appMode";
import Dashboard from "./views/Dashboard";
import Accounts from "./views/Accounts";
import Providers from "./views/Providers";
import Settings from "./views/Settings";
import Usage from "./views/Usage";
import Relay from "./views/Relay";
import Cleanup from "./views/Cleanup";
import Help from "./views/Help";
import About from "./views/About";

const VIEW_REGISTRY: Record<ShellViewTab, ComponentType> = {
  Dashboard,
  Accounts,
  Providers,
  Settings,
  Usage,
  Relay,
  Cleanup,
  Help,
  About,
};

const SHELL_TABS = Object.keys(VIEW_REGISTRY) as ShellViewTab[];

const resolveShellViewTab = (value: string | null | undefined): ShellViewTab =>
  SHELL_TABS.includes(value as ShellViewTab)
    ? (value as ShellViewTab)
    : DEFAULT_MAIN_VIEW;

export default function App() {
  // 1.2: 全局快捷键（Cmd+1~6 / Cmd+R / Cmd+K / Cmd+, / ?）
  useGlobalHotkeys();
  // 2.4: 启动还原窗口几何 + 1.5s 防抖写盘
  useWindowGeometryMemory();
  // 2.3: Dock / 标题栏徽章反映未读通知 + 号池告急
  useDockBadge();

  const activeTab = useMainViewStore((s) => s.activeTab);
  const setActiveTab = useMainViewStore((s) => s.setActiveTab);
  const importModalOpen = useMainViewStore((s) => s.importModalOpen);
  const closeImportModal = useMainViewStore((s) => s.closeImportModal);

  // 启动数据加载：settings → 账号 → MITM 状态
  useEffect(() => {
    const accounts = useAccountStore.getState();
    const settings = useSettingsStore.getState();
    const mitm = useMitmStatusStore.getState();

    void settings.fetchSettings();
    if (!(activeTab in VIEW_REGISTRY)) {
      setActiveTab(DEFAULT_MAIN_VIEW);
    }
    mitm.startPolling();
    void accounts.ensureAccountsLoaded();
    const stopTaskPolling = startTaskPolling();

    // 从后台切回前台时统一刷新（与 Vue 版 F2 修复一致）
    let lastFocusRefresh = 0;
    const onVisibilityChange = () => {
      if (
        typeof document === "undefined" ||
        document.visibilityState !== "visible"
      ) {
        return;
      }
      const now = Date.now();
      if (now - lastFocusRefresh < 2500) {
        return;
      }
      lastFocusRefresh = now;
      void useAccountStore.getState().fetchAccounts();
      useMitmStatusStore.getState().notifyVisibleAgain();
    };
    document.addEventListener("visibilitychange", onVisibilityChange);

    // 1.2: Cmd+R / 命令面板「刷新当前 view」 — 根据当前 tab 触发对应刷新
    const onMainViewRefresh = () => {
      const tab = useMainViewStore.getState().activeTab;
      if (tab === "Dashboard") {
        void useAccountStore.getState().fetchAccounts(true);
        void useMitmStatusStore.getState().fetchStatus(true);
      } else if (tab === "Accounts") {
        void useAccountStore.getState().fetchAccounts(true);
      } else if (tab === "Settings") {
        void useSettingsStore.getState().fetchSettings(true);
      } else {
        // Usage/Relay/Cleanup 各自有 polling，统一刷一遍 store 兜底
        void useMitmStatusStore.getState().fetchStatus(true);
      }
    };
    window.addEventListener("mainview-refresh", onMainViewRefresh);

    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("mainview-refresh", onMainViewRefresh);
      useMitmStatusStore.getState().stopPolling();
      stopTaskPolling();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const ActiveView = VIEW_REGISTRY[resolveShellViewTab(activeTab)];

  return (
    <div className="flex flex-col h-full text-ios-text dark:text-ios-textDark overflow-hidden antialiased app-root">
      <Header />
      <div className="flex flex-1 overflow-hidden relative">
        <Sidebar />
        <main className="flex-1 flex flex-col min-h-0 overflow-hidden relative bg-black/[0.01] dark:bg-white/[0.01]">
          <div className="flex-1 overflow-y-auto overflow-x-hidden relative scroll-smooth min-h-0 flex flex-col">
            <div className="flex-1 shrink-0 flex flex-col relative">
              <ErrorBoundary>
                <ActiveView />
              </ErrorBoundary>
            </div>
            <AppFooter />
          </div>
        </main>
      </div>
      <IConfirm />
      <IToast />
      <TaskDrawer />
      <ImportModal isOpen={importModalOpen} onClose={closeImportModal} />
      <CommandPalette />
      <HotkeysCheatSheet />
      <IContextMenuPortal />
    </div>
  );
}
