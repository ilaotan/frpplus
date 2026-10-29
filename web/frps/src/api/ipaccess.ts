import { buildQueryString, http } from './http'
import type { V2Page } from './http'
import type {
  IPAccessAction,
  IPAccessAllowResult,
  IPAccessClearedResponse,
  IPAccessCurrentIP,
  IPAccessMode,
  IPAccessPruneResponse,
  IPAccessRecord,
  IPAccessRecordListParams,
  IPAccessRule,
  IPAccessRuleListParams,
  IPAccessSettings,
} from '../types/ipaccess'

const base = '../api/v2/ip-access'

export const getIPAccessSettings = () => {
  return http.getV2<IPAccessSettings>(`${base}/settings`)
}

export const updateIPAccessGlobalMode = (globalMode: IPAccessMode) => {
  return http.putV2<IPAccessSettings>(`${base}/settings`, { globalMode })
}

export const updateIPAccessProxyMode = (
  proxyName: string,
  mode: IPAccessMode | '',
) => {
  return http.putV2<IPAccessSettings>(`${base}/proxy-modes`, { proxyName, mode })
}

export const getIPAccessRules = (params: IPAccessRuleListParams = {}) => {
  return http.getV2<V2Page<IPAccessRule>>(
    `${base}/rules${buildQueryString({
      page: params.page,
      pageSize: params.pageSize,
      proxyName: params.proxyName || undefined,
      action:
        params.action && params.action !== 'all' ? params.action : undefined,
    })}`,
  )
}

export const createIPAccessRule = (payload: {
  proxyName: string
  action: IPAccessAction
  ip: string
  note?: string
}) => {
  return http.postV2<IPAccessRule>(`${base}/rules`, payload)
}

export const deleteIPAccessRule = (id: number) => {
  return http.delete(`${base}/rules/${id}`)
}

export const getIPAccessRecords = (params: IPAccessRecordListParams = {}) => {
  return http.getV2<V2Page<IPAccessRecord>>(
    `${base}/records${buildQueryString({
      page: params.page,
      pageSize: params.pageSize,
      proxyName: params.proxyName || undefined,
      ip: params.ip || undefined,
      result:
        params.result && params.result !== 'all' ? params.result : undefined,
    })}`,
  )
}

export const deleteIPAccessRecords = (ids: number[]) => {
  return http.postV2<IPAccessClearedResponse>(`${base}/records/delete`, { ids })
}

export const pruneIPAccessRecords = () => {
  return http.postV2<IPAccessPruneResponse>(`${base}/prune`)
}

export const getIPAccessCurrentIP = () => {
  return http.getV2<IPAccessCurrentIP>(`${base}/current-ip`)
}

// hours 0 keeps the rule permanent.
export const allowIPAccessCurrentIP = (hours: number) => {
  return http.postV2<IPAccessAllowResult>(`${base}/allow-current-ip`, { hours })
}
