package download

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JeremiahM37/librarr/internal/config"
)

func TestQBittorrentAddTorrentAcceptsJSONSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "QBT_SID", Value: "abc123", Path: "/"})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("QBT_SID"); err != nil {
			t.Fatalf("expected auth cookie on add request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"added_torrent_ids":["e2f71d638953c009f17594d6982c6de68b06d985"],"failure_count":0,"pending_count":0,"success_count":1}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := &config.Config{
		QBUrl:      srv.URL,
		QBUser:     "admin",
		QBPass:     "secret",
		QBSavePath: "/downloads",
		QBCategory: "librarr",
	}
	q := NewQBittorrentClient(cfg)
	q.client = srv.Client()

	if err := q.AddTorrent("https://example.com/file.torrent", "Test Book", "", ""); err != nil {
		t.Fatalf("AddTorrent returned error: %v", err)
	}
}

func TestQBittorrentAddTorrentRejectsJSONFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "QBT_SID", Value: "abc123", Path: "/"})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"added_torrent_ids":[],"failure_count":1,"pending_count":0,"success_count":0,"error":"invalid torrent"}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := &config.Config{
		QBUrl:      srv.URL,
		QBUser:     "admin",
		QBPass:     "secret",
		QBSavePath: "/downloads",
		QBCategory: "librarr",
	}
	q := NewQBittorrentClient(cfg)
	q.client = srv.Client()

	err := q.AddTorrent("https://example.com/file.torrent", "Test Book", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "invalid torrent") {
		t.Fatalf("error = %q, want invalid torrent", got)
	}
}
