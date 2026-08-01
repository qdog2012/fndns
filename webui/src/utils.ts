import type { Provider } from './types'

export const providerName = (provider: Provider) => (provider === 'cloudflare' ? 'Cloudflare' : '腾讯云 DNSPod')

export function formatTime(value?: string, full = false): string {
  if (!value) return '尚未同步'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    ...(full ? { year: 'numeric' as const } : {}),
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

export function relativeTime(value?: string): string {
  if (!value) return '从未'
  const delta = Date.now() - new Date(value).getTime()
  if (delta < 60_000) return '刚刚'
  if (delta < 3_600_000) return `${Math.floor(delta / 60_000)} 分钟前`
  if (delta < 86_400_000) return `${Math.floor(delta / 3_600_000)} 小时前`
  return `${Math.floor(delta / 86_400_000)} 天前`
}

export const recordStatus = (status: string) => {
  const value = status.toLowerCase()
  return value === 'enable' || value === 'active' ? '已启用' : value === 'disable' || value === 'disabled' ? '已暂停' : status || '未知'
}

export const isEnabled = (status: string) => ['enable', 'active'].includes(status.toLowerCase())

export async function copyText(value: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return
    } catch {
      // 内网 HTTP、跨来源 iframe 或浏览器权限策略可能拒绝 Clipboard API。
      // 保留当前用户点击产生的激活状态，继续尝试兼容复制方案。
    }
  }

  const activeElement = document.activeElement instanceof HTMLElement ? document.activeElement : null
  const selection = document.getSelection()
  const previousRanges = selection ? Array.from({ length: selection.rangeCount }, (_, index) => selection.getRangeAt(index).cloneRange()) : []
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.readOnly = true
  textarea.setAttribute('aria-hidden', 'true')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.top = '0'
  textarea.style.opacity = '0'
  textarea.style.pointerEvents = 'none'
  document.body.appendChild(textarea)

  let copied = false
  try {
    textarea.focus({ preventScroll: true })
    textarea.select()
    textarea.setSelectionRange(0, textarea.value.length)
    copied = document.execCommand('copy')
  } finally {
    textarea.remove()
    selection?.removeAllRanges()
    previousRanges.forEach((range) => selection?.addRange(range))
    try {
      activeElement?.focus({ preventScroll: true })
    } catch {
      activeElement?.focus()
    }
  }

  if (!copied) throw new Error('浏览器未授予剪贴板权限')
}

export const actionName: Record<string, string> = {
  'credential.create': '添加凭据',
  'credential.update': '编辑凭据',
  'credential.delete': '删除凭据',
  'sync.credential': '同步凭据',
  'sync.domain': '同步域名',
  'record.create': '新增记录',
  'record.update': '编辑记录',
  'record.delete': '删除记录',
  'record.status': '启停记录',
}
