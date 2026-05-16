// F7-REMOVAL: 整文件删除（仅作者自用功能；发布前彻底移除）
// ─────────────────────────────────────────────────────────────────────
// useSmartFriend ─ 把前端散落的 SmartFriend(F7) 判定收拢到唯一来源。
//
// 设计动机：F7 是把 GetChatMessage 请求类型从 CASCADE(5) 改成 SMART_FRIEND(13)
// 的作者向 patch，开启后服务端按 SMART_FRIEND 计费、绕过日/周限额。
// 前端散落的 5 个判定维度集中到此 hook：
//   1. 卡片 tone / label
//   2. 进度条颜色（强制绿）
//   3. 「周限阻断 / 额度耗尽 / 运行时见底」展示忽略
//   4. Dashboard 健康度计数归零
//   5. 顶部 banner
//
// 删除步骤参见仓库根 docs/F7-REMOVAL.md。
// ─────────────────────────────────────────────────────────────────────
import { useSettingsStore } from "../stores/useSettingsStore";

export type CardTone = "online" | "ready" | "warning" | "danger" | "pending";

/**
 * React hook：每次 settings 变化时返回 active 标记 + 一组纯函数 helper。
 * 用法：
 *   const sf = useSmartFriend();
 *   if (sf.active) { ... }
 */
export function useSmartFriend() {
  const active = useSettingsStore(
    (s) => s.settings?.smart_friend_enabled === true,
  );

  /**
   * 进度条颜色覆盖：F7 时强制 ios-green，否则按调用方原始百分比上色。
   */
  const quotaColorOverride = (originalColor: string): string =>
    active ? "bg-ios-green" : originalColor;

  /**
   * 「额度类警告」断言包装：F7 开启时直接返回 false（按可用处理）。
   */
  const bypassQuotaWarning = <T>(rawPredicate: () => T): T | false =>
    active ? false : rawPredicate();

  /**
   * 卡片 label 改写：F7 时把可用账号的 label 改成「F7 · 已绕过额度」/
   * 「当前活跃 · F7」等。
   */
  const cardLabelOverride = (
    tone: CardTone,
    baseLabel: string,
    ctx: { isCurrent: boolean; isReadyOrOnline: boolean },
  ): string => {
    if (!active) return baseLabel;
    if (ctx.isCurrent) return "当前活跃 · F7";
    if (ctx.isReadyOrOnline) return "F7 · 已绕过额度";
    return baseLabel;
  };

  return { active, quotaColorOverride, bypassQuotaWarning, cardLabelOverride };
}
