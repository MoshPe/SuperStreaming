const w = (typeof window !== 'undefined' && window.__ENV__) || {}

function get(windowKey, viteKey, fallback) {
  const v = w[windowKey] || import.meta.env[viteKey] || fallback
  // envsubst leaves placeholder unchanged if var unset; treat as fallback
  return v.startsWith('${') ? fallback : v
}

export const MEDIAMTX_WHEP_BASE = get('MEDIAMTX_WHEP_BASE', 'VITE_MEDIAMTX_WHEP_BASE', 'http://localhost:8889/')
export const API_SERVER_URL = get('API_SERVER_URL', 'VITE_API_SERVER_URL', 'http://localhost:8080')
export const VIEWER_PASSWORD = get('VIEWER_PASSWORD', 'VITE_VIEWER_PASSWORD', '')

export function whepUrl(streamId) {
  const base = MEDIAMTX_WHEP_BASE.endsWith('/') ? MEDIAMTX_WHEP_BASE : MEDIAMTX_WHEP_BASE + '/'
  return `${base}${streamId}/whep`
}

export function readerJsUrl() {
  const base = MEDIAMTX_WHEP_BASE.endsWith('/') ? MEDIAMTX_WHEP_BASE : MEDIAMTX_WHEP_BASE + '/'
  return `${base}webrtc/js/reader.js`
}
