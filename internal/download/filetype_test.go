package download

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeFile writes b to a temp file and returns its path.
func writeFile(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

// makeEPUB builds a minimal valid EPUB (ZIP with a "mimetype" entry).
func makeEPUB(t *testing.T, mimetype string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "book.epub")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	mw, _ := zw.Create("mimetype")
	mw.Write([]byte(mimetype))
	cw, _ := zw.Create("content.opf")
	cw.Write([]byte("<package/>"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func mobiBytes() []byte {
	b := make([]byte, 80)
	copy(b[0:], []byte("TITLE\x00\x00\x00"))
	copy(b[60:], []byte("BOOK")) // PDB type
	copy(b[64:], []byte("MOBI")) // PDB creator
	return b
}

func TestContentMatchesExtension(t *testing.T) {
	pdf := writeFile(t, "x.pdf", []byte("%PDF-1.7\nrest of file"))
	plainZip := writeFile(t, "x.zip", []byte("PK\x03\x04 arbitrary zip contents here"))
	mp3 := writeFile(t, "x.mp3", append([]byte("ID3\x03"), make([]byte, 64)...))
	m4b := writeFile(t, "x.m4b", append([]byte("\x00\x00\x00\x20ftypM4B "), make([]byte, 64)...))
	mobi := writeFile(t, "x.mobi", mobiBytes())
	epub := makeEPUB(t, "application/epub+zip")
	fakeEpub := makeEPUB(t, "application/zip") // zip wearing an .epub name
	unknown := writeFile(t, "x.epub", []byte("\x00\x01\x02\x03 not a known format"))

	tests := []struct {
		name   string
		path   string
		ext    string
		wantOK bool
	}{
		{"pdf ok", pdf, ".pdf", true},
		{"pdf mislabeled epub", pdf, ".epub", false},
		{"plain zip as zip ok", plainZip, ".zip", true},
		{"plain zip as epub rejected", plainZip, ".epub", false},
		{"real epub ok", epub, ".epub", true},
		{"epub with wrong mimetype rejected", fakeEpub, ".epub", false},
		{"mp3 ok", mp3, ".mp3", true},
		{"mp3 mislabeled m4b", mp3, ".m4b", false},
		{"m4b ok", m4b, ".m4b", true},
		{"mobi ok", mobi, ".mobi", true},
		{"azw3 shares mobi family", mobi, ".azw3", true},
		{"unknown signature allowed", unknown, ".epub", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, _, err := ContentMatchesExtension(tt.path, tt.ext)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tt.wantOK {
				t.Errorf("ContentMatchesExtension(%s, %s) = %v, want %v", tt.name, tt.ext, ok, tt.wantOK)
			}
		})
	}
}
