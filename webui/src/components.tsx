import { AlertCircle, CheckCircle2, Cloud, X, Zap } from 'lucide-react'
import type { ButtonHTMLAttributes, PropsWithChildren, ReactNode } from 'react'
import type { Provider } from './types'

export function ProviderMark({ provider, compact = false }: { provider: Provider; compact?: boolean }) {
  return (
    <span className={`provider-mark provider-${provider} ${compact ? 'compact' : ''}`}>
      {provider === 'cloudflare' ? <Cloud size={compact ? 14 : 18} strokeWidth={2.2} /> : <Zap size={compact ? 14 : 18} strokeWidth={2.2} />}
      {!compact && <span>{provider === 'cloudflare' ? 'Cloudflare' : 'DNSPod'}</span>}
    </span>
  )
}

export function Button({ variant = 'secondary', loading, children, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'secondary' | 'ghost' | 'danger'; loading?: boolean }) {
  return (
    <button className={`button button-${variant}`} {...props} disabled={loading || props.disabled}>
      {loading && <span className="spinner" aria-hidden="true" />}
      {children}
    </button>
  )
}

export function Modal({ title, description, children, onClose, footer, size = 'normal' }: PropsWithChildren<{ title: string; description?: string; onClose: () => void; footer?: ReactNode; size?: 'normal' | 'wide' }>) {
  return (
    <div className="modal-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className={`modal modal-${size}`} role="dialog" aria-modal="true" aria-labelledby="modal-title">
        <header className="modal-header">
          <div>
            <h2 id="modal-title">{title}</h2>
            {description && <p>{description}</p>}
          </div>
          <button className="icon-button" onClick={onClose} aria-label="关闭"><X size={19} /></button>
        </header>
        <div className="modal-body">{children}</div>
        {footer && <footer className="modal-footer">{footer}</footer>}
      </section>
    </div>
  )
}

export type ToastItem = { id: number; type: 'success' | 'error'; message: string }

export function Toasts({ items, onDismiss }: { items: ToastItem[]; onDismiss: (id: number) => void }) {
  return (
    <div className="toast-stack" aria-live="polite">
      {items.map((item) => (
        <div className={`toast toast-${item.type}`} key={item.id}>
          {item.type === 'success' ? <CheckCircle2 size={18} /> : <AlertCircle size={18} />}
          <span>{item.message}</span>
          <button onClick={() => onDismiss(item.id)} aria-label="关闭"><X size={15} /></button>
        </div>
      ))}
    </div>
  )
}

export function Field({ label, hint, error, children, className = '' }: PropsWithChildren<{ label: string; hint?: string; error?: string; className?: string }>) {
  return (
    <label className={`field ${className}`}>
      <span className="field-label">{label}</span>
      {children}
      {error ? <span className="field-error">{error}</span> : hint ? <span className="field-hint">{hint}</span> : null}
    </label>
  )
}

export function EmptyState({ icon, title, description, action }: { icon: ReactNode; title: string; description: string; action?: ReactNode }) {
  return (
    <div className="empty-state">
      <div className="empty-orbit"><span>{icon}</span></div>
      <h3>{title}</h3>
      <p>{description}</p>
      {action}
    </div>
  )
}

