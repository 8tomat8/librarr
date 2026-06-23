package netutil

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func parseHTTPURL(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("URL is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL must use http or https")
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("URL must include a host")
	}
	return u, nil
}

func isMetadataHost(host string) bool {
	lower := strings.ToLower(host)
	for _, blocked := range []string{
		"metadata.google.internal",
		"metadata.goog",
	} {
		if lower == blocked || strings.HasSuffix(lower, "."+blocked) {
			return true
		}
	}
	return false
}

// ValidateIntegrationURL checks admin-initiated integration test URLs (Prowlarr,
// Kavita, etc.). Private and loopback addresses are allowed — homelab services
// commonly run at http://192.168.x.x:port or http://localhost:port.
func ValidateIntegrationURL(rawURL string) error {
	u, err := parseHTTPURL(rawURL)
	if err != nil {
		return err
	}
	if isMetadataHost(u.Hostname()) {
		return fmt.Errorf("URL targets a restricted host")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && isCloudMetadataIP(ip) {
		return fmt.Errorf("URL targets a restricted address")
	}
	return nil
}

// ValidateOutboundURL checks that rawURL is a safe http(s) target for server-side
// requests. It rejects loopback, private, link-local, and metadata addresses.
func ValidateOutboundURL(rawURL string) error {
	u, err := parseHTTPURL(rawURL)
	if err != nil {
		return err
	}
	host := u.Hostname()

	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("URL targets a restricted host")
	}
	if isMetadataHost(host) {
		return fmt.Errorf("URL targets a restricted host")
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

func isCloudMetadataIP(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 169 && ip4[1] == 254
}

func isRestrictedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}
	return isCloudMetadataIP(ip)
}
