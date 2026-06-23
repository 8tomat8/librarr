package download

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JeremiahM37/librarr/internal/config"
)

func TestResolveLocalPathAudiobookUsesContentPath(t *testing.T) {
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/downloads/audiobooks-incoming",
			IncomingDir:         "/downloads/incoming",
		},
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name:        "Brigands &amp; Breadknives (Legends &amp; Lattes) - Travis Baldree",
		ContentPath: "/downloads/audiobooks-incoming/Brigands &amp; Breadknives.m4b",
	}, "audiobook")

	want := "/downloads/audiobooks-incoming/Brigands & Breadknives.m4b"
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathAudiobookMapsRemoteContentPathToLocalIncoming(t *testing.T) {
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/data/audiobooks-incoming",
			IncomingDir:         "/data/incoming",
		},
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name:        "Brigands &amp; Breadknives (Legends &amp; Lattes) - Travis Baldree",
		ContentPath: "/downloads/audiobooks-incoming/Brigands &amp; Breadknives.m4b",
		SavePath:    "/downloads/audiobooks-incoming",
	}, "audiobook")

	want := "/data/audiobooks-incoming/Brigands & Breadknives.m4b"
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathAudiobookPreservesRelativeContentPath(t *testing.T) {
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/data/audiobooks-incoming",
			IncomingDir:         "/data/incoming",
		},
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name:        "Some Book",
		ContentPath: "Series/Some Book/part01.mp3",
	}, "audiobook")

	want := filepath.Join("/data/audiobooks-incoming", "Series/Some Book/part01.mp3")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathAudiobookMapsRemoteSaveRootToLocalIncoming(t *testing.T) {
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/data/audiobooks-incoming",
			IncomingDir:         "/data/incoming",
		},
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name:        "Some Book",
		ContentPath: "/downloads/audiobooks-incoming",
		SavePath:    "/downloads/audiobooks-incoming",
	}, "audiobook")

	want := "/data/audiobooks-incoming"
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathAudiobookFallsBackToName(t *testing.T) {
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/downloads/audiobooks-incoming",
			IncomingDir:         "/downloads/incoming",
		},
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name: "Brigands &amp; Breadknives (Legends &amp; Lattes) - Travis Baldree",
	}, "audiobook")

	want := filepath.Join("/downloads/audiobooks-incoming", "Brigands & Breadknives (Legends & Lattes) - Travis Baldree")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathEbookFallsBackToName(t *testing.T) {
	w := &Watcher{
		cfg: &config.Config{
			IncomingDir: "/downloads/incoming",
		},
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name: "Some Book - Author",
	}, "ebook")

	want := filepath.Join("/downloads/incoming", "Some Book - Author")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathMangaFallsBackToIncomingDir(t *testing.T) {
	w := &Watcher{
		cfg: &config.Config{
			IncomingDir: "/downloads/incoming",
		},
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name: "One Piece Vol 100",
	}, "manga")

	want := filepath.Join("/downloads/incoming", "One Piece Vol 100")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathMangaUsesConfiguredDir(t *testing.T) {
	w := &Watcher{
		cfg: &config.Config{
			IncomingDir:      "/downloads/incoming",
			MangaIncomingDir: "/downloads/manga-incoming",
		},
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name: "One Piece Vol 100",
	}, "manga")

	want := filepath.Join("/downloads/manga-incoming", "One Piece Vol 100")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

// newMockQBServer creates a test server that serves both login and torrents/files endpoints.
func newMockQBServer(files map[string][]TorrentFile) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		case "/api/v2/torrents/files":
			hash := r.URL.Query().Get("hash")
			if f, ok := files[hash]; ok {
				json.NewEncoder(w).Encode(f)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestResolveLocalPathUsesGetTorrentFilesWhenContentPathEmpty(t *testing.T) {
	srv := newMockQBServer(map[string][]TorrentFile{
		"abc123": {
			{Name: "Sublimation/track01.mp3"},
			{Name: "Sublimation/track02.mp3"},
		},
	})
	defer srv.Close()

	qb := newTestQBClient(srv.URL)
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/downloads/audiobooks-incoming",
			IncomingDir:         "/downloads/incoming",
		},
		torrent: qb,
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name: "Sublimation - Isabel J. Kim",
		Hash: "abc123",
	}, "audiobook")

	want := filepath.Join("/downloads/audiobooks-incoming", "Sublimation")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathSingleFileNoSubfolder(t *testing.T) {
	srv := newMockQBServer(map[string][]TorrentFile{
		"def456": {
			{Name: "The_Unicorn_Hunters.m4b"},
		},
	})
	defer srv.Close()

	qb := newTestQBClient(srv.URL)
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/downloads/audiobooks-incoming",
			IncomingDir:         "/downloads/incoming",
		},
		torrent: qb,
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name: "The Unicorn Hunters - Katherine Arden",
		Hash: "def456",
	}, "audiobook")

	want := filepath.Join("/downloads/audiobooks-incoming", "The_Unicorn_Hunters.m4b")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathMultiFileDifferentRootsFallsBack(t *testing.T) {
	srv := newMockQBServer(map[string][]TorrentFile{
		"ghi789": {
			{Name: "track1.mp3"},
			{Name: "track2.mp3"},
		},
	})
	defer srv.Close()

	qb := newTestQBClient(srv.URL)
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/downloads/audiobooks-incoming",
			IncomingDir:         "/downloads/incoming",
		},
		torrent: qb,
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name: "Some Audiobook - Author",
		Hash: "ghi789",
	}, "audiobook")

	// Multiple files without a common root -> falls back to t.Name
	want := filepath.Join("/downloads/audiobooks-incoming", "Some Audiobook - Author")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathAPIErrorFallsBackToName(t *testing.T) {
	// Server that always returns 500 for files endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	qb := newTestQBClient(srv.URL)
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/downloads/audiobooks-incoming",
			IncomingDir:         "/downloads/incoming",
		},
		torrent: qb,
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name: "Some Book - Author",
		Hash: "fail",
	}, "audiobook")

	want := filepath.Join("/downloads/audiobooks-incoming", "Some Book - Author")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestResolveLocalPathContentPathTakesPrecedence(t *testing.T) {
	// Even with a qb client that has files, ContentPath should win.
	srv := newMockQBServer(map[string][]TorrentFile{
		"xyz": {{Name: "WrongFolder/file.mp3"}},
	})
	defer srv.Close()

	qb := newTestQBClient(srv.URL)
	w := &Watcher{
		cfg: &config.Config{
			QBAudiobookSavePath: "/downloads/audiobooks-incoming",
			IncomingDir:         "/downloads/incoming",
		},
		torrent: qb,
	}

	got := w.resolveLocalPath(TorrentInfo{
		Name:        "Some Name",
		Hash:        "xyz",
		ContentPath: "/downloads/audiobooks-incoming/CorrectFolder",
	}, "audiobook")

	want := "/downloads/audiobooks-incoming/CorrectFolder"
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestNormalizeTorrentPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"simple name", "simple name"},
		{"Brigands &amp; Breadknives", "Brigands & Breadknives"},
		{"  /path/to/file.m4b  ", "/path/to/file.m4b"},
		{"Title &lt;Special&gt;", "Title <Special>"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeTorrentPath(tt.input)
			if got != tt.want {
				t.Errorf("normalizeTorrentPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
