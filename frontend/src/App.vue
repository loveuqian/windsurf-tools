<script setup lang="ts">
import {
  type Component,
  computed,
  defineAsyncComponent,
  onMounted,
  onUnmounted,
  ref,
  watch,
} from "vue";
import Header from "./components/layout/Header.vue";
import Sidebar from "./components/layout/Sidebar.vue";
import AppFooter from "./components/layout/AppFooter.vue";
import IConfirm from "./components/ios/IConfirm.vue";
import IToast from "./components/ios/IToast.vue";
import PageLoadingSkeleton from "./components/common/PageLoadingSkeleton.vue";
// 3 个核心 view 用 eager import 进入主 bundle —— 避免 Suspense fallback
// 把 PageLoadingSkeleton 顶替成空白主区（首次切到 Accounts 时 chunk 仍在解析
// 会让用户看到 6 个骨架卡片，体感像「号池加载不出来」）。
// 其余 5 个 view 保留懒加载，控制主 bundle 体积。
import DashboardView from "./views/Dashboard.vue";
import AccountsView from "./views/Accounts.vue";
import SettingsView from "./views/Settings.vue";
import { useAccountStore } from "./stores/useAccountStore";
import { useSettingsStore } from "./stores/useSettingsStore";
import { useMitmStatusStore } from "./stores/useMitmStatusStore";
import { useMainViewStore } from "./stores/useMainViewStore";
import {
  DEFAULT_MAIN_VIEW,
  type ShellViewTab,
} from "./utils/appMode";

const mainView = useMainViewStore();
const settings = useSettingsStore();
const mitmStore = useMitmStatusStore();
const shellReady = ref(false);
const mountedViews = ref<ShellViewTab[]>([]);
let unVisibilityRefresh: (() => void) | undefined;
let viewPreloadTimer: ReturnType<typeof setTimeout> | undefined;

type AsyncViewModule = { default: Component };

const viewLoaders: Record<ShellViewTab, () => Promise<AsyncViewModule>> = {
  Dashboard: () => import("./views/Dashboard.vue"),
  Accounts: () => import("./views/Accounts.vue"),
  Usage: () => import("./views/Usage.vue"),
  Relay: () => import("./views/Relay.vue"),
  Cleanup: () => import("./views/Cleanup.vue"),
  Settings: () => import("./views/Settings.vue"),
  Help: () => import("./views/Help.vue"),
  About: () => import("./views/About.vue"),
};

const preloadedViews = new Set<ShellViewTab>();

const viewRegistry = {
  // ── 核心 view：eager import，直接 component 不走 defineAsyncComponent
  //    避免 Suspense fallback 让用户初次切过来时看到骨架卡片。
  Dashboard: { component: DashboardView, skeleton: "dashboard", eager: true },
  Accounts: { component: AccountsView, skeleton: "accounts", eager: true },
  Settings: { component: SettingsView, skeleton: "settings", eager: true },
  // ── 次要 view：保留懒加载，控制主 bundle 体积。
  Usage: {
    component: defineAsyncComponent(viewLoaders.Usage),
    skeleton: "usage",
    eager: false,
  },
  Relay: {
    component: defineAsyncComponent(viewLoaders.Relay),
    skeleton: "relay",
    eager: false,
  },
  Cleanup: {
    component: defineAsyncComponent(viewLoaders.Cleanup),
    skeleton: "settings",
    eager: false,
  },
  Help: {
    component: defineAsyncComponent(viewLoaders.Help),
    skeleton: "settings",
    eager: false,
  },
  About: {
    component: defineAsyncComponent(viewLoaders.About),
    skeleton: "settings",
    eager: false,
  },
} as const;

const shellTabs = Object.keys(viewRegistry) as ShellViewTab[];

const resolveShellViewTab = (value: string | null | undefined): ShellViewTab =>
  shellTabs.includes(value as ShellViewTab)
    ? (value as ShellViewTab)
    : DEFAULT_MAIN_VIEW;

const ensureViewMounted = (tab: ShellViewTab) => {
  if (!mountedViews.value.includes(tab)) {
    mountedViews.value = [...mountedViews.value, tab];
  }
};

const preloadView = async (tab: ShellViewTab) => {
  // eager view 已在主 bundle 中，无需再次 import
  if (viewRegistry[tab].eager) {
    preloadedViews.add(tab);
    return;
  }
  if (preloadedViews.has(tab)) {
    return;
  }
  preloadedViews.add(tab);
  try {
    await viewLoaders[tab]();
  } catch (error) {
    preloadedViews.delete(tab);
    console.error(`Failed to preload ${tab} view:`, error);
  }
};

const scheduleBackgroundViewPreload = (activeTab?: ShellViewTab) => {
  if (viewPreloadTimer) {
    clearTimeout(viewPreloadTimer);
  }
  viewPreloadTimer = window.setTimeout(() => {
    for (const tab of shellTabs) {
      if (tab !== activeTab) {
        void preloadView(tab);
      }
    }
  }, 160);
};

const renderedViews = computed(() =>
  mountedViews.value.map((tab) => ({
    key: tab,
    component: viewRegistry[tab].component,
    skeleton: viewRegistry[tab].skeleton,
    eager: viewRegistry[tab].eager,
  })),
);

watch(
  () => mainView.activeTab,
  (value) => {
    const resolved = resolveShellViewTab(value);
    if (mainView.activeTab !== resolved) {
      mainView.activeTab = resolved;
    }
    ensureViewMounted(resolved);
    void preloadView(resolved);
    scheduleBackgroundViewPreload(resolved);
  },
  { immediate: true },
);

onMounted(async () => {
  const accounts = useAccountStore();
  await settings.fetchSettings();
  if (!(mainView.activeTab in viewRegistry)) {
    mainView.activeTab = DEFAULT_MAIN_VIEW;
  }
  shellReady.value = true;
  const currentTab = resolveShellViewTab(mainView.activeTab);
  ensureViewMounted(currentTab);
  void preloadView(currentTab);
  scheduleBackgroundViewPreload(currentTab);
  mitmStore.startPolling();
  void accounts.ensureAccountsLoaded().catch((error) => {
    console.error("App bootstrap accounts fetch failed:", error);
  });

  // 从后台切回前台时统一在这里刷新数据。
  // F2 修复：listener 单点持有 —— mitmStore 内部不再注册自己的 visibilitychange
  // listener，避免两个 listener 各自节流（2.5s vs 1.2s）造成重复 fetch。
  let lastFocusRefresh = 0;
  const onVisibilityChange = () => {
    if (
      typeof document === "undefined" ||
      document.visibilityState !== "visible"
    ) {
      return;
    }
    const now = Date.now();
    if (now - lastFocusRefresh < 2500) {
      return;
    }
    lastFocusRefresh = now;
    void accounts.fetchAccounts();
    mitmStore.notifyVisibleAgain();
  };
  document.addEventListener("visibilitychange", onVisibilityChange);
  unVisibilityRefresh = () =>
    document.removeEventListener("visibilitychange", onVisibilityChange);
});

onUnmounted(() => {
  mitmStore.stopPolling();
  unVisibilityRefresh?.();
  if (viewPreloadTimer) {
    clearTimeout(viewPreloadTimer);
    viewPreloadTimer = undefined;
  }
});
</script>

<template>
  <div
    class="flex flex-col h-full text-ios-text dark:text-ios-textDark overflow-hidden antialiased app-root"
  >
    <template v-if="!shellReady">
      <div class="flex-1 min-h-0 p-4">
        <div
          class="h-full rounded-ios-card backdrop-blur-2xl border border-black/[0.05] bg-white/72 dark:border-white/[0.08] dark:bg-[#1C1C1E]/82"
        />
      </div>
    </template>
    <template v-else>
      <Header />
      <div class="flex flex-1 overflow-hidden relative">
        <Sidebar
          :activeTab="mainView.activeTab"
          @update:activeTab="mainView.activeTab = $event"
        />
        <main
          class="flex-1 flex flex-col min-h-0 overflow-hidden relative bg-black/[0.01] dark:bg-white/[0.01]"
        >
          <div
            class="flex-1 overflow-y-auto overflow-x-hidden relative scroll-smooth min-h-0 flex flex-col"
          >
            <div class="flex-1 shrink-0 flex flex-col relative">
              <section
                v-for="view in renderedViews"
                :key="view.key"
                v-show="mainView.activeTab === view.key"
                class="flex-1 min-h-0 flex flex-col ios-view-surface"
                :aria-hidden="mainView.activeTab === view.key ? 'false' : 'true'"
              >
                <DashboardView v-if="view.key === 'Dashboard'" />
                <AccountsView v-else-if="view.key === 'Accounts'" />
                <SettingsView v-else-if="view.key === 'Settings'" />
                <Suspense v-else>
                  <component :is="view.component" />
                  <template #fallback>
                    <PageLoadingSkeleton :variant="view.skeleton" class="flex-1" />
                  </template>
                </Suspense>
              </section>
            </div>
            <AppFooter class="mt-auto" />
          </div>
        </main>
      </div>
    </template>
    <IConfirm />
    <IToast />
  </div>
</template>

<style scoped>
.ios-view-surface {
  contain: layout paint;
}
</style>
