import { AlertTriangle, Check, CloudCog, Edit3, KeyRound, Plus, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react'
import { FormEvent, useEffect, useState } from 'react'
import { api } from '../api'
import { Button, EmptyState, Field, Modal, ProviderMark } from '../components'
import type { Credential, CredentialInput, Provider } from '../types'
import { formatTime, providerName, relativeTime } from '../utils'

interface Props {
  credentials: Credential[]
  loading: boolean
  onReload: () => Promise<void>
  notify: (type: 'success' | 'error', message: string) => void
}

const blankInput: CredentialInput = { name: '', provider: 'cloudflare', token: '', secretId: '', secretKey: '' }

export default function CredentialsPage({ credentials, loading, onReload, notify }: Props) {
  const [editing, setEditing] = useState<Credential | 'new' | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  const refresh = async (credential: Credential) => {
    setBusyId(credential.id)
    try {
      await api.refreshCredential(credential.id)
      notify('success', `${credential.name} 已同步`)
      await onReload()
    } catch (error) {
      notify('error', error instanceof Error ? error.message : '同步失败')
      await onReload()
    } finally {
      setBusyId(null)
    }
  }

  const remove = async (credential: Credential) => {
    if (!window.confirm(`确定删除“${credential.name}”吗？关联的域名和解析记录缓存会立即删除，操作日志仍保留 6 个月。`)) return
    setBusyId(credential.id)
    try {
      await api.deleteCredential(credential.id)
      notify('success', 'API 凭据及关联缓存已删除')
      await onReload()
    } catch (error) {
      notify('error', error instanceof Error ? error.message : '删除失败')
    } finally {
      setBusyId(null)
    }
  }

  return (
    <main className="page-content page-enter">
      <div className="page-heading">
        <div>
          <span className="eyebrow">ACCESS KEYS</span>
          <h1>API 凭据</h1>
          <p>凭据由设备本地密钥加密，仅在调用对应平台时解密。</p>
        </div>
        <Button variant="primary" onClick={() => setEditing('new')}><Plus size={17} />添加凭据</Button>
      </div>

      <div className="security-note">
        <div className="security-icon"><ShieldCheck size={20} /></div>
        <div><strong>AES-256-GCM 本地加密</strong><span>API Token 与 SecretKey 不会返回浏览器，也不会写入操作日志。</span></div>
      </div>

      {!loading && credentials.length === 0 ? (
        <EmptyState
          icon={<KeyRound size={25} />}
          title="还没有 API 凭据"
          description="添加 Cloudflare API Token 或腾讯云密钥，验证成功后会立即拉取一次域名列表。"
          action={<Button variant="primary" onClick={() => setEditing('new')}><Plus size={17} />添加第一组凭据</Button>}
        />
      ) : (
        <div className="credential-grid">
          {credentials.map((credential, index) => (
            <article className="credential-card" key={credential.id} style={{ '--index': index } as React.CSSProperties}>
              <header>
                <ProviderMark provider={credential.provider} />
                <span className={`sync-pill sync-${credential.lastSyncStatus}`}>
                  {credential.lastSyncStatus === 'ok' ? <Check size={13} /> : credential.lastSyncStatus === 'error' ? <AlertTriangle size={13} /> : <CloudCog size={13} />}
                  {credential.lastSyncStatus === 'ok' ? '连接正常' : credential.lastSyncStatus === 'error' ? '同步异常' : '等待同步'}
                </span>
              </header>
              <div className="credential-main">
                <h3>{credential.name}</h3>
                <code>{credential.secretHint}</code>
              </div>
              <div className="credential-metrics">
                <div><strong>{credential.domainCount}</strong><span>个域名</span></div>
                <div><strong>{relativeTime(credential.lastSyncAt)}</strong><span title={formatTime(credential.lastSyncAt, true)}>最后同步</span></div>
              </div>
              {credential.lastSyncError && <div className="inline-error"><AlertTriangle size={15} /><span>{credential.lastSyncError}</span></div>}
              <footer>
                <Button variant="ghost" onClick={() => refresh(credential)} loading={busyId === credential.id}><RefreshCw size={15} />同步</Button>
                <div className="card-actions">
                  <button className="icon-button" onClick={() => setEditing(credential)} aria-label={`编辑 ${credential.name}`}><Edit3 size={16} /></button>
                  <button className="icon-button danger-icon" onClick={() => remove(credential)} disabled={busyId === credential.id} aria-label={`删除 ${credential.name}`}><Trash2 size={16} /></button>
                </div>
              </footer>
            </article>
          ))}
        </div>
      )}
      {editing && (
        <CredentialModal
          credential={editing === 'new' ? undefined : editing}
          onClose={() => setEditing(null)}
          onSaved={async (message) => { setEditing(null); notify('success', message); await onReload() }}
        />
      )}
    </main>
  )
}

function CredentialModal({ credential, onClose, onSaved }: { credential?: Credential; onClose: () => void; onSaved: (message: string) => Promise<void> }) {
  const [input, setInput] = useState<CredentialInput>(blankInput)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (credential) setInput({ name: credential.name, provider: credential.provider, token: '', secretId: '', secretKey: '' })
    else setInput(blankInput)
  }, [credential])

  const set = (field: keyof CredentialInput, value: string) => setInput((current) => ({ ...current, [field]: value }))

  const changeProvider = (provider: Provider) => setInput({ ...blankInput, name: input.name, provider })

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    if (!input.name.trim()) return setError('请填写凭据名称')
    if (!credential && input.provider === 'cloudflare' && !input.token?.trim()) return setError('请填写 Cloudflare API Token')
    if (!credential && input.provider === 'tencent' && (!input.secretId?.trim() || !input.secretKey?.trim())) return setError('请填写腾讯云 SecretId 和 SecretKey')
    setSaving(true)
    try {
      if (credential) {
        await api.updateCredential(credential.id, input)
        await onSaved('API 凭据已更新；同步状态可在凭据卡片中查看')
      } else {
        await api.createCredential(input)
        await onSaved('API 凭据已验证并加密保存；同步状态可在凭据卡片中查看')
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const providerChanged = Boolean(credential && credential.provider !== input.provider)

  return (
    <Modal
      title={credential ? '编辑 API 凭据' : '添加 API 凭据'}
      description={credential ? '密钥字段留空时会继续使用已加密保存的值。' : '保存前会连接平台验证权限，并自动同步一次域名。'}
      onClose={onClose}
      footer={<><Button variant="secondary" onClick={onClose}>取消</Button><Button variant="primary" loading={saving} onClick={() => document.getElementById('credential-form')?.dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }))}>{saving ? '正在验证…' : '验证并保存'}</Button></>}
    >
      <form id="credential-form" onSubmit={submit} className="form-stack">
        <Field label="凭据名称" hint="用于区分来源，例如“公司 Cloudflare”">
          <input value={input.name} onChange={(event) => set('name', event.target.value)} maxLength={60} autoFocus placeholder="输入一个容易识别的名称" />
        </Field>
        <fieldset className="provider-choice">
          <legend>DNS 平台</legend>
          <button type="button" className={input.provider === 'cloudflare' ? 'selected' : ''} onClick={() => changeProvider('cloudflare')}>
            <ProviderMark provider="cloudflare" /><span>API Token</span>
          </button>
          <button type="button" className={input.provider === 'tencent' ? 'selected' : ''} onClick={() => changeProvider('tencent')}>
            <ProviderMark provider="tencent" /><span>SecretId + SecretKey</span>
          </button>
        </fieldset>
        {input.provider === 'cloudflare' ? (
          <Field label="Cloudflare API Token" hint={credential && !providerChanged ? '留空表示不修改。Token 至少需要 Zone:Read 与 DNS:Edit 权限。' : '建议创建仅包含 Zone:Read 与 DNS:Edit 的最小权限 Token。'}>
            <input type="password" value={input.token} onChange={(event) => set('token', event.target.value)} autoComplete="new-password" placeholder={credential && !providerChanged ? '留空以保留现有 Token' : '粘贴 API Token'} />
          </Field>
        ) : (
          <div className="form-grid">
            <Field label="SecretId" hint={credential && !providerChanged ? '留空表示不修改' : undefined}>
              <input type="password" value={input.secretId} onChange={(event) => set('secretId', event.target.value)} autoComplete="new-password" placeholder={credential && !providerChanged ? '留空以保留' : 'AKID…'} />
            </Field>
            <Field label="SecretKey" hint={credential && !providerChanged ? '留空表示不修改' : undefined}>
              <input type="password" value={input.secretKey} onChange={(event) => set('secretKey', event.target.value)} autoComplete="new-password" placeholder={credential && !providerChanged ? '留空以保留' : '输入 SecretKey'} />
            </Field>
          </div>
        )}
        <div className="form-provider-summary"><KeyRound size={16} /><span>{providerName(input.provider)} 的凭据仅会保存在当前飞牛设备。</span></div>
        {error && <div className="form-error" role="alert"><AlertTriangle size={16} /><span>{error}</span></div>}
      </form>
    </Modal>
  )
}
