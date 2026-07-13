import { useEffect, useRef, useCallback } from 'react'
import { getAccessToken } from '../api/client'

type WsMessage = { type: string; id?: string; data?: any }
type Handler = (msg: WsMessage) => void

// Pass `channels` to auto-subscribe (and re-subscribe on reconnect) to server
// push channels, e.g. `board:{boardId}`. See notification service's ws proto.
export function useWebSocket(onMessage: Handler, enabled = true, channels: string[] = []) {
  const wsRef = useRef<WebSocket | null>(null)
  const retryRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const closedByUs = useRef(false)
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage
  const channelsKey = channels.join(',')

  const connect = useCallback(() => {
    const token = getAccessToken()
    // The access token lives in memory and may not be set yet on a cold load
    // (the auth refresh runs asynchronously at startup). Retry shortly instead
    // of giving up, otherwise the socket would stay dead until a full reload.
    if (!token) {
      retryRef.current = setTimeout(connect, 500)
      return
    }

    const url = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/ws?token=${token}`
    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onopen = () => {
      if (channelsKey) {
        ws.send(JSON.stringify({ type: 'subscribe', data: { channels: channelsKey.split(',') } }))
      }
    }

    ws.onmessage = (e) => {
      try { onMessageRef.current(JSON.parse(e.data)) } catch {}
    }

    ws.onclose = () => {
      if (closedByUs.current) return
      retryRef.current = setTimeout(connect, 3000)
    }
  }, [channelsKey])

  useEffect(() => {
    if (!enabled) return
    closedByUs.current = false
    connect()
    return () => {
      closedByUs.current = true
      wsRef.current?.close()
      if (retryRef.current) clearTimeout(retryRef.current)
    }
  }, [connect, enabled])
}
