package api

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestIsSecureRequestTrustedProxy(t *testing.T) {
	t.Cleanup(func() { setTrustedProxies(nil) })

	mkReq := func(remote, xfp string, useTLS bool) *http.Request {
		r := &http.Request{Header: http.Header{}, RemoteAddr: remote}
		if xfp != "" {
			r.Header.Set("X-Forwarded-Proto", xfp)
		}
		if useTLS {
			r.TLS = &tls.ConnectionState{}
		}
		return r
	}

	t.Run("no trusted proxies ignores forwarded proto", func(t *testing.T) {
		setTrustedProxies(nil)
		if isSecureRequest(mkReq("10.0.0.1:5000", "https", false)) {
			t.Error("X-Forwarded-Proto should not be trusted with no proxies configured")
		}
	})

	t.Run("trusted peer honored", func(t *testing.T) {
		setTrustedProxies([]string{"10.0.0.0/8"})
		if !isSecureRequest(mkReq("10.1.2.3:5000", "https", false)) {
			t.Error("trusted proxy with XFP=https should be secure")
		}
	})

	t.Run("untrusted peer ignored", func(t *testing.T) {
		setTrustedProxies([]string{"10.0.0.0/8"})
		if isSecureRequest(mkReq("203.0.113.5:5000", "https", false)) {
			t.Error("untrusted peer must not be able to spoof XFP")
		}
	})

	t.Run("bare IP entry", func(t *testing.T) {
		setTrustedProxies([]string{"192.168.1.10"})
		if !isSecureRequest(mkReq("192.168.1.10:443", "https", false)) {
			t.Error("bare IP proxy entry should match")
		}
		if isSecureRequest(mkReq("192.168.1.11:443", "https", false)) {
			t.Error("non-matching IP should not be trusted")
		}
	})

	t.Run("direct TLS always secure", func(t *testing.T) {
		setTrustedProxies(nil)
		if !isSecureRequest(mkReq("203.0.113.5:5000", "", true)) {
			t.Error("direct TLS connection should be secure")
		}
	})
}
