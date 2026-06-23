package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JeremiahM37/librarr/internal/config"
	"github.com/JeremiahM37/librarr/internal/db"
	"github.com/JeremiahM37/librarr/internal/organize"
	"github.com/JeremiahM37/librarr/internal/search"
)

// TestPipelineDirectDownloadOrganizeImport exercises search → download → organize → DB import
// without external network dependencies.
func TestPipelineDirectDownloadOrganizeImport(t *testing.T) {
	epubHeader := []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x00, 0x00}
	payload := append(epubHeader, make([]byte, 2000)...)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/epub+zip")
		w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := &config.Config{
		IncomingDir:    filepath.Join(dir, "incoming"),
		EbookDir:       filepath.Join(dir, "ebooks"),
		FileOrgEnabled: true,
		UserAgent:      "pipeline-test",
		MaxRetries:     0,
	}

	database, err := db.New(filepath.Join(dir, "pipeline.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer database.Close()

	health := search.NewHealthTracker(3, 300)
	direct := NewDirectDownloader(cfg, srv.Client())
	direct.validate = nil // httptest serves on loopback; not exercising the SSRF guard here
	organizer := organize.NewOrganizer(cfg)
	mgr := NewManager(cfg, database, nil, nil, direct, organizer, nil, health)
	mgr.SetShutdownContext(context.Background())

	job, err := mgr.StartDirectDownload(srv.URL, "Pipeline Test Book", "test", "pipeline-test-1")
	if err != nil {
		t.Fatalf("StartDirectDownload: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	var finalStatus string
	for time.Now().Before(deadline) {
		mgr.mu.Lock()
		j, ok := mgr.jobs[job.ID]
		if ok {
			finalStatus = j.Status
		}
		mgr.mu.Unlock()
		if finalStatus == "completed" || finalStatus == "error" || finalStatus == "dead_letter" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if finalStatus != "completed" {
		t.Fatalf("expected completed status, got %q", finalStatus)
	}

	if !database.HasSourceID("pipeline-test-1") {
		t.Fatal("expected library item with source_id pipeline-test-1")
	}

	items, err := database.FindByTitle("Pipeline Test Book")
	if err != nil || len(items) == 0 {
		t.Fatalf("expected library item for title, err=%v len=%d", err, len(items))
	}
}
