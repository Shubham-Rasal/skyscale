export const runtime = 'nodejs'

import { createFaasContainer, listFaasContainersAsync } from '@/lib/faas/service'
import { NextResponse } from 'next/server'

export async function GET() {
  const containers = await listFaasContainersAsync()
  return NextResponse.json({ containers })
}

export async function POST(request: Request) {
  try {
    const body = (await request.json()) as { templateId?: string }
    const templateId = body.templateId?.trim()
    if (!templateId) {
      return NextResponse.json({ error: 'templateId is required' }, { status: 400 })
    }
    const container = await createFaasContainer(templateId)
    return NextResponse.json({ container })
  } catch (e) {
    const message = e instanceof Error ? e.message : String(e)
    return NextResponse.json({ error: message }, { status: 400 })
  }
}
