package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/JeremiahM37/librarr/internal/config"
	"github.com/JeremiahM37/librarr/internal/db"
	"github.com/JeremiahM37/librarr/internal/models"
)

const (
	sessionCookieName = "librarr_session"
)

var authentikIdentityHeaders = []string{
	"X-Authentik-Username",
	"X-Authentik-Email",
	"X-Authentik-Name",
	"X-Authentik-Uid",
	"Remote-User",
	"X-Forwarded-User",
}

func proxyIdentityFromRequest(r *http.Request) string {
	for _, header := range authentikIdentityHeaders {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}
	return ""
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if strings.EqualFold(proto, "https") {
		return true
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Forwarded")), "proto=https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// resolveOIDCUser returns an existing Librarr user for the supplied SSO
// identity, or auto-creates one when OIDC user provisioning is enabled.
func resolveOIDCUser(cfg *config.Config, database *db.DB, username string) (*models.User, error) {
	if cfg == nil || !cfg.HasOIDC() {
		return nil, nil
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("empty SSO username")
	}

	user, err := database.GetUserByUsername(username)
	if err == nil {
		return user, nil
	}

	if !cfg.OIDCAutoCreateUsers {
		return nil, fmt.Errorf("user %q not found and auto-creation is disabled", username)
	}

	userCount, _ := database.CountUsers()
	role := cfg.OIDCDefaultRole
	if userCount == 0 {
		role = "admin"
	}

	randomPass := make([]byte, 32)
	if _, err := rand.Read(randomPass); err != nil {
		return nil, fmt.Errorf("generate random password seed: %w", err)
	}
	passHash, err := hashPassword(hex.EncodeToString(randomPass))
	if err != nil {
		return nil, fmt.Errorf("hash random password: %w", err)
	}

	id, err := database.CreateUser(username, passHash, role)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	user, err = database.GetUser(id)
	if err != nil {
		return nil, fmt.Errorf("load created user: %w", err)
	}

	slog.Info("OIDC user created", "id", id, "username", username, "role", role)

	return user, nil
}

// ensureSessionForUser creates a Librarr session cookie for the given user if
// the request does not already carry a matching valid session.
func ensureSessionForUser(w http.ResponseWriter, r *http.Request, sessions *SessionStore, user *models.User) bool {
	if sessions == nil || user == nil {
		return false
	}

	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if data, ok := sessions.Get(cookie.Value); ok && data.UserID == user.ID && data.Username == user.Username && data.Role == user.Role {
			return false
		}
	}

	token := sessions.Create(user.ID, user.Username, user.Role)
	setSessionCookie(w, r, token, 86400)
	return true
}
