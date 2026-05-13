<script setup lang="ts">
import { AlertTriangle, CheckCircle2, AlertCircle, Info, X } from 'lucide-vue-next'
import { toastQueue, dismissToast } from '../../utils/toast'

// IToast ── v1.5.0 iOS 顶部下滑通知
// 设计：
//   - 屏幕顶部下滑（不是底部），离活动 cursor 远些
//   - 22px 大圆角 + glass blur (iOS 通知中心风)
//   - 4 种 tone：success/绿、error/红、warning/黄、info/蓝
//   - 多 toast 堆叠时 stagger 入场 + Transition Group move
//   - 点击 toast 任意位置可手动关闭（移动端常见手势）
//   - 自动 dismiss 由 utils/toast.ts 控制（默认 3s）

const handleDismiss = (id: number) => {
  dismissToast(id)
}
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed top-[58px] left-1/2 z-[200] flex -translate-x-1/2 flex-col gap-2 pointer-events-none w-[min(460px,calc(100vw-2rem))]"
      aria-live="polite"
    >
      <TransitionGroup
        enter-active-class="transition duration-[280ms] ease-[cubic-bezier(0.34,1.56,0.64,1)]"
        enter-from-class="opacity-0 -translate-y-8 scale-95"
        enter-to-class="opacity-100 translate-y-0 scale-100"
        leave-active-class="transition duration-200 ease-in absolute left-0 right-0"
        leave-from-class="opacity-100 scale-100"
        leave-to-class="opacity-0 -translate-y-3 scale-95"
        move-class="transition-all duration-300 ease-out"
      >
        <div
          v-for="t in toastQueue"
          :key="t.id"
          class="pointer-events-auto group flex items-start gap-3 rounded-[22px] border-2 px-4 py-3 shadow-[0_12px_36px_rgba(0,0,0,0.12)] dark:shadow-[0_12px_36px_rgba(0,0,0,0.4)] backdrop-blur-[24px] text-[14px] leading-snug cursor-pointer transition-transform hover:scale-[1.01]"
          :class="[
            t.kind === 'success'
              ? 'bg-emerald-50/95 dark:bg-emerald-950/70 border-emerald-500/25 text-emerald-800 dark:text-emerald-200'
              : t.kind === 'error'
                ? 'bg-rose-50/95 dark:bg-rose-950/70 border-rose-500/25 text-rose-800 dark:text-rose-200'
                : t.kind === 'warning'
                  ? 'bg-amber-50/95 dark:bg-amber-950/70 border-amber-500/25 text-amber-800 dark:text-amber-200'
                  : 'bg-white/95 dark:bg-[#1c1c1e]/95 border-black/[0.08] dark:border-white/[0.1] text-ios-text dark:text-ios-textDark',
          ]"
          @click="handleDismiss(t.id)"
        >
          <div class="shrink-0 pt-0.5">
            <CheckCircle2
              v-if="t.kind === 'success'"
              class="w-[22px] h-[22px] text-emerald-500"
              stroke-width="2.4"
            />
            <AlertCircle
              v-else-if="t.kind === 'error'"
              class="w-[22px] h-[22px] text-rose-500"
              stroke-width="2.4"
            />
            <AlertTriangle
              v-else-if="t.kind === 'warning'"
              class="w-[22px] h-[22px] text-amber-500"
              stroke-width="2.4"
            />
            <Info v-else class="w-[22px] h-[22px] text-ios-blue" stroke-width="2.4" />
          </div>
          <p class="flex-1 whitespace-pre-line break-words font-semibold">{{ t.message }}</p>
          <button
            type="button"
            class="shrink-0 -mr-1 -mt-1 opacity-0 group-hover:opacity-100 transition-opacity p-1 rounded-full hover:bg-black/[0.06] dark:hover:bg-white/[0.08]"
            @click.stop="handleDismiss(t.id)"
            aria-label="关闭"
          >
            <X class="w-3.5 h-3.5" stroke-width="2.5" />
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>
