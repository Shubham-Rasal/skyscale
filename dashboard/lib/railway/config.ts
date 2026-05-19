export const RAILWAY_GRAPHQL_DEFAULT = 'https://backboard.railway.com/graphql/v2'

export interface RailwayServiceBinding {
  serviceId: string
  environmentId: string
  projectId?: string
}

/** Map template id → Railway service binding for real deploy/stop */
export type FaasRailwayServiceMap = Record<string, RailwayServiceBinding>

function firstEnv(...keys: string[]): string | undefined {
  for (const k of keys) {
    const v = process.env[k]?.trim()
    if (v) return v
  }
  return undefined
}

/**
 * Default target service for spin up/down when no per-template map applies.
 * Order: explicit FAAS_* vars → Railway system vars (injected when this app runs on Railway).
 */
export function getDefaultRailwayBinding(): RailwayServiceBinding | undefined {
  const serviceId = firstEnv('FAAS_RAILWAY_SERVICE_ID', 'RAILWAY_SERVICE_ID')
  const environmentId = firstEnv('FAAS_RAILWAY_ENVIRONMENT_ID', 'RAILWAY_ENVIRONMENT_ID')
  if (!serviceId || !environmentId) return undefined
  const projectId = firstEnv('FAAS_RAILWAY_PROJECT_ID', 'RAILWAY_PROJECT_ID')
  return { serviceId, environmentId, ...(projectId ? { projectId } : {}) }
}

/** Binding for a template: per-template map wins, else global default env vars */
export function resolveRailwayBinding(templateId: string): RailwayServiceBinding | undefined {
  const map = parseServiceMap()
  if (map[templateId]?.serviceId && map[templateId]?.environmentId) {
    return map[templateId]
  }
  return getDefaultRailwayBinding()
}

export function requireRailwayToken(): string {
  const t = getRailwayToken()
  if (!t) {
    throw new Error('RAILWAY_API_TOKEN is required for FaaS (set in Railway or .env.local)')
  }
  return t
}

export function getRailwayToken(): string | undefined {
  return process.env.RAILWAY_API_TOKEN?.trim() || undefined
}

export function getRailwayGraphqlUrl(): string {
  return (process.env.RAILWAY_GRAPHQL_URL || RAILWAY_GRAPHQL_DEFAULT).replace(/\/$/, '')
}

export function parseServiceMap(): FaasRailwayServiceMap {
  const raw = process.env.FAAS_RAILWAY_SERVICES_JSON?.trim()
  if (!raw) return {}
  try {
    return JSON.parse(raw) as FaasRailwayServiceMap
  } catch {
    return {}
  }
}
