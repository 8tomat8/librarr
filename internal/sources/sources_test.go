package sources

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDefault_DecodesEmbedded(t *testing.T) {
	r, err := Default()
	if err != nil {
		t.Fatalf("Default() returned error: %v", err)
	}
	if r.Annas.Domain == "" {
		t.Error("Annas.Domain is empty in embedded defaults")
	}
	if len(r.LibgenMirrors) == 0 {
		t.Error("LibgenMirrors is empty in embedded defaults")
	}
	if len(r.AudioBookBay.Mirrors) == 0 {
		t.Error("AudioBookBay.Mirrors is empty in embedded defaults")
	}
	if len(r.AudioBookBay.Trackers) == 0 {
		t.Error("AudioBookBay.Trackers is empty in embedded defaults")
	}
	if len(r.WebNovels) == 0 {
		t.Error("WebNovels is empty in embedded defaults")
	}
	if r.Gutenberg.URL == "" {
		t.Error("Gutenberg.URL is empty in embedded defaults")
	}
	if r.MangaDex.APIURL == "" || r.MangaDex.UploadsURL == "" || r.MangaDex.WebURL == "" {
		t.Error("MangaDex spec missing one or more URLs in embedded defaults")
	}
}

func TestLoad_FallsBackToEmbedded_WhenPathAndURLEmpty(t *testing.T) {
	r := Load("", "")
	if r == nil {
		t.Fatal("Load returned nil")
	}
	if r.Annas.Domain == "" {
		t.Error("expected embedded defaults to populate Annas.Domain")
	}
}

func TestLoad_FromFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "sources.json")
	custom := `{"version": 2, "annas": {"domain": "custom-example.test"}, "libgen_mirrors": ["https://example.test"]}`
	if err := os.WriteFile(tmp, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Load(tmp, "")
	if r.Version != 2 {
		t.Errorf("Version = %d, want 2", r.Version)
	}
	if r.Annas.Domain != "custom-example.test" {
		t.Errorf("Annas.Domain = %q, want custom-example.test", r.Annas.Domain)
	}
	if len(r.LibgenMirrors) != 1 || r.LibgenMirrors[0] != "https://example.test" {
		t.Errorf("LibgenMirrors = %v, want [https://example.test]", r.LibgenMirrors)
	}
}

func TestLoad_FromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version": 3, "annas": {"domain": "url-example.test"}}`))
	}))
	defer srv.Close()

	r := Load("", srv.URL)
	if r.Annas.Domain != "url-example.test" {
		t.Errorf("Annas.Domain = %q, want url-example.test", r.Annas.Domain)
	}
}

func TestLoad_PathTakesPrecedenceOverURL(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(tmp, []byte(`{"annas": {"domain": "file-wins.test"}}`), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"annas": {"domain": "url-loses.test"}}`))
	}))
	defer srv.Close()

	r := Load(tmp, srv.URL)
	if r.Annas.Domain != "file-wins.test" {
		t.Errorf("path should win over URL: got %q", r.Annas.Domain)
	}
}

func TestLoad_FallsBackOnBadURL(t *testing.T) {
	// URL returns 500 — loader should silently fall back to embedded defaults.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	r := Load("", srv.URL)
	if r == nil {
		t.Fatal("Load returned nil; expected fallback to embedded defaults")
	}
	// Embedded default has a non-empty Annas.Domain — confirm fallback succeeded.
	if r.Annas.Domain == "" {
		t.Error("expected embedded defaults after URL fallback")
	}
}

func TestLoad_FallsBackOnBadFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "broken.json")
	os.WriteFile(tmp, []byte(`{not json`), 0o644)
	r := Load(tmp, "")
	if r.Annas.Domain == "" {
		t.Error("expected embedded defaults when file is malformed")
	}
}

func TestApplyEnvOverrides_AnnasDomain(t *testing.T) {
	r, _ := Default()
	envs := map[string]string{"ANNAS_ARCHIVE_DOMAIN": "override.test"}
	r.ApplyEnvOverrides(func(k string) string { return envs[k] })
	if r.Annas.Domain != "override.test" {
		t.Errorf("Annas.Domain = %q, want override.test", r.Annas.Domain)
	}
}

func TestApplyEnvOverrides_UnsetLeavesValueAlone(t *testing.T) {
	r, _ := Default()
	original := r.Annas.Domain
	r.ApplyEnvOverrides(func(string) string { return "" })
	if r.Annas.Domain != original {
		t.Errorf("Annas.Domain mutated when env var unset: got %q, want %q", r.Annas.Domain, original)
	}
}

func TestWebNovel_Lookup(t *testing.T) {
	r, _ := Default()
	site := r.WebNovel("freewebnovel")
	if site == nil {
		t.Fatal("expected freewebnovel site to be present")
	}
	if site.URL == "" {
		t.Error("freewebnovel URL empty")
	}
	if r.WebNovel("does-not-exist") != nil {
		t.Error("expected nil for unknown site id")
	}
}
