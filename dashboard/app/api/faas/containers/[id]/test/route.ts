export const runtime = 'nodejs'

import { testFaasContainer } from '@/lib/faas/service'
import { NextResponse } from 'next/server'

export async function POST(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  const { id } = await context.params
  try {
    let body: Record<string, unknown> | undefined
    try {
      const json = (await request.json()) as { body?: Record<string, unknown> }
      body = json.body
    } catch {
      body = undefined
    }
    const result = await testFaasContainer(id, body)
    return NextResponse.json(result)
  } catch (e) {
    const message = e instanceof Error ? e.message : String(e)
    return NextResponse.json({ error: message, ok: false }, { status: 400 })
  }
}
