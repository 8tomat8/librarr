package api

import (
	"encoding/csv"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleImportGoodreads(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Failed to parse form: " + err.Error(),
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

	result := s.parseAndImportCSV(file, "goodreads")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleImportStoryGraph(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Failed to parse form: " + err.Error(),
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

	result := s.parseAndImportCSV(file, "storygraph")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) parseAndImportCSV(file io.Reader, source string) map[string]interface{} {
	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return map[string]interface{}{
			"success": false, "error": "Failed to read CSV header: " + err.Error(),
		}
	}

	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// Verify required columns exist.
	titleIdx, hasTitle := colMap["title"]
	if !hasTitle {
		return map[string]interface{}{
			"success": false, "error": "CSV must have a 'Title' column",
		}
	}

	authorIdx := -1
	if idx, ok := colMap["author"]; ok {
		authorIdx = idx
	} else if idx, ok := colMap["author l-f"]; ok {
		// Goodreads uses "Author l-f" (last, first).
		authorIdx = idx
	}

	// Goodreads column names.
	shelfIdx := -1
	if idx, ok := colMap["exclusive shelf"]; ok {
		shelfIdx = idx
	} else if idx, ok := colMap["bookshelves"]; ok {
		shelfIdx = idx
	} else if idx, ok := colMap["read status"]; ok {
		// StoryGraph uses "Read Status".
		shelfIdx = idx
	}

	ratingIdx := -1
	if idx, ok := colMap["my rating"]; ok {
		ratingIdx = idx
	} else if idx, ok := colMap["star rating"]; ok {
		ratingIdx = idx
	}

	dateReadIdx := -1
	if idx, ok := colMap["date read"]; ok {
		dateReadIdx = idx
	} else if idx, ok := colMap["last date read"]; ok {
		dateReadIdx = idx
	}

	isbnIdx := -1
	if idx, ok := colMap["isbn"]; ok {
		isbnIdx = idx
	} else if idx, ok := colMap["isbn13"]; ok {
		isbnIdx = idx
	}

	importedWishlist := 0
	importedHistory := 0
	skipped := 0

	// Get existing wishlist to deduplicate.
	existingWishlist, _ := s.db.GetWishlist()
	wishlistTitles := make(map[string]bool)
	for _, w := range existingWishlist {
		wishlistTitles[strings.ToLower(w.Title)] = true
	}

	// Get user ID for history (use first admin or ID 0).
	var userID int64
	users, _ := s.db.ListUsers()
	for _, u := range users {
		if u.Role == "admin" {
			userID = u.ID
			break
		}
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if titleIdx >= len(record) {
			continue
		}
		title := strings.TrimSpace(record[titleIdx])
		if title == "" {
			continue
		}

		author := ""
		if authorIdx >= 0 && authorIdx < len(record) {
			author = strings.TrimSpace(record[authorIdx])
			// Clean up "Last, First" format.
			if strings.Contains(author, ",") && !strings.Contains(author, " and ") {
				parts := strings.SplitN(author, ",", 2)
				if len(parts) == 2 {
					author = strings.TrimSpace(parts[1]) + " " + strings.TrimSpace(parts[0])
				}
			}
		}

		shelf := ""
		if shelfIdx >= 0 && shelfIdx < len(record) {
			shelf = strings.ToLower(strings.TrimSpace(record[shelfIdx]))
		}

		rating := 0
		if ratingIdx >= 0 && ratingIdx < len(record) {
			if r, err := strconv.Atoi(strings.TrimSpace(record[ratingIdx])); err == nil {
				rating = r
			}
		}

		var dateRead *time.Time
		if dateReadIdx >= 0 && dateReadIdx < len(record) {
			dateStr := strings.TrimSpace(record[dateReadIdx])
			if dateStr != "" {
				// Try common date formats.
				for _, layout := range []string{"2006/01/02", "2006-01-02", "01/02/2006", "Jan 2, 2006"} {
					if t, err := time.Parse(layout, dateStr); err == nil {
						dateRead = &t
						break
					}
				}
			}
		}

		switch {
		case shelf == "to-read" || shelf == "want to read":
			// Add to wishlist.
			if wishlistTitles[strings.ToLower(title)] {
				skipped++
				continue
			}
			if _, err := s.db.AddWishlistItem(title, author, "ebook"); err != nil {
				skipped++
				continue
			}
			wishlistTitles[strings.ToLower(title)] = true
			importedWishlist++

		case shelf == "read" || shelf == "finished":
			// Add to reading history.
			var ratingPtr *int
			if rating > 0 {
				ratingPtr = &rating
			}
			finishedAt := dateRead
			if finishedAt == nil {
				now := time.Now()
				finishedAt = &now
			}
			if _, err := s.db.AddReadingHistory(userID, title, author, "", nil, finishedAt, ratingPtr, "", nil); err != nil {
				skipped++
				continue
			}
			importedHistory++

		case shelf == "currently-reading" || shelf == "currently reading":
			// Add to reading history as started.
			now := time.Now()
			if _, err := s.db.AddReadingHistory(userID, title, author, "", &now, nil, nil, "", nil); err != nil {
				skipped++
				continue
			}
			importedHistory++

		default:
			// Unknown shelf, add to wishlist by default.
			if wishlistTitles[strings.ToLower(title)] {
				skipped++
				continue
			}
			if _, err := s.db.AddWishlistItem(title, author, "ebook"); err != nil {
				skipped++
				continue
			}
			wishlistTitles[strings.ToLower(title)] = true
			importedWishlist++
		}
	}

	_ = isbnIdx // ISBN available for future metadata enrichment

	return map[string]interface{}{
		"success":           true,
		"source":            source,
		"imported_wishlist": importedWishlist,
		"imported_history":  importedHistory,
		"skipped":           skipped,
	}
}
