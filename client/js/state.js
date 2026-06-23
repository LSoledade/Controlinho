// state.js — config keys, persisted settings, and small shared helpers.
//
// This module is the dependency leaf: it imports nothing. It owns the mutable
// `settings` object that the rest of the UI reads live (trackpad/controls) and the
// Ajustes panel writes. Environment flags (standalone/localhost/iOS) live here too
// so connection, pwa, and setup all agree on them.

const STORAGE_KEY = "pcremote.host", SENS_KEY = "pcremote.sens";
const SCROLL_KEY = "pcremote.scroll", NATURAL_KEY = "pcremote.natural", ENTER_KEY = "pcremote.enter";
const HAPTICS_KEY = "pcremote.haptics", TOKEN_KEY = "pcremote.token";

// Storage keys other modules need to read/write (Ajustes persistence, boot probe).
export const KEYS = { STORAGE_KEY, SENS_KEY, SCROLL_KEY, NATURAL_KEY, ENTER_KEY, HAPTICS_KEY, TOKEN_KEY };

export function getHost() { return localStorage.getItem(STORAGE_KEY) || location.host || "127.0.0.1:8443"; }
export function setHost(h) { localStorage.setItem(STORAGE_KEY, h); }
export function getToken() { return localStorage.getItem(TOKEN_KEY) || ""; }
export function setToken(t) { try { localStorage.setItem(TOKEN_KEY, t); } catch (e) {} }

// Pairing token: scanning the green QR (or tapping the setup page's "secure app"
// link) lands here with ?k=…. Persist it — the installed PWA's start_url has no
// token — and strip it from the visible URL.
(function () {
  try {
    const p = new URLSearchParams(location.search);
    const k = p.get("k");
    if (k) {
      setToken(k);
      p.delete("k");
      const q = p.toString();
      history.replaceState(null, "", location.pathname + (q ? "?" + q : "") + location.hash);
    }
  } catch (e) {}
})();

export function normalizeHost(h) {
  h = (h || "").trim().replace(/^wss?:\/\//i, "").replace(/^https?:\/\//i, "").replace(/\/.*$/, "");
  if (!h) return "";
  if (h.indexOf(":") === -1) h += (location.protocol === "https:" ? ":8443" : ":8080");
  return h;
}

// Live settings: trackpad/controls read these on every event; Ajustes mutates the
// same object in place (a stable reference, so the live bindings keep working).
export const settings = {
  sensitivity: parseFloat(localStorage.getItem(SENS_KEY) || "1.5") || 1.5,
  scrollSpeed: parseFloat(localStorage.getItem(SCROLL_KEY) || "1") || 1,
  naturalScroll: localStorage.getItem(NATURAL_KEY) === "1",
  enterAfterText: localStorage.getItem(ENTER_KEY) === "1",
  haptics: localStorage.getItem(HAPTICS_KEY) !== "0", // default ON
};

export function buzz(ms) { try { if (settings.haptics && navigator.vibrate) navigator.vibrate(ms || 10); } catch (e) {} }

// Environment flags shared across modules.
export const isStandalone = window.matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
export const isLocalhost = /^(localhost|127\.0\.0\.1|\[::1\])(:\d+)?$/i.test(location.host);
export const isIOS = /iphone|ipad|ipod/i.test(navigator.userAgent);
