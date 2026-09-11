package server

import (
	"net"
	"net/http"
	"strings"
)

type Allowlist struct {
	nets []*net.IPNet
}

func NewAllowlist(cidrs []string) *Allowlist {
	a := &Allowlist{}

	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		if !strings.Contains(raw, "/") {
			if ip := net.ParseIP(raw); ip != nil {
				if ip.To4() != nil {
					raw += "/32"
				} else {
					raw += "/128"
				}
			}
		}

		if _, n, err := net.ParseCIDR(raw); err == nil {
			a.nets = append(a.nets, n)
		}
	}

	return a
}

func (a *Allowlist) Allows(r *http.Request) bool {
	ip := clientIP(r)
	if ip == nil {
		return false
	}

	if ip.IsLoopback() {
		return true
	}

	for _, n := range a.nets {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}

func clientIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	peer := net.ParseIP(host)
	if peer == nil {
		return nil
	}

	if !peer.IsLoopback() {
		return peer
	}

	if real := r.Header.Get("X-Real-IP"); real != "" {
		if ip := net.ParseIP(strings.TrimSpace(real)); ip != nil {
			return ip
		}
	}

	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
			return ip
		}
	}

	return peer
}
