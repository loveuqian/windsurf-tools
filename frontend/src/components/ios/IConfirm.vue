<script setup lang="ts">
import { AlertTriangle, Trash2, HelpCircle } from 'lucide-vue-next'
import { confirmState, resolveConfirm } from '../../utils/toast'
import { onBeforeUnmount, onMounted } from 'vue'

// IConfirm ── v1.5.0 iOS Action Sheet 风
// 设计：
//   - 底部弹出 sheet（不是中心模态），更像 iOS Action Sheet
//   - 大圆角 + 玻璃 + 拖手柄 + 主按钮上 / 取消按钮下 (iOS HIG 标准)
//   - destructive 时主按钮红色 + 大警告 icon
//   - ESC / 遮罩点击 / 取消按钮 都关闭
//   - 弹簧曲线入场

const onKey = (e: KeyboardEvent) => {
  if (!confirmState.value.visible) return
  if (e.key === 'Escape') {
    resolveConfirm(false)
  } else if (e.key === 'Enter') {
    resolveConfirm(true)
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="confirmState.visible"
        class="fixed inset-0 z-[210] flex items-end sm:items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-md p-0 sm:p-4"
        @click.self="resolveConfirm(false)"
      >
        <Transition
          enter-active-class="transition duration-[320ms] ease-[cubic-bezier(0.34,1.56,0.64,1)]"
          enter-from-class="opacity-0 translate-y-8 sm:translate-y-0 sm:scale-95"
          enter-to-class="opacity-100 translate-y-0 sm:scale-100"
          leave-active-class="transition duration-200 ease-in"
          leave-from-class="opacity-100 translate-y-0 sm:scale-100"
          leave-to-class="opacity-0 translate-y-6 sm:translate-y-0 sm:scale-95"
        >
          <div
            v-if="confirmState.visible"
            class="w-full sm:w-[min(100%,400px)] mx-auto sm:mx-0 bg-white dark:bg-[#1c1c1e] backdrop-blur-xl rounded-t-[28px] sm:rounded-[28px] shadow-[0_-20px_60px_rgba(0,0,0,0.3)] dark:shadow-[0_-20px_60px_rgba(0,0,0,0.6)] ring-1 ring-white/50 dark:ring-white/10 overflow-hidden"
            role="dialog"
            aria-modal="true"
          >
            <!-- 拖手柄 (仅小屏底部 sheet 模式) -->
            <div class="flex items-center justify-center pt-3 pb-1 sm:hidden">
              <div class="h-1.5 w-10 rounded-full bg-black/15 dark:bg-white/20" />
            </div>

            <!-- 内容 -->
            <div class="px-6 pt-5 pb-4 flex flex-col items-center gap-4">
              <!-- 大图标 -->
              <div
                class="w-16 h-16 rounded-[20px] flex items-center justify-center shadow-sm"
                :class="
                  confirmState.destructive
                    ? 'bg-rose-500/15 text-rose-600 dark:text-rose-400'
                    : 'bg-ios-blue/15 text-ios-blue dark:text-blue-300'
                "
              >
                <AlertTriangle
                  v-if="confirmState.destructive"
                  class="w-8 h-8"
                  stroke-width="2.4"
                />
                <HelpCircle v-else class="w-8 h-8" stroke-width="2.2" />
              </div>

              <p class="text-[15px] leading-relaxed text-center text-gray-700 dark:text-gray-200 font-medium whitespace-pre-line">
                {{ confirmState.message }}
              </p>
            </div>

            <!-- 按钮组 - iOS 风：主按钮上，取消下，垂直 stack -->
            <div class="px-3 pb-3 flex flex-col gap-2">
              <button
                type="button"
                class="no-drag-region w-full py-3.5 rounded-[16px] text-[15px] font-bold transition-all active:scale-[0.98]"
                :class="
                  confirmState.destructive
                    ? 'bg-rose-500 text-white hover:bg-rose-600 shadow-md shadow-rose-500/20'
                    : 'bg-ios-blue text-white hover:bg-blue-600 shadow-md shadow-ios-blue/20'
                "
                @click="resolveConfirm(true)"
              >
                <span class="inline-flex items-center gap-2 justify-center">
                  <Trash2 v-if="confirmState.destructive" class="w-4 h-4" stroke-width="2.6" />
                  {{ confirmState.confirmText }}
                </span>
              </button>
              <button
                type="button"
                class="no-drag-region w-full py-3.5 rounded-[16px] text-[15px] font-bold bg-black/[0.05] dark:bg-white/[0.08] text-gray-700 dark:text-gray-200 hover:bg-black/[0.08] dark:hover:bg-white/[0.12] transition-all active:scale-[0.98]"
                @click="resolveConfirm(false)"
              >
                {{ confirmState.cancelText }}
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
