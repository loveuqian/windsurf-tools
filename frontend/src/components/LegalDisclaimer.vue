<script setup lang="ts">
import { AlertTriangle } from "lucide-vue-next";

// LegalDisclaimer ── 法律免责声明组件
// 设计：作为 About 视图底部 / 首次启动 Onboarding 强制阅读
// 6 条核心条款，覆盖用户使用风险 + 作者免责
</script>

<template>
  <section
    class="rounded-[22px] border-2 border-amber-500/30 bg-amber-50/60 dark:bg-amber-950/30 p-5 sm:p-6"
  >
    <div class="flex items-center gap-3 mb-4">
      <div
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-amber-500/20 text-amber-700 dark:text-amber-300"
      >
        <AlertTriangle class="h-5 w-5" stroke-width="2.4" />
      </div>
      <h3 class="text-[16px] font-bold text-amber-900 dark:text-amber-200">
        法律免责声明 · Legal Disclaimer
      </h3>
    </div>

    <ol class="space-y-2.5 text-[13px] leading-relaxed text-amber-900/90 dark:text-amber-100/90 list-decimal pl-5">
      <li>
        <b>用途限定</b> — 本工具仅供 <b>学术研究、个人学习与技术交流</b> 使用，
        <b>禁止</b> 用于商业用途、转售、滥用或任何形式的非法行为。
      </li>
      <li>
        <b>用户自负风险</b> — 用户须自行承担使用过程中产生的所有风险，包括但不限于：
        账号封禁、订阅吊销、数据丢失。作者及贡献者 <b>不承担任何法律责任</b>。
      </li>
      <li>
        <b>账号合法性</b> — 用户须保证所导入的账号是 <b>通过合法途径获得</b>，
        且使用方式符合 Windsurf / Codeium 及上游服务（Anthropic / OpenAI 等）的服务条款。
      </li>
      <li>
        <b>本地处理</b> — MITM 代理仅在本机 <code>localhost</code> 工作，
        <b>不会上传任何用户凭证、对话内容、API Key 到任何第三方服务器</b>。
        所有数据持久化在本地 <code>settings.json</code> / <code>accounts.json</code>。
      </li>
      <li>
        <b>破限注入风险</b> — Cascade 破限注入功能通过修改 system prompt 实现，
        部分预设可能触发 Anthropic 服务端内容审查策略导致请求被拒。
        默认「极简」预设最安全；「原版」预设含敏感关键词 <b>必触发网关拒绝</b>。
      </li>
      <li>
        <b>开源 + 免费</b> — 本项目以 MIT 协议开源，永久免费。
        作者保留以任何理由停止维护或修改功能的权利。
        如有问题或建议，欢迎通过仓库 issue 反馈。
      </li>
    </ol>

    <div
      class="mt-4 pt-3 border-t border-amber-500/20 text-[11.5px] text-amber-700/80 dark:text-amber-300/80 leading-relaxed"
    >
      继续使用本工具即表示您 <b>已阅读并同意</b> 上述条款。
    </div>
  </section>
</template>
