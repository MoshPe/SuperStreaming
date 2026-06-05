import { useCallback, useState } from 'react'
import { API_SERVER_URL } from '../env.js'

export function useRecordings() {
  const [segments, setSegments] = useState([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  const fetch_ = useCallback(async ({ streamId, from, to, limit = 100, offset = 0 } = {}) => {
    setLoading(true)
    setError(null)

    const params = new URLSearchParams()
    if (streamId) params.set('stream_id', streamId)
    if (from) params.set('from', from)
    if (to) params.set('to', to)
    params.set('limit', String(limit))
    params.set('offset', String(offset))

    try {
      const res = await fetch(`${API_SERVER_URL}/api/recordings?${params}`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setSegments(data.segments ?? [])
      setTotal(data.total ?? 0)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [])

  return { segments, total, loading, error, fetch: fetch_ }
}
