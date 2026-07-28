import { AlertTriangle, CheckCircle2, Clock3, Filter, History, RotateCcw, Search } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import { Button, EmptyState, ProviderMark } from '../components'
import type { AuditLog, Credential } from '../types'
import { actionName, formatTime } from '../utils'

interface Props {
  credentials: Credential[]
  notify: (type: 'success' | 'error', message: string) => void
}

export default function LogsPage({ credentials, notify }: Props) {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [loading, setLoading] = useState(true)
  const [credentialId, setCredentialId] = useState('')
  const [action, setAction] = useState('')
  const [result, setResult] = useState('')
  const [domain, setDomain] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')

  const query = useMemo(() => {
    const params = new URLSearchParams({ limit: '300' })
    if (credentialId) params.set('credentialId', credentialId)
    if (action) params.set('action', action)
    if (result) params.set('result', result)
    if (domain.trim()) params.set('domain', domain.trim())
    if (from) params.set('from', from)
    if (to) params.set('to', to)
    return params.toString()
  }, [action, credentialId, domain, from, result, to])

  const load = async () => {
    setLoading(true)
    try {
      setLogs(await api.logs(query))
    } catch (error) {
      notify('error', error instanceof Error ? error.message : '读取日志失败')
    } finally { setLoading(false) }
  }

  useEffect(() => { const timer = window.setTimeout(() => { void load() }, 180); return () => window.clearTimeout(timer) }, [query])

  const reset = () => { setCredentialId(''); setAction(''); setResult(''); setDomain(''); setFrom(''); setTo('') }
  const filtered = Boolean(credentialId || action || result || domain || from || to)

  return (
    <main className="page-content page-enter">
      <div className="page-heading">
        <div><span className="eyebrow">AUDIT TRAIL</span><h1>操作日志</h1><p>保留 6 个月；解析值默认脱敏，API 密钥永不写入日志。</p></div>
        <Button variant="secondary" onClick={load} loading={loading}><RotateCcw size={16} />刷新日志</Button>
      </div>

      <section className="log-filters">
        <div className="filter-caption"><Filter size={16} /><span>筛选条件</span>{filtered && <button onClick={reset}>重置全部</button>}</div>
        <div className="filter-grid">
          <label><span>API 凭据</span><select value={credentialId} onChange={(event) => setCredentialId(event.target.value)}><option value="">全部凭据</option>{credentials.map((credential) => <option key={credential.id} value={credential.id}>{credential.name}</option>)}</select></label>
          <label><span>操作类型</span><select value={action} onChange={(event) => setAction(event.target.value)}><option value="">全部操作</option>{Object.entries(actionName).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label>
          <label><span>结果</span><select value={result} onChange={(event) => setResult(event.target.value)}><option value="">全部结果</option><option value="success">成功</option><option value="failed">失败</option></select></label>
          <label><span>域名（精确匹配）</span><div className="mini-search"><Search size={15} /><input value={domain} onChange={(event) => setDomain(event.target.value)} placeholder="example.com" /></div></label>
          <label><span>开始日期</span><input type="date" value={from} onChange={(event) => setFrom(event.target.value)} /></label>
          <label><span>结束日期</span><input type="date" value={to} min={from || undefined} onChange={(event) => setTo(event.target.value)} /></label>
        </div>
      </section>

      {!loading && logs.length === 0 ? (
        <EmptyState icon={<History size={25} />} title={filtered ? '没有匹配的日志' : '还没有操作记录'} description={filtered ? '尝试放宽筛选条件，或重置全部筛选。' : '添加凭据、同步域名或修改解析记录后，审计事件会出现在这里。'} action={filtered ? <Button onClick={reset}>重置筛选</Button> : undefined} />
      ) : (
        <section className="data-panel log-panel">
          <div className="desktop-table table-scroll">
            <table className="log-table">
              <thead><tr><th>时间</th><th>操作</th><th>对象</th><th>来源</th><th>结果</th><th>详情</th></tr></thead>
              <tbody>{logs.map((log) => <LogRow log={log} key={log.id} />)}</tbody>
            </table>
          </div>
          <div className="mobile-log-list">{logs.map((log) => <MobileLog log={log} key={log.id} />)}</div>
        </section>
      )}
      <p className="retention-note"><Clock3 size={14} />系统会自动清理超过 6 个月的操作日志。</p>
    </main>
  )
}

function LogRow({ log }: { log: AuditLog }) {
  return (
    <tr>
      <td><span className="log-time">{formatTime(log.createdAt, true)}</span><small>{log.operator}</small></td>
      <td><strong>{actionName[log.action] || log.action}</strong></td>
      <td><span>{log.domain || log.credentialName || '—'}</span>{log.recordName && <small>{log.recordType} · {log.recordName}</small>}</td>
      <td>{log.provider ? <div className="source-cell"><ProviderMark provider={log.provider} compact /><span>{log.credentialName}</span></div> : <span className="subtle">本地</span>}</td>
      <td><ResultBadge result={log.result} /></td>
      <td><span className="log-message" title={log.message}>{log.message || '—'}</span></td>
    </tr>
  )
}

function MobileLog({ log }: { log: AuditLog }) {
  return (
    <article className="mobile-log-card">
      <header><strong>{actionName[log.action] || log.action}</strong><ResultBadge result={log.result} /></header>
      <div>{log.domain || log.credentialName || '本地操作'}{log.recordName && <span>{log.recordType} · {log.recordName}</span>}</div>
      <p>{log.message || '无详情'}</p>
      <footer><span>{formatTime(log.createdAt, true)}</span><span>{log.operator}</span></footer>
    </article>
  )
}

function ResultBadge({ result }: { result: AuditLog['result'] }) {
  return result === 'success'
    ? <span className="result-badge result-success"><CheckCircle2 size={13} />成功</span>
    : <span className="result-badge result-failed"><AlertTriangle size={13} />失败</span>
}

