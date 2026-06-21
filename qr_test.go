package main

import "testing"

func TestIsTailscale(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.100.50.1", true},
		{"100.127.255.255", true},
		{"100.63.255.255", false}, // just below CGNAT
		{"100.128.0.1", false},    // just above CGNAT
		{"100.0.0.1", false},
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"8.8.8.8", false},
		{"", false},
		{"garbage", false},
		{"100.", false},
		{"100.notanumber.1.1", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			if got := isTailscale(tc.ip); got != tc.want {
				t.Errorf("isTailscale(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestTailscaleIP(t *testing.T) {
	cases := []struct {
		name string
		ips  []string
		want string
	}{
		{"empty", nil, ""},
		{"none tailscale", []string{"192.168.1.10", "10.0.0.5"}, ""},
		{"picks tailscale", []string{"192.168.1.10", "100.100.1.2", "10.0.0.5"}, "100.100.1.2"},
		{"first tailscale wins", []string{"100.64.0.1", "100.120.0.2"}, "100.64.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tailscaleIP(tc.ips); got != tc.want {
				t.Errorf("tailscaleIP(%v) = %q, want %q", tc.ips, got, tc.want)
			}
		})
	}
}

func TestBestLANIP(t *testing.T) {
	cases := []struct {
		name string
		ips  []string
		want string
	}{
		{"empty", nil, ""},
		{"192.168 preferred over 10", []string{"10.0.0.5", "192.168.1.10"}, "192.168.1.10"},
		{"192.168 preferred over 172", []string{"172.16.0.1", "192.168.1.10"}, "192.168.1.10"},
		{"10 preferred over 172", []string{"172.16.0.1", "10.0.0.5"}, "10.0.0.5"},
		{"only 172", []string{"172.16.0.1"}, "172.16.0.1"},
		{"only 10", []string{"10.0.0.5"}, "10.0.0.5"},
		{"tailscale excluded, falls to 192.168", []string{"100.100.1.2", "192.168.1.10"}, "192.168.1.10"},
		{"tailscale excluded, falls to 10", []string{"100.100.1.2", "10.0.0.5"}, "10.0.0.5"},
		{"only tailscale -> empty", []string{"100.100.1.2", "100.64.0.1"}, ""},
		{"unknown non-tailscale falls through", []string{"100.100.1.2", "8.8.8.8"}, "8.8.8.8"},
		{"192.168 wins even after 10 seen", []string{"10.0.0.5", "172.16.0.1", "192.168.1.10"}, "192.168.1.10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bestLANIP(tc.ips); got != tc.want {
				t.Errorf("bestLANIP(%v) = %q, want %q", tc.ips, got, tc.want)
			}
		})
	}
}
