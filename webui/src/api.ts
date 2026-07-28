import type {
  AuditLog,
  BatchResult,
  Capability,
  Credential,
  CredentialInput,
  DNSRecord,
  Domain,
  Overview,
  Provider,
  RecordInput,
} from './types'

interface Envelope<T> {
  data?: T
  error?: { code: string; message: string }
}

export class ApiError extends Error {
  code: string
  status: number

  constructor(message: string, code = 'unknown', status = 0) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  headers.set('Accept', 'application/json')
  if (options.body) headers.set('Content-Type', 'application/json')
  const basePath = new URL(import.meta.env.BASE_URL, window.location.href).pathname.replace(/\/$/, '')
  const response = await fetch(`${basePath}${path}`, { ...options, headers })
  let envelope: Envelope<T>
  try {
    envelope = (await response.json()) as Envelope<T>
  } catch {
    throw new ApiError(`服务器返回了无法识别的响应（HTTP ${response.status}）`, 'bad_response', response.status)
  }
  if (!response.ok || envelope.error) {
    throw new ApiError(envelope.error?.message || `请求失败（HTTP ${response.status}）`, envelope.error?.code, response.status)
  }
  return envelope.data as T
}

const body = (value: unknown) => JSON.stringify(value)

export const api = {
  overview: () => request<Overview>('/api/v1/overview'),
  credentials: () => request<Credential[]>('/api/v1/credentials'),
  createCredential: (input: CredentialInput) => request<Credential>('/api/v1/credentials', { method: 'POST', body: body(input) }),
  updateCredential: (id: string, input: CredentialInput) => request<Credential>(`/api/v1/credentials/${id}`, { method: 'PUT', body: body(input) }),
  deleteCredential: (id: string) => request<{ deleted: boolean }>(`/api/v1/credentials/${id}`, { method: 'DELETE' }),
  refreshCredential: (id: string) => request<{ refreshed: boolean }>(`/api/v1/credentials/${id}/refresh`, { method: 'POST' }),
  refreshAll: () => request<Array<{ credentialId: string; success: boolean; error?: string }>>('/api/v1/refresh', { method: 'POST' }),
  domains: () => request<Domain[]>('/api/v1/domains'),
  refreshDomain: (id: string) => request<{ refreshed: boolean }>(`/api/v1/domains/${id}/refresh`, { method: 'POST' }),
  records: (id: string) => request<{ domain: Domain; records: DNSRecord[] }>(`/api/v1/domains/${id}/records`),
  createRecord: (domainId: string, input: RecordInput) => request<DNSRecord>(`/api/v1/domains/${domainId}/records`, { method: 'POST', body: body(input) }),
  updateRecord: (domainId: string, recordId: string, input: RecordInput) => request<DNSRecord>(`/api/v1/domains/${domainId}/records/${recordId}`, { method: 'PUT', body: body(input) }),
  deleteRecord: (domainId: string, recordId: string) => request<{ deleted: boolean }>(`/api/v1/domains/${domainId}/records/${recordId}`, { method: 'DELETE' }),
  setRecordStatus: (domainId: string, recordId: string, enabled: boolean) => request<{ updated: boolean }>(`/api/v1/domains/${domainId}/records/${recordId}/status`, { method: 'POST', body: body({ enabled }) }),
  batchDelete: (domainId: string, recordIds: string[]) => request<BatchResult[]>(`/api/v1/domains/${domainId}/records/batch-delete`, { method: 'POST', body: body({ recordIds }) }),
  batchStatus: (domainId: string, recordIds: string[], enabled: boolean) => request<BatchResult[]>(`/api/v1/domains/${domainId}/records/batch-status`, { method: 'POST', body: body({ recordIds, enabled }) }),
  capability: (provider: Provider) => request<Capability>(`/api/v1/capabilities/${provider}`),
  logs: (query = '') => request<AuditLog[]>(`/api/v1/logs${query ? `?${query}` : ''}`),
}
