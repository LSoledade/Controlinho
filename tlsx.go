package main

// Local TLS for an installable PWA — pure Go, no third-party services.
//
// Android Chrome only registers a service worker (and only offers "Install app")
// over a *trusted* HTTPS origin. A bare self-signed leaf is rejected. So we do
// exactly what mkcert does: generate a small local Certificate Authority once,
// then issue a leaf certificate for the machine's LAN/Tailscale IPs signed by it.
// The user installs the CA on the phone a single time and from then on every
// leaf we mint is trusted — secure context, SW, install prompt, all green.
//
// The CA private key never leaves this PC. Certs live in a per-user data dir.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	caCertName   = "pc-remote-ca.crt"
	caKeyName    = "pc-remote-ca.key"
	caCommonName = "Controlinho Local CA"
)

// leafRenewWindow is how close to expiry we re-mint the leaf. Well inside the
// 397-day lifetime, so a long-running server never serves a near-dead cert.
const leafRenewWindow = 30 * 24 * time.Hour

// certManager owns the CA and lazily mints (and caches) a leaf certificate via
// tls.Config.GetCertificate. It re-mints automatically when the machine's set of
// addresses changes (new Wi-Fi/DHCP) or the cached leaf nears expiry, so HTTPS
// self-heals without a restart.
type certManager struct {
	caCert    *x509.Certificate
	caKey     *ecdsa.PrivateKey
	caCertPEM []byte // public CA, served at /ca.crt

	mu       sync.RWMutex
	leaf     *tls.Certificate
	covers   []string  // sorted IP strings + DNS names the cached leaf covers
	notAfter time.Time // cached leaf expiry
}

// newCertManager loads (or creates) the persistent CA in dir and mints the first
// leaf for the current certSubjects().
func newCertManager(dir string) (*certManager, error) {
	caCert, caKey, caPEM, err := loadOrCreateCA(dir)
	if err != nil {
		return nil, fmt.Errorf("ca: %w", err)
	}
	m := &certManager{caCert: caCert, caKey: caKey, caCertPEM: caPEM}
	ips, names := certSubjects()
	if _, err := m.refresh(ips, names); err != nil {
		return nil, err
	}
	return m, nil
}

// caPEM returns the PEM-encoded CA certificate (public, for the phone to trust).
func (m *certManager) caPEM() []byte { return m.caCertPEM }

// subjectKey is the canonical sorted set of IP/name strings a leaf must cover,
// used to detect when the machine's addresses have changed.
func subjectKey(ips []net.IP, dnsNames []string) []string {
	out := make([]string, 0, len(ips)+len(dnsNames))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	out = append(out, dnsNames...)
	sort.Strings(out)
	return out
}

// coversAll reports whether the cached leaf already covers every entry in want.
func coversAll(have, want []string) bool {
	set := make(map[string]struct{}, len(have))
	for _, s := range have {
		set[s] = struct{}{}
	}
	for _, s := range want {
		if _, ok := set[s]; !ok {
			return false
		}
	}
	return true
}

// refresh mints a new leaf for the given subjects and caches it (caller must not
// hold the lock; refresh takes the write lock itself).
func (m *certManager) refresh(ips []net.IP, dnsNames []string) (*tls.Certificate, error) {
	certPEM, keyPEM, notAfter, err := mintLeaf(m.caCert, m.caKey, ips, dnsNames)
	if err != nil {
		return nil, err
	}
	leaf, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("tls keypair: %w", err)
	}
	m.mu.Lock()
	m.leaf = &leaf
	m.covers = subjectKey(ips, dnsNames)
	m.notAfter = notAfter
	m.mu.Unlock()
	return &leaf, nil
}

// GetCertificate is the tls.Config hook. It returns the cached leaf if it still
// covers all current subjects and isn't near expiry; otherwise it re-mints.
func (m *certManager) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	ips, names := certSubjects()
	want := subjectKey(ips, names)

	m.mu.RLock()
	leaf := m.leaf
	fresh := leaf != nil &&
		coversAll(m.covers, want) &&
		time.Until(m.notAfter) > leafRenewWindow
	m.mu.RUnlock()
	if fresh {
		return leaf, nil
	}
	return m.refresh(ips, names)
}

// dataDir returns a per-user, machine-local directory where we persist the CA.
// On Windows os.UserCacheDir() resolves to %LocalAppData% (machine-local, not
// roamed); on unix it's ~/.cache. We use a "pc-remote" subdir and create it with
// owner-only perms. If that can't be resolved we fall back to the directory next
// to the executable (the legacy location), then the working directory.
func dataDir() string {
	if base, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(base, "pc-remote")
		if err := os.MkdirAll(dir, 0o700); err == nil {
			return dir
		}
	}
	return legacyDataDir()
}

// legacyDataDir returns the directory next to the executable, where older
// versions stored the CA. Used as a fallback and as the migration source.
func legacyDataDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// mintLeaf signs a fresh leaf certificate for the supplied IPs/hostnames under
// the given CA, returning the leaf cert+key PEM and its NotAfter. Used by the
// certManager to (re)issue leaves as the machine's addresses change.
func mintLeaf(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, ips []net.IP, dnsNames []string) (certPEM, keyPEM []byte, notAfter time.Time, err error) {
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("leaf key: %w", err)
	}

	serial, err := randSerial()
	if err != nil {
		return nil, nil, time.Time{}, err
	}

	now := time.Now()
	expiry := now.Add(397 * 24 * time.Hour) // <398d keeps strict clients (Chrome) happy
	leaf := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: hostnameOr("pc-remote"), Organization: []string{"Controlinho"}},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              expiry,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           ips,
		DNSNames:              dnsNames,
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("sign leaf: %w", err)
	}

	leafKeyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("marshal leaf key: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyDER}),
		expiry, nil
}

// loadOrCreateCA reads the persistent CA from disk, or creates and stores it on
// first run. Returns the parsed cert+key (to sign leaves) and the CA PEM.
func loadOrCreateCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
	certPath := filepath.Join(dir, caCertName)
	keyPath := filepath.Join(dir, caKeyName)

	if certPEM, err := os.ReadFile(certPath); err == nil {
		if keyPEM, err := os.ReadFile(keyPath); err == nil {
			cert, key, err := parseCA(certPEM, keyPEM)
			if err == nil {
				return cert, key, certPEM, nil
			}
			// Corrupt/old CA: fall through and regenerate.
		}
	}

	// Migration: older versions kept the CA next to the executable. If it's
	// absent in the new data dir but present there, reuse it (and copy it over)
	// so phones that already trust this CA don't have to re-install it.
	if legacy := legacyDataDir(); legacy != dir {
		legacyCert := filepath.Join(legacy, caCertName)
		legacyKey := filepath.Join(legacy, caKeyName)
		if certPEM, err := os.ReadFile(legacyCert); err == nil {
			if keyPEM, err := os.ReadFile(legacyKey); err == nil {
				if cert, key, err := parseCA(certPEM, keyPEM); err == nil {
					// Best-effort copy into the new dir; keep working if it fails.
					_ = os.WriteFile(certPath, certPEM, 0o644)
					_ = os.WriteFile(keyPath, keyPEM, 0o600)
					return cert, key, certPEM, nil
				}
			}
		}
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	serial, err := randSerial()
	if err != nil {
		return nil, nil, nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: caCommonName, Organization: []string{caCommonName}},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, nil, err
	}

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	keyDER, err := x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		return nil, nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, caPEM, 0o644); err != nil {
		return nil, nil, nil, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, nil, nil, err
	}
	return caCert, caKey, caPEM, nil
}

func parseCA(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	cb, _ := pem.Decode(certPEM)
	if cb == nil {
		return nil, nil, fmt.Errorf("bad CA cert PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil {
		return nil, nil, fmt.Errorf("bad CA key PEM")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, ok := keyAny.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("CA key is not ECDSA")
	}
	return cert, key, nil
}

func randSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func hostnameOr(def string) string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return def
}
