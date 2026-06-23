# DESLOPPIFY — cleanup backlog

A prioritized review of `pc-remote` / Controlinho (v2.2.0). Items are grouped
**Critical → Medium → Nice-to-have**. Each has: where it is, why it matters, the
recommended change, and whether it is safe to fix now.

The Go server side is in good shape — `go vet ./...` and `go vet -tags store ./...`
both pass clean. **Every critical item is in the embedded web client**, and both
critical items come from the same regression in the latest commit
(`d4bb18e`, "first-run setup wizard").

---

## 1. Critical

### C1 — The client script crashes on boot; the app connects to nothing
- **Where:** `client/index.html`, lines ~862–875 (the "install banner (HTTP setup
  only)" block).
- **Why it matters:** Commit `d4bb18e` deleted the `#installBanner`, `#bannerClose`
  and `#bannerHttps` elements from the HTML (replaced by the new wizard) but left
  the `<script>` referencing them. `document.getElementById("bannerClose")` now
  returns `null`, and `.addEventListener(...)` on `null` throws a `TypeError`. That
  exception aborts the IIFE at that line, so **everything below it never executes**:
  the `/info` fetch, the **service-worker registration** (lines ~877–880), and the
  **`connect()` boot call** (line ~883). Result: the WebSocket never opens, the PWA
  never registers its SW (no install prompt, no offline shell). The UI renders but is
  completely non-functional. This breaks on *every* origin (HTTP and HTTPS), since
  the elements don't exist anywhere.
- **Recommend:** Remove the dead banner block, or rewrite it to drive the new wizard
  (see C2). Until then the app is shipping broken. After fixing, smoke-test: socket
  reaches "conectado", SW registers, install pill appears.
- **Safe to fix now:** **Yes — urgent.** Fix together with C2.

### C2 — The new first-run setup wizard has no JavaScript — the feature does nothing
- **Where:** `client/index.html`, lines ~317–370 (`#setupWizard` and its children:
  `caDownload`, `osName`, `howtoAndroid` / `howtoIOS`, `checkTrust`, `openSecure`,
  `setupSkip`, `trustState`).
- **Why it matters:** The wizard markup was added but **no JS shows or drives it**.
  It is `hidden` by default and never un-hidden; there is no OS detection to set
  `osName` / pick the Android-vs-iOS how-to; and `checkTrust` ("Já instalei —
  verificar"), `openSecure`, and `setupSkip` have no click handlers. So on the
  insecure HTTP setup origin — the phone's actual entry point — the user sees the
  bare app UI (which can't control anything without a token/secure context) instead
  of the onboarding flow. The headline feature of the last commit is inert.
- **Recommend:** Decide the intended flow first (small design call): the wizard is
  clearly meant to *replace* the old install banner. Then wire it up — show
  `#setupWizard` on the insecure origin, detect platform for the how-to text, do the
  trust-check (e.g. probe the HTTPS origin) on `checkTrust`, build the
  `openSecure`/`setupSkip` links with the `?k=` token carried across the http→https
  hop, and unlock step 3 when trust is confirmed.
- **Safe to fix now:** **Yes**, but needs the wizard-vs-banner decision up front.

---

## 2. Medium cleanup

### M1 — Virtual-key codes are hardcoded in three places
- **Where:** `main.go` `volumeAction`/`mediaAction` use raw hex (`0xAF`, `0xAE`,
  `0xAD`, `0xB3`, `0xB0`, `0xB1`); the same codes are also named in `vkAliases` and
  listed in `extendedVK` (`input_windows.go`).
- **Why it matters:** Three sources of truth for the same constants. A change in one
  can silently diverge from the others, and `powerAction` already shows the cleaner
  pattern (named helpers). The stub build shares none of these.
- **Recommend:** Route volume/media through the existing resolver
  (`vkFor("volumeup")`, etc.) or a single shared named-constant block.
- **Safe to fix now:** Yes — small, mechanical, covered by manual testing.

### M2 — Pairing-token generation has modulo bias
- **Where:** `token.go:61`, `tokenAlphabet[int(v)%len(tokenAlphabet)]`.
- **Why it matters:** A 30-char alphabet indexed by `byte % 30` over values 0–255
  makes the first 16 glyphs ~2× as likely. The "~49 bits" claim in the comment is
  therefore optimistic and the distribution is non-uniform. Not practically
  exploitable for a LAN-only secret, but it's an avoidable crypto smell.
- **Recommend:** Rejection sampling, or `crypto/rand.Int` over the alphabet length.
- **Safe to fix now:** Yes. (Existing persisted tokens still validate; only newly
  generated ones change.)

### M3 — `validToken` normalizes the input but not the stored secret
- **Where:** `token.go` — `validToken` calls `normalizeToken(s)` then compares to
  `sessionToken`, which is only `TrimSpace`'d at load (`token.go:39`).
- **Why it matters:** It works *only* because the generator emits uppercase
  alphabet chars. The comments explicitly invite deleting/rotating the token file by
  hand; if anyone writes a lowercase or dash-formatted value there, every correct PIN
  silently fails to authenticate. Fragile invariant held by coincidence.
- **Recommend:** `normalizeToken` the value once when loading it in
  `loadOrCreateToken`, so the stored secret is canonical regardless of file contents.
- **Safe to fix now:** Yes.

### M4 — Sibling port globals split across two files
- **Where:** `httpPortValue` is declared in `qr.go:77`; `httpsPortValue` in
  `main.go:198`. Both are set in `main()`.
- **Why it matters:** Two closely related values living in two files makes the data
  flow harder to follow and is the kind of thing that grows into "where is this set?"
  confusion.
- **Recommend:** Colocate both declarations in `main.go` next to where they're
  assigned.
- **Safe to fix now:** Yes — trivial move.

### M5 — Inconsistent ack semantics, undocumented
- **Where:** `main.go` `runCmd`.
- **Why it matters:** `ping` and `power` ack on success; `mouse_click`, `key`,
  `shortcut`, `type`, `volume`, `media` reply only on *failure*. The client only
  reacts to `ok === false`, so this is intentional — but the asymmetry isn't stated,
  and the success-ack path (`ack.OK`/`ack.ID`) is effectively dead for most commands.
  A future reader can't tell whether a missing ack is a bug or the contract.
- **Recommend:** Document the "errors-only ack (except ping/power)" contract at
  `runCmd`, or make acking uniform. Low risk either way.
- **Safe to fix now:** Yes, but low urgency.

---

## 3. Nice-to-have polish

### N1 — `truncate` can split a UTF-8 rune
- **Where:** `main.go:352`, `s[:n]` slices by bytes.
- **Why it matters:** A bad WS frame containing multibyte characters logged at the
  120-byte boundary can emit invalid UTF-8 into the log. Cosmetic, log-only.
- **Recommend:** Rune-aware truncation (`utf8`-safe).
- **Safe to fix now:** Yes; trivial, low value.

### N2 — `install.bat` is a thin legacy wrapper
- **Where:** `install.bat`.
- **Why it matters:** All logic moved into the binary (`pc-remote.exe -install`);
  the `.bat` just calls it. Still referenced by `README.md` as a double-click path,
  so not strictly dead, but it's an extra artifact to keep in sync.
- **Recommend:** Keep as a convenience and leave a one-line note, or drop it and
  update the README to point at `-install` directly.
- **Safe to fix now:** Yes; cosmetic.

### N3 — Double JSON parse on every WS frame
- **Where:** `main.go` `dispatch` — always attempts `[]cmd` first, then falls back to
  a single object.
- **Why it matters:** `mouse_move` is the hot path and takes the object branch, so it
  is parsed-then-reparsed. Negligible at current rates, but it's the latency-
  sensitive flow.
- **Recommend:** Peek the first non-space byte (`[` vs `{`) to pick the decode path.
- **Safe to fix now:** Yes; micro-optimization, optional.

### N4 — `handleClient` opens each asset twice
- **Where:** `main.go` `handleClient` — an existence probe (`clientRoot.Open`) then
  `fileServer.ServeHTTP` opens it again.
- **Why it matters:** Minor redundant work per static request; embedded FS so cheap.
- **Recommend:** Acceptable as-is; only worth touching if this code is refactored.
- **Safe to fix now:** Yes; lowest priority.

---

## Suggested order of attack
1. **C1 + C2** together — the app is currently broken at runtime; this is the only
   thing that blocks real use. One focused client-side pass fixes both.
2. **M3, M2** — token correctness/robustness (cheap, security-adjacent).
3. **M1, M4** — de-duplicate constants, colocate globals.
4. **M5, N1–N4** — clarity and polish as time allows.
