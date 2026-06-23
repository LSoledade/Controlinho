// Controlinho service worker.
//
// Goal: make the PWA installable AND load offline (the UI shell keeps working
// even if the phone briefly drops Wi-Fi; only the live commands need the socket).
//
// Strategy:
//   - On install: precache the app shell (index.html, manifest, icons).
//   - On fetch:   network-first for navigations / HTML documents (the index
//                 shell), so a rebuilt UI shows up the moment you're online and
//                 only falls back to the cached shell when offline; stale-while-
//                 revalidate (cache-first + background refresh) for the static,
//                 rarely-changing assets (icons, manifest, scripts). Non-GET and
//                 websocket requests pass straight through (an SW can't proxy WS).
//   - On activate: drop old caches.
//
// Versioning: with network-first navigations you no longer need to bump CACHE on
// every UI tweak — the freshest index.html arrives over the network. Still bump
// on breaking changes to the cached static assets. The new SW takes over on next
// navigation (clients.claim) so you don't have to close every tab.

const CACHE = "pcremote-v6";
const SHELL = [
  "./",
  "./index.html",
  "./style.css",
  "./manifest.webmanifest",
  "./icon-192.png",
  "./icon-512.png",
  "./icon-maskable-512.png",
  "./js/app.js",
  "./js/state.js",
  "./js/ui.js",
  "./js/connection.js",
  "./js/trackpad.js",
  "./js/controls.js",
  "./js/settings.js",
  "./js/pwa.js",
  "./js/setup.js"
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

  // Navigations / HTML documents → network-first: always try the live shell so
  // a rebuilt index.html is served the instant we're online, update the cache on
  // success, and only fall back to the cached shell when offline.
  const isNavigation =
    req.mode === "navigate" ||
    (req.headers.get("accept") || "").includes("text/html");
  if (isNavigation) {
    event.respondWith(
      fetch(req).then((res) => {
        // Only cache successful, basic responses (avoid opaque/error).
        if (res && res.status === 200 && res.type === "basic") {
          const copy = res.clone();
          caches.open(CACHE).then((c) => c.put(req, copy));
        }
        return res;
      }).catch(() => caches.match(req).then((cached) => cached || caches.match("./")))
    );
    return;
  }

  // Static/immutable assets → cache-first, then network, refreshing in the
  // background (stale-while-revalidate) for instant loads.
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
