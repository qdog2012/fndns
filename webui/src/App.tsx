import { Globe2, KeyRound, Menu, ScrollText, ShieldCheck, X } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from './api'
import { Toasts, type ToastItem } from './components'
import CredentialsPage from './pages/CredentialsPage'
import DomainsPage from './pages/DomainsPage'
import LogsPage from './pages/LogsPage'
import type { Credential, Domain, Overview } from './types'

type Page = 'domains' | 'credentials' | 'logs'

const navItems: Array<{ id: Page; label: string; icon: typeof Globe2 }> = [
  { id: 'domains', label: '域名', icon: Globe2 },
  { id: 'credentials', label: 'API 凭据', icon: KeyRound },
  { id: 'logs', label: '操作日志', icon: ScrollText },
]

export default function App() {
  const [page, setPage] = useState<Page>('domains')
  const [credentials, setCredentials] = useState<Credential[]>([])
  const [domains, setDomains] = useState<Domain[]>([])
  const [overview, setOverview] = useState<Overview>()
  const [loading, setLoading] = useState(true)
  const [menuOpen, setMenuOpen] = useState(false)
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const toastId = useRef(0)

  const notify = useCallback((type: 'success' | 'error', message: string) => {
    const id = ++toastId.current
    setToasts((current) => [...current, { id, type, message }].slice(-4))
    window.setTimeout(() => setToasts((current) => current.filter((item) => item.id !== id)), type === 'error' ? 7000 : 4000)
  }, [])

  const reload = useCallback(async () => {
    try {
      const [nextCredentials, nextDomains, nextOverview] = await Promise.all([api.credentials(), api.domains(), api.overview()])
      setCredentials(nextCredentials)
      setDomains(nextDomains)
      setOverview(nextOverview)
    } catch (error) {
      notify('error', error instanceof Error ? error.message : '无法读取应用数据')
    } finally {
      setLoading(false)
    }
  }, [notify])

  useEffect(() => { void reload() }, [reload])

  const navigate = (next: Page) => {
    setPage(next)
    setMenuOpen(false)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  return (
    <div className="app-shell">
      <aside className={`sidebar ${menuOpen ? 'sidebar-open' : ''}`}>
        <div className="brand">
          <DnsLogo />
          <div><strong>DNS 管理器</strong><span>FNOS NETWORK</span></div>
          <button className="mobile-close" onClick={() => setMenuOpen(false)} aria-label="关闭菜单"><X size={19} /></button>
        </div>
        <nav>
          <span className="nav-label">管理</span>
          {navItems.map((item) => {
            const Icon = item.icon
            return <button key={item.id} className={page === item.id ? 'active' : ''} onClick={() => navigate(item.id)}><Icon size={18} /><span>{item.label}</span>{item.id === 'credentials' && overview?.credentialFailures ? <i className="nav-alert">{overview.credentialFailures}</i> : null}</button>
          })}
        </nav>
        <div className="sidebar-foot">
          <div className="admin-chip"><span><ShieldCheck size={17} /></span><div><strong>FNOS 管理员</strong><small>系统身份已验证</small></div></div>
          <span className="version">DNS Manager · v1.0.11</span>
        </div>
      </aside>

      {menuOpen && <button className="sidebar-scrim" onClick={() => setMenuOpen(false)} aria-label="关闭菜单" />}

      <section className="workspace">
        <header className="mobile-header">
          <button className="icon-button" onClick={() => setMenuOpen(true)} aria-label="打开菜单"><Menu size={20} /></button>
          <div><DnsLogo small /><strong>DNS 管理器</strong></div>
          <span className={overview?.credentialFailures ? 'health-dot warning' : 'health-dot'} />
        </header>
        {page === 'domains' && <DomainsPage domains={domains} credentials={credentials} overview={overview} loading={loading} onReload={reload} onOpenCredentials={() => navigate('credentials')} notify={notify} />}
        {page === 'credentials' && <CredentialsPage credentials={credentials} loading={loading} onReload={reload} notify={notify} />}
        {page === 'logs' && <LogsPage credentials={credentials} notify={notify} />}
      </section>

      <nav className="mobile-nav" aria-label="主导航">
        {navItems.map((item) => {
          const Icon = item.icon
          return <button key={item.id} className={page === item.id ? 'active' : ''} onClick={() => navigate(item.id)}><Icon size={20} /><span>{item.label === 'API 凭据' ? '凭据' : item.label}</span></button>
        })}
      </nav>
      <Toasts items={toasts} onDismiss={(id) => setToasts((current) => current.filter((item) => item.id !== id))} />
    </div>
  )
}

function DnsLogo({ small = false }: { small?: boolean }) {
  return <span className={`dns-logo ${small ? 'dns-logo-small' : ''}`} aria-hidden="true"><i className="logo-ring ring-a" /><i className="logo-ring ring-b" /><i className="logo-node node-a" /><i className="logo-node node-b" /><i className="logo-node node-c" /><i className="logo-core" /></span>
}
