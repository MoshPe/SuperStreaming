import { Wifi, WifiOff } from 'lucide-react'

export default function StreamList({ streams, connected, activeStream, onSelect }) {
  return (
    <div className="flex flex-col h-full bg-surface border-r border-border">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2.5 border-b border-border">
        <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
          Streams
        </span>
        <span title={connected ? 'Connected' : 'Reconnecting…'}>
          {connected
            ? <Wifi size={12} className="text-emerald-400" />
            : <WifiOff size={12} className="text-slate-500 animate-pulse" />}
        </span>
      </div>

      {/* List */}
      <ul className="flex-1 overflow-y-auto py-1">
        {streams.length === 0 && (
          <li className="px-3 py-4 text-xs text-slate-500 text-center">
            No active streams
          </li>
        )}
        {streams.map((name) => (
          <li key={name}>
            <button
              onClick={() => onSelect(name)}
              className={[
                'w-full text-left px-3 py-2 text-sm flex items-center gap-2 transition-colors duration-100 cursor-pointer',
                activeStream === name
                  ? 'bg-elevated text-white'
                  : 'text-slate-300 hover:bg-elevated/50 hover:text-white',
              ].join(' ')}
            >
              {/* Live dot */}
              <span className="relative flex h-2 w-2 flex-shrink-0">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-rose-500 opacity-75" />
                <span className="relative inline-flex rounded-full h-2 w-2 bg-rose-600" />
              </span>
              <span className="truncate font-mono text-xs">{name}</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
