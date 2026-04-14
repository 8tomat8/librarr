package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JeremiahM37/librarr/internal/config"
)

// newZLibraryTestServer returns an httptest.Server that mocks the Z-Library
// login, domain-resolution, and search endpoints.
func newZLibraryTestServer(t *testing.T, unauthorizedFirst bool) *httptest.Server {
	t.Helper()
	var (
		loginCount  int
		searchCount int
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/eapi/user/login", func(w http.ResponseWriter, r *http.Request) {
		loginCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"user":    map[string]any{"id": 1, "email": "u@example.com"},
		})
	})
	mux.HandleFunc("/eapi/info/domains", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"domains": []string{"dl.example.org"},
		})
	})
	mux.HandleFunc("/eapi/book/search", func(w http.ResponseWriter, r *http.Request) {
		searchCount++
		// Optionally force a 401 on the first request to exercise re-auth path.
		if unauthorizedFirst && searchCount == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []map[string]any{
				{
					"id":        42,
					"hash":      "abc123",
					"title":     "The Go Programming Language",
					"author":    "Donovan & Kernighan",
					"extension": "epub",
					"filesize":  2_500_000,
					"language":  "english",
				},
			},
			"pagination": map[string]any{"page": 1, "total": 1},
		})
	})
	return httptest.NewServer(mux)
}

func TestZLibrarySearch(t *testing.T) {
	srv := newZLibraryTestServer(t, false)
	defer srv.Close()

	cfg := &config.Config{
		ZLibraryURL:      srv.URL,
		ZLibraryEmail:    "u@example.com",
		ZLibraryPassword: "pw",
		ZLibraryEnabled:  true,
		UserAgent:        "test-agent",
	}
	z := NewZLibrary(cfg, &http.Client{Timeout: 5 * time.Second})

	if !z.Enabled() {
		t.Fatalf("ZLibrary should be enabled")
	}

	results, err := z.Search(context.Background(), "go")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	r := results[0]
	if r.Source != "zlibrary" {
		t.Errorf("Source = %q, want zlibrary", r.Source)
	}
	if r.Title != "The Go Programming Language" {
		t.Errorf("Title = %q", r.Title)
	}
	if r.Format != "EPUB" {
		t.Errorf("Format = %q, want EPUB (extension upper-cased)", r.Format)
	}
	if r.SourceID != "zlibrary-42" {
		t.Errorf("SourceID = %q", r.SourceID)
	}
	if !strings.Contains(r.DownloadURL, "dl.example.org") || !strings.Contains(r.DownloadURL, "/dl/42/abc123") {
		t.Errorf("DownloadURL = %q, want it to use personal domain and hash", r.DownloadURL)
	}
	if r.SizeHuman == "" {
		t.Errorf("SizeHuman should be non-empty")
	}
}

func TestZLibrarySessionRenewOn401(t *testing.T) {
	srv := newZLibraryTestServer(t, true)
	defer srv.Close()

	cfg := &config.Config{
		ZLibraryURL:      srv.URL,
		ZLibraryEmail:    "u@example.com",
		ZLibraryPassword: "pw",
		ZLibraryEnabled:  true,
		UserAgent:        "test",
	}
	z := NewZLibrary(cfg, &http.Client{Timeout: 5 * time.Second})

	results, err := z.Search(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Search returned error after 401 retry: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results after retry, want 1", len(results))
	}
}

func TestZLibraryDisabled(t *testing.T) {
	cfg := &config.Config{ZLibraryEnabled: true} // missing email/password
	z := NewZLibrary(cfg, &http.Client{})
	if z.Enabled() {
		t.Errorf("Enabled() true without credentials; want false")
	}
}
