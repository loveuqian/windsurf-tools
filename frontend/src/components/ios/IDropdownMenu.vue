<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { MoreHorizontal } from 'lucide-vue-next'

// IDropdownMenu ── iOS 风格 popover「更多操作」菜单
//
// 设计目标：把「次要操作」从工具栏 / 卡片角落收进折叠菜单，避免按钮过密、
// 让新手一眼看不到主操作。和 ISelectSheet（底部 sheet，单选 / 切换值）不同：
//   - 这里是「列出动作」，点完立即关闭并执行
//   - 浮层定位锚定在触发器下方，不抢满屏注意力
//   - 单个 item 可标 danger（红字）/ disabled / divider 分隔
//
// 用法：
//   <IDropdownMenu :items="[{ label: '同步额度', icon: BarChart3, onClick: ... }]" />
//   <IDropdownMenu compact :items="..." />  紧凑圆形 ⋯
//   <IDropdownMenu :items="..." align="left" width="w-64" trigger-label="更多操作" />

export type DropdownItem =
  | { type: 'divider' }
  | {
      type?: 'item'
      label: string
      icon?: any
      onClick: () => void
      danger?: boolean
      disabled?: boolean
      hint?: string
    }

const props = withDefaults(
  defineProps<{
    items: DropdownItem[]
    align?: 'left' | 'right'
    width?: string
    triggerLabel?: string
    /**
     * lucide-vue-next 的图标组件本身是 function（functional component）。
     * Vue 在解析 prop 默认值时，若 default 是 function 且 prop 类型不是 Function，
     * 会把 default 当工厂函数 invoke 一次（`MoreHorizontal({}, undefined)`），
     * 这会把 lucide 内部 `(props, { slots })` 解构 undefined 引爆整棵子树
     * （表现为：包含 IDropdownMenu 的视图整个空白）。
     * 这里**故意不写 default**，模板里用 `triggerIcon || MoreHorizontal` fallback。
     */
    triggerIcon?: any
    /** 自定义触发器外观；compact / 普通模式各自有默认值 */
    triggerClass?: string
    /** compact 模式：圆形小按钮，仅显示图标，适合卡片角 */
    compact?: boolean
    triggerTitle?: string
    disabled?: boolean
  }>(),
  {
    align: 'right',
    width: 'w-56',
    triggerLabel: '更多',
    triggerClass: '',
    compact: false,
    triggerTitle: '更多操作',
    disabled: false,
  },
)

const open = ref(false)
const root = ref<HTMLElement | null>(null)

const toggle = () => {
  if (props.disabled) return
  open.value = !open.value
}

const handleClick = (item: DropdownItem) => {
  if ((item as any).type === 'divider') return
  if ((item as any).disabled) return
  ;(item as any).onClick?.()
  open.value = false
}

const isDivider = (item: DropdownItem) => (item as any).type === 'divider'

const onDocumentClick = (e: MouseEvent) => {
  if (!root.value) return
  if (!root.value.contains(e.target as Node)) {
    open.value = false
  }
}

const onKey = (e: KeyboardEvent) => {
  if (e.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('mousedown', onDocumentClick)
  document.addEventListener('keydown', onKey)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocumentClick)
  document.removeEventListener('keydown', onKey)
})

const triggerVariantClass = computed(() => {
  if (props.triggerClass) return props.triggerClass
  if (props.compact) {
    return 'flex h-[30px] w-[30px] items-center justify-center rounded-full bg-white text-ios-textSecondary shadow-sm transition hover:scale-105 dark:bg-black/40 dark:text-ios-textSecondaryDark disabled:opacity-50'
  }
  return 'inline-flex items-center gap-1.5 px-4 py-2 rounded-full bg-black/5 dark:bg-white/10 text-ios-text dark:text-ios-textDark text-[13px] font-semibold transition-colors hover:bg-black/10 dark:hover:bg-white/15 disabled:opacity-50'
})
</script>

<template>
  <div ref="root" class="relative inline-block">
    <button
      type="button"
      class="no-drag-region ios-btn"
      :class="triggerVariantClass"
      :title="triggerTitle"
      :disabled="disabled"
      :aria-haspopup="'menu'"
      :aria-expanded="open ? 'true' : 'false'"
      @click="toggle"
    >
      <component
        :is="triggerIcon || MoreHorizontal"
        :class="compact ? 'h-[15px] w-[15px]' : 'h-[16px] w-[16px]'"
        stroke-width="2.5"
      />
      <span v-if="!compact" class="whitespace-nowrap">{{ triggerLabel }}</span>
    </button>

    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0 scale-95 -translate-y-1"
      enter-to-class="opacity-100 scale-100 translate-y-0"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="opacity-100 scale-100"
      leave-to-class="opacity-0 scale-95"
    >
      <div
        v-show="open"
        role="menu"
        class="absolute z-[80] mt-2 origin-top rounded-[18px] border border-black/[0.06] bg-white/95 p-1.5 shadow-[0_18px_48px_-16px_rgba(15,23,42,0.28)] backdrop-blur-xl dark:border-white/[0.08] dark:bg-[#1C1C1E]/96"
        :class="[
          width,
          align === 'right' ? 'right-0' : 'left-0',
        ]"
      >
        <template v-for="(item, idx) in items" :key="idx">
          <div
            v-if="isDivider(item)"
            class="my-1 mx-1 h-px bg-black/[0.06] dark:bg-white/[0.08]"
            role="separator"
          />
          <button
            v-else
            type="button"
            role="menuitem"
            class="no-drag-region group flex w-full items-center gap-2.5 rounded-[12px] px-3 py-2 text-left text-[13px] font-semibold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            :class="[
              (item as any).danger
                ? 'text-rose-600 hover:bg-rose-500/[0.08] dark:text-rose-300'
                : 'text-ios-text hover:bg-black/[0.05] dark:text-ios-textDark dark:hover:bg-white/[0.07]',
            ]"
            :disabled="(item as any).disabled === true"
            @click="handleClick(item)"
          >
            <component
              v-if="(item as any).icon"
              :is="(item as any).icon"
              class="h-4 w-4 shrink-0 opacity-80 group-hover:opacity-100"
              stroke-width="2.4"
            />
            <span class="flex-1 truncate">{{ (item as any).label }}</span>
            <span
              v-if="(item as any).hint"
              class="shrink-0 text-[11px] font-medium text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              {{ (item as any).hint }}
            </span>
          </button>
        </template>
      </div>
    </Transition>
  </div>
</template>
