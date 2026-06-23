package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
)

// trustedProxies holds the parsed CIDR ranges of reverse proxies whose
// forwarded headers (notably X-Forwarded-Proto) may be trusted. It is set once
// at server start via setTrustedProxies and read concurrently per request.
var (
	trustedProxiesMu sync.RWMutex
	trustedProxies   []*net.IPNet
)

// setTrustedProxies parses a list of CIDRs or bare IPs (bare IPs become /32 or
// /128) into the trusted-proxy set. Invalid entries are skipped.
func setTrustedProxies(entries []string) {
	var nets []*net.IPNet
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.Contains(e, "/") {
			if ip := net.ParseIP(e); ip != nil {
				if ip.To4() != nil {
					e += "/32"
				} else {
					e += "/128"
				}
			}
		}
		if _, n, err := net.ParseCIDR(e); err == nil {
			nets = append(nets, n)
		}
	}
	trustedProxiesMu.Lock()
	trustedProxies = nets
	trustedProxiesMu.Unlock()
}

// remoteFromTrustedProxy reports whether the request's immediate peer
// (RemoteAddr) is in the configured trusted-proxy set. With no proxies
// configured this is always false, so spoofed forwarded headers are ignored.
func remoteFromTrustedProxy(r *http.Request) bool {
	trustedProxiesMu.RLock()
	nets := trustedProxies
	trustedProxiesMu.RUnlock()
	if len(nets) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// forwardedProtoHTTPS reports whether the request arrived over HTTPS, honoring
// X-Forwarded-Proto only when the immediate peer is a trusted proxy. A direct
// client cannot spoof the header into flipping the Secure cookie flag.
func forwardedProtoHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return remoteFromTrustedProxy(r) && r.Header.Get("X-Forwarded-Proto") == "https"
}

func isSecureRequest(r *http.Request) bool {
	return forwardedProtoHTTPS(r)
}

func sessionCookie(r *http.Request, token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     "librarr_session",
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	}
}

func clearSessionCookie(r *http.Request) *http.Cookie {
	c := sessionCookie(r, "", -1)
	return c
}
