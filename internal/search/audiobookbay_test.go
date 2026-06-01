package search

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/JeremiahM37/librarr/internal/config"
	"github.com/JeremiahM37/librarr/internal/sources/sourcestest"
	"github.com/PuerkitoBio/goquery"
)

type abbRoundTripper struct {
	body string
	req  *http.Request
}

func (r *abbRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.req = req.Clone(req.Context())
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Request:    req,
	}, nil
}

func TestExtractABBInfoHash(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<html><body>
		<table>
			<tr>
				<td>Info Hash:</td>
				<td>0123456789ABCDEF0123456789ABCDEF01234567</td>
			</tr>
		</table>
	</body></html>`))
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}

	got := extractABBInfoHash(doc)
	want := "0123456789ABCDEF0123456789ABCDEF01234567"
	if got != want {
		t.Fatalf("extractABBInfoHash = %q, want %q", got, want)
	}
}

func TestResolveABBMagnetUsesInfoHashRow(t *testing.T) {
	rt := &abbRoundTripper{
		body: `<html><body>
		<h1>The Martian</h1>
		<table>
			<tr>
				<td>Info Hash:</td>
				<td>0123456789ABCDEF0123456789ABCDEF01234567</td>
			</tr>
			<tr><td>udp://tracker.example:1337/announce</td></tr>
		</table>
		</body></html>`,
	}

	reg, err := sourcestest.Registry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	cfg := &config.Config{
		UserAgent: "test",
		Sources:   reg,
	}
	cfg.Sources.AudioBookBay.Mirrors = []string{"audiobookbay.lu"}
	cfg.Sources.AudioBookBay.Trackers = []string{"udp://fallback.example:1337/announce"}

	client := &http.Client{Transport: rt}
	magnet, err := ResolveABBMagnet(context.Background(), client, cfg.UserAgent, "/abss/the-martian-andy-weir/", cfg.Sources.AudioBookBay.Mirrors, cfg.Sources.AudioBookBay.Trackers)
	if err != nil {
		t.Fatalf("ResolveABBMagnet returned error: %v", err)
	}
	if !strings.HasPrefix(magnet, "magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567") {
		t.Fatalf("unexpected magnet: %s", magnet)
	}
}
