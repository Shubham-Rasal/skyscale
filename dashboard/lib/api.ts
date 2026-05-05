const BASE = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

async function parseResponse(r: Response) {
  const text = await r.text()
  if (!r.ok) throw new Error(text.trim() || `HTTP ${r.status}`)
  try { return JSON.parse(text) } catch { return text }
}

export const api = {
  listFunctions: () =>
    fetch(`${BASE}/api/functions`).then(r => r.json()),

  invoke: (name: string, body: Record<string, unknown>) =>
    fetch(`${BASE}/api/functions/name/${encodeURIComponent(name)}/invoke`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(parseResponse),

  submitTrainingJob: (body: Record<string, unknown>) =>
    fetch(`${BASE}/api/training/jobs`, {
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
}

export const WS_URL =
  (process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080')
    .replace(/^http/, 'ws') + '/api/ws/executions'
