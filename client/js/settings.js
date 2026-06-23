// settings.js — the Ajustes panel: host/PIN, sensitivity/scroll sliders, the
// toggles, and persistence. Writes the live `settings` object and reconnects.
import { settings, buzz, getHost, setHost, getToken, setToken, normalizeHost, KEYS } from "./state.js";
import { openModal, closeModal, toast } from "./ui.js";
import { connect } from "./connection.js";

const { SENS_KEY, SCROLL_KEY, NATURAL_KEY, ENTER_KEY, HAPTICS_KEY } = KEYS;

const ipBtn = document.getElementById("ipBtn"), ipInput = document.getElementById("ipInput");
const pinInput = document.getElementById("pinInput");
const sensInput = document.getElementById("sensInput"), sensVal = document.getElementById("sensVal");
const scrollInput = document.getElementById("scrollInput"), scrollVal = document.getElementById("scrollVal");
const naturalToggle = document.getElementById("naturalToggle"), enterToggle = document.getElementById("enterToggle");
const hapticsToggle = document.getElementById("hapticsToggle"), hostLabel = document.getElementById("hostLabel");
function showSens() { sensVal.textContent = parseFloat(sensInput.value).toFixed(1) + "×"; }
function showScroll() { scrollVal.textContent = parseFloat(scrollInput.value).toFixed(1) + "×"; }
function updateHostLabel() { hostLabel.textContent = normalizeHost(getHost()); }
sensInput.value = String(settings.sensitivity); showSens();
scrollInput.value = String(settings.scrollSpeed); showScroll();
naturalToggle.classList.toggle("on", settings.naturalScroll);
enterToggle.classList.toggle("on", settings.enterAfterText);
hapticsToggle.classList.toggle("on", settings.haptics);
updateHostLabel();
sensInput.addEventListener("input", showSens);
scrollInput.addEventListener("input", showScroll);
naturalToggle.addEventListener("click", () => { settings.naturalScroll = !settings.naturalScroll; naturalToggle.classList.toggle("on", settings.naturalScroll); buzz(8); });
enterToggle.addEventListener("click", () => { settings.enterAfterText = !settings.enterAfterText; enterToggle.classList.toggle("on", settings.enterAfterText); buzz(8); });
hapticsToggle.addEventListener("click", () => { settings.haptics = !settings.haptics; hapticsToggle.classList.toggle("on", settings.haptics); buzz(8); });

// Open the Ajustes modal with current values prefilled. Exported so boot can nudge
// the user into it on a first file:// open with no saved host.
export function openSettings() {
  ipInput.value = getHost(); pinInput.value = getToken(); sensInput.value = String(settings.sensitivity); showSens();
  scrollInput.value = String(settings.scrollSpeed); showScroll();
  naturalToggle.classList.toggle("on", settings.naturalScroll);
  enterToggle.classList.toggle("on", settings.enterAfterText);
  hapticsToggle.classList.toggle("on", settings.haptics);
  openModal("ipModal"); setTimeout(() => ipInput.focus(), 50);
}
ipBtn.addEventListener("click", openSettings);
document.getElementById("ipCancel").addEventListener("click", () => closeModal("ipModal"));
function saveSettings() {
  settings.sensitivity = parseFloat(sensInput.value) || 1.5; localStorage.setItem(SENS_KEY, String(settings.sensitivity));
  settings.scrollSpeed = parseFloat(scrollInput.value) || 1; localStorage.setItem(SCROLL_KEY, String(settings.scrollSpeed));
  localStorage.setItem(NATURAL_KEY, settings.naturalScroll ? "1" : "0");
  localStorage.setItem(ENTER_KEY, settings.enterAfterText ? "1" : "0");
  localStorage.setItem(HAPTICS_KEY, settings.haptics ? "1" : "0");
  const h = normalizeHost(ipInput.value);
  if (!h) { toast("Informe um endereço válido"); return; }
  setHost(h);
  setToken((pinInput.value || "").trim()); // server normalizes case/dashes
  closeModal("ipModal"); updateHostLabel();
  // connect() tears down the previous socket (generation guard prevents zombie reconnects)
  connect();
}
document.getElementById("ipSave").addEventListener("click", saveSettings);
ipInput.addEventListener("keydown", e => { if (e.key === "Enter") saveSettings(); });
