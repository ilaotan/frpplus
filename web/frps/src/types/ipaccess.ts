export type IPAccessMode = 'off' | 'allowlist' | 'denylist'
export type IPAccessAction = 'allow' | 'deny'
export type IPAccessResultFilter = 'all' | 'allowed' | 'denied'

export interface IPAccessProxyMode {
  proxyName: string
  mode: IPAccessMode
}

export interface IPAccessSettings {
  enabled: boolean
  storePath: string
  globalMode: IPAccessMode
  ruleCount: number
  recordCount: number
  retentionHours: number
  maxRecords: number
  proxyModes: IPAccessProxyMode[]
}

export interface IPAccessRule {
  id: number
  proxyName: string
  action: IPAccessAction
  ip: string
  note?: string
  createdAt: number
  // 0 means the rule is permanent.
  expiresAt?: number
  source?: 'manual' | 'api'
}

export interface IPAccessRecord {
  id: number
  proxyName: string
  ip: string
  firstSeen: number
  lastSeen: number
  connCount: number
  allowed: boolean
  reason?: string
}

export interface IPAccessRuleListParams {
  page?: number
  pageSize?: number
  proxyName?: string
  action?: IPAccessAction | 'all'
}

export interface IPAccessRecordListParams {
  page?: number
  pageSize?: number
  proxyName?: string
  ip?: string
  result?: IPAccessResultFilter
}

export interface IPAccessPruneResponse {
  cleared: number
  total: number
}

export interface IPAccessCurrentIP {
  ip: string
  // true when the dashboard is opened on the frps host itself
  loopback: boolean
}

export interface IPAccessAllowResult {
  ip: string
  proxyName: string
  action: IPAccessAction
  ruleId: number
  expiresAt: number
}

export interface IPAccessClearedResponse {
  cleared: number
}
