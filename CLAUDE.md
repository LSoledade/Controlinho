# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`pc-remote` (v2.2.0) — a single Go binary that turns a phone into a trackpad/keyboard/media
remote for a Windows PC over the local network or Tailscale. The same binary runs the server
**and** serves an installable PWA (embedded via `//go:embed all:client`). No cloud, no external
dependencies, no separate installer. Server-side comments and user-facing strings are in
Portuguese (pt-BR); keep that convention.

## Commands

```bash
# Release build (Windows/amd64, with console)
go build -ldflags "-s -w" -o pc-remote.exe .

# Silent build — no console window, for hidden Task-Scheduler logon launch
go build -ldflags "-s -w -H windowsgui" -o pc-remote.exe .

# MSIX (Microsoft Store) build — same source, `store` build tag. See packaging/.
go build -tags store -ldflags "-s -w -H windowsgui" -o pkg/pc-remote.exe .

# Tests
go test ./...                  # all
go test -run TestAllowedHost . # a single test (tests live in package main at the root)
go vet -tags store ./...        # vet the MSIX variant too (default vet skips tagged files)

# Regenerate the PWA/tray icons + MSIX Store assets (file is build-tagged `//go:build ignore`)
go run gen_icons.go
```

No GCC/CGO is required — see the input-layer note below. The project also builds and vets on
Linux/macOS for development (`CGO_ENABLED=0`), where platform stubs make input/install/tray no-ops.

Run flags: `-http` (`0.0.0.0:8080`), `-https` (`0.0.0.0:8443`), `-install` / `-uninstall`
(auto-start + firewall), `-version`, `-open` (open QR page on startup). `-setupfw` is internal
(the UAC-elevated child that adds only the firewall rule). The install flags exist **only in the
desktop build**; the `store` build has none (the Microsoft Store installs/uninstalls).

## Architecture

**One mux, two listeners.** `main.go` serves the same `http.ServeMux` on HTTP `:8080`
(bootstrap: CA download, QR pages, `/info`) and HTTPS `:8443` (the real PWA + `wss://`). Both
listeners are bound up front so a port conflict fails fast. HTTPS gets its cert from
`certManager.GetCertificate` (see TLS below), not a static file.

**Command flow.** Phone ↔ PC is a WebSocket of JSON frames (single `cmd` object or an array for
batch). `handleWS` → `dispatch` (decodes object-or-array) → `runCmd` (one big `switch` on
`cmd.Type`) → high-level action helpers (`mouseClick`, `pressKey`, `volumeAction`, …) → the
platform input layer. High-rate `mouse_move` events are fire-and-forget (no ack) to keep latency
low; discrete actions reply with an `ack`. The protocol table is in README.md.

**Platform split via build tags — the central pattern.** Every OS-specific capability has a real
`*_windows.go` implementation and a non-Windows stub (`//go:build !windows`) so the dev build keeps
compiling:

| Windows (real)        | Non-Windows (stub) | Concern                              |
|-----------------------|--------------------|--------------------------------------|
| `input_windows.go`    | `input_stub.go`    | mouse/keyboard/text/monitor input    |
| `install_windows.go`  | `install_other.go` | auto-start (Task Scheduler) + firewall |
| `tray_windows.go`     | `tray_other.go`    | system-tray UI / main-loop blocking  |

When you add a platform-specific function, you must add it to **both** sides or the cross-platform
dev build breaks. Functions like `hiddenProcAttr`, `hasConsole`, `vkFor`, `runUI`, `installSelf`
exist on both sides for this reason.

**Second axis: the `store` build tag (dual distribution).** The same source builds two Windows
targets — the standalone `.exe` (default) and the MSIX/Microsoft Store package (`-tags store`). The
Store forbids `netsh` firewall, UAC self-elevation, and `schtasks` auto-start, so those live only in
the **desktop** build and the store build swaps in Store-native equivalents:

| `windows && !store` (desktop) | `windows && store` (MSIX)        | Concern                         |
|-------------------------------|----------------------------------|---------------------------------|
| `install_windows.go`          | `install_store.go`               | auto-start + (desktop) firewall/UAC |
| `installcli_desktop.go`       | `installcli_store.go`            | `-install`/`-uninstall`/`-setupfw` flags |

Both sides define `autoStartEnabled()` / `setAutoStart(bool)` (the tray's "Iniciar com o Windows"
toggle) and `registerInstallFlags()` / `runInstallFlags()` (called from `main()`), so `tray_windows.go`
and `main.go` are identical across builds. The store build's auto-start is the WinRT `StartupTask`
API (driven from PowerShell — no CGO, no new dep), declared in `packaging/Package.appxmanifest`. It
**only works when run from the installed MSIX** (needs package identity) — verify on-device.

**Input without CGO (deliberate).** `input_windows.go` calls the Win32 API directly via `syscall`
(`mouse_event`, `keybd_event`, `MapVirtualKeyW`, `SendMessageW`) instead of `robotgo`/`volume-go`,
which would require CGO + MinGW and break the "single binary, zero install" goal. Extended keys
(arrows, nav cluster, media, right-modifiers) must carry `KEYEVENTF_EXTENDEDKEY` — that's what the
`extendedVK` map is for; without it they'd be read as numpad keys under Num Lock. Unicode text uses
the `KEYEVENTF_UNICODE` path with UTF-16 surrogate-pair handling.

**TLS / trusted HTTPS (`tlsx.go`).** Android Chrome only registers a service worker and offers
"Install app" over a *trusted* HTTPS origin, and the `localhost` exception doesn't cover a LAN IP.
So we do what `mkcert` does: generate a local CA once (in `os.UserCacheDir()` →
`%LocalAppData%\pc-remote`, owner-only perms) and have `certManager` mint leaf certs on demand,
signed by it. `GetCertificate` re-mints automatically when the machine's IP set changes (new
Wi-Fi/DHCP) or the leaf nears expiry — HTTPS self-heals without a restart. The CA is auto-migrated
from the legacy location (next to the `.exe`) so existing phones don't re-install it. **Never commit
`pc-remote-ca.key`** (it's git-ignored) — anyone with it can mint trusted certs for your phone.

**Security model — every handler enforces it.** Three layers. The first two apply on *every* HTTP
handler and the WS upgrade: (1) `allowedHost(r.RemoteAddr)` rejects anything not on loopback/private
(10/172.16/192.168) / Tailscale (100.64/10) / IPv6 ULA with `403`; (2) `checkOrigin` blocks WS
upgrades whose `Origin` doesn't match the requested host (anti DNS-rebinding/CSRF). Trust is still
network-topology based, so the ports must never be exposed to the internet. **Any new HTTP handler
must start with an `allowedHost` check** — follow the existing handlers.

(3) **Pairing token (`token.go`).** A persisted secret (`pc-remote-token` in `dataDir()`, owner-only,
never committed) required on every WS upgrade via `?k=` or the `pcr_k` cookie — `validToken` is
constant-time. It's embedded in the green "app" QR (`appURL`) and in `/info` so the http→https
handoff carries it across origins; `/qr` shows it as a PIN for manual entry. The client stores it in
`localStorage` and sends it on the WS URL. This is the second factor that turns "on the LAN" from
sufficient into merely necessary. (Replaces the former "no password" model.)

**Self-install (`install_windows.go`, desktop build).** Folds what `install.bat` did into the binary.
Registers a Task Scheduler **logon task running as the current user** — not a Windows service —
because input injection only works from the interactive session (a Session 0 service can't drive the
desktop). The firewall rule needs admin, so it's attempted directly and, if refused, re-launches
elevated via UAC (`ShellExecuteW` "runas" → `-setupfw` child). The MSIX build does none of this
(`install_store.go` toggles the manifest's `StartupTask` instead; no firewall — the Store relies on
the native Windows Firewall prompt on first listen).

**Single-instance + tray.** On a re-launch while an instance is already up, `alreadyRunning` probes
`/info` (matching our JSON shape) and just opens the `/qr` connect page instead of crashing on the
port conflict. In the `windowsgui` (no-console) build, the system tray (`fyne.io/systray`, pure-Go
on Windows) is the only UI — it owns the message loop and blocks in the main goroutine until "Sair"
or a termination signal, then runs graceful shutdown.

## Gotchas

- The PWA is **embedded at build time** — any change under `client/` (HTML, `sw.js`,
  `manifest.webmanifest`, icons) requires a rebuild to take effect.
- `sw.js` is served `Cache-Control: no-cache` and the manifest with an explicit content type;
  Chrome silently drops installability if these are wrong (see `serveTyped` in `main.go`).
- **MSIX / Microsoft Store** support lives behind the `store` build tag (see `packaging/`). The MSIX
  builds (`build-msix.ps1`), installs/runs locally (`build-msix-local.ps1`, dev-mode register), and the
  WinRT `StartupTask` toggle is **verified** (`verify-startuptask.ps1`: Disabled→Enabled→Disabled under
  package identity). The app is **submitted to the Store** as product "Controlinho" (Store ID
  `9NWT7X0QSGBJ`); the standalone `.exe` distribution stays supported in parallel.
- **WinRT from PowerShell gotcha:** `[System.WindowsRuntimeSystemExtensions]` (the `AsTask` that awaits
  a WinRT `IAsyncOperation`) does NOT resolve in Windows PowerShell 5.1 until `System.Runtime.WindowsRuntime`
  is loaded — `install_store.go`'s `psPreamble` force-loads it. The StartupTask API also needs **package
  identity**, so it only works from the installed/registered MSIX, never a bare `-tags store` .exe.
- `makeappx pack` requires the manifest staged as **`AppxManifest.xml`** (not `Package.appxmanifest`);
  the build scripts copy/rename it. MSIX Store assets in `packaging/Assets/` are generated by
  `gen_icons.go` (committed, like the `client/` icons); the manifest references the base sizes,
  scale-qualified variants are optional.
