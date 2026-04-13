package download

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JeremiahM37/librarr/internal/config"
)

// Helper: swap libgenMirrors for a test and restore afterwards.
func withMirrors(t *testing.T, mirrors []string) {
	t.Helper()
	orig := libgenMirrors
	libgenMirrors = mirrors
	t.Cleanup(func() { libgenMirrors = orig })
}

// TestFetchLibgenDownloadURL_FirstMirrorWorks is the happy path.
func TestFetchLibgenDownloadURL_FirstMirrorWorks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><a href="get.php?md5=abc123&key=XYZ">GET</a></html>`))
	}))
	defer server.Close()

	withMirrors(t, []string{server.URL})
	cfg := &config.Config{UserAgent: "test"}
	d := NewDirectDownloader(cfg, server.Client())

	url, err := d.fetchLibgenDownloadURL("abc123", nil)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(url, "get.php?md5=abc123") {
		t.Errorf("unexpected URL: %s", url)
	}
}

// TestFetchLibgenDownloadURL_FailsOverOn500 — issue #7 regression test.
// First mirror returns HTTP 500, second mirror succeeds.
func TestFetchLibgenDownloadURL_FailsOverOn500(t *testing.T) {
	brokenCalls := int32(0)
	workingCalls := int32(0)

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&brokenCalls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer broken.Close()

	working := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&workingCalls, 1)
		_, _ = w.Write([]byte(`<a href="get.php?md5=abc&key=ZZZ">GET</a>`))
	}))
	defer working.Close()

	withMirrors(t, []string{broken.URL, working.URL})
	cfg := &config.Config{UserAgent: "test"}
	d := NewDirectDownloader(cfg, working.Client())

	url, err := d.fetchLibgenDownloadURL("abc", nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	if !strings.HasPrefix(url, working.URL+"/") {
		t.Errorf("URL should be from working mirror, got: %s", url)
	}
	if atomic.LoadInt32(&brokenCalls) != 1 {
		t.Errorf("broken mirror should be tried once, got %d", brokenCalls)
	}
	if atomic.LoadInt32(&workingCalls) != 1 {
		t.Errorf("working mirror should be tried once, got %d", workingCalls)
	}
}

// TestFetchLibgenDownloadURL_AllMirrorsFail — all mirrors down.
func TestFetchLibgenDownloadURL_AllMirrorsFail(t *testing.T) {
	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer m1.Close()
	m2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer m2.Close()

	withMirrors(t, []string{m1.URL, m2.URL})
	cfg := &config.Config{UserAgent: "test"}
	d := NewDirectDownloader(cfg, m1.Client())

	_, err := d.fetchLibgenDownloadURL("abc", nil)
	if err == nil {
		t.Fatal("expected error when all mirrors fail, got nil")
	}
	// Should report the LAST mirror's error
	if !strings.Contains(err.Error(), "HTTP") {
		t.Errorf("error should mention HTTP status: %v", err)
	}
}

// TestFetchLibgenDownloadURL_MirrorLacksBook — some mirrors don't have the MD5.
func TestFetchLibgenDownloadURL_MirrorLacksBook(t *testing.T) {
	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Returns 200 but no get.php link (book not present on this mirror)
		_, _ = w.Write([]byte(`<html><body>File not found</body></html>`))
	}))
	defer m1.Close()
	m2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="get.php?md5=xyz&key=ABC">GET</a>`))
	}))
	defer m2.Close()

	withMirrors(t, []string{m1.URL, m2.URL})
	cfg := &config.Config{UserAgent: "test"}
	d := NewDirectDownloader(cfg, m1.Client())

	url, err := d.fetchLibgenDownloadURL("xyz", nil)
	if err != nil {
		t.Fatalf("expected fallback to mirror with the book, got: %v", err)
	}
	if !strings.HasPrefix(url, m2.URL+"/") {
		t.Errorf("URL should be from m2, got: %s", url)
	}
}

// TestFetchLibgenDownloadURL_NetworkErrorFailsOver — connection refused on one mirror.
func TestFetchLibgenDownloadURL_NetworkErrorFailsOver(t *testing.T) {
	working := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="get.php?md5=abc&key=XXX">GET</a>`))
	}))
	defer working.Close()

	// Point to a port that's closed
	withMirrors(t, []string{"http://127.0.0.1:1", working.URL})
	cfg := &config.Config{UserAgent: "test"}
	d := NewDirectDownloader(cfg, working.Client())

	url, err := d.fetchLibgenDownloadURL("abc", nil)
	if err != nil {
		t.Fatalf("expected fallback on network error, got: %v", err)
	}
	if !strings.HasPrefix(url, working.URL+"/") {
		t.Errorf("URL should be from working mirror, got: %s", url)
	}
}

// TestFetchLibgenDownloadURL_ProgressCallback verifies progress updates.
func TestFetchLibgenDownloadURL_ProgressCallback(t *testing.T) {
	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer m1.Close()
	m2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="get.php?md5=a&key=B">GET</a>`))
	}))
	defer m2.Close()

	withMirrors(t, []string{m1.URL, m2.URL})
	cfg := &config.Config{UserAgent: "test"}
	d := NewDirectDownloader(cfg, m1.Client())

	var messages []string
	_, err := d.fetchLibgenDownloadURL("a", func(msg string) {
		messages = append(messages, msg)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(messages) == 0 {
		t.Error("expected progress messages, got none")
	}
	found := false
	for _, m := range messages {
		if strings.Contains(m, "Trying mirror") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Trying mirror' message, got: %v", messages)
	}
}

// TestLibgenMirrors_Configured ensures we have multiple mirrors so a single
// mirror outage doesn't break downloads entirely.
func TestLibgenMirrors_Configured(t *testing.T) {
	if len(libgenMirrors) < 3 {
		t.Errorf("should have at least 3 libgen mirrors for redundancy, got %d", len(libgenMirrors))
	}
	for _, m := range libgenMirrors {
		if !strings.HasPrefix(m, "https://libgen.") {
			t.Errorf("unexpected mirror URL: %s", m)
		}
	}
}

// Avoid unused-import linter errors when only some tests are built
var _ = fmt.Sprintf
