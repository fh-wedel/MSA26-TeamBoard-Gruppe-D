import { useEffect, useRef, useCallback } from 'react'
import { getAccessToken } from '../api/client'

type WsMessage = { type: string; payload: unknown }
type Handler = (msg: WsMessage) => void

export function useWebSocket(onMessage: Handler, enabled = true) {
  const wsRef = useRef<WebSocket | null>(null)
  const retryRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage

  const connect = useCallback(() => {
    const token = getAccessToken()
    if (!token) return

    const url = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/ws?token=${token}`
    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onmessage = (e) => {
      try { onMessageRef.current(JSON.parse(e.data)) } catch {}
    }

    ws.onclose = () => {
      retryRef.current = setTimeout(connect, 3000)
    }
  }, [])

  useEffect(() => {
    if (!enabled) return
    connect()
    return () => {
      wsRef.current?.close()
      if (retryRef.current) clearTimeout(retryRef.current)
    }
  }, [connect, enabled])
}
