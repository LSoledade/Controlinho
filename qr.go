package main

// QR-code onboarding: turn "type this IP into your phone" into "point the camera".
//
// Two flows are exposed because first-run and steady-state differ:
//   • setup (HTTP :8080)  → install the local CA, then jump to the secure app
//   • app   (HTTPS :8443) → the installable PWA (use after the CA is trusted)
//
// The PC opens /qr in its own browser (auto, when launched with a console) and
// shows both codes with the right IP baked in. We also render an ASCII QR to the
// terminal for the headless-curious.

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// bestLANIP picks the address most likely to reach the phone on the same Wi-Fi.
// Preference: 192.168.* > 10.* > 172.* (but not Tailscale, handled separately).
func bestLANIP(ips []string) string {
	var ten, oneSeventyTwo string
	for _, ip := range ips {
		if isTailscale(ip) {
			continue
		}
		switch {
		case strings.HasPrefix(ip, "192.168."):
			return ip
		case strings.HasPrefix(ip, "10.") && ten == "":
			ten = ip
		case strings.HasPrefix(ip, "172.") && oneSeventyTwo == "":
			oneSeventyTwo = ip
		}
	}
	if ten != "" {
		return ten
	}
	if oneSeventyTwo != "" {
		return oneSeventyTwo
	}
	for _, ip := range ips {
		if !isTailscale(ip) {
			return ip
		}
	}
	return ""
}

func tailscaleIP(ips []string) string {
	for _, ip := range ips {
		if isTailscale(ip) {
			return ip
		}
	}
	return ""
}

// isTailscale matches the 100.64.0.0/10 CGNAT range Tailscale hands out.
func isTailscale(ip string) bool {
	if !strings.HasPrefix(ip, "100.") {
		return false
	}
	var a, b, c, d int
	if _, err := fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d); err != nil {
		return false
	}
	return b >= 64 && b <= 127
}

var httpPortValue string // ":8080" — set in main

// setupURL / appURL build the onboarding URLs for a given host.
func setupURL(host string) string { return "http://" + host + httpPortValue }
func appURL(host string) string   { return "https://" + host + httpsPortValue }

// handleQRPNG renders a QR PNG for ?t=setup|app&h=<host>.
func handleQRPNG(w http.ResponseWriter, r *http.Request) {
	if !allowedHost(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	host := r.URL.Query().Get("h")
	if host == "" {
		host = bestLANIP(listenIPs())
	}
	var target string
	if r.URL.Query().Get("t") == "app" {
		target = appURL(host)
	} else {
		target = setupURL(host)
	}
	png, err := qrcode.Encode(target, qrcode.Medium, 512)
	if err != nil {
		http.Error(w, "qr error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// handleQRPage shows a PC-facing page with both QR codes and the URLs.
func handleQRPage(w http.ResponseWriter, r *http.Request) {
	if !allowedHost(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ips := listenIPs()
	lan := bestLANIP(ips)
	ts := tailscaleIP(ips)

	type qrEntry struct {
		Title, Icon, Sub, Host, Setup, App string
		HasApp                             bool
	}
	data := struct {
		Entries []qrEntry
		Version string
	}{Version: version}

	if lan != "" {
		data.Entries = append(data.Entries, qrEntry{
			Title: "Mesma Wi-Fi (rede local)", Icon: "wifi",
			Sub:  lan,
			Host: lan, Setup: setupURL(lan), App: appURL(lan), HasApp: true,
		})
	}
	if ts != "" {
		data.Entries = append(data.Entries, qrEntry{
			Title: "Tailscale (fora de casa)", Icon: "lock",
			Sub:  ts,
			Host: ts, Setup: setupURL(ts), App: appURL(ts), HasApp: true,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := qrPageTmpl.Execute(w, data); err != nil {
		log.Printf("qr page: %v", err)
	}
}

var qrPageTmpl = template.Must(template.New("qr").Parse(`<!DOCTYPE html>
<html lang="pt-BR"><head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>PC Remote — Conectar o celular</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body { margin:0; background:radial-gradient(120% 60% at 50% 0%, #131826 0%, #0b0d12 55%), #0b0d12;
    color:#eef1f6; min-height:100vh;
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,system-ui,sans-serif;
    padding:36px 16px; }
  .ico { width:1.3em; height:1.3em; vertical-align:-.28em; stroke-width:2; stroke-linecap:round; stroke-linejoin:round; }
  h1 { font-size:24px; margin:0 0 4px; text-align:center; display:flex; align-items:center; justify-content:center; gap:10px; }
  h1 .ico { color:#5b9dff; width:26px; height:26px; }
  .ver { text-align:center; color:#9aa3b4; font-size:13px; margin-bottom:30px; }
  .wrap { max-width:960px; margin:0 auto; display:grid; gap:20px;
    grid-template-columns:repeat(auto-fit,minmax(280px,1fr)); }
  .card { background:#12151d; border:1px solid #262c39; border-radius:20px; padding:22px;
    box-shadow:0 6px 24px rgba(0,0,0,.35); }
  .card h2 { font-size:16px; margin:0 0 2px; display:flex; align-items:center; gap:8px; }
  .card h2 .ico { color:#5b9dff; width:20px; height:20px; }
  .card .ip { color:#9aa3b4; font-size:13px; margin:0 0 18px 28px; font-variant-numeric:tabular-nums; }
  .step { display:flex; gap:16px; align-items:center; margin:16px 0; }
  .step img { width:134px; height:134px; background:#fff; border-radius:12px; padding:7px; flex:0 0 auto; }
  .step .t { font-size:14px; line-height:1.45; }
  .step .t b { color:#5b9dff; }
  .step .url { font-size:12px; color:#5f6776; word-break:break-all; margin-top:6px; font-variant-numeric:tabular-nums; }
  .badge { display:inline-flex; align-items:center; gap:6px; font-size:11px; font-weight:700; letter-spacing:.04em;
    padding:3px 9px; border-radius:999px; margin-bottom:7px; }
  .badge .ico { width:13px; height:13px; }
  .b1 { background:rgba(255,194,75,.14); color:#ffc24b; }
  .b2 { background:rgba(68,208,123,.14); color:#44d07b; }
  .note { max-width:960px; margin:30px auto 0; color:#9aa3b4; font-size:13px; line-height:1.6; text-align:center; }
  .note b { color:#eef1f6; }
</style></head><body>
<svg width="0" height="0" style="position:absolute" aria-hidden="true"><defs>
  <symbol id="i-monitor" viewBox="0 0 24 24"><rect x="2" y="3" width="20" height="14" rx="2.5" fill="none" stroke="currentColor"/><path fill="none" stroke="currentColor" d="M8 21h8M12 17v4"/></symbol>
  <symbol id="i-wifi" viewBox="0 0 24 24"><path fill="none" stroke="currentColor" d="M5 12.5a10 10 0 0 1 14 0M8.5 16a5 5 0 0 1 7 0"/><circle cx="12" cy="19.5" r="1" fill="currentColor" stroke="none"/></symbol>
  <symbol id="i-lock" viewBox="0 0 24 24"><rect x="3" y="11" width="18" height="11" rx="2.5" fill="none" stroke="currentColor"/><path fill="none" stroke="currentColor" d="M7 11V7a5 5 0 0 1 10 0v4"/></symbol>
  <symbol id="i-download" viewBox="0 0 24 24"><path fill="none" stroke="currentColor" d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/></symbol>
  <symbol id="i-check" viewBox="0 0 24 24"><path fill="none" stroke="currentColor" d="M20 6L9 17l-5-5"/></symbol>
</defs></svg>
<h1><svg class="ico"><use href="#i-monitor"/></svg> Conectar o celular</h1>
<div class="ver">PC Remote {{.Version}} — aponte a câmera para um QR code</div>
<div class="wrap">
{{range .Entries}}
  <div class="card">
    <h2><svg class="ico"><use href="#i-{{.Icon}}"/></svg> {{.Title}}</h2>
    <div class="ip">{{.Sub}}</div>
    <div class="step">
      <img src="/qr.png?t=setup&h={{.Host}}" alt="QR setup">
      <div class="t">
        <span class="badge b1"><svg class="ico"><use href="#i-download"/></svg>1 · PRIMEIRA VEZ</span><br>
        Instalar o <b>certificado</b> e o app.
        <div class="url">{{.Setup}}</div>
      </div>
    </div>
    {{if .HasApp}}
    <div class="step">
      <img src="/qr.png?t=app&h={{.Host}}" alt="QR app">
      <div class="t">
        <span class="badge b2"><svg class="ico"><use href="#i-check"/></svg>2 · JÁ INSTALEI</span><br>
        Abrir direto o <b>app seguro</b>.
        <div class="url">{{.App}}</div>
      </div>
    </div>
    {{end}}
  </div>
{{end}}
</div>
<div class="note">
  Na <b>primeira vez</b>, escaneie o QR de cima: o celular abre a página de setup
  para baixar e instalar o certificado (uma vez). Depois disso, use o QR de baixo
  para abrir o app direto. Mantenha o PC e o celular na mesma rede (ou Tailscale).
</div>
</body></html>`))

// printConsoleQR draws an ASCII QR of the setup URL to the terminal and tells
// the user what to do. Only meaningful when a console is attached.
func printConsoleQR(host string) {
	if host == "" {
		return
	}
	q, err := qrcode.New(setupURL(host), qrcode.Medium)
	if err != nil {
		return
	}
	fmt.Println()
	fmt.Println("  Escaneie com o celular (primeira vez — instala o certificado):")
	fmt.Println("  " + setupURL(host))
	fmt.Println()
	fmt.Print(q.ToSmallString(false))
	fmt.Println()
}

// openBrowser opens a URL in the PC's default browser (best effort).
func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	c.SysProcAttr = hiddenProcAttr()
	_ = c.Start()
}
