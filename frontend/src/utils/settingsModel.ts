import { models } from '../../wailsjs/go/models'

/** 与 backend/utils/plan_tone.go PlanTone 顺序一致，用于排序与全选判定 */
export const SWITCH_PLAN_FILTER_TONES = [
  'pro',
  'max',
  'team',
  'enterprise',
  'trial',
  'free',
  'unknown',
] as const

export type SwitchPlanTone = (typeof SWITCH_PLAN_FILTER_TONES)[number]

/** 多选勾选列表（不含「全部」） */
export const switchPlanFilterToneOptions: Array<{ value: SwitchPlanTone; label: string }> = [
  { value: 'pro', label: 'Pro' },
  { value: 'max', label: 'Max / Ultimate' },
  { value: 'team', label: 'Teams' },
  { value: 'enterprise', label: 'Enterprise' },
  { value: 'trial', label: 'Trial' },
  { value: 'free', label: 'Free' },
  { value: 'unknown', label: '未识别' },
]

const TONE_ORDER = new Map(SWITCH_PLAN_FILTER_TONES.map((t, i) => [t, i]))

/** 下拉/兼容：含「全部」与单选旧值 */
export const switchPlanFilterOptions: Array<{ value: string; label: string }> = [
  { value: 'all', label: '全部计划（不限制）' },
  ...switchPlanFilterToneOptions.map((o) => ({ value: o.value, label: `仅 ${o.label}` })),
]

/** 与 backend/models/settings.go + wailsjs models.Settings 对齐 */
export function createDefaultSettings(): models.Settings {
  return new models.Settings({
    concurrent_limit: 5,
    auto_refresh_tokens: false,
    auto_refresh_quotas: false,
    quota_refresh_policy: 'hybrid',
    quota_custom_interval_minutes: 360,
    auto_switch_plan_filter: 'all',
    auto_switch_on_quota_exhausted: true,
    manual_pin_enabled: false,
    manual_pin_account_id: '',
    rotation_pool_enabled: false,
    rotation_pool_account_ids: [],
    rotation_pool_interval_min: 5,
    rotation_pool_quota_refresh_min: 1,
    quota_hot_poll_seconds: 12,
    minimize_to_tray: false,
    desktop_notifications: true,
    silent_start: false,
    openai_relay_enabled: false,
    openai_relay_port: 8787,
    openai_relay_secret: '',
    debug_log: false,
    import_concurrency: 3,
    forge_enabled: false,
    smart_friend_enabled: false, // F7-REMOVAL: 字段与下方 3 处同名赋值一并删除
    static_cache_intercept: true,
    mitm_jailbreak_enabled: false,
    mitm_jailbreak_override: '',
    mitm_jailbreak_preset_id: 'custom',
    mitm_jailbreak_override_source: 'inline',
    mitm_jailbreak_override_file: '',
    mitm_full_capture: false,
    mitm_debug_dump: false,
    clash_rotate_enabled: false,
    clash_controller_url: 'http://127.0.0.1:9097',
    clash_secret: '',
    clash_group: '',
    clash_nodes: '',
    clash_interval_minutes: 8,
    clash_rotate_on_rate_limit: true,
    clash_latency_test_url: 'http://www.gstatic.com/generate_204',
    clash_latency_max_ms: 800,
  })
}

export function normalizeSettings(raw: unknown): models.Settings {
  const base = createDefaultSettings()
  if (!raw || typeof raw !== 'object') {
    return base
  }
  const s = raw as Record<string, unknown>
  return new models.Settings({
    concurrent_limit: Math.max(1, Number(s.concurrent_limit) || 5),
    auto_refresh_tokens: Boolean(s.auto_refresh_tokens),
    auto_refresh_quotas: Boolean(s.auto_refresh_quotas),
    quota_refresh_policy: String(s.quota_refresh_policy || 'hybrid'),
    quota_custom_interval_minutes: clampQuotaMinutes(Number(s.quota_custom_interval_minutes)),
    auto_switch_plan_filter: normalizeSwitchPlanFilter(String(s.auto_switch_plan_filter ?? 'all')),
    auto_switch_on_quota_exhausted:
      'auto_switch_on_quota_exhausted' in s ? Boolean(s.auto_switch_on_quota_exhausted) : true,
    manual_pin_enabled: 'manual_pin_enabled' in s ? Boolean(s.manual_pin_enabled) : false,
    manual_pin_account_id: String(s.manual_pin_account_id ?? ''),
    rotation_pool_enabled:
      'rotation_pool_enabled' in s ? Boolean(s.rotation_pool_enabled) : false,
    rotation_pool_account_ids: Array.isArray(s.rotation_pool_account_ids)
      ? (s.rotation_pool_account_ids as string[]).filter((x) => typeof x === 'string' && x.trim() !== '')
      : [],
    rotation_pool_interval_min: clampRotationPoolInterval(
      Number(s.rotation_pool_interval_min ?? 5),
    ),
    rotation_pool_quota_refresh_min: clampRotationPoolQuotaRefresh(
      Number(s.rotation_pool_quota_refresh_min ?? 1),
    ),
    quota_hot_poll_seconds: clampHotPollSeconds(
      'quota_hot_poll_seconds' in s ? Number(s.quota_hot_poll_seconds) : 12,
    ),
    minimize_to_tray: Boolean(s.minimize_to_tray),
    desktop_notifications:
      'desktop_notifications' in s ? Boolean(s.desktop_notifications) : true,
    silent_start: 'silent_start' in s ? Boolean(s.silent_start) : base.silent_start,
    openai_relay_enabled: 'openai_relay_enabled' in s ? Boolean(s.openai_relay_enabled) : base.openai_relay_enabled,
    openai_relay_port: Math.max(1, Math.min(65535, Number(s.openai_relay_port) || 8787)),
    openai_relay_secret: String(s.openai_relay_secret ?? ''),
    debug_log: 'debug_log' in s ? Boolean(s.debug_log) : false,
    import_concurrency: Math.max(1, Math.min(20, Number(s.import_concurrency) || 3)),
    forge_enabled: 'forge_enabled' in s ? Boolean(s.forge_enabled) : false,
    // F7-REMOVAL: 下一行删除
    smart_friend_enabled: 'smart_friend_enabled' in s ? Boolean(s.smart_friend_enabled) : false,
    static_cache_intercept: 'static_cache_intercept' in s ? Boolean(s.static_cache_intercept) : true,
    mitm_jailbreak_enabled: 'mitm_jailbreak_enabled' in s ? Boolean(s.mitm_jailbreak_enabled) : false,
    mitm_jailbreak_override: String(s.mitm_jailbreak_override ?? ''),
    mitm_full_capture: 'mitm_full_capture' in s ? Boolean(s.mitm_full_capture) : false,
    mitm_debug_dump: 'mitm_debug_dump' in s ? Boolean(s.mitm_debug_dump) : false,
    clash_rotate_enabled: 'clash_rotate_enabled' in s ? Boolean(s.clash_rotate_enabled) : false,
    clash_controller_url: 'clash_controller_url' in s ? String(s.clash_controller_url || '') : 'http://127.0.0.1:9097',
    clash_secret: String(s.clash_secret ?? ''),
    clash_group: String(s.clash_group ?? ''),
    clash_nodes: String(s.clash_nodes ?? ''),
    clash_interval_minutes: clampClashInterval(Number(s.clash_interval_minutes ?? 8)),
    clash_rotate_on_rate_limit:
      'clash_rotate_on_rate_limit' in s ? Boolean(s.clash_rotate_on_rate_limit) : true,
    clash_latency_test_url: String(s.clash_latency_test_url ?? 'http://www.gstatic.com/generate_204'),
    clash_latency_max_ms: Math.max(0, Math.min(10000, Number(s.clash_latency_max_ms ?? 800))),
  })
}

/** 规范化存储：all；或逗号分隔的合法 tone（去重、按固定顺序排序）。支持旧版单值 pro / trial 等。 */
export function normalizeSwitchPlanFilter(v: string | undefined | null): string {
  if (v == null || v === '' || v === 'undefined') {
    return 'all'
  }
  let s = String(v).trim().toLowerCase().replace(/，/g, ',')
  if (s === 'all') {
    return 'all'
  }
  const allowed = new Set<string>(SWITCH_PLAN_FILTER_TONES as unknown as string[])
  const parts = [
    ...new Set(
      s
        .split(',')
        .map((x) => x.trim())
        .filter(Boolean)
        .filter((x) => allowed.has(x)),
    ),
  ]
  if (parts.length === 0) {
    return 'all'
  }
  if (parts.length >= SWITCH_PLAN_FILTER_TONES.length) {
    return 'all'
  }
  parts.sort((a, b) => (TONE_ORDER.get(a as SwitchPlanTone) ?? 0) - (TONE_ORDER.get(b as SwitchPlanTone) ?? 0))
  return parts.join(',')
}

/** 用于界面展示当前范围文案 */
export function formatSwitchPlanFilterSummary(filter: string | undefined | null): string {
  const n = normalizeSwitchPlanFilter(filter ?? 'all')
  if (n === 'all') {
    return '全部计划（不限制）'
  }
  const labelByValue = Object.fromEntries(switchPlanFilterToneOptions.map((o) => [o.value, o.label]))
  return n
    .split(',')
    .map((t) => labelByValue[t] || t)
    .join('、')
}

export function clampQuotaMinutes(m: number): number {
  if (!Number.isFinite(m) || m <= 0) {
    return 360
  }
  return Math.min(10080, Math.max(5, Math.round(m)))
}

/** 当前活跃席位额度快查间隔（秒），与后端 clampQuotaHotPollSeconds 一致 */
export function clampHotPollSeconds(sec: number): number {
  if (!Number.isFinite(sec) || sec <= 0) {
    return 12
  }
  return Math.min(60, Math.max(5, Math.round(sec)))
}

/** Clash 轮换间隔（分钟），与后端 ClashRotator 限制一致 [2,60] */
export function clampClashInterval(min: number): number {
  if (!Number.isFinite(min) || min <= 0) {
    return 8
  }
  return Math.min(60, Math.max(2, Math.round(min)))
}

/** RotationPool 定时切间隔（分钟），与后端钳制一致 [1,60] */
export function clampRotationPoolInterval(min: number): number {
  if (!Number.isFinite(min) || min <= 0) {
    return 5
  }
  return Math.min(60, Math.max(1, Math.round(min)))
}

/** RotationPool 额度刷新间隔（分钟），与后端钳制一致 [1,10] */
export function clampRotationPoolQuotaRefresh(min: number): number {
  if (!Number.isFinite(min) || min <= 0) {
    return 1
  }
  return Math.min(10, Math.max(1, Math.round(min)))
}

/** 与后端 JSON 字段一致，便于 reactive + v-model */
export type SettingsForm = {
  concurrent_limit: number
  auto_refresh_tokens: boolean
  auto_refresh_quotas: boolean
  quota_refresh_policy: string
  quota_custom_interval_minutes: number
  /** 无感下一席位：all 或逗号分隔多选，如 trial,pro */
  auto_switch_plan_filter: string
  /** 额度用尽时自动切下一席（需开启定期同步额度） */
  auto_switch_on_quota_exhausted: boolean
  /** 手动切号后自动锁定，所有 auto-switch 通道暂停 */
  manual_pin_enabled: boolean
  /** 锁定到的账号 ID（UUID） */
  manual_pin_account_id: string
  /** 轮换池总开关 */
  rotation_pool_enabled: boolean
  /** 池内账号 ID 列表（UUID） */
  rotation_pool_account_ids: string[]
  /** 定时切间隔（分钟），[1,60]，默认 5 */
  rotation_pool_interval_min: number
  /** 池内账号额度刷新间隔（分钟），[1,10]，默认 1 */
  rotation_pool_quota_refresh_min: number
  /** 当前活跃席位快查间隔（秒），用尽轮换依赖此轮询 */
  quota_hot_poll_seconds: number
  /** 关闭窗口时最小化到系统托盘 */
  minimize_to_tray: boolean
  /** 关键事件弹桌面通知 (Pin 解除 / 额度耗尽 / Clash 错误) */
  desktop_notifications: boolean
  /** 启动时不显示主窗口（托盘仍可打开） */
  silent_start: boolean
  /** OpenAI 兼容中转服务器 */
  openai_relay_enabled: boolean
  openai_relay_port: number
  openai_relay_secret: string
  /** 调试日志：开启后将切号/代理/额度判定写入 debug.log */
  debug_log: boolean
  /** 导入并发数 1～20 */
  import_concurrency: number
  /** GetUserStatus/GetPlanStatus 伪造为 Enterprise + 无限积分 */
  forge_enabled: boolean
  /** F7-REMOVAL: 下两行删除。SmartFriend 仅作者自用，发布前不保留字段。 */
  smart_friend_enabled: boolean
  /** 静态响应缓存拦截 (.bin 文件直返) */
  static_cache_intercept: boolean
  /** 破限注入：MITM 在 chat F2 system prompt 末尾追加 override 文本 */
  mitm_jailbreak_enabled: boolean
  /** 破限注入文本（空字符串 = 后端 fallback 到 DefaultJailbreakOverride） */
  mitm_jailbreak_override: string
  /** 预设 ID: custom / minimal / soft_safe / original_full */
  mitm_jailbreak_preset_id: string
  /** 文本来源: inline / file */
  mitm_jailbreak_override_source: string
  /** 当 source=file 时的文件路径（空 → 默认 ~/.claude/override.md） */
  mitm_jailbreak_override_file: string
  /** MITM 全量抓包落盘 */
  mitm_full_capture: boolean
  /** MITM protobuf dump 诊断 */
  mitm_debug_dump: boolean
  /** Clash IP 轮换 */
  clash_rotate_enabled: boolean
  clash_controller_url: string
  clash_secret: string
  clash_group: string
  clash_nodes: string
  clash_interval_minutes: number
  clash_rotate_on_rate_limit: boolean
  clash_latency_test_url: string
  clash_latency_max_ms: number
}

export function settingsToForm(s: models.Settings): SettingsForm {
  return {
    concurrent_limit: s.concurrent_limit || 5,
    auto_refresh_tokens: s.auto_refresh_tokens,
    auto_refresh_quotas: s.auto_refresh_quotas,
    quota_refresh_policy: s.quota_refresh_policy || 'hybrid',
    quota_custom_interval_minutes: clampQuotaMinutes(s.quota_custom_interval_minutes),
    auto_switch_plan_filter: normalizeSwitchPlanFilter(s.auto_switch_plan_filter),
    auto_switch_on_quota_exhausted: s.auto_switch_on_quota_exhausted !== false,
    manual_pin_enabled: (s as any).manual_pin_enabled === true,
    manual_pin_account_id: String((s as any).manual_pin_account_id ?? ''),
    rotation_pool_enabled: (s as any).rotation_pool_enabled === true,
    rotation_pool_account_ids: Array.isArray((s as any).rotation_pool_account_ids)
      ? (s as any).rotation_pool_account_ids.filter((x: unknown) => typeof x === 'string' && (x as string).trim() !== '')
      : [],
    rotation_pool_interval_min: clampRotationPoolInterval(Number((s as any).rotation_pool_interval_min ?? 5)),
    rotation_pool_quota_refresh_min: clampRotationPoolQuotaRefresh(Number((s as any).rotation_pool_quota_refresh_min ?? 1)),
    quota_hot_poll_seconds: clampHotPollSeconds(s.quota_hot_poll_seconds ?? 12),
    minimize_to_tray: s.minimize_to_tray === true,
    desktop_notifications: (s as any).desktop_notifications !== false,
    silent_start: s.silent_start === true,
    openai_relay_enabled: s.openai_relay_enabled === true,
    openai_relay_port: Math.max(1, Number(s.openai_relay_port) || 8787),
    openai_relay_secret: String(s.openai_relay_secret ?? ''),
    debug_log: s.debug_log === true,
    import_concurrency: Math.max(1, Math.min(20, Number(s.import_concurrency) || 3)),
    forge_enabled: s.forge_enabled === true,
    // F7-REMOVAL: 下一行删除
    smart_friend_enabled: s.smart_friend_enabled === true,
    static_cache_intercept: s.static_cache_intercept !== false,
    mitm_jailbreak_enabled: s.mitm_jailbreak_enabled === true,
    mitm_jailbreak_override: String(s.mitm_jailbreak_override ?? ''),
    mitm_jailbreak_preset_id: String((s as any).mitm_jailbreak_preset_id ?? 'custom'),
    mitm_jailbreak_override_source: String((s as any).mitm_jailbreak_override_source ?? 'inline'),
    mitm_jailbreak_override_file: String((s as any).mitm_jailbreak_override_file ?? ''),
    mitm_full_capture: s.mitm_full_capture === true,
    mitm_debug_dump: s.mitm_debug_dump === true,
    clash_rotate_enabled: s.clash_rotate_enabled === true,
    clash_controller_url: String(s.clash_controller_url ?? 'http://127.0.0.1:9097'),
    clash_secret: String(s.clash_secret ?? ''),
    clash_group: String(s.clash_group ?? ''),
    clash_nodes: String(s.clash_nodes ?? ''),
    clash_interval_minutes: clampClashInterval(Number(s.clash_interval_minutes ?? 8)),
    clash_rotate_on_rate_limit: s.clash_rotate_on_rate_limit !== false,
    clash_latency_test_url: String(
      s.clash_latency_test_url ?? 'http://www.gstatic.com/generate_204',
    ),
    clash_latency_max_ms: Math.max(0, Math.min(10000, Number(s.clash_latency_max_ms ?? 800))),
  }
}

export function formToSettings(form: SettingsForm): models.Settings {
  return new models.Settings({
    concurrent_limit: Math.max(1, Math.round(form.concurrent_limit) || 5),
    auto_refresh_tokens: form.auto_refresh_tokens,
    auto_refresh_quotas: form.auto_refresh_quotas,
    quota_refresh_policy: form.quota_refresh_policy || 'hybrid',
    quota_custom_interval_minutes: clampQuotaMinutes(form.quota_custom_interval_minutes),
    auto_switch_plan_filter: normalizeSwitchPlanFilter(form.auto_switch_plan_filter),
    auto_switch_on_quota_exhausted: form.auto_switch_on_quota_exhausted,
    manual_pin_enabled: form.manual_pin_enabled,
    manual_pin_account_id: (form.manual_pin_account_id ?? '').trim(),
    rotation_pool_enabled: form.rotation_pool_enabled,
    rotation_pool_account_ids: Array.isArray(form.rotation_pool_account_ids)
      ? form.rotation_pool_account_ids.filter((x) => typeof x === 'string' && x.trim() !== '')
      : [],
    rotation_pool_interval_min: clampRotationPoolInterval(form.rotation_pool_interval_min),
    rotation_pool_quota_refresh_min: clampRotationPoolQuotaRefresh(form.rotation_pool_quota_refresh_min),
    quota_hot_poll_seconds: clampHotPollSeconds(form.quota_hot_poll_seconds),
    minimize_to_tray: form.minimize_to_tray,
    desktop_notifications: form.desktop_notifications,
    silent_start: form.silent_start,
    openai_relay_enabled: form.openai_relay_enabled,
    openai_relay_port: Math.max(1, Math.min(65535, Math.round(form.openai_relay_port) || 8787)),
    openai_relay_secret: (form.openai_relay_secret ?? '').trim(),
    debug_log: form.debug_log,
    import_concurrency: Math.max(1, Math.min(20, Math.round(form.import_concurrency) || 3)),
    forge_enabled: form.forge_enabled,
    // F7-REMOVAL: 下一行删除
    smart_friend_enabled: form.smart_friend_enabled,
    static_cache_intercept: form.static_cache_intercept,
    mitm_jailbreak_enabled: form.mitm_jailbreak_enabled,
    mitm_jailbreak_override: (form.mitm_jailbreak_override ?? '').trim(),
    mitm_jailbreak_preset_id: (form.mitm_jailbreak_preset_id ?? 'custom').trim() || 'custom',
    mitm_jailbreak_override_source: (form.mitm_jailbreak_override_source ?? 'inline').trim() || 'inline',
    mitm_jailbreak_override_file: (form.mitm_jailbreak_override_file ?? '').trim(),
    mitm_full_capture: form.mitm_full_capture,
    mitm_debug_dump: form.mitm_debug_dump,
    clash_rotate_enabled: form.clash_rotate_enabled,
    clash_controller_url: (form.clash_controller_url ?? '').trim(),
    clash_secret: (form.clash_secret ?? '').trim(),
    clash_group: (form.clash_group ?? '').trim(),
    clash_nodes: (form.clash_nodes ?? '').trim(),
    clash_interval_minutes: clampClashInterval(form.clash_interval_minutes),
    clash_rotate_on_rate_limit: form.clash_rotate_on_rate_limit,
    clash_latency_test_url: (form.clash_latency_test_url ?? '').trim() || 'http://www.gstatic.com/generate_204',
    clash_latency_max_ms: Math.max(0, Math.min(10000, Math.round(form.clash_latency_max_ms) || 0)),
  })
}

export const quotaPolicyOptions: Array<{ value: string; label: string }> = [
  { value: 'hybrid', label: '美东换日或满 24h（推荐）' },
  { value: 'interval_24h', label: '固定每 24 小时' },
  { value: 'us_calendar', label: '仅美东日历跨日' },
  { value: 'local_calendar', label: '本机时区跨日' },
  { value: 'interval_1h', label: '每 1 小时' },
  { value: 'interval_6h', label: '每 6 小时' },
  { value: 'interval_12h', label: '每 12 小时' },
  { value: 'custom', label: '自定义间隔（分钟）' },
]
