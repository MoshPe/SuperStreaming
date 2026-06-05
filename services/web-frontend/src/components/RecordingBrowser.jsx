import { useEffect, useRef, useState } from 'react'
import { Search, Play, Clock, Video } from 'lucide-react'
import { useRecordings } from '../hooks/useRecordings.js'

function formatTime(iso) {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

function formatDuration(s) {
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const sec = s % 60
  return `${m}m ${sec}s`
}

export default function RecordingBrowser() {
  const { segments, total, loading, error, fetch: loadRecordings } = useRecordings()
  const [streamId, setStreamId] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [activeUrl, setActiveUrl] = useState(null)
  const [activeId, setActiveId] = useState(null)
  const videoRef = useRef(null)

  function handleSearch(e) {
    e.preventDefault()
    loadRecordings({
      streamId: streamId.trim() || undefined,
      from: from ? new Date(from).toISOString() : undefined,
      to: to ? new Date(to).toISOString() : undefined,
    })
  }

  function handlePlay(seg) {
    setActiveUrl(seg.url)
    setActiveId(seg.id)
  }

  useEffect(() => {
    if (videoRef.current && activeUrl) {
      videoRef.current.load()
      videoRef.current.play().catch(() => {})
    }
  }, [activeUrl])

  // Load first page on mount
  useEffect(() => { loadRecordings() }, [loadRecordings])

  return (
    <div className="flex flex-col h-full bg-black">
      {/* Filter bar */}
      <form
        onSubmit={handleSearch}
        className="flex flex-wrap items-end gap-3 px-4 py-3 bg-surface border-b border-border"
      >
        <div className="flex flex-col gap-1">
          <label className="text-xs text-slate-400" htmlFor="sid">Stream ID</label>
          <input
            id="sid"
            type="text"
            value={streamId}
            onChange={e => setStreamId(e.target.value)}
            placeholder="All streams"
            className="bg-elevated border border-border rounded px-2.5 py-1.5 text-sm text-slate-200 placeholder-slate-500 w-40 focus:outline-none focus:border-rose-500"
          />
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs text-slate-400" htmlFor="from">From</label>
          <input
            id="from"
            type="datetime-local"
            value={from}
            onChange={e => setFrom(e.target.value)}
            className="bg-elevated border border-border rounded px-2.5 py-1.5 text-sm text-slate-200 focus:outline-none focus:border-rose-500"
          />
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs text-slate-400" htmlFor="to">To</label>
          <input
            id="to"
            type="datetime-local"
            value={to}
            onChange={e => setTo(e.target.value)}
            className="bg-elevated border border-border rounded px-2.5 py-1.5 text-sm text-slate-200 focus:outline-none focus:border-rose-500"
          />
        </div>

        <button
          type="submit"
          className="flex items-center gap-1.5 bg-rose-600 hover:bg-rose-500 transition-colors px-3 py-1.5 rounded text-sm font-medium text-white cursor-pointer"
        >
          <Search size={14} />
          Search
        </button>
      </form>

      {/* Content: table + player split */}
      <div className="flex flex-1 min-h-0">
        {/* Segment list */}
        <div className="flex flex-col flex-1 min-w-0 overflow-hidden">
          {/* Results header */}
          <div className="px-4 py-2 text-xs text-slate-500 border-b border-border flex-shrink-0">
            {loading ? 'Loading…' : error ? (
              <span className="text-rose-400">Error: {error}</span>
            ) : (
              <span>{total} segment{total !== 1 ? 's' : ''}</span>
            )}
          </div>

          {/* Table */}
          <div className="flex-1 overflow-y-auto">
            {!loading && segments.length === 0 && (
              <div className="flex flex-col items-center justify-center h-40 text-slate-500">
                <Video size={24} className="mb-2 opacity-40" />
                <p className="text-sm">No recordings found</p>
              </div>
            )}
            <table className="w-full text-sm">
              <thead className="sticky top-0 bg-surface text-xs text-slate-400 uppercase tracking-wide">
                <tr>
                  <th className="text-left px-4 py-2 font-medium">Stream</th>
                  <th className="text-left px-4 py-2 font-medium">Start</th>
                  <th className="text-left px-4 py-2 font-medium">
                    <Clock size={12} className="inline mr-1" />
                    Duration
                  </th>
                  <th className="px-4 py-2" />
                </tr>
              </thead>
              <tbody>
                {segments.map((seg) => (
                  <tr
                    key={seg.id}
                    className={[
                      'border-t border-border/50 transition-colors',
                      activeId === seg.id ? 'bg-elevated' : 'hover:bg-surface/80',
                    ].join(' ')}
                  >
                    <td className="px-4 py-2 font-mono text-xs text-slate-300">{seg.stream_id}</td>
                    <td className="px-4 py-2 text-slate-400 text-xs">{formatTime(seg.start_time)}</td>
                    <td className="px-4 py-2 text-slate-400 text-xs tabular-nums">{formatDuration(seg.duration_s)}</td>
                    <td className="px-4 py-2 text-right">
                      <button
                        onClick={() => handlePlay(seg)}
                        className="inline-flex items-center gap-1 text-xs text-rose-400 hover:text-rose-300 cursor-pointer transition-colors"
                        aria-label={`Play ${seg.stream_id} at ${formatTime(seg.start_time)}`}
                      >
                        <Play size={12} />
                        Play
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Playback panel */}
        <div className="w-96 flex-shrink-0 flex flex-col border-l border-border bg-surface">
          <div className="px-3 py-2.5 border-b border-border text-xs text-slate-400 font-medium uppercase tracking-wider">
            Playback
          </div>
          <div className="flex-1 flex items-center justify-center bg-black">
            {activeUrl ? (
              <video
                ref={videoRef}
                src={activeUrl}
                controls
                className="w-full max-h-full"
                playsInline
              />
            ) : (
              <p className="text-sm text-slate-500">Select segment to play</p>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
