package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JeremiahM37/librarr/internal/config"
	"github.com/JeremiahM37/librarr/internal/db"
)

func TestSessionStore_CreateAndGet(t *testing.T) {
	store := NewSessionStore()

	token, err := store.Create(1, "testuser", "admin")
	if err != nil || token == "" {
		t.Fatal("expected non-empty token")
	}

	data, ok := store.Get(token)
	if !ok {
		t.Fatal("expected session to be valid")
	}
	if data.UserID != 1 {
		t.Errorf("expected user ID 1, got %d", data.UserID)
	}
	if data.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", data.Username)
	}
	if data.Role != "admin" {
		t.Errorf("expected role admin, got %s", data.Role)
	}
}

func TestSessionStore_Valid(t *testing.T) {
	store := NewSessionStore()
	token, err := store.Create(1, "user", "admin")
	if err != nil {
		t.Fatal(err)
	}

	if !store.Valid(token) {
		t.Error("expected token to be valid")
	}

	if store.Valid("nonexistent-token") {
		t.Error("expected nonexistent token to be invalid")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	store := NewSessionStore()
	token, err := store.Create(1, "user", "admin")
	if err != nil {
		t.Fatal(err)
	}

	store.Delete(token)
	if store.Valid(token) {
		t.Error("expected deleted token to be invalid")
	}
}

func TestSessionStore_Expiry(t *testing.T) {
	store := NewSessionStore()
	token, err := store.Create(1, "user", "admin")
	if err != nil {
		t.Fatal(err)
	}

	// Manually expire the session
	store.mu.Lock()
	store.sessions[token].Expiry = time.Now().Add(-1 * time.Hour)
	store.mu.Unlock()

	if store.Valid(token) {
		t.Error("expected expired token to be invalid")
	}

	// Should also be cleaned up from the store
	store.mu.RLock()
	_, exists := store.sessions[token]
	store.mu.RUnlock()
	if exists {
		t.Error("expected expired session to be deleted from store")
	}
}

func TestSessionStore_PendingTOTP(t *testing.T) {
	store := NewSessionStore()

	t.Run("create and validate", func(t *testing.T) {
		token, err := store.CreatePendingTOTP(42)
		if err != nil || token == "" {
			t.Fatal("expected non-empty pending token")
		}

		userID, valid := store.ValidatePendingTOTP(token)
		if !valid {
			t.Error("expected pending TOTP to be valid")
		}
		if userID != 42 {
			t.Errorf("expected user ID 42, got %d", userID)
		}
	})

	t.Run("consumed after first use", func(t *testing.T) {
		token, err := store.CreatePendingTOTP(1)
		if err != nil {
			t.Fatal(err)
		}
		store.ValidatePendingTOTP(token)

		_, valid := store.ValidatePendingTOTP(token)
		if valid {
			t.Error("expected pending TOTP to be consumed after use")
		}
	})

	t.Run("expired pending TOTP", func(t *testing.T) {
		token, err := store.CreatePendingTOTP(1)
		if err != nil {
			t.Fatal(err)
		}

		store.mu.Lock()
		store.pendingTOTP[token].Expiry = time.Now().Add(-1 * time.Minute)
		store.mu.Unlock()

		_, valid := store.ValidatePendingTOTP(token)
		if valid {
			t.Error("expected expired pending TOTP to be invalid")
		}
	})

	t.Run("nonexistent token", func(t *testing.T) {
		_, valid := store.ValidatePendingTOTP("nonexistent")
		if valid {
			t.Error("expected nonexistent token to be invalid")
		}
	})
}

func TestSessionStore_UniqueTokens(t *testing.T) {
	store := NewSessionStore()
	tokens := make(map[string]bool)

	for i := 0; i < 100; i++ {
		token, err := store.Create(int64(i), "user", "admin")
		if err != nil {
			t.Fatal(err)
		}
		if tokens[token] {
			t.Fatalf("duplicate token generated: %s", token)
		}
		tokens[token] = true
	}
}

func TestHashPassword_And_CheckPassword(t *testing.T) {
	password := "mysecretpassword"
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatalf("hashPassword failed: %v", err)
	}

	if !checkPassword(password, hash) {
		t.Error("expected correct password to match")
	}

	if checkPassword("wrongpassword", hash) {
		t.Error("expected wrong password to not match")
	}

	if checkPassword("", hash) {
		t.Error("expected empty password to not match")
	}
}

func TestHashBackupCode(t *testing.T) {
	code := "12345678"
	hash1 := hashBackupCode(code)
	hash2 := hashBackupCode(code)

	if hash1 != hash2 {
		t.Error("expected same hash for same code")
	}

	hash3 := hashBackupCode("87654321")
	if hash1 == hash3 {
		t.Error("expected different hash for different code")
	}

	if len(hash1) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(hash1))
	}
}

func TestIsExempt(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/", true},
		{"/health", true},
		{"/api/health", true},
		{"/api/login", true},
		{"/api/login/totp", true},
		{"/api/register", true},
		{"/api/auth/status", true},
		{"/readyz", true},
		{"/torznab/api", true},
		{"/torznab/api?t=caps", true},
		{"/static/style.css", true},
		{"/opds", true},
		{"/opds/", true},
		{"/opds/opensearch.xml", true},
		{"/opds/books", false},
		{"/opds/search", false},
		{"/opds/download/1", false},
		{"/metrics", true},
		{"/auth/oidc/callback", true},
		// OpenAPI spec is public so AI agents / tooling can introspect the
		// API without prior auth (same precedent as /metrics and /health).
		{"/api/openapi.json", true},
		{"/api/search", false},
		{"/api/download", false},
		{"/api/library", false},
		{"/api/users", false},
		// Suffix-attack guard — only the exact path should be exempt.
		{"/api/openapi.jsonx", false},
		// Prowlarr's indexer-discovery probe hits bare /api — must be exempt
		// because the Torznab handler does its own apikey check. But the
		// exemption MUST be exact-path only; any /api/... JSON endpoint
		// must still require auth.
		{"/api", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isExempt(tt.path)
			if result != tt.expected {
				t.Errorf("isExempt(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestHandleAuthStatus_OIDCHints(t *testing.T) {
	// /api/auth/status is the canonical pre-auth endpoint the login modal
	// hits to decide whether to render the SSO button. /api/config is gated
	// behind the multi-user middleware once any user exists, so the modal
	// MUST be able to read the OIDC hint here or the button silently
	// disappears after the first user registers.
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	sessions := NewSessionStore()

	tests := []struct {
		name             string
		cfg              *config.Config
		wantEnabled      bool
		wantProviderName string
	}{
		{
			name:        "nil config",
			cfg:         nil,
			wantEnabled: false,
		},
		{
			name:        "OIDC disabled",
			cfg:         &config.Config{OIDCEnabled: false},
			wantEnabled: false,
		},
		{
			name: "OIDC partially configured (no client secret)",
			cfg: &config.Config{
				OIDCEnabled:      true,
				OIDCIssuer:       "https://idp.example.com",
				OIDCClientID:     "client",
				OIDCProviderName: "Ignored",
			},
			wantEnabled: false,
		},
		{
			name: "OIDC fully configured",
			cfg: &config.Config{
				OIDCEnabled:      true,
				OIDCIssuer:       "https://idp.example.com",
				OIDCClientID:     "client",
				OIDCClientSecret: "secret",
				OIDCProviderName: "PocketID",
			},
			wantEnabled:      true,
			wantProviderName: "PocketID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
			rr := httptest.NewRecorder()
			handleAuthStatus(tt.cfg, database, sessions)(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
			}
			var resp map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			enabled, _ := resp["oidc_enabled"].(bool)
			if enabled != tt.wantEnabled {
				t.Errorf("oidc_enabled = %v, want %v", enabled, tt.wantEnabled)
			}
			if tt.wantEnabled {
				if got, _ := resp["oidc_provider_name"].(string); got != tt.wantProviderName {
					t.Errorf("oidc_provider_name = %q, want %q", got, tt.wantProviderName)
				}
				// MUST NOT leak the client secret on the public endpoint.
				for k, v := range resp {
					if s, ok := v.(string); ok && s == "secret" {
						t.Errorf("response leaks secret-like value at key %q", k)
					}
				}
			} else if _, present := resp["oidc_provider_name"]; present {
				t.Errorf("oidc_provider_name MUST be absent when OIDC is disabled, got %v", resp["oidc_provider_name"])
			}
		})
	}
}

func TestAuthMiddleware_NoAuthGrantsAdmin(t *testing.T) {
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{}
	sessions := NewSessionStore()

	var gotRole string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRole, _ = r.Context().Value(ctxUserRole).(string)
		w.WriteHeader(http.StatusOK)
	})

	handler := authMiddleware(cfg, database, sessions, requireAdmin(inner))

	req := httptest.NewRequest(http.MethodPost, "/api/settings", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if gotRole != "admin" {
		t.Errorf("expected admin role in context, got %q", gotRole)
	}
}
