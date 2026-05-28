package sources_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/JeremiahM37/librarr/internal/sources"
	"github.com/JeremiahM37/librarr/internal/sources/sourcestest"
)

// TestCanonicalRegistryIsComplete validates that the librarr-sources companion
// registry has every field populated. Fixture test: if the JSON in
// sourcestest/sources.json drifts and starts missing required fields, this
// catches it before it bites callers at runtime.
func TestCanonicalRegistryIsComplete(t *testing.T) {
	r, err := sourcestest.Registry()
	if err != nil {
		t.Fatalf("Registry(): %v", err)
	}
	checks := map[string]bool{
		"Annas.Domain":          r.Annas.Domain == "",
		"LibgenMirrors":         len(r.LibgenMirrors) == 0,
		"AudioBookBay.Mirrors":  len(r.AudioBookBay.Mirrors) == 0,
		"AudioBookBay.Trackers": len(r.AudioBookBay.Trackers) == 0,
		"WebNovels":             len(r.WebNovels) == 0,
		"Gutenberg.URL":         r.Gutenberg.URL == "",
		"MangaDex.APIURL":       r.MangaDex.APIURL == "",
		"MangaDex.UploadsURL":   r.MangaDex.UploadsURL == "",
		"MangaDex.WebURL":       r.MangaDex.WebURL == "",
		"OpenLibrary.SearchURL": r.OpenLibrary.SearchURL == "",
		"OpenLibrary.CoverURL":  r.OpenLibrary.CoverURL == "",
		"Librivox.URL":          r.Librivox.URL == "",
		"StandardEbooks.URL":    r.StandardEbooks.URL == "",
		"Nyaa.URL":              r.Nyaa.URL == "",
		"ThePirateBay.URL":      r.ThePirateBay.URL == "",
		"ZLibraryDefault":       r.ZLibraryDefault == "",
	}
	for field, empty := range checks {
		if empty {
			t.Errorf("canonical registry: %s is empty", field)
		}
	}
}

// good/bad/defaultURL are package-test-level test servers shared across
// TestLoad subtests. Each subtest gets its own tempdir for paths/caches so
// state doesn't bleed.
func loadTestServers(t *testing.T) (good, bad, def *httptest.Server) {
	t.Helper()
	good = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":3,"annas":{"domain":"url-example.test"}}`))
	}))
	bad = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	def = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":9,"annas":{"domain":"default-example.test"}}`))
	}))
	t.Cleanup(good.Close)
	t.Cleanup(bad.Close)
	t.Cleanup(def.Close)
	return
}

func TestLoad_PathWins(t *testing.T) {
	good, _, def := loadTestServers(t)
	t.Cleanup(sources.SwapDefaultRegistryURL(def.URL))
	dir := t.TempDir()
	p := filepath.Join(dir, "g.json")
	_ = os.WriteFile(p, []byte(`{"version":2,"annas":{"domain":"file-example.test"}}`), 0o644)

	r := sources.Load(p, good.URL, filepath.Join(dir, "cache.json"))
	if r.Annas.Domain != "file-example.test" || r.Version != 2 {
		t.Fatalf("got %+v", r.Annas)
	}
}

func TestLoad_UserURLOverridesDefault(t *testing.T) {
	good, _, def := loadTestServers(t)
	t.Cleanup(sources.SwapDefaultRegistryURL(def.URL))
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache.json")

	r := sources.Load("", good.URL, cache)
	if r.Annas.Domain != "url-example.test" || r.Version != 3 {
		t.Fatalf("got %+v", r.Annas)
	}
	// User URL success writes cache.
	if _, err := os.Stat(cache); err != nil {
		t.Errorf("expected cache write: %v", err)
	}
}

func TestLoad_FallsToDefaultURLWhenUserURLBad(t *testing.T) {
	_, bad, def := loadTestServers(t)
	t.Cleanup(sources.SwapDefaultRegistryURL(def.URL))
	dir := t.TempDir()

	r := sources.Load("", bad.URL, filepath.Join(dir, "cache.json"))
	if r.Annas.Domain != "default-example.test" || r.Version != 9 {
		t.Fatalf("got %+v", r.Annas)
	}
}

func TestLoad_FallsToCacheWhenAllURLsBad(t *testing.T) {
	_, bad, _ := loadTestServers(t)
	t.Cleanup(sources.SwapDefaultRegistryURL(bad.URL))
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache.json")
	_ = os.WriteFile(cache, []byte(`{"version":7,"annas":{"domain":"cache-example.test"}}`), 0o644)

	r := sources.Load("", bad.URL, cache)
	if r.Annas.Domain != "cache-example.test" || r.Version != 7 {
		t.Fatalf("got %+v", r.Annas)
	}
}

func TestLoad_DefaultURLWritesCache(t *testing.T) {
	_, _, def := loadTestServers(t)
	t.Cleanup(sources.SwapDefaultRegistryURL(def.URL))
	dir := t.TempDir()
	cache := filepath.Join(dir, "fresh.json")

	_ = sources.Load("", "", cache)
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("expected cache written: %v", err)
	}
}

func TestLoad_AllEmptyReturnsEmptyRegistry(t *testing.T) {
	_, bad, _ := loadTestServers(t)
	t.Cleanup(sources.SwapDefaultRegistryURL(bad.URL))

	r := sources.Load("", "", "")
	if r == nil {
		t.Fatal("Load returned nil")
	}
	if r.Annas.Domain != "" || r.Version != 0 {
		t.Errorf("expected empty registry, got %+v", r)
	}
}

func TestLoad_BrokenFileFallsThroughToDefaultURL(t *testing.T) {
	_, _, def := loadTestServers(t)
	t.Cleanup(sources.SwapDefaultRegistryURL(def.URL))
	dir := t.TempDir()
	broken := filepath.Join(dir, "b.json")
	_ = os.WriteFile(broken, []byte(`{not json`), 0o644)

	r := sources.Load(broken, "", filepath.Join(dir, "cache.json"))
	if r.Annas.Domain != "default-example.test" {
		t.Fatalf("got %+v", r.Annas)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	cases := []struct {
		name    string
		envs    map[string]string
		initial string
		want    string
	}{
		{"unset leaves value", nil, "starting.test", "starting.test"},
		{"ANNAS_ARCHIVE_DOMAIN overrides", map[string]string{"ANNAS_ARCHIVE_DOMAIN": "override.test"}, "starting.test", "override.test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &sources.Registry{Annas: sources.AnnasSpec{Domain: tc.initial}}
			r.ApplyEnvOverrides(func(k string) string { return tc.envs[k] })
			if r.Annas.Domain != tc.want {
				t.Errorf("Annas.Domain = %q, want %q", r.Annas.Domain, tc.want)
			}
		})
	}
}

func TestWebNovelLookup(t *testing.T) {
	r := &sources.Registry{WebNovels: []sources.WebNovelSite{
		{ID: "freewebnovel", URL: "https://example.test/freewebnovel"},
	}}
	if site := r.WebNovel("freewebnovel"); site == nil || site.URL == "" {
		t.Errorf("expected freewebnovel entry with URL, got %+v", site)
	}
	if r.WebNovel("does-not-exist") != nil {
		t.Errorf("expected nil for unknown site id")
	}
}
