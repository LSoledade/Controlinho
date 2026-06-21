package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllowedHost(t *testing.T) {
	cases := []struct {
		name string
		addr string
		want bool
	}{
		// Loopback, with and without port.
		{"loopback v4 with port", "127.0.0.1:54321", true},
		{"loopback v4 no port", "127.0.0.1", true},
		{"loopback v4 other", "127.5.6.7:8080", true},
		{"loopback v6 with port", "[::1]:54321", true},
		{"loopback v6 no port", "::1", true},

		// Private RFC1918 ranges.
		{"private 10.x", "10.0.0.5:1", true},
		{"private 10.x high", "10.255.255.254:9", true},
		{"private 172.16", "172.16.0.1:8080", true},
		{"private 172.31", "172.31.255.1:8080", true},
		{"private 192.168", "192.168.1.10:443", true},
		{"private 192.168 no port", "192.168.0.1", true},

		// Tailscale CGNAT 100.64.0.0/10 => 100.64.x .. 100.127.x.
		{"tailscale 100.64", "100.64.0.1:8080", true},
		{"tailscale 100.100", "100.100.50.1:8080", true},
		{"tailscale 100.127", "100.127.255.255:8080", true},

		// Just outside CGNAT.
		{"tailscale-ish 100.63 reject", "100.63.255.255:8080", false},
		{"tailscale-ish 100.128 reject", "100.128.0.1:8080", false},

		// Outside the 172.16/12 block.
		{"172.15 reject", "172.15.0.1:8080", false},
		{"172.32 reject", "172.32.0.1:8080", false},

		// Public IPs.
		{"public 8.8.8.8 reject", "8.8.8.8:8080", false},
		{"public 8.8.8.8 no port", "8.8.8.8", false},
		{"public 1.2.3.4 reject", "1.2.3.4:1234", false},

		// IPv6 ULA fd00::/8 accepted.
		{"ipv6 ula with port", "[fd00::1]:8080", true},
		{"ipv6 ula no port", "fd00::1", true},
		{"ipv6 ula fc00", "[fc00::abcd]:1", true},

		// IPv6 link-local accepted (link-local unicast).
		{"ipv6 link-local", "[fe80::1]:8080", true},

		// IPv6 public reject.
		{"ipv6 public reject", "[2001:4860:4860::8888]:8080", false},

		// Garbage / empty.
		{"empty", "", false},
		{"garbage", "not-an-ip", false},
		{"garbage with port", "garbage:8080", false},
		{"only port-ish", ":8080", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := allowedHost(tc.addr); got != tc.want {
				t.Errorf("allowedHost(%q) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
}

func TestCheckOrigin(t *testing.T) {
	cases := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{"empty origin", "192.168.1.10:8443", "", true},
		{"matching host", "192.168.1.10:8443", "https://192.168.1.10:8443", true},
		{"matching host http", "pc-remote.local:8080", "http://pc-remote.local:8080", true},
		{"case-insensitive host", "PC-Remote.local:8080", "http://pc-remote.local:8080", true},
		{"case-insensitive host 2", "pc-remote.local:8080", "http://PC-REMOTE.LOCAL:8080", true},
		{"mismatched host", "192.168.1.10:8443", "https://evil.example.com", false},
		{"mismatched port counts as mismatch", "192.168.1.10:8443", "https://192.168.1.10:9999", false},
		{"malformed origin", "192.168.1.10:8443", "://::not a url", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://example/ws", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := checkOrigin(r); got != tc.want {
				t.Errorf("checkOrigin(host=%q, origin=%q) = %v, want %v", tc.host, tc.origin, got, tc.want)
			}
		})
	}
}

func TestPortOf(t *testing.T) {
	cases := []struct {
		addr string
		want string
	}{
		{"0.0.0.0:8080", ":8080"},
		{"127.0.0.1:1", ":1"},
		{"192.168.1.10:8443", ":8443"},
		{"[::1]:443", ":443"},
		{"nohost", ""},
		{"host:abc", ""},
		{"", ""},
		{"127.0.0.1:", ""},
		{":8080", ":8080"},
	}

	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			if got := portOf(tc.addr); got != tc.want {
				t.Errorf("portOf(%q) = %q, want %q", tc.addr, got, tc.want)
			}
		})
	}
}
