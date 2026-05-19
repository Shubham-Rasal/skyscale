export const runtime = 'nodejs'

import { stopFaasContainer } from '@/lib/faas/service'
import { NextResponse } from 'next/server'

export async function POST(
  _request: Request,
  context: { params: Promise<{ id: string }> },
) {
  const { id } = await context.params
  try {
    const container = await stopFaasContainer(id)
    if (!container) {
      return NextResponse.json({ error: 'Not found' }, { status: 404 })
    }
    return NextResponse.json({ container })
  } catch (e) {
    const message = e instanceof Error ? e.message : String(e)
    return NextResponse.json({ error: message }, { status: 500 })
  }
}
