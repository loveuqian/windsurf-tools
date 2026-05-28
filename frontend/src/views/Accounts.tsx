import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowRightLeft,
  BarChart3,
  CalendarDays,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  CheckSquare,
  Download,
  KeyRound,
  LayoutGrid,
  List,
  Lock,
  LogIn,
  MoreHorizontal,
  Plus,
  RefreshCcw,
  Search,
  ShieldAlert,
  ShieldCheck,
  Shuffle,
  Sparkles,
  Square,
  Trash2,
  Unlock,
  UserX,
  Users,
  Wallet,
  X,
} from "lucide-react";
import { APIInfo } from "../api/wails";
import F7Banner from "../components/F7Banner";
import AccountCardSkeleton from "../components/accounts/AccountCardSkeleton";
import PlanFilterChips from "../components/accounts/PlanFilterChips";
import RotationPoolStatusCard from "../components/RotationPoolStatusCard";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton";
import {
  openContextMenu,
  type ContextMenuItem,
} from "../components/ios/IContextMenu";
import IDropdownMenu, {
  type DropdownItem,
} from "../components/ios/IDropdownMenu";
import ISelectSheet from "../components/ios/ISelectSheet";
import { useSmartFriend } from "../hooks/useSmartFriend";
import { useAccountStore } from "../stores/useAccountStore";
import { useMainViewStore } from "../stores/useMainViewStore";
import { useMitmStatusStore } from "../stores/useMitmStatusStore";
import { useSettingsStore } from "../stores/useSettingsStore";
import { useTaskStore } from "../stores/useTaskStore";
import {
  getPlanTone,
  isQuotaDepleted,
  isWeeklyQuotaBlocked,
} from "../utils/account";
import { PRIMARY_POOL_LABEL } from "../utils/appMode";
import {
  formatDateTimeAsiaShanghai,
  formatResetCountdownZH,
} from "../utils/datetimeAsia";
import {
  computeMitmPoolStatus,
  type MitmPoolStatus,
} from "../utils/mitmPoolStatus";
import {
  formatSwitchPlanFilterSummary,
  normalizeSwitchPlanFilter,
  SWITCH_PLAN_FILTER_TONES,
  type SwitchPlanTone,
} from "../utils/settingsModel";
import {
  confirmDialog,
  showErrorToast,
  showToast,
} from "../utils/toast";
import { models } from "../../wailsjs/go/models";

const PLAN_SECTION_ORDER = [
  "pro",
  "max",
  "team",
  "enterprise",
  "trial",
  "free",
  "unknown",
] as const;

const PLAN_SECTION_LABELS: Record<string, string> = {
  pro: "Pro",
  max: "Max / Ultimate",
  team: "Teams",
  enterprise: "Enterprise",
  trial: "Trial",
  free: "Free",
  unknown: "未识别",
};

const PLAN_TONE_LABELS: Record<string, string> = {
  pro: "Pro",
  trial: "Trial",
  free: "Free",
  max: "Max",
  team: "Teams",
  enterprise: "Enterprise",
  unknown: "未知",
};

const ACCOUNT_SORT_OPTIONS = [
  { value: "group", label: "按分组（默认）", description: "Pro / Trial / Free 等套餐归类" },
  { value: "name", label: "按邮箱 A→Z", description: "字典序排列" },
  { value: "quota", label: "按日剩余额度 ↑", description: "见底的排在最前" },
];

const PAGE_SIZE_OPTIONS = [
  { value: 30, label: "30 / 页" },
  { value: 60, label: "60 / 页" },
  { value: 120, label: "120 / 页" },
  { value: 300, label: "300 / 页" },
];

type AccountQuickFilter =
  | "all"
  | "online"
  | "switchable"
  | "depleted"
  | "runtime_exhausted"
  | "low"
  | "pending"
  | "credential_gap";

type CardStateTone = "online" | "ready" | "warning" | "danger" | "pending";

const PANEL_TONE_CLASS: Record<CardStateTone, string> = {
  online: "border-emerald-500/15 bg-emerald-500/[0.07]",
  ready: "border-ios-blue/15 bg-ios-blue/[0.06]",
  warning: "border-amber-500/15 bg-amber-500/[0.07]",
  danger: "border-rose-500/15 bg-rose-500/[0.07]",
  pending:
    "border-black/[0.08] bg-black/[0.03] dark:border-white/[0.08] dark:bg-white/[0.04]",
};

const PLAN_ACCENT_CLASS: Record<string, string> = {
  pro: "from-ios-blue via-sky-400 to-cyan-300",
  max: "from-violet-500 via-fuchsia-400 to-rose-300",
  team: "from-indigo-500 via-blue-400 to-cyan-300",
  enterprise: "from-slate-600 via-slate-500 to-slate-300",
  trial: "from-amber-500 via-orange-400 to-yellow-300",
  free: "from-slate-400 via-slate-300 to-slate-200",
  unknown: "from-gray-400 via-gray-300 to-gray-200",
};

function parseQuotaWidth(str: string): string {
  const n = parseFloat(String(str).replace("%", "").trim());
  if (!Number.isFinite(n) || n < 0) return "0%";
  if (n > 100) return "100%";
  return `${n}%`;
}

/** micros(百万分之一美元)→ "$x.xx" 显示串。 */
function formatExtraUsageBalance(micros?: number): string {
  const v = (micros ?? 0) / 1_000_000;
  const sign = v < 0 ? "-" : "";
  return `${sign}$${Math.abs(v).toFixed(2)}`;
}

/**
 * Accounts view — Vue 1:1 完整迁移。
 *
 * 主要功能：
 * - 顶部工具栏：批量导入 / 下一席位 / 批量管理 dropdown
 * - plan 类型 tabs + quick filter chips + 搜索 + 排序 + 分组操作
 * - 卡片网格：plan badge / state chip / 操作 / 进度条 / pin/pool 徽章
 * - 分页
 * - 空状态 3 步 onboarding
 */
export default function Accounts() {
  // C-2 性能：使用 selector 分别订阅字段，避免任何 store 任何字段变化
  // 都触发这个 1470 行组件整棵重渲染（旧实现 polling 8s/15s 、settings
  // autosave、accounts 刷新都会全棵重渲）。action 走 getState() 不订阅。
  const accounts = useAccountStore((s) => s.accounts);
  const accountActionLoading = useAccountStore((s) => s.actionLoading);
  const accountHasLoadedOnce = useAccountStore((s) => s.hasLoadedOnce);
  const status = useMitmStatusStore((s) => s.status);
  const switchLoading = useMitmStatusStore((s) => s.switchLoading);
  const switchTargetAccountId = useMitmStatusStore(
    (s) => s.switchTargetAccountId,
  );
  const settings = useSettingsStore((s) => s.settings);
  const sf = useSmartFriend();

  // 1.6: ImportModal 已提升为全局资源（App.tsx 挂载），这里只调 store 控制。
  const openImportModal = useMainViewStore((s) => s.openImportModal);
  // 1.5: 跨视图跳转高亮某账号卡（Header 当前活跃 Key 卡点击触发）。
  const highlightAccountId = useMainViewStore((s) => s.highlightAccountId);
  const clearHighlight = useMainViewStore((s) => s.clearHighlight);
  const highlightCardRef = useRef<HTMLDivElement | null>(null);
  const [quotaRefreshingIds, setQuotaRefreshingIds] = useState<Set<string>>(
    new Set(),
  );
  const [searchQuery, setSearchQuery] = useState("");
  const [activeTab, setActiveTab] = useState<string>("all");
  const [quickFilter, setQuickFilter] = useState<AccountQuickFilter>("all");
  const [accountSort, setAccountSort] = useState<
    "group" | "name" | "quota"
  >("group");
  const [pageSize, setPageSize] = useState<number>(60);
  const [currentPage, setCurrentPage] = useState(1);
  const [planGroupFilter, setPlanGroupFilter] = useState<string>("");
  // 多选批量操作:选中账号 id 集合 + 视图模式
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [viewMode, setViewMode] = useState<"card" | "table">(() => {
    try {
      return localStorage.getItem("wt_accounts_view") === "table"
        ? "table"
        : "card";
    } catch {
      return "card";
    }
  });
  // D-B: 顶部 MITM 池准入 chip 组的折叠状态 + 保存中标志
  const [planPoolExpanded, setPlanPoolExpanded] = useState(false);
  const [planPoolSaving, setPlanPoolSaving] = useState(false);

  // bootstrap：加载账号 / mitm 状态 / 设置；12s 后再 force 刷新
  const bootstrapTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    void useAccountStore.getState().ensureAccountsLoaded();
    void useMitmStatusStore.getState().ensureStatusLoaded();
    void useSettingsStore.getState().fetchSettings();
    bootstrapTimer.current = setTimeout(() => {
      void Promise.all([
        useAccountStore.getState().fetchAccounts(true),
        useMitmStatusStore.getState().fetchStatus(true),
      ]);
    }, 12_000);
    return () => {
      if (bootstrapTimer.current) clearTimeout(bootstrapTimer.current);
    };
  }, []);

  // 筛选/搜索变化时重置到第一页
  useEffect(() => {
    setCurrentPage(1);
  }, [activeTab, quickFilter, searchQuery, accountSort]);

  // 记住卡片/表格视图选择
  useEffect(() => {
    try {
      localStorage.setItem("wt_accounts_view", viewMode);
    } catch {
      /* ignore */
    }
  }, [viewMode]);

  // 1.5: 跨视图跳转 → 清筛选保证目标卡可见 + 滚动 + 1.6s 后清 store。
  useEffect(() => {
    if (!highlightAccountId) return;
    // 清筛选回第一页，确保目标卡一定在页面上（不被过滤 / 不在第二页）。
    setActiveTab("all");
    setQuickFilter("all");
    setSearchQuery("");
    setCurrentPage(1);

    // 等下一帧渲染完，scrollIntoView。
    const scrollTimer = setTimeout(() => {
      highlightCardRef.current?.scrollIntoView({
        behavior: "smooth",
        block: "center",
      });
    }, 100);
    // 1.6s 后清 highlight，让 ring 动画自然消失。
    const clearTimer = setTimeout(() => clearHighlight(), 1600);
    return () => {
      clearTimeout(scrollTimer);
      clearTimeout(clearTimer);
    };
  }, [highlightAccountId, clearHighlight]);

  // ── 单次遍历聚合：tab 计数 + free 计数 ──────────────────────────────
  const accountAgg = useMemo(() => {
    const counts: Partial<Record<SwitchPlanTone, number>> = {};
    for (const t of SWITCH_PLAN_FILTER_TONES) counts[t] = 0;
    let freeCount = 0;
    for (const a of accounts) {
      const tone = getPlanTone(a.plan_name) as SwitchPlanTone;
      counts[tone] = (counts[tone] ?? 0) + 1;
      if (tone === "free") freeCount++;
    }
    return { counts, freeCount };
  }, [accounts]);

  const tabsList = useMemo(() => {
    const c = accountAgg.counts;
    const tabs = PLAN_SECTION_ORDER.filter(
      (k) => (c[k as SwitchPlanTone] ?? 0) > 0,
    ).map((key) => ({
      key,
      label: PLAN_SECTION_LABELS[key] || key,
      count: c[key as SwitchPlanTone] ?? 0,
    }));
    return [
      { key: "all", label: "全部", count: accounts.length },
      ...tabs,
    ];
  }, [accountAgg, accounts.length]);

  const freePlanAccountCount = accountAgg.freeCount;

  // ── 卡片状态 / 颜色 helper ─────────────────────────────────────────
  const findMitmPoolRuntime = (acc: models.Account) => {
    const key = String(acc.windsurf_api_key || "").trim();
    const email = String(acc.email || "")
      .trim()
      .toLowerCase();
    if (!key) return null;
    return (
      status?.pool_status?.find((item) => {
        const itemEmail = String(item.email || "")
          .trim()
          .toLowerCase();
        if (email && itemEmail) return email === itemEmail;
        const short = String(item.key_short || "").trim();
        return short && short === key;
      }) ?? null
    );
  };

  const isCurrentOnline = (acc: models.Account) =>
    Boolean(findMitmPoolRuntime(acc)?.is_current);
  const getBoundSessionCount = (acc: models.Account) =>
    findMitmPoolRuntime(acc)?.bound_session_count ?? 0;

  const isWeeklyBlockedDisplay = (acc: models.Account) =>
    !sf.active && isWeeklyQuotaBlocked(acc);

  const isExpiredAccount = (acc: models.Account) => {
    const status = String(acc.status || "").toLowerCase();
    if (status === "disabled" || status === "expired") return true;
    if (!acc.subscription_expires_at) return false;
    const ts = Date.parse(acc.subscription_expires_at);
    return Number.isFinite(ts) && ts < Date.now();
  };

  const getCardStateMeta = (
    acc: models.Account,
  ): { tone: CardStateTone; label: string } => {
    const sfActive = sf.active;
    const mitmRuntime = findMitmPoolRuntime(acc);
    if (!sfActive && mitmRuntime?.runtime_exhausted) {
      return {
        tone: "danger",
        label: isCurrentOnline(acc) ? "当前活跃 · 运行时见底" : "运行时见底",
      };
    }
    if (isCurrentOnline(acc)) {
      return { tone: "online", label: sfActive ? "当前活跃 · F7" : "当前活跃" };
    }
    if (isExpiredAccount(acc)) {
      return { tone: "danger", label: "已过期" };
    }
    const daily = parseFloat(
      String(acc.daily_remaining || "")
        .replace("%", "")
        .trim(),
    );
    const weekly = parseFloat(
      String(acc.weekly_remaining || "")
        .replace("%", "")
        .trim(),
    );
    const dailyKnown = Number.isFinite(daily);
    const weeklyKnown = Number.isFinite(weekly);
    if (!sfActive) {
      const weeklyBlocked = isWeeklyQuotaBlocked(acc);
      const exhausted = isQuotaDepleted(acc);
      const lowQuota =
        (dailyKnown && daily > 0 && daily < 20) ||
        (weeklyKnown && weekly > 0 && weekly < 20);
      if (weeklyBlocked) return { tone: "danger", label: "周限不可用" };
      if (exhausted) return { tone: "danger", label: "额度见底" };
      if (lowQuota) return { tone: "warning", label: "额度偏低" };
    }
    if (!acc.windsurf_api_key) {
      return { tone: "pending", label: "待补 API Key" };
    }
    if (
      !acc.subscription_expires_at &&
      !dailyKnown &&
      !weeklyKnown &&
      !acc.last_quota_update
    ) {
      return { tone: "pending", label: "待同步" };
    }
    return { tone: "ready", label: sfActive ? "F7 · 已绕过额度" : "可参与轮换" };
  };

  const getQuotaColor = (str: string) => {
    if (sf.active) return "bg-ios-green";
    const n = parseFloat(String(str).replace("%", "").trim());
    if (!Number.isFinite(n)) return "bg-gray-400";
    if (n > 50) return "bg-ios-green";
    if (n > 20) return "bg-yellow-500";
    return "bg-ios-red";
  };

  const getPlanAccentClass = (acc: models.Account) =>
    PLAN_ACCENT_CLASS[getPlanTone(acc.plan_name) as string] ??
    PLAN_ACCENT_CLASS.unknown;

  const hasApiKey = (acc: models.Account) =>
    Boolean(String(acc.windsurf_api_key || "").trim());

  const matchesQuickFilter = (
    acc: models.Account,
    filter: AccountQuickFilter,
  ) => {
    const meta = getCardStateMeta(acc);
    switch (filter) {
      case "online":
        return meta.tone === "online";
      case "switchable":
        return meta.tone === "online" || meta.tone === "ready";
      case "depleted":
        return !sf.active && meta.tone === "danger";
      case "runtime_exhausted":
        return (
          !sf.active && Boolean(findMitmPoolRuntime(acc)?.runtime_exhausted)
        );
      case "low":
        return meta.tone === "warning";
      case "pending":
        return meta.tone === "pending";
      case "credential_gap":
        return !hasApiKey(acc);
      default:
        return true;
    }
  };

  // ── 筛选 / 排序 / 分页 ─────────────────────────────────────────────
  const filteredAccounts = useMemo(() => {
    let list = accounts;
    if (activeTab !== "all") {
      list = list.filter((a) => getPlanTone(a.plan_name) === activeTab);
    }
    if (quickFilter !== "all") {
      list = list.filter((a) => matchesQuickFilter(a, quickFilter));
    }
    const q = searchQuery.trim().toLowerCase();
    if (!q) return list;
    return list.filter(
      (a) =>
        (a.email?.toLowerCase().includes(q) ?? false) ||
        (a.nickname?.toLowerCase().includes(q) ?? false) ||
        (a.remark?.toLowerCase().includes(q) ?? false) ||
        (a.plan_name?.toLowerCase().includes(q) ?? false),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accounts, activeTab, quickFilter, searchQuery, sf.active]);

  const displayAccounts = useMemo(() => {
    const items = [...filteredAccounts];
    if (accountSort === "group") {
      const orderMap = new Map<string, number>();
      PLAN_SECTION_ORDER.forEach((p, i) => orderMap.set(p, i));
      const fallback = PLAN_SECTION_ORDER.length;
      const toneCache = new Map<string, number>();
      const getToneIdx = (plan: string) => {
        let idx = toneCache.get(plan);
        if (idx === undefined) {
          idx = orderMap.get(getPlanTone(plan)) ?? fallback;
          toneCache.set(plan, idx);
        }
        return idx;
      };
      items.sort((a, b) => {
        const d = getToneIdx(a.plan_name || "") - getToneIdx(b.plan_name || "");
        return d !== 0
          ? d
          : (a.email || "").localeCompare(b.email || "", "zh-CN");
      });
    } else if (accountSort === "name") {
      items.sort((a, b) =>
        (a.email || "").localeCompare(b.email || "", "zh-CN"),
      );
    } else if (accountSort === "quota") {
      items.sort((a, b) => {
        const pa =
          parseFloat(String(a.daily_remaining || "").replace("%", "")) || 0;
        const pb =
          parseFloat(String(b.daily_remaining || "").replace("%", "")) || 0;
        return pa - pb;
      });
    }
    return items;
  }, [filteredAccounts, accountSort]);

  const totalPages = Math.max(
    1,
    Math.ceil(displayAccounts.length / pageSize),
  );
  const pagedAccounts = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return displayAccounts.slice(start, start + pageSize);
  }, [displayAccounts, currentPage, pageSize]);
  const paginationRange = useMemo(() => {
    const total = totalPages;
    const cur = currentPage;
    const maxButtons = 7;
    if (total <= maxButtons) {
      return Array.from({ length: total }, (_, i) => i + 1);
    }
    let start = Math.max(1, cur - Math.floor(maxButtons / 2));
    let end = start + maxButtons - 1;
    if (end > total) {
      end = total;
      start = end - maxButtons + 1;
    }
    return Array.from({ length: end - start + 1 }, (_, i) => start + i);
  }, [totalPages, currentPage]);

  // ── quick filter 单次遍历计数 ──────────────────────────────────────
  const quickFilterOptions = useMemo<
    Array<{ key: AccountQuickFilter; label: string; count: number }>
  >(() => {
    const counts: Record<Exclude<AccountQuickFilter, "all">, number> = {
      online: 0,
      switchable: 0,
      depleted: 0,
      runtime_exhausted: 0,
      low: 0,
      pending: 0,
      credential_gap: 0,
    };
    for (const acc of accounts) {
      const meta = getCardStateMeta(acc);
      const runtimeExhausted =
        !sf.active && Boolean(findMitmPoolRuntime(acc)?.runtime_exhausted);
      if (meta.tone === "online") counts.online++;
      if (meta.tone === "online" || meta.tone === "ready") counts.switchable++;
      if (!sf.active && meta.tone === "danger") counts.depleted++;
      if (runtimeExhausted) counts.runtime_exhausted++;
      if (meta.tone === "warning") counts.low++;
      if (meta.tone === "pending") counts.pending++;
      if (!hasApiKey(acc)) counts.credential_gap++;
    }
    return [
      { key: "all", label: "全部", count: accounts.length },
      { key: "online", label: "当前活跃", count: counts.online },
      { key: "switchable", label: "可切换", count: counts.switchable },
      { key: "depleted", label: "额度见底", count: counts.depleted },
      {
        key: "runtime_exhausted",
        label: "运行时见底",
        count: counts.runtime_exhausted,
      },
      { key: "low", label: "额度偏低", count: counts.low },
      { key: "pending", label: "待同步", count: counts.pending },
      { key: "credential_gap", label: "待补 API Key", count: counts.credential_gap },
    ];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accounts, status, sf.active]);

  const hasListFilters =
    activeTab !== "all" ||
    quickFilter !== "all" ||
    Boolean(searchQuery.trim());
  const clearListFilters = () => {
    setActiveTab("all");
    setQuickFilter("all");
    setSearchQuery("");
  };

  // ── 操作 helpers ───────────────────────────────────────────────────
  const handleDelete = async (id: string) => {
    const ok = await confirmDialog("是否确认移除该账号？", {
      confirmText: "移除",
      cancelText: "取消",
      destructive: true,
    });
    if (!ok) return;
    try {
      await useAccountStore.getState().deleteAccount(id);
      showToast("账号已移除", "success");
    } catch (e) {
      showErrorToast(e, "移除失败");
    }
  };

  const handleCleanExpired = async () => {
    try {
      const n = await useAccountStore.getState().cleanExpiredAccounts();
      showToast(`已清理 ${n} 个过期账号`, "success");
    } catch (e) {
      showErrorToast(e, "清理失败");
    }
  };

  const handleUnlockAllExhausted = async () => {
    try {
      const n = await APIInfo.clearAllMitmExhausted();
      if (n > 0) {
        showToast(`已解锁 ${n} 个号`, "success");
      } else {
        showToast("当前没有被锁定的号", "info");
      }
    } catch (e) {
      showErrorToast(e, "解锁失败");
    }
  };

  const handleUnlockOne = async (apiKey: string, label: string) => {
    if (!apiKey) {
      showToast("此号没有 API Key，无法解锁", "warning");
      return;
    }
    try {
      const ok = await APIInfo.clearMitmKeyExhausted(apiKey);
      if (ok) {
        showToast(`已解锁 ${label}`, "success");
      } else {
        showToast(`${label} 当前未被锁定`, "info");
      }
    } catch (e) {
      showErrorToast(e, "解锁失败");
    }
  };

  const handleDeleteFreePlans = async () => {
    const n = freePlanAccountCount;
    if (n === 0) {
      showToast("当前没有 Free 计划的账号", "info");
      return;
    }
    const ok = await confirmDialog(
      `将永久删除 ${n} 个免费计划账号，不可恢复。`,
      { confirmText: "删除", cancelText: "取消", destructive: true },
    );
    if (!ok) return;
    try {
      const deleted = await useAccountStore
        .getState()
        .deleteFreePlanAccounts();
      showToast(`已删除 ${deleted} 个免费账号`, "success");
    } catch (e) {
      showErrorToast(e, "删除失败");
    }
  };

  const handleRefreshTokens = async () => {
    // F1: 进度走后端 task tracker，前端只需打开 Drawer
    useTaskStore.getState().setOpen(true);
    void useTaskStore.getState().pollServer();
    try {
      const map = await useAccountStore.getState().refreshAllTokens();
      await useMitmStatusStore.getState().fetchStatus(true);
      const entries = Object.entries(map || {});
      const ok = entries.filter(([, v]) => String(v).includes("成功")).length;
      showToast(`刷新完成：${ok} / ${entries.length}`, "success");
    } catch (e) {
      showErrorToast(e, "刷新失败");
    } finally {
      void useTaskStore.getState().pollServer();
    }
  };

  const handleRefreshAllQuotas = async () => {
    // F1: 同上
    useTaskStore.getState().setOpen(true);
    void useTaskStore.getState().pollServer();
    try {
      const map = await useAccountStore.getState().refreshAllQuotas();
      await useMitmStatusStore.getState().fetchStatus(true);
      const entries = Object.entries(map || {});
      const synced = entries.filter(([, v]) =>
        String(v).includes("已同步"),
      ).length;
      showToast(`同步完成：${synced} / ${entries.length}`, "success");
    } catch (e) {
      showErrorToast(e, "同步额度失败");
    } finally {
      void useTaskStore.getState().pollServer();
    }
  };

  const handleSwitchNextSeat = async () => {
    try {
      const target = await useMitmStatusStore.getState().switchToNext();
      showToast(`MITM 已切到下一席位：${target || "已切换"}`, "success");
    } catch (e) {
      showErrorToast(e, "手动切换失败");
    }
  };

  const handleRefreshOneQuota = async (id: string, email: string) => {
    if (quotaRefreshingIds.has(id)) return;
    setQuotaRefreshingIds((prev) => new Set(prev).add(id));
    try {
      await useAccountStore.getState().refreshAccountQuota(id);
      await useAccountStore.getState().fetchAccounts(true);
      await useMitmStatusStore.getState().fetchStatus(true);
      showToast(`${email} 额度已更新`, "success");
    } catch (e) {
      showErrorToast(e, "刷新额度失败");
    } finally {
      setQuotaRefreshingIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  };

  const isAccountCardRefreshing = (id: string) => quotaRefreshingIds.has(id);

  const handleSwitchMitmToAccount = async (acc: models.Account) => {
    try {
      const target = await useMitmStatusStore
        .getState()
        .switchToAccount(acc.id);
      await useAccountStore.getState().fetchAccounts(true);
      showToast(
        `MITM 已切到：${target || acc.email || "目标账号"}`,
        "success",
      );
    } catch (e) {
      // D-D: 错误回收
      // 切号失败若是「未加入 MITM 号池」类，从 poolStatus 推断具体 reason，
      // 给 toast 配上一键 action 按钮，让用户直接修筛选 / 补 API Key / 刷新额度。
      const msg = String((e as { message?: string })?.message ?? e ?? "");
      const isPoolReject = /未加入\s*MITM\s*号池|not in MITM pool/i.test(msg);
      const ps = mitmPoolStatusMap.get(acc.id);
      if (isPoolReject && ps && !ps.inPool) {
        const baseMsg = `切到 ${acc.email || "该账号"} 失败 · ${ps.detail}`;
        if (ps.reason === "plan_filtered" && ps.accountTone) {
          const tone = ps.accountTone;
          showToast(baseMsg, "error", 8000, {
            label: ps.suggestion || `+ 加入 ${tone}`,
            onClick: () => {
              const nextSet = (() => {
                const n = normalizeSwitchPlanFilter(planFilter);
                if (n === "all") return new Set(SWITCH_PLAN_FILTER_TONES);
                return new Set(n.split(",") as SwitchPlanTone[]);
              })();
              nextSet.add(tone);
              const ordered = SWITCH_PLAN_FILTER_TONES.filter((t) =>
                nextSet.has(t),
              );
              void handlePlanFilterChange(
                normalizeSwitchPlanFilter(ordered.join(",")),
              );
            },
          });
          return;
        }
        if (ps.reason === "no_api_key") {
          showToast(baseMsg, "error", 8000, {
            label: ps.suggestion || "打开导入弹窗",
            onClick: () => openImportModal(),
          });
          return;
        }
        if (ps.reason === "quota_exhausted") {
          showToast(baseMsg, "error", 8000, {
            label: ps.suggestion || "刷新额度",
            onClick: () => {
              void handleRefreshOneQuota(acc.id, acc.email || acc.id);
            },
          });
          return;
        }
        // account_expired 等不给一键操作，仅提示
        showToast(baseMsg, "error", 6000);
        return;
      }
      showErrorToast(e, "切换到该账号失败");
    }
  };

  const handleLoginToWindsurf = async (acc: models.Account) => {
    try {
      const target = await APIInfo.switchAccountLocal(acc.id);
      await useAccountStore.getState().fetchAccounts(true);
      showToast(
        `已写入 Windsurf 本地登录态：${target || acc.email || "目标账号"}`,
        "success",
      );
    } catch (e) {
      showErrorToast(e, "写入 Windsurf 登录态失败");
    }
  };

  const isAccountSwitching = (id: string) =>
    switchLoading && switchTargetAccountId === id;

  const isAccountPinned = (acc: models.Account) =>
    settings?.manual_pin_enabled === true &&
    settings?.manual_pin_account_id === acc.id;

  const isAccountInRotationPool = (acc: models.Account) =>
    settings?.rotation_pool_enabled === true &&
    (settings?.rotation_pool_account_ids || []).includes(acc.id);

  // D-B: plan filter 写回后端
  const planFilter = settings?.auto_switch_plan_filter ?? "all";
  const handlePlanFilterChange = async (next: string) => {
    if (planPoolSaving) return;
    if (normalizeSwitchPlanFilter(next) === normalizeSwitchPlanFilter(planFilter))
      return;
    setPlanPoolSaving(true);
    try {
      await useSettingsStore.getState().saveAutoSwitchPlanFilter(next);
    } catch (e) {
      showErrorToast(e, "保存准入计划失败");
    } finally {
      setPlanPoolSaving(false);
    }
  };

  // D-B 上面 chip 组的实时统计 + D-C 卡片用的 pool 状态 map
  const planPoolStats = useMemo(() => {
    const tones = new Set<SwitchPlanTone>(
      (() => {
        const n = normalizeSwitchPlanFilter(planFilter);
        if (n === "all") return SWITCH_PLAN_FILTER_TONES;
        return n.split(",") as SwitchPlanTone[];
      })(),
    );
    let admitted = 0;
    for (const acc of accounts) {
      const tone = getPlanTone(acc.plan_name) as SwitchPlanTone;
      if (tones.has(tone)) admitted++;
    }
    return { admitted, total: accounts.length };
  }, [accounts, planFilter]);
  const mitmPoolStatusMap = useMemo(() => {
    const map = new Map<string, MitmPoolStatus>();
    for (const acc of accounts) {
      map.set(
        acc.id,
        computeMitmPoolStatus(acc, planFilter, {
          smartFriendActive: sf.active,
          mitmStatus: status,
        }),
      );
    }
    return map;
  }, [accounts, planFilter, sf.active, status]);

  const rotationPoolActive = () =>
    settings?.rotation_pool_enabled === true;

  const handleUnpinFromCard = async () => {
    try {
      await APIInfo.unpinManualAccount();
      await useSettingsStore.getState().fetchSettings(true);
      showToast("已解锁，自动切换已恢复", "success");
    } catch (e) {
      showErrorToast(e, "解锁失败");
    }
  };

  const handleTogglePoolMember = async (acc: models.Account) => {
    const s = settings;
    if (!s) return;
    const ids = [...(s.rotation_pool_account_ids || [])];
    const idx = ids.indexOf(acc.id);
    const adding = idx < 0;
    if (adding) ids.push(acc.id);
    else ids.splice(idx, 1);
    try {
      await APIInfo.updateSettings({
        ...s,
        rotation_pool_account_ids: ids,
      } as models.Settings);
      await useSettingsStore.getState().fetchSettings(true);
      showToast(
        adding ? `已加入轮换池: ${acc.email}` : `已移出轮换池: ${acc.email}`,
        "success",
      );
    } catch (e) {
      showErrorToast(e, "修改池成员失败");
    }
  };

  // ── 多选批量操作 ──
  const toggleSelected = (id: string) =>
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const clearSelection = () => setSelectedIds(new Set());

  const copyApiKey = async (acc: models.Account) => {
    const key = String(acc.windsurf_api_key || "").trim();
    if (!key) {
      showToast("该账号未配置 API Key", "warning");
      return;
    }
    try {
      await navigator.clipboard.writeText(key);
      const short =
        key.length > 16 ? `${key.slice(0, 12)}…${key.slice(-4)}` : key;
      showToast(`已复制 ${short}`, "success");
    } catch (e) {
      showErrorToast(e, "复制失败");
    }
  };

  const planGroupCount = useMemo(() => {
    if (!planGroupFilter) return 0;
    return accountAgg.counts[planGroupFilter as SwitchPlanTone] ?? 0;
  }, [planGroupFilter, accountAgg]);

  // 选中变化时,自动剔除已不在当前筛选结果里的 id(避免误删隐藏账号)
  useEffect(() => {
    if (selectedIds.size === 0) return;
    const visible = new Set(displayAccounts.map((a) => a.id));
    setSelectedIds((prev) => {
      let changed = false;
      const next = new Set<string>();
      for (const id of prev) {
        if (visible.has(id)) next.add(id);
        else changed = true;
      }
      return changed ? next : prev;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [displayAccounts]);

  const allPageSelected =
    pagedAccounts.length > 0 &&
    pagedAccounts.every((a) => selectedIds.has(a.id));
  const toggleSelectAllPage = () => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (allPageSelected) {
        for (const a of pagedAccounts) next.delete(a.id);
      } else {
        for (const a of pagedAccounts) next.add(a.id);
      }
      return next;
    });
  };

  const handleBulkDelete = async () => {
    const ids = [...selectedIds];
    if (ids.length === 0) return;
    const ok = await confirmDialog(
      `将永久删除选中的 ${ids.length} 个账号，不可恢复。`,
      { confirmText: "删除", cancelText: "取消", destructive: true },
    );
    if (!ok) return;
    let done = 0;
    for (const id of ids) {
      try {
        await useAccountStore.getState().deleteAccount(id);
        done++;
      } catch {
        /* 单个失败不中断整体 */
      }
    }
    await useAccountStore.getState().fetchAccounts(true);
    clearSelection();
    showToast(
      done === ids.length
        ? `已删除 ${done} 个账号`
        : `已删除 ${done}/${ids.length} 个，部分失败`,
      done === ids.length ? "success" : "warning",
    );
  };

  // 批量加入/移出轮换池:一次 updateSettings 写回,避免逐个请求
  const handleBulkRotationPool = async (add: boolean) => {
    const s = settings;
    if (!s) return;
    const ids = [...selectedIds];
    if (ids.length === 0) return;
    const set = new Set(s.rotation_pool_account_ids || []);
    if (add) ids.forEach((id) => set.add(id));
    else ids.forEach((id) => set.delete(id));
    try {
      await APIInfo.updateSettings({
        ...s,
        rotation_pool_account_ids: [...set],
      } as models.Settings);
      await useSettingsStore.getState().fetchSettings(true);
      showToast(
        add
          ? `已把 ${ids.length} 个账号加入轮换池`
          : `已把 ${ids.length} 个账号移出轮换池`,
        "success",
      );
      clearSelection();
    } catch (e) {
      showErrorToast(e, "批量修改轮换池失败");
    }
  };


  const planGroupOptions = useMemo(
    () => [
      { value: "" as string | number, label: "按套餐操作…" },
      ...SWITCH_PLAN_FILTER_TONES.map((tone) => ({
        value: tone as string | number,
        label: `${PLAN_TONE_LABELS[tone] ?? tone} (${
          accountAgg.counts[tone as SwitchPlanTone] ?? 0
        })`,
      })),
    ],
    [accountAgg],
  );

  const handleDeleteByPlanGroup = async () => {
    const tone = planGroupFilter;
    if (!tone) {
      showToast("请先选择套餐类型", "warning");
      return;
    }
    const cnt = planGroupCount;
    if (cnt === 0) {
      showToast(`没有 ${PLAN_TONE_LABELS[tone] ?? tone} 类型的账号`, "info");
      return;
    }
    const label = PLAN_TONE_LABELS[tone] ?? tone;
    const ok = await confirmDialog(
      `将永久删除所有「${label}」套餐的 ${cnt} 个账号，不可恢复。`,
      { confirmText: "删除", cancelText: "取消", destructive: true },
    );
    if (!ok) return;
    try {
      const n = await APIInfo.deleteAccountsByGroup(tone);
      await useAccountStore.getState().fetchAccounts(true);
      setPlanGroupFilter("");
      showToast(`已删除 ${n} 个「${label}」账号`, "success");
    } catch (e) {
      showErrorToast(e, "删除失败");
    }
  };

  const handleExportByPlanGroup = async () => {
    const tone = planGroupFilter;
    if (!tone) {
      showToast("请先选择套餐类型", "warning");
      return;
    }
    const label = PLAN_TONE_LABELS[tone] ?? tone;
    try {
      const filePath = await APIInfo.exportAccountsByGroup(tone);
      showToast(
        `「${label}」的 ${planGroupCount} 个账号已导出到：\n${filePath}`,
        "success",
        6000,
      );
    } catch (e) {
      showErrorToast(e, "导出失败");
    }
  };

  // 顶部「批量管理」dropdown
  const bulkActionItems: DropdownItem[] = [
    {
      label: "刷新所有凭证",
      icon: KeyRound,
      onClick: () => void handleRefreshTokens(),
      disabled: accountActionLoading || accounts.length === 0,
      hint: "JWT",
    },
    {
      label: "同步所有额度",
      icon: BarChart3,
      onClick: () => void handleRefreshAllQuotas(),
      disabled: accountActionLoading || accounts.length === 0,
      hint: "Quota",
    },
    {
      label: "批量解锁号池",
      icon: Unlock,
      onClick: () => void handleUnlockAllExhausted(),
      disabled: accounts.length === 0,
      hint: "Reset",
    },
    { type: "divider" },
    {
      label: "清理过期账号",
      icon: Trash2,
      onClick: () => void handleCleanExpired(),
      disabled: accounts.length === 0,
    },
    {
      label: `删除免费账号${
        freePlanAccountCount ? ` (${freePlanAccountCount})` : ""
      }`,
      icon: UserX,
      onClick: () => void handleDeleteFreePlans(),
      danger: true,
      disabled: freePlanAccountCount === 0,
    },
  ];

  // 卡片右上角「⋯」dropdown
  const accountCardMenuItems = (acc: models.Account): DropdownItem[] => {
    const items: DropdownItem[] = [
      {
        label: "刷新此账号额度",
        icon: RefreshCcw,
        onClick: () => void handleRefreshOneQuota(acc.id, acc.email),
        disabled: isAccountCardRefreshing(acc.id) || isAccountSwitching(acc.id),
      },
    ];
    if (hasApiKey(acc)) {
      items.push({
        label: "复制 API Key",
        icon: KeyRound,
        onClick: () => void copyApiKey(acc),
      });
    }
    if (rotationPoolActive()) {
      items.push({
        label: isAccountInRotationPool(acc)
          ? "移出轮换池"
          : "加入轮换池",
        icon: Shuffle,
        onClick: () => void handleTogglePoolMember(acc),
      });
    }
    if (isAccountPinned(acc)) {
      items.push({
        label: "解除锁定",
        icon: Lock,
        onClick: () => void handleUnpinFromCard(),
        hint: "Pin",
      });
    }
    items.push({ type: "divider" });
    items.push({
      label: "移除此账号",
      icon: Trash2,
      onClick: () => void handleDelete(acc.id),
      danger: true,
      disabled: isAccountSwitching(acc.id),
    });
    return items;
  };

  // 1.4: 卡片右键上下文菜单（操作比 dropdown 更全 — 包含切到此号 + 写本地登录）
  const buildAccountContextMenu = (
    acc: models.Account,
  ): ContextMenuItem[] => {
    const online = isCurrentOnline(acc);
    const switching = isAccountSwitching(acc.id);
    const items: ContextMenuItem[] = [];
    if (!online) {
      items.push({
        id: "switch-mitm",
        label: "切到此号 (MITM)",
        icon: ArrowRightLeft,
        onSelect: () => void handleSwitchMitmToAccount(acc),
        disabled: !hasApiKey(acc) || switching || switchLoading,
      });
    }
    items.push({
      id: "login-local",
      label: "写入 Windsurf 本地登录态",
      icon: LogIn,
      onSelect: () => void handleLoginToWindsurf(acc),
      disabled: switching,
    });
    if (hasApiKey(acc)) {
      items.push({
        id: "copy-key",
        label: "复制 API Key",
        icon: KeyRound,
        onSelect: () => void copyApiKey(acc),
      });
    }
    items.push({
      id: "refresh-quota",
      label: "刷新此账号额度",
      icon: RefreshCcw,
      onSelect: () => void handleRefreshOneQuota(acc.id, acc.email),
      disabled: isAccountCardRefreshing(acc.id) || switching,
    });
    if (hasApiKey(acc)) {
      items.push({
        id: "unlock-exhausted",
        label: "解锁此号 (额度耗尽)",
        icon: Unlock,
        onSelect: () =>
          void handleUnlockOne(
            String(acc.windsurf_api_key || "").trim(),
            acc.email || acc.id,
          ),
      });
    }
    if (rotationPoolActive()) {
      items.push({
        id: "toggle-pool",
        label: isAccountInRotationPool(acc)
          ? "移出轮换池"
          : "加入轮换池",
        icon: Shuffle,
        onSelect: () => void handleTogglePoolMember(acc),
      });
    }
    if (isAccountPinned(acc)) {
      items.push({
        id: "unpin",
        label: "解除锁定 (Pin)",
        icon: Lock,
        onSelect: () => void handleUnpinFromCard(),
      });
    }
    items.push({
      id: "delete",
      label: "移除此账号",
      icon: Trash2,
      onSelect: () => void handleDelete(acc.id),
      danger: true,
      divider: true,
      disabled: switching,
    });
    return items;
  };

  // 空状态 onboarding 跳转
  const goRelay = () => useMainViewStore.getState().setActiveTab("Relay");
  const handleStepImport = () => openImportModal();
  const handleStepEnableMitm = () =>
    useMainViewStore.getState().setActiveTab("Dashboard");
  const handleStepFinish = () =>
    useMainViewStore.getState().setActiveTab("Dashboard");

  return (
    <div className="p-6 md:p-8 flex flex-1 flex-col max-w-6xl mx-auto w-full min-h-0">
      {/* ── 顶部工具栏 ── */}
      <div className="flex flex-wrap items-start justify-between gap-4 mb-4 shrink-0">
        <div className="min-w-0">
          <h1 className="text-[28px] sm:text-[32px] font-bold tracking-tight">
            {PRIMARY_POOL_LABEL}
          </h1>
          <p className="text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-1 leading-relaxed max-w-[640px]">
            粘贴 API Key / JWT / 邮箱密码一键入池，MITM 代理自动接管 IDE 流量、按额度无感切号。
          </p>
          {/* F7-REMOVAL: 整行 <F7Banner/> 删除 */}
          <F7Banner variant="compact" />
        </div>

        <div className="flex flex-wrap items-center gap-2 justify-end">
          <button
            type="button"
            className="no-drag-region inline-flex items-center gap-1.5 px-5 py-2.5 bg-gradient-to-b from-[#3b82f6] to-ios-blue text-white rounded-full font-semibold text-[14px] ios-btn shadow-md ring-1 ring-black/5 whitespace-nowrap"
            onClick={openImportModal}
          >
            <Plus className="w-[18px] h-[18px]" strokeWidth={2.5} />
            批量导入
          </button>
          <button
            type="button"
            className="no-drag-region inline-flex items-center gap-1.5 px-4 py-2.5 bg-ios-blue/10 text-ios-blue dark:text-blue-300 rounded-full font-semibold text-[14px] ios-btn hover:bg-ios-blue/15 transition-colors disabled:opacity-50"
            disabled={switchLoading || !status?.pool_status?.length}
            title={
              status?.pool_status?.length
                ? "手动切到 MITM 号池中下一席位"
                : "MITM 号池为空，先导入带 API Key 的账号"
            }
            onClick={handleSwitchNextSeat}
          >
            <ArrowRightLeft
              className={`w-[18px] h-[18px] ${
                switchLoading && !switchTargetAccountId ? "animate-pulse" : ""
              }`}
              strokeWidth={2.5}
            />
            下一席位
          </button>
          <IDropdownMenu
            items={bulkActionItems}
            align="right"
            width="w-60"
            triggerLabel="批量管理"
            triggerIcon={MoreHorizontal}
            disabled={accounts.length === 0}
            triggerTitle="批量管理：全量刷新 / 同步 / 清理"
          />
        </div>
      </div>

      {/* ── plan tabs ── */}
      <div className="flex items-center gap-2 mb-3 overflow-x-auto no-scrollbar shrink-0 pb-1">
        {tabsList.map((tab) => {
          const active = activeTab === tab.key;
          return (
            <button
              key={tab.key}
              type="button"
              className={`no-drag-region flex items-center gap-2 px-4 py-2 rounded-full font-bold text-[14px] transition-all whitespace-nowrap ${
                active
                  ? "bg-ios-text text-white dark:bg-white dark:text-black shadow-md"
                  : "bg-black/5 dark:bg-white/5 text-ios-textSecondary dark:text-ios-textSecondaryDark hover:bg-black/10 dark:hover:bg-white/10"
              }`}
              onClick={() => setActiveTab(tab.key)}
            >
              {tab.label}
              <span
                className={`px-2 py-0.5 rounded-full text-[11px] font-bold ${
                  active
                    ? "bg-white/20 dark:bg-black/10"
                    : "bg-black/5 dark:bg-white/10"
                }`}
              >
                {tab.count}
              </span>
            </button>
          );
        })}
      </div>

      {/* ── P2: RotationPool 状态卡（仅 enabled 时显示） ── */}
      <div className="mb-3 shrink-0">
        <RotationPoolStatusCard />
      </div>

      {/* ── D-B: MITM 号池准入计划 chip 组（默认折叠，summary 行可展开） ── */}
      {accounts.length > 0 ? (
        <div className="mb-3 shrink-0">
          <button
            type="button"
            onClick={() => setPlanPoolExpanded((v) => !v)}
            className="no-drag-region inline-flex items-center gap-2 rounded-full bg-black/[0.04] px-3 py-1.5 text-[12px] font-bold text-ios-textSecondary dark:text-ios-textSecondaryDark hover:bg-black/[0.08] dark:bg-white/[0.06]  dark:hover:bg-white/[0.1]"
            title="点击折叠/展开 MITM 准入计划 chip"
          >
            <ShieldCheck className="h-3.5 w-3.5" strokeWidth={2.6} />
            <span>📥 进 MITM 号池的套餐</span>
            <span className="rounded-full bg-ios-blue/15 px-2 py-0.5 text-[10px] font-black text-ios-blue">
              {planPoolStats.admitted} / {planPoolStats.total}
            </span>
            <span className="text-ios-textSecondary/80 dark:text-ios-textSecondaryDark/80">
              · {formatSwitchPlanFilterSummary(planFilter)}
            </span>
            {planPoolExpanded ? (
              <ChevronUp className="h-3.5 w-3.5" strokeWidth={2.6} />
            ) : (
              <ChevronDown className="h-3.5 w-3.5" strokeWidth={2.6} />
            )}
          </button>
          {planPoolExpanded ? (
            <div className="mt-2">
              <PlanFilterChips
                filter={planFilter}
                onChange={handlePlanFilterChange}
                counts={accountAgg.counts}
                disabled={planPoolSaving}
              />
              <div className="mt-1.5 px-1 text-[11px] text-ios-textSecondary/80 dark:text-ios-textSecondaryDark/80">
                所选 plan 类型的账号会被纳入「下一席位」候选与 MITM 号池。变更立即生效。
              </div>
            </div>
          ) : null}
        </div>
      ) : null}

      {/* ── quick filter chips ── */}
      {accounts.length > 0 ? (
        <div className="mb-3 flex flex-wrap items-center gap-2 shrink-0">
          {quickFilterOptions.map((item) => {
            const active = quickFilter === item.key;
            return (
              <button
                key={item.key}
                type="button"
                className={`no-drag-region inline-flex items-center gap-2 rounded-full px-3.5 py-2 text-[12px] font-bold transition-colors ${
                  active
                    ? "bg-ios-blue text-white shadow-sm"
                    : "bg-black/[0.04] text-ios-textSecondary dark:text-ios-textSecondaryDark hover:bg-black/[0.08] dark:bg-white/[0.05]  dark:hover:bg-white/[0.1]"
                }`}
                onClick={() => setQuickFilter(item.key)}
              >
                <span>{item.label}</span>
                <span
                  className={`rounded-full px-2 py-0.5 text-[10px] font-black ${
                    active
                      ? "bg-white/20 text-white"
                      : "bg-black/[0.05] text-ios-textSecondary dark:text-ios-textSecondaryDark dark:bg-white/[0.08] "
                  }`}
                >
                  {item.count}
                </span>
              </button>
            );
          })}
        </div>
      ) : null}

      {/* ── 搜索 + 排序 + 分组操作 ── */}
      {accounts.length > 0 ? (
        <div className="flex flex-col sm:flex-row sm:flex-wrap gap-3 mb-3 shrink-0 max-w-6xl">
          <div className="relative flex-1 min-w-[220px]">
            <Search
              className="absolute left-3.5 top-1/2 -translate-y-1/2 w-[18px] h-[18px] text-ios-textSecondary dark:text-ios-textSecondaryDark opacity-60 pointer-events-none"
              strokeWidth={2.4}
            />
            <input
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              type="search"
              placeholder="搜索邮箱、昵称、备注、计划…"
              className="no-drag-region w-full pl-11 pr-10 py-2.5 rounded-[14px] bg-black/[0.04] border border-black/[0.06] text-[14px] outline-none focus:ring-2 focus:ring-ios-blue/25 dark:bg-white/[0.06] dark:border-white/[0.08] dark:text-gray-100"
            />
            {searchQuery ? (
              <button
                type="button"
                className="no-drag-region absolute right-2 top-1/2 -translate-y-1/2 w-8 h-8 rounded-full hover:bg-black/10 text-ios-textSecondary dark:text-ios-textSecondaryDark"
                onClick={() => setSearchQuery("")}
              >
                <X className="w-4 h-4 mx-auto" strokeWidth={2.5} />
              </button>
            ) : null}
          </div>
          <div className="flex items-center gap-2">
          <ISelectSheet
            modelValue={accountSort}
            onValueChange={(v) =>
              setAccountSort(v as "group" | "name" | "quota")
            }
            options={ACCOUNT_SORT_OPTIONS}
            title="账号排序方式"
            width="w-44"
          />
          {/* 视图切换:卡片 / 表格 */}
          <div className="no-drag-region inline-flex items-center rounded-[14px] border border-black/[0.06] bg-black/[0.04] p-0.5 dark:border-white/[0.08] dark:bg-white/[0.06]">
            <button
              type="button"
              title="卡片视图"
              onClick={() => setViewMode("card")}
              className={`inline-flex h-9 w-9 items-center justify-center rounded-[11px] transition-colors ${
                viewMode === "card"
                  ? "bg-white text-ios-blue shadow-sm dark:bg-white/15 dark:text-blue-300"
                  : "text-ios-textSecondary dark:text-ios-textSecondaryDark"
              }`}
            >
              <LayoutGrid className="h-[18px] w-[18px]" strokeWidth={2.4} />
            </button>
            <button
              type="button"
              title="表格视图(紧凑)"
              onClick={() => setViewMode("table")}
              className={`inline-flex h-9 w-9 items-center justify-center rounded-[11px] transition-colors ${
                viewMode === "table"
                  ? "bg-white text-ios-blue shadow-sm dark:bg-white/15 dark:text-blue-300"
                  : "text-ios-textSecondary dark:text-ios-textSecondaryDark"
              }`}
            >
              <List className="h-[18px] w-[18px]" strokeWidth={2.4} />
            </button>
          </div>
          </div>
          <div
            className="flex items-center gap-1.5 sm:ml-auto"
            title="先选套餐类型，再按删除 / 导出该组"
          >
            <ISelectSheet
              modelValue={planGroupFilter}
              onValueChange={(v) => setPlanGroupFilter(String(v))}
              options={planGroupOptions}
              title="选择套餐类型"
              width="w-44"
            />
            {planGroupFilter ? (
              <>
                <button
                  type="button"
                  className="no-drag-region inline-flex items-center gap-1 px-3 py-2 bg-ios-red/10 text-ios-red rounded-full font-semibold text-[12px] ios-btn hover:bg-ios-red/20 transition-colors"
                  title={`删除所有「${
                    PLAN_TONE_LABELS[planGroupFilter] ?? planGroupFilter
                  }」账号`}
                  onClick={handleDeleteByPlanGroup}
                >
                  <Trash2 className="w-[14px] h-[14px]" strokeWidth={2.5} />
                  删除该组
                </button>
                <button
                  type="button"
                  className="no-drag-region inline-flex items-center gap-1 px-3 py-2 bg-violet-500/10 text-violet-700 dark:text-violet-300 rounded-full font-semibold text-[12px] ios-btn hover:bg-violet-500/20 transition-colors"
                  title={`导出「${
                    PLAN_TONE_LABELS[planGroupFilter] ?? planGroupFilter
                  }」账号`}
                  onClick={handleExportByPlanGroup}
                >
                  <Download className="w-[14px] h-[14px]" strokeWidth={2.5} />
                  导出该组
                </button>
              </>
            ) : null}
          </div>
        </div>
      ) : null}

      {/* ── 多选批量操作栏(选中后浮现) ── */}
      {accounts.length > 0 ? (
        <div className="mb-3 flex flex-wrap items-center gap-2 shrink-0">
          <button
            type="button"
            onClick={toggleSelectAllPage}
            className="no-drag-region inline-flex items-center gap-1.5 rounded-full bg-black/[0.04] px-3 py-1.5 text-[12px] font-bold text-ios-textSecondary dark:text-ios-textSecondaryDark hover:bg-black/[0.08] dark:bg-white/[0.06] dark:hover:bg-white/[0.1]"
          >
            {allPageSelected ? (
              <CheckSquare className="h-3.5 w-3.5" strokeWidth={2.5} />
            ) : (
              <Square className="h-3.5 w-3.5" strokeWidth={2.5} />
            )}
            {allPageSelected ? "取消全选本页" : "全选本页"}
          </button>
          {selectedIds.size > 0 ? (
            <>
              <span className="text-[12px] font-bold text-ios-blue">
                已选 {selectedIds.size} 个
              </span>
              <button
                type="button"
                onClick={() => void handleBulkRotationPool(true)}
                className="no-drag-region inline-flex items-center gap-1 rounded-full bg-violet-500/10 px-3 py-1.5 text-[12px] font-bold text-violet-700 dark:text-violet-300 ios-btn hover:bg-violet-500/20 transition-colors"
              >
                🔁 加入轮换池
              </button>
              <button
                type="button"
                onClick={() => void handleBulkRotationPool(false)}
                className="no-drag-region inline-flex items-center gap-1 rounded-full bg-black/[0.04] px-3 py-1.5 text-[12px] font-bold text-ios-textSecondary dark:text-ios-textSecondaryDark ios-btn hover:bg-black/[0.08] dark:bg-white/[0.06] dark:hover:bg-white/[0.1] transition-colors"
              >
                移出轮换池
              </button>
              <button
                type="button"
                onClick={() => void handleBulkDelete()}
                className="no-drag-region inline-flex items-center gap-1 rounded-full bg-ios-red/10 px-3 py-1.5 text-[12px] font-bold text-ios-red ios-btn hover:bg-ios-red/20 transition-colors"
              >
                <Trash2 className="h-[14px] w-[14px]" strokeWidth={2.5} />
                批量删除
              </button>
              <button
                type="button"
                onClick={clearSelection}
                className="no-drag-region inline-flex items-center gap-1 rounded-full px-3 py-1.5 text-[12px] font-bold text-ios-textSecondary dark:text-ios-textSecondaryDark hover:text-ios-text dark:hover:text-ios-textDark transition-colors"
              >
                <X className="h-[14px] w-[14px]" strokeWidth={2.5} />
                取消选择
              </button>
            </>
          ) : (
            <span className="text-[11px] text-ios-textSecondary/70 dark:text-ios-textSecondaryDark/70">
              勾选卡片左上角可批量删除 / 加入轮换池
            </span>
          )}
        </div>
      ) : null}

      {/* ── 整页骨架 / 空状态 / 筛选无结果 / 卡片网格 ── */}
      {!accountHasLoadedOnce && accounts.length === 0 ? (
        <PageLoadingSkeleton variant="accounts" className="flex-1" />
      ) : accounts.length === 0 ? (
        <EmptyState
          onImport={handleStepImport}
          onEnableMitm={handleStepEnableMitm}
          onFinish={handleStepFinish}
          onRelay={goRelay}
        />
      ) : displayAccounts.length === 0 ? (
        <div className="flex flex-col items-center justify-center flex-1 py-16 text-ios-textSecondary dark:text-ios-textSecondaryDark">
          <Search className="w-12 h-12 opacity-50 mb-4" />
          <p className="text-[17px] font-medium">
            {searchQuery.trim() ? "未找到匹配的账号" : "当前筛选下没有账号"}
          </p>
          {hasListFilters ? (
            <button
              className="mt-3 text-[14px] font-semibold text-ios-blue ios-btn"
              onClick={clearListFilters}
            >
              清除筛选
            </button>
          ) : null}
        </div>
      ) : (
        <div className="pb-10 min-h-0">
          {viewMode === "table" ? (
            <AccountsTable
              accounts={pagedAccounts}
              selectedIds={selectedIds}
              onToggleSelect={toggleSelected}
              isOnline={isCurrentOnline}
              isPinned={isAccountPinned}
              inPool={isAccountInRotationPool}
              hasApiKey={hasApiKey}
              isSwitching={isAccountSwitching}
              switchLoading={switchLoading}
              onSwitch={handleSwitchMitmToAccount}
              onLogin={handleLoginToWindsurf}
              menuItems={accountCardMenuItems}
            />
          ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5 auto-rows-max">
            {pagedAccounts.map((acc) => {
              if (isAccountCardRefreshing(acc.id)) {
                return <AccountCardSkeleton key={acc.id} />;
              }
              const meta = getCardStateMeta(acc);
              const online = isCurrentOnline(acc);
              const boundCount = getBoundSessionCount(acc);
              const pinned = isAccountPinned(acc);
              const inPool = isAccountInRotationPool(acc);
              const switching = isAccountSwitching(acc.id);
              // D-C: 池外原因徽标
              const poolStatus = mitmPoolStatusMap.get(acc.id);
              const isHighlighted = highlightAccountId === acc.id;
              const selected = selectedIds.has(acc.id);
              return (
                <div
                  key={acc.id}
                  ref={isHighlighted ? highlightCardRef : undefined}
                  onContextMenu={(e) =>
                    openContextMenu(e, buildAccountContextMenu(acc))
                  }
                  className={[
                    "group bg-white dark:bg-[#1C1C1E] rounded-[22px] flex flex-col relative overflow-hidden transition-all duration-300 ease-out hover:shadow-lg hover:-translate-y-0.5",
                    selected
                      ? "border-2 border-ios-blue ring-2 ring-ios-blue/30"
                      : online
                        ? "border-2 border-ios-green/40 dark:border-ios-greenDark/40 shadow-[0_0_0_1px_rgba(52,199,89,0.12)]"
                        : "border border-black/[0.05] dark:border-white/[0.08] shadow-sm",
                    isHighlighted
                      ? "ring-4 ring-ios-blue/50 ring-offset-2 ring-offset-white dark:ring-offset-[#1C1C1E] animate-pulse"
                      : "",
                  ].join(" ")}
                >
                  {/* 多选复选框:hover 或已选时显示 */}
                  <button
                    type="button"
                    title={selected ? "取消选择" : "选择此账号"}
                    onClick={() => toggleSelected(acc.id)}
                    className={`no-drag-region absolute left-3 top-3 z-20 flex h-6 w-6 items-center justify-center rounded-md transition-all ${
                      selected
                        ? "bg-ios-blue text-white shadow-sm opacity-100"
                        : "bg-white/90 text-ios-textSecondary opacity-0 hover:bg-white group-hover:opacity-100 dark:bg-black/50 dark:text-ios-textSecondaryDark"
                    }`}
                  >
                    {selected ? (
                      <CheckSquare className="h-4 w-4" strokeWidth={2.5} />
                    ) : (
                      <Square className="h-4 w-4" strokeWidth={2.5} />
                    )}
                  </button>
                  <div
                    className={`absolute inset-x-0 top-0 h-1.5 bg-gradient-to-r opacity-95 ${getPlanAccentClass(
                      acc,
                    )}`}
                  />
                  <div className="relative z-10 flex h-full flex-col p-5">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0 flex flex-wrap items-center gap-2">
                        <span className="rounded-full bg-[#F0F5FF] px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.18em] text-ios-blue dark:bg-ios-blue/20">
                          {acc.plan_name || "unknown"}
                        </span>
                        <span
                          className={`inline-flex shrink-0 items-center whitespace-nowrap rounded-full border px-2.5 py-1 text-[10px] font-bold tracking-[0.14em] text-gray-800 dark:text-gray-100 ${PANEL_TONE_CLASS[meta.tone]}`}
                        >
                          {meta.label}
                        </span>
                        {poolStatus && !poolStatus.inPool ? (
                          <span
                            className={[
                              "inline-flex shrink-0 items-center gap-1 whitespace-nowrap rounded-full border px-2 py-1 text-[10px] font-bold",
                              poolStatus.reason === "plan_filtered"
                                ? "border-slate-400/40 bg-slate-500/10 text-slate-700 dark:text-slate-300"
                                : "border-rose-500/30 bg-rose-500/10 text-rose-700 dark:text-rose-300",
                            ].join(" ")}
                            title={`${poolStatus.detail}${poolStatus.suggestion ? ` · ${poolStatus.suggestion}` : ""}`}
                          >
                            <ShieldAlert
                              className="h-3 w-3"
                              strokeWidth={2.6}
                            />
                            未进池
                          </span>
                        ) : null}
                        {boundCount > 0 ? (
                          <span
                            className="inline-flex shrink-0 items-center whitespace-nowrap rounded-full bg-violet-500/10 border border-violet-500/15 px-2 py-1 text-[10px] font-bold text-violet-700 dark:text-violet-300"
                            title={`${boundCount} 个会话绑定到此账号`}
                          >
                            {boundCount} 会话
                          </span>
                        ) : null}
                        {pinned ? (
                          <span
                            className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap rounded-full bg-amber-500/15 border border-amber-500/30 px-2 py-1 text-[10px] font-bold text-amber-700 dark:text-amber-300"
                            title="已锁定 — 自动切换通道全部暂停。点击右侧按钮解锁。"
                          >
                            🔒 锁定
                          </span>
                        ) : null}
                        {inPool ? (
                          <span
                            className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap rounded-full bg-violet-500/15 border border-violet-500/30 px-2 py-1 text-[10px] font-bold text-violet-700 dark:text-violet-300"
                            title="此账号在轮换池内，会被定时切 + 额度耗尽时优先选中"
                          >
                            🔁 池内
                          </span>
                        ) : null}
                      </div>

                      <div className="flex shrink-0 items-center gap-1.5">
                        <button
                          type="button"
                          className="no-drag-region inline-flex items-center gap-1 rounded-full bg-violet-500/10 px-3 py-1.5 text-[12px] font-bold text-violet-600 dark:text-violet-300 shadow-sm transition-colors hover:bg-violet-500/15 ios-btn disabled:opacity-50"
                          disabled={switching}
                          title="把这个账号写入 Windsurf 本地登录态(IDE 看到此账号登录)"
                          onClick={() => handleLoginToWindsurf(acc)}
                        >
                          <LogIn className="h-[14px] w-[14px]" strokeWidth={2.5} />
                          登录
                        </button>
                        {hasApiKey(acc) ? (
                          <button
                            type="button"
                            className={[
                              "no-drag-region inline-flex items-center gap-1 rounded-full px-3 py-1.5 text-[12px] font-bold shadow-sm transition-colors ios-btn disabled:opacity-45",
                              online
                                ? "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 cursor-default"
                                : "bg-ios-blue/10 text-ios-blue dark:text-blue-300 hover:bg-ios-blue/15",
                            ].join(" ")}
                            disabled={
                              !hasApiKey(acc) || online || switchLoading
                            }
                            title={online ? "当前活跃席位" : "手动切到这个 MITM 账号"}
                            onClick={() => handleSwitchMitmToAccount(acc)}
                          >
                            {!online ? (
                              <ArrowRightLeft
                                className={`h-[14px] w-[14px] ${switching ? "animate-pulse" : ""}`}
                                strokeWidth={2.5}
                              />
                            ) : (
                              <ShieldCheck className="h-[14px] w-[14px]" strokeWidth={2.5} />
                            )}
                            {online ? "活跃" : "切到此号"}
                          </button>
                        ) : null}
                        <IDropdownMenu
                          items={accountCardMenuItems(acc)}
                          align="right"
                          width="w-52"
                          compact
                          triggerTitle="更多操作"
                        />
                      </div>
                    </div>

                    <div className="mt-3.5 min-w-0">
                      <div
                        className="truncate text-[24px] font-bold tracking-tight text-ios-text dark:text-ios-textDark"
                        title={acc.nickname || acc.email || acc.id}
                      >
                        {acc.nickname || acc.email || acc.id.slice(0, 12)}
                      </div>
                      <div
                        className="mt-1.5 truncate text-[13px] font-medium text-gray-600 dark:text-gray-300"
                        title={acc.email || "未填写邮箱"}
                      >
                        {acc.email || "未填写邮箱"}
                      </div>
                      {acc.remark ? (
                        <div className="mt-2.5 flex flex-wrap gap-2">
                          <span
                            className="max-w-full truncate rounded-full bg-black/[0.04] px-2.5 py-1 text-[10px] font-medium text-ios-textSecondary/90 dark:text-ios-textSecondaryDark/90 dark:bg-white/[0.06]"
                            title={acc.remark}
                          >
                            {acc.remark}
                          </span>
                        </div>
                      ) : null}
                    </div>

                    <div className="mt-3 grid grid-cols-2 gap-3">
                      <div className="rounded-[16px] border border-black/[0.05] bg-black/[0.025] px-3 py-2.5 dark:border-white/[0.06] dark:bg-white/[0.04]">
                        <div className="flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-[0.14em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                          <CalendarDays className="h-3.5 w-3.5 opacity-70" />
                          到期时间
                        </div>
                        <div className="mt-1.5 text-[13px] font-semibold leading-snug text-ios-text dark:text-ios-textDark">
                          {acc.subscription_expires_at
                            ? formatDateTimeAsiaShanghai(
                                acc.subscription_expires_at,
                              )
                            : "待同步"}
                        </div>
                      </div>

                      <div className="rounded-[16px] border border-black/[0.05] bg-black/[0.025] px-3 py-2.5 dark:border-white/[0.06] dark:bg-white/[0.04]">
                        <div className="flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-[0.14em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                          <Wallet className="h-3.5 w-3.5 opacity-70" />
                          额外余额
                        </div>
                        {acc.has_extra_usage_balance ? (
                          <>
                            <div
                              className={`mt-1.5 text-[15px] font-bold leading-snug ${
                                (acc.extra_usage_balance_micros ?? 0) > 0
                                  ? "text-emerald-600 dark:text-emerald-400"
                                  : "text-gray-400 dark:text-gray-500"
                              }`}
                            >
                              {formatExtraUsageBalance(
                                acc.extra_usage_balance_micros,
                              )}
                            </div>
                            <div className="mt-0.5 text-[10px] text-gray-500/90 dark:text-gray-400/90">
                              {(acc.extra_usage_balance_micros ?? 0) > 0
                                ? "周额度用完后可付费兜底"
                                : "已用尽 / 无兜底"}
                            </div>
                          </>
                        ) : (
                          <div className="mt-1.5 text-[13px] font-semibold leading-snug text-gray-400 dark:text-gray-500">
                            未开通
                          </div>
                        )}
                      </div>
                    </div>

                    <div className="mt-3 rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-4 dark:border-white/[0.06] dark:bg-white/[0.04]">
                      <div className="space-y-2">
                        <div className="flex items-center justify-between text-[12px] font-bold text-gray-800 dark:text-gray-200">
                          <span>日额度</span>
                          <span className="tabular-nums">{acc.daily_remaining || "—"}</span>
                        </div>
                        <div className="h-2.5 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-white/10">
                          <div
                            className={`h-full rounded-full transition-all duration-500 ease-out ${getQuotaColor(
                              acc.daily_remaining || "",
                            )}`}
                            style={{ width: parseQuotaWidth(acc.daily_remaining || "") }}
                          />
                        </div>
                        {acc.daily_reset_at ? (
                          <div
                            className="truncate pt-0.5 text-[10px] font-normal text-gray-400 dark:text-gray-500"
                            title={formatDateTimeAsiaShanghai(acc.daily_reset_at)}
                          >
                            {formatResetCountdownZH(acc.daily_reset_at)}
                          </div>
                        ) : null}
                      </div>

                      <div className="mt-3.5 space-y-2">
                        <div className="flex items-center justify-between text-[12px] font-bold text-gray-800 dark:text-gray-200">
                          <span>周额度</span>
                          <span className="tabular-nums">
                            {acc.weekly_remaining ||
                              (isWeeklyBlockedDisplay(acc) ? "官方缺失" : "—")}
                          </span>
                        </div>
                        <div className="h-2.5 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-white/10">
                          <div
                            className={`h-full rounded-full transition-all duration-500 ease-out ${getQuotaColor(
                              acc.weekly_remaining || "",
                            )}`}
                            style={{ width: parseQuotaWidth(acc.weekly_remaining || "") }}
                          />
                        </div>
                        {acc.weekly_reset_at ? (
                          <div
                            className="truncate pt-0.5 text-[10px] font-normal text-gray-400 dark:text-gray-500"
                            title={formatDateTimeAsiaShanghai(acc.weekly_reset_at)}
                          >
                            {formatResetCountdownZH(acc.weekly_reset_at)}
                          </div>
                        ) : null}
                        {isWeeklyBlockedDisplay(acc) ? (
                          <div className="pt-1 text-[10px] font-semibold text-rose-600 dark:text-rose-300">
                            官方未返回周额度，按不可用处理
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
          )}

          {totalPages > 1 || displayAccounts.length > 30 ? (
            <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
              <div className="text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark font-medium">
                共 {displayAccounts.length} 条，第 {currentPage}/{totalPages} 页
              </div>
              <div className="flex items-center gap-2">
                <ISelectSheet
                  modelValue={pageSize}
                  onValueChange={(v) => setPageSize(Number(v))}
                  options={PAGE_SIZE_OPTIONS}
                  title="每页显示"
                  width="w-28"
                />
                <button
                  type="button"
                  className="no-drag-region rounded-lg border border-black/[0.06] bg-white px-3 py-1.5 text-[12px] font-bold transition hover:bg-black/[0.04] disabled:opacity-40 dark:border-white/[0.08] dark:bg-white/[0.06]"
                  disabled={currentPage <= 1}
                  onClick={() => setCurrentPage(Math.max(1, currentPage - 1))}
                >
                  上一页
                </button>
                {paginationRange.map((p) => (
                  <button
                    key={p}
                    type="button"
                    className={`no-drag-region h-8 min-w-[32px] rounded-lg text-[12px] font-bold transition ${
                      p === currentPage
                        ? "bg-ios-blue text-white shadow-sm"
                        : "border border-black/[0.06] bg-white hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.06]"
                    }`}
                    onClick={() => setCurrentPage(p)}
                  >
                    {p}
                  </button>
                ))}
                <button
                  type="button"
                  className="no-drag-region rounded-lg border border-black/[0.06] bg-white px-3 py-1.5 text-[12px] font-bold transition hover:bg-black/[0.04] disabled:opacity-40 dark:border-white/[0.08] dark:bg-white/[0.06]"
                  disabled={currentPage >= totalPages}
                  onClick={() =>
                    setCurrentPage(Math.min(totalPages, currentPage + 1))
                  }
                >
                  下一页
                </button>
              </div>
            </div>
          ) : null}
        </div>
      )}

    </div>
  );
}

// ── 空状态 onboarding ────────────────────────────────────────────────
interface EmptyStateProps {
  onImport: () => void;
  onEnableMitm: () => void;
  onFinish: () => void;
  onRelay: () => void;
}

// ── 表格(紧凑)视图 ──

interface AccountsTableProps {
  accounts: models.Account[];
  selectedIds: Set<string>;
  onToggleSelect: (id: string) => void;
  isOnline: (acc: models.Account) => boolean;
  isPinned: (acc: models.Account) => boolean;
  inPool: (acc: models.Account) => boolean;
  hasApiKey: (acc: models.Account) => boolean;
  isSwitching: (id: string) => boolean;
  switchLoading: boolean;
  onSwitch: (acc: models.Account) => void;
  onLogin: (acc: models.Account) => void;
  menuItems: (acc: models.Account) => DropdownItem[];
}

function quotaTextTone(s?: string): string {
  const n = parseFloat(String(s ?? "").replace("%", "").trim());
  if (!Number.isFinite(n)) return "text-gray-400 dark:text-gray-500";
  if (n <= 0.01) return "text-rose-600 dark:text-rose-400 font-bold";
  if (n < 20) return "text-amber-600 dark:text-amber-400 font-semibold";
  return "text-ios-text dark:text-ios-textDark";
}

function AccountsTable({
  accounts,
  selectedIds,
  onToggleSelect,
  isOnline,
  isPinned,
  inPool,
  hasApiKey,
  isSwitching,
  switchLoading,
  onSwitch,
  onLogin,
  menuItems,
}: AccountsTableProps) {
  return (
    <div className="overflow-x-auto rounded-[18px] border border-black/[0.06] dark:border-white/[0.08]">
      <table className="w-full min-w-[860px] border-collapse text-[13px]">
        <thead>
          <tr className="border-b border-black/[0.06] bg-black/[0.02] text-left text-[11px] font-bold uppercase tracking-wide text-ios-textSecondary dark:border-white/[0.08] dark:bg-white/[0.03] dark:text-ios-textSecondaryDark">
            <th className="w-10 px-3 py-2.5"></th>
            <th className="px-3 py-2.5">账号</th>
            <th className="px-3 py-2.5">套餐</th>
            <th className="px-3 py-2.5 text-right">日额度</th>
            <th className="px-3 py-2.5 text-right">周额度</th>
            <th className="px-3 py-2.5 text-right">额外余额</th>
            <th className="px-3 py-2.5">状态</th>
            <th className="px-3 py-2.5 text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          {accounts.map((acc) => {
            const online = isOnline(acc);
            const selected = selectedIds.has(acc.id);
            return (
              <tr
                key={acc.id}
                className={`border-b border-black/[0.04] transition-colors last:border-0 dark:border-white/[0.05] ${
                  selected
                    ? "bg-ios-blue/[0.06]"
                    : "hover:bg-black/[0.02] dark:hover:bg-white/[0.03]"
                }`}
              >
                <td className="px-3 py-2.5">
                  <button
                    type="button"
                    onClick={() => onToggleSelect(acc.id)}
                    className={`flex h-5 w-5 items-center justify-center rounded ${
                      selected
                        ? "bg-ios-blue text-white"
                        : "text-ios-textSecondary dark:text-ios-textSecondaryDark"
                    }`}
                    title={selected ? "取消选择" : "选择"}
                  >
                    {selected ? (
                      <CheckSquare className="h-4 w-4" strokeWidth={2.5} />
                    ) : (
                      <Square className="h-4 w-4" strokeWidth={2.5} />
                    )}
                  </button>
                </td>
                <td className="px-3 py-2.5">
                  <div
                    className="max-w-[220px] truncate font-semibold text-ios-text dark:text-ios-textDark"
                    title={acc.email || acc.id}
                  >
                    {acc.nickname || acc.email || acc.id.slice(0, 12)}
                  </div>
                  {acc.nickname && acc.email ? (
                    <div className="max-w-[220px] truncate text-[11px] text-gray-500 dark:text-gray-400">
                      {acc.email}
                    </div>
                  ) : null}
                </td>
                <td className="px-3 py-2.5">
                  <span className="rounded-full bg-ios-blue/10 px-2 py-0.5 text-[10px] font-bold text-ios-blue dark:bg-ios-blue/20">
                    {acc.plan_name || "unknown"}
                  </span>
                </td>
                <td
                  className={`px-3 py-2.5 text-right tabular-nums ${quotaTextTone(acc.daily_remaining)}`}
                >
                  {acc.daily_remaining || "—"}
                </td>
                <td
                  className={`px-3 py-2.5 text-right tabular-nums ${quotaTextTone(acc.weekly_remaining)}`}
                >
                  {acc.weekly_remaining || "—"}
                </td>
                <td className="px-3 py-2.5 text-right tabular-nums">
                  {acc.has_extra_usage_balance ? (
                    <span
                      className={
                        (acc.extra_usage_balance_micros ?? 0) > 0
                          ? "font-semibold text-emerald-600 dark:text-emerald-400"
                          : "text-gray-400 dark:text-gray-500"
                      }
                    >
                      {formatExtraUsageBalance(acc.extra_usage_balance_micros)}
                    </span>
                  ) : (
                    <span className="text-gray-300 dark:text-gray-600">—</span>
                  )}
                </td>
                <td className="px-3 py-2.5">
                  <div className="flex flex-wrap items-center gap-1">
                    {online ? (
                      <span className="rounded-full bg-emerald-500/15 px-2 py-0.5 text-[10px] font-bold text-emerald-700 dark:text-emerald-300">
                        活跃
                      </span>
                    ) : null}
                    {isPinned(acc) ? (
                      <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[10px] font-bold text-amber-700 dark:text-amber-300">
                        🔒
                      </span>
                    ) : null}
                    {inPool(acc) ? (
                      <span className="rounded-full bg-violet-500/15 px-2 py-0.5 text-[10px] font-bold text-violet-700 dark:text-violet-300">
                        🔁
                      </span>
                    ) : null}
                  </div>
                </td>
                <td className="px-3 py-2.5">
                  <div className="flex items-center justify-end gap-1.5">
                    <button
                      type="button"
                      onClick={() => onLogin(acc)}
                      disabled={isSwitching(acc.id)}
                      className="no-drag-region inline-flex items-center gap-1 rounded-full bg-violet-500/10 px-2.5 py-1 text-[11px] font-bold text-violet-600 dark:text-violet-300 hover:bg-violet-500/15 disabled:opacity-50"
                      title="写入本地登录态"
                    >
                      <LogIn className="h-3.5 w-3.5" strokeWidth={2.5} />
                    </button>
                    {hasApiKey(acc) ? (
                      <button
                        type="button"
                        onClick={() => onSwitch(acc)}
                        disabled={online || switchLoading}
                        className={`no-drag-region inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[11px] font-bold disabled:opacity-45 ${
                          online
                            ? "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 cursor-default"
                            : "bg-ios-blue/10 text-ios-blue dark:text-blue-300 hover:bg-ios-blue/15"
                        }`}
                        title={online ? "当前活跃" : "切到此号"}
                      >
                        <ArrowRightLeft className="h-3.5 w-3.5" strokeWidth={2.5} />
                      </button>
                    ) : null}
                    <IDropdownMenu
                      items={menuItems(acc)}
                      align="right"
                      width="w-52"
                      compact
                      triggerTitle="更多操作"
                    />
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function EmptyState({
  onImport,
  onEnableMitm,
  onFinish,
  onRelay,
}: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center flex-1 text-ios-textSecondary dark:text-ios-textSecondaryDark py-12 ios-page-enter">
      <div className="relative mb-8">
        <div className="w-32 h-32 rounded-[32px] bg-gradient-to-br from-ios-blue/15 to-violet-500/15 dark:from-ios-blue/25 dark:to-violet-500/25 flex items-center justify-center shadow-[0_12px_32px_rgba(37,99,235,0.12)]">
          <Users
            className="w-14 h-14 text-ios-blue dark:text-blue-300"
            strokeWidth={1.8}
          />
        </div>
        <div className="absolute -bottom-2 -right-2 w-12 h-12 rounded-2xl bg-white dark:bg-[#1C1C1E] flex items-center justify-center shadow-md ring-2 ring-white/80 dark:ring-black/80">
          <Sparkles className="w-7 h-7 text-emerald-500" strokeWidth={2.4} />
        </div>
      </div>

      <h2 className="text-[24px] font-bold text-ios-text dark:text-ios-textDark mb-2 text-center">
        三步开始无感切号
      </h2>
      <p className="max-w-[480px] text-center text-[14px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark mb-8 px-4">
        把账号导入号池，Windsurf Tools 会接管 Cascade 流量并按额度自动切号 ——
        无需修改 IDE，不打断对话。
      </p>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-3 max-w-[820px] w-full px-4 mb-7">
        <button
          type="button"
          className="no-drag-region group relative rounded-[22px] border border-ios-blue/25 bg-gradient-to-br from-ios-blue/[0.08] to-violet-500/[0.06] dark:from-ios-blue/[0.18] dark:to-violet-500/[0.12] p-5 text-left ios-btn shadow-[0_10px_24px_-12px_rgba(37,99,235,0.4)] hover:-translate-y-0.5 hover:shadow-[0_16px_30px_-12px_rgba(37,99,235,0.4)] transition-all"
          onClick={onImport}
        >
          <div className="flex items-center gap-2.5 mb-2">
            <div className="w-8 h-8 rounded-full bg-ios-blue text-white flex items-center justify-center text-[14px] font-black shadow-md shadow-ios-blue/40">
              1
            </div>
            <span className="text-[15px] font-bold text-ios-text dark:text-ios-textDark">
              批量导入
            </span>
          </div>
          <p className="text-[12px] text-gray-600 dark:text-gray-300 leading-relaxed">
            粘贴 API Key / JWT / 邮箱密码 / Refresh Token，自动识别格式并入池。
          </p>
          <div className="mt-3 inline-flex items-center gap-1 text-[12px] font-bold text-ios-blue group-hover:gap-1.5 transition-all">
            点击开始
            <ChevronRight className="h-3.5 w-3.5" strokeWidth={2.5} />
          </div>
        </button>

        <button
          type="button"
          className="no-drag-region group relative rounded-[22px] border border-violet-500/20 bg-white/70 dark:bg-white/[0.04] p-5 text-left ios-btn hover:-translate-y-0.5 hover:bg-violet-500/[0.04] hover:border-violet-500/30 transition-all"
          onClick={onEnableMitm}
        >
          <div className="flex items-center gap-2.5 mb-2">
            <div className="w-8 h-8 rounded-full bg-violet-500/15 text-violet-600 dark:text-violet-300 flex items-center justify-center text-[14px] font-black">
              2
            </div>
            <span className="text-[15px] font-bold text-ios-text dark:text-ios-textDark">
              启用 MITM 代理
            </span>
          </div>
          <p className="text-[12px] text-gray-500 dark:text-gray-400 leading-relaxed">
            到总览页一键完成 CA 证书 + Hosts 配置，打开代理后号池立即接管 IDE 流量。
          </p>
          <div className="mt-3 inline-flex items-center gap-1 text-[12px] font-bold text-violet-600 dark:text-violet-300 group-hover:gap-1.5 transition-all">
            前往总览
            <ChevronRight className="h-3.5 w-3.5" strokeWidth={2.5} />
          </div>
        </button>

        <button
          type="button"
          className="no-drag-region group relative rounded-[22px] border border-emerald-500/20 bg-white/70 dark:bg-white/[0.04] p-5 text-left ios-btn hover:-translate-y-0.5 hover:bg-emerald-500/[0.04] hover:border-emerald-500/30 transition-all"
          onClick={onFinish}
        >
          <div className="flex items-center gap-2.5 mb-2">
            <div className="w-8 h-8 rounded-full bg-emerald-500/15 text-emerald-600 dark:text-emerald-300 flex items-center justify-center text-[14px] font-black">
              3
            </div>
            <span className="text-[15px] font-bold text-ios-text dark:text-ios-textDark">
              开始使用
            </span>
          </div>
          <p className="text-[12px] text-gray-500 dark:text-gray-400 leading-relaxed">
            开启代理后，打开或重启 Windsurf 照常对话即可，额度用完会在后台自动换号。
          </p>
          <div className="mt-3 inline-flex items-center gap-1 text-[12px] font-bold text-emerald-600 dark:text-emerald-300 group-hover:gap-1.5 transition-all">
            查看仪表盘
            <ChevronRight className="h-3.5 w-3.5" strokeWidth={2.5} />
          </div>
        </button>
      </div>

      <div className="flex flex-wrap items-center justify-center gap-2">
        <button
          type="button"
          className="no-drag-region inline-flex items-center gap-2 rounded-full bg-gradient-to-b from-[#3b82f6] to-ios-blue px-6 py-3 text-[14px] font-bold text-white transition-all hover:scale-[1.02] active:scale-[0.98] ios-btn shadow-md shadow-ios-blue/25"
          onClick={onImport}
        >
          <Plus className="h-4 w-4" strokeWidth={2.4} />
          导入第一个账号
        </button>
        <button
          type="button"
          className="no-drag-region inline-flex items-center gap-2 rounded-full border border-black/[0.06] bg-white/80 px-4 py-3 text-[13px] font-bold text-gray-700 transition-colors hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-gray-200 dark:hover:bg-white/[0.08] ios-btn"
          onClick={onRelay}
        >
          <ChevronRight className="h-4 w-4" strokeWidth={2.4} />
          直连 OpenAI Relay
        </button>
      </div>
    </div>
  );
}
