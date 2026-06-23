// pwa.js — installability: the Chrome install CTA (beforeinstallprompt), the iOS
// "Add to Home Screen" hint, and service-worker registration.
import { buzz, isStandalone, isLocalhost, isIOS } from "./state.js";
import { toast } from "./ui.js";

const IOS_KEY = "pcremote.iosHint";

// ---- install: real PWA CTA via beforeinstallprompt ----
const installBtn = document.getElementById("installBtn");
let deferredPrompt = null;
window.addEventListener("beforeinstallprompt", (e) => {
  e.preventDefault(); deferredPrompt = e;
  if (!isStandalone) installBtn.classList.add("show");
});
installBtn.addEventListener("click", async () => {
  buzz(15);
  if (!deferredPrompt) { toast("Use o menu do Chrome → Instalar app"); return; }
  deferredPrompt.prompt();
  try { await deferredPrompt.userChoice; } catch (e) {}
  deferredPrompt = null; installBtn.classList.remove("show");
});
window.addEventListener("appinstalled", () => { installBtn.classList.remove("show"); toast("App instalado!"); });

// ---- iOS install hint (Safari has no beforeinstallprompt) ----
// Only on the secure (https/localhost) origin: on the insecure HTTP setup page the
// wizard already guides the user, so we don't stack two banners.
const iosBanner = document.getElementById("iosBanner");
document.getElementById("iosClose").addEventListener("click", () => {
  iosBanner.classList.remove("show");
  try { localStorage.setItem(IOS_KEY, "1"); } catch (e) {}
});
if (isIOS && !isStandalone && (location.protocol === "https:" || isLocalhost) && !localStorage.getItem(IOS_KEY)) {
  iosBanner.classList.add("show");
}

// ---- service worker ----
if ("serviceWorker" in navigator && location.protocol === "https:") {
  navigator.serviceWorker.register("sw.js").catch(err => console.warn("SW registration failed:", err));
}
