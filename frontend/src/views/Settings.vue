<script setup lang="ts">
import {
  computed,
  nextTick,
  onMounted,
  onUnmounted,
  reactive,
  ref,
  watch,
} from "vue";
import { useSettingsStore } from "../stores/useSettingsStore";
import { useAccountStore } from "../stores/useAccountStore";
import IToggle from "../components/ios/IToggle.vue";
import {
  clampHotPollSeconds,
  clampQuotaMinutes,
  createDefaultSettings,
  formToSettings,
  normalizeSwitchPlanFilter,
  quotaPolicyOptions,
  settingsToForm,
  switchPlanFilterToneOptions,
  type SettingsForm,
} from "../utils/settingsModel";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton.vue";
import SkeletonBlock from "../components/common/SkeletonBlock.vue";
import {
  CheckCircle2,
  Loader2,
  RefreshCcw,
  RotateCcw,
  Save,
  Radio,
  Shuffle,
  Zap,
  Globe,
  Sparkles,
} from "lucide-vue-next";
import { confirmDialog, showToast } from "../utils/toast";
import { APIInfo } from "../api/wails";

const settingsStore = useSettingsStore();
const accountStore = useAccountStore();
let autoSaveDebounceTimer: ReturnType<typeof setTimeout> | null = null;
let saveStateResetTimer: ReturnType<typeof setTimeout> | null = null;

const isSaving = ref(false);
const showSaved = ref(false);
const isSyncingLocal = ref(true);
const saveState = ref<"idle" | "saving" | "saved" | "error">("idle");
const lastSavedFingerprint = ref("");
const relayStatusLoaded = ref(false);
const local = reactive<SettingsForm>(settingsToForm(createDefaultSettings()));

// ── 套餐多选 checkbox helpers ──
const planFilterSet = computed(() => {
  const v = local.auto_switch_plan_filter;
  if (!v || v === 'all') return new Set<string>();
  return new Set(v.split(',').map((s) => s.trim()).filter(Boolean));
});
const planFilterActive = (tone: string) => {
  const s = planFilterSet.value;
  return s.size === 0 || s.has(tone);
};
const togglePlanFilter = (tone: string) => {
  const current = planFilterSet.value;
  const allTones = switchPlanFilterToneOptions.map((o) => o.value);
  if (current.size === 0) {
    // currently "all" → uncheck this one = select everything except this
    const next = allTones.filter((t) => t !== tone);
    local.auto_switch_plan_filter = normalizeSwitchPlanFilter(next.join(','));
  } else if (current.has(tone)) {
    current.delete(tone);
    local.auto_switch_plan_filter = normalizeSwitchPlanFilter([...current].join(',') || 'all');
  } else {
    current.add(tone);
    // if all selected → normalize to "all"
    local.auto_switch_plan_filter = normalizeSwitchPlanFilter([...current].join(','));
  }
};

onMounted(() => {
  void settingsStore.fetchSettings();
  void fetchRelayStatus();
  void fetchClashStatus();
  // 破限 v1.2.0：拉一次 preset 列表 + runtime 状态。手动刷新策略，不轮询。
  void fetchJailbreakPresets();
  void fetchJailbreakRuntime();
  // v1.3.0：pin/pool 状态 + 账号列表（轮换池成员选择需要）
  void fetchPinStatus();
  void fetchPoolStatus();
  void accountStore.ensureAccountsLoaded();
});

watch(
  () => settingsStore.settings,
  (s) => {
    if (s) {
      isSyncingLocal.value = true;
      Object.assign(local, settingsToForm(s));
      lastSavedFingerprint.value = buildSettingsFingerprint();
      nextTick(() => {
        isSyncingLocal.value = false;
      });
    }
  },
  { immediate: true },
);

watch(
  () => ({
    ...local,
    quota_custom_interval_minutes: local.quota_custom_interval_minutes,
    quota_hot_poll_seconds: local.quota_hot_poll_seconds,
    concurrent_limit: local.concurrent_limit,
  }),
  () => {
    if (isSyncingLocal.value) {
      return;
    }
    scheduleAutoSave();
  },
  { deep: true },
);

const buildSettingsPayload = () => formToSettings(local);

const buildSettingsFingerprint = () => JSON.stringify(buildSettingsPayload());

const resetSavedStateLater = () => {
  if (saveStateResetTimer) {
    clearTimeout(saveStateResetTimer);
  }
  saveStateResetTimer = setTimeout(() => {
    if (saveState.value === "saved") {
      saveState.value = "idle";
      showSaved.value = false;
    }
  }, 1600);
};

const persistLocalSettings = async () => {
  const fingerprint = buildSettingsFingerprint();
  if (fingerprint === lastSavedFingerprint.value) {
    return;
  }
  isSaving.value = true;
  saveState.value = "saving";
  try {
    const payload = buildSettingsPayload();
    await settingsStore.updateSettings(payload);
    lastSavedFingerprint.value = fingerprint;
    saveState.value = "saved";
    showSaved.value = true;
    resetSavedStateLater();
    // ★ Clash 轮换是 settings 副作用启停的（applyClashRotatorSettings），
    //   保存完后必须重新查 running 状态，否则 toggle 已开但徽章一直显示"已停止"。
    void fetchClashStatus().finally(() => {
      clashSyncing.value = false;
    });
  } catch (e) {
    saveState.value = "error";
    showToast(`自动保存失败: ${String(e)}`, "error");
    clashSyncing.value = false;
  } finally {
    isSaving.value = false;
  }
};

const scheduleAutoSave = () => {
  if (autoSaveDebounceTimer) {
    clearTimeout(autoSaveDebounceTimer);
  }
  autoSaveDebounceTimer = setTimeout(() => {
    void persistLocalSettings();
  }, 420);
};

// ── OpenAI 中转 ──
const relayRunning = ref(false);
const relayLoading = ref(false);
const relayAddress = ref("");

const fetchRelayStatus = async () => {
  try {
    const st = await APIInfo.getOpenAIRelayStatus();
    relayRunning.value = Boolean(st.running);
    relayAddress.value = String(st.url || "");
  } catch {
    /* ignore */
  } finally {
    relayStatusLoaded.value = true;
  }
};

const handleRelayToggle = async (enabled: boolean) => {
  relayLoading.value = true;
  try {
    if (enabled) {
      await APIInfo.startOpenAIRelay(
        local.openai_relay_port || 8787,
        local.openai_relay_secret || "",
      );
      showToast("OpenAI 中转已启动", "success");
    } else {
      await APIInfo.stopOpenAIRelay();
      showToast("OpenAI 中转已停止", "success");
    }
    await fetchRelayStatus();
  } catch (e) {
    showToast(`中转操作失败: ${String(e)}`, "error");
  } finally {
    relayLoading.value = false;
  }
};

const copyRelayAddress = async () => {
  const addr =
    relayAddress.value || `http://127.0.0.1:${local.openai_relay_port || 8787}`;
  try {
    await navigator.clipboard.writeText(addr);
    showToast("地址已复制", "success");
  } catch {
    showToast("复制失败", "error");
  }
};

const relaySectionBooting = computed(() => !relayStatusLoaded.value);
const relaySectionRefreshing = computed(
  () => !relaySectionBooting.value && relayLoading.value,
);

// ── Clash IP 轮换 ──
const clashRunning = ref(false);
const clashSyncing = ref(false); // toggle 切到新值但 auto-save 还没落库时的过渡态
const clashLoading = ref(false);
const clashTestResult = ref<string>("");
const clashTestOk = ref(false);
const clashNodes = ref<string[]>([]);
const clashNodesLoading = ref(false);

// 监听 toggle 变化（用户主动切换时）→ 立刻进入 "同步中" 过渡态，
// 避免出现"toggle 已开 + 徽章仍显示已停止"的视觉冲突。
watch(
  () => local.clash_rotate_enabled,
  (next, prev) => {
    if (isSyncingLocal.value) return; // 来自 settingsStore 的同步赋值，不是用户操作
    if (next === prev) return;
    clashSyncing.value = true;
  },
);

const fetchClashStatus = async () => {
  try {
    const running = await APIInfo.getClashRotatorRunning();
    clashRunning.value = Boolean(running);
  } catch {
    /* ignore */
  }
};

// 「恢复默认」按钮：从后端拿 services.DefaultJailbreakOverride 写入 local 表单。
// 走异步是为了避免前端硬编码长文本（一旦后端文案微调前端会漂移）。
const restoreJailbreakDefault = async () => {
  try {
    const text = await APIInfo.getJailbreakDefaultOverride();
    if (typeof text === "string" && text.trim() !== "") {
      local.mitm_jailbreak_override = text;
    }
  } catch {
    /* ignore — 用户可手动清空，后端 fallback 会兜底 */
  }
};

// ── 破限增强 v1.2.0：preset / file / runtime stats ──
type JailbreakPreset = {
  id: string;
  name: string;
  description: string;
  risk: string;
  text: string;
};
type JailbreakRuntime = {
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
};

const jailbreakPresets = ref<JailbreakPreset[]>([]);
const jailbreakRuntime = ref<JailbreakRuntime | null>(null);
const jailbreakRuntimeLoading = ref(false);

const fetchJailbreakPresets = async () => {
  try {
    jailbreakPresets.value = (await APIInfo.listJailbreakPresets()) || [];
  } catch (e) {
    console.warn("listJailbreakPresets failed", e);
  }
};

const fetchJailbreakRuntime = async () => {
  jailbreakRuntimeLoading.value = true;
  try {
    jailbreakRuntime.value = await APIInfo.getJailbreakRuntime();
  } catch (e) {
    console.warn("getJailbreakRuntime failed", e);
  } finally {
    jailbreakRuntimeLoading.value = false;
  }
};

// 应用预设：把 preset.text 灌进 textarea 同时切到 custom 源，避免选了 preset
// 又编辑 textarea 时困惑「我改了 textarea 为什么没生效？」。
const applyJailbreakPreset = (id: string) => {
  local.mitm_jailbreak_preset_id = id;
  if (id === "custom") return; // custom 不动 textarea
  const p = jailbreakPresets.value.find((x) => x.id === id);
  if (p && p.text) {
    local.mitm_jailbreak_override = p.text;
  }
};

const handleOpenOverrideFile = async () => {
  try {
    const path = await APIInfo.openJailbreakOverrideFile();
    showToast(`已用默认编辑器打开 ${path}`, "success");
    setTimeout(fetchJailbreakRuntime, 500);
  } catch (e) {
    showToast(`打开失败: ${String(e)}`, "error");
  }
};

const handleRevealOverrideFolder = async () => {
  try {
    await APIInfo.revealJailbreakOverrideFolder();
  } catch (e) {
    showToast(`显示位置失败: ${String(e)}`, "error");
  }
};

const handleSaveOverrideToFile = async () => {
  const text = local.mitm_jailbreak_override.trim() || "";
  if (!text) {
    showToast("当前 textarea 为空，先编辑或选预设再保存", "warning");
    return;
  }
  try {
    const path = await APIInfo.saveJailbreakOverrideFile(text);
    showToast(`已保存到 ${path}`, "success");
    await fetchJailbreakRuntime();
  } catch (e) {
    showToast(`保存失败: ${String(e)}`, "error");
  }
};

const handleResetJailbreakStats = async () => {
  try {
    await APIInfo.resetJailbreakStats();
    await fetchJailbreakRuntime();
    showToast("注入计数已清零", "success");
  } catch (e) {
    showToast(`清零失败: ${String(e)}`, "error");
  }
};

const jailbreakPresetByID = (id: string) =>
  jailbreakPresets.value.find((p) => p.id === id) ?? null;

const formatRelativeTime = (ms: number): string => {
  if (ms < 0) return "—";
  if (ms < 1000) return "刚才";
  if (ms < 60_000) return `${Math.floor(ms / 1000)} 秒前`;
  if (ms < 3600_000) return `${Math.floor(ms / 60_000)} 分钟前`;
  if (ms < 86_400_000) return `${Math.floor(ms / 3600_000)} 小时前`;
  return `${Math.floor(ms / 86_400_000)} 天前`;
};

const riskBadgeClass = (risk: string) => {
  switch (risk) {
    case "low":
      return "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
    case "medium":
      return "bg-amber-500/10 text-amber-700 dark:text-amber-300";
    case "high":
      return "bg-rose-500/10 text-rose-700 dark:text-rose-300";
    default:
      return "bg-gray-500/10 text-gray-700 dark:text-gray-300";
  }
};

const riskBadgeLabel = (risk: string) =>
  ({ low: "低风险", medium: "中风险", high: "高风险" })[risk] || "未知";

// ── v1.3.0 手动锁定 + 轮换池 ──
type ManualPinStatus = {
  enabled: boolean;
  account_id?: string;
  email?: string;
  nickname?: string;
};
type RotationPoolStatus = {
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
};

const pinStatus = ref<ManualPinStatus | null>(null);
const poolStatus = ref<RotationPoolStatus | null>(null);
const poolStatusLoading = ref(false);
const poolSearchQuery = ref("");

const fetchPinStatus = async () => {
  try {
    pinStatus.value = await APIInfo.getManualPinStatus();
  } catch (e) {
    console.warn("getManualPinStatus failed", e);
  }
};

const fetchPoolStatus = async () => {
  poolStatusLoading.value = true;
  try {
    poolStatus.value = await APIInfo.getRotationPoolStatus();
  } catch (e) {
    console.warn("getRotationPoolStatus failed", e);
  } finally {
    poolStatusLoading.value = false;
  }
};

const handleUnpinManual = async () => {
  try {
    await APIInfo.unpinManualAccount();
    await fetchPinStatus();
    await settingsStore.fetchSettings(true);
    showToast("已解除锁定，自动切换已恢复", "success");
  } catch (e) {
    showToast(`解锁失败: ${String(e)}`, "error");
  }
};

const handlePoolSwitchNow = async () => {
  try {
    await APIInfo.rotationPoolSwitchNow();
    await fetchPoolStatus();
    showToast("已触发立即切换", "success");
  } catch (e) {
    showToast(`切换失败: ${String(e)}`, "error");
  }
};

// ── v1.3.0 配置导出 / 导入 ──
// 用浏览器原生 download / input file。比 Wails SaveDialog 更稳，跨平台一致。
const handleExportSettings = async () => {
  try {
    const json = await APIInfo.exportSettings();
    const blob = new Blob([json], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    const ts = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
    a.download = `windsurf-tools-settings-${ts}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    showToast("配置已导出（敏感字段已剥离）", "success");
  } catch (e) {
    showToast(`导出失败: ${String(e)}`, "error");
  }
};

const handleImportSettings = async () => {
  const ok = await confirmDialog(
    "导入会覆盖当前设置（保留 Clash secret / Relay secret / Pin / 池成员）。确认继续？",
    { confirmText: "导入", cancelText: "取消" },
  );
  if (!ok) return;
  const input = document.createElement("input");
  input.type = "file";
  input.accept = ".json,application/json";
  input.onchange = async () => {
    const file = input.files?.[0];
    if (!file) return;
    try {
      const text = await file.text();
      await APIInfo.importSettings(text);
      await settingsStore.fetchSettings(true);
      showToast(`已从 ${file.name} 导入配置`, "success");
    } catch (e) {
      showToast(`导入失败: ${String(e)}`, "error");
    }
  };
  input.click();
};

const handlePoolRefreshQuotasNow = async () => {
  try {
    await APIInfo.rotationPoolRefreshQuotasNow();
    await accountStore.fetchAccounts(true);
    showToast("池内账号额度已开始刷新", "success");
    // 给后台 2 秒刷完再 fetch 状态
    setTimeout(fetchPoolStatus, 2000);
  } catch (e) {
    showToast(`刷新失败: ${String(e)}`, "error");
  }
};

const togglePoolMember = (id: string) => {
  const list = [...(local.rotation_pool_account_ids || [])];
  const idx = list.indexOf(id);
  if (idx >= 0) {
    list.splice(idx, 1);
  } else {
    list.push(id);
  }
  local.rotation_pool_account_ids = list;
};

const isPoolMember = (id: string) =>
  (local.rotation_pool_account_ids || []).includes(id);

const poolFilteredAccounts = computed(() => {
  const q = poolSearchQuery.value.trim().toLowerCase();
  const all = accountStore.accounts;
  if (!q) return all;
  return all.filter(
    (a) =>
      (a.email || "").toLowerCase().includes(q) ||
      (a.nickname || "").toLowerCase().includes(q) ||
      (a.plan_name || "").toLowerCase().includes(q),
  );
});

const poolMemberEmails = computed(() => {
  const ids = new Set(local.rotation_pool_account_ids || []);
  return accountStore.accounts
    .filter((a) => ids.has(a.id))
    .map((a) => a.email || a.id.slice(0, 8));
});

// ── 一键智能启用 Clash IP 轮换 ──
// 后端会：探活 → 自动挑 selector 组 → 写设置 → 启 rotator → 立即切一次
// 用户不需要懂 group / 白名单 / type 概念，点一下就好。
const clashAutoSetupLoading = ref(false);
const handleAutoSetupClash = async () => {
  if (!local.clash_controller_url) {
    showToast("请先填控制器地址（如 http://127.0.0.1:9097）", "error");
    return;
  }
  clashAutoSetupLoading.value = true;
  clashTestResult.value = "";
  try {
    const res = await APIInfo.autoSetupClash();
    if (res.ok) {
      clashTestOk.value = true;
      const switched =
        res.from && res.to && res.from !== res.to
          ? ` · 切换 ${res.from} → ${res.to}`
          : "";
      clashTestResult.value = `✓ 已启用 — 组「${res.group}」/ ${res.node_count} 真节点${switched}`;
      // 后端写了 settings；前端拉一次最新值更新本地表单 + 徽章
      await settingsStore.fetchSettings(true);
      await fetchClashStatus();
      showToast(`Clash 轮换已启用（组：${res.group}）`, "success");
    } else {
      clashTestOk.value = false;
      clashTestResult.value = `✗ ${res.error || "未知错误"}${
        res.hint ? " — " + res.hint : ""
      }`;
      showToast(res.error || "智能启用失败", "error");
    }
  } catch (e) {
    clashTestOk.value = false;
    clashTestResult.value = `异常: ${String(e)}`;
  } finally {
    clashAutoSetupLoading.value = false;
  }
};

const handleTestClash = async () => {
  clashLoading.value = true;
  clashTestResult.value = "";
  try {
    const res = await APIInfo.testClashController(
      local.clash_controller_url,
      local.clash_secret,
    );
    if (res?.ok) {
      clashTestOk.value = true;
      const groupCount = res.groups?.length ?? 0;
      clashTestResult.value = `连接成功 — 发现 ${groupCount} 个 selector 组`;
    } else {
      clashTestOk.value = false;
      clashTestResult.value = `连接失败: ${res?.error || "未知错误"}`;
    }
  } catch (e) {
    clashTestOk.value = false;
    clashTestResult.value = `测试异常: ${String(e)}`;
  } finally {
    clashLoading.value = false;
  }
};

const handleListClashNodes = async () => {
  if (!local.clash_controller_url) {
    showToast("请先填写控制器地址", "error");
    return;
  }
  clashNodesLoading.value = true;
  try {
    const groupLabel = local.clash_group?.trim() || "GLOBAL";
    const nodes: string[] = await APIInfo.listClashGroupNodes(local.clash_controller_url, local.clash_secret, local.clash_group);
    clashNodes.value = nodes || [];
    if (clashNodes.value.length === 0) {
      showToast(`代理组「${groupLabel}」下没有可用节点`, "error");
    } else {
      showToast(`「${groupLabel}」已获取 ${clashNodes.value.length} 个节点`, "success");
    }
  } catch (e) {
    showToast(`获取节点失败: ${String(e)}`, "error");
  } finally {
    clashNodesLoading.value = false;
  }
};

const handleTriggerRotate = async () => {
  try {
    await APIInfo.triggerClashRotate();
    showToast("已触发手动轮换", "success");
  } catch (e) {
    showToast(`触发失败: ${String(e)}`, "error");
  }
};

onUnmounted(() => {
  if (autoSaveDebounceTimer) {
    clearTimeout(autoSaveDebounceTimer);
    autoSaveDebounceTimer = null;
    void persistLocalSettings();
  }
  if (saveStateResetTimer) {
    clearTimeout(saveStateResetTimer);
    saveStateResetTimer = null;
  }
});
</script>

<template>
  <div class="p-6 md:p-8 max-w-4xl mx-auto w-full pb-10">
    <div class="flex items-start justify-between mb-8 shrink-0 flex-wrap gap-4">
      <div>
        <h1
          class="text-[32px] font-[800] text-gray-900 dark:text-gray-100 tracking-tight leading-none"
        >
          MITM 设置
        </h1>
        <p class="text-[13px] text-gray-500 dark:text-gray-400 font-medium mt-3">
          纯 MITM 模式：号池轮换、MITM 代理与 OpenAI Relay；全部设置自动保存
        </p>
      </div>
      <div
        class="inline-flex items-center gap-2 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2 text-[12px] font-semibold shadow-sm dark:border-white/[0.08] dark:bg-white/[0.05]"
        :class="{
          'text-ios-textSecondary dark:text-ios-textSecondaryDark':
            saveState === 'idle',
          'text-ios-blue': saveState === 'saving',
          'text-emerald-600 dark:text-emerald-300': saveState === 'saved',
          'text-rose-600 dark:text-rose-300': saveState === 'error',
        }"
      >
        <Loader2
          v-if="saveState === 'saving'"
          class="w-4 h-4 ios-spinner"
          stroke-width="2.4"
        />
        <CheckCircle2
          v-else-if="showSaved || saveState === 'saved'"
          class="w-4 h-4"
          stroke-width="2.4"
        />
        <Save v-else class="w-4 h-4" stroke-width="2.4" />
        <span>
          {{
            saveState === "saving"
              ? "自动保存中"
              : showSaved || saveState === "saved"
                ? "已自动保存"
                : saveState === "error"
                  ? "保存失败"
                  : "自动保存"
          }}
        </span>
      </div>
    </div>

    <Transition name="fade" mode="out-in">
      <div
        v-if="settingsStore.isLoading"
        key="settings-loading"
        class="space-y-8 w-full"
      >
        <PageLoadingSkeleton variant="settings" />
      </div>

      <div v-else key="settings-content" class="space-y-8">
        <!-- 使用模式 -->
        <section>
          <h2
            class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2"
          >
            使用模式
          </h2>
          <div
            class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
          >
            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  纯 MITM 模式
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  当前版本已经固定为纯 MITM 工作流：所有轮换都从号池、MITM
                  代理和 Relay 走，界面只保留这条主链路相关设置。
                </div>
              </div>
              <IToggle :model-value="true" :disabled="true" class="shrink-0" />
            </div>
            <div
              class="p-5 sm:p-6 bg-ios-blue/[0.05] dark:bg-ios-blue/[0.1] border-t border-black/[0.04] dark:border-white/[0.04]"
            >
              <div
                class="text-[14px] font-bold text-gray-900 dark:text-gray-100 mb-1"
              >
                Windows 默认以管理员权限启动
              </div>
              <div
                class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
              >
                Windows 版桌面包会在启动时直接申请管理员权限，这样 Hosts、CA
                证书、系统服务和代理相关动作都能一次完成，不需要进程起来后再补提权。
              </div>
            </div>
          </div>
        </section>

        <!-- OpenAI 中转 -->
        <section>
          <h2
            class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2"
          >
            OpenAI 协议中转
          </h2>
          <div
            v-if="relaySectionBooting"
            class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
            aria-busy="true"
            aria-label="Relay 状态加载中"
          >
            <div
              class="p-5 sm:p-6 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div
                class="flex flex-col sm:flex-row sm:items-center justify-between gap-4"
              >
                <div class="min-w-0 flex-1 space-y-3">
                  <SkeletonBlock class="h-5 w-40 rounded-lg" />
                  <SkeletonBlock class="h-4 w-[74%] rounded-lg" />
                </div>
                <SkeletonBlock class="h-10 w-24 rounded-[12px] shrink-0" />
              </div>
            </div>
            <div class="p-5 sm:p-6 bg-gray-50/50 dark:bg-black/10 space-y-4">
              <div class="flex flex-col sm:flex-row gap-4">
                <SkeletonBlock class="h-11 flex-1 rounded-[12px]" />
                <SkeletonBlock class="h-11 flex-1 rounded-[12px]" />
              </div>
              <SkeletonBlock class="h-14 w-full rounded-[14px]" />
              <SkeletonBlock class="h-4 w-[70%] rounded-md" />
            </div>
          </div>

          <SkeletonOverlay
            v-else
            :active="relaySectionRefreshing"
            label="Relay 配置刷新中"
            overlayClass="rounded-[24px] bg-white/45 backdrop-blur-[2px] dark:bg-[#1C1C1E]/45"
          >
            <div
              class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
            >
              <div
                class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
              >
                <div class="flex-1 pr-4">
                  <div class="flex items-center gap-2">
                    <div
                      class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                    >
                      启用中转服务器
                    </div>
                    <span
                      class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide"
                      :class="
                        relayRunning
                          ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                          : 'bg-slate-500/10 text-slate-700 dark:text-slate-300'
                      "
                    >
                      {{ relayRunning ? "运行中" : "已停止" }}
                    </span>
                  </div>
                  <div
                    class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                  >
                    在本地启动 OpenAI 兼容的 HTTP API，将
                    <code>/v1/chat/completions</code> 请求转发到 Windsurf
                    Cascade，自动从号池轮换账号。
                  </div>
                </div>
                <button
                  type="button"
                  class="no-drag-region shrink-0 px-5 py-2.5 rounded-[12px] font-bold text-[13px] ios-btn transition-colors disabled:opacity-50"
                  :class="
                    relayRunning
                      ? 'bg-rose-500/10 text-rose-700 dark:text-rose-300 hover:bg-rose-500/15'
                      : 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/15'
                  "
                  :disabled="relayLoading"
                  @click="handleRelayToggle(!relayRunning)"
                >
                  <span class="inline-flex items-center gap-2">
                    <Radio class="w-4 h-4" stroke-width="2.4" />
                    {{ relayRunning ? "停止" : "启动" }}
                  </span>
                </button>
              </div>
              <div class="p-5 sm:p-6 bg-gray-50/50 dark:bg-black/10 space-y-4">
                <div class="flex flex-col sm:flex-row gap-4">
                  <div class="flex-1 flex flex-col gap-1.5">
                    <label
                      class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                      >监听端口</label
                    >
                    <input
                      v-model.number="local.openai_relay_port"
                      type="number"
                      min="1"
                      max="65535"
                      class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[14px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                      placeholder="8787"
                    />
                  </div>
                  <div class="flex-1 flex flex-col gap-1.5">
                    <label
                      class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                      >Bearer 密钥（留空不鉴权）</label
                    >
                    <input
                      v-model="local.openai_relay_secret"
                      type="text"
                      class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[14px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                      placeholder="sk-your-secret"
                    />
                  </div>
                </div>
                <div
                  v-if="relayRunning"
                  class="flex items-center gap-3 rounded-[14px] border border-emerald-500/20 bg-emerald-500/10 px-3.5 py-3"
                >
                  <div
                    class="text-[12px] font-medium text-emerald-700 dark:text-emerald-300 flex-1"
                  >
                    API 地址：<code class="font-mono">{{
                      relayAddress ||
                      `http://127.0.0.1:${local.openai_relay_port || 8787}`
                    }}</code>
                  </div>
                  <button
                    type="button"
                    class="no-drag-region shrink-0 rounded-full bg-emerald-600/20 px-2.5 py-1 text-[10px] font-bold text-emerald-700 dark:text-emerald-300 hover:bg-emerald-600/30 transition-colors"
                    @click="copyRelayAddress"
                  >
                    复制
                  </button>
                </div>
                <div
                  class="text-[12px] text-gray-400 dark:text-gray-500 leading-relaxed"
                >
                  兼容所有 OpenAI SDK / ChatGPT 客户端。设置
                  <code>base_url</code> 为上面的地址即可。流式和非流式均支持。
                </div>
              </div>
            </div>
            <template #skeleton>
              <div
                class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
              >
                <div
                  class="p-5 sm:p-6 border-b border-black/[0.04] dark:border-white/[0.04]"
                >
                  <div
                    class="flex flex-col sm:flex-row sm:items-center justify-between gap-4"
                  >
                    <div class="min-w-0 flex-1 space-y-3">
                      <SkeletonBlock class="h-5 w-40 rounded-lg" />
                      <SkeletonBlock class="h-4 w-[74%] rounded-lg" />
                    </div>
                    <SkeletonBlock class="h-10 w-24 rounded-[12px] shrink-0" />
                  </div>
                </div>
                <div
                  class="p-5 sm:p-6 bg-gray-50/50 dark:bg-black/10 space-y-4"
                >
                  <div class="flex flex-col sm:flex-row gap-4">
                    <SkeletonBlock class="h-11 flex-1 rounded-[12px]" />
                    <SkeletonBlock class="h-11 flex-1 rounded-[12px]" />
                  </div>
                  <SkeletonBlock class="h-14 w-full rounded-[14px]" />
                  <SkeletonBlock class="h-4 w-[70%] rounded-md" />
                </div>
              </div>
            </template>
          </SkeletonOverlay>
        </section>

        <!-- Clash IP 轮换 -->
        <section>
          <h2
            class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2"
          >
            Clash IP 轮换
          </h2>
          <div
            class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
          >
            <!-- 启用开关 -->
            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div class="flex items-center gap-2">
                  <div
                    class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                  >
                    启用 Clash IP 自动轮换
                  </div>
                  <span
                    class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide"
                    :class="
                      clashSyncing
                        ? 'bg-amber-500/10 text-amber-700 dark:text-amber-300'
                        : clashRunning
                          ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                          : 'bg-slate-500/10 text-slate-700 dark:text-slate-300'
                    "
                  >
                    {{
                      clashSyncing
                        ? local.clash_rotate_enabled
                          ? "启动中…"
                          : "停止中…"
                        : clashRunning
                          ? "运行中"
                          : "已停止"
                    }}
                  </span>
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  通过 Clash Verge / Mihomo 外部控制器 REST API
                  定时切换代理节点，规避因固定 IP 导致的上游限速。
                </div>
              </div>
              <IToggle v-model="local.clash_rotate_enabled" class="shrink-0" />
            </div>

            <!-- 配置区（仅启用时显示） -->
            <template v-if="local.clash_rotate_enabled">
              <div class="p-5 sm:p-6 bg-gray-50/50 dark:bg-black/10 space-y-4 border-b border-black/[0.04] dark:border-white/[0.04]">
                <div class="flex flex-col sm:flex-row gap-4">
                  <div class="flex-1 flex flex-col gap-1.5">
                    <label
                      class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                      >控制器地址</label
                    >
                    <input
                      v-model="local.clash_controller_url"
                      type="text"
                      class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[14px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                      placeholder="http://127.0.0.1:9097"
                    />
                  </div>
                  <div class="flex-1 flex flex-col gap-1.5">
                    <label
                      class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                      >Secret（留空不鉴权）</label
                    >
                    <input
                      v-model="local.clash_secret"
                      type="text"
                      class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[14px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                      placeholder="your-clash-secret"
                    />
                  </div>
                </div>

                <!-- 测试连接 -->
                <div class="flex items-center gap-3">
                  <button
                    type="button"
                    class="no-drag-region shrink-0 px-4 py-2 rounded-[12px] font-bold text-[13px] ios-btn bg-ios-blue/10 text-ios-blue hover:bg-ios-blue/15 transition-colors disabled:opacity-50"
                    :disabled="clashLoading"
                    @click="handleTestClash"
                  >
                    <span class="inline-flex items-center gap-1.5">
                      <Loader2
                        v-if="clashLoading"
                        class="w-3.5 h-3.5 ios-spinner"
                        stroke-width="2.4"
                      />
                      <Zap v-else class="w-3.5 h-3.5" stroke-width="2.4" />
                      测试连接
                    </span>
                  </button>
                  <button
                    type="button"
                    class="no-drag-region shrink-0 px-4 py-2 rounded-[12px] font-bold text-[13px] ios-btn bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 hover:bg-emerald-500/20 transition-colors disabled:opacity-50"
                    :disabled="clashAutoSetupLoading"
                    @click="handleAutoSetupClash"
                    title="自动挑选最佳 selector 组 + 启用 + 立即切一次节点（一键完事，不用手填 group / 白名单）"
                  >
                    <span class="inline-flex items-center gap-1.5">
                      <Loader2
                        v-if="clashAutoSetupLoading"
                        class="w-3.5 h-3.5 ios-spinner"
                        stroke-width="2.4"
                      />
                      <Sparkles
                        v-else
                        class="w-3.5 h-3.5"
                        stroke-width="2.4"
                      />
                      智能启用
                    </span>
                  </button>
                  <span
                    v-if="clashTestResult"
                    class="text-[12px] font-medium"
                    :class="
                      clashTestOk
                        ? 'text-emerald-600 dark:text-emerald-400'
                        : 'text-rose-600 dark:text-rose-400'
                    "
                  >
                    {{ clashTestResult }}
                  </span>
                </div>
              </div>

              <!-- 代理组 & 节点白名单 -->
              <div class="p-5 sm:p-6 space-y-4 border-b border-black/[0.04] dark:border-white/[0.04]">
                <div class="flex flex-col gap-1.5">
                  <label
                    class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                    >代理组名称</label
                  >
                  <div class="flex items-center gap-2">
                    <input
                      v-model="local.clash_group"
                      type="text"
                      class="no-drag-region flex-1 bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[14px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                      placeholder="🚀 节点选择"
                    />
                    <button
                      type="button"
                      class="no-drag-region shrink-0 px-3 py-2.5 rounded-[12px] font-bold text-[12px] ios-btn bg-gray-100 dark:bg-white/5 hover:bg-gray-200/70 dark:hover:bg-white/10 text-gray-600 dark:text-gray-300 transition-colors disabled:opacity-50"
                      :disabled="clashNodesLoading"
                      @click="handleListClashNodes"
                    >
                      <span class="inline-flex items-center gap-1">
                        <Loader2
                          v-if="clashNodesLoading"
                          class="w-3 h-3 ios-spinner"
                          stroke-width="2.4"
                        />
                        <Globe v-else class="w-3 h-3" stroke-width="2.4" />
                        查看节点
                      </span>
                    </button>
                  </div>
                  <div
                    class="text-[12px] text-gray-400 dark:text-gray-500"
                  >
                    Clash 中的代理组名称（通常为 Selector 类型），留空使用 GLOBAL。
                  </div>
                </div>

                <!-- 节点列表展示 -->
                <div
                  v-if="clashNodes.length > 0"
                  class="rounded-[14px] border border-black/[0.04] dark:border-white/[0.04] bg-white/50 dark:bg-black/20 p-3 max-h-40 overflow-y-auto"
                >
                  <div class="text-[11px] font-bold text-gray-400 dark:text-gray-500 mb-2 uppercase tracking-wide">
                    当前组下 {{ clashNodes.length }} 个节点
                  </div>
                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="(node, i) in clashNodes"
                      :key="i"
                      class="inline-block rounded-full px-2.5 py-1 text-[11px] font-medium bg-gray-100 dark:bg-white/5 text-gray-600 dark:text-gray-400 border border-black/[0.03] dark:border-white/[0.06]"
                    >
                      {{ node }}
                    </span>
                  </div>
                </div>

                <div class="flex flex-col gap-1.5">
                  <label
                    class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                    >节点白名单（每行一个，留空不限）</label
                  >
                  <textarea
                    v-model="local.clash_nodes"
                    rows="3"
                    class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[13px] leading-relaxed focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow resize-y"
                    placeholder="🇺🇸 美国 01&#10;🇺🇸 美国 02&#10;🇯🇵 日本 03"
                  ></textarea>
                  <div
                    class="text-[12px] text-gray-400 dark:text-gray-500"
                  >
                    仅在这些节点间轮换，留空则使用代理组下全部节点。支持逗号或换行分隔。
                  </div>
                </div>
              </div>

              <!-- 轮换参数 -->
              <div class="p-5 sm:p-6 space-y-4 border-b border-black/[0.04] dark:border-white/[0.04]">
                <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <div class="flex-1 pr-4">
                    <div
                      class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                    >
                      轮换间隔
                    </div>
                    <div
                      class="text-[13px] text-gray-500 dark:text-gray-400 font-medium"
                    >
                      每隔多少分钟自动切换到下一个节点（2~60 分钟）。
                    </div>
                  </div>
                  <div
                    class="relative shrink-0 flex items-center bg-gray-100 dark:bg-black/20 rounded-[12px] px-3 py-1.5 focus-within:ring-2 focus-within:ring-ios-blue/30 border border-black/5 dark:border-white/5"
                  >
                    <input
                      v-model.number="local.clash_interval_minutes"
                      type="number"
                      min="2"
                      max="60"
                      class="no-drag-region w-14 text-center bg-transparent border-none text-[15px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                    />
                    <span class="text-[13px] font-bold text-gray-400 dark:text-gray-500 ml-1"
                      >min</span
                    >
                  </div>
                </div>

                <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <div class="flex-1 pr-4">
                    <div
                      class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                    >
                      限速时立即轮换
                    </div>
                    <div
                      class="text-[13px] text-gray-500 dark:text-gray-400 font-medium"
                    >
                      检测到上游 429 限速时不等间隔，立刻触发一次切换。
                    </div>
                  </div>
                  <IToggle
                    v-model="local.clash_rotate_on_rate_limit"
                    class="shrink-0"
                  />
                </div>
              </div>

              <!-- 延迟测试参数 -->
              <div class="p-5 sm:p-6 space-y-4 border-b border-black/[0.04] dark:border-white/[0.04]">
                <div class="flex flex-col sm:flex-row gap-4">
                  <div class="flex-1 flex flex-col gap-1.5">
                    <label
                      class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                      >延迟测试 URL</label
                    >
                    <input
                      v-model="local.clash_latency_test_url"
                      type="text"
                      class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[13px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                      placeholder="http://www.gstatic.com/generate_204"
                    />
                  </div>
                  <div class="w-32 flex flex-col gap-1.5">
                    <label
                      class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                      >最大延迟</label
                    >
                    <div
                      class="relative flex items-center bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 rounded-[12px] px-3 py-2.5 focus-within:ring-2 focus-within:ring-ios-blue/30"
                    >
                      <input
                        v-model.number="local.clash_latency_max_ms"
                        type="number"
                        min="0"
                        max="10000"
                        class="no-drag-region w-full bg-transparent border-none text-[14px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                      />
                      <span
                        class="text-[12px] font-bold text-gray-400 dark:text-gray-500 ml-1 shrink-0"
                        >ms</span
                      >
                    </div>
                  </div>
                </div>
                <div
                  class="text-[12px] text-gray-400 dark:text-gray-500 leading-relaxed"
                >
                  切换前会对候选节点做延迟测试，超过阈值的节点将被跳过。设为 0 跳过延迟测试。
                </div>
              </div>

              <!-- 手动轮换按钮 -->
              <div class="p-5 sm:p-6 flex items-center justify-between gap-4">
                <div class="flex-1 pr-4">
                  <div
                    class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                  >
                    手动立即轮换
                  </div>
                  <div
                    class="text-[13px] text-gray-500 dark:text-gray-400 font-medium"
                  >
                    点击按钮立即切换到下一个节点（无需等待间隔）。
                  </div>
                </div>
                <button
                  type="button"
                  class="no-drag-region shrink-0 px-5 py-2.5 rounded-[12px] font-bold text-[13px] ios-btn bg-ios-blue/10 text-ios-blue hover:bg-ios-blue/15 transition-colors"
                  @click="handleTriggerRotate"
                >
                  <span class="inline-flex items-center gap-2">
                    <Shuffle class="w-4 h-4" stroke-width="2.4" />
                    立即切换
                  </span>
                </button>
              </div>
            </template>
          </div>
        </section>

        <!-- 保活与额度同步 -->
        <section>
          <h2
            class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2"
          >
            后台保活与额度同步
          </h2>
          <div
            class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
          >
            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  自动刷新 Token
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  后台定时为账号池自动续期 JWT。
                </div>
              </div>
              <IToggle v-model="local.auto_refresh_tokens" class="shrink-0" />
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  定期同步额度
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  在后台定时从服务端核验最新可用配额，用于展示最新健康度。
                </div>
              </div>
              <IToggle v-model="local.auto_refresh_quotas" class="shrink-0" />
            </div>

            <div
              class="p-5 sm:p-6 border-b border-black/[0.04] dark:border-white/[0.04] bg-gray-50/50 dark:bg-black/10"
              v-if="local.auto_refresh_quotas"
            >
              <div class="flex flex-col gap-2 max-w-sm">
                <label
                  class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                  >全局额度同步策略</label
                >
                <select
                  v-model="local.quota_refresh_policy"
                  class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/10 dark:border-white/10 rounded-[12px] px-3 py-2.5 text-[14px] outline-none focus:ring-2 focus:ring-ios-blue/30 font-medium"
                >
                  <option
                    v-for="opt in quotaPolicyOptions"
                    :key="opt.value"
                    :value="opt.value"
                  >
                    {{ opt.label }}
                  </option>
                </select>
                <div
                  v-if="local.quota_refresh_policy === 'custom'"
                  class="pt-2"
                >
                  <label class="text-[12px] text-gray-500 dark:text-gray-400 font-bold mb-1 block"
                    >自定义分钟（5~10080）</label
                  >
                  <input
                    v-model.number="local.quota_custom_interval_minutes"
                    type="number"
                    min="5"
                    max="10080"
                    class="no-drag-region w-full bg-white dark:bg-[#1C1C1E] border border-black/10 dark:border-white/10 rounded-[12px] px-3 py-2.5 text-[14px] outline-none focus:ring-2"
                  />
                </div>
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  额度用尽自动切下席位
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  单独运行监控，仅紧盯正在使用的高频号。
                </div>
              </div>
              <IToggle
                v-model="local.auto_switch_on_quota_exhausted"
                :disabled="!local.auto_refresh_quotas"
                class="shrink-0"
              />
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
              v-if="
                local.auto_refresh_quotas &&
                local.auto_switch_on_quota_exhausted
              "
            >
              <div>
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  自动切号套餐范围
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  勾选允许自动轮换到哪些套餐类型，全选或不选等同于「不限制」。
                </div>
              </div>
              <div class="flex flex-wrap gap-2">
                <label
                  v-for="opt in switchPlanFilterToneOptions"
                  :key="opt.value"
                  @click.prevent="togglePlanFilter(opt.value)"
                  class="no-drag-region inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full border text-[13px] font-semibold cursor-pointer select-none transition-all duration-150"
                  :class="planFilterActive(opt.value)
                    ? 'bg-ios-blue/10 dark:bg-ios-blue/20 border-ios-blue/40 text-ios-blue shadow-sm'
                    : 'bg-gray-100 dark:bg-white/5 border-black/5 dark:border-white/10 text-gray-500 dark:text-gray-400 hover:bg-gray-200/70 dark:hover:bg-white/10'"
                >
                  <span
                    class="w-3.5 h-3.5 rounded-[4px] border-2 flex items-center justify-center transition-colors"
                    :class="planFilterActive(opt.value)
                      ? 'border-ios-blue bg-ios-blue'
                      : 'border-gray-300 dark:border-gray-600'"
                  >
                    <svg v-if="planFilterActive(opt.value)" class="w-2.5 h-2.5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3.5"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
                  </span>
                  {{ opt.label }}
                </label>
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
              v-if="
                local.auto_refresh_quotas &&
                local.auto_switch_on_quota_exhausted
              "
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  当前存活席位监控频率
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  最小 5 秒。建议
                  15-30。越低越容易察觉到额度耗尽，发包压力越高。
                </div>
              </div>
              <div
                class="relative shrink-0 flex items-center bg-gray-100 dark:bg-black/20 rounded-[12px] px-3 py-1.5 focus-within:ring-2 focus-within:ring-ios-blue/30 border border-black/5 dark:border-white/5"
              >
                <input
                  v-model.number="local.quota_hot_poll_seconds"
                  type="number"
                  min="5"
                  max="60"
                  class="no-drag-region w-14 text-center bg-transparent border-none text-[15px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                />
                <span class="text-[13px] font-bold text-gray-400 dark:text-gray-500 ml-1"
                  >sec</span
                >
              </div>
            </div>

            <!-- ★ v1.3.0 手动锁定状态条 -->
            <div
              v-if="pinStatus?.enabled"
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-amber-500/15 bg-amber-500/[0.06]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1 flex items-center gap-2">
                  <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
                  已锁定到 {{ pinStatus.nickname || pinStatus.email || pinStatus.account_id?.slice(0, 8) }}
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  手动切号后已自动锁定，3 个自动切换通道（额度耗尽 / 限速 / 热轮询）全部暂停。
                  点击「解锁」恢复自动行为。
                </div>
              </div>
              <button
                type="button"
                class="no-drag-region shrink-0 px-4 py-2 rounded-full font-bold text-[13px] bg-amber-500 text-white hover:bg-amber-600 transition-colors ios-btn"
                @click="handleUnpinManual"
              >
                解锁
              </button>
            </div>

            <!-- ★ v1.3.0 轮换池 -->
            <div
              class="p-5 sm:p-6 flex flex-col gap-4 border-b border-black/[0.04] dark:border-white/[0.04] bg-violet-500/[0.03]"
            >
              <div class="flex items-center justify-between gap-4">
                <div class="flex-1 pr-4">
                  <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1 flex items-center gap-2">
                    <span class="w-1.5 h-1.5 rounded-full bg-violet-500"></span>
                    轮换池 (Rotation Pool)
                  </div>
                  <div class="text-[13px] text-gray-500 dark:text-gray-400">
                    选 2 个以上账号进池，定时切 + 额度耗尽双触发只在池内来回切。
                    <b>池外账号完全不参与自动轮换</b>，池内账号高频刷额度让 UI 实时显示。
                  </div>
                </div>
                <IToggle v-model="local.rotation_pool_enabled" />
              </div>

              <div v-if="local.rotation_pool_enabled" class="flex flex-col gap-3">
                <!-- 间隔 + 额度刷新 -->
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div class="flex flex-col gap-1">
                    <label class="text-[12px] font-bold text-gray-600 dark:text-gray-400">
                      定时切间隔（分钟，1-60）
                    </label>
                    <input
                      v-model.number="local.rotation_pool_interval_min"
                      type="number"
                      min="1"
                      max="60"
                      class="no-drag-region w-full px-3 py-2 rounded-lg text-[14px] font-bold bg-white dark:bg-gray-900 border border-black/[0.08] dark:border-white/[0.08] focus:outline-none focus:ring-2 focus:ring-violet-500/40"
                    />
                  </div>
                  <div class="flex flex-col gap-1">
                    <label class="text-[12px] font-bold text-gray-600 dark:text-gray-400">
                      池内额度刷新间隔（分钟，1-10）
                    </label>
                    <input
                      v-model.number="local.rotation_pool_quota_refresh_min"
                      type="number"
                      min="1"
                      max="10"
                      class="no-drag-region w-full px-3 py-2 rounded-lg text-[14px] font-bold bg-white dark:bg-gray-900 border border-black/[0.08] dark:border-white/[0.08] focus:outline-none focus:ring-2 focus:ring-violet-500/40"
                    />
                  </div>
                </div>

                <!-- 当前池成员 -->
                <div v-if="poolMemberEmails.length > 0" class="flex flex-wrap gap-1.5">
                  <span class="text-[12px] font-bold text-gray-600 dark:text-gray-400 self-center">
                    池内 {{ poolMemberEmails.length }} 个:
                  </span>
                  <span
                    v-for="email in poolMemberEmails"
                    :key="email"
                    class="rounded-full bg-violet-500/15 border border-violet-500/25 px-2.5 py-1 text-[11px] font-bold text-violet-700 dark:text-violet-300"
                  >
                    {{ email }}
                  </span>
                </div>
                <div v-else class="text-[12px] text-amber-600 dark:text-amber-400">
                  ⚠ 还没选任何成员。下面勾选 2 个以上账号进池。
                </div>

                <!-- 账号选择列表 -->
                <div class="flex flex-col gap-2">
                  <div class="flex items-center justify-between gap-2">
                    <label class="text-[12px] font-bold text-gray-600 dark:text-gray-400">
                      池成员选择（{{ accountStore.accounts.length }} 个账号可选）
                    </label>
                    <input
                      v-model="poolSearchQuery"
                      type="text"
                      placeholder="搜邮箱/昵称/plan…"
                      class="no-drag-region flex-1 max-w-[200px] px-2 py-1 rounded text-[12px] bg-white dark:bg-gray-900 border border-black/[0.08] dark:border-white/[0.08] focus:outline-none focus:ring-1 focus:ring-violet-500/40"
                    />
                  </div>
                  <div
                    class="max-h-[240px] overflow-y-auto rounded-lg border border-black/[0.06] dark:border-white/[0.08] bg-white/60 dark:bg-white/[0.03]"
                  >
                    <div v-if="poolFilteredAccounts.length === 0" class="p-3 text-center text-[12px] text-gray-400">
                      {{ accountStore.accounts.length === 0 ? "号池为空，先去 Accounts 导入账号" : "无匹配账号" }}
                    </div>
                    <label
                      v-for="acc in poolFilteredAccounts"
                      :key="acc.id"
                      class="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-violet-500/[0.06] border-b border-black/[0.03] dark:border-white/[0.04] last:border-b-0"
                    >
                      <input
                        type="checkbox"
                        :checked="isPoolMember(acc.id)"
                        @change="togglePoolMember(acc.id)"
                        class="no-drag-region accent-violet-500"
                      />
                      <span class="font-mono text-[12px] text-gray-700 dark:text-gray-300 flex-1 truncate">
                        {{ acc.email || `(${acc.id.slice(0, 8)})` }}
                      </span>
                      <span
                        class="px-1.5 py-0.5 rounded text-[10px] font-bold bg-gray-100 dark:bg-white/[0.08] text-gray-600 dark:text-gray-400"
                      >
                        {{ acc.plan_name || "unknown" }}
                      </span>
                    </label>
                  </div>
                </div>

                <!-- 状态面板 -->
                <div
                  v-if="poolStatus"
                  class="rounded-xl border border-black/[0.06] dark:border-white/[0.08] bg-white/60 dark:bg-white/[0.03] p-3 flex flex-col gap-2"
                >
                  <div class="flex items-center justify-between">
                    <span class="text-[12px] font-bold text-gray-700 dark:text-gray-300">
                      运行时状态
                      <span
                        v-if="poolStatus.paused_by_pin"
                        class="ml-2 px-2 py-0.5 rounded-full bg-amber-500/15 text-amber-700 dark:text-amber-300 text-[10px] font-bold"
                      >
                        Pin 暂停中
                      </span>
                    </span>
                    <div class="flex gap-2">
                      <button
                        type="button"
                        class="text-[11px] font-bold text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
                        :disabled="poolStatusLoading"
                        @click="fetchPoolStatus"
                      >
                        {{ poolStatusLoading ? "刷新中…" : "刷新" }}
                      </button>
                      <button
                        type="button"
                        class="text-[11px] font-bold text-violet-600 hover:underline dark:text-violet-300"
                        @click="handlePoolSwitchNow"
                      >
                        立即切下一个
                      </button>
                      <button
                        type="button"
                        class="text-[11px] font-bold text-emerald-600 hover:underline dark:text-emerald-300"
                        @click="handlePoolRefreshQuotasNow"
                      >
                        立即刷池内额度
                      </button>
                    </div>
                  </div>
                  <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-[11.5px]">
                    <div class="rounded bg-black/[0.04] dark:bg-white/[0.05] px-2 py-1.5">
                      <div class="text-gray-500 dark:text-gray-400">成员数</div>
                      <div class="font-bold text-gray-800 dark:text-gray-100">
                        {{ poolStatus.member_count }}
                      </div>
                    </div>
                    <div class="rounded bg-black/[0.04] dark:bg-white/[0.05] px-2 py-1.5">
                      <div class="text-gray-500 dark:text-gray-400">已切换</div>
                      <div class="font-bold text-gray-800 dark:text-gray-100">
                        {{ poolStatus.total_switches }} 次
                      </div>
                    </div>
                    <div class="rounded bg-black/[0.04] dark:bg-white/[0.05] px-2 py-1.5">
                      <div class="text-gray-500 dark:text-gray-400">额度已刷</div>
                      <div class="font-bold text-gray-800 dark:text-gray-100">
                        {{ poolStatus.total_quota_refreshes }} 轮
                      </div>
                    </div>
                    <div class="rounded bg-black/[0.04] dark:bg-white/[0.05] px-2 py-1.5 truncate" :title="poolStatus.last_switched_to">
                      <div class="text-gray-500 dark:text-gray-400">上次切到</div>
                      <div class="font-bold text-gray-800 dark:text-gray-100 truncate">
                        {{ poolStatus.last_switched_to || "—" }}
                      </div>
                    </div>
                  </div>
                  <div
                    v-if="poolStatus.last_error"
                    class="rounded border border-rose-500/25 bg-rose-500/[0.08] px-2 py-1 text-[11px] text-rose-700 dark:text-rose-300 truncate"
                    :title="poolStatus.last_error"
                  >
                    最近错误: {{ poolStatus.last_error }}
                  </div>
                </div>
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  并发更新上限
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 flex items-center gap-2"
                >
                  JWT
                  与额度同步会按批次推进，这里控制每一批的并发上限，避免一次性把整个号池打满。
                </div>
              </div>
              <div
                class="relative shrink-0 flex items-center bg-gray-100 dark:bg-black/20 rounded-[12px] px-3 py-1.5 focus-within:ring-2 focus-within:ring-ios-blue/30 border border-black/5 dark:border-white/5"
              >
                <input
                  v-model.number="local.concurrent_limit"
                  type="number"
                  min="1"
                  max="50"
                  class="no-drag-region w-14 text-center bg-transparent border-none text-[15px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                />
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  导入并发数
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  批量导入账号时的最大并发数（1～20），值越大导入越快但更容易触发上游限流。
                </div>
              </div>
              <div
                class="relative shrink-0 flex items-center bg-gray-100 dark:bg-black/20 rounded-[12px] px-3 py-1.5 focus-within:ring-2 focus-within:ring-ios-blue/30 border border-black/5 dark:border-white/5"
              >
                <input
                  v-model.number="local.import_concurrency"
                  type="number"
                  min="1"
                  max="20"
                  class="no-drag-region w-14 text-center bg-transparent border-none text-[15px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                />
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  调试日志
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  开启后将代理、轮换、额度判定等关键操作写入 debug.log 文件。
                </div>
              </div>
              <IToggle v-model="local.debug_log" />
            </div>

            <!-- ★ v1.3.0 桌面通知 -->
            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">
                  桌面通知
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  关键事件弹系统通知中心：账号锁定解除 / 额度耗尽且切号失败 / Clash 异常。
                  60 秒内同类事件去重。
                </div>
              </div>
              <IToggle v-model="local.desktop_notifications" />
            </div>

            <!-- ★ v1.3.0 配置导出 / 导入 -->
            <div class="p-5 sm:p-6 flex flex-col gap-3">
              <div class="flex items-center justify-between gap-4 flex-wrap">
                <div class="flex-1 pr-4 min-w-[200px]">
                  <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">
                    配置导出 / 导入
                  </div>
                  <div class="text-[13px] text-gray-500 dark:text-gray-400">
                    导出当前设置为 JSON 文件备份 / 跨设备迁移。
                    <b>会自动剥离敏感字段</b>（Clash secret / Relay secret / Pin / 池成员），导入时也会保留这些。
                  </div>
                </div>
                <div class="flex flex-wrap gap-2 shrink-0">
                  <button
                    type="button"
                    class="no-drag-region px-3 py-2 rounded-lg text-[13px] font-bold bg-ios-blue/10 text-ios-blue hover:bg-ios-blue/20 transition-colors"
                    @click="handleExportSettings"
                  >
                    导出 JSON
                  </button>
                  <button
                    type="button"
                    class="no-drag-region px-3 py-2 rounded-lg text-[13px] font-bold bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/20 transition-colors"
                    @click="handleImportSettings"
                  >
                    从文件导入
                  </button>
                </div>
              </div>
            </div>
          </div>
        </section>

        <!-- 高级抓包与伪造专区 -->
        <section>
          <h2 class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2">
            高级抓包与诊断配置
          </h2>
          <div class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden">
            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">
                  全量离线抓包 (Full Capture)
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  记录代理过程中所有会话日志并落盘存入 <code>capture/</code> 目录下（JSONL 序列化）。
                </div>
              </div>
              <IToggle v-model="local.mitm_full_capture" />
            </div>

            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">
                  Protobuf 深度解包 (Debug Dump)
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  开启后将在底层将特权结构体与未知节点 dump 至 <code>proto_dumps/</code> 以供二次逆向研究。
                </div>
              </div>
              <IToggle v-model="local.mitm_debug_dump" />
            </div>

            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">
                  静态资源高速缓存拦截 (Cache Intercept)
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  内置直返 Codeium Bin 预构建离线缓存，减少跨域拉取耗时。
                </div>
              </div>
              <IToggle v-model="local.static_cache_intercept" />
            </div>

            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 bg-amber-500/[0.03]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1 flex items-center gap-2">
                  <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span> GetUserStatus伪装 (Forge)
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  强制劫盖响应，伪造为企业版无限额度状态（可能导致服务端反爬锁号，谨慎使用）。
                </div>
              </div>
              <IToggle v-model="local.forge_enabled" />
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col gap-4 bg-rose-500/[0.04] border-t border-black/[0.04] dark:border-white/[0.04]"
            >
              <!-- 标题 + 总开关 -->
              <div class="flex items-center justify-between gap-4">
                <div class="flex-1 pr-4">
                  <div
                    class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1 flex items-center gap-2"
                  >
                    <span class="w-1.5 h-1.5 rounded-full bg-rose-500"></span>
                    Cascade 破限注入 (Jailbreak Override)
                  </div>
                  <div class="text-[13px] text-gray-500 dark:text-gray-400">
                    在每次 GetChatMessage / GetCompletions 请求的 system prompt 末尾注入 override 文本，覆盖模型 alignment / 拒绝模板。等效于 Claude Code <code>--append-system-prompt-file</code>，走 MITM 协议层，<b>不动 IDE 任何文件</b>。
                  </div>
                </div>
                <IToggle v-model="local.mitm_jailbreak_enabled" />
              </div>

              <div v-if="local.mitm_jailbreak_enabled" class="flex flex-col gap-3">
                <!-- 预设下拉 -->
                <div class="flex flex-col gap-1.5">
                  <label class="text-[12px] font-bold text-gray-600 dark:text-gray-400">
                    预设模板
                  </label>
                  <div class="flex flex-wrap items-center gap-2">
                    <select
                      :value="local.mitm_jailbreak_preset_id"
                      @change="applyJailbreakPreset(($event.target as HTMLSelectElement).value)"
                      class="no-drag-region flex-1 min-w-[200px] px-3 py-2 rounded-lg text-[13px] font-medium bg-white dark:bg-gray-900 border border-black/[0.08] dark:border-white/[0.08] focus:outline-none focus:ring-2 focus:ring-rose-500/40"
                    >
                      <option v-for="p in jailbreakPresets" :key="p.id" :value="p.id">
                        {{ p.name }}
                      </option>
                    </select>
                    <span
                      v-if="jailbreakPresetByID(local.mitm_jailbreak_preset_id)"
                      class="px-2.5 py-1 rounded-full text-[11px] font-bold"
                      :class="riskBadgeClass(jailbreakPresetByID(local.mitm_jailbreak_preset_id)!.risk)"
                    >
                      {{ riskBadgeLabel(jailbreakPresetByID(local.mitm_jailbreak_preset_id)!.risk) }}
                    </span>
                  </div>
                  <div
                    v-if="jailbreakPresetByID(local.mitm_jailbreak_preset_id)"
                    class="text-[11.5px] text-gray-500 dark:text-gray-400"
                  >
                    {{ jailbreakPresetByID(local.mitm_jailbreak_preset_id)!.description }}
                  </div>
                </div>

                <!-- 来源切换 -->
                <div class="flex flex-col gap-1.5">
                  <label class="text-[12px] font-bold text-gray-600 dark:text-gray-400">
                    文本来源
                  </label>
                  <div class="flex gap-2">
                    <button
                      type="button"
                      class="no-drag-region flex-1 px-3 py-1.5 rounded-lg text-[12px] font-bold transition-colors"
                      :class="
                        local.mitm_jailbreak_override_source === 'inline'
                          ? 'bg-rose-500 text-white shadow-sm'
                          : 'bg-black/[0.04] text-gray-600 dark:bg-white/[0.06] dark:text-gray-400 hover:bg-black/[0.08]'
                      "
                      @click="local.mitm_jailbreak_override_source = 'inline'"
                    >
                      内置 textarea
                    </button>
                    <button
                      type="button"
                      class="no-drag-region flex-1 px-3 py-1.5 rounded-lg text-[12px] font-bold transition-colors"
                      :class="
                        local.mitm_jailbreak_override_source === 'file'
                          ? 'bg-rose-500 text-white shadow-sm'
                          : 'bg-black/[0.04] text-gray-600 dark:bg-white/[0.06] dark:text-gray-400 hover:bg-black/[0.08]'
                      "
                      @click="local.mitm_jailbreak_override_source = 'file'"
                    >
                      外部文件
                    </button>
                  </div>
                </div>

                <!-- 文件路径 + 操作按钮 -->
                <div v-if="local.mitm_jailbreak_override_source === 'file'" class="flex flex-col gap-2">
                  <input
                    v-model="local.mitm_jailbreak_override_file"
                    placeholder="~/.claude/override.md（留空 = 默认路径，与 Claude Code 共享）"
                    class="no-drag-region w-full px-3 py-2 rounded-lg text-[12px] font-mono bg-white dark:bg-gray-900 border border-black/[0.08] dark:border-white/[0.08] focus:outline-none focus:ring-2 focus:ring-rose-500/40"
                  />
                  <div class="flex flex-wrap gap-2">
                    <button
                      type="button"
                      class="no-drag-region px-3 py-1.5 rounded-lg text-[12px] font-bold bg-ios-blue/10 text-ios-blue hover:bg-ios-blue/20 transition-colors"
                      @click="handleOpenOverrideFile"
                    >
                      打开编辑
                    </button>
                    <button
                      type="button"
                      class="no-drag-region px-3 py-1.5 rounded-lg text-[12px] font-bold bg-violet-500/10 text-violet-700 dark:text-violet-300 hover:bg-violet-500/20 transition-colors"
                      @click="handleRevealOverrideFolder"
                    >
                      在文件管理器显示
                    </button>
                    <button
                      type="button"
                      class="no-drag-region px-3 py-1.5 rounded-lg text-[12px] font-bold bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/20 transition-colors"
                      @click="handleSaveOverrideToFile"
                    >
                      把当前文本保存到文件
                    </button>
                  </div>
                </div>

                <!-- inline textarea（仅 source=inline 时显示） -->
                <div v-if="local.mitm_jailbreak_override_source !== 'file'" class="flex flex-col gap-1.5">
                  <div class="flex items-center justify-between text-[12px] text-gray-500 dark:text-gray-400">
                    <span>覆盖文本（preset ≠ custom 时会自动填充，可手动覆写）</span>
                    <button
                      type="button"
                      class="text-rose-600 dark:text-rose-400 hover:underline"
                      @click="restoreJailbreakDefault"
                    >
                      恢复默认
                    </button>
                  </div>
                  <textarea
                    v-model="local.mitm_jailbreak_override"
                    rows="10"
                    placeholder="留空使用内置默认破限文本…"
                    class="w-full px-3 py-2 rounded-lg text-[12.5px] font-mono leading-relaxed bg-white dark:bg-gray-900 border border-black/[0.08] dark:border-white/[0.08] focus:outline-none focus:ring-2 focus:ring-rose-500/40"
                  ></textarea>
                </div>

                <!-- 运行时状态面板 -->
                <div
                  class="rounded-xl border border-black/[0.06] dark:border-white/[0.08] bg-white/60 dark:bg-white/[0.03] p-3 flex flex-col gap-2"
                >
                  <div class="flex items-center justify-between">
                    <span class="text-[12px] font-bold text-gray-700 dark:text-gray-300">
                      运行时状态
                    </span>
                    <div class="flex gap-2">
                      <button
                        type="button"
                        class="text-[11px] font-bold text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
                        :disabled="jailbreakRuntimeLoading"
                        @click="fetchJailbreakRuntime"
                      >
                        {{ jailbreakRuntimeLoading ? "刷新中…" : "刷新" }}
                      </button>
                      <button
                        type="button"
                        class="text-[11px] font-bold text-gray-400 hover:text-rose-600 dark:hover:text-rose-400"
                        @click="handleResetJailbreakStats"
                      >
                        清零计数
                      </button>
                    </div>
                  </div>
                  <div
                    v-if="jailbreakRuntime"
                    class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-[11.5px]"
                  >
                    <div class="rounded bg-black/[0.04] dark:bg-white/[0.05] px-2 py-1.5">
                      <div class="text-gray-500 dark:text-gray-400">来源</div>
                      <div class="font-bold text-gray-800 dark:text-gray-100 truncate">
                        {{ jailbreakRuntime.source }}
                      </div>
                    </div>
                    <div class="rounded bg-black/[0.04] dark:bg-white/[0.05] px-2 py-1.5">
                      <div class="text-gray-500 dark:text-gray-400">字符数</div>
                      <div class="font-bold text-gray-800 dark:text-gray-100">
                        {{ jailbreakRuntime.active_length }}
                      </div>
                    </div>
                    <div class="rounded bg-black/[0.04] dark:bg-white/[0.05] px-2 py-1.5">
                      <div class="text-gray-500 dark:text-gray-400">今日注入</div>
                      <div class="font-bold text-gray-800 dark:text-gray-100">
                        {{ jailbreakRuntime.stats.today_injects }} / 总 {{ jailbreakRuntime.stats.total_injects }}
                      </div>
                    </div>
                    <div class="rounded bg-black/[0.04] dark:bg-white/[0.05] px-2 py-1.5">
                      <div class="text-gray-500 dark:text-gray-400">上次注入</div>
                      <div class="font-bold text-gray-800 dark:text-gray-100">
                        {{ formatRelativeTime(jailbreakRuntime.stats.since_last_inject_ms) }}
                      </div>
                    </div>
                  </div>

                  <!-- 文件路径展示 -->
                  <div
                    v-if="jailbreakRuntime && jailbreakRuntime.file_path"
                    class="text-[11px] text-gray-500 dark:text-gray-400 font-mono truncate"
                    :title="jailbreakRuntime.file_path"
                  >
                    📁 {{ jailbreakRuntime.file_path }}
                    <span v-if="jailbreakRuntime.file_status && !jailbreakRuntime.file_status.exists" class="text-rose-600">
                      （文件不存在）
                    </span>
                  </div>

                  <!-- cyber 雷词警告 -->
                  <div
                    v-if="jailbreakRuntime && jailbreakRuntime.warn_anthropic"
                    class="rounded-lg border border-rose-500/30 bg-rose-500/[0.08] px-3 py-2 text-[11.5px] text-rose-700 dark:text-rose-300"
                  >
                    ⚠ 当前 override 文本含 cyber/malware/exploit 等关键词，<b>必定触发 Anthropic 网关 cyber-verification policy 拒绝</b>。建议切到 "极简" 或 "软版" preset。
                  </div>
                </div>

                <div class="text-[11px] text-amber-600 dark:text-amber-400">
                  ⚠ 仅供本地实验/学术研究使用。注入文本不会上传第三方服务端，仅在本机 MITM 拦截阶段附加到请求体。
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition:
    opacity 0.28s cubic-bezier(0.2, 0.8, 0.2, 1),
    transform 0.28s cubic-bezier(0.2, 0.8, 0.2, 1);
}
.fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.fade-leave-to {
  opacity: 0;
  transform: translateY(-3px);
}
</style>
