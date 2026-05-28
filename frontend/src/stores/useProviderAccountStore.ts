import { create } from "zustand";
import { APIInfo, type ImportResult } from "../api/wails";
import { createAsyncResource } from "./_async";

// ════════════════════════════════════════════════════════════════
// useProviderAccountStore — 第三方 LLM 提供商账号(独立链路)
//
// 与 useAccountStore 物理隔离：不同后端 store(provider_accounts.json),
// 不同 wails API(importByProvider / getAllProviderAccounts / ...)。
// Windsurf 号池逻辑完全不动。
// ════════════════════════════════════════════════════════════════

export interface ProviderAccountModel {
  id: string;
  provider: string;
  base_url: string;
  auth_token: string;
  nickname?: string;
  remark?: string;
  status: string;
  created_at: string;
  last_used_at?: string;
  used_quota?: number;
  total_quota?: number;
  // 阶段 2: 路由调度字段
  activated?: boolean;
  active_model?: string;
  models?: string[];
  models_refreshed_at?: string;
  models_error?: string;
}

export interface ProviderImportItem {
  provider: string;
  base_url: string;
  token: string;
  remark?: string;
  nickname?: string;
}

interface ProviderAccountState {
  accounts: ProviderAccountModel[];
  isLoading: boolean;
  isRefreshing: boolean;
  hasLoadedOnce: boolean;
  actionLoading: boolean;

  // 当前全局唯一激活卡（activated=true && status!=disabled && 配置完整）
  activeAccount: () => ProviderAccountModel | null;

  fetchAccounts: (force?: boolean) => Promise<void>;
  ensureAccountsLoaded: (maxAgeMs?: number) => Promise<void>;
  deleteAccount: (id: string) => Promise<void>;
  importBatch: (items: ProviderImportItem[]) => Promise<ImportResult[]>;
  updateAccount: (acc: ProviderAccountModel) => Promise<void>;
  refreshModels: (id: string) => Promise<void>;
  // 翻到下一席;失败抛 Error("no_candidates" / "only_one")
  next: () => Promise<ProviderAccountModel>;
}

export const useProviderAccountStore = create<ProviderAccountState>((set, get) => {
  const resource = createAsyncResource<ProviderAccountModel[]>({
    ttlMs: 1500,
    yieldBeforeApply: true,
    fetcher: async () => {
      const data = await APIInfo.getAllProviderAccounts();
      return ((data || []) as ProviderAccountModel[]);
    },
    apply: (data) => set({ accounts: data }),
    onError: (e) => console.error("Failed to fetch provider accounts:", e),
    isHydrated: () => get().hasLoadedOnce && get().accounts.length > 0,
    setHydrated: () => set({ hasLoadedOnce: true }),
    shouldBlock: () => !get().hasLoadedOnce && get().accounts.length === 0,
    setLoading: (b) => set({ isLoading: b }),
    setRefreshing: (b) => set({ isRefreshing: b }),
    defaultEnsureAgeMs: 20_000,
  });

  return {
    accounts: [],
    isLoading: false,
    isRefreshing: false,
    hasLoadedOnce: false,
    actionLoading: false,

    activeAccount: () => {
      const found = get().accounts.find(
        (a) =>
          a.activated === true &&
          String(a.status || "active") !== "disabled" &&
          Boolean(String(a.base_url || "").trim()) &&
          Boolean(String(a.auth_token || "").trim()),
      );
      return found ?? null;
    },

    fetchAccounts: (force) => resource.fetch(force),
    ensureAccountsLoaded: (maxAgeMs) => resource.ensureLoaded(maxAgeMs),

    deleteAccount: async (id) => {
      await APIInfo.deleteProviderAccount(id);
      await resource.fetch(true);
    },

    importBatch: async (items) => {
      set({ actionLoading: true });
      try {
        const results = ((await APIInfo.importByProvider(items)) || []) as ImportResult[];
        await resource.fetch(true);
        return results;
      } finally {
        set({ actionLoading: false });
      }
    },

    updateAccount: async (acc) => {
      await APIInfo.updateProviderAccount(acc);
      await resource.fetch(true);
    },

    refreshModels: async (id) => {
      set({ actionLoading: true });
      try {
        await APIInfo.refreshProviderModels(id);
        await resource.fetch(true);
      } finally {
        set({ actionLoading: false });
      }
    },

    next: async () => {
      set({ actionLoading: true });
      try {
        const next = (await APIInfo.nextActiveAccount()) as ProviderAccountModel;
        await resource.fetch(true);
        return next;
      } finally {
        set({ actionLoading: false });
      }
    },
  };
});
