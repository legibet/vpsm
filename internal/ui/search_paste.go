package ui

import "unicode"

func normalizeSearchPaste(content string) string {
	buf := make([]rune, 0, len(content))
	for _, r := range content {
		switch r {
		case '\r', '\n', '\t':
			buf = append(buf, ' ')
		default:
			if !unicode.IsControl(r) {
				buf = append(buf, r)
			}
		}
	}
	return string(buf)
}
