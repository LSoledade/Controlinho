// controls.js — the discrete control buttons: mouse buttons, key/shortcut/media/
// volume buttons, the text-to-type field, and the power confirmation modal.
import { settings, buzz } from "./state.js";
import { toast, openModal, closeModal } from "./ui.js";
import { send } from "./connection.js";

// ---- command buttons ----
document.querySelectorAll("[data-click]").forEach(b => b.addEventListener("click", () => { buzz(); send({ type: "mouse_click", button: b.dataset.click }); }));
document.querySelectorAll("[data-key]").forEach(b => b.addEventListener("click", () => { buzz(); send({ type: "key", key: b.dataset.key }); }));
document.querySelectorAll("[data-shortcut]").forEach(b => b.addEventListener("click", () => { buzz(); try { send({ type: "shortcut", keys: JSON.parse(b.dataset.shortcut) }); } catch (e) {} }));
document.querySelectorAll("[data-media]").forEach(b => b.addEventListener("click", () => { buzz(); send({ type: "media", action: b.dataset.media }); }));
document.querySelectorAll("[data-volume]").forEach(b => b.addEventListener("click", () => { buzz(); send({ type: "volume", action: b.dataset.volume }); }));

// ---- text input ----
const typeInput = document.getElementById("typeInput");
function sendType() {
  const t = typeInput.value; if (!t) return;
  send({ type: "type", text: t });
  if (settings.enterAfterText) send({ type: "key", key: "enter" });
  typeInput.value = ""; buzz(); toast("Texto enviado");
}
document.getElementById("typeSend").addEventListener("click", sendType);
typeInput.addEventListener("keydown", e => { if (e.key === "Enter") sendType(); });

// ---- power confirm ----
let pendingPower = null;
const confirmText = document.getElementById("confirmText");
document.querySelectorAll("[data-power-confirm]").forEach(b => {
  b.addEventListener("click", () => {
    buzz(); pendingPower = { action: b.dataset.powerConfirm, label: b.dataset.powerLabel };
    const soft = b.dataset.soft === "1";
    confirmText.textContent = pendingPower.label + (soft ? "?" : "? Esta ação não pode ser desfeita.");
    openModal("confirmModal");
  });
});
document.getElementById("confirmCancel").addEventListener("click", () => { pendingPower = null; closeModal("confirmModal"); });
document.getElementById("confirmOk").addEventListener("click", () => {
  if (pendingPower) send({ type: "power", action: pendingPower.action });
  pendingPower = null; closeModal("confirmModal"); buzz(20); toast("Comando enviado");
});
