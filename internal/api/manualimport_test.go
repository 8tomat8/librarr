package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/JeremiahM37/librarr/internal/config"
	"github.com/JeremiahM37/librarr/internal/db"
	"github.com/JeremiahM37/librarr/internal/search"
)

func manualImportTestServer(t *testing.T, roots map[string]string) *Server {
	t.Helper()
	dir := t.TempDir()

	cfg := &config.Config{
		EbookDir:     roots["ebook"],
		AudiobookDir: roots["audiobook"],
		IncomingDir:  roots["incoming"],
	}

	for k, v := range roots {
		if v == "" {
			continue
		}
		if err := os.MkdirAll(v, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", k, err)
		}
	}

	database, err := db.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	health := search.NewHealthTracker(3, 300)
	searchMgr := search.NewManager(cfg, nil, health)

	return &Server{cfg: cfg, db: database, searchMgr: searchMgr}
}

func TestValidateAllowedPath_RejectsOutsideRoots(t *testing.T) {
	dir := t.TempDir()
	allowed := filepath.Join(dir, "books")
	s := manualImportTestServer(t, map[string]string{"ebook": allowed})

	body, _ := json.Marshal(map[string]string{"path": "/etc"})
	req := httptest.NewRequest(http.MethodPost, "/api/import/scan", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxUserRole, "admin"))
	rr := httptest.NewRecorder()
	s.handleScanImport(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestValidateAllowedPath_AllowsInsideRoot(t *testing.T) {
	dir := t.TempDir()
	allowed := filepath.Join(dir, "books")
	s := manualImportTestServer(t, map[string]string{"ebook": allowed})

	body, _ := json.Marshal(map[string]string{"path": allowed})
	req := httptest.NewRequest(http.MethodPost, "/api/import/scan", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxUserRole, "admin"))
	rr := httptest.NewRecorder()
	s.handleScanImport(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestValidateTestURL_BlocksLocalhost(t *testing.T) {
	cases := []string{
		"http://127.0.0.1:8080",
		"http://localhost/api",
		"http://10.0.0.1/",
	}
	for _, u := range cases {
		if err := validateTestURL(u); err == nil {
			t.Errorf("validateTestURL(%q) should fail", u)
		}
	}
}

func TestValidateTestURL_AllowsPublicHost(t *testing.T) {
	if err := validateTestURL("https://prowlarr.example:9696"); err != nil {
		t.Errorf("expected public host to pass, got %v", err)
	}
}
