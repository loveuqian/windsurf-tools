import { useEffect, useRef, useState } from "react";
import { Settings as SettingsIcon, ShieldCheck } from "lucide-react";
import { APIInfo } from "../api/wails";
import IAutoSaveIndicator from "../components/ios/IAutoSaveIndicator";
import INumberStepper from "../components/ios/INumberStepper";
import ISettingRow from "../components/ios/ISettingRow";
import IToggle from "../components/ios/IToggle";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton";
import { useSettingsStore } from "../stores/useSettingsStore";
import {
  formToSettings,
  quotaPolicyOptions,
  settingsToForm,
  switchPlanFilterOptions,
  type SettingsForm,
} from "../utils/settingsModel";
import { showErrorToast, showToast } from "../utils/toast";

const AUTOSAVE_DEBOUNCE_MS = 500;

type SaveState = "idle" | "saving" | "saved" | "error";

/**
 * Settings — Vue 1:1 完整字段迁移；UI 紧凑（统一用 ISettingRow），自动保存防抖 500ms。
 *
 * 5 个分组：基础（导入并发 / 静态缓存）/ 自动切号 / Pin & 轮换池 / Clash IP 轮换 /
 * 破限注入 / 桌面行为 / 调试 / 配置导入导出 / F7 卡片（作者自用）。
 */
export default function Settings() {
  const settings = useSettingsStore((s) => s.settings);
  const isLoading = useSettingsStore((s) => s.isLoading);
  const hasLoadedOnce = useSettingsStore((s) => s.hasLoadedOnce);

  const [form, setForm] = useState<SettingsForm | null>(null);
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [saveError, setSaveError] = useState("");

  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const saveStateResetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const formRef = useRef<SettingsForm | null>(null);
  const savingRef = useRef(false);
  const pendingSaveRef = useRef(false);

  // 初次加载 settings → 转 form
  useEffect(() => {
    void useSettingsStore.getState().fetchSettings();
  }, []);

  useEffect(() => {
    if (settings) {
      const next = settingsToForm(settings);
      setForm((prev) => {
        if (savingRef.current || pendingSaveRef.current) {
          if (!prev) {
            formRef.current = next;
            return next;
          }
          return prev;
        }
        formRef.current = next;
        return next;
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settings]);

  const flushSave = async () => {
    if (saveTimer.current) {
      clearTimeout(saveTimer.current);
      saveTimer.current = null;
    }
    const cur = formRef.current;
    if (!cur) return;
    if (savingRef.current) {
      pendingSaveRef.current = true;
      return;
    }
    savingRef.current = true;
    pendingSaveRef.current = false;
    setSaveState("saving");
    try {
      await APIInfo.updateSettings(formToSettings(cur));
      await useSettingsStore.getState().fetchSettings(true);
      setSaveState("saved");
      if (saveStateResetTimer.current) clearTimeout(saveStateResetTimer.current);
      saveStateResetTimer.current = setTimeout(
        () => setSaveState("idle"),
        2200,
      );
    } catch (e) {
      setSaveState("error");
      setSaveError(String(e));
    } finally {
      savingRef.current = false;
      if (pendingSaveRef.current) {
        void flushSave();
      }
    }
  };

  // patch + 防抖触发自动保存
  const patch = (delta: Partial<SettingsForm>) => {
    pendingSaveRef.current = true;
    setForm((prev) => {
      if (!prev) return prev;
      const next = { ...prev, ...delta };
      formRef.current = next;
      return next;
    });
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      void flushSave();
    }, AUTOSAVE_DEBOUNCE_MS);
  };

  useEffect(() => {
    return () => {
      if (saveTimer.current) clearTimeout(saveTimer.current);
      if (saveStateResetTimer.current)
        clearTimeout(saveStateResetTimer.current);
      // 切走时立刻 flush
      if (formRef.current) void flushSave();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleExport = async () => {
    try {
      const path = await APIInfo.exportSettings();
      showToast(`配置已导出到：\n${path}`, "success", 6000);
    } catch (e) {
      showErrorToast(e, "导出配置失败");
    }
  };

  const handleImport = async () => {
    const text = window.prompt(
      "粘贴 settings.json 全文（导出文件的内容）以导入配置",
      "",
    );
    if (!text || !text.trim()) return;
    try {
      await APIInfo.importSettings(text);
      await useSettingsStore.getState().fetchSettings(true);
      const latest = useSettingsStore.getState().settings;
      if (latest) {
        const next = settingsToForm(latest);
        formRef.current = next;
        setForm(next);
      }
      showToast("配置已导入并应用", "success");
    } catch (e) {
      showErrorToast(e, "导入配置失败");
    }
  };

  if (!hasLoadedOnce && isLoading) {
    return <PageLoadingSkeleton variant="settings" className="w-full" />;
  }

  if (!form) {
    return <PageLoadingSkeleton variant="settings" className="w-full" />;
  }

  return (
    <div className="p-6 md:p-8 max-w-3xl mx-auto w-full pb-12">
      <header className="flex items-start justify-between mb-6 gap-4 flex-wrap">
        <div>
          <h1 className="text-[28px] font-bold text-ios-text dark:text-ios-textDark tracking-tight flex items-center gap-3">
            <SettingsIcon className="w-7 h-7 text-ios-blue" strokeWidth={2.4} />
            MITM 设置
          </h1>
          <p className="mt-1 text-[13px] text-gray-500 dark:text-gray-400">
            修改任何项会自动保存（500ms 防抖）。
          </p>
        </div>
        <IAutoSaveIndicator state={saveState} errorText={saveError} />
      </header>

      <div className="space-y-8">
        {/* ═══ 基础 ═══ */}
        <Section title="基础" icon="⚙️">
          <ISettingRow
            title="导入并发数"
            description="批量导入时同时进行的请求数。调高可加速，但容易触发上游限速 (429)。"
          >
            <INumberStepper
              modelValue={form.import_concurrency}
              onValueChange={(v) => patch({ import_concurrency: v })}
              min={1}
              max={20}
              suffix="并发"
              width={70}
            />
          </ISettingRow>
          <ISettingRow
            title="自动刷新所有凭证"
            description="启动时刷新一次 JWT；之后每 24h 刷一次。"
          >
            <IToggle
              modelValue={form.auto_refresh_tokens}
              onValueChange={(v) => patch({ auto_refresh_tokens: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="自动同步所有额度"
            description="启动时同步一次额度；之后按下方策略刷新。"
          >
            <IToggle
              modelValue={form.auto_refresh_quotas}
              onValueChange={(v) => patch({ auto_refresh_quotas: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="额度刷新策略"
            description="决定全局定时同步额度的频率。"
          >
            <select
              value={form.quota_refresh_policy}
              onChange={(e) => patch({ quota_refresh_policy: e.target.value })}
              className="no-drag-region rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] dark:border-white/[0.08] dark:bg-white/[0.06]"
            >
              {quotaPolicyOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </ISettingRow>
          <ISettingRow
            title="自定义刷新间隔"
            description="quota_refresh_policy=interval_custom 时生效。"
          >
            <INumberStepper
              modelValue={form.quota_custom_interval_minutes}
              onValueChange={(v) =>
                patch({ quota_custom_interval_minutes: v })
              }
              min={5}
              max={1440}
              suffix="分"
              width={70}
            />
          </ISettingRow>
          <ISettingRow
            title="额度热轮询间隔"
            description="MITM 启动后定时拉当前活跃账号额度，发现见底主动切号。"
            noBorder
          >
            <INumberStepper
              modelValue={form.quota_hot_poll_seconds}
              onValueChange={(v) => patch({ quota_hot_poll_seconds: v })}
              min={3}
              max={120}
              suffix="秒"
              width={70}
            />
          </ISettingRow>
        </Section>

        {/* ═══ 自动切号 ═══ */}
        <Section title="自动切号" icon="🔄">
          <ISettingRow
            title="额度耗尽时自动切下一席"
            description="MITM 收到 quota exceeded 时立刻切换。"
          >
            <IToggle
              modelValue={form.auto_switch_on_quota_exhausted}
              onValueChange={(v) =>
                patch({ auto_switch_on_quota_exhausted: v })
              }
            />
          </ISettingRow>
          <ISettingRow
            title="自动切号 套餐筛选"
            description="只有该 plan 类型的账号会被纳入候选。"
            noBorder
          >
            <select
              value={form.auto_switch_plan_filter}
              onChange={(e) => patch({ auto_switch_plan_filter: e.target.value })}
              className="no-drag-region rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] dark:border-white/[0.08] dark:bg-white/[0.06]"
            >
              {switchPlanFilterOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </ISettingRow>
        </Section>

        {/* ═══ Pin & 轮换池 ═══ */}
        <Section title="Pin · 轮换池" icon="🔒">
          <ISettingRow
            title="启用手动锁定 (Pin)"
            description="锁定后所有自动切都跳过，用户 100% 控制。"
          >
            <IToggle
              modelValue={form.manual_pin_enabled}
              onValueChange={(v) => patch({ manual_pin_enabled: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="启用轮换池"
            description="勾 2+ 个账号进池，定时切 + 额度耗尽双触发都只在池内来回切。"
          >
            <IToggle
              modelValue={form.rotation_pool_enabled}
              onValueChange={(v) => patch({ rotation_pool_enabled: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="轮换池 定时切换间隔"
            description="即便没耗尽也会按此间隔切下一个池内账号。"
          >
            <INumberStepper
              modelValue={form.rotation_pool_interval_min}
              onValueChange={(v) => patch({ rotation_pool_interval_min: v })}
              min={1}
              max={1440}
              suffix="分"
              width={70}
            />
          </ISettingRow>
          <ISettingRow
            title="轮换池 额度刷新间隔"
            description="池内账号独立的 quota 同步频率（不影响全局策略）。"
            noBorder
          >
            <INumberStepper
              modelValue={form.rotation_pool_quota_refresh_min}
              onValueChange={(v) =>
                patch({ rotation_pool_quota_refresh_min: v })
              }
              min={1}
              max={120}
              suffix="分"
              width={70}
            />
          </ISettingRow>
        </Section>

        {/* ═══ Clash IP 轮换 ═══ */}
        <Section title="Clash IP 轮换" icon="🔀">
          <ISettingRow
            title="启用 Clash 轮换"
            description="按下方间隔自动切换 Clash 出口节点；上游 429 时也会立刻切。"
          >
            <IToggle
              modelValue={form.clash_rotate_enabled}
              onValueChange={(v) => patch({ clash_rotate_enabled: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="控制器 URL"
            description="Clash / Mihomo / Verge 控制器地址（默认 9097）。"
          >
            <input
              value={form.clash_controller_url}
              onChange={(e) => patch({ clash_controller_url: e.target.value })}
              type="text"
              className="no-drag-region w-[260px] rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] font-mono dark:border-white/[0.08] dark:bg-white/[0.06]"
              placeholder="http://127.0.0.1:9097"
            />
          </ISettingRow>
          <ISettingRow
            title="Secret"
            description="如果 Clash external-controller 配了 secret，填这里。"
          >
            <input
              value={form.clash_secret}
              onChange={(e) => patch({ clash_secret: e.target.value })}
              type="password"
              className="no-drag-region w-[260px] rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] font-mono dark:border-white/[0.08] dark:bg-white/[0.06]"
            />
          </ISettingRow>
          <ISettingRow
            title="选择器组 (group)"
            description="留空 → 智能启用时自动检测节点最多的 selector 组。"
          >
            <input
              value={form.clash_group}
              onChange={(e) => patch({ clash_group: e.target.value })}
              type="text"
              className="no-drag-region w-[200px] rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] dark:border-white/[0.08] dark:bg-white/[0.06]"
              placeholder="(自动)"
            />
          </ISettingRow>
          <ISettingRow
            title="切换间隔"
            description="定时切换出口节点的频率。"
          >
            <INumberStepper
              modelValue={form.clash_interval_minutes}
              onValueChange={(v) => patch({ clash_interval_minutes: v })}
              min={1}
              max={1440}
              suffix="分"
              width={70}
            />
          </ISettingRow>
          <ISettingRow
            title="429 时立即切节点"
            description="收到 rate limit 时不等周期，立刻换 IP。"
          >
            <IToggle
              modelValue={form.clash_rotate_on_rate_limit}
              onValueChange={(v) => patch({ clash_rotate_on_rate_limit: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="延迟测试 URL"
            description="选节点时用来探活 + 测延迟的目标 URL。"
          >
            <input
              value={form.clash_latency_test_url}
              onChange={(e) =>
                patch({ clash_latency_test_url: e.target.value })
              }
              type="text"
              className="no-drag-region w-[260px] rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] font-mono dark:border-white/[0.08] dark:bg-white/[0.06]"
            />
          </ISettingRow>
          <ISettingRow
            title="最大延迟阈值"
            description="超过此延迟的节点不参与轮换。0 = 不限制。"
            noBorder
          >
            <INumberStepper
              modelValue={form.clash_latency_max_ms}
              onValueChange={(v) => patch({ clash_latency_max_ms: v })}
              min={0}
              max={10000}
              step={100}
              suffix="ms"
              width={80}
            />
          </ISettingRow>
        </Section>

        {/* ═══ 破限注入 ═══ */}
        <Section title="Cascade 破限注入" icon="✨">
          <ISettingRow
            title="启用破限注入"
            description="MITM 拦截 chat 请求，在 F2 system prompt 末尾追加 override 文本。"
          >
            <IToggle
              modelValue={form.mitm_jailbreak_enabled}
              onValueChange={(v) => patch({ mitm_jailbreak_enabled: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="预设"
            description="custom: 用下方文本框；minimal/soft_safe/original_full 见 Help 第 7 章。"
          >
            <select
              value={form.mitm_jailbreak_preset_id}
              onChange={(e) =>
                patch({ mitm_jailbreak_preset_id: e.target.value })
              }
              className="no-drag-region rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] dark:border-white/[0.08] dark:bg-white/[0.06]"
            >
              <option value="custom">custom · 自定义</option>
              <option value="minimal">minimal · 极简（推荐）</option>
              <option value="soft_safe">soft_safe · 软版</option>
              <option value="original_full">
                original_full · 原版（高风险）
              </option>
            </select>
          </ISettingRow>
          <ISettingRow
            title="文本来源"
            description="inline = 用下方文本框；file = 从外部文件读取。"
          >
            <select
              value={form.mitm_jailbreak_override_source}
              onChange={(e) =>
                patch({ mitm_jailbreak_override_source: e.target.value })
              }
              className="no-drag-region rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] dark:border-white/[0.08] dark:bg-white/[0.06]"
            >
              <option value="inline">inline · 内嵌</option>
              <option value="file">file · 外部文件</option>
            </select>
          </ISettingRow>
          {form.mitm_jailbreak_override_source === "file" ? (
            <ISettingRow
              title="文件路径"
              description="留空 → 默认 ~/.claude/override.md。"
            >
              <input
                value={form.mitm_jailbreak_override_file}
                onChange={(e) =>
                  patch({ mitm_jailbreak_override_file: e.target.value })
                }
                type="text"
                className="no-drag-region w-[280px] rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] font-mono dark:border-white/[0.08] dark:bg-white/[0.06]"
                placeholder="~/.claude/override.md"
              />
            </ISettingRow>
          ) : (
            <ISettingRow
              title="自定义注入文本"
              description="只有 preset=custom + source=inline 时使用。"
              stacked
              noBorder
            >
              <textarea
                value={form.mitm_jailbreak_override}
                onChange={(e) =>
                  patch({ mitm_jailbreak_override: e.target.value })
                }
                rows={6}
                className="no-drag-region w-full rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[12.5px] font-mono dark:border-white/[0.08] dark:bg-white/[0.06]"
                placeholder="留空 → 后端 fallback 到 DefaultJailbreakOverride"
              />
            </ISettingRow>
          )}
        </Section>

        {/* ═══ 桌面行为 ═══ */}
        <Section title="桌面行为" icon="🖥️">
          <ISettingRow
            title="关闭窗口时最小化到托盘"
            description="点 X 后程序仍在托盘运行；右键托盘菜单可彻底退出。"
          >
            <IToggle
              modelValue={form.minimize_to_tray}
              onValueChange={(v) => patch({ minimize_to_tray: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="桌面通知"
            description="Pin 解除 / 额度耗尽 / Clash 错误等关键事件弹通知（60s 同类去重）。"
          >
            <IToggle
              modelValue={form.desktop_notifications}
              onValueChange={(v) => patch({ desktop_notifications: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="启动时不显示主窗口"
            description="开机自启场景下静默挂托盘，托盘图标可打开主窗口。"
            noBorder
          >
            <IToggle
              modelValue={form.silent_start}
              onValueChange={(v) => patch({ silent_start: v })}
            />
          </ISettingRow>
        </Section>

        {/* ═══ OpenAI Relay ═══ */}
        <Section title="OpenAI Relay" icon="🌐">
          <ISettingRow
            title="启用 Relay"
            description="对外暴露 OpenAI 兼容 Chat Completions 中转。"
          >
            <IToggle
              modelValue={form.openai_relay_enabled}
              onValueChange={(v) => patch({ openai_relay_enabled: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="监听端口"
            description="本机 127.0.0.1:此端口 提供服务。"
          >
            <INumberStepper
              modelValue={form.openai_relay_port}
              onValueChange={(v) => patch({ openai_relay_port: v })}
              min={1}
              max={65535}
              width={90}
            />
          </ISettingRow>
          <ISettingRow
            title="鉴权 Bearer Secret"
            description="留空 = 任何 API Key 都接受；填了 = 客户端必须带匹配 Bearer。"
            noBorder
          >
            <input
              value={form.openai_relay_secret}
              onChange={(e) => patch({ openai_relay_secret: e.target.value })}
              type="text"
              className="no-drag-region w-[260px] rounded-[12px] border border-black/[0.06] bg-white px-3 py-2 text-[13px] font-mono dark:border-white/[0.08] dark:bg-white/[0.06]"
              placeholder="(可选)"
            />
          </ISettingRow>
        </Section>

        {/* ═══ 高级 / 调试 ═══ */}
        <Section title="高级 · 调试" icon="🔧">
          <ISettingRow
            title="MITM 静态缓存拦截"
            description=".bin 静态资源直返本地缓存，减少上游回源。"
          >
            <IToggle
              modelValue={form.static_cache_intercept}
              onValueChange={(v) => patch({ static_cache_intercept: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="伪造 Enterprise Plan"
            description="GetUserStatus/GetPlanStatus 伪造为 Enterprise 无限积分（仅 IDE 显示用）。"
          >
            <IToggle
              modelValue={form.forge_enabled}
              onValueChange={(v) => patch({ forge_enabled: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="MITM 全量抓包"
            description="把所有 MITM 经过的 HTTPS 请求/响应落盘到 capture/ 目录。"
          >
            <IToggle
              modelValue={form.mitm_full_capture}
              onValueChange={(v) => patch({ mitm_full_capture: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="protobuf debug dump"
            description="把 GetChatMessage 的 protobuf 字段树打印到日志。"
          >
            <IToggle
              modelValue={form.mitm_debug_dump}
              onValueChange={(v) => patch({ mitm_debug_dump: v })}
            />
          </ISettingRow>
          <ISettingRow
            title="调试日志"
            description="切号/代理/额度判定的详细决策过程写入 debug.log。"
            noBorder
          >
            <IToggle
              modelValue={form.debug_log}
              onValueChange={(v) => patch({ debug_log: v })}
            />
          </ISettingRow>
        </Section>

        {/* ═══ 配置导入导出 ═══ */}
        <Section title="配置导入 / 导出" icon="💾">
          <ISettingRow
            title="导出当前配置"
            description="把 settings.json 复制到桌面，便于多设备同步。"
          >
            <button
              type="button"
              onClick={handleExport}
              className="no-drag-region rounded-full bg-ios-blue/10 hover:bg-ios-blue/15 px-4 py-2 text-[12px] font-bold text-ios-blue ios-btn"
            >
              导出
            </button>
          </ISettingRow>
          <ISettingRow
            title="导入配置"
            description="粘贴 settings.json 全文，覆盖当前配置。"
            noBorder
          >
            <button
              type="button"
              onClick={handleImport}
              className="no-drag-region rounded-full bg-violet-500/10 hover:bg-violet-500/15 px-4 py-2 text-[12px] font-bold text-violet-700 dark:text-violet-300 ios-btn"
            >
              粘贴并导入
            </button>
          </ISettingRow>
        </Section>

        {/* ═══ F7 SmartFriend（仅作者自用） ═══ */}
        {/* F7-REMOVAL-BEGIN */}
        <Section title="F7 · SmartFriend（仅作者自用）" icon="🎩">
          <ISettingRow
            title="启用 F7 模式"
            description="把 GetChatMessage 类型从 CASCADE(5) 改成 SMART_FRIEND(13)，服务端按 SMART_FRIEND 计费、绕过日/周额度限制。仅作者自用，发布前会被移除。"
          >
            <div className="flex items-center gap-3">
              <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[10px] font-bold text-amber-700 dark:text-amber-300">
                Author-only
              </span>
              <IToggle
                modelValue={form.smart_friend_enabled}
                onValueChange={(v) => patch({ smart_friend_enabled: v })}
              />
            </div>
          </ISettingRow>
          {form.smart_friend_enabled ? (
            <ISettingRow
              title="状态"
              description="F7 已开启 — 显示「耗尽」的账号实际仍可用，自动切号已暂停。"
              noBorder
            >
              <ShieldCheck
                className="w-5 h-5 text-emerald-500"
                strokeWidth={2.4}
              />
            </ISettingRow>
          ) : null}
        </Section>
        {/* F7-REMOVAL-END */}
      </div>
    </div>
  );
}

// ── 内部 Section wrapper ─────────────
function Section({
  title,
  icon,
  children,
}: {
  title: string;
  icon: string;
  children: React.ReactNode;
}) {
  return (
    <section>
      <h2 className="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2 flex items-center gap-2">
        <span className="text-[14px]">{icon}</span>
        {title}
      </h2>
      <div className="rounded-[18px] border border-black/[0.05] bg-white/70 overflow-hidden shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
        {children}
      </div>
    </section>
  );
}
