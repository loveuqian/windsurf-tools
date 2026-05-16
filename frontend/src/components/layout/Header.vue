<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { Copy, Lock, Moon, Monitor, RadioTower, ShieldCheck, Sun } from 'lucide-vue-next'
import { useMitmStatusStore } from '../../stores/useMitmStatusStore'
import { useSettingsStore } from '../../stores/useSettingsStore'
import { useAccountStore } from '../../stores/useAccountStore'
import { APP_VERSION } from '../../utils/appMeta'
import { APP_PRODUCT_NAME, APP_PRODUCT_TAGLINE } from '../../utils/appMode'
import { cycleTheme, themeLabel, themeMode } from '../../utils/theme'
import { APIInfo } from '../../api/wails'
import { showToast, showErrorToast  } from '../../utils/toast'

const mitmStore = useMitmStatusStore()
const settingsStore = useSettingsStore()
const accountStore = useAccountStore()

onMounted(() => {
  // 让 Pin 徽章可见性跟随 settings 变化
  void settingsStore.fetchSettings()
  void accountStore.ensureAccountsLoaded()
})

const modeLabel = computed(() => 'Pure MITM')
const activeKey = computed(() => mitmStore.status?.pool_status?.find((item) => item.is_current) ?? null)

const isPinned = computed(() => settingsStore.settings?.manual_pin_enabled === true)
const pinnedAccount = computed(() => {
  const id = settingsStore.settings?.manual_pin_account_id
  if (!id) return null
  return accountStore.accounts.find((a) => a.id === id) ?? null
})
const pinnedLabel = computed(() => {
  if (!isPinned.value) return ''
  const acc = pinnedAccount.value
  if (acc?.email) return acc.email
  return settingsStore.settings?.manual_pin_account_id?.slice(0, 8) || '账号'
})

// 找当前活跃 pool key → 在 accountStore 里通过 email 查 windsurf_api_key
// 后端 GetMitmProxyStatus 已用 KeyHash 严格填好 PoolKeyInfo.Email
const activeApiKey = computed(() => {
  const k = activeKey.value
  if (!k?.email) return ''
  const acc = accountStore.accounts.find((a) => a.email === k.email)
  return (acc?.windsurf_api_key || '').trim()
})

const handleCopyActiveKey = async () => {
  if (!activeApiKey.value) {
    showToast('当前活跃账号未配置 API Key', 'warning')
    return
  }
  try {
    await navigator.clipboard.writeText(activeApiKey.value)
    const k = activeApiKey.value
    const short = k.length > 16 ? k.slice(0, 12) + '…' + k.slice(-4) : k
    showToast(`已复制 ${short}`, 'success')
  } catch (e) {
    showErrorToast(e, "复制失败")
  }
}

const handleUnpinFromHeader = async () => {
  try {
    await APIInfo.unpinManualAccount()
    await settingsStore.fetchSettings(true)
    showToast('已解锁，自动切换已恢复', 'success')
  } catch (e) {
    showErrorToast(e, "解锁失败")
  }
}
const poolCount = computed(() => mitmStore.status?.pool_status?.length ?? 0)
const healthyCount = computed(() => mitmStore.status?.pool_status?.filter((item) => item.healthy).length ?? 0)

const onlineEmail = computed(() => {
  const key = String(activeKey.value?.key_short || '').trim()
  if (!key) {
    return mitmStore.status?.running ? '等待活跃 Key' : 'MITM 未启动'
  }
  return key.length > 28 ? `${key.slice(0, 26)}…` : key
})

const onlineEmailFull = computed(() => String(activeKey.value?.key_short || '').trim())

const onlineSummary = computed(() => {
  if (!mitmStore.status?.running) {
    return '启动后将从 MITM 号池轮换'
  }
  if (!onlineEmailFull.value) {
    return `健康 ${healthyCount.value} / ${poolCount.value}`
  }
  return `健康 ${healthyCount.value} / ${poolCount.value}`
})

const sessionStateLabel = computed(() =>
  onlineEmailFull.value ? '当前活跃 Key' : 'MITM 状态',
)

const sessionStateTone = computed(() =>
  mitmStore.status?.running
    ? 'border-emerald-500/18 bg-emerald-500/[0.08] text-emerald-700 dark:text-emerald-300'
    : 'border-black/[0.06] bg-black/[0.03] text-ios-textSecondary dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textSecondaryDark',
)
</script>

<template>
  <header
    class="drag-region grid h-[64px] w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-4 px-4 md:px-5 bg-white/82 dark:bg-[#1C1C1E]/88 backdrop-blur-2xl border-b border-ios-divider dark:border-ios-dividerDark select-none z-50 shrink-0"
  >
    <div class="flex min-w-0 items-center gap-3">
      <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-gradient-to-br from-ios-blue to-sky-400 text-white shadow-[0_10px_22px_rgba(37,99,235,0.24)]">
        <ShieldCheck class="h-[18px] w-[18px]" stroke-width="2.6" />
      </div>
      <div class="min-w-0">
        <div class="flex min-w-0 items-center gap-2">
          <span class="truncate text-[16px] font-semibold tracking-tight text-ios-text dark:text-ios-textDark">
            {{ APP_PRODUCT_NAME }}
          </span>
          <span class="hidden rounded-full bg-ios-blue/10 px-2.5 py-0.5 text-[10px] font-bold tracking-wide text-ios-blue md:inline-flex">
            {{ modeLabel }}
          </span>
        </div>
        <div class="mt-0.5 flex min-w-0 items-center gap-2 text-ios-textSecondary dark:text-ios-textSecondaryDark">
          <span class="text-[10px] font-medium tracking-wide tabular-nums">
            MITM Control · v{{ APP_VERSION }}
          </span>
          <span class="hidden h-1 w-1 rounded-full bg-black/20 dark:bg-white/20 md:block" />
          <span class="hidden truncate text-[11px] font-medium md:block">
            {{ APP_PRODUCT_TAGLINE }}
          </span>
        </div>
      </div>
    </div>

    <div class="no-drag-region flex min-w-0 items-center justify-end gap-2">
      <div
        class="hidden min-w-[240px] max-w-[360px] items-center gap-3 rounded-[18px] border px-3.5 py-2 shadow-[0_8px_22px_rgba(15,23,42,0.05)] lg:flex"
        :class="sessionStateTone"
      >
        <div
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl"
          :class="mitmStore.status?.running ? 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-300' : 'bg-black/[0.05] text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark'"
        >
          <RadioTower class="h-4 w-4" stroke-width="2.4" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="truncate text-[10px] font-bold uppercase tracking-[0.16em]">
              {{ sessionStateLabel }}
            </span>
          </div>
          <div class="mt-1 truncate text-[12px] font-semibold text-ios-text dark:text-ios-textDark" :title="onlineEmailFull || ''">
            {{ onlineEmail }}
          </div>
        </div>
        <span
          class="hidden shrink-0 rounded-full px-2 py-1 text-[10px] font-bold tracking-wide xl:inline-flex"
          :class="mitmStore.status?.running ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'bg-black/[0.05] text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark'"
          :title="onlineEmailFull || onlineSummary"
        >
          {{ onlineSummary }}
        </span>
      </div>

      <div
        class="flex min-w-0 max-w-[220px] items-center gap-2 rounded-full border border-black/[0.06] bg-black/[0.03] px-3 py-1.5 text-[11px] font-medium text-ios-textSecondary dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textSecondaryDark lg:hidden"
        :title="onlineEmailFull || ''"
      >
        <span
          class="h-2 w-2 shrink-0 rounded-full"
          :class="mitmStore.status?.running ? 'bg-emerald-500' : 'bg-slate-400 dark:bg-slate-500'"
        />
        <span class="truncate">{{ onlineEmail }}</span>
      </div>

      <!-- ★ v1.3.0 Pin 锁定徽章 + 解锁按钮 -->
      <div
        v-if="isPinned"
        class="no-drag-region hidden sm:flex items-center gap-1.5 rounded-full bg-amber-500/15 border border-amber-500/30 px-3 py-1.5 text-[11px] font-bold text-amber-700 dark:text-amber-300"
        :title="`已锁定: ${pinnedLabel} — 自动切换全部暂停`"
      >
        <Lock class="h-3 w-3" stroke-width="2.6" />
        <span class="truncate max-w-[120px]">{{ pinnedLabel }}</span>
        <button
          type="button"
          class="rounded-full bg-amber-500 px-2 py-0.5 text-[10px] font-black text-white hover:bg-amber-600 transition-colors"
          @click="handleUnpinFromHeader"
        >
          解锁
        </button>
      </div>

      <!-- ★ v1.3.0 复制当前活跃 API Key -->
      <button
        v-if="activeApiKey"
        type="button"
        class="no-drag-region hidden md:flex h-9 w-9 items-center justify-center rounded-full border border-black/[0.06] bg-white/70 text-ios-text shadow-sm transition-colors hover:bg-black/5 dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textDark dark:hover:bg-white/10"
        title="复制当前活跃账号的 sk-ws- API Key"
        @click="handleCopyActiveKey"
      >
        <Copy class="w-[16px] h-[16px]" stroke-width="2.4" />
      </button>

      <button
        type="button"
        class="flex h-9 w-9 items-center justify-center rounded-full border border-black/[0.06] bg-white/70 text-ios-text shadow-sm transition-colors hover:bg-black/5 dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textDark dark:hover:bg-white/10"
        :title="themeLabel(themeMode)"
        :aria-label="`主题：${themeLabel(themeMode)}，点击切换`"
        @click="cycleTheme()"
      >
        <Sun v-if="themeMode === 'light'" class="w-[18px] h-[18px]" stroke-width="2.5" />
        <Moon v-else-if="themeMode === 'dark'" class="w-[18px] h-[18px]" stroke-width="2.5" />
        <Monitor v-else class="w-[18px] h-[18px]" stroke-width="2.5" />
      </button>
    </div>
  </header>
</template>
