import { useEffect, useRef, useState } from 'react'
import { whepUrl, readerJsUrl, VIEWER_PASSWORD } from '../env.js'

let readerJsPromise = null

function loadReaderJs(src) {
  if (readerJsPromise) return readerJsPromise
  readerJsPromise = new Promise((resolve, reject) => {
    if (window.MediaMTXWebRTCReader) { resolve(); return }
    const script = document.createElement('script')
    script.src = src
    script.onload = resolve
    script.onerror = () => {
      readerJsPromise = null
      reject(new Error('Failed to load reader.js from ' + src))
    }
    document.head.appendChild(script)
  })
  return readerJsPromise
}

export default function VideoPlayer({ streamId }) {
  const videoRef = useRef(null)
  const readerRef = useRef(null)
  const [error, setError] = useState(null)

  useEffect(() => {
    if (!streamId) return
    setError(null)

    loadReaderJs(readerJsUrl())
      .then(() => {
        if (!videoRef.current) return
        const video = videoRef.current
        const opts = {
          url: whepUrl(streamId),
          onTrack: (evt) => {
            if (evt.streams && evt.streams[0]) {
              video.srcObject = evt.streams[0]
            } else {
              video.srcObject = new MediaStream([evt.track])
            }
          },
        }
        if (VIEWER_PASSWORD) {
          opts.user = 'viewer'
          opts.pass = VIEWER_PASSWORD
        }
        const reader = new window.MediaMTXWebRTCReader(opts)
        readerRef.current = reader
      })
      .catch((err) => setError(err.message))

    return () => {
      readerRef.current?.close()
      readerRef.current = null
    }
  }, [streamId])

  if (!streamId) {
    return (
      <div className="flex items-center justify-center h-full text-slate-500 text-sm">
        Select stream from sidebar
      </div>
    )
  }

  return (
    <div className="relative w-full h-full bg-black flex items-center justify-center">
      <video
        ref={videoRef}
        autoPlay
        muted
        playsInline
        className="w-full h-full object-contain"
      />
      {error && (
        <div className="absolute inset-0 flex items-center justify-center bg-black/80">
          <div className="text-center px-6">
            <p className="text-rose-400 text-sm font-medium mb-1">Player error</p>
            <p className="text-slate-400 text-xs">{error}</p>
          </div>
        </div>
      )}
      {/* Stream label */}
      <div className="absolute top-2 left-2 flex items-center gap-1.5 bg-black/60 rounded px-2 py-1">
        <span className="h-1.5 w-1.5 rounded-full bg-rose-500 animate-pulse" />
        <span className="text-xs font-mono text-slate-300">LIVE</span>
        <span className="text-xs font-mono text-slate-400">{streamId}</span>
      </div>
    </div>
  )
}
