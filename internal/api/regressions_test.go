package api

import (
	"net/http/httptest"
	"testing"
)

// queryBoundedInt rejects out-of-range and unparseable values, returning fallback.
// Regression test for `/api/activity?limit=-1` returning the entire activity log
// (potential DoS / data-exfil) — fuzz-discovered against prod v1.1.0.
func TestQueryBoundedInt(t *testing.T) {
	cases := []struct {
		name, qs string
		want     int
	}{
		{"missing -> fallback", "", 50},
		{"valid in range", "?limit=10", 10},
		{"negative -> fallback", "?limit=-1", 50},
		{"zero -> fallback (below min)", "?limit=0", 50},
		{"above max -> fallback", "?limit=999999", 50},
		{"garbage -> fallback", "?limit=garbage", 50},
		{"empty -> fallback", "?limit=", 50},
		{"min boundary", "?limit=1", 1},
		{"max boundary", "?limit=500", 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/x"+tc.qs, nil)
			got := queryBoundedInt(r, "limit", 50, 1, 500)
			if got != tc.want {
				t.Errorf("queryBoundedInt(%q) = %d, want %d", tc.qs, got, tc.want)
			}
		})
	}
}
