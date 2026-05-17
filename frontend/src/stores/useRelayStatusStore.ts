import { create } from "zustand";
import { APIInfo } from "../api/wails";

type RelayStatus = {
  running?: boolean;
  port?: number;
  url?: string;
};

interface RelayStatusState {
  status: RelayStatus | null;
  isLoading: boolean;
  isRefreshing: boolean;
  hasLoadedOnce: boolean;
  fetchStatus: (force?: boolean) => Promise<void> | void;
  ensureStatusLoaded: (maxAgeMs?: number) => Promise<void> | void;
}

let fetchInFlight: Promise<void> | null = null;
let lastFetchedAt = 0;

export const useRelayStatusStore = create<RelayStatusState>((set, get) => ({
  status: null,
  isLoading: false,
  isRefreshing: false,
  hasLoadedOnce: false,

  fetchStatus: async (force = false) => {
    const now = Date.now();
    if (fetchInFlight && !force) return fetchInFlight;
    if (!force && get().hasLoadedOnce && now - lastFetchedAt < 10_000) return;
    const blocking = !get().hasLoadedOnce;
    set(blocking ? { isLoading: true } : { isRefreshing: true });
    fetchInFlight = (async () => {
      try {
        const s = await APIInfo.getOpenAIRelayStatus();
        set({ status: s as RelayStatus });
      } catch (error) {
        console.error("getOpenAIRelayStatus error:", error);
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
}));
