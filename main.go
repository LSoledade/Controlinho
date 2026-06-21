package main

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed all:client
var clientFS embed.FS

const version = "2.1.0"

// --- Command payload ---

// cmd is the JSON envelope exchanged over the WebSocket.
// Example: {"type":"mouse_move","dx":3,"dy":-2}
//
//	{"type":"shortcut","keys":["ctrl","w"]}
//	{"type":"type","text":"hello"}
type cmd struct {
	Type   string   `json:"type"`
	DX     int      `json:"dx,omitempty"`
	DY     int      `json:"dy,omitempty"`
	Button string   `json:"button,omitempty"`
	Delta  int      `json:"delta,omitempty"`
	Key    string   `json:"key,omitempty"`
	Keys   []string `json:"keys,omitempty"`
	Action string   `json:"action,omitempty"`
	Text   string   `json:"text,omitempty"`
	ID     int      `json:"id,omitempty"`
}

// ack is the optional reply for action commands.
type ack struct {
	Type  string `json:"type"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	ID    int    `json:"id,omitempty"`
}

// --- IP allowlist ---

// allowedHost reports whether a remote address is on the local network or Tailscale.
// Allowed: loopback, link-local, private (10/172.16/192.168), and Tailscale's CGNAT (100.64/10).
func allowedHost(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr // best effort
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return true
	}
	if ip.IsInterfaceLocalMulticast() || ip.IsLinkLocalMulticast() {
		return true
	}
	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10", // Tailscale CGNAT range
		"fd00::/8",      // IPv6 ULA
		"fc00::/7",
	} {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// --- HTTP handlers ---

var upgrader = websocket.Upgrader{
	// Defense in depth on top of the IP allowlist: only accept WebSocket
	// upgrades whose Origin matches the host being requested. This blocks a
	// malicious web page (loaded on a phone that *is* on the LAN, or via DNS
	// rebinding) from opening a socket to us — its Origin would be the
	// attacker's domain, not ours. Non-browser clients send no Origin and pass.
	CheckOrigin: checkOrigin,
}

func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // curl, native tools, etc.
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// clientRoot is the embedded client subtree, resolved once at startup.
var clientRoot http.FileSystem
var fileServer http.Handler

func init() {
	sub, err := fs.Sub(clientFS, "client")
	if err != nil {
		panic("client embed missing: " + err.Error())
	}
	clientRoot = http.FS(sub)
	fileServer = http.FileServer(clientRoot)
}

// handleClient serves the embedded PWA. "/" and any path that doesn't map to a
// real embedded file both return index.html (single-file app), so deep links and
// unknown assets fall back to the UI instead of 404.
func handleClient(w http.ResponseWriter, r *http.Request) {
	if !allowedHost(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		serveIndex(w, r)
		return
	}
	if f, err := clientRoot.Open("/" + name); err == nil {
		_ = f.Close()
		fileServer.ServeHTTP(w, r)
		return
	}
	serveIndex(w, r)
}

// serveTyped serves an embedded client file with an explicit Content-Type.
// noCache=true is for the service worker, which must always be revalidated.
func serveTyped(name, ctype string, noCache bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		data, err := clientFS.ReadFile("client/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ctype)
		if noCache {
			w.Header().Set("Cache-Control", "no-cache")
		}
		_, _ = w.Write(data)
	}
}

// serveIndex writes the single-page index.html directly from the embed.
func serveIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := clientFS.ReadFile("client/index.html")
	if err != nil {
		http.Error(w, "client not available", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// runtime config shared with the client (so the UI can build the HTTPS link and
// know whether the CA still needs installing).
var httpsPortValue string

// handleInfo returns small runtime facts the PWA uses to bootstrap install.
func handleInfo(w http.ResponseWriter, r *http.Request) {
	if !allowedHost(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"version":   version,
		"httpsPort": httpsPortValue,
		"secure":    r.TLS != nil,
	})
}

// caCertPEM is filled at startup with the local CA so phones can download+trust it.
var caCertPEM []byte

// handleCA serves the local CA so the phone can install it as a trusted root.
// The CA cert is public (no private key) — installing it is what unlocks the
// trusted-HTTPS context Android Chrome needs for service workers / PWA install.
func handleCA(w http.ResponseWriter, r *http.Request) {
	if !allowedHost(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="pc-remote-ca.crt"`)
	_, _ = w.Write(caCertPEM)
}

// handleWS upgrades to a WebSocket and processes commands.
func handleWS(w http.ResponseWriter, r *http.Request) {
	if !allowedHost(r.RemoteAddr) {
		log.Printf("ws: rejected connection from %s (not on local/Tailscale network)", r.RemoteAddr)
		http.Error(w, "forbidden: host not allowed", http.StatusForbidden)
		return
	}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: upgrade failed: %v", err)
		return
	}
	defer c.Close()

	remote := r.RemoteAddr
	log.Printf("ws: client connected (%s)", remote)
	defer log.Printf("ws: client disconnected (%s)", remote)

	c.SetReadLimit(64 * 1024)
	_ = c.SetReadDeadline(time.Now().Add(120 * time.Second))
	c.SetPongHandler(func(string) error {
		_ = c.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	// A single writer goroutine isn't needed: gorilla forbids concurrent writes,
	// and we only write from this read loop plus the ping ticker. Guard the ping.
	var writeMu sync.Mutex
	pingDone := make(chan struct{})
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				writeMu.Lock()
				_ = c.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
				writeMu.Unlock()
			case <-pingDone:
				return
			}
		}
	}()
	defer close(pingDone)

	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("ws: read error: %v", err)
			}
			return
		}
		dispatch(c, &writeMu, data)
	}
}

// dispatch decodes one frame (object or array) and runs each command.
func dispatch(c *websocket.Conn, wmu *sync.Mutex, data []byte) {
	var many []cmd
	if err := json.Unmarshal(data, &many); err == nil && many != nil {
		for i := range many {
			runCmd(c, wmu, &many[i])
		}
		return
	}
	var one cmd
	if err := json.Unmarshal(data, &one); err != nil {
		log.Printf("ws: bad message %q: %v", truncate(string(data), 120), err)
		return
	}
	runCmd(c, wmu, &one)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// runCmd executes a single command. Discrete actions reply with an ack; high-rate
// movement events (mouse_move) are fire-and-forget to keep latency low.
func runCmd(c *websocket.Conn, wmu *sync.Mutex, m *cmd) {
	reply := func(ok bool, msg string) {
		wmu.Lock()
		_ = c.WriteJSON(ack{Type: m.Type, OK: ok, Error: msg, ID: m.ID})
		wmu.Unlock()
	}

	switch m.Type {
	case "ping":
		reply(true, "")
		return

	case "mouse_move":
		mouseMoveRelative(m.DX, m.DY)
		return // no ack — too chatty

	case "mouse_move_abs":
		mouseMoveAbsolute(m.DX, m.DY)
		return

	case "mouse_click":
		mouseClick(m.Button)
		return

	case "mouse_down":
		mouseButton(m.Button, true)
		return

	case "mouse_up":
		mouseButton(m.Button, false)
		return

	case "mouse_scroll":
		mouseScroll(m.Delta)
		return

	case "key":
		vk, ok := vkFor(m.Key)
		if !ok {
			reply(false, "unknown key: "+m.Key)
			return
		}
		pressKey(vk)
		return

	case "shortcut":
		vks := make([]uint16, 0, len(m.Keys))
		for _, k := range m.Keys {
			vk, ok := vkFor(k)
			if !ok {
				reply(false, "unknown key in shortcut: "+k)
				return
			}
			vks = append(vks, vk)
		}
		tapChord(vks)
		return

	case "type":
		typeText(m.Text)
		return

	case "volume":
		if !volumeAction(m.Action) {
			reply(false, "bad volume action: "+m.Action)
		}
		return

	case "media":
		if !mediaAction(m.Action) {
			reply(false, "bad media action: "+m.Action)
		}
		return

	case "power":
		if err := powerAction(m.Action); err != nil {
			reply(false, err.Error())
			return
		}
		reply(true, "")
		return

	default:
		reply(false, "unknown command type: "+m.Type)
	}
}

// --- High-level action dispatch ---

func volumeAction(action string) bool {
	switch strings.ToLower(action) {
	case "up":
		pressKey(0xAF)
	case "down":
		pressKey(0xAE)
	case "mute":
		pressKey(0xAD)
	default:
		return false
	}
	return true
}

func mediaAction(action string) bool {
	var vk uint16
	switch strings.ToLower(action) {
	case "play_pause", "play", "pause":
		vk = 0xB3
	case "next":
		vk = 0xB0
	case "prev", "previous":
		vk = 0xB1
	default:
		return false
	}
	pressKey(vk)
	return true
}

// powerAction performs OS power actions. monitor_off works on Windows directly;
// shutdown/restart/sleep shell out to the OS.
func powerAction(action string) error {
	switch strings.ToLower(action) {
	case "monitor_off":
		monitorOff()
		return nil
	case "shutdown":
		return runPower("shutdown", "/s", "/t", "0")
	case "restart", "reboot":
		return runPower("shutdown", "/r", "/t", "0")
	case "sleep":
		return runPower("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0")
	default:
		return fmt.Errorf("bad power action: %s", action)
	}
}

// runPower spawns a power command (non-blocking via Start, so we can ack first).
// Non-Windows hosts just log and skip so the server stays buildable for dev.
func runPower(name string, args ...string) error {
	if runtime.GOOS != "windows" {
		log.Printf("power: %s %v (skipped on %s)", name, args, runtime.GOOS)
		return nil
	}
	c := exec.Command(name, args...)
	c.SysProcAttr = hiddenProcAttr()
	if err := c.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	return nil
}

// probeHost returns the host to reach a locally-bound listener: the bound host,
// or loopback when the bind address is a wildcard (0.0.0.0 / :: / empty). This
// lets the single-instance probe and the re-launch browser-open work even when
// an instance is pinned to a specific interface, not just the default 0.0.0.0.
func probeHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

// alreadyRunning probes the /info endpoint of the address we're about to bind to
// detect a pc-remote instance that already holds it. It returns true only when
// /info answers 200 with our JSON shape (a "version" field), so an unrelated
// server on the same port won't be mistaken for us.
func alreadyRunning(httpAddr string) bool {
	port := portOf(httpAddr)
	if port == "" {
		return false
	}
	client := &http.Client{Timeout: 400 * time.Millisecond}
	resp, err := client.Get("http://" + probeHost(httpAddr) + port + "/info")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var info struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false
	}
	return info.Version != ""
}

// --- server entrypoint ---

func main() {
	httpAddr := flag.String("http", "0.0.0.0:8080", "HTTP listen address (bootstrap + CA download)")
	httpsAddr := flag.String("https", "0.0.0.0:8443", "HTTPS listen address (PWA install + wss)")
	openUI := flag.Bool("open", true, "open the QR/connect page in the browser on startup (when run with a console)")
	showVer := flag.Bool("version", false, "print version and exit")
	doInstall := flag.Bool("install", false, "register auto-start at logon + open the firewall, then start")
	doUninstall := flag.Bool("uninstall", false, "remove the auto-start task and firewall rule")
	setupFW := flag.Bool("setupfw", false, "(internal) add the firewall rule then exit — used for UAC elevation")
	flag.Parse()

	if *showVer {
		fmt.Println("pc-remote", version)
		return
	}

	// Internal: the elevated child spawned by -install adds the firewall rule and exits.
	if *setupFW {
		if err := addFirewallRule(); err != nil {
			log.Fatalf("firewall: %v", err)
		}
		return
	}

	if *doInstall {
		if err := installSelf(true); err != nil {
			log.Fatalf("install: %v", err)
		}
		log.Printf("pronto. No celular: abra http://SEU-IP:8080 e siga o passo a passo (ou veja a bandeja do sistema).")
		return
	}
	if *doUninstall {
		if err := uninstallSelf(); err != nil {
			log.Fatalf("uninstall: %v", err)
		}
		return
	}

	httpPortValue = portOf(*httpAddr)
	httpsPortValue = portOf(*httpsAddr)

	// If an instance is already listening, treat a re-launch (e.g. the user
	// double-clicked the exe again) as "show me the connect page" rather than
	// crashing on a port conflict. Open the browser unconditionally here: this
	// path is only hit on a manual re-launch (the Task-Scheduler logon launch
	// runs when nothing is up yet), so the GUI/no-console double-click — the
	// whole point — still gets its connect page.
	if alreadyRunning(*httpAddr) {
		log.Printf("pc-remote já está em execução — abrindo a página de conexão")
		openBrowser("http://" + probeHost(*httpAddr) + portOf(*httpAddr) + "/qr")
		return
	}

	// Set up the dynamic certificate manager. It owns the persistent CA (with
	// migration from the legacy location) and mints leaves on demand, re-minting
	// when the machine's IPs change or the leaf nears expiry — no restart needed.
	mgr, err := newCertManager(dataDir())
	if err != nil {
		log.Fatalf("tls: %v", err)
	}
	caCertPEM = mgr.caPEM()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWS)
	mux.HandleFunc("/info", handleInfo)
	mux.HandleFunc("/ca.crt", handleCA)
	mux.HandleFunc("/qr", handleQRPage)
	mux.HandleFunc("/qr.png", handleQRPNG)
	// Explicit content types: Chrome is picky about the manifest, and the SW
	// must never be cached or installability silently breaks.
	mux.HandleFunc("/manifest.webmanifest", serveTyped("manifest.webmanifest", "application/manifest+json", false))
	mux.HandleFunc("/sw.js", serveTyped("sw.js", "text/javascript; charset=utf-8", true))
	mux.HandleFunc("/", handleClient)

	httpSrv := &http.Server{
		Addr:              *httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	httpsSrv := &http.Server{
		Addr:              *httpsAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         &tls.Config{GetCertificate: mgr.GetCertificate, MinVersion: tls.VersionTLS12},
	}

	// Bind both listeners up front so a port conflict fails fast with an
	// actionable message instead of crashing inside a goroutine after startup.
	httpLn, err := net.Listen("tcp", *httpAddr)
	if err != nil {
		log.Fatalf("porta %s em uso — outra instância do pc-remote? feche-a, ou rode com -http em outra porta: %v", *httpAddr, err)
	}
	httpsLn, err := net.Listen("tcp", *httpsAddr)
	if err != nil {
		_ = httpLn.Close()
		log.Fatalf("porta %s em uso — outra instância do pc-remote? feche-a, ou rode com -https em outra porta: %v", *httpsAddr, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		ips := listenIPs()
		log.Printf("pc-remote %s", version)
		log.Printf("HTTP  (setup):  http://127.0.0.1%s", portOf(*httpAddr))
		log.Printf("HTTPS (app):    https://127.0.0.1%s", portOf(*httpsAddr))
		for _, ip := range ips {
			log.Printf("  phone setup:  http://%s%s   →  install the CA, then open the HTTPS link", ip, portOf(*httpAddr))
			log.Printf("  phone app:    https://%s%s", ip, portOf(*httpsAddr))
		}
		log.Printf("allowed networks: local (10/172.16/192.168) and Tailscale (100.64/10)")
		log.Printf("connect page (QR): http://127.0.0.1%s/qr", portOf(*httpAddr))
		if err := httpSrv.Serve(httpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http listen: %v", err)
		}
	}()

	// When launched with a console (manual run), show a QR in the terminal and
	// pop the connect page so the phone can be paired by camera. A hidden
	// Task-Scheduler launch has no console, so nothing intrusive happens there.
	if hasConsole() {
		printConsoleQR(bestLANIP(listenIPs()))
		if *openUI {
			openBrowser("http://127.0.0.1" + portOf(*httpAddr) + "/qr")
		}
	}

	go func() {
		if err := httpsSrv.ServeTLS(httpsLn, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("https listen: %v", err)
		}
	}()

	// Block on the platform UI: the Windows system tray (until "Sair" or a
	// termination signal) or, on other OSes, simply the context. Either way, run
	// a graceful shutdown of both servers on the way out.
	runUI(ctx, *httpAddr, func() {
		log.Printf("shutting down…")
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
		_ = httpsSrv.Shutdown(shutCtx)
	})
}

// certSubjects returns the IPs and DNS names the leaf certificate should cover.
func certSubjects() ([]net.IP, []string) {
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	for _, s := range listenIPs() {
		if ip := net.ParseIP(s); ip != nil {
			ips = append(ips, ip)
		}
	}
	host := hostnameOr("pc-remote")
	names := []string{"localhost", host, host + ".local"}
	return ips, names
}

// listenIPs returns the non-loopback IPv4 addresses we're listening on,
// to surface in startup logs and to put in the certificate SANs.
func listenIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				out = append(out, ip4.String())
			}
		}
	}
	return out
}

// portOf returns ":port" from a "host:port" address.
func portOf(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	if _, err := strconv.Atoi(port); err != nil {
		return ""
	}
	return ":" + port
}
