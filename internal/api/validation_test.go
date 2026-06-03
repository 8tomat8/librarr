package api

import (
	"strings"
	"testing"
)

func TestTruncateSearchQuery(t *testing.T) {
	short := "hello world"
	if got := truncateSearchQuery(short); got != short {
		t.Errorf("short query changed: %q", got)
	}
	long := strings.Repeat("x", 600)
	got := truncateSearchQuery(long)
	if len([]rune(got)) != maxSearchQueryRunes {
		t.Errorf("expected %d runes, got %d", maxSearchQueryRunes, len([]rune(got)))
	}
}

func TestValidateMD5(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef"
	if !validateMD5(valid) {
		t.Error("expected valid md5")
	}
	if validateMD5("tooshort") {
		t.Error("expected invalid short md5")
	}
	if validateMD5(strings.Repeat("g", 32)) {
		t.Error("expected invalid hex md5")
	}
}
