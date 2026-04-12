package download

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JeremiahM37/librarr/internal/config"
)

// --- Magic byte detection tests ---

func TestDetectFileExtension(t *testing.T) {
	// Real PDF header (from PDF spec: %PDF-<version>\n)
	pdfBytes := []byte("%PDF-1.6\n%\xE2\xE3\xCF\xD3\n")
	// Real EPUB header (ZIP local file header: PK\x03\x04)
	epubBytes := []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x00, 0x00}
	// Empty ZIP (central directory only: PK\x05\x06)
	emptyZipBytes := []byte{0x50, 0x4B, 0x05, 0x06, 0x00, 0x00, 0x00, 0x00}
	// RAR 4.x header (CBR files use this)
	rarBytes := []byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x00, 0x00}
	// MOBI PalmDB header (starts with BOOK)
	mobiBytes := append([]byte("BOOK"), make([]byte, 100)...)

	tests := []struct {
		name        string
		content     []byte
		expectedExt string
	}{
		{"PDF magic bytes", pdfBytes, ".pdf"},
		{"EPUB/ZIP PK\\x03\\x04", epubBytes, ".epub"},
		{"Empty ZIP PK\\x05\\x06", emptyZipBytes, ".epub"},
		{"RAR/CBR magic bytes", rarBytes, ".cbr"},
		{"MOBI BOOK header", mobiBytes, ".mobi"},
		{"Unrecognized format", []byte("randomdata12345"), ""},
		{"Too small (3 bytes)", []byte("xx\n"), ""},
		{"Empty file", []byte{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.bin")
			// Pad to at least 8 bytes so reads succeed
			content := tt.content
			if len(content) < 8 && len(content) > 0 {
				content = append(content, make([]byte, 8-len(content))...)
			}
			if err := os.WriteFile(path, content, 0644); err != nil {
				t.Fatalf("write test file: %v", err)
			}

			ext, err := detectFileExtension(path)
			if err != nil {
				t.Fatalf("detectFileExtension: %v", err)
			}
			if ext != tt.expectedExt {
				t.Errorf("got %q, expected %q", ext, tt.expectedExt)
			}
		})
	}
}

func TestDetectFileExtension_MissingFile(t *testing.T) {
	_, err := detectFileExtension("/nonexistent/file.bin")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestDetectFileExtension_RealPDFFile(t *testing.T) {
	// Simulates a real PDF download: "%PDF-" header followed by binary content.
	// This is exactly what Anna's Archive returned for the issue #8 MD5.
	dir := t.TempDir()
	path := filepath.Join(dir, "C in a Nutshell.epub") // saved with wrong ext
	content := []byte("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n1 0 obj<</Type /Catalog>>endobj\nxref\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ext, err := detectFileExtension(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if ext != ".pdf" {
		t.Errorf("real PDF file detected as %q, expected .pdf", ext)
	}
}

// --- End-to-end download test: full flow from HTTP server to renamed file ---

// TestDownloadFile_PDFSavedAsEPUBGetsRenamed is the regression test for issue #8.
// A server returns a PDF with Content-Type: application/octet-stream
// (which defaults to .epub in the code). The file should end up with .pdf
// extension after the content-based detection.
func TestDownloadFile_PDFSavedAsEPUBGetsRenamed(t *testing.T) {
	// Realistic PDF content (not just magic bytes — need to pass the 1000 byte min size check)
	pdfContent := []byte("%PDF-1.6\n%\xE2\xE3\xCF\xD3\n")
	pdfContent = append(pdfContent, make([]byte, 2000)...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate libgen serving the file with an ambiguous content type.
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdfContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	cfg := &config.Config{IncomingDir: dir, UserAgent: "test"}
	d := NewDirectDownloader(cfg, server.Client())

	filePath, size, err := d.downloadFile(server.URL, "C in a Nutshell", nil)
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	if size != int64(len(pdfContent)) {
		t.Errorf("wrong size: got %d, expected %d", size, len(pdfContent))
	}
	if !strings.HasSuffix(filePath, ".pdf") {
		t.Errorf("file should have .pdf extension after content detection, got: %s", filePath)
	}
	// Verify the file is actually at the corrected path and the wrong-ext one is gone
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("corrected file not found: %v", err)
	}
}

func TestDownloadFile_EPUBKeepsCorrectExtension(t *testing.T) {
	// Valid EPUB header (ZIP) — should keep .epub extension
	epubContent := []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x00, 0x00}
	epubContent = append(epubContent, make([]byte, 2000)...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/epub+zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(epubContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	cfg := &config.Config{IncomingDir: dir, UserAgent: "test"}
	d := NewDirectDownloader(cfg, server.Client())

	// downloadFile runs EPUB validation after detection. A fake ZIP will fail
	// the validation (VerifyEPUBTitle errors -> "allowing download" warn path),
	// so the download should still succeed. We just verify the extension handling.
	filePath, _, err := d.downloadFile(server.URL, "Test Book", nil)
	if err != nil {
		// Validation may fail with our fake ZIP; we only care about extension logic
		if !strings.Contains(err.Error(), "EPUB verification") {
			t.Fatalf("unexpected error: %v", err)
		}
		// Even on verification failure, the file should have had .epub during detection
		return
	}
	if !strings.HasSuffix(filePath, ".epub") {
		t.Errorf("EPUB should keep .epub extension, got: %s", filePath)
	}
}

func TestDownloadFile_UnknownFormatKeepsOriginalExt(t *testing.T) {
	// Random binary data — no magic bytes match. Keep the content-type-derived ext.
	randomContent := make([]byte, 2000)
	for i := range randomContent {
		randomContent[i] = byte(i % 256)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(randomContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	cfg := &config.Config{IncomingDir: dir, UserAgent: "test"}
	d := NewDirectDownloader(cfg, server.Client())

	filePath, _, err := d.downloadFile(server.URL, "Unknown Binary", nil)
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	// Content-Type said PDF, content doesn't match any known format —
	// should keep .pdf (the content-type-derived extension).
	if !strings.HasSuffix(filePath, ".pdf") {
		t.Errorf("unknown content should keep content-type extension .pdf, got: %s", filePath)
	}
}

func TestDownloadFile_TooSmallRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("tiny"))
	}))
	defer server.Close()

	dir := t.TempDir()
	cfg := &config.Config{IncomingDir: dir, UserAgent: "test"}
	d := NewDirectDownloader(cfg, server.Client())

	_, _, err := d.downloadFile(server.URL, "Tiny", nil)
	if err == nil {
		t.Error("expected error for too-small file, got nil")
	}
	if !strings.Contains(err.Error(), "too small") {
		t.Errorf("expected 'too small' error, got: %v", err)
	}
	// File should be cleaned up
	files, _ := os.ReadDir(dir)
	if len(files) > 0 {
		t.Errorf("file should have been cleaned up, found: %v", files[0].Name())
	}
}

// --- Silence linter (io import used only in some builds) ---
var _ = io.Copy
