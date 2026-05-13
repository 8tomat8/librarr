package sources

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDefault_EmbeddedRegistryIsComplete(t *testing.T) {
	r, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	checks := map[string]bool{
		"Annas.Domain":              r.Annas.Domain == "",
		"LibgenMirrors":             len(r.LibgenMirrors) == 0,
		"AudioBookBay.Mirrors":      len(r.AudioBookBay.Mirrors) == 0,
		"AudioBookBay.Trackers":     len(r.AudioBookBay.Trackers) == 0,
		"WebNovels":                 len(r.WebNovels) == 0,
		"Gutenberg.URL":             r.Gutenberg.URL == "",
		"MangaDex.APIURL":           r.MangaDex.APIURL == "",
		"MangaDex.UploadsURL":       r.MangaDex.UploadsURL == "",
		"MangaDex.WebURL":           r.MangaDex.WebURL == "",
		"OpenLibrary.SearchURL":     r.OpenLibrary.SearchURL == "",
		"OpenLibrary.CoverURL":      r.OpenLibrary.CoverURL == "",
		"Librivox.URL":              r.Librivox.URL == "",
		"StandardEbooks.URL":        r.StandardEbooks.URL == "",
		"Nyaa.URL":                  r.Nyaa.URL == "",
		"ThePirateBay.URL":          r.ThePirateBay.URL == "",
		"ZLibraryDefault":           r.ZLibraryDefault == "",
	}
	for field, empty := range checks {
		if empty {
			t.Errorf("embedded registry: %s is empty", field)
		}
	}
}

// TestLoad covers every resolution path: empty -> embedded, file, URL,
// precedence, file-corrupt fallback, URL-500 fallback.
func TestLoad(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":3,"annas":{"domain":"url-example.test"}}`))
	}))
	defer good.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()

	dir := t.TempDir()
	goodFile := filepath.Join(dir, "good.json")
	_ = os.WriteFile(goodFile, []byte(`{"version":2,"annas":{"domain":"file-example.test"},"libgen_mirrors":["https://example.test"]}`), 0o644)
	brokenFile := filepath.Join(dir, "broken.json")
	_ = os.WriteFile(brokenFile, []byte(`{not json`), 0o644)

	cases := []struct {
		name        string
		path, url   string
		wantDomain  string // "" means "embedded default"
		wantVersion int    // 0 means "don't care / embedded"
	}{
		{"empty -> embedded", "", "", "", 0},
		{"file overrides", goodFile, "", "file-example.test", 2},
		{"url overrides", "", good.URL, "url-example.test", 3},
		{"path beats url", goodFile, good.URL, "file-example.test", 2},
		{"bad url falls back to embedded", "", bad.URL, "", 0},
		{"bad file falls back to embedded", brokenFile, "", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Load(tc.path, tc.url)
			if r == nil {
				t.Fatal("Load returned nil")
			}
			if tc.wantDomain == "" {
				// Embedded fallback expected — confirm something non-empty.
				if r.Annas.Domain == "" {
					t.Errorf("expected embedded Annas.Domain, got empty")
				}
				return
			}
			if r.Annas.Domain != tc.wantDomain {
				t.Errorf("Annas.Domain = %q, want %q", r.Annas.Domain, tc.wantDomain)
			}
			if tc.wantVersion > 0 && r.Version != tc.wantVersion {
				t.Errorf("Version = %d, want %d", r.Version, tc.wantVersion)
			}
		})
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	cases := []struct {
		name string
		envs map[string]string
		want string // "" => unchanged from embedded default
	}{
		{"unset leaves value", nil, ""},
		{"ANNAS_ARCHIVE_DOMAIN overrides", map[string]string{"ANNAS_ARCHIVE_DOMAIN": "override.test"}, "override.test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := Default()
			original := r.Annas.Domain
			r.ApplyEnvOverrides(func(k string) string { return tc.envs[k] })
			want := tc.want
			if want == "" {
				want = original
			}
			if r.Annas.Domain != want {
				t.Errorf("Annas.Domain = %q, want %q", r.Annas.Domain, want)
			}
		})
	}
}

func TestWebNovelLookup(t *testing.T) {
	r, _ := Default()
	if site := r.WebNovel("freewebnovel"); site == nil || site.URL == "" {
		t.Errorf("expected freewebnovel entry with URL, got %+v", site)
	}
	if r.WebNovel("does-not-exist") != nil {
		t.Errorf("expected nil for unknown site id")
	}
}
