import { create } from "zustand";
import { APIInfo } from "../api/wails";
import type { models } from "../../wailsjs/go/models";

interface AccountState {
  accounts: models.Account[];
  isLoading: boolean;
  isRefreshing: boolean;
  hasLoadedOnce: boolean;
  actionLoading: boolean;

  patchAccount: (account: models.Account | null | undefined) => models.Account | null;
  fetchAccounts: (force?: boolean) => Promise<void> | void;
  ensureAccountsLoaded: (maxAgeMs?: number) => Promise<void> | void;
  deleteAccount: (id: string) => Promise<void>;
  cleanExpiredAccounts: () => Promise<number>;
  deleteFreePlanAccounts: () => Promise<number>;
  refreshAllTokens: () => Promise<Record<string, string>>;
  refreshAllQuotas: () => Promise<Record<string, string>>;
  refreshAccountQuota: (id: string) => Promise<models.Account | null>;
}

let fetchInFlight: Promise<void> | null = null;
let lastFetchedAt = 0;

export const useAccountStore = create<AccountState>((set, get) => ({
  accounts: [],
  isLoading: false,
  isRefreshing: false,
  hasLoadedOnce: false,
  actionLoading: false,

  patchAccount: (account) => {
    if (!account?.id) return null;
    set((s) => {
      const next = [...s.accounts];
      const idx = next.findIndex((item) => item.id === account.id);
      if (idx >= 0) {
        next[idx] = account;
      } else {
        next.unshift(account);
      }
      return { accounts: next };
    });
    return account;
  },

  fetchAccounts: async (force = false) => {
    const now = Date.now();
    // 关键：仅在 force=false 时复用 in-flight。force=true 是显式「我要最新数据」
    // 的语义（用户点刷新 / 切号后），不能让旧 in-flight 把旧快照当结果返回。
    if (fetchInFlight && !force) {
      return fetchInFlight;
    }
    if (!force && now - lastFetchedAt < 1500) {
      return;
    }
    const blocking = !get().hasLoadedOnce && get().accounts.length === 0;
    set(blocking ? { isLoading: true } : { isRefreshing: true });
    fetchInFlight = (async () => {
      try {
        const data = await APIInfo.getAllAccounts();
        // 让出主线程一帧，减轻大列表回填时的界面卡顿
        await new Promise<void>((resolve) =>
          requestAnimationFrame(() => resolve()),
        );
        set({ accounts: data || [] });
        lastFetchedAt = Date.now();
        set({ hasLoadedOnce: true });
      } catch (e) {
        console.error("Failed to fetch accounts:", e);
      } finally {
        set({ hasLoadedOnce: true });
        if (blocking) set({ isLoading: false });
        else set({ isRefreshing: false });
        fetchInFlight = null;
      }
    })();
    return fetchInFlight;
  },

  ensureAccountsLoaded: async (maxAgeMs = 20_000) => {
    const now = Date.now();
    if (get().hasLoadedOnce && now - lastFetchedAt < maxAgeMs) return;
    return get().fetchAccounts();
  },

  deleteAccount: async (id) => {
    await APIInfo.deleteAccount(id);
    await get().fetchAccounts(true);
  },

  cleanExpiredAccounts: async () => {
    const n = await APIInfo.deleteExpiredAccounts();
    await get().fetchAccounts(true);
    return n;
  },

  deleteFreePlanAccounts: async () => {
    const n = await APIInfo.deleteFreePlanAccounts();
    await get().fetchAccounts(true);
    return n;
  },

  refreshAllTokens: async () => {
    set({ actionLoading: true });
    try {
      const result = await APIInfo.refreshAllTokens();
      await get().fetchAccounts(true);
      return result || {};
    } finally {
      set({ actionLoading: false });
    }
  },

  refreshAllQuotas: async () => {
    set({ actionLoading: true });
    try {
      const result = await APIInfo.refreshAllQuotas();
      await get().fetchAccounts(true);
      return result || {};
    } finally {
      set({ actionLoading: false });
    }
  },

  refreshAccountQuota: async (id) => {
    await APIInfo.refreshAccountQuota(id);
    const updated = await APIInfo.getAccount(id);
    return get().patchAccount(updated);
  },
}));
