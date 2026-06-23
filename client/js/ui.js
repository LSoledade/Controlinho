// ui.js — view helpers shared across panels: toast, modals, and the bottom tab bar.
import { buzz } from "./state.js";

const TAB_KEY = "pcremote.tab";
const TABS = ["trackpad", "media", "keyboard", "power"];

const toastEl = document.getElementById("toast");
const toastMsg = document.getElementById("toastMsg");
let toastTimer = null;
export function toast(msg, ms) {
  toastMsg.textContent = msg; toastEl.classList.add("show");
  clearTimeout(toastTimer); toastTimer = setTimeout(() => toastEl.classList.remove("show"), ms || 1600);
}
export function openModal(id) { document.getElementById(id).classList.add("show"); }
export function closeModal(id) { document.getElementById(id).classList.remove("show"); }

const tabButtons = document.querySelectorAll(".tabbar button");
const tabs = document.querySelectorAll(".tab");
export function gotoTab(name) {
  if (!TABS.includes(name)) name = "trackpad";
  tabs.forEach(t => t.classList.toggle("active", t.dataset.tab === name));
  tabButtons.forEach(b => b.classList.toggle("active", b.dataset.goto === name));
  try { localStorage.setItem(TAB_KEY, name); } catch (e) {}
}
tabButtons.forEach(b => b.addEventListener("click", () => { buzz(8); gotoTab(b.dataset.goto); }));
const urlTab = new URLSearchParams(location.search).get("t");
gotoTab(urlTab || localStorage.getItem(TAB_KEY) || "trackpad");
