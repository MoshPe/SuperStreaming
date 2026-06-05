import { useEffect, useRef, useState } from 'react'
import { API_SERVER_URL } from '../env.js'

export function useSSE() {
  const [streams, setStreams] = useState([])
  const [connected, setConnected] = useState(false)
  const esRef = useRef(null)

  useEffect(() => {
    function connect() {
      const es = new EventSource(`${API_SERVER_URL}/api/streams/live`)
      esRef.current = es

      es.addEventListener('streams', (e) => {
        try {
          const data = JSON.parse(e.data)
          setStreams(data.active ?? [])
          setConnected(true)
        } catch {
          // ignore malformed event
        }
      })

      es.addEventListener('heartbeat', () => {
        setConnected(true)
      })

      es.onerror = () => {
        setConnected(false)
        es.close()
        // SSE auto-reconnects, but we close and reconnect manually for control
        esRef.current = null
        setTimeout(connect, 3000)
      }
    }

    connect()

    return () => {
      esRef.current?.close()
      esRef.current = null
    }
  }, [])

  return { streams, connected }
}
