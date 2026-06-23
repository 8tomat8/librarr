package api

import "unicode/utf8"

const maxSearchQueryRunes = 512

// truncateSearchQuery caps search query length to prevent abuse.
func truncateSearchQuery(q string) string {
	if utf8.RuneCountInString(q) <= maxSearchQueryRunes {
		return q
	}
	runes := []rune(q)
	return string(runes[:maxSearchQueryRunes])
}

// validateMD5 checks Anna's Archive MD5 hash format.
func validateMD5(md5 string) bool {
	if len(md5) != 32 {
		return false
	}
	for _, c := range md5 {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}
