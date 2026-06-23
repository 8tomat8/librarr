package download

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
)

// fileFamily is a coarse content classification derived from magic bytes. It is
// intentionally broader than a file extension because several extensions share
// a container (e.g. .mobi and .azw3 are both PDB/MOBI; .epub is a ZIP).
type fileFamily string

const (
	familyPDF  fileFamily = "pdf"
	familyZip  fileFamily = "zip"
	familyRar  fileFamily = "rar"
	familyMobi fileFamily = "mobi"
	familyMP3  fileFamily = "mp3"
	familyMP4  fileFamily = "mp4" // ISO base media: m4a/m4b/mp4
)

// familyExtensions maps a detected family to the upload extensions that are
// legitimately stored in that container. An extension not present for its
// detected family is a content/extension mismatch.
var familyExtensions = map[fileFamily]map[string]bool{
	familyPDF:  {".pdf": true},
	familyZip:  {".epub": true, ".zip": true},
	familyRar:  {".rar": true},
	familyMobi: {".mobi": true, ".azw3": true},
	familyMP3:  {".mp3": true},
	familyMP4:  {".m4b": true, ".m4a": true},
}

// detectFileFamily inspects the leading bytes of a file and returns its content
// family, or "" if the signature is not recognized.
func detectFileFamily(path string) (fileFamily, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// 68 bytes covers the PDB type/creator fields at offset 60-67.
	var h [68]byte
	n, err := f.Read(h[:])
	if err != nil && err != io.EOF {
		return "", err
	}
	b := h[:n]

	switch {
	case len(b) >= 5 && string(b[:5]) == "%PDF-":
		return familyPDF, nil
	case len(b) >= 4 && b[0] == 0x50 && b[1] == 0x4B &&
		(b[2] == 0x03 || b[2] == 0x05 || b[2] == 0x07):
		return familyZip, nil // PK.. — ZIP/EPUB/CBZ
	case len(b) >= 4 && b[0] == 0x52 && b[1] == 0x61 && b[2] == 0x72 && b[3] == 0x21:
		return familyRar, nil // "Rar!"
	case len(b) >= 8 && string(b[4:8]) == "ftyp":
		return familyMP4, nil // ISO base media (m4a/m4b/mp4)
	case len(b) >= 3 && string(b[:3]) == "ID3":
		return familyMP3, nil // ID3v2-tagged MP3
	case len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0:
		return familyMP3, nil // MPEG audio frame sync
	case len(b) >= 68 && string(b[60:64]) == "BOOK":
		return familyMobi, nil // PDB type "BOOK" (MOBI/AZW/AZW3)
	}
	return "", nil
}

// ContentMatchesExtension reports whether a file's magic bytes are consistent
// with the declared extension. When the signature is unrecognized it returns
// (true, "") — the caller cannot prove a mismatch. For .epub it additionally
// requires a valid EPUB OCF structure so an arbitrary ZIP cannot pose as one.
func ContentMatchesExtension(path, ext string) (bool, fileFamily, error) {
	ext = strings.ToLower(ext)
	fam, err := detectFileFamily(path)
	if err != nil {
		return false, "", err
	}
	if fam == "" {
		return true, "", nil
	}
	if !familyExtensions[fam][ext] {
		return false, fam, nil
	}
	if ext == ".epub" {
		ok, err := isEPUB(path)
		if err != nil {
			return false, fam, err
		}
		return ok, fam, nil
	}
	return true, fam, nil
}

// isEPUB reports whether the file is a valid EPUB: a ZIP archive containing a
// "mimetype" entry whose content is exactly "application/epub+zip" (EPUB OCF).
func isEPUB(path string) (bool, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false, nil // not even a readable ZIP
	}
	defer zr.Close()

	for _, zf := range zr.File {
		if zf.Name != "mimetype" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return false, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 64))
		rc.Close()
		if err != nil {
			return false, err
		}
		return bytes.Equal(bytes.TrimSpace(data), []byte("application/epub+zip")), nil
	}
	return false, nil
}
