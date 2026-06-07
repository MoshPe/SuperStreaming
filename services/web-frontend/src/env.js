const w = (typeof window !== 'undefined' && window.__ENV__) || {}

function get(windowKey, viteKey, fallback) {
  const v = w[windowKey] || import.meta.env[viteKey] || fallback
  return v.startsWith('${') ? fallback : v
}

// READ_BASE_URLS: comma-separated WHEP base URLs indexed by origin_index.
// e.g. "http://node:30889/,http://node:30890/,http://node:30891/"
// Falls back to single MEDIAMTX_WHEP_BASE for local dev / single-origin.
const _readBasesRaw = get('READ_BASE_URLS', 'VITE_READ_BASE_URLS', '')
const _whepFallback = get('MEDIAMTX_WHEP_BASE', 'VITE_MEDIAMTX_WHEP_BASE', 'http://localhost:8889/')

export const READ_BASE_URLS = _readBasesRaw
  ? _readBasesRaw.split(',').map(u => u.trim()).filter(Boolean)
  : [_whepFallback]

export const API_SERVER_URL = get('API_SERVER_URL', 'VITE_API_SERVER_URL', 'http://localhost:8080')
export const VIEWER_PASSWORD = get('VIEWER_PASSWORD', 'VITE_VIEWER_PASSWORD', '')

function baseForOrigin(originIndex) {
  const b = READ_BASE_URLS[originIndex] ?? READ_BASE_URLS[0] ?? 'http://localhost:8889/'
  return b.endsWith('/') ? b : b + '/'
}

export function whepUrl(streamName, originIndex = 0) {
  return `${baseForOrigin(originIndex)}${streamName}/whep`
}

export function readerJsUrl() {
  return `${baseForOrigin(0)}webrtc/js/reader.js`
}
