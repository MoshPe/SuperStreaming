import { useState } from 'react'
import { useSSE } from '../hooks/useSSE.js'
import StreamList from './StreamList.jsx'
import VideoPlayer from './VideoPlayer.jsx'

export default function LiveViewer() {
  const { streams, connected } = useSSE()
  const [activeStream, setActiveStream] = useState(null)

  return (
    <div className="flex h-full">
      {/* Sidebar */}
      <div className="w-48 flex-shrink-0">
        <StreamList
          streams={streams}
          connected={connected}
          activeStream={activeStream}
          onSelect={setActiveStream}
        />
      </div>

      {/* Video pane */}
      <div className="flex-1 min-w-0 bg-black">
        <VideoPlayer streamId={activeStream} />
      </div>
    </div>
  )
}
