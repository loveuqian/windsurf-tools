import { useEffect, useMemo } from "react";
import {
  Copy,
  Lock,
  Monitor,
  Moon,
  RadioTower,
  ShieldCheck,
  Sun,
} from "lucide-react";
import { APIInfo } from "../../api/wails";
import { useAccountStore } from "../../stores/useAccountStore";
import { useMitmStatusStore } from "../../stores/useMitmStatusStore";
import { useSettingsStore } from "../../stores/useSettingsStore";
import { APP_VERSION } from "../../utils/appMeta";
import { APP_PRODUCT_NAME, APP_PRODUCT_TAGLINE } from "../../utils/appMode";
import { themeLabel, useThemeStore } from "../../utils/theme";
import { showErrorToast, showToast } from "../../utils/toast";

/**
 * Header — Vue 1:1 完整迁移：产品名 / 模式徽章 / 当前活跃 Key 卡 / Pin 锁定徽章 /
 * 复制 API Key / 主题切换。
 */
export default function Header() {
  const themeMode = useThemeStore((s) => s.mode);
  const cycleTheme = useThemeStore((s) => s.cycle);

  const status = useMitmStatusStore((s) => s.status);
  const settings = useSettingsStore((s) => s.settings);
  const accounts = useAccountStore((s) => s.accounts);

  useEffect(() => {
    void useSettingsStore.getState().fetchSettings();
    void useAccountStore.getState().ensureAccountsLoaded();
  }, []);

  const activeKey = useMemo(
    () => status?.pool_status?.find((item) => item.is_current) ?? null,
    [status],
  );
  const isPinned = settings?.manual_pin_enabled === true;
  const pinnedAccount = useMemo(() => {
    const id = settings?.manual_pin_account_id;
    if (!id) return null;
    return accounts.find((a) => a.id === id) ?? null;
  }, [settings, accounts]);
  const pinnedLabel = isPinned
    ? pinnedAccount?.email ||
      settings?.manual_pin_account_id?.slice(0, 8) ||
      "账号"
    : "";

  const activeApiKey = useMemo(() => {
    const k = activeKey;
    if (!k?.email) return "";
    const acc = accounts.find((a) => a.email === k.email);
    return (acc?.windsurf_api_key || "").trim();
  }, [activeKey, accounts]);

  const handleCopyActiveKey = async () => {
    if (!activeApiKey) {
      showToast("当前活跃账号未配置 API Key", "warning");
      return;
    }
    try {
      await navigator.clipboard.writeText(activeApiKey);
      const k = activeApiKey;
      const short = k.length > 16 ? `${k.slice(0, 12)}…${k.slice(-4)}` : k;
      showToast(`已复制 ${short}`, "success");
    } catch (e) {
      showErrorToast(e, "复制失败");
    }
  };

  const handleUnpinFromHeader = async () => {
    try {
      await APIInfo.unpinManualAccount();
      await useSettingsStore.getState().fetchSettings(true);
      showToast("已解锁，自动切换已恢复", "success");
    } catch (e) {
      showErrorToast(e, "解锁失败");
    }
  };

  const poolCount = status?.pool_status?.length ?? 0;
  const healthyCount =
    status?.pool_status?.filter((item) => item.healthy).length ?? 0;

  const onlineEmailFull = String(activeKey?.key_short || "").trim();
  const onlineEmail = onlineEmailFull
    ? onlineEmailFull.length > 28
      ? `${onlineEmailFull.slice(0, 26)}…`
      : onlineEmailFull
    : status?.running
    ? "等待活跃 Key"
    : "MITM 未启动";
  const onlineSummary = !status?.running
    ? "启动后将从 MITM 号池轮换"
    : `健康 ${healthyCount} / ${poolCount}`;
  const sessionStateLabel = onlineEmailFull ? "当前活跃 Key" : "MITM 状态";
  const sessionStateTone = status?.running
    ? "border-emerald-500/18 bg-emerald-500/[0.08] text-emerald-700 dark:text-emerald-300"
    : "border-black/[0.06] bg-black/[0.03] text-ios-textSecondary dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textSecondaryDark";

  const ThemeIcon =
    themeMode === "light" ? Sun : themeMode === "dark" ? Moon : Monitor;

  return (
    <header className="drag-region grid h-[64px] w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-4 px-4 md:px-5 bg-white/82 dark:bg-[#1C1C1E]/88 backdrop-blur-2xl border-b border-ios-divider dark:border-ios-dividerDark select-none z-50 shrink-0">
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-gradient-to-br from-ios-blue to-sky-400 text-white shadow-[0_10px_22px_rgba(37,99,235,0.24)]">
          <ShieldCheck className="h-[18px] w-[18px]" strokeWidth={2.6} />
        </div>
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-[16px] font-semibold tracking-tight text-ios-text dark:text-ios-textDark">
              {APP_PRODUCT_NAME}
            </span>
            <span className="hidden rounded-full bg-ios-blue/10 px-2.5 py-0.5 text-[10px] font-bold tracking-wide text-ios-blue md:inline-flex">
              Pure MITM
            </span>
          </div>
          <div className="mt-0.5 flex min-w-0 items-center gap-2 text-ios-textSecondary dark:text-ios-textSecondaryDark">
            <span className="text-[10px] font-medium tracking-wide tabular-nums">
              MITM Control · v{APP_VERSION}
            </span>
            <span className="hidden h-1 w-1 rounded-full bg-black/20 dark:bg-white/20 md:block" />
            <span className="hidden truncate text-[11px] font-medium md:block">
              {APP_PRODUCT_TAGLINE}
            </span>
          </div>
        </div>
      </div>

      <div className="no-drag-region flex min-w-0 items-center justify-end gap-2">
        <div
          className={[
            "hidden min-w-[240px] max-w-[360px] items-center gap-3 rounded-[18px] border px-3.5 py-2 shadow-[0_8px_22px_rgba(15,23,42,0.05)] lg:flex",
            sessionStateTone,
          ].join(" ")}
        >
          <div
            className={[
              "flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl",
              status?.running
                ? "bg-emerald-500/12 text-emerald-600 dark:text-emerald-300"
                : "bg-black/[0.05] text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark",
            ].join(" ")}
          >
            <RadioTower className="h-4 w-4" strokeWidth={2.4} />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="truncate text-[10px] font-bold uppercase tracking-[0.16em]">
                {sessionStateLabel}
              </span>
            </div>
            <div
              className="mt-1 truncate text-[12px] font-semibold text-ios-text dark:text-ios-textDark"
              title={onlineEmailFull || ""}
            >
              {onlineEmail}
            </div>
          </div>
          <span
            className={[
              "hidden shrink-0 rounded-full px-2 py-1 text-[10px] font-bold tracking-wide xl:inline-flex",
              status?.running
                ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
                : "bg-black/[0.05] text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark",
            ].join(" ")}
            title={onlineEmailFull || onlineSummary}
          >
            {onlineSummary}
          </span>
        </div>

        <div
          className="flex min-w-0 max-w-[220px] items-center gap-2 rounded-full border border-black/[0.06] bg-black/[0.03] px-3 py-1.5 text-[11px] font-medium text-ios-textSecondary dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textSecondaryDark lg:hidden"
          title={onlineEmailFull || ""}
        >
          <span
            className={[
              "h-2 w-2 shrink-0 rounded-full",
              status?.running ? "bg-emerald-500" : "bg-slate-400 dark:bg-slate-500",
            ].join(" ")}
          />
          <span className="truncate">{onlineEmail}</span>
        </div>

        {isPinned ? (
          <div
            className="no-drag-region hidden sm:flex items-center gap-1.5 rounded-full bg-amber-500/15 border border-amber-500/30 px-3 py-1.5 text-[11px] font-bold text-amber-700 dark:text-amber-300"
            title={`已锁定: ${pinnedLabel} — 自动切换全部暂停`}
          >
            <Lock className="h-3 w-3" strokeWidth={2.6} />
            <span className="truncate max-w-[120px]">{pinnedLabel}</span>
            <button
              type="button"
              className="rounded-full bg-amber-500 px-2 py-0.5 text-[10px] font-black text-white hover:bg-amber-600 transition-colors"
              onClick={handleUnpinFromHeader}
            >
              解锁
            </button>
          </div>
        ) : null}

        {activeApiKey ? (
          <button
            type="button"
            className="no-drag-region hidden md:flex h-9 w-9 items-center justify-center rounded-full border border-black/[0.06] bg-white/70 text-ios-text shadow-sm transition-colors hover:bg-black/5 dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textDark dark:hover:bg-white/10"
            title="复制当前活跃账号的 sk-ws- API Key"
            onClick={handleCopyActiveKey}
          >
            <Copy className="w-[16px] h-[16px]" strokeWidth={2.4} />
          </button>
        ) : null}

        <button
          type="button"
          className="flex h-9 w-9 items-center justify-center rounded-full border border-black/[0.06] bg-white/70 text-ios-text shadow-sm transition-colors hover:bg-black/5 dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textDark dark:hover:bg-white/10"
          title={themeLabel(themeMode)}
          aria-label={`主题：${themeLabel(themeMode)}，点击切换`}
          onClick={cycleTheme}
        >
          <ThemeIcon className="w-[18px] h-[18px]" strokeWidth={2.5} />
        </button>
      </div>
    </header>
  );
}
