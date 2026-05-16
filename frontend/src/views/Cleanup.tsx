import { useEffect, useMemo, useState } from "react";
import {
  CheckCircle2,
  HardDrive,
  Loader2,
  RotateCcw,
  Shield,
  Sparkles,
  Trash2,
  Zap,
} from "lucide-react";
import { APIInfo } from "../api/wails";
import { confirmDialog, showErrorToast, showToast } from "../utils/toast";

interface CleanupCategory {
  id: string;
  name: string;
  description: string;
  size_bytes: number;
  size_human: string;
  file_count: number;
  safe: boolean;
}
interface DiskUsage {
  categories: CleanupCategory[];
  total_bytes: number;
  total_human: string;
}
interface CleanupResult {
  category: string;
  success: boolean;
  freed_bytes: number;
  freed_human: string;
  deleted_dirs: number;
  error?: string;
}
interface PerformanceTip {
  id: string;
  title: string;
  description: string;
  impact: string;
  auto_fix: boolean;
}

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KB`;
  const mb = kb / 1024;
  if (mb < 1024) return `${mb.toFixed(1)} MB`;
  return `${(mb / 1024).toFixed(2)} GB`;
}

function impactColor(impact: string) {
  switch (impact) {
    case "high":
      return "text-red-500 bg-red-500/10";
    case "medium":
      return "text-amber-500 bg-amber-500/10";
    default:
      return "text-emerald-500 bg-emerald-500/10";
  }
}

function impactLabel(impact: string) {
  switch (impact) {
    case "high":
      return "高";
    case "medium":
      return "中";
    default:
      return "低";
  }
}

/**
 * Cleanup — Vue 1:1 完整迁移：磁盘占用分析 + 性能优化建议。
 */
export default function Cleanup() {
  const [diskUsage, setDiskUsage] = useState<DiskUsage | null>(null);
  const [tips, setTips] = useState<PerformanceTip[]>([]);
  const [loadingDisk, setLoadingDisk] = useState(false);
  const [loadingTips, setLoadingTips] = useState(false);
  const [cleaning, setCleaning] = useState(false);
  const [applyingTips, setApplyingTips] = useState(false);
  const [selectedCategories, setSelectedCategories] = useState<Set<string>>(
    new Set(),
  );
  const [lastCleanResults, setLastCleanResults] = useState<CleanupResult[]>([]);

  const safeTotalHuman = useMemo(() => {
    if (!diskUsage) return "0 B";
    const safe = diskUsage.categories.filter((c) => c.safe);
    const bytes = safe.reduce((sum, c) => sum + c.size_bytes, 0);
    return humanSize(bytes);
  }, [diskUsage]);

  const toggleCategory = (id: string) => {
    setSelectedCategories((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectAllSafe = () => {
    if (!diskUsage) return;
    setSelectedCategories((prev) => {
      const next = new Set(prev);
      diskUsage.categories
        .filter((c) => c.safe && c.size_bytes > 0)
        .forEach((c) => next.add(c.id));
      return next;
    });
  };

  const fetchDiskUsage = async () => {
    setLoadingDisk(true);
    try {
      const r = (await APIInfo.getWindsurfDiskUsage()) as DiskUsage;
      setDiskUsage(r);
    } catch (e) {
      showErrorToast(e, "获取磁盘占用失败");
    } finally {
      setLoadingDisk(false);
    }
  };

  const fetchTips = async () => {
    setLoadingTips(true);
    try {
      const r = (await APIInfo.getPerformanceTips()) as PerformanceTip[];
      setTips(r);
    } catch (e) {
      console.error("GetPerformanceTips error:", e);
    } finally {
      setLoadingTips(false);
    }
  };

  const cleanSelected = async () => {
    const ids = [...selectedCategories];
    if (ids.length === 0) {
      showToast("请先选择要清理的类别", "warning");
      return;
    }
    if (ids.includes("cascade")) {
      const ok = await confirmDialog(
        "Cascade 对话缓存包含 AI 对话历史，清理后无法恢复。确定继续？",
      );
      if (!ok) return;
    }
    setCleaning(true);
    try {
      const results = (await APIInfo.cleanupWindsurf(ids)) as CleanupResult[];
      setLastCleanResults(results);
      const totalFreed = results.reduce((s, r) => s + r.freed_bytes, 0);
      showToast(`清理完成，释放 ${humanSize(totalFreed)}`, "success");
      setSelectedCategories(new Set());
      await fetchDiskUsage();
    } catch (e) {
      showErrorToast(e, "清理失败");
    } finally {
      setCleaning(false);
    }
  };

  const quickCleanStartup = async () => {
    setCleaning(true);
    try {
      const results = (await APIInfo.cleanupStartupCache()) as CleanupResult[];
      setLastCleanResults(results);
      const totalFreed = results.reduce((s, r) => s + r.freed_bytes, 0);
      showToast(`启动缓存已清理，释放 ${humanSize(totalFreed)}`, "success");
      await fetchDiskUsage();
    } catch (e) {
      showErrorToast(e, "清理失败");
    } finally {
      setCleaning(false);
    }
  };

  const applyAllFixes = async () => {
    setApplyingTips(true);
    try {
      const results = (await APIInfo.applyAllPerformanceFixes()) as Record<
        string,
        string
      >;
      const applied = Object.values(results).filter(
        (v) => v === "已应用",
      ).length;
      const skipped = Object.values(results).filter(
        (v) => v === "已存在，跳过",
      ).length;
      showToast(
        `性能优化: ${applied} 项已应用, ${skipped} 项已存在`,
        "success",
      );
    } catch (e) {
      showErrorToast(e, "应用失败");
    } finally {
      setApplyingTips(false);
    }
  };

  useEffect(() => {
    void fetchDiskUsage();
    void fetchTips();
  }, []);

  return (
    <div className="p-6 space-y-6 max-w-4xl mx-auto">
      {/* 磁盘占用分析 */}
      <section>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <HardDrive className="w-5 h-5 text-ios-blue" strokeWidth={2.2} />
            <h2 className="text-lg font-bold text-ios-text dark:text-ios-textDark">
              Windsurf 磁盘占用
            </h2>
          </div>
          <button
            type="button"
            onClick={fetchDiskUsage}
            disabled={loadingDisk}
            className="no-drag-region flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-semibold text-ios-textSecondary hover:bg-black/5 dark:hover:bg-white/10 transition-colors ios-btn"
          >
            <RotateCcw
              className={`w-3.5 h-3.5 ${loadingDisk ? "animate-spin" : ""}`}
              strokeWidth={2.2}
            />
            刷新
          </button>
        </div>

        {loadingDisk && !diskUsage ? (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <div className="w-20 h-20 rounded-[24px] bg-gradient-to-br from-ios-blue/15 to-violet-500/15 dark:from-ios-blue/25 dark:to-violet-500/25 flex items-center justify-center mb-4 shadow-[0_8px_24px_rgba(37,99,235,0.12)]">
              <Loader2 className="w-9 h-9 text-ios-blue ios-spinner" strokeWidth={2.4} />
            </div>
            <h3 className="text-[16px] font-bold text-ios-text dark:text-ios-textDark mb-1">
              正在扫描磁盘占用…
            </h3>
            <p className="text-[12.5px] text-gray-500 dark:text-gray-400">
              统计 Cascade 对话缓存 / 渲染器缓存 / 启动缓存等
            </p>
          </div>
        ) : diskUsage ? (
          <>
            <div className="grid grid-cols-3 gap-3 mb-4">
              <div className="rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]">
                <div className="text-[11px] font-bold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark">
                  总占用
                </div>
                <div className="mt-1 text-xl font-extrabold text-ios-text dark:text-ios-textDark">
                  {diskUsage.total_human}
                </div>
              </div>
              <div className="rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]">
                <div className="text-[11px] font-bold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark">
                  可安全清理
                </div>
                <div className="mt-1 text-xl font-extrabold text-emerald-600 dark:text-emerald-400">
                  {safeTotalHuman}
                </div>
              </div>
              <div className="rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]">
                <div className="text-[11px] font-bold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark">
                  类别数
                </div>
                <div className="mt-1 text-xl font-extrabold text-ios-text dark:text-ios-textDark">
                  {diskUsage.categories.length}
                </div>
              </div>
            </div>

            <div className="space-y-2">
              {diskUsage.categories.map((cat) => {
                const selected = selectedCategories.has(cat.id);
                const clickable = cat.size_bytes > 0;
                return (
                  <div
                    key={cat.id}
                    onClick={() => clickable && toggleCategory(cat.id)}
                    className={[
                      "flex items-center gap-3 rounded-2xl border px-4 py-3 transition-all duration-200",
                      selected
                        ? "border-ios-blue/30 bg-ios-blue/[0.04] ring-1 ring-ios-blue/20"
                        : "border-black/[0.05] bg-white/60 dark:border-white/[0.06] dark:bg-white/[0.04]",
                      clickable
                        ? "cursor-pointer hover:border-ios-blue/20"
                        : "opacity-50 cursor-default",
                    ].join(" ")}
                  >
                    <div
                      className={[
                        "w-5 h-5 rounded-md border-2 flex items-center justify-center shrink-0 transition-colors",
                        selected
                          ? "bg-ios-blue border-ios-blue"
                          : "border-gray-300 dark:border-gray-600",
                      ].join(" ")}
                    >
                      {selected ? (
                        <CheckCircle2
                          className="w-3.5 h-3.5 text-white"
                          strokeWidth={3}
                        />
                      ) : null}
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ios-text dark:text-ios-textDark">
                          {cat.name}
                        </span>
                        {cat.safe ? (
                          <span className="rounded-full bg-emerald-500/10 px-1.5 py-0.5 text-[9px] font-bold text-emerald-600 dark:text-emerald-400">
                            安全
                          </span>
                        ) : (
                          <span className="rounded-full bg-red-500/10 px-1.5 py-0.5 text-[9px] font-bold text-red-500">
                            谨慎
                          </span>
                        )}
                      </div>
                      <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-0.5">
                        {cat.description}
                      </div>
                    </div>
                    <div className="text-right shrink-0">
                      <div
                        className={[
                          "text-sm font-bold",
                          cat.size_bytes > 100 * 1024 * 1024
                            ? "text-red-500"
                            : cat.size_bytes > 10 * 1024 * 1024
                              ? "text-amber-500"
                              : "text-ios-text dark:text-ios-textDark",
                        ].join(" ")}
                      >
                        {cat.size_human}
                      </div>
                      <div className="text-[10px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                        {cat.file_count} 文件
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>

            <div className="mt-4 flex items-center gap-3">
              <button
                type="button"
                onClick={selectAllSafe}
                className="no-drag-region flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-ios-blue bg-ios-blue/10 hover:bg-ios-blue/20 transition-colors ios-btn"
              >
                <Shield className="w-3.5 h-3.5" strokeWidth={2.2} />
                全选安全项
              </button>
              <button
                type="button"
                onClick={cleanSelected}
                disabled={cleaning || selectedCategories.size === 0}
                className="no-drag-region flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-gradient-to-b from-red-500 to-red-600 shadow-sm hover:shadow-md transition-all ios-btn disabled:opacity-40"
              >
                {cleaning ? (
                  <Loader2 className="w-3.5 h-3.5 animate-spin" strokeWidth={2.2} />
                ) : (
                  <Trash2 className="w-3.5 h-3.5" strokeWidth={2.2} />
                )}
                清理选中 ({selectedCategories.size})
              </button>
              <button
                type="button"
                onClick={quickCleanStartup}
                disabled={cleaning}
                className="no-drag-region flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-gradient-to-b from-amber-500 to-amber-600 shadow-sm hover:shadow-md transition-all ios-btn disabled:opacity-40"
              >
                <Zap className="w-3.5 h-3.5" strokeWidth={2.2} />
                一键清理启动缓存
              </button>
            </div>

            {lastCleanResults.length > 0 ? (
              <div className="mt-4 rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]">
                <div className="text-[11px] font-bold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark mb-2">
                  清理结果
                </div>
                <div className="space-y-1">
                  {lastCleanResults.map((r) => (
                    <div
                      key={r.category}
                      className="flex items-center justify-between text-xs"
                    >
                      <span className="text-ios-text dark:text-ios-textDark font-medium">
                        {r.category}
                      </span>
                      <span
                        className={
                          r.success
                            ? "text-emerald-600 dark:text-emerald-400"
                            : "text-red-500"
                        }
                      >
                        {r.success ? `释放 ${r.freed_human}` : r.error}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ) : null}
          </>
        ) : null}
      </section>

      {/* 性能优化建议 */}
      <section>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <Sparkles className="w-5 h-5 text-amber-500" strokeWidth={2.2} />
            <h2 className="text-lg font-bold text-ios-text dark:text-ios-textDark">
              性能优化建议
            </h2>
          </div>
          <button
            type="button"
            onClick={applyAllFixes}
            disabled={applyingTips}
            className="no-drag-region flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-gradient-to-b from-ios-blue to-blue-600 shadow-sm hover:shadow-md transition-all ios-btn disabled:opacity-40"
          >
            {applyingTips ? (
              <Loader2 className="w-3.5 h-3.5 animate-spin" strokeWidth={2.2} />
            ) : (
              <Zap className="w-3.5 h-3.5" strokeWidth={2.2} />
            )}
            一键优化
          </button>
        </div>

        {loadingTips ? (
          <div className="text-center py-4">
            <Loader2 className="w-5 h-5 animate-spin mx-auto text-ios-blue" />
          </div>
        ) : (
          <div className="space-y-2">
            {tips.map((tip) => (
              <div
                key={tip.id}
                className="rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-semibold text-ios-text dark:text-ios-textDark">
                        {tip.title}
                      </span>
                      <span
                        className={`rounded-full px-1.5 py-0.5 text-[9px] font-bold ${impactColor(tip.impact)}`}
                      >
                        影响: {impactLabel(tip.impact)}
                      </span>
                      {tip.auto_fix ? (
                        <span className="rounded-full bg-ios-blue/10 px-1.5 py-0.5 text-[9px] font-bold text-ios-blue">
                          可自动
                        </span>
                      ) : (
                        <span className="rounded-full bg-gray-500/10 px-1.5 py-0.5 text-[9px] font-bold text-gray-500 dark:text-gray-400">
                          手动
                        </span>
                      )}
                    </div>
                    <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-1 leading-relaxed">
                      {tip.description}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
