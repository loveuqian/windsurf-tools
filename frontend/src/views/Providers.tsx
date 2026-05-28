import { useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle,
  CheckCircle2,
  Layers,
  Pencil,
  Plus,
  Power,
  RefreshCcw,
  Save,
  Search,
  Trash2,
  X,
} from 'lucide-react'
import {
  useProviderAccountStore,
  type ProviderAccountModel,
} from '../stores/useProviderAccountStore'
import ImportModal from '../components/accounts/ProviderImportModal'
import {
  PROVIDER_DISPLAY_ORDER,
  PROVIDER_META,
  type ProviderID,
  type ProviderMeta,
} from '../utils/provider'
import { formatDateTimeAsiaShanghai } from '../utils/datetimeAsia'
import { showToast } from '../utils/toast'

// Providers 视图只展示通过「批量导入提供商」入库的 ProviderAccount，
// 与 Windsurf 号池(useAccountStore.accounts)物理隔离 —— 不复用 sk-* 前缀
// 误识别号池账号的旧逻辑(那是 detectProviderFromAccount 留给 Phase 0 的兜底)。

interface ProviderStat {
  meta: ProviderMeta
  total: number
  active: number
}

// 缺省卡片元信息：未识别 provider
const UNKNOWN_PROVIDER_META: ProviderMeta = {
  id: 'openai' as ProviderID,
  label: '未识别',
  tagline: '未在已知提供商名单内',
  host: '—',
  credentialKinds: [],
  accent: 'from-slate-400 via-slate-300 to-slate-200',
  badge: 'bg-slate-500/15 text-slate-700 dark:text-slate-300',
  initials: '??',
  envHint: { baseUrl: '', token: '', exampleBaseUrl: '', exampleToken: '' },
}

function normalizeProviderID(p: string | undefined): ProviderID | null {
  const v = String(p || '').trim().toLowerCase()
  if (!v) return null
  return (PROVIDER_DISPLAY_ORDER as string[]).includes(v) ? (v as ProviderID) : null
}

function getProviderMeta(acc: ProviderAccountModel): ProviderMeta {
  const id = normalizeProviderID(acc.provider)
  return id ? PROVIDER_META[id] : UNKNOWN_PROVIDER_META
}

function truncateMiddle(v: string, head = 12, tail = 6): string {
  const s = String(v || '').trim()
  if (s.length <= head + tail + 1) return s
  return s.slice(0, head) + '…' + s.slice(-tail)
}

function getDisplayName(acc: ProviderAccountModel): string {
  const nick = String(acc.nickname || '').trim()
  if (nick) return nick
  const meta = getProviderMeta(acc)
  return `${meta.label} 账号`
}

export default function Providers() {
  const accounts = useProviderAccountStore((s) => s.accounts)
  const isLoading = useProviderAccountStore((s) => s.isLoading)
  const isRefreshing = useProviderAccountStore((s) => s.isRefreshing)
  const ensureAccountsLoaded = useProviderAccountStore((s) => s.ensureAccountsLoaded)
  const fetchAccounts = useProviderAccountStore((s) => s.fetchAccounts)
  const updateAccount = useProviderAccountStore((s) => s.updateAccount)
  const deleteAccount = useProviderAccountStore((s) => s.deleteAccount)
  const refreshModelsAction = useProviderAccountStore((s) => s.refreshModels)

  const [searchQuery, setSearchQuery] = useState('')
  const [activeTab, setActiveTab] = useState<'all' | 'unknown' | ProviderID>('all')
  const [showImportModal, setShowImportModal] = useState(false)

  // 行内编辑：一次只允许一行进入编辑态
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState({ nickname: '', remark: '', status: 'active' })
  const [savingId, setSavingId] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [refreshingModelsId, setRefreshingModelsId] = useState<string | null>(null)
  const mountedRef = useRef(false)

  useEffect(() => {
    if (mountedRef.current) return
    mountedRef.current = true
    void ensureAccountsLoaded()
  }, [ensureAccountsLoaded])

  // ── 提供商分组统计 ──
  const providerStats = useMemo<ProviderStat[]>(() => {
    const map = new Map<ProviderID, ProviderStat>()
    for (const id of PROVIDER_DISPLAY_ORDER) {
      map.set(id, { meta: PROVIDER_META[id], total: 0, active: 0 })
    }
    for (const acc of accounts) {
      const id = normalizeProviderID(acc.provider)
      if (!id) continue
      const s = map.get(id)
      if (!s) continue
      s.total++
      if (acc.status !== 'disabled') s.active++
    }
    return PROVIDER_DISPLAY_ORDER.map((id) => map.get(id)!)
  }, [accounts])

  const unknownStat = useMemo(() => {
    let total = 0
    let active = 0
    for (const acc of accounts) {
      if (normalizeProviderID(acc.provider) !== null) continue
      total++
      if (acc.status !== 'disabled') active++
    }
    return { total, active }
  }, [accounts])

  const totalAvailable = useMemo(
    () => providerStats.reduce((s, b) => s + b.active, 0) + unknownStat.active,
    [providerStats, unknownStat],
  )
  const totalAccounts = accounts.length

  const tabsList = useMemo(() => {
    const tabs = providerStats.map((s) => ({
      key: s.meta.id as 'all' | 'unknown' | ProviderID,
      label: s.meta.label,
      badge: `${s.active}/${s.total}`,
    }))
    const list: Array<{ key: 'all' | 'unknown' | ProviderID; label: string; badge: string }> = [
      { key: 'all', label: '全部', badge: `${totalAvailable}/${totalAccounts}` },
      ...tabs,
    ]
    if (unknownStat.total > 0) {
      list.push({
        key: 'unknown',
        label: '未识别',
        badge: `${unknownStat.active}/${unknownStat.total}`,
      })
    }
    return list
  }, [providerStats, totalAvailable, totalAccounts, unknownStat])

  const filteredAccounts = useMemo<ProviderAccountModel[]>(() => {
    let list = accounts
    if (activeTab === 'unknown') {
      list = list.filter((a) => normalizeProviderID(a.provider) === null)
    } else if (activeTab !== 'all') {
      list = list.filter((a) => normalizeProviderID(a.provider) === activeTab)
    }
    const q = searchQuery.trim().toLowerCase()
    if (!q) return list
    return list.filter(
      (a) =>
        (a.provider || '').toLowerCase().includes(q) ||
        (a.nickname || '').toLowerCase().includes(q) ||
        (a.remark || '').toLowerCase().includes(q) ||
        (a.base_url || '').toLowerCase().includes(q) ||
        (a.auth_token || '').toLowerCase().includes(q),
    )
  }, [accounts, activeTab, searchQuery])

  const startEdit = (acc: ProviderAccountModel) => {
    setEditingId(acc.id)
    setEditDraft({
      nickname: acc.nickname ?? '',
      remark: acc.remark ?? '',
      status: acc.status || 'active',
    })
  }

  const cancelEdit = () => setEditingId(null)

  const saveEdit = async (acc: ProviderAccountModel) => {
    if (savingId) return
    setSavingId(acc.id)
    try {
      const next: ProviderAccountModel = {
        ...acc,
        nickname: editDraft.nickname.trim(),
        remark: editDraft.remark.trim(),
        status: editDraft.status || 'active',
      }
      await updateAccount(next)
      setEditingId(null)
      showToast('已保存', 'success')
    } catch (e: unknown) {
      showToast(`保存失败: ${String(e)}`, 'error')
    } finally {
      setSavingId(null)
    }
  }

  const toggleStatus = async (acc: ProviderAccountModel) => {
    if (savingId) return
    setSavingId(acc.id)
    try {
      const next: ProviderAccountModel = {
        ...acc,
        status: acc.status === 'disabled' ? 'active' : 'disabled',
      }
      await updateAccount(next)
      showToast(next.status === 'disabled' ? '已禁用' : '已启用', 'success')
    } catch (e: unknown) {
      showToast(`切换状态失败: ${String(e)}`, 'error')
    } finally {
      setSavingId(null)
    }
  }

  const handleDelete = async (acc: ProviderAccountModel) => {
    if (deletingId) return
    const ok = window.confirm(`确定删除「${getDisplayName(acc)}」吗?此操作不可撤销。`)
    if (!ok) return
    setDeletingId(acc.id)
    try {
      await deleteAccount(acc.id)
      showToast('已删除', 'success')
    } catch (e: unknown) {
      showToast(`删除失败: ${String(e)}`, 'error')
    } finally {
      setDeletingId(null)
    }
  }

  const handleRefresh = async () => {
    try {
      await fetchAccounts(true)
    } catch (e: unknown) {
      showToast(`刷新失败: ${String(e)}`, 'error')
    }
  }

  const toggleActivated = async (acc: ProviderAccountModel) => {
    if (savingId) return
    setSavingId(acc.id)
    try {
      const next: ProviderAccountModel = { ...acc, activated: !acc.activated }
      await updateAccount(next)
      showToast(next.activated ? '已激活' : '已取消激活', 'success')
    } catch (e: unknown) {
      showToast(`切换激活态失败: ${String(e)}`, 'error')
    } finally {
      setSavingId(null)
    }
  }

  const setActiveModel = async (acc: ProviderAccountModel, model: string) => {
    if (savingId) return
    if ((acc.active_model || '') === model) return
    setSavingId(acc.id)
    try {
      const next: ProviderAccountModel = { ...acc, active_model: model }
      await updateAccount(next)
    } catch (e: unknown) {
      showToast(`设置 active_model 失败: ${String(e)}`, 'error')
    } finally {
      setSavingId(null)
    }
  }

  const refreshModels = async (acc: ProviderAccountModel) => {
    if (refreshingModelsId) return
    setRefreshingModelsId(acc.id)
    try {
      await refreshModelsAction(acc.id)
      showToast('model 列表已更新', 'success')
    } catch (e: unknown) {
      showToast(`拉 model 列表失败: ${String(e)}`, 'error')
    } finally {
      setRefreshingModelsId(null)
    }
  }

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-5">
      {/* 顶部标题区 */}
      <header className="rounded-[28px] border border-black/[0.06] bg-white/80 p-5 shadow-[0_18px_44px_rgba(15,23,42,0.06)] backdrop-blur-2xl dark:border-white/[0.06] dark:bg-black/30">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className="flex h-12 w-12 items-center justify-center rounded-[18px] bg-gradient-to-br from-violet-500 via-fuchsia-400 to-rose-300 text-white shadow-[0_14px_30px_rgba(168,85,247,0.25)]">
              <Layers className="h-6 w-6" strokeWidth={2.4} />
            </div>
            <div>
              <h1 className="text-[22px] font-extrabold tracking-tight text-ios-text dark:text-ios-textDark">
                提供商
              </h1>
              <p className="text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                第三方 LLM 账号池(OpenAI / Anthropic / Google / DeepSeek …) — 与 Windsurf 号池物理隔离
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              className="ios-btn flex h-9 items-center gap-1.5 rounded-full border border-black/[0.06] bg-white/70 px-3 text-[12px] font-bold text-ios-text shadow-sm dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textDark"
              disabled={isLoading || isRefreshing}
              onClick={handleRefresh}
            >
              <span>{isRefreshing ? '刷新中…' : '刷新'}</span>
            </button>
            <button
              type="button"
              className="ios-btn flex h-9 items-center gap-1.5 rounded-full bg-gradient-to-b from-[#3b82f6] to-ios-blue px-3 text-[12px] font-bold text-white shadow-md shadow-ios-blue/25"
              onClick={() => setShowImportModal(true)}
            >
              <Plus className="h-3.5 w-3.5" strokeWidth={2.6} />
              <span>批量导入</span>
            </button>
          </div>
        </div>

        {/* 搜索 + tab 条 */}
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <div className="relative flex h-9 min-w-[220px] flex-1 items-center rounded-full border border-black/[0.06] bg-white/80 px-3 shadow-sm dark:border-white/[0.08] dark:bg-white/[0.04]">
            <Search className="h-4 w-4 text-ios-textSecondary dark:text-ios-textSecondaryDark" strokeWidth={2.4} />
            <input
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="ml-2 flex-1 bg-transparent text-[13px] outline-none placeholder:text-ios-textSecondary/70 dark:placeholder:text-ios-textSecondaryDark/70"
              placeholder="搜索 nickname / remark / base_url / token …"
            />
            {searchQuery && (
              <button
                type="button"
                className="ios-btn rounded-full p-1 hover:bg-black/[0.06] dark:hover:bg-white/[0.08]"
                onClick={() => setSearchQuery('')}
              >
                <X className="h-3.5 w-3.5" strokeWidth={2.4} />
              </button>
            )}
          </div>
        </div>
        <div className="mt-3 flex flex-wrap gap-1.5">
          {tabsList.map((tab) => (
            <button
              key={tab.key}
              type="button"
              className={`ios-btn flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12px] font-bold transition-all ${
                activeTab === tab.key
                  ? 'border-ios-blue/40 bg-ios-blue/[0.12] text-ios-blue'
                  : 'border-black/[0.06] bg-white/70 text-ios-textSecondary hover:text-ios-text dark:border-white/[0.06] dark:bg-white/[0.04] dark:text-ios-textSecondaryDark dark:hover:text-ios-textDark'
              }`}
              onClick={() => setActiveTab(tab.key)}
            >
              <span>{tab.label}</span>
              <span className="rounded-full bg-black/[0.06] px-1.5 py-0.5 text-[10px] font-black tabular-nums text-ios-textSecondary dark:bg-white/[0.1] dark:text-ios-textSecondaryDark">
                {tab.badge}
              </span>
            </button>
          ))}
        </div>
      </header>

      {/* 账号列表 */}
      {filteredAccounts.length > 0 ? (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {filteredAccounts.map((acc) => (
            <ProviderCard
              key={acc.id}
              acc={acc}
              isEditing={editingId === acc.id}
              editDraft={editDraft}
              setEditDraft={setEditDraft}
              savingId={savingId}
              deletingId={deletingId}
              refreshingModelsId={refreshingModelsId}
              onStartEdit={() => startEdit(acc)}
              onCancelEdit={cancelEdit}
              onSaveEdit={() => saveEdit(acc)}
              onToggleStatus={() => toggleStatus(acc)}
              onDelete={() => handleDelete(acc)}
              onToggleActivated={() => toggleActivated(acc)}
              onSetActiveModel={(m) => setActiveModel(acc, m)}
              onRefreshModels={() => refreshModels(acc)}
            />
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center gap-4 rounded-[28px] border border-dashed border-black/[0.1] bg-white/60 p-12 text-center dark:border-white/[0.08] dark:bg-black/20">
          <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-violet-500/15 to-fuchsia-500/10 text-violet-600 dark:text-violet-300">
            <Layers className="h-8 w-8" strokeWidth={2.2} />
          </div>
          <div className="space-y-1">
            <h3 className="text-[15px] font-extrabold text-ios-text dark:text-ios-textDark">
              {accounts.length === 0 ? '还没有任何提供商账号' : '当前 tab / 搜索没有结果'}
            </h3>
            <p className="text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
              {accounts.length === 0
                ? '点击右上角「批量导入」粘贴 base_url + token,落库即可'
                : '切换其它 tab 或清空搜索关键字'}
            </p>
          </div>
          {accounts.length === 0 && (
            <button
              type="button"
              className="ios-btn flex h-9 items-center gap-1.5 rounded-full bg-gradient-to-b from-[#3b82f6] to-ios-blue px-4 text-[12px] font-bold text-white shadow-md shadow-ios-blue/25"
              onClick={() => setShowImportModal(true)}
            >
              <Plus className="h-3.5 w-3.5" strokeWidth={2.6} />
              批量导入提供商账号
            </button>
          )}
        </div>
      )}

      <ImportModal
        isOpen={showImportModal}
        defaultProvider={
          activeTab !== 'all' && activeTab !== 'unknown' ? (activeTab as ProviderID) : null
        }
        onClose={() => setShowImportModal(false)}
      />
    </div>
  )
}

// ── ProviderCard 子组件 ──

interface ProviderCardProps {
  acc: ProviderAccountModel
  isEditing: boolean
  editDraft: { nickname: string; remark: string; status: string }
  setEditDraft: (d: { nickname: string; remark: string; status: string }) => void
  savingId: string | null
  deletingId: string | null
  refreshingModelsId: string | null
  onStartEdit: () => void
  onCancelEdit: () => void
  onSaveEdit: () => void
  onToggleStatus: () => void
  onDelete: () => void
  onToggleActivated: () => void
  onSetActiveModel: (model: string) => void
  onRefreshModels: () => void
}

function ProviderCard(props: ProviderCardProps) {
  const {
    acc, isEditing, editDraft, setEditDraft,
    savingId, deletingId, refreshingModelsId,
    onStartEdit, onCancelEdit, onSaveEdit, onToggleStatus, onDelete,
    onToggleActivated, onSetActiveModel, onRefreshModels,
  } = props
  const meta = getProviderMeta(acc)
  const cardCls = acc.activated
    ? 'border-violet-500/40 ring-2 ring-violet-500/30 ring-offset-1 ring-offset-white dark:ring-offset-black/40'
    : 'border-black/[0.06] hover:border-ios-blue/30 dark:border-white/[0.06]'

  return (
    <div className={`group relative flex flex-col gap-3 rounded-[24px] border bg-white/80 p-4 shadow-[0_14px_36px_rgba(15,23,42,0.06)] backdrop-blur-xl transition-all dark:bg-black/25 ${cardCls}`}>
      {acc.activated && (
        <span className="absolute -top-2 -right-2 inline-flex items-center gap-1 rounded-full bg-gradient-to-b from-violet-500 to-fuchsia-500 px-2.5 py-0.5 text-[10px] font-extrabold uppercase tracking-wide text-white shadow-md shadow-violet-500/40">
          ★ 当前
        </span>
      )}
      {/* 头部:provider 徽章 + 状态 */}
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <div className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-[14px] bg-gradient-to-br text-[13px] font-extrabold text-white shadow-md ${meta.accent}`}>
            {meta.initials}
          </div>
          <div className="min-w-0">
            <div
              className="truncate text-[14px] font-extrabold text-ios-text dark:text-ios-textDark"
              title={getDisplayName(acc)}
            >
              {getDisplayName(acc)}
            </div>
            <div className="flex items-center gap-1.5">
              <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-bold tracking-wide ${meta.badge}`}>
                {meta.label}
              </span>
              <span className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                {formatDateTimeAsiaShanghai(acc.created_at)}
              </span>
            </div>
          </div>
        </div>
        <span className={`inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-bold ${
          acc.status === 'disabled'
            ? 'border-rose-500/15 bg-rose-500/[0.08] text-rose-700 dark:text-rose-300'
            : 'border-emerald-500/15 bg-emerald-500/[0.08] text-emerald-700 dark:text-emerald-300'
        }`}>
          {acc.status !== 'disabled' ? <CheckCircle2 className="h-3 w-3" strokeWidth={2.6} /> : <AlertCircle className="h-3 w-3" strokeWidth={2.6} />}
          {acc.status === 'disabled' ? '已禁用' : '可用'}
        </span>
      </div>

      {!isEditing ? (
        // 凭证摘要(展示态)
        <div className="space-y-1.5">
          <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
            <span className="font-bold uppercase tracking-[0.16em]">Base URL</span>
            <code className="mt-0.5 block break-all rounded-md bg-black/[0.04] px-2 py-1 font-mono text-[11px] text-ios-text dark:bg-white/[0.06] dark:text-ios-textDark">
              {acc.base_url || '—'}
            </code>
          </div>
          <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
            <span className="font-bold uppercase tracking-[0.16em]">Token</span>
            <code
              className="mt-0.5 block truncate rounded-md bg-black/[0.04] px-2 py-1 font-mono text-[11px] text-ios-text dark:bg-white/[0.06] dark:text-ios-textDark"
              title={acc.auth_token}
            >
              {truncateMiddle(acc.auth_token, 14, 6)}
            </code>
          </div>
          {acc.remark && (
            <div className="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark" title={acc.remark}>
              <span className="font-bold uppercase tracking-[0.16em]">Remark</span>
              <span className="ml-1 line-clamp-2">{acc.remark}</span>
            </div>
          )}

          {/* 阶段 2: 路由调度区(激活开关 + active_model 下拉) */}
          <div className="rounded-[14px] border border-dashed border-violet-500/20 bg-violet-500/[0.04] px-3 py-2.5 space-y-2">
            <div className="flex items-center justify-between gap-2">
              <span className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                <Power className="h-3 w-3" strokeWidth={2.6} />
                提供商接管
              </span>
              <button
                type="button"
                className={`ios-btn flex h-6 items-center gap-1 rounded-full px-2.5 text-[10px] font-bold transition-all ${
                  acc.activated
                    ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'
                    : 'bg-black/[0.06] text-ios-textSecondary dark:bg-white/[0.08] dark:text-ios-textSecondaryDark'
                }`}
                disabled={savingId === acc.id}
                onClick={onToggleActivated}
              >
                {acc.activated ? '已激活' : '未激活'}
              </button>
            </div>
            <div className="flex items-center gap-1.5">
              <select
                value={acc.active_model || ''}
                className="flex-1 min-w-0 rounded-[10px] border border-black/[0.08] bg-white px-2 py-1.5 text-[11px] font-mono outline-none focus:border-ios-blue/60 dark:border-white/[0.08] dark:bg-white/[0.06] disabled:opacity-50"
                disabled={savingId === acc.id || !(acc.models && acc.models.length)}
                onChange={(e) => onSetActiveModel(e.target.value)}
              >
                <option value="" disabled>
                  {acc.models && acc.models.length ? '选择 active model' : '未发现 model — 点右侧刷新'}
                </option>
                {(acc.models || []).map((m) => (
                  <option key={m} value={m}>{m}</option>
                ))}
              </select>
              <button
                type="button"
                className="ios-btn flex h-7 w-7 shrink-0 items-center justify-center rounded-[10px] border border-black/[0.06] bg-white/80 text-ios-textSecondary hover:text-ios-blue dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textSecondaryDark dark:hover:text-ios-blue disabled:opacity-50"
                disabled={refreshingModelsId === acc.id}
                title="重新拉取 /v1/models"
                onClick={onRefreshModels}
              >
                <RefreshCcw
                  className={`h-3 w-3 ${refreshingModelsId === acc.id ? 'animate-spin' : ''}`}
                  strokeWidth={2.6}
                />
              </button>
            </div>
            {acc.models_error ? (
              <div className="text-[10px] text-rose-700 dark:text-rose-300 line-clamp-2" title={acc.models_error}>
                ↳ {acc.models_error}
              </div>
            ) : acc.models_refreshed_at ? (
              <div className="text-[10px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                ↳ {(acc.models || []).length} 个 model · 最近 {formatDateTimeAsiaShanghai(acc.models_refreshed_at)}
              </div>
            ) : null}
          </div>
        </div>
      ) : (
        // 编辑态
        <div className="space-y-2">
          <label className="block">
            <span className="block text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
              Nickname
            </span>
            <input
              value={editDraft.nickname}
              onChange={(e) => setEditDraft({ ...editDraft, nickname: e.target.value })}
              className="mt-1 w-full rounded-[10px] border border-black/[0.08] bg-white px-2.5 py-1.5 text-[12px] outline-none focus:border-ios-blue/60 dark:border-white/[0.08] dark:bg-white/[0.06]"
              placeholder="（可选）"
            />
          </label>
          <label className="block">
            <span className="block text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
              Remark
            </span>
            <input
              value={editDraft.remark}
              onChange={(e) => setEditDraft({ ...editDraft, remark: e.target.value })}
              className="mt-1 w-full rounded-[10px] border border-black/[0.08] bg-white px-2.5 py-1.5 text-[12px] outline-none focus:border-ios-blue/60 dark:border-white/[0.08] dark:bg-white/[0.06]"
              placeholder="（可选）"
            />
          </label>
          <label className="block">
            <span className="block text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
              Status
            </span>
            <select
              value={editDraft.status}
              onChange={(e) => setEditDraft({ ...editDraft, status: e.target.value })}
              className="mt-1 w-full rounded-[10px] border border-black/[0.08] bg-white px-2.5 py-1.5 text-[12px] outline-none focus:border-ios-blue/60 dark:border-white/[0.08] dark:bg-white/[0.06]"
            >
              <option value="active">active</option>
              <option value="disabled">disabled</option>
            </select>
          </label>
        </div>
      )}

      {/* 操作区 */}
      <div className="mt-auto flex flex-wrap items-center justify-end gap-1.5">
        {isEditing ? (
          <>
            <button
              type="button"
              className="ios-btn flex h-8 items-center gap-1 rounded-full border border-black/[0.06] bg-white/70 px-3 text-[11px] font-bold text-ios-textSecondary hover:text-ios-text dark:border-white/[0.06] dark:bg-white/[0.04] dark:text-ios-textSecondaryDark dark:hover:text-ios-textDark"
              disabled={savingId === acc.id}
              onClick={onCancelEdit}
            >
              取消
            </button>
            <button
              type="button"
              className="ios-btn flex h-8 items-center gap-1 rounded-full bg-gradient-to-b from-[#3b82f6] to-ios-blue px-3 text-[11px] font-bold text-white shadow-md shadow-ios-blue/25 disabled:opacity-50"
              disabled={savingId === acc.id}
              onClick={onSaveEdit}
            >
              <Save className="h-3 w-3" strokeWidth={2.6} />
              {savingId === acc.id ? '保存中…' : '保存'}
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              className="ios-btn flex h-8 items-center gap-1 rounded-full border border-black/[0.06] bg-white/70 px-3 text-[11px] font-bold text-ios-textSecondary hover:text-ios-text dark:border-white/[0.06] dark:bg-white/[0.04] dark:text-ios-textSecondaryDark dark:hover:text-ios-textDark"
              disabled={savingId === acc.id}
              onClick={onToggleStatus}
            >
              {acc.status === 'disabled' ? '启用' : '禁用'}
            </button>
            <button
              type="button"
              className="ios-btn flex h-8 items-center gap-1 rounded-full border border-black/[0.06] bg-white/70 px-3 text-[11px] font-bold text-ios-textSecondary hover:text-ios-text dark:border-white/[0.06] dark:bg-white/[0.04] dark:text-ios-textSecondaryDark dark:hover:text-ios-textDark"
              onClick={onStartEdit}
            >
              <Pencil className="h-3 w-3" strokeWidth={2.6} />
              编辑
            </button>
            <button
              type="button"
              className="ios-btn flex h-8 items-center gap-1 rounded-full border border-rose-500/20 bg-rose-500/[0.08] px-3 text-[11px] font-bold text-rose-700 hover:bg-rose-500/[0.14] dark:text-rose-300 disabled:opacity-50"
              disabled={deletingId === acc.id}
              onClick={onDelete}
            >
              <Trash2 className="h-3 w-3" strokeWidth={2.6} />
              {deletingId === acc.id ? '删除中…' : '删除'}
            </button>
          </>
        )}
      </div>
    </div>
  )
}
