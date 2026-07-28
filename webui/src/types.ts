export type Provider = 'cloudflare' | 'tencent'
export type SyncStatus = 'never' | 'ok' | 'error'
export type CacheState = 'never' | 'fresh' | 'expired'

export interface Credential {
  id: string
  name: string
  provider: Provider
  secretHint: string
  createdAt: string
  updatedAt: string
  lastSyncAt?: string
  lastSyncStatus: SyncStatus
  lastSyncError?: string
  domainCount: number
}

export interface CredentialInput {
  name: string
  provider: Provider
  token?: string
  secretId?: string
  secretKey?: string
}

export interface Domain {
  id: string
  credentialId: string
  credentialName: string
  provider: Provider
  remoteId: string
  name: string
  status: string
  grade?: string
  recordCount: number
  lastSyncAt?: string
  syncError?: string
  cacheState: CacheState
  createdAt: string
  updatedAt: string
}

export interface DNSRecord {
  id: string
  domainId: string
  remoteId: string
  name: string
  type: string
  value: string
  ttl: number
  status: string
  line?: string
  lineId?: string
  mx?: number
  weight?: number
  proxied: boolean
  supportsProxied: boolean
  supportsDisable: boolean
  remark?: string
  remoteUpdatedAt?: string
  lastSyncAt: string
}

export interface RecordInput {
  name: string
  type: string
  value: string
  ttl: number
  status?: string
  line?: string
  lineId?: string
  mx?: number
  weight?: number
  proxied: boolean
  remark?: string
}

export interface Capability {
  provider: Provider
  recordTypes: string[]
  supportsProxied: boolean
  supportsDisable: boolean
  supportsLine: boolean
  supportsWeight: boolean
  ttlAutomatic: boolean
  defaultTtl: number
  lines?: string[]
}

export interface AuditLog {
  id: string
  createdAt: string
  operator: string
  credentialId?: string
  credentialName?: string
  provider?: Provider
  domain?: string
  recordName?: string
  recordType?: string
  action: string
  result: 'success' | 'failed'
  message?: string
}

export interface Overview {
  domainCount: number
  recordCount: number
  credentialCount: number
  credentialFailures: number
  lastSyncAt?: string
}

export interface BatchResult {
  recordId: string
  success: boolean
  error?: string
}

