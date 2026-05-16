import { defineStore } from "pinia";
import { ref } from "vue";
import { APIInfo } from "../api/wails";
import type { services } from "../../wailsjs/go/models";

/** 纯 MITM 壳层与工具栏共用状态，由 App 统一 start/stop 轮询 */
export const useMitmStatusStore = defineStore("mitmStatus", () => {
  const status = ref<services.MitmProxyStatus | null>(null);
  const isLoading = ref(false);
  const isRefreshing = ref(false);
  const hasLoadedOnce = ref(false);
  const switchLoading = ref(false);
  const switchTargetAccountId = ref("");
  let pollTimer: ReturnType<typeof setTimeout> | null = null;
  let fetchInFlight: Promise<void> | null = null;
  let lastFetchedAt = 0;

  const fetchStatus = async (force = false) => {
    const now = Date.now();
    // force=true 不复用 in-flight；切号后立刻刷状态需要拿到新 currentKey。
    if (fetchInFlight && !force) return fetchInFlight;
    if (!force && status.value && now - lastFetchedAt < 1200) {
      return;
    }
    // F3 修复：和 useAccountStore 对齐，已有数据时不再阻塞 UI。否则切回 tab
    // 触发 fetchStatus 时会短暂闪一次骨架屏。
    const blocking = !hasLoadedOnce.value && status.value == null;
    if (blocking) {
      isLoading.value = true;
    } else {
      isRefreshing.value = true;
    }
    fetchInFlight = (async () => {
      try {
        status.value = await APIInfo.getMitmProxyStatus();
      } catch (e) {
        console.error("GetMitmProxyStatus error:", e);
      } finally {
        lastFetchedAt = Date.now();
        hasLoadedOnce.value = true;
        if (blocking) {
          isLoading.value = false;
        } else {
          isRefreshing.value = false;
        }
        fetchInFlight = null;
      }
    })();
    return fetchInFlight;
  };

  const ensureStatusLoaded = async (maxAgeMs = 10_000) => {
    const now = Date.now();
    if (hasLoadedOnce.value && now - lastFetchedAt < maxAgeMs) {
      return;
    }
    return fetchStatus();
  };

  const nextPollDelay = () => (status.value?.running ? 8000 : 15000);

  const scheduleNextTick = () => {
    if (pollTimer) {
      clearTimeout(pollTimer);
    }
    pollTimer = setTimeout(() => {
      if (
        typeof document !== "undefined" &&
        document.visibilityState !== "visible"
      ) {
        scheduleNextTick();
        return;
      }
      void fetchStatus().finally(scheduleNextTick);
    }, nextPollDelay());
  };

  const startPolling = () => {
    if (pollTimer) return;
    void fetchStatus().finally(scheduleNextTick);
  };

  const stopPolling = () => {
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
  };

  // notifyVisibleAgain 由 App.vue 的统一 visibilitychange listener 调用。
  // 之前 store 自己监听 visibilitychange，与 App.vue 重复，每次切回前台
  // 都触发两次 fetch。现在改成显式上推：listener 单点持有，store 只管刷新。
  const notifyVisibleAgain = () => {
    if (!pollTimer) {
      // polling 未启动 → 只刷一次最新状态，不重启循环
      void fetchStatus(true);
      return;
    }
    void fetchStatus(true).finally(scheduleNextTick);
  };

  const switchToNext = async () => {
    switchLoading.value = true;
    switchTargetAccountId.value = "";
    try {
      const result = await APIInfo.switchMitmToNext();
      await fetchStatus(true);
      return result;
    } finally {
      switchLoading.value = false;
    }
  };

  const switchToAccount = async (accountID: string) => {
    switchLoading.value = true;
    switchTargetAccountId.value = accountID;
    try {
      const result = await APIInfo.switchMitmToAccount(accountID);
      await fetchStatus(true);
      return result;
    } finally {
      switchLoading.value = false;
      switchTargetAccountId.value = "";
    }
  };

  const sessionCount = () => status.value?.session_count ?? 0;
  const activeSessions = () => status.value?.active_sessions ?? [];

  const unbindSession = async (convIDPrefix: string) => {
    try {
      const ok = await APIInfo.unbindMitmSession(convIDPrefix);
      if (ok) {
        await fetchStatus(true);
      }
      return ok;
    } catch (e) {
      console.error("UnbindMitmSession error:", e);
      return false;
    }
  };

  return {
    status,
    isLoading,
    isRefreshing,
    hasLoadedOnce,
    switchLoading,
    switchTargetAccountId,
    fetchStatus,
    ensureStatusLoaded,
    startPolling,
    stopPolling,
    notifyVisibleAgain,
    switchToNext,
    switchToAccount,
    sessionCount,
    activeSessions,
    unbindSession,
  };
});
