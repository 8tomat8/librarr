package download

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/JeremiahM37/librarr/internal/config"
	"github.com/JeremiahM37/librarr/internal/db"
	"github.com/JeremiahM37/librarr/internal/models"
	"github.com/JeremiahM37/librarr/internal/organize"
	"github.com/JeremiahM37/librarr/internal/search"
)

// Watcher monitors qBittorrent for completed torrents and runs the import pipeline.
type Watcher struct {
	cfg       *config.Config
	db        *db.DB
	qb        *QBittorrentClient
	organizer *organize.Organizer
	targets   *organize.LibraryTargets
	health    *search.HealthTracker

	processing sync.Map // hash -> struct{}, tracks in-progress imports
	imported   sync.Map // hash -> struct{}, tracks already-imported hashes
}

// NewWatcher creates a new torrent completion watcher.
func NewWatcher(cfg *config.Config, database *db.DB, qb *QBittorrentClient, organizer *organize.Organizer, targets *organize.LibraryTargets, health *search.HealthTracker) *Watcher {
	return &Watcher{
		cfg:       cfg,
		db:        database,
		qb:        qb,
		organizer: organizer,
		targets:   targets,
		health:    health,
	}
}

// Start begins the background watcher loop. It blocks until ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) {
	if !w.cfg.HasQBittorrent() {
		slog.Info("torrent watcher disabled (qBittorrent not configured)")
		return
	}

	slog.Info("torrent completion watcher started", "interval", "30s")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Run once immediately.
	w.checkCompleted()

	for {
		select {
		case <-ctx.Done():
			slog.Info("torrent watcher stopping")
			return
		case <-ticker.C:
			w.checkCompleted()
		}
	}
}

func (w *Watcher) checkCompleted() {
	categories := []struct {
		name      string
		mediaType string
	}{
		{w.cfg.QBCategory, "ebook"},
		{w.cfg.QBAudiobookCategory, "audiobook"},
		{w.cfg.QBMangaCategory, "manga"},
	}

	for _, cat := range categories {
		torrents, err := w.qb.GetTorrents(cat.name)
		if err != nil {
			continue
		}
		for _, t := range torrents {
			if t.Progress < 1.0 {
				continue
			}

			// Skip already imported.
			if _, ok := w.imported.Load(t.Hash); ok {
				continue
			}
			// Skip currently processing.
			if _, loaded := w.processing.LoadOrStore(t.Hash, struct{}{}); loaded {
				continue
			}

			go w.importTorrent(t, cat.mediaType)
		}
	}
}

func (w *Watcher) importTorrent(t TorrentInfo, mediaType string) {
	defer w.processing.Delete(t.Hash)

	slog.Info("importing completed torrent", "name", t.Name, "hash", t.Hash, "type", mediaType)

	savePath := w.resolveLocalPath(t, mediaType)

	var importErr error
	switch mediaType {
	case "ebook":
		importErr = w.importEbook(t, savePath)
	case "audiobook":
		importErr = w.importAudiobook(t, savePath)
	case "manga":
		importErr = w.importManga(t, savePath)
	}

	if importErr != nil {
		slog.Error("torrent import failed", "name", t.Name, "type", mediaType, "error", importErr)
		return
	}

	// Optionally remove torrent from qBit after import. Default is to remove;
	// set REMOVE_TORRENT_AFTER_IMPORT=false to keep seeding (e.g. private trackers).
	if w.cfg.RemoveTorrentAfterImport {
		if err := w.qb.DeleteTorrent(t.Hash, false); err != nil {
			slog.Warn("failed to remove torrent after import", "hash", t.Hash, "error", err)
		} else {
			slog.Info("removed completed torrent", "name", t.Name)
		}
	} else {
		slog.Info("torrent left seeding after import", "name", t.Name, "hash", t.Hash)
	}

	// Mark as imported.
	w.imported.Store(t.Hash, struct{}{})

	// Log the import.
	_ = w.db.LogEvent("torrent_import", t.Name, fmt.Sprintf("Imported %s from torrent", mediaType), nil, t.Hash)
}

// resolveLocalPath maps qBittorrent container paths to local paths.
// Each media type uses its dedicated INCOMING directory (where qBit
// downloads to), NOT the final organized library directory. Previously
// audiobooks used AudiobookDir (the library target) which caused files
// to be looked up in the wrong place and could lead to ebooks being
// misrouted if paths overlapped.
func (w *Watcher) resolveLocalPath(t TorrentInfo, mediaType string) string {
	var rootName string

	if t.ContentPath != "" {
		return w.resolveContentPath(t, mediaType)
	}

	// Fetch files from qBittorrent to find the actual root folder/file.
	var files []TorrentFile
	var err error
	if w.qb != nil {
		files, err = w.qb.GetTorrentFiles(t.Hash)
	}
	if err == nil && len(files) > 0 {
		var firstPart string
		allSameRoot := true
		for i, f := range files {
			parts := strings.Split(f.Name, "/")
			if len(parts) > 0 {
				if i == 0 {
					firstPart = parts[0]
				} else if firstPart != parts[0] {
					allSameRoot = false
					break
				}
			}
		}
		if allSameRoot && firstPart != "" {
			rootName = normalizeTorrentPath(firstPart)
		}
	}

	if rootName == "" {
		rootName = normalizeTorrentPath(t.Name)
	}

	return filepath.Join(w.incomingDirForMedia(mediaType), rootName)
}

func (w *Watcher) incomingDirForMedia(mediaType string) string {
	switch mediaType {
	case "ebook":
		return w.cfg.IncomingDir
	case "audiobook":
		if w.cfg.QBAudiobookSavePath != "" {
			return w.cfg.QBAudiobookSavePath
		}
		return w.cfg.IncomingDir
	case "manga":
		if w.cfg.MangaIncomingDir != "" {
			return w.cfg.MangaIncomingDir
		}
		return w.cfg.IncomingDir
	default:
		return w.cfg.IncomingDir
	}
}

func (w *Watcher) resolveContentPath(t TorrentInfo, mediaType string) string {
	contentPath := normalizeTorrentPath(t.ContentPath)
	savePath := normalizeTorrentPath(t.SavePath)
	localIncoming := w.incomingDirForMedia(mediaType)

	if contentPath == "" {
		return localIncoming
	}

	if localIncoming != "" {
		if rel, err := filepath.Rel(localIncoming, contentPath); err == nil && rel == "." {
			return contentPath
		} else if err == nil && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." {
			return contentPath
		}
	}

	if savePath != "" {
		if rel, err := filepath.Rel(savePath, contentPath); err == nil && rel == "." {
			return localIncoming
		} else if err == nil && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." {
			return filepath.Join(localIncoming, rel)
		}
	}

	if !filepath.IsAbs(contentPath) && contentPath != ".." && !strings.HasPrefix(contentPath, ".."+string(os.PathSeparator)) {
		return filepath.Join(localIncoming, contentPath)
	}

	return filepath.Join(localIncoming, filepath.Base(contentPath))
}

func (w *Watcher) importEbook(t TorrentInfo, savePath string) error {
	bookFiles := findFilesByExt(savePath, []string{".epub", ".mobi", ".pdf", ".azw3"})
	if len(bookFiles) == 0 {
		return fmt.Errorf("no ebook files found at %s", savePath)
	}

	for _, bf := range bookFiles {
		destPath, err := w.organizer.OrganizeEbook(bf, t.Name, "")
		if err != nil {
			slog.Warn("organize ebook failed", "file", bf, "error", err)
			destPath = bf
		}

		// Try to extract author from EPUB metadata.
		author := ""
		if strings.HasSuffix(strings.ToLower(destPath), ".epub") {
			if meta, err := organize.ExtractEPUBMeta(destPath); err == nil && meta.Author != "" {
				author = meta.Author
			}
		}

		w.db.AddItem(&models.LibraryItem{
			Title:     t.Name,
			Author:    author,
			FilePath:  destPath,
			FileSize:  t.TotalSize,
			MediaType: "ebook",
			Source:    "torrent",
			SourceID:  t.Hash,
		})

		// Import to external libraries.
		w.targets.ImportEbook(destPath, t.Name, author)
	}

	return nil
}

func (w *Watcher) importAudiobook(t TorrentInfo, savePath string) error {
	// If the source path doesn't even exist, fail the import.
	if _, statErr := os.Stat(savePath); os.IsNotExist(statErr) {
		return fmt.Errorf("source path does not exist: %s", savePath)
	}

	// Extract author from torrent name if possible.
	author := ""
	title := t.Name
	if strings.Contains(title, " - ") {
		parts := strings.SplitN(title, " - ", 2)
		author = strings.TrimSpace(parts[0])
		title = strings.TrimSpace(parts[1])
	}
	if author == "" {
		author = "Unknown"
	}

	destPath, err := w.organizer.OrganizeAudiobook(savePath, title, author)
	if err != nil {
		return fmt.Errorf("organize audiobook %q: %w", savePath, err)
	}

	w.db.AddItem(&models.LibraryItem{
		Title:     title,
		Author:    author,
		FilePath:  destPath,
		FileSize:  t.TotalSize,
		MediaType: "audiobook",
		Source:    "torrent",
		SourceID:  t.Hash,
	})

	w.targets.ImportAudiobook()

	return nil
}

func (w *Watcher) importManga(t TorrentInfo, savePath string) error {
	mangaFiles := findFilesByExt(savePath, []string{".cbz", ".cbr", ".zip", ".pdf", ".epub"})
	if len(mangaFiles) == 0 {
		return fmt.Errorf("no manga files found at %s", savePath)
	}

	for _, mf := range mangaFiles {
		destPath, err := w.organizer.OrganizeManga(mf, t.Name)
		if err != nil {
			slog.Warn("organize manga failed", "file", mf, "error", err)
			destPath = mf
		}

		w.db.AddItem(&models.LibraryItem{
			Title:     t.Name,
			FilePath:  destPath,
			FileSize:  t.TotalSize,
			MediaType: "manga",
			Source:    "torrent",
			SourceID:  t.Hash,
		})

		w.targets.ImportManga(destPath, t.Name)
	}

	return nil
}

// normalizeTorrentPath unescapes HTML entities (e.g. &amp; -> &) that
// qBittorrent may embed in torrent names.
func normalizeTorrentPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return html.UnescapeString(value)
}

// findFilesByExt recursively finds files with given extensions.
func findFilesByExt(root string, exts []string) []string {
	var files []string

	info, err := os.Stat(root)
	if err != nil {
		return files
	}

	if !info.IsDir() {
		lower := strings.ToLower(root)
		for _, ext := range exts {
			if strings.HasSuffix(lower, ext) {
				return []string{root}
			}
		}
		return files
	}

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		for _, ext := range exts {
			if strings.HasSuffix(lower, ext) {
				files = append(files, path)
				break
			}
		}
		return nil
	})

	return files
}
