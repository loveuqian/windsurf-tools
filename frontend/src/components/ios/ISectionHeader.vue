<script setup lang="ts">
/**
 * ISectionHeader — 面板顶部统一样式：[图标盒][标题/副标题][右侧操作槽]。
 *
 * 替代各 view 自写的 header div 块，统一字号 / 间距 / 图标盒尺寸。
 */
import type { Component } from "vue";
defineProps<{
  /** lucide-vue-next 图标组件 */
  icon?: Component;
  title: string;
  subtitle?: string;
  /** 图标盒色调（默认 ios-blue） */
  iconTone?: "blue" | "green" | "amber" | "violet" | "rose";
}>();

const iconBoxToneClass = {
  blue: "bg-ios-blue/12 text-ios-blue",
  green: "bg-emerald-500/12 text-emerald-600 dark:text-emerald-400",
  amber: "bg-amber-500/12 text-amber-600 dark:text-amber-400",
  violet: "bg-violet-500/12 text-violet-600 dark:text-violet-400",
  rose: "bg-rose-500/12 text-rose-600 dark:text-rose-400",
} as const;
</script>

<template>
  <header class="flex items-center justify-between gap-4 px-6 pt-5 pb-4">
    <div class="flex items-center gap-3 min-w-0">
      <div
        v-if="icon"
        :class="[
          'flex h-9 w-9 shrink-0 items-center justify-center rounded-ios-tile',
          iconBoxToneClass[iconTone ?? 'blue'],
        ]"
      >
        <component :is="icon" class="h-5 w-5" stroke-width="2.4" />
      </div>
      <div class="min-w-0">
        <h2
          class="text-[16px] font-bold text-gray-900 dark:text-gray-100 leading-tight truncate"
        >
          {{ title }}
        </h2>
        <p
          v-if="subtitle"
          class="text-[12px] text-gray-500 dark:text-gray-400 leading-snug truncate"
        >
          {{ subtitle }}
        </p>
      </div>
    </div>
    <div class="flex items-center gap-2 shrink-0">
      <slot name="actions" />
    </div>
  </header>
</template>
