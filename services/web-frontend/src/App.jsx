import { useState } from 'react'
import { Radio, Film } from 'lucide-react'
import LiveViewer from './components/LiveViewer.jsx'
import RecordingBrowser from './components/RecordingBrowser.jsx'

const TABS = [
  { id: 'live', label: 'Live', Icon: Radio },
  { id: 'recordings', label: 'Recordings', Icon: Film },
]

export default function App() {
  const [tab, setTab] = useState('live')

  return (
    <div className="flex flex-col h-screen bg-black">
      {/* Top bar */}
      <header className="flex items-center justify-between px-4 h-12 bg-surface border-b border-border flex-shrink-0">
        <span className="font-semibold text-sm tracking-widest text-slate-300 uppercase">
          SuperStreaming
        </span>
        <nav className="flex gap-1">
          {TABS.map(({ id, label, Icon }) => (
            <button
              key={id}
              onClick={() => setTab(id)}
              className={[
                'flex items-center gap-1.5 px-3 py-1.5 rounded text-sm font-medium transition-colors duration-150 cursor-pointer',
                tab === id
                  ? 'bg-elevated text-white'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-surface',
              ].join(' ')}
            >
              <Icon size={14} />
              {label}
            </button>
          ))}
        </nav>
      </header>

      {/* Content */}
      <main className="flex-1 min-h-0">
        {tab === 'live' ? <LiveViewer /> : <RecordingBrowser />}
      </main>
    </div>
  )
}
