import { useMemo } from "react";
import {
  Activity,
  BookOpen,
  Globe,
  HardDriveDownload,
  Hash,
  Heart,
  LayoutDashboard,
  MessageSquare,
  Settings as SettingsIcon,
  Shield,
  User,
  Users,
} from "lucide-react";
import { useAccountStore } from "../../stores/useAccountStore";
import { useMainViewStore } from "../../stores/useMainViewStore";
import { useMitmStatusStore } from "../../stores/useMitmStatusStore";
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
  { id: "Settings", icon: SettingsIcon, label: "MITM 设置" },
];

/**
 * Sidebar — Vue 1:1 完整迁移：导航 + MITM 概况卡（号池总数 / 当前活跃 Key /
 * 当前活跃账号 / 绑定对话 / 健康度）+ Help/About 入口。
 */
export default function Sidebar() {
  const activeTab = useMainViewStore((s) => s.activeTab);
  const setActiveTab = useMainViewStore((s) => s.setActiveTab);

  const accounts = useAccountStore((s) => s.accounts);
  const status = useMitmStatusStore((s) => s.status);

  const activeKey = useMemo(
    () => status?.pool_status?.find((item) => item.is_current) ?? null,
    [status],
  );

  const activeSummary = useMemo(() => {
    const key = String(activeKey?.key_short || "").trim();
    if (!key) return "等待活跃 Key";
    return key;
  }, [activeKey]);

  const activeAccountLabel = useMemo(() => {
    const k = activeKey;
    if (!k) return "";
    const nick = String(k.nickname || "").trim();
    const email = String(k.email || "").trim();
    if (nick && email) return `${nick} (${email})`;
    return email || nick || "";
  }, [activeKey]);

  const boundSessions = useMemo(() => {
    const sessions = status?.active_sessions ?? [];
    const currentHash = activeKey?.key_hash ?? "";
    const currentShort = activeKey?.key_short ?? "";
    if (!currentHash && !currentShort) return [];
    return sessions.filter((s) => {
      if (currentHash && s.pool_key_hash) {
        return s.pool_key_hash === currentHash;
      }
      return s.pool_key_short === currentShort;
    });
  }, [status, activeKey]);

  const healthyCount =
    status?.pool_status?.filter((item) => item.healthy).length ?? 0;
  const totalCount = status?.pool_status?.length ?? 0;

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

      <div className="mx-3 mt-4 rounded-[18px] border border-black/[0.05] bg-white/60 px-4 py-4 shadow-[0_8px_24px_rgba(15,23,42,0.06)] dark:border-white/[0.06] dark:bg-white/[0.04]">
        <div className="text-[11px] font-bold uppercase tracking-[0.22em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
          MITM 概况
        </div>
        <div className="mt-3 flex items-center justify-between">
          <div>
            <div className="text-[20px] font-extrabold leading-none text-ios-text dark:text-ios-textDark">
              {accounts.length}
            </div>
            <div className="mt-1 text-[11px] font-medium text-ios-textSecondary dark:text-ios-textSecondaryDark">
              号池总数
            </div>
          </div>
          <span className="rounded-full bg-ios-blue/10 px-2.5 py-1 text-[10px] font-bold tracking-wide text-ios-blue">
            Pure MITM
          </span>
        </div>

        <div className="mt-3 rounded-[14px] bg-black/[0.03] px-3 py-2 text-[11px] font-medium text-ios-textSecondary dark:bg-white/[0.05] dark:text-ios-textSecondaryDark">
          当前活跃 Key
          <div
            className="mt-1 truncate text-[12px] font-semibold text-ios-text dark:text-ios-textDark"
            title={activeSummary}
          >
            {activeSummary}
          </div>
        </div>

        {activeAccountLabel ? (
          <div className="mt-2 flex items-center gap-1.5 rounded-[14px] bg-ios-blue/[0.06] px-3 py-2 text-[11px] font-medium text-ios-blue">
            <User className="h-3.5 w-3.5 shrink-0" strokeWidth={2.4} />
            <span className="truncate" title={activeAccountLabel}>
              {activeAccountLabel}
            </span>
          </div>
        ) : null}

        {boundSessions.length > 0 ? (
          <div className="mt-2 rounded-[14px] bg-black/[0.02] px-3 py-2 dark:bg-white/[0.03]">
            <div className="flex items-center gap-1 text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark mb-1.5">
              <MessageSquare className="h-3 w-3 shrink-0" strokeWidth={2.2} />
              绑定对话 ({boundSessions.length})
            </div>
            <ul className="space-y-1">
              {boundSessions.map((session) => (
                <li
                  key={session.conv_id_short}
                  className="flex items-center gap-1.5 text-[10px] text-ios-text dark:text-ios-textDark"
                >
                  <Hash className="h-3 w-3 shrink-0 opacity-40" strokeWidth={2} />
                  <span
                    className="truncate font-mono"
                    title={session.conv_id_short}
                  >
                    {session.conv_id_short}
                  </span>
                  <span className="ml-auto shrink-0 text-[9px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                    {session.request_count}次
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}

        <div className="mt-3 flex items-center gap-2 rounded-[14px] border border-emerald-500/12 bg-emerald-500/[0.06] px-3 py-2 text-[11px] font-medium text-emerald-700 dark:text-emerald-300">
          <Shield className="h-3.5 w-3.5 shrink-0" strokeWidth={2.4} />
          健康 {healthyCount} / {totalCount}
        </div>
      </div>

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
