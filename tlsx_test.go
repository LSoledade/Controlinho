package main

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestSubjectKey(t *testing.T) {
	cases := []struct {
		name     string
		ips      []net.IP
		dnsNames []string
		want     []string
	}{
		{"empty", nil, nil, []string{}},
		{
			"sorted mix",
			[]net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("10.0.0.5")},
			[]string{"localhost", "pc-remote.local"},
			[]string{"10.0.0.5", "192.168.1.10", "localhost", "pc-remote.local"},
		},
		{
			"already sorted is stable",
			[]net.IP{net.ParseIP("127.0.0.1")},
			[]string{"a", "b"},
			[]string{"127.0.0.1", "a", "b"},
		},
		{
			"reversed order canonicalizes",
			[]net.IP{net.ParseIP("10.0.0.5")},
			[]string{"zeta", "alpha"},
			[]string{"10.0.0.5", "alpha", "zeta"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := subjectKey(tc.ips, tc.dnsNames)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("subjectKey() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSubjectKeyIsCanonical(t *testing.T) {
	// Different input orderings of the same set must produce identical keys.
	a := subjectKey([]net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("10.0.0.5")}, []string{"b", "a"})
	b := subjectKey([]net.IP{net.ParseIP("10.0.0.5"), net.ParseIP("192.168.1.10")}, []string{"a", "b"})
	if !reflect.DeepEqual(a, b) {
		t.Errorf("subjectKey not canonical: %v vs %v", a, b)
	}
}

func TestCoversAll(t *testing.T) {
	cases := []struct {
		name string
		have []string
		want []string
		res  bool
	}{
		{"equal sets", []string{"a", "b"}, []string{"a", "b"}, true},
		{"superset", []string{"a", "b", "c"}, []string{"a", "b"}, true},
		{"want empty", []string{"a"}, nil, true},
		{"both empty", nil, nil, true},
		{"missing entry", []string{"a", "b"}, []string{"a", "c"}, false},
		{"have empty want not", nil, []string{"a"}, false},
		{"want extra entry", []string{"a"}, []string{"a", "b"}, false},
		{"duplicate in have", []string{"a", "a"}, []string{"a"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := coversAll(tc.have, tc.want); got != tc.res {
				t.Errorf("coversAll(%v, %v) = %v, want %v", tc.have, tc.want, got, tc.res)
			}
		})
	}
}

func TestLoadOrCreateCAAndMintLeaf(t *testing.T) {
	dir := t.TempDir()

	caCert, caKey, caPEM, err := loadOrCreateCA(dir)
	if err != nil {
		t.Fatalf("loadOrCreateCA: %v", err)
	}
	if caCert == nil || caKey == nil || len(caPEM) == 0 {
		t.Fatal("loadOrCreateCA returned nil cert/key/PEM")
	}
	if !caCert.IsCA {
		t.Error("CA cert IsCA = false, want true")
	}
	if caCert.Subject.CommonName != caCommonName {
		t.Errorf("CA CommonName = %q, want %q", caCert.Subject.CommonName, caCommonName)
	}

	// Mint a leaf for a couple of IPs and names.
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("192.168.1.10")}
	names := []string{"localhost", "pc-remote.local"}
	certPEM, keyPEM, notAfter, err := mintLeaf(caCert, caKey, ips, names)
	if err != nil {
		t.Fatalf("mintLeaf: %v", err)
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatal("mintLeaf returned empty PEM")
	}

	// Parse the leaf from its PEM.
	leaf := parseLeafCert(t, certPEM)

	// Leaf must be signed by the CA.
	if err := leaf.CheckSignatureFrom(caCert); err != nil {
		t.Errorf("leaf not signed by CA: %v", err)
	}

	// Verify via a cert pool as well (full chain build).
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSName:   "pc-remote.local",
	}); err != nil {
		t.Errorf("leaf verify against pool failed: %v", err)
	}

	// Covers the requested IP SANs.
	for _, want := range ips {
		found := false
		for _, got := range leaf.IPAddresses {
			if got.Equal(want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("leaf missing IP SAN %v", want)
		}
	}

	// Covers the requested DNS SANs.
	for _, want := range names {
		found := false
		for _, got := range leaf.DNSNames {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("leaf missing DNS SAN %q", want)
		}
	}

	// NotAfter must be < 398 days out (strict-client friendly).
	maxAfter := time.Now().Add(398 * 24 * time.Hour)
	if !leaf.NotAfter.Before(maxAfter) {
		t.Errorf("leaf NotAfter %v not before %v (398d)", leaf.NotAfter, maxAfter)
	}
	// The returned notAfter must match the cert (x509 truncates to whole
	// seconds and stores UTC, so allow up to 1s of difference).
	if d := notAfter.Sub(leaf.NotAfter); d < -time.Second || d > time.Second {
		t.Errorf("returned notAfter %v differs from leaf.NotAfter %v by %v", notAfter, leaf.NotAfter, d)
	}

	// Loading again from the same dir must reuse the same CA (same serial),
	// not regenerate.
	caCert2, _, caPEM2, err := loadOrCreateCA(dir)
	if err != nil {
		t.Fatalf("loadOrCreateCA (second): %v", err)
	}
	if caCert.SerialNumber.Cmp(caCert2.SerialNumber) != 0 {
		t.Errorf("CA serial changed on reload: %v vs %v", caCert.SerialNumber, caCert2.SerialNumber)
	}
	if string(caPEM) != string(caPEM2) {
		t.Error("CA PEM changed on reload (regenerated instead of loaded)")
	}
}

func parseLeafCert(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("parse leaf: no PEM block")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	return leaf
}
