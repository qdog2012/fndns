import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  ChevronRight,
  CirclePause,
  CloudOff,
  Copy,
  DatabaseZap,
  Edit3,
  Filter,
  Globe2,
  ListFilter,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Search,
  ServerCog,
  Trash2,
  X,
} from 'lucide-react'
import { FormEvent, useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import { Button, EmptyState, Field, Modal, ProviderMark } from '../components'
import type { Capability, Credential, DNSRecord, Domain, Overview, Provider, RecordInput } from '../types'
import { formatTime, isEnabled, providerName, recordStatus, relativeTime } from '../utils'

interface DomainsProps {
  domains: Domain[]
  credentials: Credential[]
  overview?: Overview
  loading: boolean
  onReload: () => Promise<void>
  onOpenCredentials: () => void
  notify: (type: 'success' | 'error', message: string) => void
}

export default function DomainsPage(props: DomainsProps) {
  const [selectedDomain, setSelectedDomain] = useState<string | null>(null)
  if (selectedDomain) {
    return <RecordsPage domainId={selectedDomain} onBack={() => { setSelectedDomain(null); void props.onReload() }} notify={props.notify} />
  }
  return <DomainList {...props} onSelect={setSelectedDomain} />
}

function DomainList({ domains, credentials, overview, loading, onReload, onOpenCredentials, notify, onSelect }: DomainsProps & { onSelect: (id: string) => void }) {
  const [query, setQuery] = useState('')
  const [providerFilter, setProviderFilter] = useState<'all' | Provider>('all')
  const [refreshing, setRefreshing] = useState(false)
  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return domains.filter((domain) => (providerFilter === 'all' || domain.provider === providerFilter) && (!needle || domain.name.toLowerCase().includes(needle) || domain.credentialName.toLowerCase().includes(needle)))
  }, [domains, query, providerFilter])

  const refreshAll = async () => {
    if (credentials.length === 0) return
    setRefreshing(true)
    try {
      const results = await api.refreshAll()
      const failures = results.filter((result) => !result.success)
      notify(failures.length ? 'error' : 'success', failures.length ? `${results.length - failures.length} 组成功，${failures.length} 组同步失败` : `已同步 ${results.length} 组凭据的域名`)
      await onReload()
    } catch (error) {
      notify('error', error instanceof Error ? error.message : '同步失败')
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <main className="page-content page-enter">
      <div className="page-heading domain-heading">
        <div>
          <span className="eyebrow">DNS OVERVIEW</span>
          <h1>域名</h1>
          <p>跨凭据汇总展示；同名域名按凭据创建顺序保留第一个。</p>
        </div>
        <Button variant="primary" onClick={refreshAll} loading={refreshing} disabled={!credentials.length}><RefreshCw size={17} />全部同步</Button>
      </div>

      <section className="metric-ribbon" aria-label="DNS 概览">
        <div className="metric-primary"><span className="metric-orb"><Globe2 size={21} /></span><div><strong>{overview?.domainCount ?? domains.length}</strong><span>域名总数</span></div></div>
        <div><strong>{overview?.recordCount ?? 0}</strong><span>缓存记录</span></div>
        <div><strong>{overview?.credentialCount ?? credentials.length}</strong><span>API 凭据</span></div>
        <div className={overview?.credentialFailures ? 'metric-warning' : ''}><strong>{overview?.credentialFailures ?? 0}</strong><span>异常连接</span></div>
        <div className="metric-last"><strong>{relativeTime(overview?.lastSyncAt)}</strong><span>最近同步</span></div>
      </section>

      {overview?.credentialFailures ? (
        <div className="page-alert"><CloudOff size={18} /><div><strong>部分凭据同步异常</strong><span>当前仍在展示本地缓存。修复凭据或网络后请手动同步。</span></div><button onClick={onOpenCredentials}>查看凭据</button></div>
      ) : null}

      <div className="list-toolbar">
        <label className="search-box"><Search size={17} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索域名或凭据名称" />{query && <button onClick={() => setQuery('')} aria-label="清除"><X size={15} /></button>}</label>
        <div className="segmented" aria-label="平台筛选">
          <button className={providerFilter === 'all' ? 'active' : ''} onClick={() => setProviderFilter('all')}>全部</button>
          <button className={providerFilter === 'cloudflare' ? 'active' : ''} onClick={() => setProviderFilter('cloudflare')}>Cloudflare</button>
          <button className={providerFilter === 'tencent' ? 'active' : ''} onClick={() => setProviderFilter('tencent')}>DNSPod</button>
        </div>
      </div>

      {!loading && credentials.length === 0 ? (
        <EmptyState icon={<ServerCog size={26} />} title="先连接一个 DNS 平台" description="添加 API 凭据后，应用会验证权限并拉取域名。密钥会在本机加密保存。" action={<Button variant="primary" onClick={onOpenCredentials}><Plus size={17} />添加 API 凭据</Button>} />
      ) : !loading && domains.length === 0 ? (
        <EmptyState icon={<Globe2 size={26} />} title="缓存中还没有域名" description="凭据已经添加。点击“全部同步”从平台手动拉取域名列表。" action={<Button variant="primary" onClick={refreshAll}><RefreshCw size={17} />立即同步</Button>} />
      ) : (
        <section className="data-panel">
          <div className="table-scroll desktop-table">
            <table className="domain-table">
              <thead><tr><th>域名</th><th>平台 / 凭据</th><th>记录</th><th>缓存状态</th><th>最后同步</th><th aria-label="操作" /></tr></thead>
              <tbody>
                {visible.map((domain) => (
                  <tr key={domain.id} onClick={() => onSelect(domain.id)} tabIndex={0} onKeyDown={(event) => event.key === 'Enter' && onSelect(domain.id)}>
                    <td><div className="domain-name"><span className="favicon-dot">{domain.name.slice(0, 1).toUpperCase()}</span><div><strong>{domain.name}</strong><span>{domain.grade || '标准域名'}</span></div></div></td>
                    <td><div className="source-cell"><ProviderMark provider={domain.provider} compact /><span>{domain.credentialName}</span></div></td>
                    <td><strong className="record-count">{domain.recordCount}</strong></td>
                    <td><CacheBadge domain={domain} /></td>
                    <td><span className="subtle" title={formatTime(domain.lastSyncAt, true)}>{relativeTime(domain.lastSyncAt)}</span></td>
                    <td><ChevronRight className="row-arrow" size={18} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="mobile-domain-list">
            {visible.map((domain) => (
              <button className="mobile-domain-card" key={domain.id} onClick={() => onSelect(domain.id)}>
                <span className="favicon-dot">{domain.name.slice(0, 1).toUpperCase()}</span>
                <span className="mobile-domain-main"><strong>{domain.name}</strong><span><ProviderMark provider={domain.provider} compact />{domain.credentialName}</span></span>
                <span className="mobile-domain-meta"><strong>{domain.recordCount}</strong><span>记录</span></span>
                <ChevronRight size={17} />
              </button>
            ))}
          </div>
          {visible.length === 0 && <div className="no-results"><ListFilter size={20} /><span>没有符合筛选条件的域名</span></div>}
        </section>
      )}
    </main>
  )
}

function CacheBadge({ domain }: { domain: Domain }) {
  if (domain.syncError) return <span className="cache-badge cache-error"><AlertTriangle size={13} />同步失败</span>
  if (domain.cacheState === 'expired') return <span className="cache-badge cache-expired"><CirclePause size={13} />已过期</span>
  if (domain.cacheState === 'never') return <span className="cache-badge cache-never"><DatabaseZap size={13} />待同步</span>
  return <span className="cache-badge cache-fresh"><CheckCircle2 size={13} />可用</span>
}

function RecordsPage({ domainId, onBack, notify }: { domainId: string; onBack: () => void; notify: (type: 'success' | 'error', message: string) => void }) {
  const [domain, setDomain] = useState<Domain | null>(null)
  const [records, setRecords] = useState<DNSRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [query, setQuery] = useState('')
  const [typeFilter, setTypeFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [editing, setEditing] = useState<DNSRecord | 'new' | null>(null)
  const [busy, setBusy] = useState(false)

  const load = async () => {
    try {
      const data = await api.records(domainId)
      setDomain(data.domain)
      setRecords(data.records)
      setSelected(new Set())
    } catch (error) {
      notify('error', error instanceof Error ? error.message : '读取解析记录失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [domainId])

  const types = useMemo(() => Array.from(new Set(records.map((record) => record.type))).sort(), [records])
  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return records.filter((record) =>
      (typeFilter === 'all' || record.type === typeFilter) &&
      (statusFilter === 'all' || (statusFilter === 'enabled' ? isEnabled(record.status) : !isEnabled(record.status))) &&
      (!needle || record.name.toLowerCase().includes(needle) || record.value.toLowerCase().includes(needle) || record.type.toLowerCase().includes(needle)),
    )
  }, [records, query, statusFilter, typeFilter])

  const refresh = async () => {
    setRefreshing(true)
    try {
      await api.refreshDomain(domainId)
      await load()
      notify('success', `${domain?.name || '域名'} 的解析记录已同步`)
    } catch (error) {
      notify('error', error instanceof Error ? error.message : '同步失败')
      await load()
    } finally {
      setRefreshing(false)
    }
  }

  const removeOne = async (record: DNSRecord) => {
    if (!window.confirm(`确定删除 ${record.type} 记录“${record.name}”吗？此操作会立即提交到 ${domain ? providerName(domain.provider) : '平台'}。`)) return
    setBusy(true)
    try {
      await api.deleteRecord(domainId, record.id)
      notify('success', '解析记录已删除')
      await load()
    } catch (error) {
      notify('error', error instanceof Error ? error.message : '删除失败')
    } finally { setBusy(false) }
  }

  const toggleStatus = async (record: DNSRecord) => {
    if (!record.supportsDisable) return
    setBusy(true)
    try {
      const enabled = !isEnabled(record.status)
      await api.setRecordStatus(domainId, record.id, enabled)
      notify('success', enabled ? '解析记录已启用' : '解析记录已暂停')
      await load()
    } catch (error) {
      notify('error', error instanceof Error ? error.message : '操作失败')
    } finally { setBusy(false) }
  }

  const batch = async (action: 'delete' | 'enable' | 'disable') => {
    const ids = Array.from(selected)
    if (!ids.length) return
    const label = action === 'delete' ? '删除' : action === 'enable' ? '启用' : '暂停'
    if ((action === 'delete' || action === 'disable') && !window.confirm(`确定批量${label}选中的 ${ids.length} 条记录吗？`)) return
    setBusy(true)
    try {
      const results = action === 'delete' ? await api.batchDelete(domainId, ids) : await api.batchStatus(domainId, ids, action === 'enable')
      const failed = results.filter((result) => !result.success)
      notify(failed.length ? 'error' : 'success', failed.length ? `${results.length - failed.length} 条成功，${failed.length} 条失败` : `已批量${label} ${results.length} 条记录`)
      await load()
    } catch (error) {
      notify('error', error instanceof Error ? error.message : `批量${label}失败`)
    } finally { setBusy(false) }
  }

  const toggleSelection = (id: string) => setSelected((current) => { const next = new Set(current); next.has(id) ? next.delete(id) : next.add(id); return next })
  const allVisibleSelected = visible.length > 0 && visible.every((record) => selected.has(record.id))
  const mutationDisabled = !domain?.lastSyncAt || domain.cacheState === 'expired' || Boolean(domain.syncError)

  return (
    <main className="page-content records-page page-enter">
      <button className="back-button" onClick={onBack}><ArrowLeft size={17} />返回域名</button>
      <div className="record-heading">
        <div className="record-title">
          <span className="favicon-dot large">{domain?.name.slice(0, 1).toUpperCase() || 'D'}</span>
          <div><h1>{domain?.name || '加载中…'}</h1><span>{domain && <><ProviderMark provider={domain.provider} compact />{domain.credentialName}</>}</span></div>
        </div>
        <div className="record-actions">
          <Button variant="secondary" loading={refreshing} onClick={refresh}><RefreshCw size={16} />同步记录</Button>
          <Button variant="primary" disabled={mutationDisabled} onClick={() => setEditing('new')}><Plus size={17} />新增记录</Button>
        </div>
      </div>

      {domain && (domain.cacheState === 'expired' || domain.syncError || !domain.lastSyncAt) && (
        <div className={`stale-banner ${domain.syncError ? 'is-error' : ''}`}>
          <AlertTriangle size={18} />
          <div><strong>{domain.syncError ? '最近同步失败，当前缓存只读' : domain.cacheState === 'expired' ? '缓存已超过 6 个月，当前只读' : '尚未同步解析记录'}</strong><span>{domain.syncError || '请先手动同步，确认远端最新状态后再修改记录。'}</span></div>
          <Button variant="secondary" onClick={refresh} loading={refreshing}>立即同步</Button>
        </div>
      )}

      <div className="record-summary-line">
        <span><strong>{records.length}</strong> 条记录</span><i />
        <span>缓存于 {formatTime(domain?.lastSyncAt, true)}</span><i />
        <span>不会自动刷新</span>
      </div>

      <div className="list-toolbar record-toolbar">
        <label className="search-box"><Search size={17} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索主机记录、类型或记录值" />{query && <button onClick={() => setQuery('')} aria-label="清除"><X size={15} /></button>}</label>
        <label className="select-control"><Filter size={15} /><select value={typeFilter} onChange={(event) => setTypeFilter(event.target.value)}><option value="all">全部类型</option>{types.map((type) => <option value={type} key={type}>{type}</option>)}</select></label>
        <label className="select-control"><select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}><option value="all">全部状态</option><option value="enabled">已启用</option><option value="disabled">已暂停</option></select></label>
      </div>

      {selected.size > 0 && (
        <div className="batch-bar">
          <span>已选择 <strong>{selected.size}</strong> 条</span>
          <div>
            <Button variant="ghost" disabled={busy || domain?.provider === 'cloudflare'} onClick={() => batch('enable')}><Play size={15} />启用</Button>
            <Button variant="ghost" disabled={busy || domain?.provider === 'cloudflare'} onClick={() => batch('disable')}><Pause size={15} />暂停</Button>
            <Button variant="danger" disabled={busy} onClick={() => batch('delete')}><Trash2 size={15} />删除</Button>
            <button className="icon-button" onClick={() => setSelected(new Set())} aria-label="取消选择"><X size={16} /></button>
          </div>
        </div>
      )}

      {!loading && records.length === 0 ? (
        <EmptyState icon={<DatabaseZap size={25} />} title="缓存中还没有解析记录" description="应用不会自动请求平台。先手动同步一次，再新增或管理记录。" action={<Button variant="primary" onClick={refresh}><RefreshCw size={17} />同步解析记录</Button>} />
      ) : (
        <section className="data-panel record-panel">
          <div className="table-scroll desktop-table">
            <table className="record-table">
              <thead><tr><th className="checkbox-cell"><input type="checkbox" checked={allVisibleSelected} onChange={() => setSelected(allVisibleSelected ? new Set() : new Set(visible.map((record) => record.id)))} aria-label="全选" /></th><th>主机记录</th><th>类型</th><th>记录值</th><th>线路 / TTL</th><th>状态</th><th aria-label="操作" /></tr></thead>
              <tbody>
                {visible.map((record) => (
                  <tr key={record.id} className={selected.has(record.id) ? 'selected-row' : ''}>
                    <td className="checkbox-cell"><input type="checkbox" checked={selected.has(record.id)} onChange={() => toggleSelection(record.id)} aria-label={`选择 ${record.name}`} /></td>
                    <td><strong className="host-name">{record.name}</strong>{record.remark && <span className="row-note">{record.remark}</span>}</td>
                    <td><span className={`record-type type-${record.type.toLowerCase()}`}>{record.type}</span></td>
                    <td><div className="record-value"><code title={record.value}>{record.value}</code><button onClick={() => { void navigator.clipboard?.writeText(record.value); notify('success', '记录值已复制') }} aria-label="复制记录值"><Copy size={14} /></button></div>{record.proxied && <span className="proxy-note">橙云代理</span>}</td>
                    <td><span className="line-value">{record.line || '默认'}</span><span className="ttl-value">TTL {record.ttl === 1 && domain?.provider === 'cloudflare' ? '自动' : `${record.ttl}s`}</span></td>
                    <td><span className={`status-dot ${isEnabled(record.status) ? 'enabled' : 'disabled'}`}><i />{recordStatus(record.status)}</span></td>
                    <td><div className="row-actions"><button className="icon-button" onClick={() => setEditing(record)} disabled={mutationDisabled || busy} aria-label="编辑"><Edit3 size={15} /></button><button className="icon-button" onClick={() => toggleStatus(record)} disabled={!record.supportsDisable || mutationDisabled || busy} title={record.supportsDisable ? (isEnabled(record.status) ? '暂停' : '启用') : 'Cloudflare 不支持暂停单条记录'} aria-label="启停">{isEnabled(record.status) ? <Pause size={15} /> : <Play size={15} />}</button><button className="icon-button danger-icon" onClick={() => removeOne(record)} disabled={mutationDisabled || busy} aria-label="删除"><Trash2 size={15} /></button></div></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="mobile-record-list">
            {visible.map((record) => (
              <article className={`mobile-record-card ${selected.has(record.id) ? 'selected' : ''}`} key={record.id}>
                <header><label><input type="checkbox" checked={selected.has(record.id)} onChange={() => toggleSelection(record.id)} /><span className={`record-type type-${record.type.toLowerCase()}`}>{record.type}</span><strong>{record.name}</strong></label><span className={`status-dot ${isEnabled(record.status) ? 'enabled' : 'disabled'}`}><i />{recordStatus(record.status)}</span></header>
                <code>{record.value}</code>
                <div className="mobile-record-meta"><span>{record.line || '默认'} · TTL {record.ttl === 1 && domain?.provider === 'cloudflare' ? '自动' : record.ttl}</span>{record.proxied && <span className="proxy-note">橙云代理</span>}</div>
                <footer><Button variant="ghost" onClick={() => setEditing(record)} disabled={mutationDisabled || busy}><Edit3 size={15} />编辑</Button><Button variant="ghost" onClick={() => toggleStatus(record)} disabled={!record.supportsDisable || mutationDisabled || busy}>{isEnabled(record.status) ? <Pause size={15} /> : <Play size={15} />}{record.supportsDisable ? (isEnabled(record.status) ? '暂停' : '启用') : '不支持暂停'}</Button><button className="icon-button danger-icon" onClick={() => removeOne(record)} disabled={mutationDisabled || busy}><Trash2 size={15} /></button></footer>
              </article>
            ))}
          </div>
          {visible.length === 0 && <div className="no-results"><ListFilter size={20} /><span>没有符合筛选条件的解析记录</span></div>}
        </section>
      )}
      {editing && domain && <RecordModal domain={domain} record={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)} onSaved={async (message) => { setEditing(null); notify('success', message); await load() }} />}
    </main>
  )
}

function RecordModal({ domain, record, onClose, onSaved }: { domain: Domain; record?: DNSRecord; onClose: () => void; onSaved: (message: string) => Promise<void> }) {
  const [capability, setCapability] = useState<Capability | null>(null)
  const [input, setInput] = useState<RecordInput>({ name: '@', type: 'A', value: '', ttl: domain.provider === 'cloudflare' ? 1 : 600, line: domain.provider === 'tencent' ? '默认' : '', mx: 0, weight: 0, proxied: false, status: 'enable', remark: '' })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    void api.capability(domain.provider).then((data) => {
      setCapability(data)
      if (!record) setInput((current) => ({ ...current, ttl: data.defaultTtl }))
    }).catch((caught) => setError(caught instanceof Error ? caught.message : '读取平台能力失败'))
    if (record) setInput({ name: record.name, type: record.type, value: record.value, ttl: record.ttl, line: record.line || (domain.provider === 'tencent' ? '默认' : ''), lineId: record.lineId, mx: record.mx || 0, weight: record.weight || 0, proxied: record.proxied, status: isEnabled(record.status) ? 'enable' : 'disable', remark: record.remark || '' })
  }, [domain.provider, record])

  const set = <K extends keyof RecordInput>(field: K, value: RecordInput[K]) => setInput((current) => ({ ...current, [field]: value }))
  const supportsProxyType = ['A', 'AAAA', 'CNAME'].includes(input.type)
  const showPriority = input.type === 'MX' || input.type === 'SRV'

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    if (!input.name.trim() || !input.value.trim()) return setError('请填写主机记录和记录值')
    setSaving(true)
    try {
      if (record) {
        await api.updateRecord(domain.id, record.id, input)
        await onSaved('解析记录已更新')
      } else {
        await api.createRecord(domain.id, input)
        await onSaved('解析记录已新增')
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '保存失败')
    } finally { setSaving(false) }
  }

  const valueHint = input.type === 'SRV' ? '格式：端口 目标，例如“443 service.example.com”' : input.type === 'CAA' ? '格式：标志 标签 值，例如“0 issue letsencrypt.org”' : input.type === 'TXT' ? '输入 TXT 内容，应用会交由平台正确编码' : undefined

  return (
    <Modal
      title={record ? '编辑解析记录' : '新增解析记录'}
      description={`${domain.name} · ${providerName(domain.provider)}`}
      onClose={onClose}
      size="wide"
      footer={<><Button variant="secondary" onClick={onClose}>取消</Button><Button variant="primary" loading={saving} onClick={() => document.getElementById('record-form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))}>{saving ? '正在提交…' : '保存到平台'}</Button></>}
    >
      <form id="record-form" onSubmit={submit} className="form-stack record-form">
        <div className="form-grid record-main-grid">
          <Field label="主机记录" hint="根域名使用 @">
            <div className="input-suffix"><input value={input.name} onChange={(event) => set('name', event.target.value)} autoFocus placeholder="@ 或 www" /><span>.{domain.name}</span></div>
          </Field>
          <Field label="记录类型">
            <select value={input.type} onChange={(event) => set('type', event.target.value)}>{(capability?.recordTypes || [input.type]).map((type) => <option key={type}>{type}</option>)}</select>
          </Field>
        </div>
        <Field label="记录值" hint={valueHint}>
          <textarea value={input.value} onChange={(event) => set('value', event.target.value)} rows={input.type === 'TXT' ? 3 : 2} placeholder={input.type === 'A' ? '192.0.2.1' : input.type === 'AAAA' ? '2001:db8::1' : input.type === 'CNAME' ? 'target.example.com' : '输入记录值'} />
        </Field>
        <div className="form-grid form-grid-3">
          <Field label="TTL" hint={domain.provider === 'cloudflare' ? '1 表示自动' : '单位：秒'}>
            <input type="number" min={domain.provider === 'cloudflare' ? 1 : 1} value={input.ttl} onChange={(event) => set('ttl', Number(event.target.value))} />
          </Field>
          {capability?.supportsLine && <Field label="解析线路"><select value={input.line} onChange={(event) => set('line', event.target.value)}>{(capability.lines || ['默认']).map((line) => <option key={line}>{line}</option>)}</select></Field>}
          {showPriority && <Field label={input.type === 'MX' ? 'MX 优先级' : 'SRV 优先级'}><input type="number" min="0" max="65535" value={input.mx || 0} onChange={(event) => set('mx', Number(event.target.value))} /></Field>}
          {input.type === 'SRV' && capability?.supportsWeight && <Field label="SRV 权重"><input type="number" min="0" max="65535" value={input.weight || 0} onChange={(event) => set('weight', Number(event.target.value))} /></Field>}
        </div>
        <Field label="备注" hint="可选，不会写入审计日志中的记录值">
          <input value={input.remark || ''} onChange={(event) => set('remark', event.target.value)} maxLength={100} placeholder="为这条记录添加说明" />
        </Field>
        <div className="toggle-grid">
          {capability?.supportsProxied && supportsProxyType && <label className="toggle-row"><span><strong>Cloudflare 代理</strong><small>启用橙云代理流量</small></span><input type="checkbox" checked={input.proxied} onChange={(event) => set('proxied', event.target.checked)} /><i /></label>}
          {capability?.supportsDisable && <label className="toggle-row"><span><strong>创建后启用</strong><small>可稍后从列表中暂停</small></span><input type="checkbox" checked={input.status === 'enable'} onChange={(event) => set('status', event.target.checked ? 'enable' : 'disable')} /><i /></label>}
        </div>
        {domain.provider === 'cloudflare' && <div className="platform-caveat"><AlertTriangle size={16} /><span>Cloudflare 不支持暂停单条 DNS 记录，因此列表中的启停操作会显示为不可用。</span></div>}
        {error && <div className="form-error" role="alert"><AlertTriangle size={16} /><span>{error}</span></div>}
      </form>
    </Modal>
  )
}

