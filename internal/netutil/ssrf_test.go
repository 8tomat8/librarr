package netutil

import "testing"

func TestValidateOutboundURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty", "", true},
		{"invalid scheme", "ftp://example.com/file", true},
		{"localhost", "http://localhost:8080/", true},
		{"127.0.0.1", "http://127.0.0.1/api", true},
		{"private 10.x", "http://10.0.0.1/", true},
		{"private 192.168", "http://192.168.1.1/", true},
		{"link-local", "http://169.254.169.254/", true},
		{"metadata host", "http://metadata.google.internal/", true},
		{"public https", "https://example.com/path", false},
		{"public http", "http://prowlarr.example:9696/", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutboundURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOutboundURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
