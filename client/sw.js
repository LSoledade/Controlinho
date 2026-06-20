// PC Remote service worker.
//
// Goal: make the PWA installable AND load offline (the UI shell keeps working
// even if the phone briefly drops Wi-Fi; only the live commands need the socket).
//
// Strategy:
//   - On install: precache the app shell (index.html, manifest, icons).
//   - On fetch:   cache-first for the shell assets; network fallback that also
//                 stashes a fresh copy. Non-GET and websocket requests are
//                 passed straight through (a service worker can't proxy WS).
//   - On activate: drop old caches.
//
// Versioning: bump CACHE on any breaking shell change. The new SW takes over
// on next navigation (clients.claim) so you don't have to close every tab.

const CACHE = "pcremote-v4";
const SHELL = [
  "./",
  "./index.html",
  "./manifest.webmanifest",
  "./icon-192.png",
  "./icon-512.png",
  "./icon-maskable-512.png"
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;

  // Only handle same-origin GETs; let everything else hit the network.
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  // Never touch the websocket endpoint or the dynamic/bootstrap endpoints —
  // /info must always be live, /ca.crt is a download, /ws can't be proxied.
  if (url.pathname === "/ws" || url.pathname === "/info" || url.pathname === "/ca.crt") return;

  // Cache-first, then network — and refresh the cache in the background.
  event.respondWith(
    caches.match(req).then((cached) => {
      const network = fetch(req).then((res) => {
        // Only cache successful, basic responses (avoid opaque/error).
        if (res && res.status === 200 && res.type === "basic") {
          const copy = res.clone();
          caches.open(CACHE).then((c) => c.put(req, copy));
        }
        return res;
      }).catch(() => cached); // offline: fall back to cache if fetch fails
      return cached || network;
    })
  );
});
