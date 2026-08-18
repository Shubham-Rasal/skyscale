import type { FaasContainer, FaasTemplate } from '@/lib/faas/types'

const BASE = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

async function parseResponse(r: Response) {
  const text = await r.text()
  let data: unknown = text
  try { data = JSON.parse(text) } catch {}
  if (!r.ok) {
    const message = data && typeof data === 'object' && 'error' in data
      ? String((data as { error: unknown }).error)
      : text.trim().startsWith('<!DOCTYPE html>')
        ? `Dashboard proxy returned HTML for ${r.url}. Check SKYSCALE_CONTROL_PLANE_URL/NEXT_PUBLIC_API_URL.`
      : text.trim() || `HTTP ${r.status}`
    throw new Error(message)
  }
  return data
}

export const api = {
  listFunctions: () =>
    fetch('/api/functions').then(parseResponse),

  registerFunction: (body: Record<string, unknown>) =>
    fetch('/api/functions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(parseResponse),

  invoke: (name: string, body: Record<string, unknown>) =>
    fetch(`/api/functions/name/${encodeURIComponent(name)}/invoke`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(parseResponse),

  submitTrainingJob: (body: Record<string, unknown>) =>
    fetch('/api/training/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(parseResponse),

  listVMs: () =>
    fetch(`${BASE}/api/vms`).then(r => r.json()),

  gpuMetrics: () =>
    fetch(`${BASE}/api/metrics/gpu`).then(r => r.json()),

  getExecution: (id: string) =>
    fetch(`${BASE}/api/executions/${id}`).then(r => r.json()),

  listExecutions: (functionId: string) =>
    fetch(`${BASE}/api/executions/function/${functionId}`).then(r => r.json()),

  listArtifacts: (executionId: string): Promise<string[]> =>
    fetch(`${BASE}/api/executions/${executionId}/artifacts`).then(r => r.json()),

  artifactDownloadURL: (executionId: string, filename: string) =>
    `${BASE}/api/executions/${executionId}/artifacts/${encodeURIComponent(filename)}`,

  /** FaaS / Railway dashboard — same-origin Next routes */
  listFaasTemplates: (): Promise<{ templates: FaasTemplate[] }> =>
    fetch('/api/faas/templates').then(parseResponse) as Promise<{ templates: FaasTemplate[] }>,

  listFaasContainers: (): Promise<{ containers: FaasContainer[] }> =>
    fetch('/api/faas/containers').then(parseResponse) as Promise<{ containers: FaasContainer[] }>,

  createFaasContainer: (templateId: string): Promise<{ container: FaasContainer }> =>
    fetch('/api/faas/containers', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ templateId }),
    }).then(parseResponse) as Promise<{ container: FaasContainer }>,

  getFaasContainer: (id: string): Promise<{ container: FaasContainer }> =>
    fetch(`/api/faas/containers/${encodeURIComponent(id)}`).then(parseResponse) as Promise<{ container: FaasContainer }>,

  stopFaasContainer: (id: string): Promise<{ container: FaasContainer }> =>
    fetch(`/api/faas/containers/${encodeURIComponent(id)}/stop`, { method: 'POST' }).then(parseResponse) as Promise<{ container: FaasContainer }>,

  testFaasContainer: (
    id: string,
    body?: Record<string, unknown>,
  ): Promise<{ ok: boolean; status: number; result: unknown }> =>
    fetch(`/api/faas/containers/${encodeURIComponent(id)}/test`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ body }),
    }).then(parseResponse) as Promise<{ ok: boolean; status: number; result: unknown }>,

  // RL Training runs
  startRLRun: (body: {
    backend: 'slime' | 'skyscale'
    base_model: string
    num_workers: number
    gpu_model: string
    problem_set?: string
    control_plane_url?: string
  }): Promise<{
    run_id: string
    status: string
    backend: string
    trainer_exec_id?: string
    worker_exec_ids?: string[]
    snapshot_sha256?: string
    namespace?: string
  }> =>
    fetch('/api/rl/runs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(parseResponse) as Promise<{
      run_id: string
      status: string
      backend: string
      trainer_exec_id?: string
      worker_exec_ids?: string[]
      snapshot_sha256?: string
      namespace?: string
    }>,

  listRLRuns: (): Promise<RLRun[]> =>
    fetch('/api/rl/runs').then(parseResponse) as Promise<RLRun[]>,

  getRLRun: (id: string): Promise<RLRunDetail> =>
    fetch(`/api/rl/runs/${encodeURIComponent(id)}`).then(parseResponse) as Promise<RLRunDetail>,

  getRLEvents: (id: string, limit = 100): Promise<{ run_id: string; events: RLEvent[] }> =>
    fetch(`/api/rl/runs/${encodeURIComponent(id)}/events?limit=${limit}`).then(parseResponse) as Promise<{ run_id: string; events: RLEvent[] }>,

  stopRLRun: (id: string): Promise<void> =>
    fetch(`/api/rl/runs/${encodeURIComponent(id)}`, { method: 'DELETE' }).then(() => undefined),

  getRLProblems: (): Promise<RLProblem[]> =>
    fetch('/api/rl/env/problems').then(parseResponse) as Promise<RLProblem[]>,

  getRLBufferStats: (runId: string): Promise<{ run_id: string; size: number }> =>
    fetch(`/api/rl/buffer/stats?run_id=${encodeURIComponent(runId)}`).then(parseResponse) as Promise<{ run_id: string; size: number }>,
}

export interface RLRun {
  ID: string
  Status: string
  Backend?: string
  DesiredState?: string
  ObservedState?: string
  BaseModel: string
  NumWorkers: number
  GPUModel: string
  Namespace?: string
  OptimizerStep?: number
  CheckpointID?: string
  PolicyVersion?: string
  PolicyServerURL: string
  TrainerExecID: string
  WorkerExecIDs: string
  CreatedAt: string
  UpdatedAt: string
}

export interface RLRunDetail {
  run: RLRun
  trainer_status: string
  worker_statuses: Array<{ id: string; status: string }>
  buffer_size: number
  metrics: Array<{ step: number; episode_reward: number; loss: number; gpu_util: number; timestamp: number }>
  stage?: string
  policy_status?: string
  event_count?: number
  grafana_url?: string
}

export interface RLEvent {
  timestamp: string
  run_id: string
  component: string
  level: string
  message: string
}

export interface RLProblem {
  id: string
  prompt: string
  test_cases: string[]
  difficulty: string
}

export function getRealtimeURL() {
  const base = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'
  const url = new URL('/api/ws/executions', base)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'

  if (typeof window !== 'undefined' && window.location.protocol === 'https:' && url.protocol === 'ws:') {
    return null
  }

  return url.toString()
}
