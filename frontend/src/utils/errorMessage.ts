/**
 * errorMessage — 把后端 / runtime 抛出的英文错误翻译成给用户看的中文短句。
 *
 * 设计动机：
 *   旧写法 showToast(`失败: ${String(e)}`, 'error') 会直接把 Go 的
 *   `rpc error: ... permission denied: ...` 暴露给用户。新建 util 集中翻译常见模式：
 *     - 网络 / 超时
 *     - 提权 / 权限拒绝
 *     - JWT / token / 认证
 *     - hosts / CA / 端口
 *     - Clash / 控制器
 *     - SQLite / store / 文件锁
 *
 * 用法：showToast(friendlyError(e, '保存设置失败'), 'error')
 */

interface Pattern {
  /** 匹配关键字（小写后子串匹配） */
  match: string[];
  /** 翻译后的短句 */
  msg: string;
  /** 可选 tail 修复建议 */
  hint?: string;
}

const PATTERNS: Pattern[] = [
  // ── 网络 / 超时 ──
  {
    match: ["context deadline exceeded", "i/o timeout", "request timeout", "deadline exceeded"],
    msg: "请求超时",
    hint: "检查网络是否能直连 windsurf.com / Clash 是否在跑",
  },
  {
    match: ["no such host", "dial tcp", "connection refused", "lookup ", "dial: connection"],
    msg: "网络连接失败",
    hint: "检查代理 / Clash 控制器地址 / DNS",
  },
  // ── 提权 / 权限 ──
  {
    match: ["permission denied", "拒绝访问", "operation not permitted"],
    msg: "需要管理员权限",
    hint: "macOS：在弹出的 Terminal 输登录密码；Windows：右键以管理员身份运行",
  },
  {
    match: ["pkexec", "polkit"],
    msg: "提权对话框被取消或失败",
    hint: "Linux：sudo apt install policykit-1；或退化为终端 sudo",
  },
  // ── 认证 / token ──
  {
    match: ["jwt", "id_token", "expired token", "invalid_grant", "invalid token"],
    msg: "凭证已失效，需要重新登录或刷新",
    hint: "在号池里点「刷新所有 token」，或重新导入账号",
  },
  {
    match: ["unauthenticated", "unauthorized", "401"],
    msg: "鉴权失败",
    hint: "API Key 或 JWT 无效，建议重新导入或刷新",
  },
  {
    match: ["permission_denied", "user is disabled", "subscription is not active"],
    msg: "账号已被禁用或订阅已过期",
    hint: "在号池删除该账号，或换其他账号",
  },
  // ── hosts / CA / 端口 ──
  {
    match: ["address already in use", "bind: address"],
    msg: "本机端口已被占用",
    hint: "Windsurf IDE / 其他代理可能正占着端口；先关掉再试",
  },
  {
    match: ["porthelper", "fd-passing", "443"],
    msg: "443 端口拿不到",
    hint: "macOS 需在 Terminal 弹出的密码框输入登录密码",
  },
  {
    match: ["hosts 文件", "/etc/hosts", "drivers\\etc\\hosts"],
    msg: "hosts 写入失败",
    hint: "需要管理员权限；macOS 输登录密码 / Windows 用管理员身份启动",
  },
  {
    match: ["certutil", "sectrustsettings", "update-ca-certificates", "ca 证书"],
    msg: "CA 证书安装失败",
    hint: "在「Settings → MITM」面板里手动「安装证书」并按提示授权",
  },
  // ── Clash 控制器 ──
  {
    match: ["clash"],
    msg: "Clash 控制器连接失败",
    hint: "检查 1) Clash 是否运行 2) external-controller 端口对不对 3) secret 对不对",
  },
  // ── SQLite / 存储 ──
  {
    match: ["database is locked", "no such table", "sqlite"],
    msg: "本地存储读写失败",
    hint: "确认软件没被多开；或备份后重启工具",
  },
  // ── Windsurf API ──
  {
    match: ["resource_exhausted", "quota", "rate limit"],
    msg: "服务端额度或限速触发",
    hint: "等几分钟再试，或在号池里切到其他账号",
  },
  {
    match: ["invalid argument", "invalid_argument"],
    msg: "请求参数被服务端拒绝",
    hint: "可能 protobuf 格式或 model 名不被支持；查看调试日志",
  },
];

/**
 * friendlyError 把任意类型的 error / unknown 翻译成短句。
 *  fallback 在没匹配上时返回，默认是「操作失败」。
 *  原始错误总会拼在末尾的小字里方便排查（pre-line 换行）。
 */
export function friendlyError(err: unknown, fallback = "操作失败"): string {
  const raw = errorText(err);
  if (!raw) return fallback;

  const lower = raw.toLowerCase();
  for (const p of PATTERNS) {
    if (p.match.some((kw) => lower.includes(kw))) {
      const head = p.hint ? `${p.msg} · ${p.hint}` : p.msg;
      return `${head}\n（${raw.length > 200 ? raw.slice(0, 200) + "…" : raw}）`;
    }
  }

  return `${fallback}\n（${raw.length > 200 ? raw.slice(0, 200) + "…" : raw}）`;
}

function errorText(err: unknown): string {
  if (err == null) return "";
  if (typeof err === "string") return err;
  if (err instanceof Error) return err.message;
  if (typeof err === "object" && "message" in err) {
    return String((err as { message: unknown }).message ?? "");
  }
  try {
    return String(err);
  } catch {
    return "";
  }
}
