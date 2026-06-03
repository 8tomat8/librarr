package api

import (
	"net/http/httptest"
	"testing"
)

func FuzzQueryBoundedInt(f *testing.F) {
	seeds := []string{"", "1", "500", "-1", "999999", "garbage", "0"}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		r := httptest.NewRequest("GET", "/x?limit="+raw, nil)
		got := queryBoundedInt(r, "limit", 50, 1, 500)
		if got < 1 || got > 500 {
			t.Fatalf("out of bounds: %d for input %q", got, raw)
		}
	})
}

func FuzzTruncateSearchQuery(f *testing.F) {
	f.Add("")
	f.Add("hello")
	f.Add("作者测试")

	f.Fuzz(func(t *testing.T, q string) {
		got := truncateSearchQuery(q)
		if len([]rune(got)) > maxSearchQueryRunes {
			t.Fatalf("truncation failed: len=%d", len([]rune(got)))
		}
	})
}

func FuzzValidateMD5(f *testing.F) {
	f.Add("")
	f.Add("0123456789abcdef0123456789abcdef")
	f.Add("not-a-hash")

	f.Fuzz(func(t *testing.T, md5 string) {
		ok := validateMD5(md5)
		if ok && len(md5) != 32 {
			t.Fatalf("validateMD5 returned true for len=%d", len(md5))
		}
	})
}
