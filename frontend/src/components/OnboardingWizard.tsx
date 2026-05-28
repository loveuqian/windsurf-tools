import { useState } from "react";
import {
  AlertTriangle,
  ChevronRight,
  Database,
  Rocket,
  ShieldCheck,
  Sparkles,
} from "lucide-react";

/**
 * OnboardingWizard — 首次启动全屏分步向导。
 *
 * 只在第一次打开(localStorage 无 ONBOARDING_KEY)时出现,看完写标志,之后不再弹。
 * 三步:欢迎(大白话说清这软件干嘛)→ 三步上手 → 法律声明勾选同意。
 *
 * 设计目标:把原本散落在 Dashboard 术语堆里的"第一步做什么"前置成全屏聚焦引导,
 * 让完全不懂 MITM/号池的新手也能一眼明白价值 + 知道接下来点哪。
 */

const ONBOARDING_KEY = "wt_onboarding_done_v1";

export function shouldShowOnboarding(): boolean {
  try {
    return localStorage.getItem(ONBOARDING_KEY) !== "1";
  } catch {
    return false; // localStorage 不可用时不打扰
  }
}

function markOnboardingDone() {
  try {
    localStorage.setItem(ONBOARDING_KEY, "1");
  } catch {
    /* ignore */
  }
}

interface Props {
  /** 看完向导后回调:done=用户点了「开始导入账号」希望直接进导入流程 */
  onClose: (goImport: boolean) => void;
}

export default function OnboardingWizard({ onClose }: Props) {
  const [step, setStep] = useState(0); // 0 欢迎 / 1 三步 / 2 声明
  const [agreed, setAgreed] = useState(false);

  const finish = (goImport: boolean) => {
    markOnboardingDone();
    onClose(goImport);
  };

  return (
    <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/45 backdrop-blur-md p-4">
      <div className="w-full max-w-[560px] rounded-[26px] border border-black/[0.06] bg-white dark:border-white/[0.08] dark:bg-[#1C1C1E] shadow-[0_24px_60px_rgba(15,23,42,0.35)] overflow-hidden ios-page-enter">
        {/* 顶部进度点 */}
        <div className="flex items-center justify-center gap-2 pt-5">
          {[0, 1, 2].map((i) => (
            <span
              key={i}
              className={[
                "h-1.5 rounded-full transition-all",
                i === step
                  ? "w-6 bg-ios-blue"
                  : "w-1.5 bg-black/15 dark:bg-white/20",
              ].join(" ")}
            />
          ))}
        </div>

        <div className="px-7 py-6">
          {step === 0 && <WelcomeStep />}
          {step === 1 && <StepsStep />}
          {step === 2 && <DisclaimerStep agreed={agreed} setAgreed={setAgreed} />}
        </div>

        {/* 底部按钮 */}
        <div className="flex items-center justify-between gap-3 border-t border-black/[0.06] dark:border-white/[0.08] px-7 py-4">
          {step === 0 ? (
            <button
              type="button"
              onClick={() => finish(false)}
              className="no-drag-region text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark hover:text-ios-text dark:hover:text-ios-textDark transition-colors"
            >
              跳过引导
            </button>
          ) : (
            <button
              type="button"
              onClick={() => setStep((s) => s - 1)}
              className="no-drag-region text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark hover:text-ios-text dark:hover:text-ios-textDark transition-colors"
            >
              上一步
            </button>
          )}

          {step < 2 ? (
            <button
              type="button"
              onClick={() => setStep((s) => s + 1)}
              className="no-drag-region inline-flex items-center gap-1.5 rounded-full bg-ios-blue px-5 py-2 text-[13px] font-bold text-white shadow-md shadow-ios-blue/30 ios-btn hover:bg-ios-blue/90 transition-colors"
            >
              下一步
              <ChevronRight className="h-4 w-4" strokeWidth={2.6} />
            </button>
          ) : (
            <button
              type="button"
              disabled={!agreed}
              onClick={() => finish(true)}
              className={[
                "no-drag-region inline-flex items-center gap-1.5 rounded-full px-5 py-2 text-[13px] font-bold text-white shadow-md transition-all",
                agreed
                  ? "bg-emerald-500 shadow-emerald-500/30 ios-btn hover:bg-emerald-600"
                  : "bg-gray-300 dark:bg-white/15 cursor-not-allowed",
              ].join(" ")}
            >
              <Rocket className="h-4 w-4" strokeWidth={2.4} />
              同意并开始导入账号
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function WelcomeStep() {
  return (
    <div className="text-center">
      <div className="mx-auto mb-5 flex h-20 w-20 items-center justify-center rounded-[24px] bg-gradient-to-br from-ios-blue/15 to-violet-500/15 dark:from-ios-blue/25 dark:to-violet-500/25 shadow-[0_12px_28px_rgba(37,99,235,0.18)]">
        <Sparkles className="h-10 w-10 text-ios-blue dark:text-blue-300" strokeWidth={1.8} />
      </div>
      <h2 className="mb-3 text-[22px] font-bold text-ios-text dark:text-ios-textDark">
        欢迎使用 Windsurf Tools
      </h2>
      <p className="mx-auto max-w-[420px] text-[14px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
        一句话说清它干嘛：
        <b className="text-ios-text dark:text-ios-textDark">
          把你的多个 Windsurf 账号放进一个「号池」，
        </b>
        在本机后台拦截 Windsurf 的请求，
        <b className="text-ios-text dark:text-ios-textDark">
          额度用完自动换下一个账号
        </b>
        ，你在 IDE 里照常聊天、完全无感。
      </p>
      <p className="mx-auto mt-4 max-w-[420px] rounded-2xl bg-black/[0.03] dark:bg-white/[0.05] px-4 py-3 text-[12.5px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
        全程<b>只在你自己的电脑上运行</b>，不上传任何账号、对话或密钥到外部服务器。
      </p>
    </div>
  );
}

function StepsStep() {
  const steps = [
    {
      icon: Database,
      tint: "text-ios-blue dark:text-blue-300 bg-ios-blue/12",
      title: "导入账号",
      desc: "粘贴账号(API Key / 邮箱密码等都行)，自动识别并入池。只有一个账号也能用。",
    },
    {
      icon: ShieldCheck,
      tint: "text-violet-600 dark:text-violet-300 bg-violet-500/12",
      title: "开启代理",
      desc: "在「总览」页一键完成证书 + 配置并打开代理。期间会让你输一次电脑开机密码，这是安装本地证书的正常步骤。",
    },
    {
      icon: Rocket,
      tint: "text-emerald-600 dark:text-emerald-300 bg-emerald-500/12",
      title: "回 IDE 用",
      desc: "打开 / 重启 Windsurf，照常对话即可。额度用完时本工具会在后台自动换号，你不会被打断。",
    },
  ];
  return (
    <div>
      <h2 className="mb-1 text-center text-[20px] font-bold text-ios-text dark:text-ios-textDark">
        三步开始
      </h2>
      <p className="mb-5 text-center text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
        整个过程不到一分钟
      </p>
      <div className="space-y-3">
        {steps.map((s, i) => (
          <div
            key={i}
            className="flex items-start gap-3.5 rounded-[18px] border border-black/[0.05] dark:border-white/[0.06] bg-black/[0.015] dark:bg-white/[0.03] p-4"
          >
            <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl ${s.tint}`}>
              <s.icon className="h-5 w-5" strokeWidth={2.1} />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="flex h-5 w-5 items-center justify-center rounded-full bg-ios-text/80 dark:bg-white/80 text-[11px] font-black text-white dark:text-black">
                  {i + 1}
                </span>
                <span className="text-[14.5px] font-bold text-ios-text dark:text-ios-textDark">
                  {s.title}
                </span>
              </div>
              <p className="mt-1 text-[12.5px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                {s.desc}
              </p>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function DisclaimerStep({
  agreed,
  setAgreed,
}: {
  agreed: boolean;
  setAgreed: (v: boolean) => void;
}) {
  return (
    <div>
      <div className="mb-4 flex items-center gap-2.5">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-amber-500/15 text-amber-600 dark:text-amber-300">
          <AlertTriangle className="h-5 w-5" strokeWidth={2.3} />
        </div>
        <h2 className="text-[18px] font-bold text-ios-text dark:text-ios-textDark">
          开始前请知悉
        </h2>
      </div>
      <ul className="space-y-2.5 text-[13px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
        <li className="flex gap-2">
          <span className="text-amber-500">•</span>
          <span>
            本工具仅供
            <b className="text-ios-text dark:text-ios-textDark">学习研究与技术交流</b>
            ，禁止商用、转售或任何非法用途。
          </span>
        </li>
        <li className="flex gap-2">
          <span className="text-amber-500">•</span>
          <span>
            使用风险<b className="text-ios-text dark:text-ios-textDark">自负</b>
            （含账号封禁、订阅吊销等），作者不承担任何法律责任。
          </span>
        </li>
        <li className="flex gap-2">
          <span className="text-amber-500">•</span>
          <span>请保证导入的账号通过合法途径获得，使用方式符合相关服务条款。</span>
        </li>
        <li className="flex gap-2">
          <span className="text-emerald-500">•</span>
          <span>
            所有数据
            <b className="text-ios-text dark:text-ios-textDark">仅存本地</b>
            ，不会上传到任何第三方服务器。完整声明见「关于」页。
          </span>
        </li>
      </ul>

      <label className="mt-5 flex cursor-pointer items-center gap-2.5 rounded-2xl border border-black/[0.06] dark:border-white/[0.08] bg-black/[0.02] dark:bg-white/[0.04] px-4 py-3 select-none">
        <input
          type="checkbox"
          checked={agreed}
          onChange={(e) => setAgreed(e.target.checked)}
          className="h-4 w-4 accent-emerald-500"
        />
        <span className="text-[13px] font-semibold text-ios-text dark:text-ios-textDark">
          我已阅读并同意上述条款
        </span>
      </label>
    </div>
  );
}
