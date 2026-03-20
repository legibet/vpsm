package ui

import "testing"

func TestNormalizeSearchPaste(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain text", input: "prod", want: "prod"},
		{name: "newline to space", input: "hong\nkong", want: "hong kong"},
		{name: "carriage return and tab to spaces", input: "beta\r\nlogs\t2026", want: "beta  logs 2026"},
		{name: "drops other controls", input: "pro\x00d", want: "prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeSearchPaste(tt.input); got != tt.want {
				t.Fatalf("normalizeSearchPaste(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
