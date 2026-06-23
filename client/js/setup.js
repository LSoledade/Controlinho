// setup.js — first-run wizard, shown only on the insecure HTTP origin (the phone's
// entry point, http://<lan-ip>:8080). The plain-HTTP page can't control anything
// (no secure context, no token), so its whole job is: get the local CA trusted,
// then hand off to the HTTPS app with the pairing token baked into the URL. On the
// HTTPS app and on localhost (both already secure contexts) the wizard stays hidden.
import { buzz, getToken, isStandalone, isLocalhost, isIOS } from "./state.js";
import { toast } from "./ui.js";

export const inSetup = location.protocol === "http:" && !isLocalhost && !isStandalone;

if (inSetup) initWizard();

function initWizard() {
  const wizard = document.getElementById("setupWizard");
  const st1 = document.getElementById("st1"), st2 = document.getElementById("st2"), st3 = document.getElementById("st3");
  const caDownload = document.getElementById("caDownload");
  const checkTrust = document.getElementById("checkTrust");
  const openSecure = document.getElementById("openSecure");
  const setupSkip = document.getElementById("setupSkip");
  const st3msg = document.getElementById("st3msg");
  const trustState = document.getElementById("trustState"), trustText = trustState.querySelector("span");
  let httpsPort = ":8443";
  let secureURL = buildSecureURL(getToken()); // best-effort until /info refines it

  function buildSecureURL(tok) {
    return "https://" + location.hostname + httpsPort + "/" + (tok ? "?k=" + encodeURIComponent(tok) : "");
  }
  function setTrust(state, text) { trustState.className = "trust " + state; trustText.textContent = text; }
  function goSecure() { if (secureURL) location.href = secureURL; }

  // Trust probe: load an HTTPS image from the app origin. If the local CA is
  // trusted the TLS handshake succeeds and the image loads; if not, the browser
  // blocks it and we get an error. A timeout guards against a hung handshake.
  function probeTrust(timeoutMs) {
    return new Promise(resolve => {
      const img = new Image();
      let done = false;
      const finish = ok => { if (done) return; done = true; clearTimeout(timer); img.onload = img.onerror = null; resolve(ok); };
      const timer = setTimeout(() => finish(false), timeoutMs || 5000);
      img.onload = () => finish(true);
      img.onerror = () => finish(false);
      img.src = "https://" + location.hostname + httpsPort + "/icon-192.png?probe=" + Date.now();
    });
  }

  // Step 3 unlocked: the CA is trusted, the secure handoff is live.
  function unlockSecure() {
    st1.classList.add("done"); st1.classList.remove("active");
    st2.classList.remove("locked", "active"); st2.classList.add("done");
    st3.classList.remove("locked"); st3.classList.add("active");
    openSecure.setAttribute("href", secureURL);
    openSecure.removeAttribute("aria-disabled");
    st3msg.textContent = "Tudo pronto — abra a versão segura para usar o app.";
    setTrust("ok", "Certificado confiável");
  }

  // OS-specific install hint.
  if (isIOS) {
    document.getElementById("osName").textContent = "iPhone/iPad";
    document.getElementById("howtoAndroid").hidden = true;
    document.getElementById("howtoIOS").hidden = false;
  }

  // Step 1 → 2: tapping download advances the wizard (we can't observe the file
  // being saved, but the tap is the trigger). The <a href="/ca.crt"> still downloads.
  caDownload.addEventListener("click", () => {
    buzz(12);
    st1.classList.add("done"); st1.classList.remove("active");
    st2.classList.remove("locked"); st2.classList.add("active");
  });

  // Step 2: "Já instalei — verificar" probes whether the CA is now trusted.
  checkTrust.addEventListener("click", async () => {
    buzz(12); setTrust("checking", "Verificando…");
    checkTrust.setAttribute("aria-disabled", "true");
    const ok = await probeTrust();
    checkTrust.removeAttribute("aria-disabled");
    if (ok) unlockSecure();
    else {
      setTrust("bad", "Certificado não detectado");
      st2.classList.add("shake"); setTimeout(() => st2.classList.remove("shake"), 400);
      toast("Ainda não detectei o certificado — instale-o como Certificado CA e tente de novo");
    }
  });

  // Step 3 / "Já configurei": jump straight to the secure app with the token.
  setupSkip.addEventListener("click", () => { buzz(15); goSecure(); });
  openSecure.addEventListener("click", (e) => { e.preventDefault(); buzz(15); goSecure(); });

  // Reveal the wizard with step 1 active.
  openSecure.setAttribute("href", secureURL);
  st1.classList.add("active");
  wizard.hidden = false;

  // Pull the pairing token + the real HTTPS port from /info so the handoff link
  // authenticates across the http→https origin hop (localStorage doesn't cross
  // origins). Then opportunistically probe: a returning visitor who already trusts
  // the CA skips straight to the unlocked state without tapping "verificar".
  fetch("/info", { cache: "no-store" })
    .then(r => r.ok ? r.json() : null)
    .then(info => {
      if (info && info.httpsPort) httpsPort = info.httpsPort;
      const tok = (info && info.token) ? info.token : getToken();
      secureURL = buildSecureURL(tok);
      if (openSecure.getAttribute("aria-disabled") === "true") openSecure.setAttribute("href", secureURL);
      return probeTrust(2500);
    })
    .then(ok => { if (ok) unlockSecure(); })
    .catch(() => {});
}
