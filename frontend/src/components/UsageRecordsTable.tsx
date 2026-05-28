import {
  Activity,
  BarChart3,
  CheckCircle2,
  Clock,
  Search,
  XCircle,
} from "lucide-react";
import ISegmented from "./ios/ISegmented";
import type { Models } from "../api/wails";

const STATUS_FILTER_OPTIONS = [
  { label: "全部", value: "all" },
  { label: "成功", value: "ok" },
  { label: "错误", value: "error" },
];

function formatNumber(num: number) {
  return new Intl.NumberFormat("en-US").format(num || 0);
}
function formatDuration(ms: number) {
  if (!ms) return "-";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}
function formatDate(dateStr: string) {
  if (!dateStr) return "-";
  return new Date(dateStr).toLocaleString();
}

interface Props {
  records: Models.services.UsageRecord[];
  filteredRecords: Models.services.UsageRecord[];
  paginatedRecords: Models.services.UsageRecord[];
  modelOptions: string[];
  selectedDate: string | null;
  onSelectedDate: (d: string | null) => void;
  statusFilter: string;
  onStatusFilter: (s: string) => void;
  modelFilter: string;
  onModelFilter: (s: string) => void;
  sourceFilter: string;
  onSourceFilter: (s: string) => void;
  searchQuery: string;
  onSearchQuery: (s: string) => void;
  currentPage: number;
  totalPages: number;
  onCurrentPage: (n: number) => void;
  pageSize: number;
  activeFilterLabel: string;
  visibleRecordHint: string;
}

/**
 * UsageRecordsTable — 调用流水卡（筛选条 + 表格 + 分页）。
 * 单独抽出避免 Usage.tsx 单文件过长。
 */
export default function UsageRecordsTable({
  records,
  filteredRecords,
  paginatedRecords,
  modelOptions,
  selectedDate,
  onSelectedDate,
  statusFilter,
  onStatusFilter,
  modelFilter,
  onModelFilter,
  sourceFilter,
  onSourceFilter,
  searchQuery,
  onSearchQuery,
  currentPage,
  totalPages,
  onCurrentPage,
  pageSize,
  activeFilterLabel,
  visibleRecordHint,
}: Props) {
  return (
    <div className="rounded-[24px] border border-black/[0.04] bg-white/70 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 overflow-hidden">
      <div className="px-6 py-5 border-b border-black/[0.04] dark:border-white/[0.04] flex items-start justify-between gap-4 flex-wrap">
        <div>
          <h2 className="text-[16px] font-bold text-gray-900 dark:text-gray-100 flex items-center gap-2">
            <Clock
              className="w-4 h-4 text-gray-500 dark:text-gray-400"
              strokeWidth={2.4}
            />
            {selectedDate ? `${selectedDate} 调用流水` : "近期调用流水"}
          </h2>
          <p className="mt-1 text-[12px] font-medium text-gray-500 dark:text-gray-400">
            {activeFilterLabel} · {visibleRecordHint}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {selectedDate ? (
            <button
              type="button"
              className="text-[12px] font-bold text-ios-blue hover:underline"
              onClick={() => onSelectedDate(null)}
            >
              清除筛选
            </button>
          ) : null}
          <span className="text-[12px] font-medium text-gray-500 dark:text-gray-400">
            当前命中 {formatNumber(filteredRecords.length)} 条
          </span>
        </div>
      </div>

      <div className="px-6 py-4 border-b border-black/[0.04] dark:border-white/[0.04] bg-black/[0.015] dark:bg-white/[0.015]">
        <div className="grid grid-cols-1 xl:grid-cols-[220px_minmax(0,1fr)_220px] gap-3">
          <div className="min-w-0">
            <div className="text-[11px] font-bold uppercase tracking-[0.1em] text-gray-400 dark:text-gray-500 mb-2">
              状态
            </div>
            <ISegmented
              modelValue={statusFilter}
              onValueChange={onStatusFilter}
              options={STATUS_FILTER_OPTIONS}
            />
          </div>
          <label className="min-w-0">
            <div className="text-[11px] font-bold uppercase tracking-[0.1em] text-gray-400 dark:text-gray-500 mb-2">
              搜索
            </div>
            <div className="flex items-center gap-2 rounded-ios-block border border-black/[0.06] bg-white/80 px-4 py-3 shadow-sm dark:border-white/[0.08] dark:bg-black/20">
              <Search
                className="h-4 w-4 shrink-0 text-gray-400"
                strokeWidth={2.2}
              />
              <input
                value={searchQuery}
                onChange={(e) => onSearchQuery(e.target.value)}
                type="text"
                className="w-full bg-transparent text-[13px] text-gray-800 outline-none placeholder:text-gray-400 dark:text-gray-100"
                placeholder="模型 / 状态 / Key / 错误信息"
              />
            </div>
          </label>
          <label className="min-w-0">
            <div className="text-[11px] font-bold uppercase tracking-[0.1em] text-gray-400 dark:text-gray-500 mb-2">
              模型
            </div>
            <select
              value={modelFilter}
              onChange={(e) => onModelFilter(e.target.value)}
              className="no-drag-region w-full rounded-ios-block border border-black/[0.06] bg-white/80 px-4 py-3 text-[13px] font-medium text-gray-800 shadow-sm outline-none transition focus:border-ios-blue/40 dark:border-white/[0.08] dark:bg-black/20 dark:text-gray-100"
            >
              <option value="all">全部模型</option>
              {modelOptions.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </label>
          <label className="min-w-0">
            <div className="text-[11px] font-bold uppercase tracking-[0.1em] text-gray-400 dark:text-gray-500 mb-2">
              来源
            </div>
            <select
              value={sourceFilter}
              onChange={(e) => onSourceFilter(e.target.value)}
              className="no-drag-region w-full rounded-ios-block border border-black/[0.06] bg-white/80 px-4 py-3 text-[13px] font-medium text-gray-800 shadow-sm outline-none transition focus:border-ios-blue/40 dark:border-white/[0.08] dark:bg-black/20 dark:text-gray-100"
            >
              <option value="all">全部来源</option>
              <option value="pool">号池</option>
              <option value="provider">提供商</option>
            </select>
          </label>
        </div>
      </div>

      {filteredRecords.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 px-6 text-center">
          <div className="relative mb-6">
            <div className="w-24 h-24 rounded-ios-card bg-gradient-to-br from-violet-500/15 to-ios-blue/15 dark:from-violet-500/25 dark:to-ios-blue/25 flex items-center justify-center shadow-[0_10px_28px_rgba(99,102,241,0.12)]">
              <BarChart3
                className="w-11 h-11 text-violet-500 dark:text-violet-300"
                strokeWidth={1.8}
              />
            </div>
            {!records.length ? (
              <div className="absolute -bottom-1 -right-1 w-10 h-10 rounded-2xl bg-white dark:bg-[#1C1C1E] flex items-center justify-center shadow-md ring-2 ring-white/80 dark:ring-black/80">
                <Activity
                  className="w-5 h-5 text-emerald-500"
                  strokeWidth={2.6}
                />
              </div>
            ) : null}
          </div>
          <h3 className="text-[20px] font-bold text-ios-text dark:text-ios-textDark mb-2">
            {records.length === 0 ? "暂无调用记录" : "当前筛选无结果"}
          </h3>
          <p className="max-w-[400px] text-[13px] leading-relaxed text-gray-500 dark:text-gray-400">
            {records.length === 0
              ? "Cascade 对话后这里会实时显示 token 流水、模型分布与美金成本。先在 Dashboard 启动 MITM 代理后开始对话。"
              : "尝试清空搜索 / 切换日期 / 调整状态筛选"}
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-[13px]">
            <thead className="bg-gray-50/50 dark:bg-black/10 text-gray-500 dark:text-gray-400 text-[11px] uppercase tracking-wider font-bold">
              <tr>
                <th className="px-6 py-3">时间</th>
                <th className="px-6 py-3">状态</th>
                <th className="px-6 py-3">模型</th>
                <th className="px-6 py-3 text-right">Prompt</th>
                <th className="px-6 py-3 text-right">Completion</th>
                <th className="px-6 py-3 text-right">Total Tokens</th>
                <th className="px-6 py-3 text-right">耗时</th>
                <th className="px-6 py-3">来源</th>
                <th className="px-6 py-3">来源 Key (短尾)</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-black/[0.04] dark:divide-white/[0.04]">
              {paginatedRecords.map((rec) => (
                <tr
                  key={rec.id}
                  className="hover:bg-black/[0.01] dark:hover:bg-white/[0.01] transition-colors"
                >
                  <td className="px-6 py-3.5 whitespace-nowrap text-gray-500 dark:text-gray-400">
                    {formatDate(rec.at)}
                  </td>
                  <td className="px-6 py-3.5 whitespace-nowrap">
                    <div
                      className={[
                        "flex items-center gap-1.5",
                        rec.status === "ok"
                          ? "text-emerald-600 dark:text-emerald-400"
                          : "text-rose-600 dark:text-rose-400",
                      ].join(" ")}
                    >
                      {rec.status === "ok" ? (
                        <CheckCircle2 className="w-4 h-4" strokeWidth={2.5} />
                      ) : (
                        <XCircle className="w-4 h-4" strokeWidth={2.5} />
                      )}
                      <span className="font-bold text-[11px] uppercase">
                        {rec.status}
                      </span>
                    </div>
                  </td>
                  <td className="px-6 py-3.5 whitespace-nowrap">
                    <span className="bg-black/5 dark:bg-white/10 px-2 py-0.5 rounded font-mono text-[11px] text-gray-700 dark:text-gray-300 font-semibold shadow-sm">
                      {rec.model || rec.request_model || "unknown"}
                    </span>
                  </td>
                  <td className="px-6 py-3.5 whitespace-nowrap text-right font-mono text-gray-600 dark:text-gray-300">
                    {formatNumber(rec.prompt_tokens)}
                  </td>
                  <td className="px-6 py-3.5 whitespace-nowrap text-right font-mono text-gray-600 dark:text-gray-300">
                    {formatNumber(rec.completion_tokens)}
                  </td>
                  <td className="px-6 py-3.5 whitespace-nowrap text-right font-mono font-bold text-gray-900 dark:text-gray-100">
                    {formatNumber(rec.total_tokens)}
                  </td>
                  <td className="px-6 py-3.5 whitespace-nowrap text-right text-gray-500 dark:text-gray-400">
                    {formatDuration(rec.duration_ms)}
                  </td>
                  <td className="px-6 py-3.5 whitespace-nowrap">
                    {(() => {
                      const isProvider = rec.format === "provider-relay";
                      const isPool = rec.format === "windsurf-mitm";
                      const label = isProvider
                        ? `提供商${rec.request_model ? "·" + rec.request_model : ""}`
                        : isPool
                          ? "号池"
                          : rec.format || "-";
                      const cls = isProvider
                        ? "bg-violet-500/10 text-violet-700 dark:bg-violet-500/20 dark:text-violet-300"
                        : isPool
                          ? "bg-sky-500/10 text-sky-700 dark:bg-sky-500/20 dark:text-sky-300"
                          : "bg-gray-500/10 text-gray-600 dark:bg-gray-500/20 dark:text-gray-300";
                      return (
                        <span
                          className={`${cls} px-2 py-0.5 rounded text-[11px] font-semibold`}
                        >
                          {label}
                        </span>
                      );
                    })()}
                  </td>
                  <td className="px-6 py-3.5 whitespace-nowrap">
                    {rec.api_key_short || rec.error_detail ? (
                      <div className="space-y-1">
                        {rec.api_key_short ? (
                          <span className="inline-flex font-mono text-[11px] text-gray-500 dark:text-gray-400 bg-gray-100 dark:bg-gray-800 px-1.5 py-0.5 rounded">
                            ...{rec.api_key_short}
                          </span>
                        ) : null}
                        {rec.error_detail ? (
                          <div
                            className="max-w-[280px] truncate text-[11px] text-rose-500"
                            title={rec.error_detail}
                          >
                            {rec.error_detail}
                          </div>
                        ) : null}
                      </div>
                    ) : (
                      <span className="text-gray-400 dark:text-gray-500">-</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {totalPages > 1 ? (
            <div className="px-6 py-4 border-t border-black/[0.04] dark:border-white/[0.04] flex items-center justify-between">
              <div className="text-[12px] text-gray-500 dark:text-gray-400">
                显示 {(currentPage - 1) * pageSize + 1} -{" "}
                {Math.min(currentPage * pageSize, filteredRecords.length)} 条，共{" "}
                {filteredRecords.length} 条记录
              </div>
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  disabled={currentPage === 1}
                  className="px-2.5 py-1.5 rounded border border-black/[0.06] dark:border-white/[0.08] text-[12px] font-medium disabled:opacity-30 disabled:cursor-not-allowed hover:bg-black/[0.04] dark:hover:bg-white/[0.04] transition-colors"
                  onClick={() => onCurrentPage(currentPage - 1)}
                >
                  上一页
                </button>
                <div className="px-3 text-[12px] font-bold text-gray-700 dark:text-gray-300">
                  {currentPage} / {totalPages}
                </div>
                <button
                  type="button"
                  disabled={currentPage === totalPages}
                  className="px-2.5 py-1.5 rounded border border-black/[0.06] dark:border-white/[0.08] text-[12px] font-medium disabled:opacity-30 disabled:cursor-not-allowed hover:bg-black/[0.04] dark:hover:bg-white/[0.04] transition-colors"
                  onClick={() => onCurrentPage(currentPage + 1)}
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
