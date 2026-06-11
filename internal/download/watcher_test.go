package download

import (
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
