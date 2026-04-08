package organize

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// RenameConfig holds configuration for file renaming on import.
type RenameConfig struct {
	Enabled bool
	Pattern string // e.g., "{author} - {title} ({year}).{ext}"
}

// RenameFile renames a file according to the configured pattern.
// Returns the new file path.
func RenameFile(filePath string, cfg RenameConfig, author, title, year string) (string, error) {
	if !cfg.Enabled || cfg.Pattern == "" {
		return filePath, nil
	}

	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
	dir := filepath.Dir(filePath)

	// Clean up metadata for filename use.
	cleanAuthor := cleanForFilename(author)
	cleanTitle := cleanForFilename(title)
	if cleanTitle == "" {
		cleanTitle = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}
	if cleanAuthor == "" {
		cleanAuthor = "Unknown"
	}

	// Build new filename from pattern.
	newName := cfg.Pattern
	newName = strings.ReplaceAll(newName, "{author}", cleanAuthor)
	newName = strings.ReplaceAll(newName, "{title}", cleanTitle)
	newName = strings.ReplaceAll(newName, "{ext}", ext)

	// Handle year: if no year provided, remove the year placeholder and surrounding parens.
	if year != "" {
		newName = strings.ReplaceAll(newName, "{year}", year)
	} else {
		// Remove patterns like " ({year})" or " {year}".
		newName = strings.ReplaceAll(newName, " ({year})", "")
		newName = strings.ReplaceAll(newName, "({year})", "")
		newName = strings.ReplaceAll(newName, "{year}", "")
	}

	// Final cleanup.
	newName = strings.TrimSpace(newName)
	newName = strings.TrimRight(newName, ".-")
	newName = strings.TrimSpace(newName)

	if newName == "" || newName == "."+ext {
		return filePath, nil // Can't rename to empty.
	}

	// Ensure extension is present.
	if !strings.HasSuffix(newName, "."+ext) {
		newName += "." + ext
	}

	newPath := filepath.Join(dir, newName)

	// If same path, nothing to do.
	if newPath == filePath {
		return filePath, nil
	}

	// Handle collisions: append (1), (2), etc.
	newPath = handleCollision(newPath)

	if err := os.Rename(filePath, newPath); err != nil {
		return filePath, fmt.Errorf("rename failed: %w", err)
	}
	return newPath, nil
}

// cleanForFilename removes problematic characters and applies title case.
func cleanForFilename(name string) string {
	if name == "" {
		return ""
	}

	// Remove characters that are unsafe in filenames.
	unsafe := regexp.MustCompile(`[<>:"/\\|?*]`)
	name = unsafe.ReplaceAllString(name, "")

	// Collapse multiple spaces.
	spaces := regexp.MustCompile(`\s+`)
	name = spaces.ReplaceAllString(name, " ")

	name = strings.TrimSpace(name)

	// Apply title case.
	name = titleCase(name)

	return name
}

// titleCase converts a string to Title Case, preserving small words in the middle.
func titleCase(s string) string {
	smallWords := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "but": true,
		"or": true, "for": true, "nor": true, "on": true, "at": true,
		"to": true, "by": true, "in": true, "of": true, "with": true,
		"is": true,
	}

	words := strings.Fields(s)
	for i, w := range words {
		lower := strings.ToLower(w)
		if i == 0 || !smallWords[lower] {
			// Capitalize first letter.
			runes := []rune(lower)
			if len(runes) > 0 {
				runes[0] = unicode.ToUpper(runes[0])
			}
			words[i] = string(runes)
		} else {
			words[i] = lower
		}
	}
	return strings.Join(words, " ")
}

// handleCollision appends (1), (2), etc. if the file already exists.
func handleCollision(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)

	for i := 1; i <= 100; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return path // give up after 100 collisions
}
