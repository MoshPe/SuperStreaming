// Headless WebRTC WHEP render test.
// Verifies: read replica pulls cam-01 from origin and delivers decodable
// video frames to a browser over WebRTC. Exits 0 on first frame, 1 on timeout.
import { chromium } from 'playwright';

const WHEP = process.env.WHEP_URL || 'http://localhost:8889/cam-01/whep';
const USER = process.env.VIEWER_USER || 'viewer';
const PASS = process.env.VIEWER_PASS || 'devviewer123';
const TIMEOUT_MS = 25000;

const browser = await chromium.launch({ args: ['--use-fake-ui-for-media-stream'] });
const page = await browser.newPage();
page.on('console', (m) => console.log('  [browser]', m.text()));

const result = await page.evaluate(async ({ WHEP, USER, PASS, TIMEOUT_MS }) => {
  const auth = 'Basic ' + btoa(`${USER}:${PASS}`);
  const pc = new RTCPeerConnection();
  pc.addTransceiver('video', { direction: 'recvonly' });
  pc.addTransceiver('audio', { direction: 'recvonly' });

  const framePromise = new Promise((resolve) => {
    pc.ontrack = (ev) => {
      if (ev.track.kind !== 'video') return;
      const v = document.createElement('video');
      v.autoplay = true; v.muted = true; v.srcObject = new MediaStream([ev.track]);
      document.body.appendChild(v);
      const t = setInterval(() => {
        if (v.videoWidth > 0 && v.videoHeight > 0) {
          clearInterval(t);
          resolve({ ok: true, w: v.videoWidth, h: v.videoHeight });
        }
      }, 200);
    };
  });

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  // wait for ICE gathering to finish (non-trickle WHEP)
  await new Promise((r) => {
    if (pc.iceGatheringState === 'complete') return r();
    pc.onicegatheringstatechange = () => pc.iceGatheringState === 'complete' && r();
    setTimeout(r, 3000);
  });

  const res = await fetch(WHEP, {
    method: 'POST',
    headers: { 'Content-Type': 'application/sdp', Authorization: auth },
    body: pc.localDescription.sdp,
  });
  if (res.status !== 201) return { ok: false, stage: 'signaling', status: res.status };
  const answer = await res.text();
  await pc.setRemoteDescription({ type: 'answer', sdp: answer });

  const timeout = new Promise((r) => setTimeout(() => r({ ok: false, stage: 'no-frame' }), TIMEOUT_MS));
  return Promise.race([framePromise, timeout]);
}, { WHEP, USER, PASS, TIMEOUT_MS });

console.log('RESULT:', JSON.stringify(result));
await browser.close();
process.exit(result.ok ? 0 : 1);
