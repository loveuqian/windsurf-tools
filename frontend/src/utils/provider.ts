import type { models } from '../../wailsjs/go/models'

export type ProviderID =
  | 'openai'
  | 'anthropic'
  | 'google'
  | 'deepseek'
  | 'moonshot'
  | 'qwen'
  | 'doubao'
  | 'minimax'
  | 'zhipu'
  | 'xai'

export interface ProviderEnvHint {
  /** ENV 变量名 — base url */
  baseUrl: string
  /** ENV 变量名 — token / api key */
  token: string
  /** 写在 placeholder 里的示例 host(用于演示行) */
  exampleBaseUrl: string
  /** 写在 placeholder 里的示例 token(用于演示行) */
  exampleToken: string
}

export interface ProviderMeta {
  id: ProviderID
  label: string
  tagline: string
  host: string
  credentialKinds: string[]
  accent: string
  badge: string
  initials: string
  envHint: ProviderEnvHint
}

export const PROVIDER_META: Record<ProviderID, ProviderMeta> = {
  openai: {
    id: 'openai',
    label: 'OpenAI',
    tagline: 'GPT-4o · GPT-4.1 · o3 系列',
    host: 'api.openai.com',
    credentialKinds: ['sk-proj-*', 'sk-*'],
    accent: 'from-emerald-500 via-teal-400 to-cyan-300',
    badge: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
    initials: 'AI',
    envHint: {
      baseUrl: 'OPENAI_BASE_URL',
      token: 'OPENAI_API_KEY',
      exampleBaseUrl: 'https://api.openai.com/v1',
      exampleToken: 'sk-proj-xxxxxxxxxxxxxxxx',
    },
  },
  anthropic: {
    id: 'anthropic',
    label: 'Anthropic',
    tagline: 'Claude Opus / Sonnet / Haiku',
    host: 'api.anthropic.com',
    credentialKinds: ['sk-ant-*'],
    accent: 'from-orange-500 via-rose-400 to-amber-300',
    badge: 'bg-orange-500/15 text-orange-700 dark:text-orange-300',
    initials: 'AN',
    envHint: {
      baseUrl: 'ANTHROPIC_BASE_URL',
      token: 'ANTHROPIC_AUTH_TOKEN',
      exampleBaseUrl: 'https://api.anthropic.com',
      exampleToken: 'sk-ant-xxxxxxxxxxxxxxxx',
    },
  },
  google: {
    id: 'google',
    label: 'Google',
    tagline: 'Gemini Pro · Flash · 2.5',
    host: 'generativelanguage.googleapis.com',
    credentialKinds: ['AIza*'],
    accent: 'from-blue-500 via-rose-400 to-amber-300',
    badge: 'bg-blue-500/15 text-blue-700 dark:text-blue-300',
    initials: 'GG',
    envHint: {
      baseUrl: 'GOOGLE_BASE_URL',
      token: 'GOOGLE_API_KEY',
      exampleBaseUrl: 'https://generativelanguage.googleapis.com',
      exampleToken: 'AIzaSyXXXXXXXXXXXXXXXXXX',
    },
  },
  deepseek: {
    id: 'deepseek',
    label: 'DeepSeek',
    tagline: 'V3 · R1 推理',
    host: 'api.deepseek.com',
    credentialKinds: ['sk-*'],
    accent: 'from-indigo-500 via-blue-400 to-cyan-300',
    badge: 'bg-indigo-500/15 text-indigo-700 dark:text-indigo-300',
    initials: 'DS',
    envHint: {
      baseUrl: 'DEEPSEEK_BASE_URL',
      token: 'DEEPSEEK_API_KEY',
      exampleBaseUrl: 'https://api.deepseek.com',
      exampleToken: 'sk-xxxxxxxxxxxxxxxxxxxxxxxx',
    },
  },
  moonshot: {
    id: 'moonshot',
    label: 'Kimi',
    tagline: 'Moonshot · K1.5 · 长上下文',
    host: 'api.moonshot.cn',
    credentialKinds: ['sk-*'],
    accent: 'from-violet-500 via-purple-400 to-fuchsia-300',
    badge: 'bg-violet-500/15 text-violet-700 dark:text-violet-300',
    initials: 'KI',
    envHint: {
      baseUrl: 'MOONSHOT_BASE_URL',
      token: 'MOONSHOT_API_KEY',
      exampleBaseUrl: 'https://api.moonshot.cn/v1',
      exampleToken: 'sk-xxxxxxxxxxxxxxxxxxxxxxxx',
    },
  },
  qwen: {
    id: 'qwen',
    label: '通义千问',
    tagline: 'Qwen · DashScope',
    host: 'dashscope.aliyuncs.com',
    credentialKinds: ['sk-*'],
    accent: 'from-purple-500 via-violet-400 to-pink-300',
    badge: 'bg-purple-500/15 text-purple-700 dark:text-purple-300',
    initials: 'QW',
    envHint: {
      baseUrl: 'DASHSCOPE_BASE_URL',
      token: 'DASHSCOPE_API_KEY',
      exampleBaseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
      exampleToken: 'sk-xxxxxxxxxxxxxxxxxxxxxxxx',
    },
  },
  doubao: {
    id: 'doubao',
    label: '豆包',
    tagline: '字节火山方舟 · Doubao Pro',
    host: 'ark.cn-beijing.volces.com',
    credentialKinds: ['Bearer *'],
    accent: 'from-sky-500 via-blue-400 to-indigo-300',
    badge: 'bg-sky-500/15 text-sky-700 dark:text-sky-300',
    initials: 'DB',
    envHint: {
      baseUrl: 'ARK_BASE_URL',
      token: 'ARK_API_KEY',
      exampleBaseUrl: 'https://ark.cn-beijing.volces.com/api/v3',
      exampleToken: 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx',
    },
  },
  minimax: {
    id: 'minimax',
    label: 'MiniMax',
    tagline: 'abab6.5s · 海螺',
    host: 'api.minimax.chat',
    credentialKinds: ['eyJ* JWT'],
    accent: 'from-rose-500 via-pink-400 to-fuchsia-300',
    badge: 'bg-rose-500/15 text-rose-700 dark:text-rose-300',
    initials: 'MM',
    envHint: {
      baseUrl: 'MINIMAX_BASE_URL',
      token: 'MINIMAX_API_KEY',
      exampleBaseUrl: 'https://api.minimax.chat/v1',
      exampleToken: 'eyJhbGciOi...xxxxxxxx',
    },
  },
  zhipu: {
    id: 'zhipu',
    label: '智谱',
    tagline: 'GLM-4 · GLM-4-Flash',
    host: 'open.bigmodel.cn',
    credentialKinds: ['JWT id.secret'],
    accent: 'from-cyan-500 via-sky-400 to-blue-300',
    badge: 'bg-cyan-500/15 text-cyan-700 dark:text-cyan-300',
    initials: 'ZP',
    envHint: {
      baseUrl: 'ZHIPUAI_BASE_URL',
      token: 'ZHIPUAI_API_KEY',
      exampleBaseUrl: 'https://open.bigmodel.cn/api/paas/v4',
      exampleToken: 'xxxxxxxxxxxxxxxxxxxxxxxx.xxxxxxxxxxxxxxxx',
    },
  },
  xai: {
    id: 'xai',
    label: 'xAI',
    tagline: 'Grok 系列',
    host: 'api.x.ai',
    credentialKinds: ['xai-*'],
    accent: 'from-slate-700 via-slate-500 to-slate-300',
    badge: 'bg-slate-700/15 text-slate-800 dark:text-slate-200',
    initials: 'XA',
    envHint: {
      baseUrl: 'XAI_BASE_URL',
      token: 'XAI_API_KEY',
      exampleBaseUrl: 'https://api.x.ai/v1',
      exampleToken: 'xai-xxxxxxxxxxxxxxxxxxxxxxxx',
    },
  },
}

export const PROVIDER_DISPLAY_ORDER: ProviderID[] = [
  'openai',
  'anthropic',
  'google',
  'deepseek',
  'moonshot',
  'qwen',
  'doubao',
  'minimax',
  'zhipu',
  'xai',
]

const ANTHROPIC_RE = /^sk-ant-/i
const GOOGLE_RE = /^AIza[0-9A-Za-z_-]{20,}/
const XAI_RE = /^xai-/i
const OPENAI_RE = /^sk-proj-|^sk-[A-Za-z0-9_-]{20,}/

/**
 * 当前 Account 模型只有 windsurf_api_key 一个凭证字段,API Key 前缀是唯一可靠
 * 信号 — 同名 sk-* 在 OpenAI / DeepSeek / Moonshot / Qwen 之间无法区分,
 * 这种情况返回 null,等账号模型扩 provider 字段后再做硬识别。
 */
export function detectProviderFromAccount(acc: models.Account): ProviderID | null {
  const hints: string[] = []
  const key = String(acc.windsurf_api_key || '').trim()
  if (key) hints.push(key)
  const tok = String(acc.token || '').trim()
  if (tok) hints.push(tok)
  const refresh = String(acc.refresh_token || '').trim()
  if (refresh) hints.push(refresh)

  for (const h of hints) {
    if (ANTHROPIC_RE.test(h)) return 'anthropic'
    if (GOOGLE_RE.test(h)) return 'google'
    if (XAI_RE.test(h)) return 'xai'
  }

  // 凭证类型相同的 sk-* 无法区分,先看 nickname/remark/plan_name 兜底关键词
  const text = [acc.nickname, acc.remark, acc.plan_name, acc.email]
    .map((v) => String(v || '').toLowerCase())
    .join(' ')
  if (text.includes('openai') || text.includes('gpt')) return 'openai'
  if (text.includes('claude') || text.includes('anthropic')) return 'anthropic'
  if (text.includes('gemini') || text.includes('google')) return 'google'
  if (text.includes('deepseek')) return 'deepseek'
  if (text.includes('kimi') || text.includes('moonshot')) return 'moonshot'
  if (text.includes('qwen') || text.includes('通义') || text.includes('dashscope')) return 'qwen'
  if (text.includes('doubao') || text.includes('豆包') || text.includes('volc')) return 'doubao'
  if (text.includes('minimax') || text.includes('海螺')) return 'minimax'
  if (text.includes('zhipu') || text.includes('智谱') || text.includes('glm')) return 'zhipu'
  if (text.includes('grok') || text.includes('xai')) return 'xai'

  // 兜底用 sk-* 一般属于 OpenAI 兼容(占比最大)
  if (key && OPENAI_RE.test(key)) return 'openai'
  return null
}

/**
 * 生成 ImportModal 的 placeholder 文案。
 * - provider=null 时返回混合识别提示(原行为)
 * - provider 命中时:每行 `<base_url> <token>` 用空格分隔,只展示值
 */
export function getImportPlaceholder(provider: ProviderID | null): string {
  if (!provider) {
    return [
      '粘贴任意格式的凭证…',
      'sk-ws-01-xxxx',
      'eyJhbGciOi...',
      'user@mail.com password123',
      'user@mail.com----devin-session-token$eyJ...',
      'AMf-vBx...',
    ].join('\n')
  }
  const meta = PROVIDER_META[provider]
  const { exampleBaseUrl, exampleToken } = meta.envHint
  return [
    `${exampleBaseUrl} ${exampleToken}`,
    `${exampleBaseUrl} ${exampleToken}`,
  ].join('\n')
}

// ── Provider 模式专用解析与校验 ──

const URL_RE = /^https?:\/\/[^\s/$.?#].[^\s]*$/i

/** 各 provider 接受的 token 形态。
 * 统一放宽为 sk- 前缀通用风格 — 历史上每家收紧到自家前缀(sk-ant-/AIza/xai-)
 * 反而把第三方代理商发的兼容 key(sk- 风格)挡在外面。
 * 后端真正鉴权时上游会把不合法 token 拒掉, UI 这层只挡明显格式错误。 */
const TOKEN_PATTERNS: Record<ProviderID, RegExp[]> = {
  openai: [/^sk-[A-Za-z0-9_-]+$/],
  anthropic: [/^sk-[A-Za-z0-9_-]+$/],
  google: [/^sk-[A-Za-z0-9_-]+$/, /^AIza[0-9A-Za-z_-]+$/],
  deepseek: [/^sk-[A-Za-z0-9_-]+$/],
  moonshot: [/^sk-[A-Za-z0-9_-]+$/],
  qwen: [/^sk-[A-Za-z0-9_-]+$/],
  doubao: [/^sk-[A-Za-z0-9_-]+$/, /^[A-Za-z0-9-]+$/],
  minimax: [/^sk-[A-Za-z0-9_-]+$/, /^eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/],
  zhipu: [/^sk-[A-Za-z0-9_-]+$/, /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/, /^eyJ[A-Za-z0-9_-]+/],
  xai: [/^sk-[A-Za-z0-9_-]+$/, /^xai-[A-Za-z0-9_-]+$/],
}

export interface ProviderLineParseResult {
  ok: boolean
  /** 解析出的 base_url(含 scheme)*/
  baseUrl: string
  /** 解析出的 token */
  token: string
  /** ok=false 时的人类可读错误描述 */
  error: string
}

/**
 * 解析单行 `<base_url> <token>` 文本。
 * - 第一段必须是 http/https URL
 * - 第二段必须非空且匹配 provider 的 token 形态(任一即可)
 */
export function parseProviderLine(line: string, provider: ProviderID): ProviderLineParseResult {
  const trimmed = line.trim()
  if (!trimmed) {
    return { ok: false, baseUrl: '', token: '', error: '空行' }
  }
  const parts = trimmed.split(/\s+/)
  if (parts.length < 2) {
    return {
      ok: false,
      baseUrl: parts[0] || '',
      token: '',
      error: `缺少 token (期望:<base_url> <token>)`,
    }
  }
  const baseUrl = parts[0]
  const token = parts.slice(1).join(' ').trim()

  if (!URL_RE.test(baseUrl)) {
    return {
      ok: false,
      baseUrl,
      token,
      error: 'base_url 必须以 http:// 或 https:// 开头',
    }
  }
  const patterns = TOKEN_PATTERNS[provider] ?? []
  // 不再硬拦 token 形态 —— 第三方中转(one-api 等)发的兼容 key 常常没有 sk-/AIza
  // 这类前缀,旧逻辑把合法 key 挡在外面。这里只做"非空"硬校验,前缀仅作软提示;
  // 真正不合法的 token 上游鉴权时会拒。
  if (patterns.length > 0 && !patterns.some((re) => re.test(token))) {
    // 仅在 token 含明显非法字符(空白)时才拒,否则放行
    if (/\s/.test(token)) {
      return {
        ok: false,
        baseUrl,
        token,
        error: `token 不应包含空格`,
      }
    }
  }
  return { ok: true, baseUrl, token, error: '' }
}

export interface ProviderInputSummary {
  /** 已分行 trim 过的非空原始行,与 results 对齐 */
  lines: string[]
  /** 每行的解析结果 */
  results: ProviderLineParseResult[]
  validCount: number
  invalidCount: number
  totalLines: number
}

export function validateProviderInput(
  text: string,
  provider: ProviderID,
): ProviderInputSummary {
  const lines = text.split('\n').map((l) => l.trim()).filter(Boolean)
  const results = lines.map((l) => parseProviderLine(l, provider))
  let validCount = 0
  let invalidCount = 0
  for (const r of results) {
    if (r.ok) validCount++
    else invalidCount++
  }
  return { lines, results, validCount, invalidCount, totalLines: lines.length }
}
