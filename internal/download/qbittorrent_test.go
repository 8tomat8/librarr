package download

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JeremiahM37/librarr/internal/config"
)

func newTestQBClient(serverURL string) *QBittorrentClient {
	cfg := &config.Config{
		QBUrl:  serverURL,
		QBUser: "admin",
		QBPass: "adminadmin",
	}
	return NewQBittorrentClient(cfg)
}

// Simulates qBittorrent 4.x: HTTP 200 with body "Ok." and a session cookie.
func TestLogin_QB4x_OkBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "session4x"})
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ok."))
	}))
	defer srv.Close()

	q := newTestQBClient(srv.URL)
	if err := q.Login(); err != nil {
		t.Fatalf("expected qBittorrent 4.x login to succeed, got error: %v", err)
	}
	if !q.authenticated {
		t.Errorf("expected authenticated=true after successful 4.x login")
	}
}

// Simulates qBittorrent 5.x: HTTP 204 No Content with empty body and a QBT_SID_* cookie.
func TestLogin_QB5x_NoContentWithSessionCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "QBT_SID_8080", Value: "session5x"})
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	q := newTestQBClient(srv.URL)
	if err := q.Login(); err != nil {
		t.Fatalf("expected qBittorrent 5.x login to succeed, got error: %v", err)
	}
	if !q.authenticated {
		t.Errorf("expected authenticated=true after successful 5.x login")
	}
	// Session cookie should be retained for subsequent requests.
	found := false
	for _, c := range q.cookies {
		if c.Name == "QBT_SID_8080" && c.Value == "session5x" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected QBT_SID_8080 session cookie to be stored")
	}
}

// Some 5.x deployments may return 200 OK with empty body but the session cookie set.
func TestLogin_QB5x_EmptyBodyWithSessionCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "QBT_SID_8080", Value: "session5x"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	q := newTestQBClient(srv.URL)
	if err := q.Login(); err != nil {
		t.Fatalf("expected login with empty body + cookie to succeed, got error: %v", err)
	}
	if !q.authenticated {
		t.Errorf("expected authenticated=true")
	}
}

// Wrong credentials in 4.x: HTTP 200 with body "Fails." and no session cookie.
func TestLogin_QB4x_FailsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	q := newTestQBClient(srv.URL)
	err := q.Login()
	if err == nil {
		t.Fatalf("expected login to fail when body is 'Fails.'")
	}
	if !strings.Contains(err.Error(), "Fails.") {
		t.Errorf("expected error to include 'Fails.', got: %v", err)
	}
	if q.authenticated {
		t.Errorf("expected authenticated=false after failed login")
	}
}

// IP ban response should be detected from body text.
func TestLogin_Banned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Your IP address has been banned"))
	}))
	defer srv.Close()

	q := newTestQBClient(srv.URL)
	err := q.Login()
	if err == nil || !strings.Contains(err.Error(), "banned") {
		t.Fatalf("expected ban error, got: %v", err)
	}
	if q.authenticated {
		t.Errorf("expected authenticated=false when banned")
	}
}

// Empty body without a session cookie must fail (not be mistaken for 5.x success).
func TestLogin_EmptyBodyNoCookieFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	q := newTestQBClient(srv.URL)
	err := q.Login()
	if err == nil {
		t.Fatalf("expected login failure on 403 + empty body + no cookie")
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("expected error to mention HTTP 403, got: %v", err)
	}
	if q.authenticated {
		t.Errorf("expected authenticated=false")
	}
}

func TestMapTorrentStatus(t *testing.T) {
	tests := []struct {
		state    string
		expected string
	}{
		{"downloading", "downloading"},
		{"stalledDL", "downloading"},
		{"metaDL", "downloading"},
		{"forcedDL", "downloading"},
		{"pausedDL", "paused"},
		{"queuedDL", "queued"},
		{"uploading", "completed"},
		{"stalledUP", "completed"},
		{"pausedUP", "completed"},
		{"queuedUP", "completed"},
		{"stoppedUP", "completed"},
		{"checkingDL", "checking"},
		{"checkingUP", "checking"},
		{"error", "error"},
		{"missingFiles", "missingFiles"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			result := MapTorrentStatus(tt.state)
			if result != tt.expected {
				t.Errorf("MapTorrentStatus(%q) = %q, want %q", tt.state, result, tt.expected)
			}
		})
	}
}

func TestMapSABStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"Downloading", "downloading"},
		{"Paused", "paused"},
		{"Queued", "queued"},
		{"Completed", "completed"},
		{"downloading", "downloading"},
		{"SomeOtherStatus", "SomeOtherStatus"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := mapSABStatus(tt.status)
			if result != tt.expected {
				t.Errorf("mapSABStatus(%q) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestValidTransitions(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		allowed bool
	}{
		{"queued", "searching", true},
		{"queued", "downloading", true},
		{"queued", "error", true},
		{"queued", "completed", false},
		{"searching", "downloading", true},
		{"searching", "queued", true},
		{"downloading", "importing", true},
		{"downloading", "completed", true},
		{"downloading", "error", true},
		{"downloading", "retry_wait", true},
		{"downloading", "queued", false},
		{"importing", "completed", true},
		{"importing", "error", true},
		{"importing", "queued", false},
		{"retry_wait", "downloading", true},
		{"retry_wait", "searching", true},
		{"error", "queued", true},
		{"error", "dead_letter", true},
		{"error", "downloading", false},
		{"dead_letter", "queued", true},
		{"dead_letter", "downloading", false},
		{"completed", "queued", false},
	}

	for _, tt := range tests {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			allowed, ok := validTransitions[tt.from]
			if !ok {
				t.Fatalf("no transitions defined for state %q", tt.from)
			}
			result := allowed[tt.to]
			if result != tt.allowed {
				t.Errorf("transition %s -> %s: got %v, want %v", tt.from, tt.to, result, tt.allowed)
			}
		})
	}
}

func TestGetTorrentFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/files" {
			hash := r.URL.Query().Get("hash")
			if hash == "testhash" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[{"name": "RootFolder/file1.mp3"}, {"name": "RootFolder/file2.mp3"}]`))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	q := newTestQBClient(srv.URL)
	files, err := q.GetTorrentFiles("testhash")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got: %d", len(files))
	}
	if files[0].Name != "RootFolder/file1.mp3" {
		t.Errorf("expected RootFolder/file1.mp3, got: %s", files[0].Name)
	}
}
