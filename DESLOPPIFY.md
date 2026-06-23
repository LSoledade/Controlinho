# DESLOPPIFY — cleanup backlog

A prioritized review of `pc-remote` / Controlinho (v2.2.0). Items are grouped
**Critical → Medium → Nice-to-have**. Each has: where it is, why it matters, the
recommended change, and whether it is safe to fix now.

The Go server side is in good shape — `go vet ./...` and `go vet -tags store ./...`
both pass clean.

---

## 1. Critical — ✅ RESOLVED

Both critical items came from the `d4bb18e` regression and were resolved when the
client was refactored from a monolithic inline-script `index.html` into ES modules
(`client/js/*.js`). Verified 2026-06-23: `index.html` loads `js/app.js` (a module),
the dead install-banner block is gone (only `iosBanner` remains), and the wizard is
fully driven by `client/js/setup.js`.

### C1 — ✅ The client script crashes on boot — FIXED
- The dead `#installBanner`/`#bannerClose` block no longer exists. Boot now lives in
  `client/js/app.js`: it imports each module (listeners attach on load), then calls
  `connect()` unless `inSetup`. SW registration moved to `client/js/pwa.js`. No
  null-element `addEventListener`, so nothing aborts the boot path.

### C2 — ✅ The first-run wizard had no JavaScript — FIXED
- `client/js/setup.js` fully drives `#setupWizard`: it shows the wizard only on the
  insecure HTTP origin (`location.protocol === "http:" && !isLocalhost && !isStandalone`),
  detects iOS to pick the how-to text, probes the HTTPS origin (`probeTrust` loads
  `icon-192.png`) on `checkTrust`, and builds the `openSecure`/`setupSkip` handoff
  with the `?k=` token pulled from `/info` so it crosses the http→https origin hop.

---

## 2. Medium cleanup

### M1 — ✅ Virtual-key codes are hardcoded in three places — FIXED
- **Was:** `main.go` `volumeAction`/`mediaAction` used raw hex (`0xAF`, `0xAE`,
  `0xAD`, `0xB3`, `0xB0`, `0xB1`), duplicating codes that also live in `vkAliases`.
- **Done:** both now map the action to a named key (`"volumeup"`, `"media_next"`, …)
  and resolve it through `vkFor` via a new `pressNamed` helper, so the codes live in
  exactly one place (`vkAliases` in `input_windows.go`). No raw hex left in `main.go`.

### M2 — ✅ Pairing-token generation has modulo bias — FIXED
- **Was:** `token.go` `newToken` used `tokenAlphabet[int(v)%len(tokenAlphabet)]`; a
  30-char alphabet indexed by `byte % 30` made the first 16 glyphs ~2× as likely.
- **Done:** `newToken` now uses rejection sampling (rejects byte values ≥ 240, the
  largest multiple of 30 that fits in a byte) so every glyph is equally likely and
  the ~49-bit claim holds. Existing persisted tokens are unaffected.

### M3 — ✅ `validToken` normalized input but not the stored secret — FIXED
- **Was:** `loadOrCreateToken` returned the file's contents only `TrimSpace`'d, so a
  hand-edited lowercase/dashed token would never match `normalizeToken`'d input.
- **Done:** `loadOrCreateToken` now `normalizeToken`s the stored value at load, so
  the secret is canonical regardless of file contents.

### M4 — ✅ Sibling port globals split across two files — FIXED
- **Was:** `httpPortValue` in `qr.go`, `httpsPortValue` in `main.go`.
- **Done:** both colocated in a single `var (…)` block in `main.go` next to where
  they're assigned; `qr.go` reads them unchanged.

### M5 — ✅ Inconsistent ack semantics, undocumented — FIXED
- **Was:** `ping`/`power` ack on success; everything else replies only on failure;
  the contract was unstated.
- **Done:** documented the "errors-only ack, except ping/power; mouse_move never
  acks" contract in the `runCmd` doc comment. Behavior unchanged (left as-is, since
  the client only reacts to `ok===false`).

---

## 3. Nice-to-have polish

### N1 — ✅ `truncate` could split a UTF-8 rune — FIXED
- **Was:** `truncate` sliced `s[:n]` by bytes, so a multibyte char at the 120-byte
  boundary could emit invalid UTF-8 into the log.
- **Done:** `truncate` now backs `n` off to a rune boundary (`utf8.RuneStart`) before
  slicing.

### N2 — `install.bat` is a thin legacy wrapper
- **Where:** `install.bat`.
- **Why it matters:** All logic moved into the binary (`pc-remote.exe -install`);
  the `.bat` just calls it. Still referenced by `README.md` as a double-click path,
  so not strictly dead, but it's an extra artifact to keep in sync.
- **Recommend:** Keep as a convenience and leave a one-line note, or drop it and
  update the README to point at `-install` directly.
- **Safe to fix now:** Yes; cosmetic.

### N3 — ✅ Double JSON parse on every WS frame — FIXED
- **Was:** `dispatch` always tried `[]cmd` first, so the hot `mouse_move` object path
  was parsed-then-reparsed.
- **Done:** `dispatch` now peeks the first non-space byte (new `firstNonSpace` helper):
  `[` → batch decode, anything else → single-object decode. The hot path is parsed once.

### N4 — `handleClient` opens each asset twice
- **Where:** `main.go` `handleClient` — an existence probe (`clientRoot.Open`) then
  `fileServer.ServeHTTP` opens it again.
- **Why it matters:** Minor redundant work per static request; embedded FS so cheap.
- **Recommend:** Acceptable as-is; only worth touching if this code is refactored.
- **Safe to fix now:** Yes; lowest priority.

---

## Status (updated 2026-06-23)

Done: **C1, C2** (resolved by the earlier ESM client refactor) and **M1–M5, N1, N3**
(this pass). Build, `go vet ./...`, `go vet -tags store ./...`, the Linux stub
cross-compile, and `go test ./...` all pass.

Remaining — both optional, cosmetic, lowest priority:
- **N2** — `install.bat` is a thin legacy wrapper. Keep with a one-line note, or drop
  it and point `README.md` at `-install` directly.
- **N4** — `handleClient` opens each asset twice (existence probe + `ServeHTTP`).
  Acceptable as-is; only worth touching if that code is refactored.
