'use client'

import { useEffect, useRef, useState } from 'react'
import { WSPayload } from '@/types'
import { getRealtimeURL } from '@/lib/api'

const EMPTY: WSPayload = {
  active: [],
  recent: [],
  vm_pool: [],
  gpu: [],
  training_metrics: {},
}

export function useRealtimeStream() {
  const [data, setData] = useState<WSPayload>(EMPTY)
  const [connected, setConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    function connect() {
      const url = getRealtimeURL()
      if (!url) {
        setConnected(false)
        return
      }

      let ws: WebSocket
      try {
        ws = new WebSocket(url)
      } catch {
        setConnected(false)
        return
      }
      wsRef.current = ws

      ws.onopen = () => setConnected(true)

      ws.onmessage = (e) => {
        try {
          setData(JSON.parse(e.data) as WSPayload)
        } catch {
          // ignore malformed frames
        }
      }

      ws.onclose = () => {
        setConnected(false)
        // auto-reconnect after 2s
        timerRef.current = setTimeout(connect, 2000)
      }

      ws.onerror = () => ws.close()
    }

    connect()

    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
      wsRef.current?.close()
    }
  }, [])

  return { data, connected }
}
