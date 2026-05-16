import { useEffect, useMemo, useState } from "react";
import {
  Copy,
  Globe,
  Info,
  Play,
  RefreshCw,
  Shield,
  Terminal,
  Zap,
} from "lucide-react";
import IToggle from "../components/ios/IToggle";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton";
import SkeletonOverlay from "../components/common/SkeletonOverlay";
import { APIInfo } from "../api/wails";
import { useMitmStatusStore } from "../stores/useMitmStatusStore";
import { useRelayStatusStore } from "../stores/useRelayStatusStore";
import { useSettingsStore } from "../stores/useSettingsStore";
import { showErrorToast, showToast } from "../utils/toast";

const RELAY_DEFAULT_MODEL = "cascade";

/**
 * Relay — Vue 1:1 完整迁移：OpenAI 兼容 Chat Completions 中转。
 */
export default function Relay() {
  const settings = useSettingsStore((s) => s.settings);
  const settingsHasLoadedOnce = useSettingsStore((s) => s.hasLoadedOnce);
  const mitmStatus = useMitmStatusStore((s) => s.status);
  const mitmHasLoadedOnce = useMitmStatusStore((s) => s.hasLoadedOnce);
  const relayStatus = useRelayStatusStore((s) => s.status);
  const relayHasLoadedOnce = useRelayStatusStore((s) => s.hasLoadedOnce);
  const fetchRelay = useRelayStatusStore((s) => s.fetchStatus);

  const [relayLoading, setRelayLoading] = useState(false);
  const [testResult, setTestResult] = useState("");
  const [testLoading, setTestLoading] = useState(false);

  useEffect(() => {
    void useRelayStatusStore.getState().ensureStatusLoaded();
    void useMitmStatusStore.getState().ensureStatusLoaded();
  }, []);

  const relayPort = settings?.openai_relay_port || 8787;
  const relaySecret = settings?.openai_relay_secret || "";
  const relayURL = relayStatus?.running
    ? relayStatus.url || `http://127.0.0.1:${relayPort}`
    : `http://127.0.0.1:${relayPort}`;
  const endpoint = `${relayURL}/v1/chat/completions`;
  const poolCount = mitmStatus?.pool_status?.length ?? 0;
  const hasPool = poolCount > 0;

  const relayBooting =
    !settingsHasLoadedOnce || !mitmHasLoadedOnce || !relayHasLoadedOnce;

  const handleToggle = async (on: boolean) => {
    setRelayLoading(true);
    try {
      if (on) {
        await APIInfo.startOpenAIRelay(relayPort, relaySecret);
      } else {
        await APIInfo.stopOpenAIRelay();
      }
      await fetchRelay(true);
      showToast(on ? "Relay 已启动" : "Relay 已停止", "success");
    } catch (e) {
      showErrorToast(e, `Relay ${on ? "启动" : "停止"}失败`);
    } finally {
      setRelayLoading(false);
    }
  };

  const copyText = (text: string, label: string) => {
    navigator.clipboard.writeText(text).then(
      () => showToast(`已复制${label}`, "success"),
      () => showToast(`复制${label}失败`, "error"),
    );
  };

  const curlCmd = useMemo(() => {
    const auth = relaySecret ? ` -H "Authorization: Bearer ${relaySecret}"` : "";
    return `curl "${endpoint}"${auth} -H "Content-Type: application/json" -d "{\\"model\\":\\"${RELAY_DEFAULT_MODEL}\\",\\"stream\\":true,\\"messages\\":[{\\"role\\":\\"user\\",\\"content\\":\\"hello\\"}]}"`;
  }, [endpoint, relaySecret]);

  const pythonExample = useMemo(() => {
    const authLine = relaySecret
      ? `    api_key="${relaySecret}",`
      : `    api_key="no-key",`;
    return `from openai import OpenAI

client = OpenAI(
    base_url="${relayURL}/v1",
${authLine}
)

stream = client.chat.completions.create(
    model="${RELAY_DEFAULT_MODEL}",
    messages=[{"role": "user", "content": "hello"}],
    stream=True,
)
for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")`;
  }, [relayURL, relaySecret]);

  const handleTest = async () => {
    if (!relayStatus?.running) {
      showToast("请先启动 Relay", "error");
      return;
    }
    setTestLoading(true);
    setTestResult("");
    try {
      const headers: Record<string, string> = {
        "Content-Type": "application/json",
      };
      if (relaySecret) {
        headers["Authorization"] = `Bearer ${relaySecret}`;
      }
      const resp = await fetch(endpoint, {
        method: "POST",
        headers,
        body: JSON.stringify({
          model: RELAY_DEFAULT_MODEL,
          stream: false,
          messages: [{ role: "user", content: 'Say "hello" in one word.' }],
        }),
      });
      const data = await resp.json();
      if (data.error) {
        setTestResult(
          `❌ 错误: ${data.error.message || JSON.stringify(data.error)}`,
        );
      } else if (data.choices?.[0]?.message?.content !== undefined) {
        setTestResult(
          `✅ 成功: ${data.choices[0].message.content.slice(0, 200) || "(空回复)"}`,
        );
      } else {
        setTestResult(`⚠️ 未知响应: ${JSON.stringify(data).slice(0, 300)}`);
      }
    } catch (e) {
      setTestResult(`❌ 请求失败: ${String(e)}`);
    } finally {
      setTestLoading(false);
    }
  };

  if (relayBooting) {
    return <PageLoadingSkeleton variant="relay" className="w-full" />;
  }

  return (
    <SkeletonOverlay
      active={false}
      label="Relay 刷新中"
      skeleton={<PageLoadingSkeleton variant="relay" className="w-full" />}
    >
      <div className="p-6 md:p-8 max-w-4xl mx-auto w-full pb-12">
        <header className="flex items-center gap-4 mb-6">
          <div className="w-12 h-12 rounded-2xl bg-gradient-to-br from-ios-blue to-violet-500 text-white flex items-center justify-center shadow-[0_10px_24px_rgba(37,99,235,0.24)]">
            <Globe className="h-6 w-6" strokeWidth={2.4} />
          </div>
          <div>
            <h1 className="text-[24px] font-bold text-ios-text dark:text-ios-textDark tracking-tight">
              OpenAI Relay
            </h1>
            <p className="mt-0.5 text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
              本地 OpenAI 兼容 Chat Completions 中转，复用号池
            </p>
          </div>
        </header>

        <div className="space-y-4">
          {/* 状态 + Toggle */}
          <div className="flex items-center justify-between gap-4 rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                <span
                  className={[
                    "h-2.5 w-2.5 rounded-full",
                    relayStatus?.running
                      ? "bg-emerald-400 shadow-[0_0_10px_rgba(52,211,153,0.45)]"
                      : "bg-slate-400 dark:bg-slate-500",
                  ].join(" ")}
                />
                {relayStatus?.running ? "Relay 运行中" : "Relay 未启动"}
              </div>
              <p className="mt-1 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                {relayStatus?.running
                  ? `监听 127.0.0.1:${relayStatus?.port || relayPort}`
                  : "启动后号池中的 API Key 将自动用于对话请求"}
              </p>
            </div>
            <IToggle
              modelValue={Boolean(relayStatus?.running)}
              onValueChange={handleToggle}
              disabled={relayLoading || !hasPool}
            />
          </div>

          {/* 号池为空提示 */}
          {!hasPool ? (
            <div className="rounded-[18px] border border-amber-500/15 bg-amber-500/[0.06] px-4 py-3">
              <div className="flex items-start gap-3">
                <Info
                  className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-300"
                  strokeWidth={2.4}
                />
                <div className="text-[12px] leading-relaxed text-amber-700 dark:text-amber-300">
                  号池为空，请先在「号池」页面通过 <strong>API Key 导入</strong>{" "}
                  添加{" "}
                  <code className="rounded bg-black/5 px-1 dark:bg-white/10">
                    sk-ws-01-...
                  </code>{" "}
                  格式的 Key。Relay 复用 MITM 号池。
                </div>
              </div>
            </div>
          ) : null}

          {/* Endpoint 信息（运行时） */}
          {relayStatus?.running ? (
            <div className="space-y-3">
              <div className="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
                <div className="flex items-center gap-2 mb-3">
                  <Zap className="h-4 w-4 text-violet-500" strokeWidth={2.4} />
                  <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                    接入信息
                  </div>
                </div>

                <div className="space-y-2.5">
                  {/* Base URL */}
                  <div className="flex items-center gap-2">
                    <div className="flex-1 min-w-0 rounded-[14px] border border-black/[0.05] bg-black/[0.02] px-3 py-2.5 dark:border-white/[0.06] dark:bg-white/[0.03]">
                      <div className="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                        Base URL
                      </div>
                      <div className="mt-0.5 truncate font-mono text-[12px] font-semibold text-ios-text dark:text-ios-textDark select-all">
                        {relayURL}/v1
                      </div>
                    </div>
                    <button
                      type="button"
                      className="no-drag-region flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-black/[0.06] bg-white/80 text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05]"
                      title="复制 Base URL"
                      onClick={() => copyText(`${relayURL}/v1`, "Base URL")}
                    >
                      <Copy className="h-3.5 w-3.5" strokeWidth={2.4} />
                    </button>
                  </div>

                  {/* Endpoint */}
                  <div className="flex items-center gap-2">
                    <div className="flex-1 min-w-0 rounded-[14px] border border-emerald-500/15 bg-emerald-500/[0.04] px-3 py-2.5">
                      <div className="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                        Chat Endpoint
                      </div>
                      <div className="mt-0.5 truncate font-mono text-[12px] font-semibold text-ios-text dark:text-ios-textDark select-all">
                        {endpoint}
                      </div>
                    </div>
                    <button
                      type="button"
                      className="no-drag-region flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-black/[0.06] bg-white/80 text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05]"
                      title="复制 Endpoint"
                      onClick={() => copyText(endpoint, "Endpoint")}
                    >
                      <Copy className="h-3.5 w-3.5" strokeWidth={2.4} />
                    </button>
                  </div>

                  {/* API Key */}
                  {relaySecret ? (
                    <div className="flex items-center gap-2">
                      <div className="flex-1 min-w-0 rounded-[14px] border border-black/[0.05] bg-black/[0.02] px-3 py-2.5 dark:border-white/[0.06] dark:bg-white/[0.03]">
                        <div className="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                          API Key (Bearer)
                        </div>
                        <div className="mt-0.5 font-mono text-[12px] text-ios-text dark:text-ios-textDark select-all break-all">
                          {relaySecret}
                        </div>
                      </div>
                      <button
                        type="button"
                        className="no-drag-region flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-black/[0.06] bg-white/80 text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05]"
                        title="复制 API Key"
                        onClick={() => copyText(relaySecret, "API Key")}
                      >
                        <Copy className="h-3.5 w-3.5" strokeWidth={2.4} />
                      </button>
                    </div>
                  ) : (
                    <div className="rounded-[14px] border border-black/[0.05] bg-black/[0.02] px-3 py-2.5 dark:border-white/[0.06] dark:bg-white/[0.03]">
                      <div className="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                        API Key
                      </div>
                      <div className="mt-0.5 text-[11.5px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                        未配置鉴权（任何 API Key 都接受）—
                        在「设置」中可配置 secret 提高安全性。
                      </div>
                    </div>
                  )}

                  {/* 可用模型 */}
                  <div>
                    <div className="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                      可用模型
                    </div>
                    <div className="mt-1 flex flex-wrap gap-1.5">
                      {["cascade", "gpt-4", "gpt-4o", "claude-sonnet-4.6"].map(
                        (m) => (
                          <span
                            key={m}
                            className="rounded-full bg-black/[0.04] px-2 py-0.5 text-[10px] font-bold tracking-wide text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark"
                          >
                            {m}
                          </span>
                        ),
                      )}
                    </div>
                  </div>
                </div>
              </div>

              {/* 快速测试 */}
              <div className="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
                <div className="flex items-center justify-between gap-3 mb-3">
                  <div className="flex items-center gap-2">
                    <Terminal
                      className="h-4 w-4 text-ios-textSecondary dark:text-ios-textSecondaryDark"
                      strokeWidth={2.4}
                    />
                    <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                      快速测试
                    </div>
                  </div>
                  <button
                    type="button"
                    className={[
                      "no-drag-region flex items-center gap-1.5 rounded-[12px] px-3 py-1.5 text-[11px] font-semibold transition-all ios-btn",
                      testLoading
                        ? "bg-slate-500/10 text-slate-500"
                        : "bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/15 dark:text-emerald-300",
                    ].join(" ")}
                    disabled={testLoading}
                    onClick={handleTest}
                  >
                    {testLoading ? (
                      <RefreshCw
                        className="h-3 w-3 animate-spin"
                        strokeWidth={2.4}
                      />
                    ) : (
                      <Play className="h-3 w-3" strokeWidth={2.4} />
                    )}
                    {testLoading ? "测试中..." : "发送测试请求"}
                  </button>
                </div>

                {testResult ? (
                  <div
                    className={[
                      "rounded-[14px] border px-3 py-2.5 text-[12px] font-medium leading-relaxed break-words",
                      testResult.startsWith("✅")
                        ? "border-emerald-500/15 bg-emerald-500/[0.05] text-emerald-700 dark:text-emerald-300"
                        : "border-rose-500/15 bg-rose-500/[0.05] text-rose-700 dark:text-rose-300",
                    ].join(" ")}
                  >
                    {testResult}
                  </div>
                ) : null}

                <div className="mt-3 space-y-2">
                  <button
                    type="button"
                    className="no-drag-region flex w-full items-center justify-center gap-2 rounded-[14px] border border-black/[0.06] bg-white/80 px-3 py-2.5 text-[11px] font-semibold text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textSecondaryDark"
                    onClick={() => copyText(curlCmd, "curl 命令")}
                  >
                    <Copy className="h-3 w-3" strokeWidth={2.4} />
                    复制 curl 命令（Windows CMD）
                  </button>
                  <button
                    type="button"
                    className="no-drag-region flex w-full items-center justify-center gap-2 rounded-[14px] border border-black/[0.06] bg-white/80 px-3 py-2.5 text-[11px] font-semibold text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textSecondaryDark"
                    onClick={() => copyText(pythonExample, "Python 示例")}
                  >
                    <Copy className="h-3 w-3" strokeWidth={2.4} />
                    复制 Python OpenAI SDK 示例
                  </button>
                </div>
              </div>
            </div>
          ) : null}

          {/* 未运行说明 */}
          {!relayStatus?.running ? (
            <div className="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
              <div className="flex items-center gap-2 mb-3">
                <Shield
                  className="h-4 w-4 text-ios-textSecondary dark:text-ios-textSecondaryDark"
                  strokeWidth={2.4}
                />
                <div className="text-[13px] font-bold text-ios-text dark:text-ios-textDark">
                  使用说明
                </div>
              </div>
              <div className="space-y-2.5 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                {[
                  <>
                    在「号池」页面通过{" "}
                    <strong className="text-ios-text dark:text-ios-textDark">
                      API Key 导入
                    </strong>{" "}
                    添加{" "}
                    <code className="rounded bg-black/5 px-1 dark:bg-white/10">
                      sk-ws-01-...
                    </code>{" "}
                    账号
                  </>,
                  "打开上方开关启动 Relay（端口和密钥可在「设置」中修改）",
                  <>
                    将 Base URL{" "}
                    <code className="rounded bg-black/5 px-1 dark:bg-white/10">
                      http://127.0.0.1:{relayPort}/v1
                    </code>{" "}
                    填入你的 OpenAI 客户端
                  </>,
                  "额度耗尽时自动轮转到下一个号池 Key，无需手动切换",
                ].map((item, idx) => (
                  <div key={idx} className="flex gap-2.5">
                    <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-violet-500/10 text-[10px] font-bold text-violet-600 dark:text-violet-300">
                      {idx + 1}
                    </span>
                    <span>{item}</span>
                  </div>
                ))}
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </SkeletonOverlay>
  );
}
