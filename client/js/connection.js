// connection.js — the WebSocket to the PC: connect/reconnect with backoff, the
// send queue, and the status pill. Every control panel sends through `send()`.
import { buzz, getHost, getToken, normalizeHost } from "./state.js";
import { toast } from "./ui.js";

const dot = document.getElementById("dot");
const statusText = document.getElementById("statusText");
let ws = null, manualClose = false, reconnectDelay = 500, reconnectTimer = null, pending = [];
let wsGen = 0; // monotonic connection generation: stale sockets no-op their handlers
const RECONNECT_BASE = 500;
const PENDING_MAX = 200;
function setStatus(state, text) { dot.className = "dot " + state; statusText.textContent = text; }

export function send(obj) {
  const data = JSON.stringify(obj);
  if (ws && ws.readyState === WebSocket.OPEN) ws.send(data);
  else {
    if (obj.type === "mouse_move" || obj.type === "mouse_scroll") pending = pending.filter(m => m.type !== obj.type);
    pending.push(obj); if (pending.length > PENDING_MAX) pending.shift();
  }
}
function flushPending() { if (!ws || ws.readyState !== WebSocket.OPEN) return; while (pending.length) ws.send(JSON.stringify(pending.shift())); }

export function connect() {
  const host = normalizeHost(getHost());
  if (!host) { setStatus("offline", "configure o IP"); return; }
  const tok = getToken();
  // Every WS now requires the pairing token, so without one the upgrade would just
  // 403. Stop and tell the user to pair (scan the QR or enter the PIN) instead of
  // looping reconnects. Saving a PIN in Ajustes calls connect() again, which will
  // then have a token. (On the HTTP setup origin localStorage is empty by design —
  // its job is CA install + jumping to the secure app, not controlling.)
  if (!tok) { setStatus("offline", "informe o PIN nos ajustes"); return; }
  if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
  // tear down any previous socket so its onclose can't schedule a duplicate reconnect
  if (ws) { ws.onopen = ws.onclose = ws.onerror = ws.onmessage = null; try { ws.close(); } catch (e) {} ws = null; }
  const gen = ++wsGen; // this connection's identity
  manualClose = false; setStatus("connecting", "conectando…");
  const url = (location.protocol === "https:" ? "wss://" : "ws://") + host + "/ws" + (tok ? "?k=" + encodeURIComponent(tok) : "");
  let sock;
  try { sock = new WebSocket(url); } catch (e) { scheduleReconnect(); return; }
  ws = sock;
  sock.onopen = () => { if (gen !== wsGen) return; setStatus("online", "conectado"); reconnectDelay = RECONNECT_BASE; flushPending(); };
  sock.onclose = () => { if (gen !== wsGen) return; if (manualClose) { setStatus("offline", "desconectado"); return; } setStatus("offline", "reconectando…"); scheduleReconnect(); };
  sock.onerror = () => { if (gen !== wsGen) return; setStatus("offline", "erro de conexão"); };
  sock.onmessage = (ev) => { if (gen !== wsGen) return; try { const m = JSON.parse(ev.data); if (m && m.ok === false && m.error) toast("Erro: " + m.error); } catch (e) {} };
}
function scheduleReconnect() {
  if (reconnectTimer) return;
  reconnectTimer = setTimeout(() => { reconnectTimer = null; reconnectDelay = Math.min(reconnectDelay * 1.7, 8000); connect(); }, reconnectDelay);
}
setInterval(() => { if (ws && ws.readyState === WebSocket.OPEN) { try { ws.send(JSON.stringify({ type: "ping" })); } catch (e) {} } }, 20000);
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState !== "visible") return;
  if (!ws || ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING) connect();
});
// tap the status chip to force an immediate reconnect
function forceReconnect() {
  buzz(12);
  reconnectDelay = RECONNECT_BASE;
  if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
  connect();
}
const statusChip = document.getElementById("statusChip");
statusChip.addEventListener("click", forceReconnect);
statusChip.addEventListener("keydown", e => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); forceReconnect(); } });
