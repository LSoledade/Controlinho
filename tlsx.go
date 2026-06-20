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
// The CA private key never leaves this PC. Certs live next to the executable.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	caCertName   = "pc-remote-ca.crt"
	caKeyName    = "pc-remote-ca.key"
	caCommonName = "PC Remote Local CA"
)

// tlsBundle is everything the servers and the /ca.crt endpoint need.
type tlsBundle struct {
	leafCertPEM []byte // PEM leaf certificate
	leafKeyPEM  []byte // PEM leaf private key
	caCertPEM   []byte // PEM CA certificate (handed to the phone for install)
}

// dataDir returns the directory next to the executable where we persist certs.
// Falls back to the working directory if the executable path can't be resolved.
func dataDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// ensureTLS loads the persistent CA (creating it on first run) and mints a fresh
// leaf certificate covering the supplied IPs/hostnames. The leaf is regenerated
// every start so it always matches the machine's current addresses; the CA is
// stable so the phone only ever trusts it once.
func ensureTLS(dir string, ips []net.IP, dnsNames []string) (*tlsBundle, error) {
	caCert, caKey, caPEM, err := loadOrCreateCA(dir)
	if err != nil {
		return nil, fmt.Errorf("ca: %w", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("leaf key: %w", err)
	}

	serial, err := randSerial()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	leaf := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: hostnameOr("pc-remote"), Organization: []string{"PC Remote"}},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(397 * 24 * time.Hour), // <398d keeps strict clients (Chrome) happy
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           ips,
		DNSNames:              dnsNames,
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("sign leaf: %w", err)
	}

	leafKeyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		return nil, fmt.Errorf("marshal leaf key: %w", err)
	}

	return &tlsBundle{
		leafCertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		leafKeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyDER}),
		caCertPEM:   caPEM,
	}, nil
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
