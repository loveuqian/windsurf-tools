import {
  Activity,
  BookOpen,
  Globe,
  HardDriveDownload,
  Heart,
  LayoutDashboard,
  Settings,
  Users,
} from "lucide-react";
import { useMainViewStore } from "../../stores/useMainViewStore";
import { PRIMARY_POOL_LABEL, type ShellViewTab } from "../../utils/appMode";

interface MenuItem {
  id: ShellViewTab;
  icon: typeof Users;
  label: string;
}

const MENU_ITEMS: MenuItem[] = [
  { id: "Dashboard", icon: LayoutDashboard, label: "总览" },
  { id: "Accounts", icon: Users, label: PRIMARY_POOL_LABEL },
  { id: "Usage", icon: Activity, label: "用量统计" },
  { id: "Relay", icon: Globe, label: "OpenAI Relay" },
  { id: "Cleanup", icon: HardDriveDownload, label: "清理优化" },
  { id: "Settings", icon: Settings, label: "MITM 设置" },
];

/**
 * Sidebar — React 重构版（Day 2 完整迁移到带 MITM 概况卡片版本）。
 * 当前最小可用：8 个 view 切换 + 底部 Help/About 入口。
 */
export default function Sidebar() {
  const activeTab = useMainViewStore((s) => s.activeTab);
  const setActiveTab = useMainViewStore((s) => s.setActiveTab);

  return (
    <nav className="w-60 h-full ios-glass border-r border-ios-divider dark:border-ios-dividerDark flex flex-col pt-6 pb-6 z-40 shrink-0">
      <div className="px-5 pb-2 mb-2 text-xs font-semibold uppercase text-ios-textSecondary dark:text-ios-textSecondaryDark tracking-wider">
        导航
      </div>
      <ul className="flex-1 space-y-1.5 px-3">
        {MENU_ITEMS.map((item) => {
          const Icon = item.icon;
          const active = activeTab === item.id;
          return (
            <li key={item.id}>
              <button
                type="button"
                className={[
                  "no-drag-region",
                  "w-full flex items-center px-4 py-2.5 rounded-[14px] text-[14px] transition-all duration-[250ms] font-medium ios-btn",
                  active
                    ? "bg-gradient-to-b from-[#3b82f6] to-ios-blue text-white shadow-md shadow-ios-blue/25 ring-1 ring-black/5 dark:ring-white/10 ring-inset"
                    : "text-ios-text dark:text-ios-textDark hover:bg-black/5 dark:hover:bg-white/10",
                ].join(" ")}
                onClick={() => setActiveTab(item.id)}
              >
                <Icon
                  className={`w-5 h-5 mr-3 transition-opacity duration-300 ${
                    active ? "opacity-100" : "opacity-70"
                  }`}
                  strokeWidth={2.2}
                />
                {item.label}
              </button>
            </li>
          );
        })}
      </ul>

      <div className="mx-3 mt-3 flex gap-2">
        <button
          type="button"
          className={`no-drag-region flex-1 flex items-center justify-center gap-1.5 py-2 rounded-[12px] text-[12px] font-bold transition-all ios-btn ${
            activeTab === "Help"
              ? "bg-ios-blue text-white shadow-sm"
              : "bg-black/[0.04] text-gray-600 dark:bg-white/[0.06] dark:text-gray-400 hover:bg-black/[0.08] dark:hover:bg-white/[0.1]"
          }`}
          onClick={() => setActiveTab("Help")}
        >
          <BookOpen className="h-3.5 w-3.5" strokeWidth={2.4} />
          帮助
        </button>
        <button
          type="button"
          className={`no-drag-region flex-1 flex items-center justify-center gap-1.5 py-2 rounded-[12px] text-[12px] font-bold transition-all ios-btn ${
            activeTab === "About"
              ? "bg-rose-500 text-white shadow-sm"
              : "bg-black/[0.04] text-gray-600 dark:bg-white/[0.06] dark:text-gray-400 hover:bg-black/[0.08] dark:hover:bg-white/[0.1]"
          }`}
          onClick={() => setActiveTab("About")}
        >
          <Heart className="h-3.5 w-3.5" strokeWidth={2.4} />
          关于
        </button>
      </div>
    </nav>
  );
}
