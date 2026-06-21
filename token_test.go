package main

import (
	"path/filepath"
	"testing"
)

func TestNormalizeToken(t *testing.T) {
	cases := map[string]string{
		"abcd23wxyz":   "ABCD23WXYZ",
		"ABCD-23-WXYZ": "ABCD23WXYZ",
		" abc 23 ":     "ABC23",
		"":             "",
	}
	for in, want := range cases {
		if got := normalizeToken(in); got != want {
			t.Errorf("normalizeToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidToken(t *testing.T) {
	orig := sessionToken
	defer func() { sessionToken = orig }()

	sessionToken = ""
	if validToken("ANYTHING") {
		t.Error("validToken should be false when no session token is set")
	}

	sessionToken = "ABCD23WXYZ"
	cases := []struct {
		in   string
		want bool
	}{
		{"ABCD23WXYZ", true},
		{"abcd23wxyz", true},   // case-insensitive
		{"ABCD-23-WXYZ", true}, // dashes stripped
		{" ABCD23WXYZ ", true}, // trimmed
		{"WRONGTOKEN", false},
		{"", false},
		{"ABCD23WXY", false}, // too short
	}
	for _, tc := range cases {
		if got := validToken(tc.in); got != tc.want {
			t.Errorf("validToken(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLoadOrCreateToken(t *testing.T) {
	dir := t.TempDir()

	first, err := loadOrCreateToken(dir)
	if err != nil {
		t.Fatalf("loadOrCreateToken: %v", err)
	}
	if len(first) != tokenLen {
		t.Errorf("token length = %d, want %d", len(first), tokenLen)
	}
	for _, r := range first {
		if !containsRune(tokenAlphabet, r) {
			t.Errorf("token char %q not in alphabet", r)
		}
	}

	// A second call must return the same persisted token, not a fresh one.
	second, err := loadOrCreateToken(dir)
	if err != nil {
		t.Fatalf("loadOrCreateToken (2nd): %v", err)
	}
	if second != first {
		t.Errorf("token not persisted: first=%q second=%q", first, second)
	}

	if _, err := filepath.Abs(filepath.Join(dir, tokenFile)); err != nil {
		t.Fatalf("abs: %v", err)
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
