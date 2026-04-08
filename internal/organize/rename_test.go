package organize

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanForFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"normal text", "John Smith", "John Smith"},
		{"removes angle brackets", "Book <1>", "Book 1"},
		{"removes colons", "Title: Subtitle", "Title Subtitle"},
		{"removes quotes", `He said "hello"`, "He Said Hello"},
		{"removes backslash", `Path\Name`, "Pathname"},
		{"removes pipe", "Author | Publisher", "Author Publisher"},
		{"removes question mark", "What?", "What"},
		{"removes asterisk", "Star*Wars", "Starwars"},
		{"collapses spaces", "  Too   Many   Spaces  ", "Too Many Spaces"},
		{"applies title case", "the lord of the rings", "The Lord of the Rings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanForFilename(tt.input)
			if result != tt.expected {
				t.Errorf("cleanForFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"single word", "hello", "Hello"},
		{"already capitalized", "Hello", "Hello"},
		{"all caps normalized", "HELLO WORLD", "Hello World"},
		{"small words in middle", "the lord of the rings", "The Lord of the Rings"},
		{"first word always capitalized", "a tale of two cities", "A Tale of Two Cities"},
		{"prepositions lowercase", "war and peace", "War and Peace"},
		{"mixed small words", "to kill a mockingbird", "To Kill a Mockingbird"},
		{"by in middle", "written by hand", "Written by Hand"},
		{"for in middle", "battle for the ages", "Battle for the Ages"},
		{"single letter", "a", "A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := titleCase(tt.input)
			if result != tt.expected {
				t.Errorf("titleCase(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestHandleCollision(t *testing.T) {
	dir := t.TempDir()

	t.Run("no collision returns same path", func(t *testing.T) {
		path := filepath.Join(dir, "unique.epub")
		result := handleCollision(path)
		if result != path {
			t.Errorf("expected %q, got %q", path, result)
		}
	})

	t.Run("first collision appends (1)", func(t *testing.T) {
		path := filepath.Join(dir, "existing.epub")
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}

		result := handleCollision(path)
		expected := filepath.Join(dir, "existing (1).epub")
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("second collision appends (2)", func(t *testing.T) {
		path := filepath.Join(dir, "existing.epub")
		// existing.epub already created above
		path1 := filepath.Join(dir, "existing (1).epub")
		if err := os.WriteFile(path1, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}

		result := handleCollision(path)
		expected := filepath.Join(dir, "existing (2).epub")
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})
}

func TestApplyRenamePattern(t *testing.T) {
	dir := t.TempDir()

	t.Run("disabled config returns original", func(t *testing.T) {
		path := filepath.Join(dir, "original.epub")
		os.WriteFile(path, []byte("test"), 0644)

		cfg := RenameConfig{Enabled: false, Pattern: "{author} - {title}.{ext}"}
		result, err := RenameFile(path, cfg, "Author", "Title", "2020")
		if err != nil {
			t.Fatal(err)
		}
		if result != path {
			t.Errorf("expected original path when disabled, got %q", result)
		}
	})

	t.Run("empty pattern returns original", func(t *testing.T) {
		path := filepath.Join(dir, "original2.epub")
		os.WriteFile(path, []byte("test"), 0644)

		cfg := RenameConfig{Enabled: true, Pattern: ""}
		result, err := RenameFile(path, cfg, "Author", "Title", "2020")
		if err != nil {
			t.Fatal(err)
		}
		if result != path {
			t.Errorf("expected original path with empty pattern, got %q", result)
		}
	})

	t.Run("basic pattern substitution", func(t *testing.T) {
		path := filepath.Join(dir, "input.epub")
		os.WriteFile(path, []byte("test content"), 0644)

		cfg := RenameConfig{Enabled: true, Pattern: "{author} - {title} ({year}).{ext}"}
		result, err := RenameFile(path, cfg, "Frank Herbert", "Dune", "1965")
		if err != nil {
			t.Fatal(err)
		}

		expected := filepath.Join(dir, "Frank Herbert - Dune (1965).epub")
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}

		// Verify the file actually moved.
		if _, err := os.Stat(result); os.IsNotExist(err) {
			t.Error("renamed file does not exist")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("original file still exists after rename")
		}
	})

	t.Run("missing year removes year placeholder", func(t *testing.T) {
		path := filepath.Join(dir, "noyear.epub")
		os.WriteFile(path, []byte("test"), 0644)

		cfg := RenameConfig{Enabled: true, Pattern: "{author} - {title} ({year}).{ext}"}
		result, err := RenameFile(path, cfg, "Jane Austen", "Pride", "")
		if err != nil {
			t.Fatal(err)
		}

		expected := filepath.Join(dir, "Jane Austen - Pride.epub")
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("empty author defaults to Unknown", func(t *testing.T) {
		path := filepath.Join(dir, "noauthor.epub")
		os.WriteFile(path, []byte("test"), 0644)

		cfg := RenameConfig{Enabled: true, Pattern: "{author} - {title}.{ext}"}
		result, err := RenameFile(path, cfg, "", "Some Title", "")
		if err != nil {
			t.Fatal(err)
		}

		expected := filepath.Join(dir, "Unknown - Some Title.epub")
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("collision handling on rename", func(t *testing.T) {
		// Create the target file first.
		targetPath := filepath.Join(dir, "Author - Book.epub")
		os.WriteFile(targetPath, []byte("existing"), 0644)

		path := filepath.Join(dir, "source.epub")
		os.WriteFile(path, []byte("new content"), 0644)

		cfg := RenameConfig{Enabled: true, Pattern: "{author} - {title}.{ext}"}
		result, err := RenameFile(path, cfg, "Author", "Book", "")
		if err != nil {
			t.Fatal(err)
		}

		expected := filepath.Join(dir, "Author - Book (1).epub")
		if result != expected {
			t.Errorf("expected collision path %q, got %q", expected, result)
		}
	})
}
