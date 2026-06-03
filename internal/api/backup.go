package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	// Name is optional.
	json.NewDecoder(r.Body).Decode(&req)

	name := req.Name
	if name == "" {
		name = time.Now().Format("2006-01-02_150405")
	}
	// Sanitize name.
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)

	backupDir := filepath.Join(filepath.Dir(s.db.GetDBPath()), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": "Failed to create backup directory",
		})
		return
	}

	zipPath := filepath.Join(backupDir, fmt.Sprintf("librarr-backup-%s.zip", name))
	if err := s.createBackupZip(zipPath); err != nil {
		slog.Error("backup creation failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": "Failed to create backup",
		})
		return
	}

	// Cleanup old backups (keep last 5).
	s.cleanupOldBackups(backupDir, 5)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"name":    name,
	})
}

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	// Create a temporary backup.
	tmpDir := os.TempDir()
	zipPath := filepath.Join(tmpDir, fmt.Sprintf("librarr-backup-%d.zip", time.Now().Unix()))
	defer os.Remove(zipPath)

	if err := s.createBackupZip(zipPath); err != nil {
		slog.Error("backup creation failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": "Failed to create backup",
		})
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=librarr-backup-%s.zip", time.Now().Format("2006-01-02")))
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, zipPath)
}

func (s *Server) handleListBackups(w http.ResponseWriter, _ *http.Request) {
	backupDir := filepath.Join(filepath.Dir(s.db.GetDBPath()), "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		// No backups directory yet.
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	type backupInfo struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		Created  string `json:"created"`
		Filename string `json:"filename"`
	}

	var backups []backupInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupInfo{
			Name:     strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "librarr-backup-"), ".zip"),
			Size:     info.Size(),
			Created:  info.ModTime().Format(time.RFC3339),
			Filename: entry.Name(),
		})
	}

	if backups == nil {
		backups = []backupInfo{}
	}
	writeJSON(w, http.StatusOK, backups)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(50 << 20); err != nil { // 50MB max
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Failed to parse upload: " + err.Error(),
		})
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "No file uploaded",
		})
		return
	}
	defer file.Close()

	// Save uploaded ZIP to temp.
	tmpFile, err := os.CreateTemp("", "librarr-restore-*.zip")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": "Failed to save upload",
		})
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": "Failed to save upload",
		})
		return
	}
	tmpFile.Close()

	// Validate ZIP contents.
	zr, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Invalid ZIP file",
		})
		return
	}

	hasDB := false
	for _, f := range zr.File {
		if f.Name == "librarr.db" {
			hasDB = true
			break
		}
	}
	zr.Close()

	if !hasDB {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "ZIP must contain librarr.db",
		})
		return
	}

	// Extract the DB file.
	zr, _ = zip.OpenReader(tmpFile.Name())
	defer zr.Close()

	dbPath := s.db.GetDBPath()
	for _, f := range zr.File {
		if f.Name == "librarr.db" {
			rc, err := f.Open()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"success": false, "error": "Failed to read DB from ZIP",
				})
				return
			}

			// Write to DB path (will require restart).
			destFile, err := os.Create(dbPath + ".restore")
			if err != nil {
				rc.Close()
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"success": false, "error": "Failed to write restored DB",
				})
				return
			}
			io.Copy(destFile, rc)
			destFile.Close()
			rc.Close()
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Backup restored to " + dbPath + ".restore. Restart the application to apply.",
	})
}

func (s *Server) createBackupZip(zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	// Add DB file.
	dbPath := s.db.GetDBPath()
	if err := addFileToZip(zw, dbPath, "librarr.db"); err != nil {
		return fmt.Errorf("add db: %w", err)
	}

	// Add settings file if exists.
	settingsPath := s.cfg.SettingsFile
	if settingsPath != "" {
		if _, err := os.Stat(settingsPath); err == nil {
			addFileToZip(zw, settingsPath, "settings.json")
		}
	}

	return nil
}

func addFileToZip(zw *zip.Writer, filePath, archiveName string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = archiveName
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

func (s *Server) cleanupOldBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type fileEntry struct {
		name    string
		modTime time.Time
	}

	var zips []fileEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		zips = append(zips, fileEntry{name: e.Name(), modTime: info.ModTime()})
	}

	if len(zips) <= keep {
		return
	}

	sort.Slice(zips, func(i, j int) bool {
		return zips[i].modTime.After(zips[j].modTime)
	})

	for _, z := range zips[keep:] {
		path := filepath.Join(dir, z.name)
		if err := os.Remove(path); err != nil {
			slog.Warn("failed to remove old backup", "path", path, "error", err)
		}
	}
}
