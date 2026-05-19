export const runtime = 'nodejs'

import { getFaasContainerAsync } from '@/lib/faas/service'
import { NextResponse } from 'next/server'

export async function GET(
  _request: Request,
  context: { params: Promise<{ id: string }> },
) {
  const { id } = await context.params
  const container = await getFaasContainerAsync(id)
  if (!container) {
    return NextResponse.json({ error: 'Not found' }, { status: 404 })
  }
  return NextResponse.json({ container })
}
