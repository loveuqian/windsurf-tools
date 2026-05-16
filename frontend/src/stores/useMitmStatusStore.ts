import { create } from "zustand";
import { APIInfo } from "../api/wails";
import type { services } from "../../wailsjs/go/models";

interface MitmStatusState {
  status: services.MitmProxyStatus | null;
  isLoading: boolean;
  isRefreshing: boolean;
  hasLoadedOnce: boolean;
  switchLoading: boolean;
  switchTargetAccountId: string;

  fetchStatus: (force?: boolean) => Promise<void> | void;
  ensureStatusLoaded: (maxAgeMs?: number) => Promise<void> | void;
  startPolling: () => void;
  stopPolling: () => void;
  notifyVisibleAgain: () => void;
  switchToNext: () => Promise<string>;
  switchToAccount: (accountID: string) => Promise<string>;
  sessionCount: () => number;
  activeSessions: () => services.SessionBindingInfo[];
  unbindSession: (convIDPrefix: string) => Promise<boolean>;
}

let pollTimer: ReturnType<typeof setTimeout> | null = null;
let fetchInFlight: Promise<void> | null = null;
let lastFetchedAt = 0;

const nextPollDelay = (running: boolean | undefined) =>
  running ? 8000 : 15000;

export const useMitmStatusStore = create<MitmStatusState>((set, get) => {
  const scheduleNextTick = () => {
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = setTimeout(() => {
      if (
        typeof document !== "undefined" &&
        document.visibilityState !== "visible"
      ) {
        scheduleNextTick();
        return;
      }
      const p = get().fetchStatus();
      if (p && typeof (p as Promise<void>).finally === "function") {
        (p as Promise<void>).finally(scheduleNextTick);
      } else {
        scheduleNextTick();
      }
    }, nextPollDelay(get().status?.running));
  };

  return {
    status: null,
    isLoading: false,
    isRefreshing: false,
    hasLoadedOnce: false,
    switchLoading: false,
    switchTargetAccountId: "",

    fetchStatus: async (force = false) => {
      const now = Date.now();
      if (fetchInFlight && !force) return fetchInFlight;
      if (!force && get().status && now - lastFetchedAt < 1200) return;
      // F3 修复：和 useAccountStore 对齐，已有数据时不再阻塞 UI。否则切回 tab
      // 触发 fetchStatus 时会短暂闪一次骨架屏。
      const blocking = !get().hasLoadedOnce && get().status == null;
      set(blocking ? { isLoading: true } : { isRefreshing: true });
      fetchInFlight = (async () => {
        try {
          const s = await APIInfo.getMitmProxyStatus();
          set({ status: s });
        } catch (e) {
          console.error("GetMitmProxyStatus error:", e);
        } finally {
          lastFetchedAt = Date.now();
          set({ hasLoadedOnce: true });
          if (blocking) set({ isLoading: false });
          else set({ isRefreshing: false });
          fetchInFlight = null;
        }
      })();
      return fetchInFlight;
    },

    ensureStatusLoaded: async (maxAgeMs = 10_000) => {
      const now = Date.now();
      if (get().hasLoadedOnce && now - lastFetchedAt < maxAgeMs) return;
      return get().fetchStatus();
    },

    startPolling: () => {
      if (pollTimer) return;
      const p = get().fetchStatus();
      if (p && typeof (p as Promise<void>).finally === "function") {
        (p as Promise<void>).finally(scheduleNextTick);
      } else {
        scheduleNextTick();
      }
    },

    stopPolling: () => {
      if (pollTimer) {
        clearTimeout(pollTimer);
        pollTimer = null;
      }
    },

    // notifyVisibleAgain 由 App.tsx 的统一 visibilitychange listener 调用。
    notifyVisibleAgain: () => {
      if (!pollTimer) {
        // polling 未启动 → 只刷一次最新状态，不重启循环
        void get().fetchStatus(true);
        return;
      }
      const p = get().fetchStatus(true);
      if (p && typeof (p as Promise<void>).finally === "function") {
        (p as Promise<void>).finally(scheduleNextTick);
      } else {
        scheduleNextTick();
      }
    },

    switchToNext: async () => {
      set({ switchLoading: true, switchTargetAccountId: "" });
      try {
        const result = await APIInfo.switchMitmToNext();
        await get().fetchStatus(true);
        return result;
      } finally {
        set({ switchLoading: false });
      }
    },

    switchToAccount: async (accountID) => {
      set({ switchLoading: true, switchTargetAccountId: accountID });
      try {
        const result = await APIInfo.switchMitmToAccount(accountID);
        await get().fetchStatus(true);
        return result;
      } finally {
        set({ switchLoading: false, switchTargetAccountId: "" });
      }
    },

    sessionCount: () => get().status?.session_count ?? 0,
    activeSessions: () => get().status?.active_sessions ?? [],

    unbindSession: async (convIDPrefix) => {
      try {
        const ok = await APIInfo.unbindMitmSession(convIDPrefix);
        if (ok) await get().fetchStatus(true);
        return ok;
      } catch (e) {
        console.error("UnbindMitmSession error:", e);
        return false;
      }
    },
  };
});
