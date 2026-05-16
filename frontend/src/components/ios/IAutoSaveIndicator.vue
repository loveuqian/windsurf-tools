<script setup lang="ts">
/**
 * IAutoSaveIndicator — Settings.vue 自动保存状态指示。
 *
 * 4 种状态：idle / saving / saved / error。saved 显示后 3s 自动 fade，
 * error 持续显示直到状态变化。
 */
import { computed } from "vue";
import { CheckCircle2, Loader2, AlertCircle } from "lucide-vue-next";

const props = defineProps<{
  state: "idle" | "saving" | "saved" | "error";
  errorText?: string;
}>();

const visible = computed(() => props.state !== "idle");

const display = computed(() => {
  switch (props.state) {
    case "saving":
      return {
        icon: Loader2,
        text: "保存中…",
        cls: "text-ios-blue dark:text-ios-blueDark",
        spin: true,
      };
    case "saved":
      return {
        icon: CheckCircle2,
        text: "已保存",
        cls: "text-emerald-600 dark:text-emerald-400",
        spin: false,
      };
    case "error":
      return {
        icon: AlertCircle,
        text: props.errorText || "保存失败",
        cls: "text-rose-600 dark:text-rose-400",
        spin: false,
      };
    default:
      return { icon: CheckCircle2, text: "", cls: "", spin: false };
  }
});
</script>

<template>
  <Transition
    enter-active-class="transition-opacity duration-200"
    leave-active-class="transition-opacity duration-300"
    enter-from-class="opacity-0"
    leave-to-class="opacity-0"
  >
    <span
      v-if="visible"
      :class="[
        'inline-flex items-center gap-1.5 text-[12px] font-semibold',
        display.cls,
      ]"
      role="status"
      :aria-live="state === 'error' ? 'assertive' : 'polite'"
    >
      <component
        :is="display.icon"
        :class="['h-3.5 w-3.5', display.spin ? 'animate-spin' : '']"
        stroke-width="2.5"
      />
      {{ display.text }}
    </span>
  </Transition>
</template>
