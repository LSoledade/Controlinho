// trackpad.js — the touch trackpad: 1-finger move (with velocity acceleration),
// 2-finger scroll, tap = left click, 2-finger tap = right click, and the
// hold-to-drag toggle. Reads sensitivity/scroll/natural live from `settings`.
import { settings, buzz } from "./state.js";
import { toast } from "./ui.js";
import { send } from "./connection.js";

const HINT_KEY = "pcremote.hintSeen";
// pointer acceleration: effective = sensitivity * (1 + min(speed * K, CAP))
const ACCEL_K = 6, ACCEL_CAP = 3;
let moveRemX = 0, moveRemY = 0, moveT = 0; // sub-pixel remainder + last sample time

const pad = document.getElementById("pad"), dragToggle = document.getElementById("dragToggle");
const padHint = pad.querySelector(".hint");
const TAP_MS = 220, TAP_MOVE_TOL = 12;
let dragMode = false, dragging = false;
if (localStorage.getItem(HINT_KEY)) padHint.classList.add("faded");
function fadeHint() {
  if (padHint.classList.contains("faded")) return;
  padHint.classList.add("faded");
  try { localStorage.setItem(HINT_KEY, "1"); } catch (e) {}
}
// subtle ripple at touch coords inside #pad (transform/opacity only)
function ripple(clientX, clientY) {
  const r = pad.getBoundingClientRect();
  const el = document.createElement("span");
  el.className = "ripple";
  el.style.left = (clientX - r.left) + "px";
  el.style.top = (clientY - r.top) + "px";
  pad.appendChild(el);
  el.addEventListener("animationend", () => el.remove());
  setTimeout(() => { if (el.parentNode) el.remove(); }, 700);
}
dragToggle.addEventListener("click", () => {
  dragMode = !dragMode; dragToggle.classList.toggle("on", dragMode);
  buzz(dragMode ? 20 : 8); toast(dragMode ? "Modo arrastar: toque e mova" : "Modo arrastar desligado");
});
const g = { fingers: {}, maxFingers: 0, startT: 0, travel: 0, movePrev: null, scrollPrev: null, mode: "idle" };
function count(o) { return Object.keys(o).length; }
function mid(a, b) { return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 }; }
function startDrag() { dragging = true; pad.classList.add("dragging"); send({ type: "mouse_down", button: "left" }); buzz(20); }
function endDrag() { if (!dragging) return; dragging = false; pad.classList.remove("dragging"); send({ type: "mouse_up", button: "left" }); }
pad.addEventListener("touchstart", function (e) {
  e.preventDefault();
  fadeHint();
  for (const t of e.changedTouches) g.fingers[t.identifier] = { x: t.clientX, y: t.clientY };
  const n = count(g.fingers);
  if (g.mode === "idle") { g.startT = Date.now(); g.travel = 0; g.maxFingers = 0; }
  g.maxFingers = Math.max(g.maxFingers, n);
  if (n === 1) { g.mode = "move"; g.movePrev = { ...g.fingers[Object.keys(g.fingers)[0]] }; moveRemX = moveRemY = 0; moveT = 0; if (dragMode && !dragging) startDrag(); }
  else if (n === 2) {
    // a second finger landing while dragging: release the held left button cleanly first
    if (dragging) endDrag();
    g.mode = "scroll"; g.movePrev = null; const ids = Object.keys(g.fingers); g.scrollPrev = mid(g.fingers[ids[0]], g.fingers[ids[1]]);
  }
  else { g.mode = "idle"; }
}, { passive: false });
pad.addEventListener("touchmove", function (e) {
  e.preventDefault();
  for (const t of e.changedTouches) if (g.fingers[t.identifier]) g.fingers[t.identifier] = { x: t.clientX, y: t.clientY };
  const n = count(g.fingers);
  if (n === 1 && g.mode === "move" && g.movePrev) {
    const cur = g.fingers[Object.keys(g.fingers)[0]];
    const rdx = cur.x - g.movePrev.x, rdy = cur.y - g.movePrev.y;
    g.travel += Math.abs(rdx) + Math.abs(rdy);
    // velocity-based acceleration: slow = precise, fast flicks travel farther
    const now = Date.now();
    const dt = moveT ? Math.max(now - moveT, 1) : 16; moveT = now;
    const speed = Math.hypot(rdx, rdy) / dt; // px per ms
    const accel = 1 + Math.min(speed * ACCEL_K, ACCEL_CAP);
    const factor = settings.sensitivity * accel;
    // accumulate sub-pixel remainder so slow movement isn't lost to rounding
    const fx = rdx * factor + moveRemX, fy = rdy * factor + moveRemY;
    const dx = Math.round(fx), dy = Math.round(fy);
    moveRemX = fx - dx; moveRemY = fy - dy;
    if (dx || dy) send({ type: "mouse_move", dx: dx, dy: dy });
    g.movePrev = { x: cur.x, y: cur.y };
  } else if (n >= 2 && g.mode === "scroll" && g.scrollPrev) {
    const ids = Object.keys(g.fingers).slice(0, 2);
    const m = mid(g.fingers[ids[0]], g.fingers[ids[1]]);
    const dy = Math.round((m.y - g.scrollPrev.y) * settings.sensitivity * settings.scrollSpeed);
    g.travel += Math.abs(m.y - g.scrollPrev.y);
    // natural scroll flips the sign (content follows the fingers)
    if (dy) send({ type: "mouse_scroll", delta: settings.naturalScroll ? dy : -dy });
    g.scrollPrev = m;
  }
}, { passive: false });
pad.addEventListener("touchend", function (e) {
  e.preventDefault();
  const last = e.changedTouches[e.changedTouches.length - 1];
  for (const t of e.changedTouches) delete g.fingers[t.identifier];
  const n = count(g.fingers);
  if (n === 0) {
    if (dragging) endDrag();
    else {
      const dur = Date.now() - g.startT;
      if (g.travel <= TAP_MOVE_TOL && dur <= TAP_MS) {
        if (g.maxFingers >= 2) { buzz(); send({ type: "mouse_click", button: "right" }); }
        else if (g.maxFingers === 1) { buzz(); send({ type: "mouse_click", button: "left" }); if (last) ripple(last.clientX, last.clientY); }
      }
    }
    g.mode = "idle"; g.movePrev = null; g.scrollPrev = null;
  } else if (n === 1) { g.mode = "move"; g.movePrev = { ...g.fingers[Object.keys(g.fingers)[0]] }; g.scrollPrev = null; moveRemX = moveRemY = 0; moveT = 0; }
}, { passive: false });
pad.addEventListener("touchcancel", function () { g.fingers = {}; if (dragging) endDrag(); g.mode = "idle"; g.movePrev = null; g.scrollPrev = null; }, { passive: false });
pad.addEventListener("contextmenu", e => e.preventDefault());
