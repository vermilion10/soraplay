// Service worker: makes SoraPlay installable as an app. It caches nothing, so
// videos, API calls and updates always come straight from the server; when the
// server can't be reached (homelab off, Tailscale disconnected) page loads get a
// short message instead of the browser's error page.
self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()))

const OFFLINE = `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>SoraPlay</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#13110f;color:#f2ece4;
font:16px system-ui,sans-serif;text-align:center;padding:24px}p{color:#a89f94;max-width:320px}button{margin-top:16px;padding:10px 20px;
border:0;border-radius:8px;background:#ff5a36;color:#13110f;font:600 15px system-ui}</style>
<div><h1>Can't reach SoraPlay</h1><p>Check that Tailscale is connected and the homelab is on.</p>
<button onclick="location.reload()">Try again</button></div>`

self.addEventListener('fetch', (e) => {
  if (e.request.mode !== 'navigate') return
  e.respondWith(
    fetch(e.request).catch(
      () => new Response(OFFLINE, { status: 503, headers: { 'Content-Type': 'text/html; charset=utf-8' } }),
    ),
  )
})
