import { useCallback, useEffect, useMemo, useRef, useState, type ComponentType } from "react";
import {
  Activity,
  AlertCircle,
  ArrowRight,
  CheckCircle2,
  ChevronRight,
  Copy,
  Globe,
  Link2,
  Play,
  Plus,
  RefreshCcw,
  ShieldCheck,
  TriangleAlert,
  Users,
  X,
  XCircle,
} from "lucide-react";
import { APIInfo } from "../api/wails";
import DashboardMetrics from "../components/DashboardMetrics";
import F7Banner from "../components/F7Banner";
import MitmPanel from "../components/MitmPanel";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton";
import SkeletonOverlay from "../components/common/SkeletonOverlay";
// F7-REMOVAL: useSmartFriend 仅在 F7 模式下生效，发布前一并删除
import { useSmartFriend } from "../hooks/useSmartFriend";
import { useAccountStore } from "../stores/useAccountStore";
import { useMainViewStore } from "../stores/useMainViewStore";
import { useMitmStatusStore } from "../stores/useMitmStatusStore";
import { useRelayStatusStore } from "../stores/useRelayStatusStore";
import { useSettingsStore } from "../stores/useSettingsStore";
import { useProviderAccountStore } from "../stores/useProviderAccountStore";
import { useMergedTasks, useTaskStore, type Task } from "../stores/useTaskStore";
import { PROVIDER_DISPLAY_ORDER, PROVIDER_META, type ProviderID } from "../utils/provider";
import {
  getAccountHealth,
  isWeeklyQuotaBlocked,
  truncateMiddle,
} from "../utils/account";
import { showErrorToast, showToast } from "../utils/toast";
import IInfoTooltip from "../components/ios/IInfoTooltip";

type DiagnoseStatus = "ok" | "warn" | "error" | "n/a";
type DiagnoseCheckItem = {
  id: string;
  title: string;
  status: DiagnoseStatus;
  detail: string;
  fix_hint?: string;
};
type DiagnoseReportData = {
  platform: string;
  arch: string;
  ok: number;
  warn: number;
  error: number;
  checks: DiagnoseCheckItem[];
};
type HealthTone = "ok" | "warn" | "error" | "info";
type HealthCenterItem = {
  key: string;
  title: string;
  detail: string;
  tone: HealthTone;
  icon: ComponentType<{ className?: string; strokeWidth?: number | string }>;
  actionLabel?: string;
  onAction?: () => void;
};
type DiagnosticAction = {
  key: string;
  label: string;
  onClick: () => void;
};

const diagnoseStatusClass = (s: DiagnoseStatus): string => {
  switch (s) {
    case "ok":
      return "border-emerald-500/15 bg-emerald-500/[0.05]";
    case "warn":
      return "border-amber-500/20 bg-amber-500/[0.06]";
    case "error":
      return "border-rose-500/20 bg-rose-500/[0.07]";
    default:
      return "border-gray-300/30 bg-gray-100/30";
  }
};

const diagnoseStatusLabel: Record<DiagnoseStatus, string> = {
  ok: "通过",
  warn: "警告",
  error: "错误",
  "n/a": "不适用",
};

const healthToneClass: Record<HealthTone, string> = {
  ok: "border-emerald-500/15 bg-emerald-500/[0.06]",
  warn: "border-amber-500/20 bg-amber-500/[0.07]",
  error: "border-rose-500/20 bg-rose-500/[0.08]",
  info: "border-sky-500/15 bg-sky-500/[0.06]",
};

const healthIconClass: Record<HealthTone, string> = {
  ok: "bg-emerald-500/12 text-emerald-600 dark:text-emerald-300",
  warn: "bg-amber-500/12 text-amber-700 dark:text-amber-300",
  error: "bg-rose-500/12 text-rose-700 dark:text-rose-300",
  info: "bg-sky-500/12 text-sky-700 dark:text-sky-300",
};

const healthPillClass: Record<HealthTone, string> = {
  ok: "bg-emerald-500/12 text-emerald-700 dark:text-emerald-300",
  warn: "bg-amber-500/12 text-amber-800 dark:text-amber-300",
  error: "bg-rose-500/12 text-rose-700 dark:text-rose-300",
  info: "bg-sky-500/12 text-sky-700 dark:text-sky-300",
};

function diagnoseSearchText(c: DiagnoseCheckItem): string {
  return `${c.id} ${c.title} ${c.detail} ${c.fix_hint || ""}`.toLowerCase();
}

function isCertDiagnostic(c: DiagnoseCheckItem): boolean {
  const text = diagnoseSearchText(c);
  return c.id === "cert" || text.includes("证书") || /(^|[^a-z])ca([^a-z]|$)/.test(text);
}

function isHostsDiagnostic(c: DiagnoseCheckItem): boolean {
  return diagnoseSearchText(c).includes("hosts");
}

function isClashDiagnostic(c: DiagnoseCheckItem): boolean {
  const text = diagnoseSearchText(c);
  return c.id === "clash" || text.includes("clash") || text.includes("mihomo");
}

function isRelayDiagnostic(c: DiagnoseCheckItem): boolean {
  const text = diagnoseSearchText(c);
  return c.id === "relay" || text.includes("relay") || text.includes("openai");
}

function buildDiagnosticsReportText(report: DiagnoseReportData): string {
  const lines = [
    "Windsurf Tools 平台兼容性检查",
    `平台: ${report.platform}`,
    `架构: ${report.arch}`,
    `汇总: ${report.ok} 通过 / ${report.warn} 警告 / ${report.error} 错误`,
    "",
  ];
  for (const c of report.checks) {
    lines.push(`[${diagnoseStatusLabel[c.status] || c.status}] ${c.title}`);
    lines.push(`详情: ${c.detail}`);
    if (c.fix_hint) lines.push(`建议: ${c.fix_hint}`);
    lines.push("");
  }
  return lines.join("\n").trimEnd();
}

function taskFailureLines(tasks: Task[]): string[] {
  const failedTasks = tasks.filter((t) => t.failed > 0);
  if (failedTasks.length === 0) return ["暂无失败任务"];
  const lines: string[] = [];
  for (const task of failedTasks.slice(0, 5)) {
    lines.push(
      `- ${task.title}: ${task.failed} 失败 / ${task.succeeded} 成功 / ${task.completed}/${task.total}`,
    );
    const failedItems = task.items.filter((it) => it.status === "failed").slice(0, 5);
    for (const item of failedItems) {
      lines.push(`  - ${item.name}${item.detail ? ` — ${item.detail}` : ""}`);
    }
  }
  if (failedTasks.length > 5) {
    lines.push(`- 另有 ${failedTasks.length - 5} 个失败任务未展开`);
  }
  return lines;
}

/**
 * Dashboard — Vue 1:1 完整迁移。
 * hero header / 5 summary 卡 / F7Banner / MitmPanel / onboarding 3 步 /
 * 快速跳转 actionCards / 周阻断提示 / 平台兼容性诊断 modal。
 */
export default function Dashboard() {
  const accounts = useAccountStore((s) => s.accounts);
  const accHasLoadedOnce = useAccountStore((s) => s.hasLoadedOnce);
  const mitmStatus = useMitmStatusStore((s) => s.status);
  const mitmHasLoadedOnce = useMitmStatusStore((s) => s.hasLoadedOnce);
  const relayStatus = useRelayStatusStore((s) => s.status);
  const relayHasLoadedOnce = useRelayStatusStore((s) => s.hasLoadedOnce);
  const tasks = useMergedTasks();
  const openTaskDrawer = useTaskStore((s) => s.setOpen);
  const setActiveTab = useMainViewStore((s) => s.setActiveTab);
  const dashboardDiagnosticsRequestSeq = useMainViewStore(
    (s) => s.dashboardDiagnosticsRequestSeq,
  );
  const sf = useSmartFriend();

  const [refreshing, setRefreshing] = useState(false);
  const [diagnostics, setDiagnostics] = useState<DiagnoseReportData | null>(null);
  const [showDiagnostics, setShowDiagnostics] = useState(false);
  const [diagnosticsLoading, setDiagnosticsLoading] = useState(false);
  const [troubleshootingLoading, setTroubleshootingLoading] = useState(false);
  const [healthExpanded, setHealthExpanded] = useState(true);
  const [healthOnlyIssues, setHealthOnlyIssues] = useState(false);
  const [proxyStatus, setProxyStatus] = useState<{
    source: string
    url: string
    last_applied_at: string
  } | null>(null);

  const mitmPanelRef = useRef<HTMLDivElement | null>(null);
  const handledDiagnosticsRequestSeqRef = useRef(0);

  useEffect(() => {
    void Promise.all([
      useAccountStore.getState().ensureAccountsLoaded(),
      useMitmStatusStore.getState().ensureStatusLoaded(),
      useRelayStatusStore.getState().ensureStatusLoaded(),
      fetchProxyStatus(),
    ]);
  }, []);

  async function fetchProxyStatus() {
    try {
      const s = await APIInfo.getUpstreamProxyStatus();
      setProxyStatus(s ?? null);
    } catch {
      setProxyStatus(null);
    }
  }

  function proxySourceLabel(s?: string): string {
    switch (s) {
      case "clash+nodes":
        return "Clash + 轮换";
      case "clash":
        return "Clash";
      case "manual":
        return "手动代理";
      case "env":
        return "系统代理";
      case "direct":
        return "直连";
      default:
        return "—";
    }
  }

  function proxySourceTone(s?: string): string {
    switch (s) {
      case "clash+nodes":
        return "bg-violet-500/10 text-violet-700 dark:text-violet-300";
      case "clash":
        return "bg-sky-500/10 text-sky-700 dark:text-sky-300";
      case "manual":
        return "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
      case "env":
        return "bg-amber-500/10 text-amber-700 dark:text-amber-300";
      case "direct":
        return "bg-slate-500/10 text-slate-700 dark:text-slate-300";
      default:
        return "bg-slate-500/10 text-slate-700 dark:text-slate-300";
    }
  }

  const refreshOverview = async () => {
    setRefreshing(true);
    try {
      await Promise.all([
        useAccountStore.getState().fetchAccounts(true),
        useMitmStatusStore.getState().fetchStatus(),
        useRelayStatusStore.getState().fetchStatus(true),
        fetchProxyStatus(),
      ]);
    } finally {
      setRefreshing(false);
    }
  };

  const handleRunDiagnostics = useCallback(async () => {
    setDiagnosticsLoading(true);
    try {
      const r = (await APIInfo.runDiagnostics()) as DiagnoseReportData;
      setDiagnostics(r);
      setShowDiagnostics(true);
      const summary = `${r.ok} 通过 / ${r.warn} 警告 / ${r.error} 错误`;
      if (r.error > 0) {
        showToast(`平台兼容性: ${summary}（有问题需修）`, "warning", 5000);
      } else if (r.warn > 0) {
        showToast(`平台兼容性: ${summary}（建议优化）`, "info", 4000);
      } else {
        showToast("平台兼容性: 全部通过 ✓", "success", 3000);
      }
    } catch (e) {
      showErrorToast(e, "诊断失败");
    } finally {
      setDiagnosticsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (
      dashboardDiagnosticsRequestSeq === 0 ||
      handledDiagnosticsRequestSeqRef.current === dashboardDiagnosticsRequestSeq
    ) {
      return;
    }
    handledDiagnosticsRequestSeqRef.current = dashboardDiagnosticsRequestSeq;
    void handleRunDiagnostics();
  }, [dashboardDiagnosticsRequestSeq, handleRunDiagnostics]);

  const handleCopyDiagnostics = useCallback(async () => {
    if (!diagnostics) {
      showToast("暂无诊断报告可复制", "warning");
      return;
    }
    try {
      await navigator.clipboard.writeText(buildDiagnosticsReportText(diagnostics));
      showToast("诊断报告已复制", "success");
    } catch (e) {
      showErrorToast(e, "复制诊断报告失败");
    }
  }, [diagnostics]);

  const booting = !accHasLoadedOnce || !mitmHasLoadedOnce || !relayHasLoadedOnce;

  const totalAccounts = accounts.length;
  const expiredAccounts = useMemo(
    () => accounts.filter((a) => getAccountHealth(a) === "expired").length,
    [accounts],
  );
  const criticalAccounts = useMemo(() => {
    if (sf.active) return 0;
    return accounts.filter((a) => getAccountHealth(a) === "critical").length;
  }, [accounts, sf.active]);
  const blockedAccounts = useMemo(() => {
    if (sf.active) return 0;
    return accounts.filter((a) => isWeeklyQuotaBlocked(a)).length;
  }, [accounts, sf.active]);
  const healthyAccounts = useMemo(() => {
    if (sf.active) return Math.max(0, totalAccounts - expiredAccounts);
    return accounts.filter((a) => getAccountHealth(a) === "healthy").length;
  }, [accounts, sf.active, totalAccounts, expiredAccounts]);

  const activeKey = useMemo(
    () => mitmStatus?.pool_status?.find((i) => i.is_current) ?? null,
    [mitmStatus],
  );
  const relayRunning = relayStatus?.running === true;
  const failedTaskCount = useMemo(
    () => tasks.reduce((sum, task) => sum + task.failed, 0),
    [tasks],
  );
  const runningTaskCount = useMemo(
    () => tasks.filter((task) => task.running).length,
    [tasks],
  );

  const handleCopyTroubleshootingBundle = useCallback(async () => {
    setTroubleshootingLoading(true);
    try {
      const report = (await APIInfo.runDiagnostics()) as DiagnoseReportData;
      setDiagnostics(report);
      const currentActiveKey =
        mitmStatus?.pool_status?.find((i) => i.is_current) ?? null;
      const lines = [
        "Windsurf Tools 排障包",
        `生成时间: ${new Date().toLocaleString()}`,
        "",
        "== 运行状态 ==",
        `MITM: ${mitmStatus?.running ? "运行中" : "未启动"}`,
        `CA: ${mitmStatus?.ca_installed ? "已信任" : "未信任"}`,
        `Hosts: ${mitmStatus?.hosts_mapped ? "已映射" : "未映射"}`,
        `活跃账号: ${
          currentActiveKey?.nickname ||
          currentActiveKey?.email ||
          currentActiveKey?.key_short ||
          "无"
        }`,
        `活跃会话: ${mitmStatus?.session_count ?? 0}`,
        `代理请求: ${mitmStatus?.total_requests ?? 0}`,
        `Relay: ${
          relayRunning
            ? `运行中 (${relayStatus?.url || `127.0.0.1:${relayStatus?.port || 8787}`})`
            : "未启动"
        }`,
        "",
        "== 号池 ==",
        `总数: ${totalAccounts}`,
        `健康: ${healthyAccounts}`,
        `额度告急: ${criticalAccounts}`,
        `已过期: ${expiredAccounts}`,
        `周额度阻断: ${blockedAccounts}`,
        `F7/SmartFriend: ${sf.active ? "启用" : "关闭"}`,
        "",
        "== 任务失败摘要 ==",
        ...taskFailureLines(tasks),
        "",
        "== 平台兼容性诊断 ==",
        buildDiagnosticsReportText(report),
      ];
      await navigator.clipboard.writeText(lines.join("\n"));
      showToast("排障包已复制", "success");
    } catch (e) {
      showErrorToast(e, "复制排障包失败");
    } finally {
      setTroubleshootingLoading(false);
    }
  }, [
    blockedAccounts,
    criticalAccounts,
    expiredAccounts,
    healthyAccounts,
    mitmStatus,
    relayRunning,
    relayStatus?.port,
    relayStatus?.url,
    sf.active,
    tasks,
    totalAccounts,
  ]);

  const topSummaryCards: Array<{
    key: string;
    label: string;
    value: string;
    detail: string;
    tone: string;
    icon: ComponentType<{ className?: string; strokeWidth?: number | string }>;
  }> = [
    {
      key: "pool",
      label: "号池总数",
      value: String(totalAccounts),
      detail:
        healthyAccounts > 0 ? `健康 ${healthyAccounts} 个` : "等待可用账号",
      tone: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
      icon: Users,
    },
    {
      key: "mitm",
      label: "MITM 状态",
      value: mitmStatus?.running ? "运行中" : "未启动",
      detail: mitmStatus?.running
        ? activeKey?.nickname || activeKey?.email
          ? `当前 ${activeKey.nickname || activeKey.email}`
          : activeKey?.key_short
            ? `当前 ${truncateMiddle(activeKey.key_short, 10, 5)}`
            : "等待活跃 Key"
        : "先完成证书、Hosts 与启用",
      tone: mitmStatus?.running
        ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
        : "bg-amber-500/10 text-amber-700 dark:text-amber-300",
      icon: ShieldCheck,
    },
    {
      key: "relay",
      label: "Relay",
      value: relayRunning ? "已启动" : "未启动",
      detail: relayRunning
        ? `127.0.0.1:${relayStatus?.port || 8787}`
        : "需要时可单独启动",
      tone: relayRunning
        ? "bg-violet-500/10 text-violet-700 dark:text-violet-300"
        : "bg-slate-500/10 text-slate-700 dark:text-slate-300",
      icon: Globe,
    },
    {
      key: "sessions",
      label: "活跃会话",
      value: String(mitmStatus?.session_count ?? 0),
      detail:
        (mitmStatus?.session_count ?? 0) > 0
          ? `${mitmStatus?.session_count} 个对话绑定中`
          : "暂无活跃会话绑定",
      tone: "bg-violet-500/10 text-violet-700 dark:text-violet-300",
      icon: Link2,
    },
    {
      key: "requests",
      label: "代理请求",
      value: String(mitmStatus?.total_requests ?? 0),
      detail:
        blockedAccounts > 0
          ? `周额度阻断 ${blockedAccounts}`
          : "暂未发现周额度阻断",
      tone: "bg-fuchsia-500/10 text-fuchsia-700 dark:text-fuchsia-300",
      icon: Activity,
    },
    {
      key: "upstream-proxy",
      label: "上游代理",
      value: proxySourceLabel(proxyStatus?.source),
      detail: proxyStatus?.url
        ? proxyStatus.url === "<direct>"
          ? "未走任何代理"
          : proxyStatus.url
        : "未启动 / 未探活",
      tone: proxySourceTone(proxyStatus?.source),
      icon: Globe,
    },
  ];

  const actionCards = [
    {
      key: "accounts",
      title: "管理号池",
      body: "导入 API Key、刷新额度、检查健康状态与到期账号。",
      tab: "Accounts" as const,
    },
    {
      key: "relay",
      title: "配置 Relay",
      body: "查看 8787 端口、复制接入地址，验证 OpenAI 兼容调用。",
      tab: "Relay" as const,
    },
    {
      key: "settings",
      title: "调整 MITM 设置",
      body: "确认后台服务、自动刷新、自动切换与启动参数。",
      tab: "Settings" as const,
    },
  ];

  // ── onboarding 3 步 ──
  const hasAccount = totalAccounts > 0;
  const caReady = Boolean(mitmStatus?.ca_installed);
  const hostsReady = Boolean(mitmStatus?.hosts_mapped);
  const setupReady = caReady && hostsReady;
  const mitmRunning = Boolean(mitmStatus?.running);

  const scrollToMitm = () => {
    mitmPanelRef.current?.scrollIntoView({
      behavior: "smooth",
      block: "start",
    });
  };

  const scrollToMitmAnchor = (anchor: "ca" | "hosts") => {
    setShowDiagnostics(false);
    window.setTimeout(() => {
      const el = document.querySelector<HTMLElement>(
        `[data-health-anchor="mitm-${anchor}"]`,
      );
      (el ?? mitmPanelRef.current)?.scrollIntoView({
        behavior: "smooth",
        block: "center",
      });
    }, 80);
    showToast(anchor === "ca" ? "已定位到 MITM 证书区" : "已定位到 MITM Hosts 区", "info");
  };

  const openClashSettings = () => {
    setShowDiagnostics(false);
    setActiveTab("Settings");
    window.setTimeout(() => {
      document
        .querySelector<HTMLElement>('[data-health-anchor="clash-settings"]')
        ?.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 160);
    showToast("已打开 Settings / Clash 助手", "info");
  };

  const openRelayPage = () => {
    setShowDiagnostics(false);
    setActiveTab("Relay");
    showToast("已打开 Relay 页", "info");
  };

  const copyDiagnosticHint = async (c: DiagnoseCheckItem) => {
    const text = c.fix_hint || c.detail;
    try {
      await navigator.clipboard.writeText(text);
      showToast(c.fix_hint ? "修复建议已复制" : "诊断详情已复制", "success");
    } catch (e) {
      showErrorToast(e, "复制诊断建议失败");
    }
  };

  const diagnosticActionsFor = (c: DiagnoseCheckItem): DiagnosticAction[] => {
    const actions: DiagnosticAction[] = [];
    if (isCertDiagnostic(c)) {
      actions.push({
        key: "cert",
        label: "证书区",
        onClick: () => scrollToMitmAnchor("ca"),
      });
    }
    if (isHostsDiagnostic(c)) {
      actions.push({
        key: "hosts",
        label: "Hosts 区",
        onClick: () => scrollToMitmAnchor("hosts"),
      });
    }
    if (isClashDiagnostic(c)) {
      actions.push({ key: "clash", label: "Clash 助手", onClick: openClashSettings });
    }
    if (isRelayDiagnostic(c)) {
      actions.push({ key: "relay", label: "Relay 页", onClick: openRelayPage });
    }
    if (c.status !== "ok") {
      actions.push({
        key: "rerun",
        label: "重新检查",
        onClick: () => void handleRunDiagnostics(),
      });
    }
    actions.push({
      key: "copy",
      label: c.fix_hint ? "复制建议" : "复制详情",
      onClick: () => void copyDiagnosticHint(c),
    });
    return actions;
  };

  const healthItems: HealthCenterItem[] = [
    {
      key: "setup",
      title: "接管链路",
      detail: setupReady
        ? "CA 与 Hosts 已就绪"
        : !caReady && !hostsReady
          ? "CA 与 Hosts 都未完成"
          : !caReady
            ? "CA 证书未信任"
            : "Hosts 未映射",
      tone: setupReady ? "ok" : "error",
      icon: ShieldCheck,
      actionLabel: setupReady ? undefined : "去配置",
      onAction: setupReady ? undefined : scrollToMitm,
    },
    {
      key: "mitm",
      title: "MITM 代理",
      detail: mitmRunning
        ? activeKey?.nickname || activeKey?.email || activeKey?.key_short
          ? `运行中 · ${activeKey.nickname || activeKey.email || activeKey.key_short}`
          : "运行中 · 等待活跃 Key"
        : setupReady
          ? "已具备启动条件"
          : "需先完成接管链路",
      tone: mitmRunning ? "ok" : setupReady ? "warn" : "info",
      icon: Activity,
      actionLabel: mitmRunning ? undefined : "打开面板",
      onAction: mitmRunning ? undefined : scrollToMitm,
    },
    {
      key: "pool",
      title: "号池健康",
      detail:
        totalAccounts === 0
          ? "暂无账号"
          : blockedAccounts > 0 || expiredAccounts > 0
            ? `需处理 ${blockedAccounts + expiredAccounts} 个风险账号`
            : criticalAccounts > 0
              ? `${criticalAccounts} 个账号额度告急`
              : `健康 ${healthyAccounts}/${totalAccounts}`,
      tone:
        totalAccounts === 0 || blockedAccounts > 0 || expiredAccounts > 0
          ? "error"
          : criticalAccounts > 0
            ? "warn"
            : "ok",
      icon: Users,
      actionLabel:
        totalAccounts === 0 || blockedAccounts > 0 || expiredAccounts > 0 || criticalAccounts > 0
          ? "看号池"
          : undefined,
      onAction:
        totalAccounts === 0 || blockedAccounts > 0 || expiredAccounts > 0 || criticalAccounts > 0
          ? () => setActiveTab("Accounts")
          : undefined,
    },
    {
      key: "relay",
      title: "OpenAI Relay",
      detail: relayRunning
        ? relayStatus?.url || `127.0.0.1:${relayStatus?.port || 8787}`
        : "未启动 · 按需启用",
      tone: relayRunning ? "ok" : "info",
      icon: Globe,
      actionLabel: relayRunning ? undefined : "去 Relay",
      onAction: relayRunning ? undefined : () => setActiveTab("Relay"),
    },
    {
      key: "tasks",
      title: "任务失败",
      detail:
        failedTaskCount > 0
          ? `${failedTaskCount} 条失败明细`
          : runningTaskCount > 0
            ? `${runningTaskCount} 个任务运行中`
            : "暂无失败任务",
      tone: failedTaskCount > 0 ? "warn" : runningTaskCount > 0 ? "info" : "ok",
      icon: failedTaskCount > 0 ? TriangleAlert : CheckCircle2,
      actionLabel: failedTaskCount > 0 || runningTaskCount > 0 ? "打开任务" : undefined,
      onAction:
        failedTaskCount > 0 || runningTaskCount > 0
          ? () => openTaskDrawer(true)
          : undefined,
    },
    {
      key: "diagnostics",
      title: "平台诊断",
      detail: diagnostics
        ? `${diagnostics.ok} 通过 / ${diagnostics.warn} 警告 / ${diagnostics.error} 错误`
        : "尚未运行本次诊断",
      tone: diagnostics
        ? diagnostics.error > 0
          ? "error"
          : diagnostics.warn > 0
            ? "warn"
            : "ok"
        : "info",
      icon: diagnostics?.error ? XCircle : ShieldCheck,
      actionLabel: "检查",
      onAction: () => void handleRunDiagnostics(),
    },
  ];
  const healthSummaryTone: HealthTone = healthItems.some((item) => item.tone === "error")
    ? "error"
    : healthItems.some((item) => item.tone === "warn")
      ? "warn"
      : healthItems.some((item) => item.tone === "info")
        ? "info"
        : "ok";
  const healthSummaryText =
    healthSummaryTone === "error"
      ? "存在阻断项"
      : healthSummaryTone === "warn"
        ? "有待处理项"
        : healthSummaryTone === "info"
          ? "可继续完善"
          : "全部稳定";
  const healthIssueCount = healthItems.filter(
    (item) => item.tone === "error" || item.tone === "warn",
  ).length;
  const visibleHealthItems = healthOnlyIssues
    ? healthItems.filter((item) => item.tone === "error" || item.tone === "warn")
    : healthItems;

  const onboardingStepsRaw = [
    {
      key: "import",
      index: 1,
      title: "导入账号到号池",
      description: hasAccount
        ? `已有 ${totalAccounts} 个账号，可继续追加。`
        : "粘贴 API Key / JWT / 邮箱密码，自动识别格式入池。",
      done: hasAccount,
      icon: Plus,
      cta: hasAccount ? "再导一批" : "立即导入",
      // 1.6: 直接调起全局 ImportModal，不必先切到 Accounts 页。
      onClick: () => useMainViewStore.getState().openImportModal(),
    },
    {
      key: "ca-hosts",
      index: 2,
      title: "装 CA 证书 + Hosts 接管",
      description: setupReady
        ? "CA 已信任 + Hosts 已映射，本机接管路径就绪。"
        : !caReady && !hostsReady
          ? "下方 MITM 面板「一键安装」会同时配好两项，需要管理员密码。"
          : !caReady
            ? "CA 证书还没信任，下方 MITM 面板里点「安装证书」。"
            : "Hosts 还没配置，下方 MITM 面板里点「配置 Hosts」。",
      done: setupReady,
      icon: ShieldCheck,
      cta: setupReady ? "已就绪" : "前往配置",
      onClick: scrollToMitm,
    },
    {
      key: "mitm-on",
      index: 3,
      title: "打开 MITM 代理",
      description: mitmRunning
        ? "✅ 全部就绪 — 现在打开或重启 Windsurf，照常对话即可，本工具会在后台自动换号。"
        : setupReady
          ? "下方 MITM 面板里点开关，启动后 IDE 即可正常对话。"
          : "完成上一步后再回来打开。",
      done: mitmRunning,
      icon: Play,
      cta: mitmRunning ? "运行中" : "启动代理",
      onClick: scrollToMitm,
    },
  ];

  const firstUndoneIdx = onboardingStepsRaw.findIndex((s) => !s.done);
  const onboardingSteps = onboardingStepsRaw.map((s, i) => ({
    ...s,
    current: i === firstUndoneIdx,
  }));
  const allOnboardingDone = onboardingSteps.every((s) => s.done);

  if (booting) {
    return <PageLoadingSkeleton variant="dashboard" className="w-full" />;
  }

  return (
    <SkeletonOverlay
      active={refreshing}
      label="总览刷新中"
      skeleton={<PageLoadingSkeleton variant="dashboard" className="w-full" />}
    >
      <div className="space-y-6 p-6">
        {/* hero header */}
        <section className="ios-glass overflow-hidden rounded-ios-card border border-black/[0.05] shadow-[0_20px_48px_-20px_rgba(15,23,42,0.18)] dark:border-white/[0.06]">
          <div className="border-b border-black/[0.05] dark:border-white/[0.06] px-6 py-5">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div className="flex min-w-0 items-start gap-3">
                <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-ios-blue/10 text-ios-blue shadow-inner">
                  <ShieldCheck className="h-5 w-5" strokeWidth={2.4} />
                </div>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h1 className="text-[17px] font-bold text-ios-text dark:text-ios-textDark">
                      总览
                    </h1>
                    <span className="inline-flex items-center rounded-full bg-ios-blue/10 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-ios-blue">
                      MITM
                      <IInfoTooltip size={11} maxWidth={260}>
                        <b>MITM</b>（中间人代理）：本工具在你电脑本地拦截 Windsurf
                        发出的请求、替换成号池里的账号再转发出去。全程只在本机进行，
                        IDE 完全无感知，不会上传你的数据。
                      </IInfoTooltip>
                    </span>
                  </div>
                  <p className="mt-1 max-w-3xl text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                    在这里完成启用的关键四步：看账号池是否健康 → 装好证书与
                    Hosts → 打开代理 → 确认当前正在用哪个账号。下面按提示一步步点即可。
                  </p>
                </div>
              </div>

              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  className="no-drag-region inline-flex items-center gap-2 rounded-full border border-violet-500/15 bg-violet-500/10 px-4 py-2 text-[12px] font-semibold text-violet-700 dark:text-violet-300 shadow-sm transition-all ios-btn hover:bg-violet-500/15 disabled:opacity-50"
                  disabled={diagnosticsLoading}
                  onClick={handleRunDiagnostics}
                >
                  <ShieldCheck
                    className={`h-3.5 w-3.5 ${
                      diagnosticsLoading ? "animate-spin" : ""
                    }`}
                    strokeWidth={2.4}
                  />
                  {diagnosticsLoading ? "检查中..." : "平台兼容性检查"}
                </button>
                <button
                  type="button"
                  className="no-drag-region inline-flex items-center gap-2 rounded-full border border-emerald-500/15 bg-emerald-500/10 px-4 py-2 text-[12px] font-semibold text-emerald-700 shadow-sm transition-all ios-btn hover:bg-emerald-500/15 disabled:opacity-50 dark:text-emerald-300"
                  disabled={troubleshootingLoading}
                  onClick={handleCopyTroubleshootingBundle}
                >
                  <Copy
                    className={`h-3.5 w-3.5 ${
                      troubleshootingLoading ? "animate-spin" : ""
                    }`}
                    strokeWidth={2.4}
                  />
                  {troubleshootingLoading ? "整理中..." : "复制排障包"}
                </button>
                <button
                  type="button"
                  className="no-drag-region inline-flex items-center gap-2 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2 text-[12px] font-semibold text-ios-text shadow-sm transition-all ios-btn hover:bg-black/[0.04] disabled:opacity-50 dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textDark"
                  disabled={refreshing}
                  onClick={refreshOverview}
                >
                  <RefreshCcw
                    className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`}
                    strokeWidth={2.4}
                  />
                  {refreshing ? "刷新中..." : "刷新总览"}
                </button>
              </div>
            </div>
          </div>

          {/* F7Banner full */}
          <F7Banner variant="full" />

          {/* summary 卡片 grid */}
          <div className="grid grid-cols-1 gap-4 p-6 md:grid-cols-2 xl:grid-cols-5">
            {topSummaryCards.map((card) => {
              const Icon = card.icon;
              return (
                <div
                  key={card.key}
                  className="rounded-[20px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]"
                >
                  <div className="flex items-center justify-between">
                    <div
                      className={`flex h-9 w-9 items-center justify-center rounded-2xl ${card.tone}`}
                    >
                      <Icon className="h-4 w-4" strokeWidth={2.4} />
                    </div>
                    <div className="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                      {card.label}
                    </div>
                  </div>
                  <div className="mt-3 text-[24px] font-extrabold leading-none text-ios-text dark:text-ios-textDark">
                    {card.value}
                  </div>
                  <div className="mt-2 text-[11.5px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                    {card.detail}
                  </div>
                </div>
              );
            })}
          </div>
        </section>

        {/* F2: 切号历史趋势卡（24h 折线 + 原因分布 + Top 账号） */}
        <DashboardMetrics />

        {/* ★ 提供商汇总(仅 routeMode=providers 时显示) */}
        <ProviderRouteOverview />

        {/* 主区 grid: MitmPanel | (onboarding + actions + warning) */}
        <section className="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.25fr)_360px]">
          <div ref={mitmPanelRef} className="min-w-0">
            <MitmPanel />
          </div>

          <div className="space-y-6">
            <div className="ios-glass rounded-[24px] border border-black/[0.05] p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)] dark:border-white/[0.06]">
              <div className="mb-4 flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2">
                  <div
                    className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl ${healthIconClass[healthSummaryTone]}`}
                  >
                    {healthSummaryTone === "error" ? (
                      <XCircle className="h-4 w-4" strokeWidth={2.4} />
                    ) : healthSummaryTone === "warn" ? (
                      <TriangleAlert className="h-4 w-4" strokeWidth={2.4} />
                    ) : (
                      <CheckCircle2 className="h-4 w-4" strokeWidth={2.4} />
                    )}
                  </div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                        健康中心
                      </div>
                      <span
                        className={`rounded-full px-2 py-0.5 text-[10px] font-bold ${healthPillClass[healthSummaryTone]}`}
                      >
                        {healthSummaryText}
                      </span>
                    </div>
                    <div className="mt-0.5 text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                      {healthIssueCount > 0
                        ? `${healthIssueCount} 项需要关注，优先处理红色与黄色项。`
                        : "关键链路暂无高风险项。"}
                    </div>
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-1.5">
                  <button
                    type="button"
                    className="no-drag-region inline-flex items-center gap-1.5 rounded-full border border-black/[0.06] bg-white/70 px-2.5 py-1.5 text-[11px] font-semibold text-ios-text shadow-sm transition-all ios-btn hover:bg-black/[0.04] disabled:opacity-50 dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textDark"
                    disabled={troubleshootingLoading}
                    onClick={handleCopyTroubleshootingBundle}
                  >
                    <Copy
                      className={`h-3.5 w-3.5 ${
                        troubleshootingLoading ? "animate-spin" : ""
                      }`}
                      strokeWidth={2.4}
                    />
                    复制
                  </button>
                  <button
                    type="button"
                    className="no-drag-region rounded-full border border-black/[0.06] bg-white/70 px-2.5 py-1.5 text-[11px] font-semibold text-ios-text shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textDark"
                    onClick={() => setHealthExpanded((v) => !v)}
                  >
                    {healthExpanded ? "折叠" : "展开"}
                  </button>
                </div>
              </div>

              {healthExpanded ? (
                <>
                  <div className="mb-3 flex items-center justify-between gap-3 rounded-[14px] border border-black/[0.04] bg-black/[0.02] px-3 py-2 dark:border-white/[0.06] dark:bg-white/[0.03]">
                    <span className="text-[11px] font-semibold text-ios-textSecondary dark:text-ios-textSecondaryDark">
                      {healthOnlyIssues
                        ? `只显示异常项 · ${visibleHealthItems.length}/${healthItems.length}`
                        : `显示全部 · ${healthItems.length} 项`}
                    </span>
                    <button
                      type="button"
                      className="no-drag-region rounded-full bg-white/70 px-2.5 py-1 text-[10px] font-bold text-ios-text shadow-sm transition-all ios-btn hover:bg-white dark:bg-white/[0.08] dark:text-ios-textDark dark:hover:bg-white/[0.12]"
                      onClick={() => setHealthOnlyIssues((v) => !v)}
                    >
                      {healthOnlyIssues ? "显示全部" : "只看异常"}
                    </button>
                  </div>
                  <div className="space-y-2.5">
                    {visibleHealthItems.length === 0 ? (
                      <div className="rounded-[16px] border border-emerald-500/15 bg-emerald-500/[0.06] px-3 py-4 text-center text-[11px] font-semibold text-emerald-700 dark:text-emerald-300">
                        暂无异常项，可以切回“显示全部”查看完整状态。
                      </div>
                    ) : null}
                    {visibleHealthItems.map((item) => {
                      const Icon = item.icon;
                      return (
                        <div
                          key={item.key}
                          className={`rounded-[16px] border px-3 py-3 ${healthToneClass[item.tone]}`}
                        >
                          <div className="flex items-start gap-3">
                            <div
                              className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full ${healthIconClass[item.tone]}`}
                            >
                              <Icon className="h-3.5 w-3.5" strokeWidth={2.5} />
                            </div>
                            <div className="min-w-0 flex-1">
                              <div className="text-[12.5px] font-bold text-ios-text dark:text-ios-textDark">
                                {item.title}
                              </div>
                              <div className="mt-0.5 truncate text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                                {item.detail}
                              </div>
                            </div>
                            {item.actionLabel && item.onAction ? (
                              <button
                                type="button"
                                className="no-drag-region shrink-0 rounded-full bg-white/70 px-2.5 py-1 text-[10px] font-bold text-ios-text shadow-sm transition-all ios-btn hover:bg-white dark:bg-white/[0.08] dark:text-ios-textDark dark:hover:bg-white/[0.12]"
                                onClick={item.onAction}
                              >
                                {item.actionLabel}
                              </button>
                            ) : null}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </>
              ) : (
                <div className="rounded-[16px] border border-black/[0.04] bg-black/[0.02] px-3 py-3 text-[11px] leading-relaxed text-ios-textSecondary dark:border-white/[0.06] dark:bg-white/[0.03] dark:text-ios-textSecondaryDark">
                  已折叠 · {healthIssueCount > 0 ? `${healthIssueCount} 项需要关注` : "暂无异常项"}。
                </div>
              )}
            </div>

            {/* onboarding */}
            <div className="ios-glass rounded-[24px] border border-black/[0.05] p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)] dark:border-white/[0.06]">
              <div className="flex items-center gap-2">
                <div
                  className={`flex h-9 w-9 items-center justify-center rounded-2xl ${
                    allOnboardingDone
                      ? "bg-emerald-500/12 text-emerald-600 dark:text-emerald-300"
                      : "bg-ios-blue/12 text-ios-blue"
                  }`}
                >
                  {allOnboardingDone ? (
                    <CheckCircle2 className="h-4 w-4" strokeWidth={2.4} />
                  ) : (
                    <Activity className="h-4 w-4" strokeWidth={2.4} />
                  )}
                </div>
                <div>
                  <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                    {allOnboardingDone ? "已就绪 · 三步全部完成" : "三步上手"}
                  </div>
                  <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                    {allOnboardingDone
                      ? "MITM 已接管 IDE 流量,可以专注号池管理或对接外部客户端。"
                      : "按顺序点击下方步骤,每步都会跳到对应位置。"}
                  </div>
                </div>
              </div>

              <div className="mt-4 space-y-2">
                {onboardingSteps.map((step) => {
                  const Icon = step.icon;
                  return (
                    <button
                      key={step.key}
                      type="button"
                      className={[
                        "no-drag-region group flex w-full items-start gap-3 rounded-ios-block border px-3 py-3 text-left transition-all ios-btn",
                        step.done
                          ? "border-emerald-500/15 bg-emerald-500/[0.05] hover:bg-emerald-500/[0.08]"
                          : step.current
                            ? "border-ios-blue/30 bg-ios-blue/[0.06] shadow-[0_8px_24px_-14px_rgba(37,99,235,0.4)] hover:-translate-y-0.5 hover:bg-ios-blue/[0.10]"
                            : "border-black/[0.05] bg-black/[0.02] dark:border-white/[0.06] dark:bg-white/[0.03] hover:bg-black/[0.04] dark:hover:bg-white/[0.05]",
                      ].join(" ")}
                      onClick={step.onClick}
                    >
                      <span
                        className={[
                          "mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-[12px] font-black",
                          step.done
                            ? "bg-emerald-500 text-white"
                            : step.current
                              ? "bg-ios-blue text-white shadow-md shadow-ios-blue/30"
                              : "bg-black/[0.08] text-ios-textSecondary dark:bg-white/[0.1] dark:text-ios-textSecondaryDark",
                        ].join(" ")}
                      >
                        {step.done ? (
                          <CheckCircle2 className="h-4 w-4" strokeWidth={2.6} />
                        ) : (
                          <span>{step.index}</span>
                        )}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5 text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                          <Icon className="h-3.5 w-3.5 opacity-80" strokeWidth={2.5} />
                          {step.title}
                        </div>
                        <div className="mt-1 text-[11px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                          {step.description}
                        </div>
                      </div>
                      <div
                        className={[
                          "ml-auto inline-flex shrink-0 items-center gap-0.5 self-center text-[11px] font-bold transition-all",
                          step.done
                            ? "text-emerald-600 dark:text-emerald-300"
                            : step.current
                              ? "text-ios-blue dark:text-blue-300 group-hover:gap-1.5"
                              : "text-ios-textSecondary dark:text-ios-textSecondaryDark",
                        ].join(" ")}
                      >
                        {step.cta}
                        {!step.done ? (
                          <ChevronRight className="h-3.5 w-3.5" strokeWidth={2.5} />
                        ) : null}
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>

            {/* 快速跳转 */}
            <div className="ios-glass rounded-[24px] border border-black/[0.05] p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)] dark:border-white/[0.06]">
              <div className="mb-3 flex items-center gap-2">
                <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-sky-500/10 text-sky-600 dark:text-sky-300">
                  <ArrowRight className="h-4 w-4" strokeWidth={2.4} />
                </div>
                <div>
                  <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                    快速跳转
                  </div>
                  <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                    保留总览，但把重操作仍放回各自页面。
                  </div>
                </div>
              </div>

              <div className="space-y-2.5">
                {actionCards.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    className="no-drag-region flex w-full items-start justify-between gap-3 rounded-[18px] border border-black/[0.05] bg-white/70 px-4 py-3 text-left shadow-sm transition-all ios-btn hover:-translate-y-0.5 dark:border-white/[0.06] dark:bg-white/[0.04]"
                    onClick={() => setActiveTab(item.tab)}
                  >
                    <div>
                      <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                        {item.title}
                      </div>
                      <div className="mt-1 text-[11px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                        {item.body}
                      </div>
                    </div>
                    <ArrowRight
                      className="mt-0.5 h-4 w-4 shrink-0 text-ios-textSecondary dark:text-ios-textSecondaryDark"
                      strokeWidth={2.4}
                    />
                  </button>
                ))}
              </div>
            </div>

            {blockedAccounts > 0 ? (
              <div className="rounded-[20px] border border-amber-500/18 bg-amber-500/[0.07] px-4 py-3 text-[12px] leading-relaxed text-amber-800 dark:text-amber-300">
                <div className="flex items-start gap-3">
                  <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" strokeWidth={2.4} />
                  <div>
                    当前检测到 {blockedAccounts}{" "}
                    个账号处于"周额度阻断"状态。即使日额度看起来还有值，这类账号也不应再参与可用候选。
                  </div>
                </div>
              </div>
            ) : null}

            {criticalAccounts > 0 || expiredAccounts > 0 ? (
              <div className="rounded-[20px] border border-rose-500/18 bg-rose-500/[0.06] px-4 py-3 text-[12px] leading-relaxed text-rose-700 dark:text-rose-300">
                <div className="flex items-start gap-3">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" strokeWidth={2.4} />
                  <div>
                    {criticalAccounts > 0
                      ? `${criticalAccounts} 个账号额度告急；`
                      : ""}
                    {expiredAccounts > 0
                      ? `${expiredAccounts} 个账号已过期，建议到「号池」清理。`
                      : ""}
                  </div>
                </div>
              </div>
            ) : null}
          </div>
        </section>
      </div>

      {/* 诊断结果 modal */}
      {showDiagnostics && diagnostics ? (
        <div
          className="fixed inset-0 z-[150] flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-md p-4"
          onClick={(e) => {
            if (e.target === e.currentTarget) setShowDiagnostics(false);
          }}
        >
          <div className="w-full max-w-[640px] max-h-[80vh] flex flex-col bg-white dark:bg-[#1c1c1e] rounded-ios-card shadow-ios-sheet ring-1 ring-white/50 dark:ring-white/10 overflow-hidden">
            <div className="flex items-center justify-between gap-4 px-5 py-4 border-b border-black/[0.06] dark:border-white/[0.06]">
              <div>
                <h2 className="text-[16px] font-bold text-ios-text dark:text-ios-textDark">
                  平台兼容性检查
                </h2>
                <p className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                  {diagnostics.platform} · {diagnostics.arch} · 通过{" "}
                  {diagnostics.ok}，警告 {diagnostics.warn}，错误{" "}
                  {diagnostics.error}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <button
                  type="button"
                  className="no-drag-region inline-flex items-center gap-1.5 rounded-full border border-black/[0.06] bg-white/80 px-3 py-1.5 text-[11px] font-semibold text-ios-text shadow-sm hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textDark dark:hover:bg-white/[0.08] ios-btn"
                  onClick={handleCopyDiagnostics}
                >
                  <Copy className="h-3.5 w-3.5" strokeWidth={2.4} />
                  复制报告
                </button>
                <button
                  type="button"
                  className="rounded-full p-2 hover:bg-black/5 dark:hover:bg-white/10 ios-btn"
                  onClick={() => setShowDiagnostics(false)}
                  aria-label="关闭诊断报告"
                >
                  <X className="h-4 w-4" strokeWidth={2.4} />
                </button>
              </div>
            </div>
            <div className="flex-1 overflow-y-auto p-5 space-y-3">
              {diagnostics.checks.map((c) => {
                const actions = diagnosticActionsFor(c);
                return (
                  <div
                    key={c.id}
                    className={`rounded-[14px] border p-3 ${diagnoseStatusClass(c.status)}`}
                  >
                    <div className="flex items-start gap-2">
                      <div className="mt-0.5">
                        {c.status === "ok" ? (
                          <CheckCircle2
                            className="h-4 w-4 text-emerald-600 dark:text-emerald-300"
                            strokeWidth={2.4}
                          />
                        ) : c.status === "error" ? (
                          <XCircle
                            className="h-4 w-4 text-rose-600 dark:text-rose-300"
                            strokeWidth={2.4}
                          />
                        ) : c.status === "warn" ? (
                          <TriangleAlert
                            className="h-4 w-4 text-amber-600 dark:text-amber-300"
                            strokeWidth={2.4}
                          />
                        ) : (
                          <AlertCircle
                            className="h-4 w-4 text-gray-500 dark:text-gray-400"
                            strokeWidth={2.4}
                          />
                        )}
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                            {c.title}
                          </div>
                          <span className="rounded-full bg-black/[0.04] px-2 py-0.5 text-[10px] font-mono text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark">
                            {c.id}
                          </span>
                        </div>
                        <div className="mt-1 text-[11.5px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                          {c.detail}
                        </div>
                        {c.fix_hint ? (
                          <div className="mt-2 rounded-[10px] bg-black/[0.04] dark:bg-white/[0.06] px-2.5 py-1.5 text-[11px] text-ios-text dark:text-ios-textDark">
                            💡 {c.fix_hint}
                          </div>
                        ) : null}
                        <div className="mt-2.5 flex flex-wrap gap-1.5">
                          {actions.map((action) => (
                            <button
                              key={action.key}
                              type="button"
                              className="no-drag-region rounded-full border border-black/[0.06] bg-white/70 px-2.5 py-1 text-[10px] font-bold text-ios-text shadow-sm transition-all ios-btn hover:bg-white dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textDark dark:hover:bg-white/[0.10]"
                              onClick={action.onClick}
                            >
                              {action.label}
                            </button>
                          ))}
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      ) : null}
    </SkeletonOverlay>
  );
}

// ── 子组件：提供商概览（仅 routeMode=providers 时显示） ──

interface ProviderBucket {
  id: ProviderID;
  label: string;
  badge: string;
  accent: string;
  total: number;
  ready: number;
}

function ProviderRouteOverview() {
  const settings = useSettingsStore((s) => s.settings);
  const setActiveTab = useMainViewStore((s) => s.setActiveTab);
  const accounts = useProviderAccountStore((s) => s.accounts);
  const ensureAccountsLoaded = useProviderAccountStore((s) => s.ensureAccountsLoaded);

  const routeMode: 'pool' | 'providers' =
    (settings as any)?.mitm_route_mode === 'providers' ? 'providers' : 'pool';

  useEffect(() => {
    if (routeMode === 'providers') {
      void ensureAccountsLoaded();
    }
  }, [routeMode, ensureAccountsLoaded]);

  const buckets = useMemo<ProviderBucket[]>(() => {
    const map = new Map<ProviderID, { total: number; ready: number }>();
    for (const id of PROVIDER_DISPLAY_ORDER) {
      map.set(id, { total: 0, ready: 0 });
    }
    for (const acc of accounts) {
      const provider = String(acc.provider || '').toLowerCase() as ProviderID;
      const bucket = map.get(provider);
      if (!bucket) continue;
      bucket.total++;
      const hasToken = Boolean(String(acc.auth_token || '').trim());
      const active = String(acc.status || 'active') !== 'disabled';
      if (hasToken && active) bucket.ready++;
    }
    return PROVIDER_DISPLAY_ORDER.map((id) => {
      const meta = PROVIDER_META[id];
      const stat = map.get(id) ?? { total: 0, ready: 0 };
      return { id, label: meta.label, badge: meta.badge, accent: meta.accent, total: stat.total, ready: stat.ready };
    });
  }, [accounts]);

  const total = buckets.reduce((s, b) => s + b.total, 0);
  const ready = buckets.reduce((s, b) => s + b.ready, 0);
  const visible = buckets.filter((b) => b.total > 0);
  const goProviders = () => setActiveTab('Providers');

  if (routeMode !== 'providers') return null;

  return (
    <section className="ios-glass overflow-hidden rounded-[28px] border border-black/[0.05] dark:border-white/[0.06]">
      <header className="flex flex-wrap items-start justify-between gap-3 border-b border-black/[0.04] px-6 py-4 dark:border-white/[0.06]">
        <div>
          <h2 className="text-[16px] font-bold text-ios-text dark:text-ios-textDark">
            提供商概览
          </h2>
          <p className="mt-1 text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
            已切到「提供商」接管 — IDE chat 请求被 MITM 翻译给已激活的卡片 ({ready} / {total} 可用)。
            <span className="text-amber-700 dark:text-amber-300 font-semibold">
              {' '}需要至少 1 张激活卡 + 设了 active_model 才能跑通。
            </span>
          </p>
        </div>
        <button
          type="button"
          className="no-drag-region inline-flex items-center gap-1.5 rounded-full bg-black/[0.04] px-3 py-1.5 text-[12px] font-bold text-ios-text dark:bg-white/[0.06] dark:text-ios-textDark hover:bg-black/[0.08] dark:hover:bg-white/[0.1] ios-btn"
          onClick={goProviders}
        >
          管理提供商 →
        </button>
      </header>

      {total === 0 ? (
        <div className="px-6 py-8 text-center text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
          号池里还没有提供商账号 —
          <button
            type="button"
            className="ml-1 font-semibold text-ios-blue ios-btn"
            onClick={goProviders}
          >
            前往「提供商」批量导入
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 p-6 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
          {visible.map((bucket) => (
            <article
              key={bucket.id}
              className="rounded-[18px] border border-black/[0.05] bg-white/80 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]"
            >
              <div className="flex items-center justify-between gap-2">
                <span className={`rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.16em] ${bucket.badge}`}>
                  {bucket.label}
                </span>
                <span className="rounded-full bg-black/[0.05] px-2 py-0.5 text-[10px] font-mono font-bold text-ios-textSecondary dark:bg-white/[0.08]">
                  {bucket.ready}/{bucket.total}
                </span>
              </div>
              <div className={`mt-3 h-1 rounded-full bg-gradient-to-r ${bucket.accent}`} />
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
