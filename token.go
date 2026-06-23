package main

// Pairing token — a second factor on top of the network-topology trust. Until now
// trust was purely "you're on the LAN/Tailscale"; that leaves the WebSocket open to
// any device (or rebinding/CSRF page) that reaches the port. Every WS upgrade now
// must also present this token (via ?k=… or the pairing cookie), so reaching the
// network is necessary but no longer sufficient.
//
// The token is embedded in the "app" QR (green) so scanning pairs with zero
// friction; for manual IP entry the user types it from the connect page. We persist
// it next to the CA (owner-only) so pairing is one-time and survives restarts —
// same model as the trusted-CA install, and the same MSIX-persistent storage. Delete
// the file (pc-remote-token in dataDir) to rotate the secret.

import (
	"crypto/rand"
	"crypto/subtle"
	"os"
	"path/filepath"
	"strings"
)

// sessionToken is the live pairing secret, set once at startup.
var sessionToken string

const tokenFile = "pc-remote-token"

// tokenAlphabet is Crockford-style base32 minus ambiguous glyphs (no I, L, O, U,
// 0, 1) so the token reads and types cleanly off the QR page.
const tokenAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

const tokenLen = 10 // ~49 bits — overkill for LAN-only, trivial to type once

// loadOrCreateToken returns the persisted pairing token, generating and saving one
// (owner-only) on first run.
func loadOrCreateToken(dir string) (string, error) {
	path := filepath.Join(dir, tokenFile)
	if b, err := os.ReadFile(path); err == nil {
		// Canonicalize the stored secret so a hand-edited file (lowercase, dashes)
		// still matches: validToken normalizes the input, so the secret must be
		// normalized too or every correct PIN would silently fail.
		if t := normalizeToken(string(b)); t != "" {
			return t, nil
		}
	}
	t, err := newToken(tokenLen)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(t), 0o600); err != nil {
		return "", err
	}
	return t, nil
}

// newToken returns n random characters from tokenAlphabet. It uses rejection
// sampling rather than a plain modulo so every glyph is equally likely: with a
// 30-char alphabet, `byte % 30` would make the first 16 glyphs ~2× as frequent
// and erode the ~49-bit claim.
func newToken(n int) (string, error) {
	// largest multiple of the alphabet length that fits in a byte (240 for 30)
	const limit = 256 - 256%len(tokenAlphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, v := range buf {
			if int(v) >= limit {
				continue // reject the biased tail
			}
			out = append(out, tokenAlphabet[int(v)%len(tokenAlphabet)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out), nil
}

// normalizeToken upper-cases and strips spaces/dashes so a hand-typed PIN with
// formatting still matches.
func normalizeToken(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// validToken reports whether s matches the session token (constant time).
func validToken(s string) bool {
	if sessionToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(normalizeToken(s)), []byte(sessionToken)) == 1
}
