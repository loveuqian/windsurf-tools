/**
 * 对 `wailsjs/go/main/App` 的单一入口封装（业务页请优先用 `APIInfo`）。
 * MITM 相关方法已一并挂到 `APIInfo`，与直连 `App` 等价，见 `README.md`。
 */
import * as AppHooks from '../../wailsjs/go/main/App';
import * as Models from '../../wailsjs/go/models';

export { AppHooks, Models };

// Specific typed helper types matching the Go struct
export interface ImportResult {
  email: string;
  success: boolean;
  error?: string;
}

export const APIInfo = {
  getAllAccounts: AppHooks.GetAllAccounts,
  getAccount: AppHooks.GetAccount,
  deleteAccount: AppHooks.DeleteAccount,
  deleteExpiredAccounts: AppHooks.DeleteExpiredAccounts,
  deleteFreePlanAccounts: AppHooks.DeleteFreePlanAccounts,
  deleteAccountsByGroup: AppHooks.DeleteAccountsByGroup,
  exportAccountsByGroup: AppHooks.ExportAccountsByGroup,

  importByEmailPassword: AppHooks.ImportByEmailPassword,
  importByJWT: AppHooks.ImportByJWT,
  importByAPIKey: AppHooks.ImportByAPIKey,
  importByEmailAPIKey: AppHooks.ImportByEmailAPIKey,
  importByRefreshToken: AppHooks.ImportByRefreshToken,
  addSingleAccount: AppHooks.AddSingleAccount,

  refreshAllTokens: AppHooks.RefreshAllTokens,
  refreshAllQuotas: AppHooks.RefreshAllQuotas,
  refreshAccountQuota: AppHooks.RefreshAccountQuota,

  getSettings: AppHooks.GetSettings,
  updateSettings: AppHooks.UpdateSettings,

  // MITM（与 AppHooks.* 一一对应，便于统一从 APIInfo 调用）
  startMitmProxy: AppHooks.StartMitmProxy,
  stopMitmProxy: AppHooks.StopMitmProxy,
  getMitmProxyStatus: AppHooks.GetMitmProxyStatus,
  setupMitmCA: AppHooks.SetupMitmCA,
  setupMitmHosts: AppHooks.SetupMitmHosts,
  setupMitmAll: AppHooks.SetupMitmAll,
  uninstallMitmCA: AppHooks.UninstallMitmCA,
  uninstallMitmHosts: AppHooks.UninstallMitmHosts,
  teardownMitm: AppHooks.TeardownMitm,
  getMitmCAPath: AppHooks.GetMitmCAPath,
  switchMitmToNext: AppHooks.SwitchMitmToNext,
  switchMitmToAccount: AppHooks.SwitchMitmToAccount,
  switchAccountLocal: (AppHooks as any).SwitchAccountLocal,
  // Cascade 破限注入（system prompt 末尾追加 override 文本）
  getJailbreakDefaultOverride: (AppHooks as any).GetJailbreakDefaultOverride as () => Promise<string>,
  // v1.2.0 破限增强：预设 / 文件源 / 统计 / OS 集成
  listJailbreakPresets: (AppHooks as any).ListJailbreakPresets as () => Promise<Array<{
    id: string; name: string; description: string; risk: string; text: string;
  }>>,
  getJailbreakRuntime: (AppHooks as any).GetJailbreakRuntime as () => Promise<{
    enabled: boolean;
    preset_id: string;
    source: string;
    active_text: string;
    active_length: number;
    file_path?: string;
    file_status?: {
      path: string; exists: boolean; size: number; charset: string;
      excerpt: string; truncated: boolean; is_dir: boolean; error?: string;
    };
    stats: {
      total_injects: number; today_injects: number;
      last_inject_at?: string; since_last_inject_ms: number;
    };
    warn_anthropic: boolean;
  }>,
  saveJailbreakOverrideFile: (AppHooks as any).SaveJailbreakOverrideFile as (text: string) => Promise<string>,
  openJailbreakOverrideFile: (AppHooks as any).OpenJailbreakOverrideFile as () => Promise<string>,
  revealJailbreakOverrideFolder: (AppHooks as any).RevealJailbreakOverrideFolder as () => Promise<string>,
  resetJailbreakStats: (AppHooks as any).ResetJailbreakStats as () => Promise<void>,

  // v1.3.0 手动锁定 + 轮换池
  getManualPinStatus: (AppHooks as any).GetManualPinStatus as () => Promise<{
    enabled: boolean; account_id?: string; email?: string; nickname?: string;
  }>,
  unpinManualAccount: (AppHooks as any).UnpinManualAccount as () => Promise<void>,
  getRotationPoolStatus: (AppHooks as any).GetRotationPoolStatus as () => Promise<{
    enabled: boolean;
    member_count: number;
    interval_min: number;
    quota_refresh_min: number;
    next_switch_at?: string;
    last_switched_to?: string;
    last_switched_at?: string;
    last_quota_refresh_at?: string;
    last_error?: string;
    total_switches: number;
    total_quota_refreshes: number;
    paused_by_pin: boolean;
  }>,
  rotationPoolSwitchNow: (AppHooks as any).RotationPoolSwitchNow as () => Promise<string>,
  rotationPoolRefreshQuotasNow: (AppHooks as any).RotationPoolRefreshQuotasNow as () => Promise<void>,

  // 配置导出/导入（多设备迁移 + 备份）
  exportSettings: (AppHooks as any).ExportSettings as () => Promise<string>,
  importSettings: (AppHooks as any).ImportSettings as (jsonText: string) => Promise<void>,

  // v1.6.0 跨平台兼容性诊断
  runDiagnostics: (AppHooks as any).RunDiagnostics as () => Promise<{
    platform: string;
    arch: string;
    ok: number;
    warn: number;
    error: number;
    checks: Array<{
      id: string;
      title: string;
      status: 'ok' | 'warn' | 'error' | 'n/a';
      detail: string;
      fix_hint?: string;
    }>;
  }>,

  // Clash IP 轮换
  testClashController: (AppHooks as any).TestClashController,
  listClashGroupNodes: (AppHooks as any).ListClashGroupNodes,
  triggerClashRotate: (AppHooks as any).TriggerClashRotate,
  getClashRotatorRunning: (AppHooks as any).GetClashRotatorRunning,
  // 一键智能启用：自动挑 selector 组 + 启 rotator + 立即切一次
  autoSetupClash: (AppHooks as any).AutoSetupClash as () => Promise<{
    ok: boolean; error?: string; hint?: string;
    group?: string; node_count?: number;
    from?: string; to?: string;
  }>,
  autoDetectClashGroup: (AppHooks as any).AutoDetectClashGroup as () => Promise<{
    ok: boolean; error?: string;
    group?: string; node_count?: number;
    candidates?: string[]; all_groups?: string[];
  }>,

  // OpenAI 中转
  startOpenAIRelay: AppHooks.StartOpenAIRelay,
  stopOpenAIRelay: AppHooks.StopOpenAIRelay,
  getOpenAIRelayStatus: AppHooks.GetOpenAIRelayStatus,

  // MITM debug dump
  toggleMitmDebugDump: AppHooks.ToggleMitmDebugDump,

  // MITM 全量抓包
  toggleMitmFullCapture: AppHooks.ToggleMitmFullCapture,
  getMitmFullCaptureEnabled: AppHooks.GetMitmFullCaptureEnabled,
  getCaptureDir: AppHooks.GetCaptureDir,

  getMitmSessionBindings: AppHooks.GetMitmSessionBindings,
  unbindMitmSession: AppHooks.UnbindMitmSession,

  // 用量追踪
  getUsageRecords: AppHooks.GetUsageRecords,
  getUsageSummary: AppHooks.GetUsageSummary,
  deleteAllUsage: AppHooks.DeleteAllUsage,

  // Windsurf 清理 & 性能优化
  getWindsurfDiskUsage: AppHooks.GetWindsurfDiskUsage,
  cleanupWindsurf: AppHooks.CleanupWindsurf,
  cleanupStartupCache: AppHooks.CleanupStartupCache,
  cleanupAllSafe: AppHooks.CleanupAllSafe,
  getPerformanceTips: AppHooks.GetPerformanceTips,
  applyPerformanceFix: AppHooks.ApplyPerformanceFix,
  applyAllPerformanceFixes: AppHooks.ApplyAllPerformanceFixes,
  getWindsurfProcessInfo: AppHooks.GetWindsurfProcessInfo,
};
