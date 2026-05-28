import { useEffect, useMemo, useRef, useState } from 'react'
import { CheckCircle2, Loader2, X } from 'lucide-react'
import {
  PROVIDER_DISPLAY_ORDER,
  PROVIDER_META,
  validateProviderInput,
  type ProviderID,
} from '../../utils/provider'
import { useProviderAccountStore } from '../../stores/useProviderAccountStore'
import { showErrorToast, showToast } from '../../utils/toast'

interface Props {
  isOpen: boolean
  defaultProvider?: ProviderID | null
  onClose: () => void
}

// Provider 模式批量导入：每行 `<base_url> <token>` 用空格分隔
//
// 与 windsurf 号池的 ImportModal 物理隔离：
// - 后端走 ImportByProvider(落 provider_accounts.json)
// - 前端走 useProviderAccountStore.importBatch
// - Token 形态校验由 utils/provider.ts 的 parseProviderLine 完成
export default function ProviderImportModal({ isOpen, defaultProvider, onClose }: Props) {
  const importBatch = useProviderAccountStore((s) => s.importBatch)
  const fetchAccounts = useProviderAccountStore((s) => s.fetchAccounts)

  const [selectedProvider, setSelectedProvider] = useState<ProviderID>('openai')
  const [inputText, setInputText] = useState('')
  const [importing, setImporting] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  // open / 默认 provider 变化时重置
  useEffect(() => {
    if (!isOpen) return
    setSelectedProvider(defaultProvider || 'openai')
    setInputText('')
    setTimeout(() => textareaRef.current?.focus(), 50)
  }, [isOpen, defaultProvider])

  const summary = useMemo(() => {
    if (!inputText.trim()) return null
    return validateProviderInput(inputText, selectedProvider)
  }, [inputText, selectedProvider])

  const meta = PROVIDER_META[selectedProvider]
  const validCount = summary?.validCount ?? 0
  const invalidCount = summary?.invalidCount ?? 0

  const handleImport = async () => {
    if (importing) return
    if (!summary || summary.validCount === 0) {
      showErrorToast(new Error('没有可导入的有效行'), '没有可导入的有效行')
      return
    }
    const items = summary.results
      .map((r, i) => ({ ok: r.ok, baseUrl: r.baseUrl, token: r.token, line: summary.lines[i] }))
      .filter((r) => r.ok)
      .map((r) => ({
        provider: selectedProvider,
        base_url: r.baseUrl,
        token: r.token,
        nickname: '',
        remark: '',
      }))
    setImporting(true)
    try {
      const results = await importBatch(items)
      const ok = results.filter((r) => r.success).length
      const fail = results.length - ok
      if (fail === 0) {
        showToast(`已导入 ${ok} 条 ${meta.label} 账号`, 'success')
      } else {
        showToast(`导入完成: ${ok} 成功 / ${fail} 失败`, ok > 0 ? 'success' : 'error')
      }
      await fetchAccounts(true)
      if (ok > 0) onClose()
    } catch (e: unknown) {
      showErrorToast(e, '导入失败')
    } finally {
      setImporting(false)
    }
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4 py-6 backdrop-blur-sm">
      <div className="relative flex max-h-full w-full max-w-2xl flex-col overflow-hidden rounded-[28px] border border-black/[0.06] bg-white/95 shadow-[0_30px_80px_rgba(15,23,42,0.18)] backdrop-blur-2xl dark:border-white/[0.06] dark:bg-black/80">
        {/* header */}
        <div className="flex items-start justify-between gap-4 border-b border-black/[0.05] px-6 py-4 dark:border-white/[0.06]">
          <div>
            <h2 className="text-[18px] font-extrabold text-ios-text dark:text-ios-textDark">
              批量导入提供商账号
            </h2>
            <p className="mt-1 text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
              先选 provider,再每行粘贴 <code className="font-mono">{'<base_url> <token>'}</code>
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={importing}
            className="ios-btn flex h-8 w-8 items-center justify-center rounded-full hover:bg-black/[0.06] dark:hover:bg-white/[0.08]"
          >
            <X className="h-4 w-4" strokeWidth={2.4} />
          </button>
        </div>

        {/* provider picker */}
        <div className="border-b border-black/[0.05] px-6 py-4 dark:border-white/[0.06]">
          <label className="mb-2 block text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
            选择 Provider
          </label>
          <div className="flex flex-wrap gap-1.5">
            {PROVIDER_DISPLAY_ORDER.map((id) => {
              const m = PROVIDER_META[id]
              const active = selectedProvider === id
              return (
                <button
                  key={id}
                  type="button"
                  className={`ios-btn flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12px] font-bold transition-all ${
                    active
                      ? 'border-ios-blue/40 bg-ios-blue/[0.12] text-ios-blue'
                      : 'border-black/[0.06] bg-white/70 text-ios-textSecondary hover:text-ios-text dark:border-white/[0.06] dark:bg-white/[0.04] dark:text-ios-textSecondaryDark'
                  }`}
                  onClick={() => setSelectedProvider(id)}
                  disabled={importing}
                >
                  <span className={`h-2.5 w-2.5 shrink-0 rounded-full bg-gradient-to-br ${m.accent}`} />
                  <span>{m.label}</span>
                </button>
              )
            })}
          </div>
          <div className="mt-3 rounded-[12px] bg-black/[0.03] px-3 py-2 text-[11px] text-ios-textSecondary dark:bg-white/[0.04] dark:text-ios-textSecondaryDark">
            <span className="font-bold uppercase tracking-[0.16em]">{meta.label}</span>
            <span className="ml-2">{meta.tagline}</span>
            <div className="mt-1 font-mono text-[10px]">
              例: <code>{meta.envHint.exampleBaseUrl}</code> <code>{meta.envHint.exampleToken}</code>
            </div>
          </div>
        </div>

        {/* textarea + summary */}
        <div className="flex flex-1 flex-col gap-2 overflow-hidden px-6 py-4">
          <div className="flex items-center justify-between">
            <label className="text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
              凭证(每行 base_url 空格 token)
            </label>
            {summary && (
              <span className="text-[11px] font-bold tabular-nums">
                <span className="text-emerald-700 dark:text-emerald-300">{validCount} 有效</span>
                {invalidCount > 0 && (
                  <span className="ml-2 text-rose-700 dark:text-rose-300">{invalidCount} 无效</span>
                )}
                <span className="ml-2 text-ios-textSecondary dark:text-ios-textSecondaryDark">
                  / {summary.totalLines} 行
                </span>
              </span>
            )}
          </div>
          <textarea
            ref={textareaRef}
            value={inputText}
            onChange={(e) => setInputText(e.target.value)}
            disabled={importing}
            rows={10}
            className="w-full flex-1 rounded-[16px] border border-black/[0.08] bg-white/80 p-3 font-mono text-[12px] outline-none focus:border-ios-blue/60 dark:border-white/[0.08] dark:bg-white/[0.04]"
            placeholder={`${meta.envHint.exampleBaseUrl} ${meta.envHint.exampleToken}\n${meta.envHint.exampleBaseUrl} ${meta.envHint.exampleToken}`}
          />

          {summary && invalidCount > 0 && (
            <div className="max-h-32 overflow-y-auto rounded-[12px] border border-rose-500/15 bg-rose-500/[0.04] px-3 py-2 text-[11px]">
              <div className="mb-1 font-bold text-rose-700 dark:text-rose-300">解析失败 {invalidCount} 行</div>
              {summary.results
                .map((r, i) => ({ ...r, line: summary.lines[i] }))
                .filter((r) => !r.ok)
                .slice(0, 5)
                .map((r, i) => (
                  <div key={i} className="font-mono text-[10px] text-rose-700 dark:text-rose-300">
                    <span className="opacity-60">"{r.line.slice(0, 60)}"</span> — {r.error}
                  </div>
                ))}
            </div>
          )}
        </div>

        {/* footer */}
        <div className="flex items-center justify-end gap-2 border-t border-black/[0.05] px-6 py-4 dark:border-white/[0.06]">
          <button
            type="button"
            onClick={onClose}
            disabled={importing}
            className="ios-btn flex h-9 items-center rounded-full border border-black/[0.06] bg-white/70 px-4 text-[12px] font-bold text-ios-textSecondary dark:border-white/[0.06] dark:bg-white/[0.04] dark:text-ios-textSecondaryDark"
          >
            取消
          </button>
          <button
            type="button"
            onClick={handleImport}
            disabled={importing || !validCount}
            className="ios-btn flex h-9 items-center gap-1.5 rounded-full bg-gradient-to-b from-[#3b82f6] to-ios-blue px-4 text-[12px] font-bold text-white shadow-md shadow-ios-blue/25 disabled:opacity-50"
          >
            {importing ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={2.6} />
            ) : (
              <CheckCircle2 className="h-3.5 w-3.5" strokeWidth={2.6} />
            )}
            {importing ? '导入中…' : `导入 ${validCount} 条`}
          </button>
        </div>
      </div>
    </div>
  )
}
