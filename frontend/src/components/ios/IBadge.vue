<script setup lang="ts">
/**
 * IBadge — iOS 风格 chip / pill 标签。
 *
 * 5 种 tone 对应 success/info/warn/danger/neutral，颜色与 ios.green/blue/红/黄 对齐。
 * 替代 Header / Dashboard / Accounts 中所有 inline emerald/amber/rose chip。
 *
 * 可选 `pill` 让外形变成完整圆 chip（默认 rounded-ios-block 带轻角）。
 */
defineProps<{
  tone?: "success" | "info" | "warn" | "danger" | "neutral";
  /** 完整圆角 chip 形（默认 rounded-ios-block） */
  pill?: boolean;
  /** 加粗文字（默认 semibold） */
  strong?: boolean;
}>();

const toneClass = {
  success:
    "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 border-emerald-500/20",
  info:
    "bg-ios-blue/12 text-ios-blue dark:text-ios-blueDark border-ios-blue/20",
  warn:
    "bg-amber-500/15 text-amber-700 dark:text-amber-300 border-amber-500/20",
  danger:
    "bg-rose-500/15 text-rose-700 dark:text-rose-300 border-rose-500/20",
  neutral:
    "bg-black/[0.05] text-gray-700 dark:bg-white/[0.06] dark:text-gray-300 border-black/[0.06] dark:border-white/[0.08]",
} as const;
</script>

<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 px-2.5 py-1 text-[11px] border',
      pill ? 'rounded-ios-pill' : 'rounded-ios-block',
      strong !== false ? 'font-bold' : 'font-semibold',
      toneClass[tone ?? 'neutral'],
    ]"
  >
    <slot />
  </span>
</template>
