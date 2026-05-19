export const runtime = 'nodejs'

import { FAAS_TEMPLATES } from '@/lib/faas/templates'
import { NextResponse } from 'next/server'

export async function GET() {
  return NextResponse.json({ templates: FAAS_TEMPLATES })
}
