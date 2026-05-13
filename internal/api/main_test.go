package api

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// Silence slog during test runs — handlers emit INFO/ERROR logs (database
// init, settings persistence) that look like noise in `go test -v` output
// but are not test failures.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}
