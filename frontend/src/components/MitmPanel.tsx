import { useMemo, useState } from "react";
import {
  AlertTriangle,
  ArrowRightLeft,
  CheckCircle,
  Download,
  Search,
  KeyRound,
  Link2,
  Loader2,
  Power,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Wand2,
  Wrench,
  XCircle,
} from "lucide-react";
import SessionPanel from "./SessionPanel";
import IToggle from "./ios/IToggle";
import IInfoTooltip from "./ios/IInfoTooltip";
import { APIInfo } from "../api/wails";
import { useMitmStatusStore } from "../stores/useMitmStatusStore";
import { confirmDialog, showToast, showErrorToast } from "../utils/toast";
import { formatDateTimeAsiaShanghai } from "../utils/datetimeAsia";
import { usePersistentMitmEvents } from "../hooks/usePersistentMitmEvents";

type PrereqStepResult = {
  step: string;
  title: string;
  ok: boolean;
  skipped: boolean;
  error?: string;
  hint?: string;
};

type RecentMitmEvent = { at?: string; message?: string; tone?: string };

const ERROR_LABEL_MAP: Record<string, string> = {
  quota: "额度错误",
  internal: "上游内部错误",
  permission: "权限错误",
  grpc: "gRPC 错误",
};

const isMac =
  typeof navigator !== "undefined" &&
  /Mac|iPhone|iPad|iPod/.test(navigator.platform);

function recentEventToneClass(tone?: string): string {
  switch (tone) {
    case "success":
      return "border-emerald-500/15 bg-emerald-500/[0.06] text-emerald-700 dark:text-emerald-300";
    case "warning":
      return "border-amber-500/15 bg-amber-500/[0.06] text-amber-700 dark:text-amber-300";
    case "error":
      return "border-rose-500/15 bg-rose-500/[0.06] text-rose-700 dark:text-rose-300";
    default:
      return "border-black/[0.05] bg-black/[0.03] text-ios-text dark:border-white/[0.06] dark:bg-white/[0.03] dark:text-ios-textDark";
  }
}

/**
 * MitmPanel — Vue 1:1 完整功能迁移。
 * 状态条 / 启停 / 切下一席 / 上游错误卡 / 最近事件 / Setup all /
 * CA + Hosts 卡 + 卸载 / 号池活跃状态 / SessionPanel / Teardown。
 */
export default function MitmPanel() {
  const status = useMitmStatusStore((s) => s.status);
  const switchLoading = useMitmStatusStore((s) => s.switchLoading);
  const switchToNext = useMitmStatusStore((s) => s.switchToNext);
  const fetchStatus = useMitmStatusStore((s) => s.fetchStatus);

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [stepBusy, setStepBusy] = useState("");
  const [prereqResults, setPrereqResults] = useState<PrereqStepResult[]>([]);

  const poolCount = status?.pool_status?.length ?? 0;
  const healthyKeys =
    status?.pool_status?.filter((i) => i.healthy).length ?? 0;
  const runtimeExhaustedKeys =
    status?.pool_status?.filter((i) => i.runtime_exhausted).length ?? 0;
  const activeKey = status?.pool_status?.find((i) => i.is_current) ?? null;

  const lastResultByStep = useMemo(() => {
    const m: Record<string, PrereqStepResult | undefined> = {};
    for (const r of prereqResults) m[r.step] = r;
    return m;
  }, [prereqResults]);

  // 3.4: 后端只返回最近 5-8 条事件 + 进程重启即丢。这里把每次拉到的事件合并到
  // localStorage（最多 200 条），按 (at, message, tone) 去重，让用户能看完整历史。
  const rawRecentEvents: RecentMitmEvent[] = useMemo(() => {
    const raw = (status as unknown as { recent_events?: RecentMitmEvent[] } | null)
      ?.recent_events;
    return Array.isArray(raw) ? raw : [];
  }, [status]);
  const { events: persistedEvents, clear: clearPersistedEvents } =
    usePersistentMitmEvents(rawRecentEvents);
  const [eventToneFilter, setEventToneFilter] = useState<
    "all" | "success" | "info" | "warning" | "error"
  >("all");
  // 3.1: 实时日志搜索
  const [eventSearchQuery, setEventSearchQuery] = useState("");
  // 倒序展示（最新在上），按 tone + 文本过滤
  const recentEvents = useMemo(() => {
    let list = [...persistedEvents].reverse();
    if (eventToneFilter !== "all") {
      list = list.filter((e) => (e.tone || "info") === eventToneFilter);
    }
    const q = eventSearchQuery.trim().toLowerCase();
    if (q) {
      list = list.filter((e) =>
        (e.message || "").toLowerCase().includes(q) ||
        (e.tone || "").toLowerCase().includes(q),
      );
    }
    return list;
  }, [persistedEvents, eventToneFilter, eventSearchQuery]);

  // 3.1: 导出全部 events 为 .txt（按时间正序，每行一条）
  const handleExportEvents = () => {
    const lines = persistedEvents.map((e) => {
      const ts = e.at ? formatDateTimeAsiaShanghai(e.at) : "(no-ts)";
      const tone = (e.tone || "info").toUpperCase();
      const msg = (e.message || "").replace(/\n/g, " ");
      return `[${ts}] [${tone}] ${msg}`;
    });
    const text = lines.join("\n") + "\n";
    const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    const stamp = new Date()
      .toISOString()
      .replace(/[:.]/g, "-")
      .slice(0, 19);
    a.href = url;
    a.download = `mitm-events-${stamp}.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    showToast(`已导出 ${persistedEvents.length} 条事件到 ${a.download}`, "success");
  };
  const eventToneCounts = useMemo(() => {
    const c: Record<string, number> = {
      all: persistedEvents.length,
      success: 0,
      info: 0,
      warning: 0,
      error: 0,
    };
    for (const e of persistedEvents) {
      const t = e.tone || "info";
      if (t in c) c[t]++;
    }
    return c;
  }, [persistedEvents]);

  const lastProxyIssue = useMemo(() => {
    const rawKind = String(status?.last_error_kind || "").trim();
    const summary = String(status?.last_error_summary || "").trim();
    if (!rawKind && !summary) return null;
    return {
      kind: rawKind || "unknown",
      label: ERROR_LABEL_MAP[rawKind] || "未知错误",
      summary: summary || "未提供更多细节",
      key: String(status?.last_error_key || "").trim(),
      at: String(status?.last_error_at || "").trim(),
    };
  }, [status]);

  const lastProxyIssueTone = (() => {
    switch (lastProxyIssue?.kind) {
      case "quota":
        return "border-amber-500/20 bg-amber-500/[0.07] text-amber-700 dark:text-amber-300";
      case "permission":
        return "border-rose-500/20 bg-rose-500/[0.07] text-rose-700 dark:text-rose-300";
      case "internal":
        return "border-orange-500/20 bg-orange-500/[0.07] text-orange-700 dark:text-orange-300";
      default:
        return "border-slate-500/20 bg-slate-500/[0.07] text-slate-700 dark:text-slate-300";
    }
  })();

  const activeKeyLabel =
    activeKey?.nickname ||
    activeKey?.email ||
    activeKey?.key_short ||
    "";
  const statusTone = status?.running
    ? {
        chip: "bg-emerald-500/12 text-emerald-700 dark:text-emerald-300",
        panel: "border-emerald-500/15 bg-emerald-500/[0.07]",
        dot: "bg-emerald-500",
        label: "代理运行中",
        detail: activeKeyLabel
          ? `当前活跃 ${activeKeyLabel}`
          : "流量已接入本机 MITM",
      }
    : {
        chip: "bg-slate-500/12 text-slate-700 dark:text-slate-300",
        panel:
          "border-black/[0.06] bg-black/[0.03] dark:border-white/[0.08] dark:bg-white/[0.04]",
        dot: "bg-slate-400 dark:bg-slate-500",
        label: "代理未启动",
        detail: "启动后会按号池顺序轮换 JWT / API Key，请先确认 CA 与 Hosts。",
      };

  // ── handlers ───
  const handleToggle = async (on: boolean) => {
    setLoading(true);
    setError("");
    try {
      if (on) await APIInfo.startMitmProxy();
      else await APIInfo.stopMitmProxy();
      await fetchStatus(true);
    } catch (e) {
      showErrorToast(e, on ? "启动代理失败" : "停止代理失败");
    } finally {
      setLoading(false);
    }
  };

  const handleSwitchToNext = async () => {
    setError("");
    try {
      const target = await switchToNext();
      showToast(`MITM 已手动切到下一席位：${target || "已切换"}`, "success");
    } catch (e) {
      setError(`手动切换失败: ${String(e)}`);
    }
  };

  const runStep = async (
    step: string,
    fn: () => Promise<unknown>,
    successMsg: string,
    errPrefix: string,
    macToast?: string,
  ) => {
    setLoading(true);
    setStepBusy(step);
    setError("");
    if (isMac && macToast) showToast(macToast, "info");
    try {
      await fn();
      await fetchStatus(true);
      setPrereqResults((prev) => prev.filter((r) => r.step !== step));
      showToast(successMsg, "success");
    } catch (e) {
      setError(`${errPrefix}: ${String(e)}`);
    } finally {
      setLoading(false);
      setStepBusy("");
    }
  };

  // CA 安装前的人话说明:第一次装证书会触发系统密码弹窗(mac 弹 Terminal、
  // Windows 弹 UAC),提前告知避免用户看到突然弹窗以为中毒/误操作。
  const caInstallNotice = isMac
    ? "接下来会弹出一个「终端」黑色窗口，要求输入你的电脑开机密码。\n\n这是 macOS 安装本地证书的正常步骤，密码只在你本机使用、不会上传。输入后按回车，窗口会自动关闭。"
    : "接下来系统可能弹出权限确认（UAC），用于把本地证书装进系统信任库。\n\n证书只在你本机使用，不会上传。请允许继续。";

  const handleSetupCA = async () => {
    const ok = await confirmDialog(caInstallNotice, {
      confirmText: "我知道了，继续",
      cancelText: "取消",
    });
    if (!ok) return;
    await runStep(
      "ca",
      () => APIInfo.setupMitmCA(),
      "CA 证书已生成并安装到系统信任库",
      "CA 安装失败",
      "正在弹出 Terminal 安装 CA 信任，请在终端窗口里输入登录密码后回车",
    );
  };
  const handleSetupHosts = () =>
    runStep(
      "hosts",
      () => APIInfo.setupMitmHosts(),
      "Hosts 已配置",
      "Hosts 配置失败（Linux 会尝试 pkexec/sudo 提权）",
    );

  const handleSetupAll = async () => {
    const ok = await confirmDialog(caInstallNotice, {
      confirmText: "我知道了，继续",
      cancelText: "取消",
    });
    if (!ok) return;
    setLoading(true);
    setStepBusy("all");
    setError("");
    if (isMac) {
      showToast(
        "macOS 上 CA 信任会弹出 Terminal 索取登录密码，输入后回车即可",
        "info",
      );
    }
    try {
      const results = (await APIInfo.setupMitmAll()) as PrereqStepResult[];
      setPrereqResults(results);
      await fetchStatus(true);
      const failed = results.filter((r) => !r.ok);
      if (failed.length === 0) {
        const skippedAll = results.every((r) => r.skipped);
        showToast(
          skippedAll ? "前置条件已就绪，无需修改" : "前置条件全部就绪",
          "success",
        );
      } else {
        const first = failed[0]!;
        setError(
          `${first.title}: ${first.error || first.hint || "未提供详细错误"}`,
        );
      }
    } catch (e) {
      setError(`一键就绪失败: ${String(e)}`);
    } finally {
      setLoading(false);
      setStepBusy("");
    }
  };

  const handleUninstallCA = async () => {
    const ok = await confirmDialog(
      "卸载 CA 信任？卸载后浏览器/IDE 将不再信任本机 MITM 证书",
      { confirmText: "卸载", cancelText: "取消", destructive: true },
    );
    if (!ok) return;
    await runStep(
      "uninstall-ca",
      () => APIInfo.uninstallMitmCA(),
      "CA 信任已卸载",
      "CA 卸载失败",
    );
    setPrereqResults((prev) => prev.filter((r) => r.step !== "ca"));
  };

  const handleUninstallHosts = async () => {
    const ok = await confirmDialog("移除 Hosts 劫持？流量将不再经本机 MITM", {
      confirmText: "移除",
      cancelText: "取消",
      destructive: true,
    });
    if (!ok) return;
    await runStep(
      "uninstall-hosts",
      () => APIInfo.uninstallMitmHosts(),
      "Hosts 劫持已移除",
      "Hosts 移除失败",
    );
    setPrereqResults((prev) => prev.filter((r) => r.step !== "hosts"));
  };

  const handleTeardown = async () => {
    const ok = await confirmDialog(
      "确认卸载？将停止代理、移除 hosts 和 CA 证书",
      { confirmText: "卸载", cancelText: "取消", destructive: true },
    );
    if (!ok) return;
    setLoading(true);
    setError("");
    try {
      await APIInfo.teardownMitm();
      await fetchStatus(true);
      showToast("已卸载完成", "success");
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const StatusIcon = status?.running ? ShieldCheck : Shield;

  const setupCards = [
    {
      key: "ca",
      title: "CA 证书",
      tip: "一张本地根证书,装进系统信任库后,本工具才能解密并改写 Windsurf 的 HTTPS 请求。只在你本机使用,可随时一键卸载。",
      ready: status?.ca_installed === true,
      onClick: handleSetupCA,
      onUninstall: handleUninstallCA,
      sR: "系统已信任",
      sP: "点击安装到系统信任库",
    },
    {
      key: "hosts",
      title: "Hosts 劫持",
      tip: "把 Windsurf 的服务器域名在本机指向 127.0.0.1,这样请求才会先经过本工具。卸载时会自动还原,不影响你 hosts 里的其它内容。",
      ready: status?.hosts_mapped === true,
      onClick: handleSetupHosts,
      onUninstall: handleUninstallHosts,
      sR: "域名已指向本机 MITM",
      sP: "点击写入 hosts 映射",
    },
  ];

  return (
    <div className="ios-glass rounded-ios-card border border-black/[0.05] dark:border-white/[0.06] overflow-hidden shadow-[0_20px_48px_-20px_rgba(15,23,42,0.28)]">
      <div className="border-b border-black/[0.05] dark:border-white/[0.06] px-6 py-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            <div
              className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl shadow-inner ${
                status?.running
                  ? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-300"
                  : "bg-ios-blue/10 text-ios-blue"
              }`}
            >
              <StatusIcon className="h-5 w-5" strokeWidth={2.4} />
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="text-[17px] font-bold text-ios-text dark:text-ios-textDark">
                  MITM 无感换号代理
                </h2>
                <span
                  className={`rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide ${statusTone.chip}`}
                >
                  {statusTone.label}
                </span>
              </div>
              <p className="mt-1 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                当前产品默认走纯 MITM：流量经本机代理轮换 JWT / API Key，号池与 IDE
                客户端无感对接。
              </p>
            </div>
          </div>
        </div>
      </div>

      <div className="space-y-5 p-6">
        <div className={`rounded-[20px] border px-4 py-3 shadow-sm ${statusTone.panel}`}>
          <div className="flex flex-wrap items-center gap-3">
            <span className={`inline-block h-2 w-2 rounded-full ${statusTone.dot}`} />
            <div className="min-w-0 flex-1">
              <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                {statusTone.label}
              </div>
              <div className="mt-0.5 text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                {statusTone.detail}
              </div>
            </div>
            <div className="flex items-center gap-2">
              <button
                type="button"
                className="no-drag-region inline-flex items-center gap-1.5 rounded-full border border-ios-blue/20 bg-ios-blue/[0.08] px-3 py-1.5 text-[12px] font-semibold text-ios-blue transition-colors ios-btn hover:bg-ios-blue/[0.14] disabled:opacity-50 dark:text-blue-300"
                disabled={loading || switchLoading || poolCount === 0}
                onClick={handleSwitchToNext}
              >
                <ArrowRightLeft
                  className={`h-3.5 w-3.5 ${switchLoading ? "animate-pulse" : ""}`}
                  strokeWidth={2.4}
                />
                下一席位
              </button>
              <IInfoTooltip size={12} maxWidth={240}>
                立即手动切换到号池里的下一个可用账号（平时额度用尽会自动切，
                这里是手动加速）。
              </IInfoTooltip>
              <IToggle
                modelValue={Boolean(status?.running)}
                onValueChange={handleToggle}
                disabled={
                  loading ||
                  switchLoading ||
                  (!status?.running &&
                    (!status?.ca_installed || !status?.hosts_mapped))
                }
              />
            </div>
          </div>
        </div>

        {lastProxyIssue ? (
          <div className={`rounded-[18px] border px-4 py-3 shadow-sm ${lastProxyIssueTone}`}>
            <div className="flex items-start gap-3">
              <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-black/[0.05] dark:bg-white/[0.08]">
                <AlertTriangle className="h-4 w-4" strokeWidth={2.4} />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <div className="text-[13px] font-bold">最近一次上游错误</div>
                  <span className="rounded-full bg-black/[0.05] px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide dark:bg-white/[0.08]">
                    {lastProxyIssue.label}
                  </span>
                  {lastProxyIssue.key ? (
                    <span className="rounded-full bg-black/[0.05] px-2 py-0.5 text-[10px] font-mono dark:bg-white/[0.08]">
                      {lastProxyIssue.key}
                    </span>
                  ) : null}
                </div>
                <div className="mt-1 text-[12px] leading-relaxed break-words">
                  {lastProxyIssue.summary}
                </div>
                {lastProxyIssue.at ? (
                  <div className="mt-2 text-[11px] opacity-80">
                    {formatDateTimeAsiaShanghai(lastProxyIssue.at)}
                  </div>
                ) : null}
              </div>
            </div>
          </div>
        ) : null}

        {persistedEvents.length > 0 ? (
          <div className="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-black/[0.04] text-ios-textSecondary dark:text-ios-textSecondaryDark dark:bg-white/[0.06] ">
                  <Sparkles className="h-4 w-4" strokeWidth={2.4} />
                </div>
                <div>
                  <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                    实时事件日志
                  </div>
                  <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                    本地保留最近 200 条 · 搜索 / 筛选 / 导出 · 进程重启不丢
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  className="no-drag-region inline-flex items-center gap-1 rounded-full bg-ios-blue/10 px-2.5 py-1 text-[10px] font-bold text-ios-blue transition-colors hover:bg-ios-blue/15"
                  onClick={handleExportEvents}
                  title="导出全部事件为 .txt 文件"
                >
                  <Download className="h-3 w-3" strokeWidth={2.6} />
                  导出
                </button>
                <button
                  type="button"
                  className="no-drag-region rounded-full px-2.5 py-1 text-[10px] font-bold text-rose-600 hover:bg-rose-500/10 transition-colors dark:text-rose-300"
                  onClick={clearPersistedEvents}
                  title="清空所有事件历史"
                >
                  清空
                </button>
              </div>
            </div>

            {/* 3.1: 搜索框 */}
            <div className="mb-2 relative">
              <Search
                className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3 w-3 text-ios-textSecondary dark:text-ios-textSecondaryDark"
                strokeWidth={2.6}
              />
              <input
                type="search"
                value={eventSearchQuery}
                onChange={(e) => setEventSearchQuery(e.target.value)}
                placeholder="搜索事件文本…"
                className="no-drag-region w-full rounded-[10px] border border-black/[0.06] bg-white/70 dark:border-white/[0.08] dark:bg-white/[0.05] pl-7 pr-2.5 py-1.5 text-[11.5px] text-ios-text dark:text-ios-textDark focus:outline-none focus:ring-2 focus:ring-ios-blue/30"
              />
            </div>

            {/* 3.4: tone 筛选 chips */}
            <div className="mb-3 flex flex-wrap gap-1.5">
              {(
                [
                  { key: "all", label: "全部", color: "bg-ios-blue/10 text-ios-blue" },
                  { key: "success", label: "成功", color: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300" },
                  { key: "info", label: "信息", color: "bg-sky-500/10 text-sky-700 dark:text-sky-300" },
                  { key: "warning", label: "警告", color: "bg-amber-500/10 text-amber-700 dark:text-amber-300" },
                  { key: "error", label: "错误", color: "bg-rose-500/10 text-rose-700 dark:text-rose-300" },
                ] as const
              ).map((c) => {
                const active = eventToneFilter === c.key;
                const count = eventToneCounts[c.key] ?? 0;
                return (
                  <button
                    key={c.key}
                    type="button"
                    className={[
                      "no-drag-region inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[10px] font-bold transition-all",
                      active
                        ? c.color + " ring-2 ring-ios-blue/40"
                        : "bg-black/[0.04] text-ios-textSecondary dark:text-ios-textSecondaryDark hover:bg-black/[0.06] dark:bg-white/[0.06] dark:hover:bg-white/[0.08]",
                    ].join(" ")}
                    onClick={() => setEventToneFilter(c.key)}
                  >
                    {c.label}
                    <span
                      className={[
                        "rounded-full px-1 text-[9px] tabular-nums",
                        active ? "bg-white/40 dark:bg-black/30" : "opacity-60",
                      ].join(" ")}
                    >
                      {count}
                    </span>
                  </button>
                );
              })}
            </div>

            <div className="space-y-2 max-h-56 overflow-y-auto pr-1">
              {recentEvents.length === 0 ? (
                <div className="px-3 py-4 text-center text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                  当前 tone 没有事件
                </div>
              ) : null}
              {recentEvents.map((event, index) => (
                <div
                  key={`${event.at || "mitm"}-${index}`}
                  className={`rounded-ios-block border px-3 py-2.5 ${recentEventToneClass(event.tone)}`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="break-words text-[12px] font-medium leading-relaxed">
                        {event.message || "未提供事件详情"}
                      </div>
                      {event.at ? (
                        <div
                          className="mt-1 text-[10px] opacity-80"
                          title={formatDateTimeAsiaShanghai(event.at)}
                        >
                          {formatDateTimeAsiaShanghai(event.at)}
                        </div>
                      ) : null}
                    </div>
                    <span className="shrink-0 rounded-full px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide bg-black/[0.05] dark:bg-white/[0.08]">
                      {event.tone || "info"}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : null}

        <div className="rounded-[18px] border border-ios-blue/20 bg-ios-blue/[0.06] px-4 py-3 text-[12px] leading-relaxed text-ios-text dark:text-ios-textDark">
          Windows 桌面包现在默认请求管理员权限启动，便于直接管理 Hosts、安装 CA
          证书和控制后台服务，避免运行中再因为权限不足中断流程。
        </div>

        <div className="space-y-3">
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-black/[0.04] text-ios-textSecondary dark:text-ios-textSecondaryDark dark:bg-white/[0.06] ">
                <Wrench className="h-4 w-4" strokeWidth={2.4} />
              </div>
              <div>
                <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                  前置条件
                </div>
                <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                  证书与 hosts 这两步完成后，MITM 路径才会真正接管流量。
                </div>
              </div>
            </div>
            <button
              type="button"
              className="no-drag-region inline-flex items-center gap-2 rounded-full border border-ios-blue/20 bg-ios-blue/[0.08] px-3.5 py-2 text-[12px] font-semibold text-ios-blue transition-colors ios-btn hover:bg-ios-blue/[0.14] disabled:opacity-50 dark:text-blue-300"
              disabled={
                loading ||
                (status?.ca_installed === true && status?.hosts_mapped === true)
              }
              title={
                status?.ca_installed && status?.hosts_mapped
                  ? "前置条件已就绪"
                  : "顺序安装 CA + Hosts，单次密码弹窗"
              }
              onClick={handleSetupAll}
            >
              {stepBusy === "all" ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={2.4} />
              ) : (
                <Wand2 className="h-3.5 w-3.5" strokeWidth={2.4} />
              )}
              一键就绪
            </button>
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {setupCards.map((item) => {
              const r = lastResultByStep[item.key];
              return (
                <div
                  key={item.key}
                  data-health-anchor={`mitm-${item.key}`}
                  className={`rounded-[18px] border px-4 py-3 shadow-sm transition-all ${
                    item.ready
                      ? "border-emerald-500/15 bg-emerald-500/[0.06]"
                      : "border-amber-500/15 bg-amber-500/[0.06]"
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                        {item.title}
                        {item.tip ? (
                          <IInfoTooltip size={12} maxWidth={260}>
                            {item.tip}
                          </IInfoTooltip>
                        ) : null}
                      </div>
                      <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-0.5">
                        {item.ready ? item.sR : item.sP}
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-1.5">
                      {item.ready ? (
                        <button
                          type="button"
                          className="no-drag-region inline-flex items-center gap-1 rounded-full border border-rose-500/15 bg-rose-500/[0.06] px-2.5 py-1 text-[11px] font-bold text-rose-700 dark:text-rose-300 transition-colors hover:bg-rose-500/[0.12] disabled:opacity-50"
                          disabled={loading}
                          onClick={item.onUninstall}
                        >
                          {stepBusy === `uninstall-${item.key}` ? (
                            <Loader2 className="h-3 w-3 animate-spin" strokeWidth={2.4} />
                          ) : null}
                          卸载
                        </button>
                      ) : (
                        <button
                          type="button"
                          className="no-drag-region inline-flex items-center gap-1 rounded-full border border-ios-blue/20 bg-ios-blue/[0.08] px-2.5 py-1 text-[11px] font-bold text-ios-blue dark:text-blue-300 transition-colors hover:bg-ios-blue/[0.14] disabled:opacity-50"
                          disabled={loading}
                          onClick={item.onClick}
                        >
                          {stepBusy === item.key ? (
                            <Loader2 className="h-3 w-3 animate-spin" strokeWidth={2.4} />
                          ) : null}
                          安装
                        </button>
                      )}
                    </div>
                  </div>
                  {r && !r.ok ? (
                    <div className="mt-2.5 rounded-[12px] border border-rose-500/15 bg-rose-500/[0.06] px-3 py-1.5 text-[11px] text-rose-700 dark:text-rose-300 leading-relaxed">
                      {r.error || r.hint || "未提供详细信息"}
                    </div>
                  ) : null}
                  {r && r.ok && r.skipped ? (
                    <div className="mt-2.5 rounded-[12px] border border-emerald-500/15 bg-emerald-500/[0.05] px-3 py-1.5 text-[11px] text-emerald-700 dark:text-emerald-300">
                      已就绪，无需重复安装
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>
        </div>

        {status?.pool_status?.length ? (
          <div className="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-ios-blue/10 text-ios-blue">
                  <KeyRound className="h-4 w-4" strokeWidth={2.4} />
                </div>
                <div>
                  <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                    号池活跃状态
                  </div>
                  <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                    当前活跃 key 会优先标记，便于确认轮换是否生效。
                  </div>
                </div>
              </div>
              <span className="rounded-full bg-black/[0.04] px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-ios-textSecondary dark:text-ios-textSecondaryDark dark:bg-white/[0.06] ">
                {poolCount} keys · 健康 {healthyKeys}
                {runtimeExhaustedKeys > 0 ? ` · 见底 ${runtimeExhaustedKeys}` : ""}
              </span>
            </div>
            <div className="space-y-2 max-h-56 overflow-y-auto pr-1">
              {status.pool_status.map((k) => {
                const cls =
                  k.is_current && k.healthy && !k.runtime_exhausted
                    ? "border-emerald-500/15 bg-emerald-500/[0.07]"
                    : k.runtime_exhausted
                      ? "border-amber-500/15 bg-amber-500/[0.08]"
                      : !k.healthy
                        ? "border-rose-500/15 bg-rose-500/[0.06]"
                        : "border-black/[0.05] bg-black/[0.03] dark:border-white/[0.06] dark:bg-white/[0.03]";
                const dot = k.runtime_exhausted
                  ? "bg-amber-500"
                  : k.healthy && k.has_jwt
                    ? "bg-emerald-500"
                    : k.healthy
                      ? "bg-sky-500"
                      : "bg-rose-500";
                return (
                  <div
                    key={k.key_hash || k.key_short}
                    className={`flex items-center justify-between gap-3 rounded-ios-block border px-3 py-2.5 text-[12px] font-mono transition-all ${cls}`}
                  >
                    <div className="flex min-w-0 items-center gap-2.5">
                      <span className={`h-2 w-2 rounded-full shrink-0 ${dot}`} />
                      <span
                        className="truncate text-ios-text dark:text-ios-textDark"
                        title={k.key_short}
                      >
                        {k.nickname || k.email || k.key_short}
                      </span>
                      {(k.nickname || k.email) && k.key_short ? (
                        <span
                          className="hidden md:inline shrink-0 text-[10px] font-mono opacity-50"
                          title="API Key 短哈希"
                        >
                          {k.key_short}
                        </span>
                      ) : null}
                      {k.is_current ? (
                        <span className="rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-emerald-700 dark:text-emerald-300">
                          ACTIVE
                        </span>
                      ) : null}
                      {k.runtime_exhausted ? (
                        <span className="inline-flex items-center rounded-full bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold tracking-wide text-amber-700 dark:text-amber-300">
                          额度用尽
                          <IInfoTooltip size={11} maxWidth={240}>
                            这个账号本周期额度已用完，已被自动跳过、不再分配新对话。
                            等官方额度刷新或你手动刷新额度后会自动恢复可用。
                          </IInfoTooltip>
                        </span>
                      ) : null}
                    </div>
                    <div className="flex items-center gap-3 shrink-0 text-ios-textSecondary dark:text-ios-textSecondaryDark">
                      <span>
                        {k.success_count}/{k.request_count}
                      </span>
                      {k.total_exhausted > 0 ? (
                        <span className="text-rose-500">⟲{k.total_exhausted}</span>
                      ) : null}
                      {k.runtime_exhausted && k.cooldown_until ? (
                        <span
                          className="rounded-full bg-black/[0.05] px-2 py-0.5 text-[10px] font-semibold dark:bg-white/[0.08]"
                          title={formatDateTimeAsiaShanghai(k.cooldown_until)}
                        >
                          冷却至 {formatDateTimeAsiaShanghai(k.cooldown_until)}
                        </span>
                      ) : null}
                      {k.bound_session_count > 0 ? (
                        <span
                          className="flex items-center gap-1 rounded-full bg-violet-500/10 px-2 py-0.5 text-[10px] font-bold text-violet-700 dark:text-violet-300"
                          title={`${k.bound_session_count} 个会话绑定到此 Key`}
                        >
                          <Link2 className="h-2.5 w-2.5" strokeWidth={2.4} />
                          {k.bound_session_count}
                        </span>
                      ) : null}
                      {k.has_jwt ? (
                        <CheckCircle
                          className="h-3.5 w-3.5 text-emerald-500"
                          strokeWidth={2.4}
                        />
                      ) : (
                        <XCircle
                          className="h-3.5 w-3.5 text-gray-400"
                          strokeWidth={2.4}
                        />
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ) : status ? (
          <div className="rounded-[20px] border border-dashed border-black/[0.08] bg-black/[0.02] px-4 py-5 text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark dark:border-white/[0.08] dark:bg-white/[0.03] ">
            <div className="flex items-start gap-3">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-black/[0.04] dark:bg-white/[0.06]">
                <Sparkles
                  className="h-4 w-4 text-ios-textSecondary dark:text-ios-textSecondaryDark"
                  strokeWidth={2.4}
                />
              </div>
              <div>
                <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                  号池待补全
                </div>
                <div className="mt-1 leading-relaxed">
                  号池为空，请到「号池 (MITM)」导入带 sk-ws- 前缀的 API
                  Key；导入后这里会自动出现。
                </div>
              </div>
            </div>
          </div>
        ) : null}

        <SessionPanel />

        {error ? (
          <div className="rounded-[18px] border border-rose-500/15 bg-rose-500/[0.06] p-3 text-[12px] text-rose-700 dark:text-rose-300">
            <div className="flex items-start gap-2">
              <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" strokeWidth={2.4} />
              <span>{error}</span>
            </div>
          </div>
        ) : null}

        <button
          type="button"
          className="no-drag-region flex w-full items-center justify-center gap-2 rounded-ios-block border border-rose-500/12 bg-rose-500/[0.06] px-4 py-3 text-[12px] font-semibold text-rose-700 transition-colors ios-btn hover:bg-rose-500/[0.11] disabled:opacity-50 dark:text-rose-300"
          disabled={loading}
          onClick={handleTeardown}
        >
          <Power className="h-3.5 w-3.5" strokeWidth={2.4} />
          卸载 MITM（停止代理 + 移除 Hosts / CA）
        </button>
      </div>
    </div>
  );
}
