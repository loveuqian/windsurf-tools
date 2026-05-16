<script setup lang="ts">
import { computed, ref } from "vue";
import {
  Github,
  Heart,
  MessageSquare,
  ShieldCheck,
  UserCircle2,
  Users,
  X,
  ZoomIn,
} from "lucide-vue-next";
import { APP_PRODUCT_NAME, APP_PRODUCT_TAGLINE } from "../utils/appMode";
import { APP_VERSION, REPO_URL } from "../utils/appMeta";
import LegalDisclaimer from "../components/LegalDisclaimer.vue";
import { showToast } from "../utils/toast";

// 三张作者 QR 码（vite 自动 hash）
import authorWechatImg from "../assets/contact/author-wechat.jpg";
import sponsorQrImg from "../assets/contact/sponsor-qr.jpg";
import wechatGroup1 from "../assets/contact/wechat-group-1.jpg";
import wechatGroup2 from "../assets/contact/wechat-group-2.jpg";
import wechatGroup3 from "../assets/contact/wechat-group-3.jpg";

// 微信群轮播（3 张图，用户加群满了可以切下一张）
const groupImages = [wechatGroup1, wechatGroup2, wechatGroup3];
const groupIndex = ref(0);
const currentGroupImg = computed(() => groupImages[groupIndex.value]);
const cycleGroup = () => {
  groupIndex.value = (groupIndex.value + 1) % groupImages.length;
};

// 二维码点击放大
const lightboxImg = ref<string | null>(null);
const lightboxLabel = ref("");
const openLightbox = (img: string, label: string) => {
  lightboxImg.value = img;
  lightboxLabel.value = label;
};

const copyWechatID = async () => {
  try {
    await navigator.clipboard.writeText("Seven77078");
    showToast("已复制微信号: Seven77078", "success");
  } catch {
    showToast("复制失败，请手动记下: Seven77078", "warning");
  }
};

const openGithub = () => window.open(REPO_URL, "_blank", "noopener");
</script>

<template>
  <div class="p-6 md:p-10 max-w-5xl mx-auto w-full pb-12">
    <!-- 顶部：app logo + 版本 + 标语 -->
    <header class="flex flex-col items-center text-center mb-10">
      <div
        class="w-24 h-24 rounded-ios-card bg-gradient-to-br from-ios-blue to-violet-500 text-white flex items-center justify-center shadow-[0_16px_40px_rgba(37,99,235,0.32)] mb-5"
      >
        <ShieldCheck class="h-12 w-12" stroke-width="2.4" />
      </div>
      <h1 class="text-[32px] font-bold text-ios-text dark:text-ios-textDark tracking-tight">
        {{ APP_PRODUCT_NAME }}
      </h1>
      <p class="mt-2 text-[14px] text-gray-500 dark:text-gray-400 font-medium">
        v{{ APP_VERSION }} · {{ APP_PRODUCT_TAGLINE }}
      </p>
      <div class="mt-4 flex gap-2 flex-wrap justify-center">
        <button
          type="button"
          class="no-drag-region inline-flex items-center gap-2 px-4 py-2 rounded-full bg-gray-100 dark:bg-white/[0.08] hover:bg-gray-200 dark:hover:bg-white/[0.12] text-[13px] font-bold text-gray-800 dark:text-gray-200 transition-all ios-btn"
          @click="openGithub"
        >
          <Github class="w-4 h-4" stroke-width="2.4" />
          仓库 / Issues
        </button>
        <button
          type="button"
          class="no-drag-region inline-flex items-center gap-2 px-4 py-2 rounded-full bg-ios-blue/10 hover:bg-ios-blue/15 text-[13px] font-bold text-ios-blue transition-all ios-btn"
          @click="copyWechatID"
        >
          <MessageSquare class="w-4 h-4" stroke-width="2.4" />
          复制微信号 Seven77078
        </button>
      </div>
    </header>

    <!-- 3 张 QR 码 -->
    <section class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
      <!-- 作者微信 -->
      <div
        class="rounded-[22px] border border-black/[0.05] dark:border-white/[0.08] bg-white/70 dark:bg-white/[0.04] p-5 flex flex-col items-center text-center"
      >
        <div
          class="w-10 h-10 rounded-2xl bg-ios-blue/15 text-ios-blue flex items-center justify-center mb-3"
        >
          <UserCircle2 class="h-5 w-5" stroke-width="2.4" />
        </div>
        <h3 class="text-[15px] font-bold text-ios-text dark:text-ios-textDark mb-1">
          作者微信
        </h3>
        <p class="text-[11.5px] text-gray-500 dark:text-gray-400 mb-3">
          技术交流 / 反馈 / 协作
        </p>
        <button
          type="button"
          class="no-drag-region group relative w-full max-w-[200px] rounded-ios-block overflow-hidden bg-white shadow-md ring-1 ring-black/[0.05] transition-all ios-btn hover:shadow-lg"
          @click="openLightbox(authorWechatImg, '作者微信 — Seven')"
        >
          <img
            :src="authorWechatImg"
            alt="作者微信 QR"
            class="w-full h-auto block"
            loading="lazy"
          />
          <div
            class="absolute inset-0 flex items-center justify-center bg-black/0 group-hover:bg-black/30 transition-colors"
          >
            <ZoomIn
              class="w-6 h-6 text-white opacity-0 group-hover:opacity-100 transition-opacity"
              stroke-width="2.5"
            />
          </div>
        </button>
        <div class="mt-3 text-[12px] font-mono font-bold text-gray-700 dark:text-gray-300">
          Seven77078
        </div>
      </div>

      <!-- 赞赏支持 -->
      <div
        class="rounded-[22px] border border-amber-500/20 bg-amber-50/60 dark:bg-amber-950/30 p-5 flex flex-col items-center text-center"
      >
        <div
          class="w-10 h-10 rounded-2xl bg-amber-500/20 text-amber-700 dark:text-amber-300 flex items-center justify-center mb-3"
        >
          <Heart class="h-5 w-5" stroke-width="2.4" />
        </div>
        <h3 class="text-[15px] font-bold text-amber-900 dark:text-amber-200 mb-1">
          赞赏支持
        </h3>
        <p class="text-[11.5px] text-amber-700/80 dark:text-amber-300/80 mb-3">
          请作者喝杯咖啡 ☕
        </p>
        <button
          type="button"
          class="no-drag-region group relative w-full max-w-[200px] rounded-ios-block overflow-hidden bg-white shadow-md ring-1 ring-black/[0.05] transition-all ios-btn hover:shadow-lg"
          @click="openLightbox(sponsorQrImg, '赞赏码 — 给 Seven 赞赏')"
        >
          <img
            :src="sponsorQrImg"
            alt="赞赏码"
            class="w-full h-auto block"
            loading="lazy"
          />
          <div
            class="absolute inset-0 flex items-center justify-center bg-black/0 group-hover:bg-black/30 transition-colors"
          >
            <ZoomIn
              class="w-6 h-6 text-white opacity-0 group-hover:opacity-100 transition-opacity"
              stroke-width="2.5"
            />
          </div>
        </button>
        <div class="mt-3 text-[11px] text-amber-700/70 dark:text-amber-300/70">
          完全自愿，不影响任何功能
        </div>
      </div>

      <!-- 微信交流群 -->
      <div
        class="rounded-[22px] border border-emerald-500/20 bg-emerald-50/60 dark:bg-emerald-950/30 p-5 flex flex-col items-center text-center"
      >
        <div
          class="w-10 h-10 rounded-2xl bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 flex items-center justify-center mb-3"
        >
          <Users class="h-5 w-5" stroke-width="2.4" />
        </div>
        <h3 class="text-[15px] font-bold text-emerald-900 dark:text-emerald-200 mb-1">
          微信交流群
        </h3>
        <p class="text-[11.5px] text-emerald-700/80 dark:text-emerald-300/80 mb-3">
          一群满了点切换下一群 ({{ groupIndex + 1 }}/{{ groupImages.length }})
        </p>
        <button
          type="button"
          class="no-drag-region group relative w-full max-w-[200px] rounded-ios-block overflow-hidden bg-white shadow-md ring-1 ring-black/[0.05] transition-all ios-btn hover:shadow-lg"
          @click="openLightbox(currentGroupImg, `微信群 ${groupIndex + 1}`)"
        >
          <img
            :src="currentGroupImg"
            alt="微信群 QR"
            class="w-full h-auto block"
            loading="lazy"
          />
          <div
            class="absolute inset-0 flex items-center justify-center bg-black/0 group-hover:bg-black/30 transition-colors"
          >
            <ZoomIn
              class="w-6 h-6 text-white opacity-0 group-hover:opacity-100 transition-opacity"
              stroke-width="2.5"
            />
          </div>
        </button>
        <button
          type="button"
          class="no-drag-region mt-3 text-[12px] font-bold text-emerald-700 dark:text-emerald-300 hover:underline"
          @click="cycleGroup"
        >
          切换下一群 →
        </button>
      </div>
    </section>

    <!-- 法律免责声明 -->
    <LegalDisclaimer />

    <!-- 致谢 -->
    <section class="mt-8 text-center text-[12px] text-gray-500 dark:text-gray-500 leading-relaxed">
      Made with ❤️ by Seven
      <br />
      Built with <a :href="'https://wails.io/'" target="_blank" rel="noopener" class="text-ios-blue hover:underline">Wails v2</a> ·
      <a :href="'https://vuejs.org/'" target="_blank" rel="noopener" class="text-ios-blue hover:underline">Vue 3</a> ·
      <a :href="'https://tailwindcss.com/'" target="_blank" rel="noopener" class="text-ios-blue hover:underline">TailwindCSS</a>
    </section>

    <!-- 二维码放大 Lightbox -->
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
          v-if="lightboxImg"
          class="fixed inset-0 z-[300] flex items-center justify-center bg-black/85 backdrop-blur-md p-6 cursor-pointer"
          @click="lightboxImg = null"
        >
          <div class="relative flex flex-col items-center max-w-md w-full">
            <button
              type="button"
              class="absolute -top-12 right-0 flex h-9 w-9 items-center justify-center rounded-full bg-white/10 hover:bg-white/20 text-white transition-colors"
              @click.stop="lightboxImg = null"
            >
              <X class="h-4 w-4" stroke-width="2.5" />
            </button>
            <img
              :src="lightboxImg"
              :alt="lightboxLabel"
              class="w-full h-auto rounded-[20px] shadow-2xl"
            />
            <p class="mt-4 text-[14px] font-bold text-white/90 text-center">
              {{ lightboxLabel }}
            </p>
            <p class="mt-1 text-[11px] text-white/50 text-center">
              点击任意处关闭 · 用微信扫描二维码
            </p>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
