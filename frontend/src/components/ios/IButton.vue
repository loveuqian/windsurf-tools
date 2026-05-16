<script setup lang="ts">
/**
 * IButton — iOS 风格按钮，重点是 disabled 时通过 :reason 给 tooltip。
 *
 * 用户痛点：「立即换 IP」/「切到下一席」等按钮 disabled 时无原因，用户不知道
 * 为什么点不动。reason prop 自动作为 :title 提示。
 */
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    tone?: "primary" | "secondary" | "ghost" | "danger";
    size?: "sm" | "md" | "lg";
    /** 禁用时的解释（自动作为 title hover tooltip 显示） */
    reason?: string;
    disabled?: boolean;
    loading?: boolean;
    /** 整宽 */
    block?: boolean;
    type?: "button" | "submit" | "reset";
  }>(),
  { tone: "primary", size: "md", type: "button" },
);

const toneClass = {
  primary:
    "bg-ios-blue text-white hover:bg-ios-blue/90 active:bg-ios-blue/80 disabled:bg-ios-blue/40",
  secondary:
    "bg-black/[0.06] text-gray-800 dark:bg-white/[0.08] dark:text-gray-200 hover:bg-black/[0.1] dark:hover:bg-white/[0.12]",
  ghost:
    "text-gray-700 dark:text-gray-300 hover:bg-black/[0.04] dark:hover:bg-white/[0.06]",
  danger:
    "bg-rose-500 text-white hover:bg-rose-500/90 active:bg-rose-500/80 disabled:bg-rose-500/40",
} as const;

const sizeClass = {
  sm: "px-2.5 py-1 text-[12px] gap-1.5",
  md: "px-3 py-1.5 text-[13px] gap-2",
  lg: "px-4 py-2 text-[14px] gap-2",
} as const;

const titleAttr = computed(() =>
  props.disabled && props.reason ? props.reason : undefined,
);
</script>

<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :title="titleAttr"
    :class="[
      'inline-flex items-center justify-center rounded-ios-pill font-semibold transition-colors',
      'disabled:cursor-not-allowed disabled:opacity-60',
      block ? 'w-full' : '',
      toneClass[tone],
      sizeClass[size],
    ]"
  >
    <slot name="leading" />
    <span v-if="loading" aria-hidden="true" class="inline-block animate-spin">⟳</span>
    <slot />
    <slot name="trailing" />
  </button>
</template>
