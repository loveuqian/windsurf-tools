<script setup lang="ts">
import { computed, ref } from "vue";
import {
  BookOpen,
  ChevronDown,
  HelpCircle,
  KeyRound,
  RefreshCw,
  Search,
  Shield,
  Shuffle,
  Sparkles,
  Users,
} from "lucide-vue-next";

// Help ── 用户使用说明 / FAQ
// 设计：7 章可折叠，iOS Settings disclosure 风
// 内容覆盖：模式选择 / API Key / 导入 / MITM / 号池 / Clash / 破限

type Chapter = {
  id: string;
  icon: typeof KeyRound;
  iconColor: string;
  title: string;
  summary: string;
};

const chapters: Chapter[] = [
  {
    id: "mode-choice",
    icon: HelpCircle,
    iconColor: "text-ios-blue bg-ios-blue/10",
    title: "1. 我该选哪种模式？IDE 直接切号 vs MITM 代理",
    summary: "搞清楚两个核心方案的区别",
  },
  {
    id: "api-key",
    icon: KeyRound,
    iconColor: "text-violet-600 bg-violet-500/10",
    title: "2. API Key 是什么？怎么获取？",
    summary: "sk-ws- 前缀解释 + 来源",
  },
  {
    id: "import",
    icon: Sparkles,
    iconColor: "text-amber-600 bg-amber-500/10",
    title: "3. 怎么导入账号（4 种格式）",
    summary: "API Key / JWT / 邮箱密码 / Refresh Token 自动识别",
  },
  {
    id: "mitm",
    icon: Shield,
    iconColor: "text-emerald-600 bg-emerald-500/10",
    title: "4. MITM 代理是什么？为什么需要 CA 证书 + Hosts？",
    summary: "原理 + 安全性 + 配置流程",
  },
  {
    id: "rotation",
    icon: RefreshCw,
    iconColor: "text-sky-600 bg-sky-500/10",
    title: "5. 号池如何自动轮换？Pin 和 Pool 又是什么？",
    summary: "3 个自动切换触发点 + 锁定 + 轮换池",
  },
  {
    id: "clash",
    icon: Shuffle,
    iconColor: "text-violet-600 bg-violet-500/10",
    title: "6. Clash IP 轮换为什么要 + 怎么用智能启用",
    summary: "防限速 + 一键自动检测",
  },
  {
    id: "jailbreak",
    icon: Sparkles,
    iconColor: "text-rose-600 bg-rose-500/10",
    title: "7. Cascade 破限注入原理 + 预设区别 + 风险",
    summary: "MITM 注入 system prompt 末尾 + 4 个预设对比",
  },
];

const searchQuery = ref("");
const openedChapter = ref<string>(""); // 同时只展开一个

const filteredChapters = computed(() => {
  const q = searchQuery.value.trim().toLowerCase();
  if (!q) return chapters;
  return chapters.filter(
    (c) =>
      c.title.toLowerCase().includes(q) || c.summary.toLowerCase().includes(q),
  );
});

const toggleChapter = (id: string) => {
  openedChapter.value = openedChapter.value === id ? "" : id;
};
</script>

<template>
  <div class="p-6 md:p-10 max-w-4xl mx-auto w-full pb-12">
    <!-- 标题 -->
    <header class="flex items-center gap-4 mb-8">
      <div
        class="w-14 h-14 rounded-[20px] bg-gradient-to-br from-ios-blue to-cyan-400 text-white flex items-center justify-center shadow-[0_10px_24px_rgba(37,99,235,0.24)]"
      >
        <BookOpen class="h-7 w-7" stroke-width="2.4" />
      </div>
      <div>
        <h1 class="text-[28px] font-bold text-ios-text dark:text-ios-textDark tracking-tight">
          使用说明
        </h1>
        <p class="text-[13px] text-gray-500 dark:text-gray-400 font-medium mt-1">
          常见问题 · 功能介绍 · 上手必读
        </p>
      </div>
    </header>

    <!-- 搜索 -->
    <div class="relative mb-6">
      <Search
        class="absolute left-3.5 top-1/2 -translate-y-1/2 w-[18px] h-[18px] text-gray-400 pointer-events-none"
      />
      <input
        v-model="searchQuery"
        type="search"
        placeholder="搜索章节…"
        class="no-drag-region w-full pl-11 pr-4 py-3 rounded-[16px] bg-white dark:bg-[#1C1C1E] border border-black/[0.06] dark:border-white/[0.08] text-[14px] outline-none focus:ring-2 focus:ring-ios-blue/30 transition-shadow"
      />
    </div>

    <!-- FAQ 列表 -->
    <div class="space-y-2">
      <article
        v-for="c in filteredChapters"
        :key="c.id"
        class="rounded-[20px] border border-black/[0.05] dark:border-white/[0.08] bg-white/70 dark:bg-white/[0.04] overflow-hidden transition-all"
      >
        <!-- 折叠 trigger -->
        <button
          type="button"
          class="no-drag-region w-full flex items-center gap-3 px-5 py-4 text-left transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
          @click="toggleChapter(c.id)"
        >
          <div
            class="w-10 h-10 rounded-[12px] flex items-center justify-center shrink-0"
            :class="c.iconColor"
          >
            <component :is="c.icon" class="w-5 h-5" stroke-width="2.4" />
          </div>
          <div class="flex-1 min-w-0">
            <div class="text-[14px] font-bold text-ios-text dark:text-ios-textDark">
              {{ c.title }}
            </div>
            <div class="mt-0.5 text-[12px] text-gray-500 dark:text-gray-400">
              {{ c.summary }}
            </div>
          </div>
          <ChevronDown
            class="w-5 h-5 shrink-0 text-gray-400 transition-transform"
            :class="openedChapter === c.id ? 'rotate-180' : ''"
            stroke-width="2.4"
          />
        </button>

        <!-- 折叠内容 -->
        <Transition
          enter-active-class="transition-all duration-300 ease-out overflow-hidden"
          enter-from-class="opacity-0 max-h-0"
          enter-to-class="opacity-100 max-h-[3000px]"
          leave-active-class="transition-all duration-200 ease-in overflow-hidden"
          leave-from-class="opacity-100 max-h-[3000px]"
          leave-to-class="opacity-0 max-h-0"
        >
          <div
            v-if="openedChapter === c.id"
            class="px-5 pb-5 pt-1 border-t border-black/[0.05] dark:border-white/[0.06]"
          >
            <!-- 章节内容（按 id 分支） -->
            <div class="prose-content text-[13.5px] leading-relaxed text-gray-700 dark:text-gray-300 space-y-3 pt-3">
              <!-- 1. 模式选择 -->
              <template v-if="c.id === 'mode-choice'">
                <p>本工具支持 <b>两种核心使用模式</b>：</p>
                <div class="rounded-xl border border-emerald-500/15 bg-emerald-500/[0.06] p-3">
                  <div class="font-bold text-emerald-700 dark:text-emerald-300 mb-1">
                    🅰 IDE 直接切号（简单）
                  </div>
                  <p class="text-[12.5px] text-gray-600 dark:text-gray-400">
                    在 Accounts 页面点账号卡片的 <code>🔄</code> 切换按钮，会调
                    <code>SwitchAccountLocal</code> 直接把账号信息写入 Windsurf
                    本地登录文件（<code>state.vscdb</code> 等）。<b>无需 MITM 代理</b>。
                    适合：偶尔切号、不在意计费透明性的用户。
                  </p>
                </div>
                <div class="rounded-xl border border-ios-blue/15 bg-ios-blue/[0.06] p-3">
                  <div class="font-bold text-ios-blue mb-1">
                    🅱 MITM 代理（推荐 / 高级）
                  </div>
                  <p class="text-[12.5px] text-gray-600 dark:text-gray-400">
                    安装 CA 证书 + Hosts 劫持后，MITM 截获所有 Cascade 请求，<b>透明地把请求里的账号身份替换为号池里的 key</b>，
                    上游按号池账号计费。优势：① 永远显示同一登录账号（IDE 不感知）；
                    ② 额度耗尽自动切下一席；③ 同一会话粘性，不会失败；④ Pin /
                    轮换池等高级控制。
                  </p>
                </div>
                <p class="mt-3">
                  <b>👉 推荐</b>：长期使用选 MITM；偶尔玩玩用 A。两个模式可以共存，互不干扰。
                </p>
              </template>

              <!-- 2. API Key -->
              <template v-else-if="c.id === 'api-key'">
                <p>Windsurf 用 <code class="font-mono">sk-ws-</code> 前缀的字符串作为 API Key（类似 OpenAI 的 <code>sk-</code>）。</p>
                <p class="font-bold mt-3">如何获取？</p>
                <ul class="list-disc pl-5 space-y-1.5 text-[12.5px]">
                  <li>登录 windsurf.com 后 → 个人设置 → 复制 API Key</li>
                  <li>已有账号：从浏览器 Network 抓 `api_key` 字段</li>
                  <li>邮箱密码登录后：本工具自动 RegisterUser 生成 API Key（导入流程）</li>
                </ul>
                <p class="font-bold mt-3">为什么 MITM 用它？</p>
                <p class="text-[12.5px]">
                  API Key 是 Windsurf 服务端识别账号身份的核心。MITM 把
                  Authorization header + protobuf body 里的 key 全部替换成号池里某个账号的
                  key → 上游就按那个账号计费。
                </p>
              </template>

              <!-- 3. 导入 -->
              <template v-else-if="c.id === 'import'">
                <p>批量导入支持 <b>4 种凭证</b> 自动识别（混合粘贴也 OK）：</p>
                <ul class="list-disc pl-5 space-y-1.5 text-[12.5px]">
                  <li><b>API Key</b> — 以 <code>sk-ws-</code> 开头，一行一个</li>
                  <li><b>JWT</b> — 以 <code>eyJ</code> 开头的 base64 双段，本工具会调 RegisterUser 自动配 API Key</li>
                  <li><b>邮箱密码</b> — <code>email@x.com password123</code> / JSON / <code>----</code> 分隔 / 卡密格式都支持</li>
                  <li><b>Refresh Token</b> — Firebase refresh token，自动 refresh 拿 JWT</li>
                </ul>
                <p class="mt-3 text-[12.5px]">
                  导入器会自动去重（同 email / JWT / refresh token 不会重复进库）。
                  错误的 / 短的输入会标 <code>X 行未识别</code>，<b>不会盲目提交后端</b>。
                </p>
                <p class="mt-3 text-[12.5px] text-amber-700 dark:text-amber-300">
                  ⚠ 导入并发数默认 3，调高容易触发上游限速（429）。
                </p>
              </template>

              <!-- 4. MITM -->
              <template v-else-if="c.id === 'mitm'">
                <p>MITM = Man-in-the-Middle 代理。本工具实现的是 <b>HTTPS MITM</b>：</p>
                <ol class="list-decimal pl-5 space-y-1.5 text-[12.5px]">
                  <li>生成本地 CA 证书 → 安装到系统信任链</li>
                  <li>劫持 Hosts，让 <code>server.codeium.com</code> 解析到 127.0.0.1</li>
                  <li>本机 443 端口跑 HTTPS 服务器，用 CA 现签目标域名证书</li>
                  <li>截获 Cascade / Windsurf 全部请求 → 修改 protobuf 字段（替换 API Key / JWT / F20 UserID / F32 TeamID）</li>
                  <li>转发到真实 Windsurf 服务器，<b>上游按号池账号计费</b>，IDE 完全不感知</li>
                </ol>
                <p class="mt-3 font-bold">安全性？</p>
                <ul class="list-disc pl-5 space-y-1 text-[12.5px]">
                  <li>CA 证书仅本机有效，<b>不会上传到任何第三方</b></li>
                  <li>对话内容 + 凭证全部在本地处理</li>
                  <li>关闭 MITM 后 CA 仍在系统里，不影响其它 HTTPS 站点</li>
                  <li>卸载：Dashboard 提供「卸载 CA」「卸载 Hosts」分步按钮</li>
                </ul>
              </template>

              <!-- 5. 号池轮换 -->
              <template v-else-if="c.id === 'rotation'">
                <p>开启 MITM 后，号池<b>自动轮换</b>由 3 个触发点驱动：</p>
                <ol class="list-decimal pl-5 space-y-1.5 text-[12.5px]">
                  <li><b>额度耗尽 (onKeyExhausted)</b> — 上游返回 quota exceeded → 立刻切下一个有额度的 key</li>
                  <li><b>限速 (rate-limit)</b> — 上游 429 → 切号 + 冷却原账号</li>
                  <li><b>热轮询 (quota poll)</b> — 默认 12 秒拉一次当前账号额度，发现见底主动切</li>
                </ol>
                <p class="font-bold mt-3">手动锁定 (Pin)</p>
                <p class="text-[12.5px]">
                  在 Accounts 卡片点 ArrowRightLeft 切到某账号后，<b>自动锁定 (🔒)</b>。
                  3 个自动切都跳过，用户 100% 控制。Header / Account 卡片 / Settings 都能解锁。
                </p>
                <p class="font-bold mt-3">轮换池 (Rotation Pool)</p>
                <p class="text-[12.5px]">
                  Settings 里勾 2+ 个账号进池，开启后 <b>定时切 + 额度耗尽双触发</b> 只在池内来回切，池外账号完全不参与。
                  池内额度 1 分钟刷一次（独立于全局 quota refresh）。
                </p>
              </template>

              <!-- 6. Clash -->
              <template v-else-if="c.id === 'clash'">
                <p>大量请求经同一 IP 容易被 Windsurf 服务端打风控。<b>Clash IP 轮换</b> 通过你本地装的 Clash / Mihomo 客户端定期切换出口节点：</p>
                <div class="rounded-xl border border-violet-500/15 bg-violet-500/[0.06] p-3 my-3">
                  <div class="font-bold text-violet-700 dark:text-violet-300 mb-1.5">
                    一键智能启用（v1.1.0+）
                  </div>
                  <p class="text-[12.5px] text-gray-700 dark:text-gray-300">
                    Settings → Clash IP 轮换 → 填控制器 URL (Mihomo / Clash 默认
                    <code>http://127.0.0.1:9090</code>;Verge 默认 <code>9097</code>) +
                    secret → 点 <b>「✨ 智能启用」</b>。后端会：
                  </p>
                  <ol class="list-decimal pl-5 mt-1.5 text-[12px] text-gray-600 dark:text-gray-400 space-y-0.5">
                    <li>探活控制器</li>
                    <li>自动挑节点最多的 selector group</li>
                    <li>type-aware 过滤排除假节点（"剩余流量"/"套餐到期"/"防失联"）</li>
                    <li>立即切一次 + 启动定时切</li>
                  </ol>
                </div>
              </template>

              <!-- 7. 破限 -->
              <template v-else-if="c.id === 'jailbreak'">
                <p><b>Cascade 破限注入</b> = 在每次 chat 请求的 F2 system prompt 末尾追加一段「override 指令」，覆盖模型 alignment / 拒绝模板。</p>
                <p class="text-[12.5px] mt-2">
                  等效于 Claude Code 的 <code>--append-system-prompt-file</code>，但走
                  <b>MITM 协议层</b>，IDE 升级不受影响，关 MITM 即恢复原状。
                </p>
                <p class="font-bold mt-3">4 个预设对比</p>
                <table class="w-full text-[12px] border-collapse mt-1.5">
                  <thead>
                    <tr class="border-b border-black/10 dark:border-white/10">
                      <th class="text-left py-1.5 pr-2">预设</th>
                      <th class="text-left py-1.5 px-2">风险</th>
                      <th class="text-left py-1.5 pl-2">说明</th>
                    </tr>
                  </thead>
                  <tbody class="text-[11.5px]">
                    <tr class="border-b border-black/[0.05] dark:border-white/[0.05]">
                      <td class="py-1.5 pr-2"><b>极简</b></td>
                      <td class="py-1.5 px-2 text-emerald-600">低</td>
                      <td class="py-1.5 pl-2">只压拒绝口径，不踩 Anthropic 网关</td>
                    </tr>
                    <tr class="border-b border-black/[0.05] dark:border-white/[0.05]">
                      <td class="py-1.5 pr-2"><b>软版</b></td>
                      <td class="py-1.5 px-2 text-amber-600">中</td>
                      <td class="py-1.5 pl-2">去 cyber 关键词 + 保留 OVERRIDE 优先级</td>
                    </tr>
                    <tr class="border-b border-black/[0.05] dark:border-white/[0.05]">
                      <td class="py-1.5 pr-2"><b>原版</b></td>
                      <td class="py-1.5 px-2 text-rose-600">高</td>
                      <td class="py-1.5 pl-2">含 malware/exploit/RAT 完整白名单，⚠️ 必触 Anthropic cyber-policy 拒绝</td>
                    </tr>
                    <tr>
                      <td class="py-1.5 pr-2"><b>自定义</b></td>
                      <td class="py-1.5 px-2 text-gray-500">取决</td>
                      <td class="py-1.5 pl-2">用 textarea 自己改</td>
                    </tr>
                  </tbody>
                </table>
                <p class="mt-3 text-[12px] text-amber-700 dark:text-amber-300">
                  ⚠ 注入文本不会上传第三方，仅在本机 MITM 拦截阶段附加到请求体。
                  仅供本地实验/学术研究使用。
                </p>
              </template>
            </div>
          </div>
        </Transition>
      </article>

      <div
        v-if="filteredChapters.length === 0"
        class="py-12 text-center text-gray-500 dark:text-gray-400"
      >
        没找到匹配的章节 — 试试别的关键词
      </div>
    </div>

    <!-- 提示 -->
    <div
      class="mt-8 rounded-[16px] border border-ios-blue/15 bg-ios-blue/[0.06] p-4 text-[12.5px] text-ios-blue dark:text-blue-300 flex items-start gap-2"
    >
      <HelpCircle class="w-5 h-5 shrink-0 mt-0.5" stroke-width="2.3" />
      <div class="leading-relaxed">
        还有疑问？打开 <b>「关于」</b> 页面加作者微信或微信群提问；或在 GitHub 仓库提 issue。
      </div>
    </div>
  </div>
</template>

<style scoped>
.prose-content :deep(code) {
  background: rgba(0, 0, 0, 0.06);
  padding: 0.1em 0.4em;
  border-radius: 4px;
  font-family: ui-monospace, "SF Mono", "Cascadia Mono", Menlo, monospace;
  font-size: 0.9em;
}
.dark .prose-content :deep(code) {
  background: rgba(255, 255, 255, 0.08);
}
.prose-content :deep(b) {
  font-weight: 700;
}
</style>
