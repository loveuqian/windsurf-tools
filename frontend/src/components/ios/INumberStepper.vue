<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from 'vue'
import { Minus, Plus } from 'lucide-vue-next'

// INumberStepper ── iOS-style 数字增减组件
// 行为：
//   - 点击 ± 按钮 ±step（默认 1）
//   - 按住 500ms 后开始连续 +/− , 间隔 80ms (iOS 原生 stepper 节奏)
//   - 数字区可点击直接键入（保留键盘输入的灵活）
//   - 越过 min/max 自动钳制
//   - active:scale-95 + spring 过渡，触感

const props = withDefaults(
  defineProps<{
    modelValue: number
    min?: number
    max?: number
    step?: number
    suffix?: string
    disabled?: boolean
    /** 数字区宽度（不同语义可能要不同宽，比如分钟 3 位 vs 端口 5 位） */
    width?: number
    /** 数字区是否可直接键入（默认 true） */
    editable?: boolean
  }>(),
  {
    min: Number.MIN_SAFE_INTEGER,
    max: Number.MAX_SAFE_INTEGER,
    step: 1,
    suffix: '',
    disabled: false,
    width: 56,
    editable: true,
  },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: number): void }>()

const clamp = (v: number) =>
  Math.min(props.max, Math.max(props.min, Math.round(v)))

const displayValue = computed(() => String(props.modelValue))

const editing = ref(false)
const editBuffer = ref('')

const apply = (delta: number) => {
  if (props.disabled) return
  emit('update:modelValue', clamp(props.modelValue + delta))
}

// ── 长按加速逻辑 ──
let pressTimer: ReturnType<typeof setTimeout> | null = null
let repeatTimer: ReturnType<typeof setInterval> | null = null

const clearTimers = () => {
  if (pressTimer) {
    clearTimeout(pressTimer)
    pressTimer = null
  }
  if (repeatTimer) {
    clearInterval(repeatTimer)
    repeatTimer = null
  }
}

const startPress = (delta: number) => {
  if (props.disabled) return
  apply(delta) // 即时响应一次
  pressTimer = setTimeout(() => {
    repeatTimer = setInterval(() => apply(delta), 80)
  }, 500)
}

const endPress = () => clearTimers()

onBeforeUnmount(clearTimers)

const handleEditFocus = (e: FocusEvent) => {
  if (!props.editable || props.disabled) {
    ;(e.target as HTMLElement).blur()
    return
  }
  editing.value = true
  editBuffer.value = String(props.modelValue)
}

const handleEditBlur = () => {
  editing.value = false
  const n = Number(editBuffer.value)
  if (Number.isFinite(n)) {
    emit('update:modelValue', clamp(n))
  }
}

const handleEditKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Enter') {
    ;(e.target as HTMLInputElement).blur()
  } else if (e.key === 'Escape') {
    editBuffer.value = String(props.modelValue)
    ;(e.target as HTMLInputElement).blur()
  }
}

const atMin = computed(() => props.modelValue <= props.min)
const atMax = computed(() => props.modelValue >= props.max)
</script>

<template>
  <div
    class="no-drag-region inline-flex items-center gap-0.5 rounded-full bg-gray-100 dark:bg-white/[0.08] border border-black/[0.06] dark:border-white/[0.08] p-0.5 shadow-inner"
  >
    <button
      type="button"
      class="flex h-7 w-7 items-center justify-center rounded-full transition-all duration-150 ease-out active:scale-90"
      :class="
        atMin || disabled
          ? 'text-gray-400 dark:text-white/30 cursor-not-allowed'
          : 'text-gray-700 dark:text-gray-200 hover:bg-white dark:hover:bg-white/[0.1] active:bg-white'
      "
      :disabled="atMin || disabled"
      @mousedown="startPress(-step)"
      @mouseup="endPress"
      @mouseleave="endPress"
      @touchstart.passive="startPress(-step)"
      @touchend="endPress"
      @touchcancel="endPress"
    >
      <Minus class="h-3.5 w-3.5" stroke-width="3" />
    </button>
    <div
      class="flex items-center justify-center px-1"
      :style="{ minWidth: `${width}px` }"
    >
      <input
        v-if="editing"
        v-model="editBuffer"
        type="number"
        :min="min"
        :max="max"
        :step="step"
        class="w-full text-center bg-transparent border-none outline-none text-[15px] font-bold tabular-nums text-gray-900 dark:text-gray-100"
        @blur="handleEditBlur"
        @keydown="handleEditKeydown"
      />
      <span
        v-else
        class="text-[15px] font-bold tabular-nums text-gray-900 dark:text-gray-100"
        :class="editable && !disabled ? 'cursor-text' : ''"
        tabindex="0"
        @focus="handleEditFocus"
        @click="handleEditFocus"
      >
        {{ displayValue }}<span
          v-if="suffix"
          class="ml-0.5 text-[11px] font-medium text-gray-400 dark:text-gray-500"
        >{{ suffix }}</span>
      </span>
    </div>
    <button
      type="button"
      class="flex h-7 w-7 items-center justify-center rounded-full transition-all duration-150 ease-out active:scale-90"
      :class="
        atMax || disabled
          ? 'text-gray-400 dark:text-white/30 cursor-not-allowed'
          : 'text-gray-700 dark:text-gray-200 hover:bg-white dark:hover:bg-white/[0.1] active:bg-white'
      "
      :disabled="atMax || disabled"
      @mousedown="startPress(step)"
      @mouseup="endPress"
      @mouseleave="endPress"
      @touchstart.passive="startPress(step)"
      @touchend="endPress"
      @touchcancel="endPress"
    >
      <Plus class="h-3.5 w-3.5" stroke-width="3" />
    </button>
  </div>
</template>
