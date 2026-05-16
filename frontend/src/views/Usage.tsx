import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ComponentType,
} from "react";
import {
  Activity,
  ArrowRightLeft,
  Box,
  CheckCircle2,
  Clock,
  Layers,
  RefreshCw,
  Trash2,
  XCircle,
} from "lucide-react";
import { APIInfo, type Models } from "../api/wails";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton";
import UsageRecordsTable from "../components/UsageRecordsTable";
import { useMainViewStore } from "../stores/useMainViewStore";
import { confirmDialog, showErrorToast, showToast } from "../utils/toast";

const POLL_INTERVAL_MS = 5000;
const RECORDS_REFRESH_MS = 20000;
const RECORD_LIMIT = 5000;
const MODEL_BREAKDOWN_LIMIT = 6;

type IconType = ComponentType<{ className?: string; strokeWidth?: number | string }>;

function formatNumber(num: number) {
  return new Intl.NumberFormat("en-US").format(num || 0);
}
function formatCompactToken(num: number) {
  if (!num) return "0";
  if (num >= 1_000_000) {
    return new Intl.NumberFormat("en-US", {
      notation: "compact",
      maximumFractionDigits: 2,
    }).format(num);
  }
  return new Intl.NumberFormat("en-US").format(num);
}
function formatPercent(value: number) {
  return `${value.toFixed(value >= 10 ? 1 : 2)}%`;
}

interface KpiCardProps {
  icon: IconType;
  label: string;
  value: string;
  valueTitle?: string;
  detail: string;
  tone?: "neutral" | "blue" | "violet" | "emerald" | "rose";
  badge?: string;
  progress?: number;
}

const KPI_TONE: Record<NonNullable<KpiCardProps["tone"]>, string> = {
  neutral:
    "border-black/[0.04] bg-white/70 text-gray-900 dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 dark:text-gray-100",
  blue: "border-blue-500/15 bg-blue-500/[0.04] text-blue-700 dark:text-blue-300",
  violet:
    "border-violet-500/15 bg-violet-500/[0.04] text-violet-700 dark:text-violet-300",
  emerald:
    "border-emerald-500/15 bg-emerald-500/[0.04] text-emerald-700 dark:text-emerald-300",
  rose:
    "border-rose-500/15 bg-rose-500/[0.04] text-rose-700 dark:text-rose-300",
};

function KpiCard({
  icon: Icon,
  label,
  value,
  valueTitle,
  detail,
  tone = "neutral",
  badge,
  progress,
}: KpiCardProps) {
  return (
    <div
      className={`rounded-[24px] border p-5 shadow-sm ios-glass relative min-w-0 ${KPI_TONE[tone]}`}
    >
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em]">
          <Icon className="w-4 h-4" strokeWidth={2.4} /> {label}
        </div>
        {badge ? (
          <div className="bg-black/[0.05] dark:bg-white/[0.08] px-2 py-0.5 rounded text-[10px] font-bold opacity-90">
            {badge}
          </div>
        ) : null}
      </div>
      <div
        className="text-[28px] lg:text-[32px] font-extrabold tracking-tight truncate"
        title={valueTitle}
      >
        {value}
      </div>
      <div className="mt-1 text-[12px] opacity-70 font-medium">{detail}</div>
      {progress !== undefined ? (
        <div className="mt-3 h-1.5 rounded-full bg-black/[0.06] dark:bg-white/[0.08] overflow-hidden">
          <div
            className="h-full rounded-full bg-current opacity-60"
            style={{ width: `${Math.min(100, Math.max(0, progress))}%` }}
          />
        </div>
      ) : null}
    </div>
  );
}

/**
 * Usage — Vue 1:1 完整迁移：6 KPI / 每日趋势 / 模型分布 / 调用流水（子组件）/ 自动 5s poll。
 */
export default function Usage() {
  const activeTab = useMainViewStore((s) => s.activeTab);

  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [summary, setSummary] = useState<Models.services.UsageSummary | null>(
    null,
  );
  const [records, setRecords] = useState<Models.services.UsageRecord[]>([]);
  const [selectedDate, setSelectedDate] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState("all");
  const [modelFilter, setModelFilter] = useState("all");
  const [searchQuery, setSearchQuery] = useState("");
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 100;

  const summaryFetchInFlight = useRef<Promise<void> | null>(null);
  const recordsFetchInFlight = useRef<Promise<void> | null>(null);
  const lastSummaryFetchedAt = useRef(0);
  const lastRecordsFetchedAt = useRef(0);
  const pollTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const totalRecordCount = summary?.total_requests || 0;
  const successCount = Math.max(
    0,
    totalRecordCount - (summary?.error_count || 0),
  );
  const successRate = totalRecordCount
    ? (successCount / totalRecordCount) * 100
    : 0;
  const errorRate = totalRecordCount
    ? ((summary?.error_count || 0) / totalRecordCount) * 100
    : 0;
  const estimatedCost = (summary?.estimated_cost_usd || 0).toFixed(2);

  const modelOptions = useMemo(() => {
    const byModel = summary?.by_model || {};
    const byTokens = summary?.by_model_tokens || {};
    return Object.keys(byModel).sort((l, r) => {
      const tDelta = (byTokens[r] || 0) - (byTokens[l] || 0);
      if (tDelta !== 0) return tDelta;
      return (byModel[r] || 0) - (byModel[l] || 0);
    });
  }, [summary]);

  const topModels = useMemo(() => {
    const byModel = summary?.by_model || {};
    const byTokens = summary?.by_model_tokens || {};
    const total = totalRecordCount || 1;
    return modelOptions.slice(0, MODEL_BREAKDOWN_LIMIT).map((model) => ({
      model,
      requests: byModel[model] || 0,
      tokens: byTokens[model] || 0,
      share: ((byModel[model] || 0) / total) * 100,
    }));
  }, [modelOptions, summary, totalRecordCount]);

  const dailyDates = useMemo(
    () =>
      Object.keys(summary?.by_date || {}).sort((l, r) => r.localeCompare(l)),
    [summary],
  );
  const dailyByDate = summary?.by_date || {};
  const dailyTokensByDate = summary?.by_date_tokens || {};

  const activeFilterLabel = useMemo(() => {
    const parts: string[] = [];
    if (selectedDate) parts.push(`日期 ${selectedDate}`);
    if (statusFilter !== "all")
      parts.push(statusFilter === "ok" ? "仅成功" : "仅错误");
    if (modelFilter !== "all") parts.push(modelFilter);
    if (searchQuery.trim()) parts.push(`搜索 "${searchQuery.trim()}"`);
    return parts.length ? parts.join(" · ") : "全部记录";
  }, [selectedDate, statusFilter, modelFilter, searchQuery]);

  const visibleRecordHint = useMemo(() => {
    const loaded = formatNumber(records.length);
    const total = formatNumber(totalRecordCount);
    if (!records.length) return "尚未加载到调用记录";
    if (totalRecordCount > records.length) {
      return `已加载最近 ${loaded} 条，累计 ${total} 条`;
    }
    return `累计 ${total} 条记录`;
  }, [records, totalRecordCount]);

  const filteredRecords = useMemo(() => {
    const dateF = selectedDate;
    const status = statusFilter;
    const model = modelFilter;
    const query = searchQuery.trim().toLowerCase();
    return records.filter((rec) => {
      if (dateF && (!rec.at || !rec.at.startsWith(dateF))) return false;
      if (status !== "all" && rec.status !== status) return false;
      const recordModel = rec.model || rec.request_model || "unknown";
      if (model !== "all" && recordModel !== model) return false;
      if (!query) return true;
      const haystack = [
        rec.model,
        rec.request_model,
        rec.api_key_short,
        rec.status,
        rec.error_detail,
        rec.format,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();
      return haystack.includes(query);
    });
  }, [records, selectedDate, statusFilter, modelFilter, searchQuery]);

  const totalPages = Math.max(1, Math.ceil(filteredRecords.length / pageSize));
  const paginatedRecords = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return filteredRecords.slice(start, start + pageSize);
  }, [filteredRecords, currentPage, pageSize]);

  useEffect(() => {
    setCurrentPage(1);
  }, [selectedDate, statusFilter, modelFilter, searchQuery]);

  useEffect(() => {
    if (currentPage > totalPages) setCurrentPage(totalPages);
  }, [currentPage, totalPages]);

  // ── fetch + poll ─────────────
  const clearPollTimer = () => {
    if (pollTimer.current) {
      clearTimeout(pollTimer.current);
      pollTimer.current = null;
    }
  };

  const shouldPoll = () => {
    if (
      typeof document !== "undefined" &&
      document.visibilityState !== "visible"
    ) {
      return false;
    }
    return useMainViewStore.getState().activeTab === "Usage";
  };

  const shouldRefreshRecords = (force = false) =>
    force ||
    records.length === 0 ||
    Date.now() - lastRecordsFetchedAt.current >= RECORDS_REFRESH_MS;

  const fetchSummary = async (force = false) => {
    if (summaryFetchInFlight.current) return summaryFetchInFlight.current;
    if (!force && Date.now() - lastSummaryFetchedAt.current < 2500) return;
    summaryFetchInFlight.current = (async () => {
      const s = await APIInfo.getUsageSummary();
      setSummary(s);
      lastSummaryFetchedAt.current = Date.now();
    })();
    try {
      await summaryFetchInFlight.current;
    } finally {
      summaryFetchInFlight.current = null;
    }
  };

  const fetchRecords = async (force = false) => {
    if (recordsFetchInFlight.current) return recordsFetchInFlight.current;
    if (!shouldRefreshRecords(force)) return;
    recordsFetchInFlight.current = (async () => {
      const r = (await APIInfo.getUsageRecords(RECORD_LIMIT)) || [];
      setRecords(r);
      lastRecordsFetchedAt.current = Date.now();
    })();
    try {
      await recordsFetchInFlight.current;
    } finally {
      recordsFetchInFlight.current = null;
    }
  };

  const fetchUsageData = async (opts?: {
    silent?: boolean;
    forceSummary?: boolean;
    forceRecords?: boolean;
  }) => {
    const silent = opts?.silent ?? false;
    const forceSummary = opts?.forceSummary ?? false;
    const forceRecords = opts?.forceRecords ?? false;
    if (!silent) setLoading(true);
    else setRefreshing(true);
    try {
      await Promise.all([
        fetchSummary(forceSummary),
        forceRecords || shouldRefreshRecords()
          ? fetchRecords(forceRecords)
          : Promise.resolve(),
      ]);
    } catch (e) {
      if (!silent) showErrorToast(e, "获取用量数据失败");
      else console.error("Silent usage refresh failed:", e);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  const scheduleNextPoll = () => {
    clearPollTimer();
    if (!shouldPoll()) return;
    pollTimer.current = setTimeout(() => {
      void fetchUsageData({ silent: true }).finally(scheduleNextPoll);
    }, POLL_INTERVAL_MS);
  };

  const resumePolling = (forceRefresh = false) => {
    if (!shouldPoll()) {
      clearPollTimer();
      return;
    }
    void fetchUsageData({
      silent: true,
      forceSummary: forceRefresh,
      forceRecords: forceRefresh,
    }).finally(scheduleNextPoll);
  };

  const handleRefresh = async () => {
    await fetchUsageData({
      silent: true,
      forceSummary: true,
      forceRecords: true,
    });
    showToast("用量统计已刷新", "success");
  };

  const handleClear = async () => {
    const ok = await confirmDialog("确认清空所有用量记录？", {
      confirmText: "清空",
      destructive: true,
    });
    if (!ok) return;
    try {
      const deletedCount = await APIInfo.deleteAllUsage();
      await fetchUsageData({ forceSummary: true, forceRecords: true });
      showToast(`已清空 ${deletedCount} 条用量记录`, "success");
    } catch (e) {
      showErrorToast(e, "清空记录失败");
    }
  };

  useEffect(() => {
    if (activeTab === "Usage") resumePolling();
    else clearPollTimer();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab]);

  useEffect(() => {
    void fetchUsageData({
      forceSummary: true,
      forceRecords: true,
    }).finally(scheduleNextPoll);
    const onVis = () => {
      if (typeof document === "undefined") return;
      if (document.visibilityState === "visible") resumePolling();
      else clearPollTimer();
    };
    document.addEventListener("visibilitychange", onVis);
    return () => {
      clearPollTimer();
      document.removeEventListener("visibilitychange", onVis);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="p-6 md:p-8 max-w-5xl mx-auto w-full pb-10">
      <div className="flex items-start justify-between mb-8 shrink-0 flex-wrap gap-4">
        <div>
          <h1 className="text-[32px] font-[800] text-gray-900 dark:text-gray-100 tracking-tight leading-none flex items-center gap-3">
            <Activity className="w-8 h-8 text-ios-blue" strokeWidth={2.4} />
            用量统计
          </h1>
          <p className="text-[13px] text-gray-500 dark:text-gray-400 font-medium mt-3">
            实时监控底层 MITM 代理与 OpenAI Relay 的全量请求日志与 Token 消耗流水。
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="no-drag-region flex items-center gap-1.5 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2 text-[12px] font-semibold text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textSecondaryDark disabled:opacity-50"
            disabled={refreshing || loading}
            onClick={handleRefresh}
          >
            <RefreshCw
              className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`}
              strokeWidth={2.4}
            />
            刷新
          </button>
          <button
            type="button"
            className="no-drag-region flex items-center gap-1.5 rounded-full border border-rose-500/15 bg-rose-500/[0.06] px-4 py-2 text-[12px] font-semibold text-rose-600 shadow-sm transition-all ios-btn hover:bg-rose-500/[0.1] dark:text-rose-400"
            onClick={handleClear}
          >
            <Trash2 className="h-4 w-4" strokeWidth={2.4} />
            清空数据
          </button>
        </div>
      </div>

      {loading && !summary ? (
        <PageLoadingSkeleton variant="usage" className="w-full" />
      ) : (
        <div className="space-y-6">
          {/* 6 KPI */}
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            <KpiCard
              icon={ArrowRightLeft}
              label="请求总数"
              value={formatCompactToken(summary?.total_requests || 0)}
              valueTitle={formatNumber(summary?.total_requests || 0)}
              detail="包含出错与未完成"
            />
            <KpiCard
              icon={Box}
              label="总计 Tokens"
              value={formatCompactToken(summary?.total_tokens || 0)}
              valueTitle={formatNumber(summary?.total_tokens || 0)}
              detail="Prompt + Completion"
              tone="blue"
              badge={`等效约 $${estimatedCost}`}
            />
            <KpiCard
              icon={Layers}
              label="Prompt Tokens"
              value={formatCompactToken(summary?.total_prompt_tokens || 0)}
              valueTitle={formatNumber(summary?.total_prompt_tokens || 0)}
              detail="上行请求用量"
              tone="violet"
            />
            <KpiCard
              icon={Layers}
              label="Completion Tokens"
              value={formatCompactToken(summary?.total_completion_tokens || 0)}
              valueTitle={formatNumber(summary?.total_completion_tokens || 0)}
              detail="下行响应用量"
              tone="emerald"
            />
            <KpiCard
              icon={CheckCircle2}
              label="成功率"
              value={formatPercent(successRate)}
              detail={`${formatNumber(successCount)} / ${formatNumber(totalRecordCount)}`}
              tone="emerald"
              progress={successRate}
            />
            <KpiCard
              icon={XCircle}
              label="错误率"
              value={formatPercent(errorRate)}
              detail={`${formatNumber(summary?.error_count || 0)} 条错误`}
              tone="rose"
              progress={errorRate}
            />
          </div>

          {/* 每日趋势 + 模型分布 */}
          <div className="grid grid-cols-1 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)] gap-4">
            <div className="rounded-[24px] border border-black/[0.04] bg-white/70 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 overflow-hidden">
              <div className="px-6 py-5 border-b border-black/[0.04] dark:border-white/[0.04]">
                <h2 className="text-[16px] font-bold text-gray-900 dark:text-gray-100 flex items-center gap-2">
                  <Clock
                    className="w-4 h-4 text-gray-500 dark:text-gray-400"
                    strokeWidth={2.4}
                  />
                  每日趋势
                </h2>
                <p className="mt-1 text-[12px] font-medium text-gray-500 dark:text-gray-400">
                  点击日期可筛选下方流水。
                </p>
              </div>
              {dailyDates.length ? (
                <div className="px-6 py-3 space-y-1.5 max-h-[320px] overflow-y-auto">
                  {dailyDates.map((d) => {
                    const reqs = dailyByDate[d] || 0;
                    const tokens = dailyTokensByDate[d] || 0;
                    const active = selectedDate === d;
                    return (
                      <button
                        key={d}
                        type="button"
                        onClick={() => setSelectedDate(active ? null : d)}
                        className={[
                          "w-full grid grid-cols-[140px_1fr_1fr] items-center gap-4 rounded-[14px] border px-4 py-2.5 text-[12px] transition-all text-left",
                          active
                            ? "border-ios-blue/30 bg-ios-blue/[0.06]"
                            : "border-black/[0.04] hover:bg-black/[0.02] dark:border-white/[0.05] dark:hover:bg-white/[0.03]",
                        ].join(" ")}
                      >
                        <span className="font-mono font-bold text-gray-700 dark:text-gray-300">
                          {d}
                        </span>
                        <span className="font-mono justify-self-end text-gray-600 dark:text-gray-300">
                          {formatNumber(reqs)} 次
                        </span>
                        <span className="font-mono justify-self-end text-gray-600 dark:text-gray-300">
                          {formatCompactToken(tokens)} tk
                        </span>
                      </button>
                    );
                  })}
                </div>
              ) : (
                <div className="px-6 py-10 text-center text-[13px] text-gray-500 dark:text-gray-400">
                  暂无每日数据。
                </div>
              )}
            </div>

            <div className="rounded-[24px] border border-black/[0.04] bg-white/70 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 overflow-hidden">
              <div className="px-6 py-5 border-b border-black/[0.04] dark:border-white/[0.04] flex items-center justify-between gap-3">
                <div>
                  <h2 className="text-[16px] font-bold text-gray-900 dark:text-gray-100 flex items-center gap-2">
                    <Box
                      className="w-4 h-4 text-gray-500 dark:text-gray-400"
                      strokeWidth={2.4}
                    />
                    模型分布
                  </h2>
                  <p className="mt-1 text-[12px] font-medium text-gray-500 dark:text-gray-400">
                    按请求量展示最常用模型与累计 Tokens。
                  </p>
                </div>
                <span className="text-[12px] font-semibold text-gray-400 dark:text-gray-500">
                  {formatNumber(modelOptions.length)} 个模型
                </span>
              </div>
              {topModels.length ? (
                <div className="px-6 py-5 space-y-4 max-h-[320px] overflow-y-auto">
                  {topModels.map((entry) => (
                    <div
                      key={entry.model}
                      className="rounded-[20px] border border-black/[0.04] bg-black/[0.02] p-4 dark:border-white/[0.04] dark:bg-white/[0.03]"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="truncate font-mono text-[12px] font-bold text-gray-800 dark:text-gray-100">
                            {entry.model}
                          </div>
                          <div className="mt-1 text-[12px] text-gray-500 dark:text-gray-400">
                            {formatNumber(entry.requests)} 次请求 ·{" "}
                            {formatCompactToken(entry.tokens)} Tokens
                          </div>
                        </div>
                        <div className="text-[12px] font-bold text-ios-blue">
                          {formatPercent(entry.share)}
                        </div>
                      </div>
                      <div className="mt-3 h-2 rounded-full bg-black/[0.06] dark:bg-white/[0.08] overflow-hidden">
                        <div
                          className="h-full rounded-full bg-gradient-to-r from-ios-blue to-cyan-400"
                          style={{
                            width: `${Math.min(100, Math.max(entry.share, 3))}%`,
                          }}
                        />
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="px-6 py-10 text-center text-[13px] text-gray-500 dark:text-gray-400">
                  暂无模型分布数据。
                </div>
              )}
            </div>
          </div>

          {/* 调用流水 */}
          <UsageRecordsTable
            records={records}
            filteredRecords={filteredRecords}
            paginatedRecords={paginatedRecords}
            modelOptions={modelOptions}
            selectedDate={selectedDate}
            onSelectedDate={setSelectedDate}
            statusFilter={statusFilter}
            onStatusFilter={setStatusFilter}
            modelFilter={modelFilter}
            onModelFilter={setModelFilter}
            searchQuery={searchQuery}
            onSearchQuery={setSearchQuery}
            currentPage={currentPage}
            totalPages={totalPages}
            onCurrentPage={setCurrentPage}
            pageSize={pageSize}
            activeFilterLabel={activeFilterLabel}
            visibleRecordHint={visibleRecordHint}
          />
        </div>
      )}
    </div>
  );
}
