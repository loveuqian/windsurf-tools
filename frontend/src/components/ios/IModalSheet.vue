<script setup lang="ts">
/**
 * IModalSheet — iOS 风格底部 sheet / 居中 modal 双模式。
 *
 * 替代 Dashboard 诊断 modal、ImportModal 部分 layer、IConfirm 容器各自手写
 * 的「fixed inset-0 z-200 bg-black/40 backdrop-blur-md p-4 + sm:rounded-ios-card」
 * 长串样式。
 *
 * 行为：
 *   - 点击遮罩关闭（除非 dismissable=false）
 *   - ESC 关闭（focus 在文档上时；改善键盘可达性）
 *   - 默认手机端从底部弹出（rounded-t-ios-card），sm+ 居中（rounded-ios-card）
 */
import { onMounted, onBeforeUnmount } from "vue";

const props = withDefaults(
  defineProps<{
    /** 是否显示 */
    open: boolean;
    /** 是否允许遮罩 / ESC 关闭，默认 true */
    dismissable?: boolean;
    /** 最大宽度（sm+ 居中模式下生效），默认 600 */
    maxWidth?: number;
  }>(),
  { dismissable: true, maxWidth: 600 },
);

const emit = defineEmits<{ (e: "close"): void }>();

const handleEsc = (e: KeyboardEvent) => {
  if (!props.open || !props.dismissable) return;
  if (e.key === "Escape") {
    e.preventDefault();
    emit("close");
  }
};

onMounted(() => document.addEventListener("keydown", handleEsc));
onBeforeUnmount(() => document.removeEventListener("keydown", handleEsc));
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-[200] flex items-end sm:items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-md p-0 sm:p-4"
      role="dialog"
      aria-modal="true"
      @click.self="dismissable && $emit('close')"
    >
      <div
        :style="{ maxWidth: maxWidth + 'px' }"
        class="w-full sm:w-[min(100%,var(--sheet-max,600px))] mx-auto bg-white dark:bg-[#1c1c1e] rounded-t-ios-card sm:rounded-ios-card shadow-ios-sheet ring-1 ring-white/50 dark:ring-white/10 max-h-[80vh] flex flex-col overflow-hidden animate-sheet-up"
      >
        <slot />
      </div>
    </div>
  </Teleport>
</template>
