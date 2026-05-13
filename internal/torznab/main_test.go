package torznab

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// Silence slog during test runs — handler emits INFO logs on every torznab
// search call, which appear as noise in `go test -v` output but are not test
// failures.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}
