package db

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// Silence slog during test runs — db.New emits an INFO log on each test
// fixture setup, which appears as noise in `go test -v` output but is not a
// test failure.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}
