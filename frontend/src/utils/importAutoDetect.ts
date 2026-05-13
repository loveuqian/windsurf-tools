import { main } from '../../wailsjs/go/models'
import { parsePasswordLine, mergePasswordContinuationLines } from './importParse'

/**
 * 凭证类型自动检测
 * - sk-ws-*                 → api_key
 * - eyJ* (base64 JWT 双段)  → jwt
 * - 含 @ + 有密码           → password
 * - 长度 ≥ 40 的长字符串    → refresh_token
 * - 其他（短乱码 / 中文备注 / 误粘）→ unknown，忽略不提交
 *
 * 之前的版本把所有无法识别的输入都兜底成 refresh_token，导致用户
 * 误粘 "test" / "1234" / 中文备注 等会被提交到后端 → 后端 Firebase
 * refresh API 一定失败，浪费请求、堆错误结果、UX 困惑。
 */
export type DetectedType = 'api_key' | 'jwt' | 'password' | 'refresh_token' | 'unknown'

export interface DetectedLine {
  type: DetectedType
  raw: string
}

const API_KEY_RE = /^sk-ws-/i
const JWT_RE = /^eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+/
const EMAIL_RE = /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/i
// refresh_token 的最小可接受长度（Firebase refresh token 实际约 200+ 字符；
// 这里宽松一点 40 字符门槛，过滤掉明显是错粘的短串）
const REFRESH_TOKEN_MIN_LEN = 40

export function detectLineType(line: string): DetectedType {
  const trimmed = line.trim()
  const first = trimmed.split(/\s+/)[0] || ''

  if (API_KEY_RE.test(first)) return 'api_key'
  if (JWT_RE.test(first)) return 'jwt'
  if (EMAIL_RE.test(trimmed)) return 'password'
  // 长字符串 → 可能是 refresh token
  if (first.length >= REFRESH_TOKEN_MIN_LEN) return 'refresh_token'
  // 短输入 / 中文备注 / 乱码 → 不识别，不提交
  return 'unknown'
}

export interface GroupedImportItems {
  apiKeys: main.APIKeyItem[]
  jwts: main.JWTItem[]
  tokens: main.TokenItem[]
  passwords: main.EmailPasswordItem[]
  /** 未识别的行（短输入 / 备注 / 乱码），仅用于 UI 提示，不会提交 */
  unknown: string[]
}

/**
 * 将混合输入按凭证类型自动分组
 */
export function groupImportLines(rawLines: string[]): GroupedImportItems {
  const lines = rawLines.map(l => l.trim()).filter(Boolean)
  const result: GroupedImportItems = {
    apiKeys: [],
    jwts: [],
    tokens: [],
    passwords: [],
    unknown: [],
  }

  // 先把可能是邮箱+密码续行的合并
  const merged = mergePasswordContinuationLines(lines)

  // 去重 map（邮箱密码按 email 去重）
  const emailSeen = new Map<string, main.EmailPasswordItem>()

  for (const line of merged) {
    const type = detectLineType(line)
    const parts = line.trim().split(/\s+/)
    const first = parts[0] || ''
    const remark = parts.slice(1).join(' ').trim()

    switch (type) {
      case 'api_key':
        result.apiKeys.push(new main.APIKeyItem({ api_key: first, remark }))
        break
      case 'jwt':
        result.jwts.push(new main.JWTItem({ jwt: first, remark }))
        break
      case 'refresh_token':
        result.tokens.push(new main.TokenItem({ token: first, remark }))
        break
      case 'password': {
        const parsed = parsePasswordLine(line)
        if (parsed) {
          emailSeen.set(parsed.email.toLowerCase(), parsed)
        } else {
          // EMAIL_RE 命中但 parsePasswordLine 解析失败（如缺密码、WFH 卡密格式等）
          result.unknown.push(line)
        }
        break
      }
      case 'unknown':
        result.unknown.push(line)
        break
    }
  }

  result.passwords = Array.from(emailSeen.values())
  return result
}

export interface DetectionSummary {
  api_key: number
  jwt: number
  refresh_token: number
  password: number
  unknown: number
  /** 可提交的总数（不含 unknown） */
  total: number
}

export function summarizeGrouped(g: GroupedImportItems): DetectionSummary {
  return {
    api_key: g.apiKeys.length,
    jwt: g.jwts.length,
    refresh_token: g.tokens.length,
    password: g.passwords.length,
    unknown: g.unknown.length,
    total: g.apiKeys.length + g.jwts.length + g.tokens.length + g.passwords.length,
  }
}
