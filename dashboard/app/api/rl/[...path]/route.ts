/**
 * Catch-all proxy for /api/rl/* → control plane /api/rl/*
 * Forwards GET, POST, DELETE without auth for internal RL worker calls.
 * Dashboard calls (start/stop run) are proxied here too.
 */
export const runtime = 'nodejs'

const CONTROL_PLANE_URL =
  process.env.SKYSCALE_CONTROL_PLANE_URL ??
  process.env.NEXT_PUBLIC_API_URL ??
  'http://localhost:8080'

async function proxy(request: Request, { params }: { params: Promise<{ path: string[] }> }) {
  const { path: segments } = await params
  const path = segments.join('/')
  const url = new URL(request.url)
  const upstream = new URL(`${CONTROL_PLANE_URL}/api/rl/${path}`)
  upstream.search = url.search

  const headers: Record<string, string> = {}
  const ct = request.headers.get('content-type')
  if (ct) headers['Content-Type'] = ct

  const body = ['GET', 'HEAD', 'DELETE'].includes(request.method)
    ? undefined
    : await request.text()

  const res = await fetch(upstream.toString(), {
    method: request.method,
    headers,
    body,
    cache: 'no-store',
  })

  return new Response(res.body, {
    status: res.status,
    headers: {
      'Content-Type': res.headers.get('content-type') ?? 'application/json',
    },
  })
}

export const GET = proxy
export const POST = proxy
export const DELETE = proxy
