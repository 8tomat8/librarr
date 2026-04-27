package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleListInvites(w http.ResponseWriter, _ *http.Request) {
	codes, err := s.db.ListInviteCodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": err.Error(),
		})
		return
	}
	if codes == nil {
		codes = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"invites": codes})
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role       string `json:"role"`
		MaxUses    int    `json:"max_uses"`
		ExpiresIn  int    `json:"expires_in"` // seconds from now; 0 = no expiry
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Invalid request",
		})
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role != "user" && req.Role != "admin" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Role must be 'user' or 'admin'",
		})
		return
	}
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}

	// Generate a secure random code.
	codeBytes := make([]byte, 16)
	if _, err := rand.Read(codeBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": "Failed to generate code",
		})
		return
	}
	code := hex.EncodeToString(codeBytes)

	var expiresAt *float64
	if req.ExpiresIn > 0 {
		t := float64(time.Now().Unix()) + float64(req.ExpiresIn)
		expiresAt = &t
	}

	// Get admin user ID from context.
	userID := int64(0)
	if id, ok := r.Context().Value(ctxUserID).(int64); ok {
		userID = id
	}

	id, err := s.db.CreateInviteCode(code, userID, req.Role, req.MaxUses, expiresAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"id":      id,
		"code":    code,
		"role":    req.Role,
		"max_uses": req.MaxUses,
	})
}

func (s *Server) handleDeleteInvite(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Invalid ID",
		})
		return
	}
	if err := s.db.DeleteInviteCode(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}
