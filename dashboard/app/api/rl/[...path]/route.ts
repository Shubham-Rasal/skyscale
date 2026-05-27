/**
 * Catch-all proxy for /api/rl/* → control plane /api/rl/*
 * GPU spend actions (start/stop RL runs) require a signed-in dashboard session.
 */
export const runtime = 'nodejs'

import {
  controlPlaneAuthHeaders,
  isRlRunSpendMethod,
  requireSession,
} from '@/lib/require-auth'

const CONTROL_PLANE_URL =
  process.env.SKYSCALE_CONTROL_PLANE_URL ??
  process.env.NEXT_PUBLIC_API_URL ??
  'http://localhost:8080'

async function proxy(request: Request, { params }: { params: Promise<{ path: string[] }> }) {
  const { path: segments } = await params
  const path = segments.join('/')

  if (isRlRunSpendMethod(request.method, path)) {
    const denied = await requireSession(
      request,
      'Sign in to start or stop GPU RL training runs.',
    )
    if (denied) return denied
  }

  const url = new URL(request.url)
  const upstream = new URL(`${CONTROL_PLANE_URL}/api/rl/${path}`)
  upstream.search = url.search

  const body = ['GET', 'HEAD', 'DELETE'].includes(request.method)
    ? undefined
    : await request.text()

  const res = await fetch(upstream.toString(), {
    method: request.method,
    headers: controlPlaneAuthHeaders(
      request.headers.get('content-type')
        ? { 'Content-Type': request.headers.get('content-type')! }
        : undefined,
    ),
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
