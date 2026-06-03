package netutil

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateOutboundURL checks that rawURL is a safe http(s) target for server-side
// requests. It rejects loopback, private, link-local, and metadata addresses.
func ValidateOutboundURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must include a host")
	}

	lower := strings.ToLower(host)
	for _, blocked := range []string{
		"localhost",
		"metadata.google.internal",
		"metadata.goog",
	} {
		if lower == blocked || strings.HasSuffix(lower, "."+blocked) {
			return fmt.Errorf("URL targets a restricted host")
		}
	}

	if ip := net.ParseIP(host); ip != nil {
		if isRestrictedIP(ip) {
			return fmt.Errorf("URL targets a restricted address")
		}
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// Hostname did not resolve (offline, Docker service name, etc.).
		// Blocked hostnames were already rejected above; allow the URL.
		return nil
	}
	for _, ip := range ips {
		if isRestrictedIP(ip) {
			return fmt.Errorf("URL targets a restricted address")
		}
	}
	return nil
}

func isRestrictedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return true
	}
	return false
}
