<script setup lang="ts">
// ISettingRow ── iOS Settings.app 风格的设置行
// 设计：
//   - 左侧：title (bold) + description (灰色小字)
//   - 右侧：slot 给 control (IToggle / INumberStepper / 输入框 / 按钮)
//   - 行间底分隔线（最后一行可隐藏 :last 用 group）
//   - hover 微背景变化（暗示行可交互）
//   - 自适应布局：宽屏左右排，窄屏上下叠

withDefaults(
  defineProps<{
    title?: string
    description?: string
    /** 用于标记 destructive 行（红色调） */
    destructive?: boolean
    /** 隐藏底分隔线（用在 section 最后一行） */
    noBorder?: boolean
    /** 强制堆叠布局（适合 control 很宽，比如 textarea） */
    stacked?: boolean
  }>(),
  {
    title: '',
    description: '',
    destructive: false,
    noBorder: false,
    stacked: false,
  },
)
</script>

<template>
  <div
    class="px-5 sm:px-6 py-4 transition-colors"
    :class="[
      !noBorder ? 'border-b border-black/[0.04] dark:border-white/[0.04]' : '',
      destructive ? 'bg-rose-500/[0.02]' : '',
    ]"
  >
    <div
      class="flex gap-4"
      :class="
        stacked
          ? 'flex-col items-stretch'
          : 'flex-col sm:flex-row sm:items-center sm:justify-between'
      "
    >
      <div v-if="title || description || $slots.label" class="min-w-0 flex-1">
        <slot name="label">
          <div
            v-if="title"
            class="text-[15px] font-bold leading-snug mb-0.5"
            :class="
              destructive
                ? 'text-rose-700 dark:text-rose-300'
                : 'text-gray-900 dark:text-gray-100'
            "
          >
            <slot name="title">{{ title }}</slot>
          </div>
          <div
            v-if="description"
            class="text-[12.5px] text-gray-500 dark:text-gray-400 leading-relaxed"
          >
            <slot name="description">{{ description }}</slot>
          </div>
        </slot>
      </div>
      <div
        class="shrink-0 flex items-center"
        :class="stacked ? 'w-full' : 'sm:justify-end'"
      >
        <slot />
      </div>
    </div>
    <!-- 二级附加内容（折叠区/警告条 等） -->
    <div v-if="$slots.extra" class="mt-3">
      <slot name="extra" />
    </div>
  </div>
</template>
