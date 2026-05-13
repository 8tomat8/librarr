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

// settingsTestServer builds the minimum Server needed for settings handler
// tests: an on-disk SettingsFile, a real in-memory DB (LogActivity needs one),
// and a search.Manager (ForeignLangFilterEnabled is read on GET).
func settingsTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	cfg := &config.Config{
		SettingsFile: settingsPath,
		// Seed the env-layer values that the handler injects as defaults.
		ProwlarrURL:    "http://env-prowlarr:9696",
		ProwlarrAPIKey: "ENV_API_KEY",
		QBUrl:          "http://env-qbit:8080",
		QBUser:         "admin",
		QBPass:         "env-qb-pass",
	}

	database, err := db.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	health := search.NewHealthTracker(3, 300)
	searchMgr := search.NewManager(cfg, nil, health)

	return &Server{cfg: cfg, db: database, searchMgr: searchMgr}, settingsPath
}

func saveSettings(t *testing.T, s *Server, payload map[string]interface{}) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxUsername, "admin"))
	rr := httptest.NewRecorder()
	s.handleSaveSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("save returned %d: %s", rr.Code, rr.Body.String())
	}
}

func readSettingsFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		// Treat missing file as empty — that's the post-clear state.
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.json malformed: %v", err)
	}
	return m
}

// TestSaveSettings_EmptyStringDeletesKey — regression guard for the bug found
// in E2E. Clearing a UI field must remove the override from settings.json so
// the env value reapplies; persisting "" would make /api/settings disagree
// with the runtime cfg.
func TestSaveSettings_EmptyStringDeletesKey(t *testing.T) {
	s, path := settingsTestServer(t)

	saveSettings(t, s, map[string]interface{}{"prowlarr_url": "http://custom:9696"})
	if got := readSettingsFile(t, path)["prowlarr_url"]; got != "http://custom:9696" {
		t.Fatalf("setup: expected URL persisted, got %v", got)
	}

	saveSettings(t, s, map[string]interface{}{"prowlarr_url": ""})

	stored := readSettingsFile(t, path)
	if _, present := stored["prowlarr_url"]; present {
		t.Errorf("empty-string save should delete the key, but it is still present: %v", stored)
	}
}

// TestSaveSettings_FalseBoolPersists — the empty-string-delete rule must NOT
// drop legitimate falsy values like bool false. Toggles depend on this.
func TestSaveSettings_FalseBoolPersists(t *testing.T) {
	s, path := settingsTestServer(t)

	saveSettings(t, s, map[string]interface{}{"remove_torrent_after_import": false})

	stored := readSettingsFile(t, path)
	v, present := stored["remove_torrent_after_import"]
	if !present {
		t.Fatal("bool false should be persisted, but key was deleted")
	}
	if b, ok := v.(bool); !ok || b != false {
		t.Errorf("expected false, got %v (%T)", v, v)
	}
}

// TestSaveSettings_MaskedSentinelPreservesRealValue — when a user saves a form
// without touching a sensitive field, the JS sends back the masked sentinel
// "--------". The handler must drop that key entirely so the previously-saved
// real value remains on disk. Without this, a single UI save could wipe every
// API key in settings.json.
func TestSaveSettings_MaskedSentinelPreservesRealValue(t *testing.T) {
	s, path := settingsTestServer(t)

	saveSettings(t, s, map[string]interface{}{
		"prowlarr_url":     "http://saved:9696",
		"prowlarr_api_key": "REAL_SECRET_KEY",
	})

	// User edits only the URL and submits the form; API key field still holds
	// the "--------" mask the GET handler returned.
	saveSettings(t, s, map[string]interface{}{
		"prowlarr_url":     "http://updated:9696",
		"prowlarr_api_key": maskedValue,
	})

	stored := readSettingsFile(t, path)
	if got := stored["prowlarr_url"]; got != "http://updated:9696" {
		t.Errorf("URL should have updated, got %v", got)
	}
	if got := stored["prowlarr_api_key"]; got != "REAL_SECRET_KEY" {
		t.Errorf("real API key should have been preserved, got %v", got)
	}
}

// TestGetSettings_MasksSensitiveValues — non-empty sensitive values must come
// back as the sentinel, never as plaintext. Empty values stay empty so the UI
// can distinguish "unset" from "set but hidden".
func TestGetSettings_MasksSensitiveValues(t *testing.T) {
	s, _ := settingsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET returned %d", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Prowlarr API key is set in env, must be masked.
	if got := resp["prowlarr_api_key"]; got != maskedValue {
		t.Errorf("prowlarr_api_key should be masked, got %v", got)
	}
	if got := resp["qb_pass"]; got != maskedValue {
		t.Errorf("qb_pass should be masked, got %v", got)
	}
	// Non-sensitive URL is exposed.
	if got := resp["prowlarr_url"]; got != "http://env-prowlarr:9696" {
		t.Errorf("URL should be exposed, got %v", got)
	}
	// Unset sensitive (no env, no file) stays empty — empty is distinguishable
	// from masked so the UI can show a placeholder.
	if got := resp["abs_token"]; got != "" {
		t.Errorf("unset abs_token should be empty string, got %v", got)
	}
}
