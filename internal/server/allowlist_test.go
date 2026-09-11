package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsAllowlistProdIPs(t *testing.T) {
	a := NewAllowlist([]string{
		"10.0.0.0/8",
		"141.125.104.199/32",
		"158.176.182.217/32",
		"161.156.201.94/32",
		"141.125.104.103/32",
	})

	cases := map[string]bool{
		"141.125.104.199": true,
		"158.176.182.217": true,
		"161.156.201.94":  true,
		"141.125.104.103": true,
		"10.242.64.7":     true,
		"10.242.128.10":   true,
		"8.8.8.8":         false,
		"51.89.98.189":    false,
	}

	for ip, want := range cases {
		r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		r.RemoteAddr = ip + ":1234"
		if got := a.Allows(r); got != want {
			t.Errorf("direct %s: got %v want %v", ip, got, want)
		}

		r = httptest.NewRequest(http.MethodGet, "/metrics", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Real-IP", ip)
		if got := a.Allows(r); got != want {
			t.Errorf("via nginx %s: got %v want %v", ip, got, want)
		}
	}
}
