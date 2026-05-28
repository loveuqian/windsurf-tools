import { useMemo, useState } from "react";
import {
  Activity,
  BookOpen,
  Globe,
  HardDriveDownload,
  Hash,
  Heart,
  Layers,
  LayoutDashboard,
  MessageSquare,
  Plus,
  Settings as SettingsIcon,
  Shield,
  User,
  Users,
} from "lucide-react";
import { useAccountStore } from "../../stores/useAccountStore";
import { useMainViewStore } from "../../stores/useMainViewStore";
import { useMitmStatusStore } from "../../stores/useMitmStatusStore";
import { useSettingsStore } from "../../stores/useSettingsStore";
import { showErrorToast, showToast } from "../../utils/toast";
import { PRIMARY_POOL_LABEL, type ShellViewTab } from "../../utils/appMode";

interface MenuItem {
  id: ShellViewTab;
  icon: typeof Users;
  label: string;
}

const MENU_ITEMS: MenuItem[] = [
  { id: "Dashboard", icon: LayoutDashboard, label: "总览" },
  { id: "Accounts", icon: Users, label: PRIMARY_POOL_LABEL },
  { id: "Providers", icon: Layers, label: "提供商" },
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
  const settings = useSettingsStore((s) => s.settings);
  const updateSettings = useSettingsStore((s) => s.updateSettings);

  // ── 路由模式胶囊：号池 ↔ 提供商 ──
  const routeMode: 'pool' | 'providers' =
    (settings as any)?.mitm_route_mode === 'providers' ? 'providers' : 'pool';
  const [switchingRouteMode, setSwitchingRouteMode] = useState(false);
  const setRouteMode = async (target: 'pool' | 'providers') => {
    if (routeMode === target || switchingRouteMode || !settings) return;
    setSwitchingRouteMode(true);
    try {
      await updateSettings({ ...settings, mitm_route_mode: target } as any);
      showToast(
        target === 'providers'
          ? '已切到提供商: MITM chat 走已激活卡片'
          : '已切回 Windsurf 号池接管',
        'success',
      );
    } catch (e: unknown) {
      showErrorToast(e, '切换路由模式失败');
    } finally {
      setSwitchingRouteMode(false);
    }
  };

  const activeKey = useMemo(
    () => status?.pool_status?.find((item) => item.is_current) ?? null,
    [status],
  );

  // 0.2: 合并 activeSummary + activeAccountLabel 为一个完整展示：
  // 主行优先 nickname > email > key_short。双信息时（nickname 与 email 均有）附副行。
  // hash 仅在 hover title 里出现，调试用。
  const activeKeyShort = String(activeKey?.key_short || "").trim();
  const activeNickname = String(activeKey?.nickname || "").trim();
  const activeEmail = String(activeKey?.email || "").trim();
  const activePrimary = activeNickname || activeEmail || activeKeyShort;
  const activeSecondary =
    activeNickname && activeEmail ? activeEmail : "";
  const activeSummary = activePrimary || "等待活跃 Key";

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

        <div
          className="mt-3 rounded-[14px] bg-black/[0.03] px-3 py-2 text-[11px] font-medium text-ios-textSecondary dark:bg-white/[0.05] dark:text-ios-textSecondaryDark"
          title={activeKeyShort || activeSummary}
        >
          当前活跃账号
          <div className="mt-1 flex items-center gap-1.5">
            {activePrimary && activePrimary !== "等待活跃 Key" ? (
              <User className="h-3 w-3 shrink-0 text-ios-blue" strokeWidth={2.4} />
            ) : null}
            <span className="truncate text-[12px] font-semibold text-ios-text dark:text-ios-textDark">
              {activeSummary}
            </span>
          </div>
          {activeSecondary ? (
            <div
              className="mt-0.5 truncate text-[10px] font-medium opacity-80"
              title={activeSecondary}
            >
              {activeSecondary}
            </div>
          ) : null}
        </div>

        {boundSessions.length > 0 ? (
          <div className="mt-2 rounded-[14px] bg-black/[0.02] px-3 py-2 dark:bg-white/[0.03]">
            <div className="flex items-center gap-1 text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark mb-1.5">
              <MessageSquare className="h-3 w-3 shrink-0" strokeWidth={2.2} />
              绑定对话 ({boundSessions.length})
            </div>
            <ul className="space-y-1">
              {boundSessions.map((session) => {
                // 0.3: 优先 title（对话名），fallback 到 conv_id_short hash。
                const title = String(session.title || "").trim();
                const display = title || session.conv_id_short;
                const isHash = !title;
                return (
                  <li
                    key={session.conv_id_short}
                    className="flex items-center gap-1.5 text-[10px] text-ios-text dark:text-ios-textDark"
                  >
                    <Hash className="h-3 w-3 shrink-0 opacity-40" strokeWidth={2} />
                    <span
                      className={`truncate ${isHash ? "font-mono" : ""}`}
                      title={
                        title
                          ? `${title}　·　${session.conv_id_short}`
                          : session.conv_id_short
                      }
                    >
                      {display}
                    </span>
                    <span className="ml-auto shrink-0 text-[9px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                      {session.request_count}次
                    </span>
                  </li>
                );
              })}
            </ul>
          </div>
        ) : null}

        {/* 1.6: 空号池 → 醒目 CTA；非空 → 健康统计 */}
        {accounts.length === 0 ? (
          <button
            type="button"
            className="no-drag-region mt-3 flex w-full items-center justify-center gap-1.5 rounded-[14px] bg-gradient-to-b from-[#3b82f6] to-ios-blue px-3 py-2 text-[12px] font-bold text-white shadow-md shadow-ios-blue/25 ring-1 ring-black/5 ring-inset transition-transform ios-btn hover:-translate-y-px"
            onClick={() => useMainViewStore.getState().openImportModal()}
            title="粘贴 API Key / JWT / 邮箱密码批量导入"
          >
            <Plus className="h-3.5 w-3.5" strokeWidth={2.6} />
            立即导入第一个账号
          </button>
        ) : (
          <div className="mt-3 flex items-center gap-2 rounded-[14px] border border-emerald-500/12 bg-emerald-500/[0.06] px-3 py-2 text-[11px] font-medium text-emerald-700 dark:text-emerald-300">
            <Shield className="h-3.5 w-3.5 shrink-0" strokeWidth={2.4} />
            健康 {healthyCount} / {totalCount}
          </div>
        )}

        {/* ★ 路由模式胶囊：号池 ↔ 提供商(iOS 风滑动指示条) */}
        <div
          className="no-drag-region relative mt-2 flex items-stretch rounded-full border border-black/[0.06] bg-white/80 p-0.5 shadow-sm dark:border-white/[0.08] dark:bg-white/[0.05]"
          role="tablist"
        >
          <span
            className={`absolute top-0.5 bottom-0.5 left-0.5 rounded-full shadow-md transition-[transform,background-image,box-shadow] duration-[420ms] ease-[cubic-bezier(0.25,1,0.5,1)] ${
              routeMode === 'providers'
                ? 'bg-gradient-to-b from-violet-500 via-fuchsia-400 to-rose-300 shadow-fuchsia-500/25'
                : 'bg-gradient-to-b from-[#3b82f6] to-ios-blue shadow-ios-blue/25'
            }`}
            style={{
              width: 'calc(50% - 2px)',
              transform: routeMode === 'providers' ? 'translateX(calc(100% + 0px))' : 'translateX(0)',
            }}
          />
          <button
            type="button"
            className={`ios-btn relative z-10 flex-1 flex h-7 items-center justify-center gap-1 rounded-full text-[11px] font-bold transition-colors duration-200 ${
              routeMode === 'pool'
                ? 'text-white'
                : 'text-ios-textSecondary hover:text-ios-text dark:text-ios-textSecondaryDark dark:hover:text-ios-textDark'
            }`}
            disabled={switchingRouteMode}
            onClick={() => setRouteMode('pool')}
          >
            <Users className="h-3 w-3" strokeWidth={2.6} />
            号池
          </button>
          <button
            type="button"
            className={`ios-btn relative z-10 flex-1 flex h-7 items-center justify-center gap-1 rounded-full text-[11px] font-bold transition-colors duration-200 ${
              routeMode === 'providers'
                ? 'text-white'
                : 'text-ios-textSecondary hover:text-ios-text dark:text-ios-textSecondaryDark dark:hover:text-ios-textDark'
            }`}
            disabled={switchingRouteMode}
            onClick={() => setRouteMode('providers')}
          >
            <Globe className="h-3 w-3" strokeWidth={2.6} />
            提供商
          </button>
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
