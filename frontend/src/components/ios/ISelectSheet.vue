<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { Check, ChevronDown, X } from 'lucide-vue-next'

// ISelectSheet ── iOS-style 弹底部选择 sheet
// 行为：
//   - trigger button 显示当前选中 label + ⌄ 图标
//   - 点 trigger → 底部弹 sheet (固定 bottom，rounded-top-3xl，backdrop-blur)
//   - sheet 内: option list, 当前选中行右侧 Check 图标, 点选 → 关闭 + emit
//   - 背景遮罩点击 / ESC / 右上角 × 都能关
//   - 支持 description 行（小灰字）
//   - 大列表自动滚动 (max-h-70vh)

type Option = {
  value: string | number
  label: string
  description?: string
}

const props = withDefaults(
  defineProps<{
    modelValue: string | number
    options: Option[]
    /** sheet 标题，省略时不显示标题栏 */
    title?: string
    placeholder?: string
    /** trigger 按钮宽度 (Tailwind w- class，比如 'w-full' / 'w-40') */
    width?: string
    disabled?: boolean
  }>(),
  {
    title: '',
    placeholder: '未选择',
    width: 'w-full',
    disabled: false,
  },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: string | number): void }>()

const open = ref(false)

const selectedLabel = computed(() => {
  const opt = props.options.find((o) => o.value === props.modelValue)
  return opt?.label || props.placeholder
})

const handleSelect = (opt: Option) => {
  emit('update:modelValue', opt.value)
  open.value = false
}

const onKey = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && open.value) {
    open.value = false
  }
}

onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div :class="['relative inline-flex', width]">
    <!-- Trigger -->
    <button
      type="button"
      class="no-drag-region w-full flex items-center justify-between gap-2 rounded-[14px] px-4 py-2.5 bg-white dark:bg-[#1C1C1E] border border-black/[0.06] dark:border-white/[0.08] text-[14px] font-medium text-gray-800 dark:text-gray-200 shadow-sm transition-all active:scale-[0.98] hover:bg-black/[0.02] dark:hover:bg-white/[0.04] disabled:opacity-50"
      :disabled="disabled"
      @click="open = true"
    >
      <span class="truncate">{{ selectedLabel }}</span>
      <ChevronDown
        class="h-4 w-4 shrink-0 text-gray-500 dark:text-gray-400 transition-transform"
        :class="open ? 'rotate-180' : ''"
        stroke-width="2.4"
      />
    </button>

    <!-- Sheet 用 teleport 跳到 body 末尾，避免被父 overflow 截断 -->
    <Teleport to="body">
      <Transition name="sheet">
        <div
          v-if="open"
          class="fixed inset-0 z-[200] flex items-end justify-center bg-black/40 dark:bg-black/60 backdrop-blur-md"
          @click.self="open = false"
        >
          <div
            class="w-full sm:max-w-[440px] mx-auto bg-white dark:bg-[#1C1C1E] rounded-t-ios-card sm:rounded-ios-card sm:mb-8 shadow-ios-sheet ring-1 ring-white/50 dark:ring-white/10 max-h-[75vh] flex flex-col overflow-hidden animate-sheet-up"
          >
            <!-- header -->
            <div
              v-if="title"
              class="flex items-center justify-between px-5 pt-4 pb-3 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <h3 class="text-[16px] font-bold text-gray-900 dark:text-gray-100">
                {{ title }}
              </h3>
              <button
                type="button"
                class="flex h-8 w-8 items-center justify-center rounded-full bg-black/[0.05] dark:bg-white/[0.08] hover:bg-black/[0.1] dark:hover:bg-white/[0.12] transition-colors"
                @click="open = false"
              >
                <X class="h-4 w-4 text-gray-700 dark:text-gray-300" stroke-width="2.5" />
              </button>
            </div>

            <!-- 拖手柄 (无 title 时显示) -->
            <div
              v-else
              class="flex items-center justify-center pt-3 pb-2"
            >
              <div class="h-1.5 w-10 rounded-full bg-black/15 dark:bg-white/20" />
            </div>

            <!-- options -->
            <div class="flex-1 overflow-y-auto px-2 py-2">
              <button
                v-for="opt in options"
                :key="opt.value"
                type="button"
                class="w-full flex items-center justify-between gap-3 rounded-[14px] px-4 py-3 text-left transition-colors active:bg-black/[0.05] dark:active:bg-white/[0.06]"
                :class="
                  opt.value === modelValue
                    ? 'bg-ios-blue/[0.08] dark:bg-ios-blue/[0.18]'
                    : 'hover:bg-black/[0.03] dark:hover:bg-white/[0.04]'
                "
                @click="handleSelect(opt)"
              >
                <div class="min-w-0 flex-1">
                  <div
                    class="text-[15px] font-bold truncate"
                    :class="
                      opt.value === modelValue
                        ? 'text-ios-blue'
                        : 'text-gray-900 dark:text-gray-100'
                    "
                  >
                    {{ opt.label }}
                  </div>
                  <div
                    v-if="opt.description"
                    class="mt-0.5 text-[12px] text-gray-500 dark:text-gray-400 leading-snug"
                  >
                    {{ opt.description }}
                  </div>
                </div>
                <Check
                  v-if="opt.value === modelValue"
                  class="h-5 w-5 shrink-0 text-ios-blue"
                  stroke-width="2.6"
                />
              </button>
            </div>

            <!-- cancel 按钮 (iOS 风) -->
            <div class="p-3 border-t border-black/[0.04] dark:border-white/[0.04]">
              <button
                type="button"
                class="w-full py-3 rounded-[14px] bg-black/[0.05] dark:bg-white/[0.08] text-[15px] font-bold text-gray-800 dark:text-gray-200 hover:bg-black/[0.08] dark:hover:bg-white/[0.12] transition-colors"
                @click="open = false"
              >
                取消
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.sheet-enter-active,
.sheet-leave-active {
  transition: opacity 0.25s ease;
}
.sheet-enter-from,
.sheet-leave-to {
  opacity: 0;
}

.animate-sheet-up {
  animation: sheet-up 0.35s cubic-bezier(0.34, 1.56, 0.64, 1);
}
@keyframes sheet-up {
  from {
    transform: translateY(20px);
    opacity: 0.6;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}
</style>
