import { randomUUID } from 'crypto'
import type { FaasContainer, FaasLogLevel, FaasLogLine } from './types'
import { getTemplateById } from './templates'
import {
  fetchDeploymentLogs,
  listRecentDeployments,
  stopDeployment,
  triggerServiceDeploy,
  type DeploymentNode,
} from '@/lib/railway/deployments'
import { requireRailwayToken, resolveRailwayBinding } from '@/lib/railway/config'

const containers = new Map<string, FaasContainer>()

function nowIso() {
  return new Date().toISOString()
}

function pushLog(c: FaasContainer, level: FaasLogLevel, message: string) {
  const line: FaasLogLine = { ts: nowIso(), level, message }
  c.logs = [...c.logs, line].slice(-200)
  c.updatedAt = nowIso()
}

function sortDeployments(nodes: DeploymentNode[]) {
  return [...nodes].sort((a, b) => {
    const ta = a.createdAt ? Date.parse(a.createdAt) : 0
    const tb = b.createdAt ? Date.parse(b.createdAt) : 0
    return tb - ta
  })
}

async function refreshRailwayDeployment(c: FaasContainer) {
  if (!c.railway) return
  const { serviceId, environmentId, projectId } = c.railway
  const input: Record<string, unknown> = { serviceId, environmentId }
  if (projectId) input.projectId = projectId

  try {
    const nodes = sortDeployments(await listRecentDeployments(input, 12))
    const latest = nodes[0]
    if (!latest) return

    c.railway.deploymentId = latest.id
    const status = latest.status?.toUpperCase() ?? ''
    if (status === 'SUCCESS' || status === 'RUNNING') {
      c.status = 'running'
      c.endpointUrl = latest.url || latest.staticUrl || undefined
    } else if (status === 'SLEEPING') {
      c.endpointUrl = latest.url || latest.staticUrl || undefined
      c.status = c.endpointUrl ? 'running' : 'deploying'
    } else if (status === 'BUILDING' || status === 'DEPLOYING' || status === 'QUEUED' || status === 'WAITING') {
      c.status = 'deploying'
    } else if (
      status === 'FAILED' ||
      status === 'CRASHED' ||
      status === 'REMOVED'
    ) {
      c.status = 'failed'
      c.errorMessage = `Deployment status: ${latest.status}`
    } else {
      c.status = 'deploying'
    }

    if (latest.id) {
      const rawLogs = await fetchDeploymentLogs(latest.id, 60)
      c.logs = rawLogs.map((row) => ({
        ts: row.timestamp || nowIso(),
        level: (row.severity === 'err' ? 'error' : 'info') as FaasLogLevel,
        message: row.message,
      }))
    }
    c.updatedAt = nowIso()
    containers.set(c.id, c)
  } catch (e) {
    pushLog(
      c,
      'warn',
      `[railway] Could not refresh deployment: ${e instanceof Error ? e.message : String(e)}`,
    )
    containers.set(c.id, c)
  }
}

async function refreshAllRailwayContainers() {
  for (const c of containers.values()) {
    if (c.railway && c.status !== 'stopped' && c.status !== 'failed') {
      await refreshRailwayDeployment(c)
    }
  }
}

export async function listFaasContainersAsync(): Promise<FaasContainer[]> {
  await refreshAllRailwayContainers()
  return [...containers.values()].sort(
    (a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt),
  )
}

export function listFaasContainers(): FaasContainer[] {
  return [...containers.values()].sort(
    (a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt),
  )
}

export function getFaasContainer(id: string): FaasContainer | undefined {
  return containers.get(id)
}

export async function getFaasContainerAsync(id: string): Promise<FaasContainer | undefined> {
  const c = containers.get(id)
  if (!c) return undefined
  if (c.railway) await refreshRailwayDeployment(c)
  return containers.get(id)
}

export async function createFaasContainer(templateId: string): Promise<FaasContainer> {
  requireRailwayToken()

  const tmpl = getTemplateById(templateId)
  if (!tmpl) {
    throw new Error(`Unknown template: ${templateId}`)
  }

  const binding = resolveRailwayBinding(templateId)
  if (!binding?.serviceId || !binding.environmentId) {
    throw new Error(
      `No Railway service binding for template "${templateId}". ` +
        'If this app runs on Railway, RAILWAY_SERVICE_ID and RAILWAY_ENVIRONMENT_ID are usually set automatically. ' +
        'Otherwise set FAAS_RAILWAY_SERVICE_ID and FAAS_RAILWAY_ENVIRONMENT_ID in .env.local, ' +
        'or FAAS_RAILWAY_SERVICES_JSON for per-template services. ' +
        'IDs: Railway dashboard → Cmd/Ctrl+K → Copy Service ID / Environment ID.',
    )
  }

  const id = randomUUID()
  const base: FaasContainer = {
    id,
    templateId: tmpl.id,
    templateName: tmpl.name,
    status: 'deploying',
    createdAt: nowIso(),
    updatedAt: nowIso(),
    logs: [],
    railway: {
      serviceId: binding.serviceId,
      environmentId: binding.environmentId,
      projectId: binding.projectId,
    },
  }

  pushLog(base, 'info', '[railway] Triggering serviceInstanceDeploy…')
  containers.set(id, base)

  try {
    const dep = await triggerServiceDeploy(binding.serviceId, binding.environmentId)
    if (dep?.id) {
      base.railway = { ...base.railway!, deploymentId: dep.id }
      base.endpointUrl = dep.url || dep.staticUrl || undefined
      pushLog(base, 'info', `[railway] Deploy initiated — deployment ${dep.id} (${dep.status})`)
    } else {
      pushLog(base, 'info', '[railway] Deploy triggered — resolving deployment id…')
    }
  } catch (e) {
    base.status = 'failed'
    base.errorMessage = e instanceof Error ? e.message : String(e)
    pushLog(base, 'error', `[railway] Deploy failed: ${base.errorMessage}`)
  }

  containers.set(id, base)
  await refreshRailwayDeployment(containers.get(id)!)
  return containers.get(id)!
}

export async function stopFaasContainer(id: string): Promise<FaasContainer | undefined> {
  requireRailwayToken()

  const c = containers.get(id)
  if (!c) return undefined

  c.status = 'stopping'
  pushLog(c, 'info', '[railway] Stopping deployment…')
  containers.set(id, c)

  try {
    await refreshRailwayDeployment(c)
    let depId = c.railway?.deploymentId
    if (!depId && c.railway) {
      const input: Record<string, unknown> = {
        serviceId: c.railway.serviceId,
        environmentId: c.railway.environmentId,
      }
      if (c.railway.projectId) input.projectId = c.railway.projectId
      const nodes = sortDeployments(await listRecentDeployments(input, 5))
      if (nodes[0]?.id) {
        c.railway.deploymentId = nodes[0].id
        depId = nodes[0].id
      }
    }
    if (depId) {
      await stopDeployment(depId)
      pushLog(c, 'info', `[railway] deploymentStop(${depId}) succeeded`)
    } else {
      pushLog(c, 'warn', '[railway] No deployment id found to stop')
    }
    c.status = 'stopped'
    c.endpointUrl = undefined
  } catch (e) {
    c.status = 'failed'
    c.errorMessage = e instanceof Error ? e.message : String(e)
    pushLog(c, 'error', `[railway] Stop failed: ${c.errorMessage}`)
  }

  c.updatedAt = nowIso()
  containers.set(id, c)
  return c
}

export async function testFaasContainer(
  id: string,
  body?: Record<string, unknown>,
): Promise<{ ok: boolean; status: number; result: unknown }> {
  const c = await getFaasContainerAsync(id)
  if (!c) throw new Error('Container not found')

  if (c.status === 'stopped' || c.status === 'stopping') {
    throw new Error('Container is not running (stopped or stopping)')
  }

  const tmpl = getTemplateById(c.templateId)
  if (!tmpl) throw new Error('Template missing')

  const baseUrl = c.endpointUrl?.replace(/\/$/, '')
  if (!baseUrl) {
    throw new Error('No endpoint URL yet — wait until deployment is SUCCESS')
  }

  const url = `${baseUrl}${tmpl.testPath.startsWith('/') ? tmpl.testPath : `/${tmpl.testPath}`}`
  const init: RequestInit = {
    method: tmpl.testMethod,
    headers: { 'Content-Type': 'application/json' },
    cache: 'no-store',
  }
  if (tmpl.testMethod === 'POST') {
    init.body = JSON.stringify(body ?? tmpl.testBodyExample)
  }

  pushLog(c, 'info', `[test] ${tmpl.testMethod} ${url}`)
  containers.set(id, c)

  const res = await fetch(url, init)
  const text = await res.text()
  let parsed: unknown = text
  try {
    parsed = JSON.parse(text)
  } catch {
    /* keep text */
  }

  pushLog(
    c,
    res.ok ? 'info' : 'warn',
    `[test] Response HTTP ${res.status}`,
  )
  containers.set(id, c)

  return { ok: res.ok, status: res.status, result: parsed }
}
