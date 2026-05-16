<!-- F7-REMOVAL: 整文件删除（仅作者自用功能；发布前彻底移除） -->
<script setup lang="ts">
/**
 * F7Banner — SmartFriend(F7) 模式下显示的提示条。
 *
 * variant:
 *   - 'full'    Dashboard 顶部完整状态条（图标 + 标题 + 副标题）
 *   - 'compact' Accounts 头部 inline chip（小图标 + 一行）
 *
 * 仅依赖 useSmartFriend().active 控制显隐，不直接读 store。
 * 调用方写法：<F7Banner variant="full" />。
 */
import { ShieldCheck } from "lucide-vue-next";
import { useSmartFriend } from "../composables/useSmartFriend";

defineProps<{ variant?: "full" | "compact" }>();

const sf = useSmartFriend();
</script>

<template>
  <template v-if="sf.active.value">
    <!-- compact: 与 Accounts 头部 chip 行齐 -->
    <div
      v-if="variant === 'compact'"
      class="mt-2 inline-flex items-center gap-2 rounded-ios-pill border border-emerald-500/25 bg-gradient-to-r from-emerald-500/[0.10] to-violet-500/[0.06] px-3 py-1.5 text-[11px] font-semibold text-emerald-700 dark:text-emerald-300"
      title="SmartFriend 模式下，服务端按 SMART_FRIEND 计费、绕过日/周限额"
    >
      <ShieldCheck class="h-3.5 w-3.5" stroke-width="2.5" />
      F7 模式 · 已绕过日/周额度限制
    </div>

    <!-- full: Dashboard 顶部完整状态条 -->
    <div
      v-else
      class="mx-6 mb-5 flex flex-wrap items-center gap-2 rounded-ios-block border border-emerald-500/25 bg-gradient-to-r from-emerald-500/[0.10] to-violet-500/[0.06] px-4 py-2.5 text-[12px]"
    >
      <span
        class="inline-flex items-center gap-1.5 rounded-ios-pill bg-emerald-500/15 px-2.5 py-1 text-[11px] font-bold text-emerald-700 dark:text-emerald-300"
      >
        <ShieldCheck class="h-3.5 w-3.5" stroke-width="2.5" />
        F7 已开启
      </span>
      <span class="font-semibold text-emerald-800 dark:text-emerald-200">
        SmartFriend 模式 · 服务端按 SMART_FRIEND 计费、绕过日/周限额
      </span>
      <span class="text-emerald-700/70 dark:text-emerald-300/80 leading-relaxed">
        · 显示「耗尽」的账号实际仍可用，自动切号已暂停。
      </span>
    </div>
  </template>
</template>
