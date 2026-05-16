<script setup lang="ts">
/**
 * ISection — iOS 风格标准面板。
 *
 * 替代各 view 散落的「rounded-ios-card backdrop-blur-2xl border border-black/[0.05]
 * bg-white/72 dark:border-white/[0.08] dark:bg-[#1C1C1E]/82」长串 utility。
 *
 * 用法：
 *   <ISection>
 *     <ISectionHeader title="..." />
 *     <div class="p-6">...</div>
 *   </ISection>
 *
 * tone='inset' 是嵌套区块（无 backdrop-blur），用于 Section 内部再分块。
 */
defineProps<{
  /** 'card' = 顶层大面板（带 backdrop-blur）；'inset' = 嵌套小区块 */
  tone?: "card" | "inset";
  /** 是否给一个 ring 强调线（默认 card 带，inset 不带） */
  ring?: boolean;
}>();
</script>

<template>
  <section
    :class="[
      'flex flex-col',
      tone === 'inset'
        ? 'rounded-ios-block bg-white/60 dark:bg-white/[0.04] border border-black/[0.04] dark:border-white/[0.05]'
        : 'rounded-ios-card backdrop-blur-2xl border border-black/[0.05] bg-white/72 dark:border-white/[0.08] dark:bg-[#1C1C1E]/82',
      ring !== false && tone !== 'inset' ? 'shadow-ios-card' : '',
    ]"
  >
    <slot />
  </section>
</template>
