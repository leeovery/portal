package tui

import (
	"testing"
)

// TestInjectSGRResets covers the pure helper that appends '\x1b[0m' (SGR
// reset) to every non-empty line of the scrollback body before frame
// composition, protecting the preview frame's right border from
// unterminated SGR sequences bleeding into the border colour. See
// specification.md § SGR reset injection.
//
// Empty lines (including the trailing empty element produced by a
// terminating newline) are left untouched. The helper is idempotent in
// observable behaviour because terminals collapse '\x1b[0m\x1b[0m' to a
// single reset.
func TestInjectSGRResets(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "line ending in unterminated SGR gets reset appended",
			input: "\x1b[41mhello",
			want:  "\x1b[41mhello\x1b[0m",
		},
		{
			name:  "line already ending in reset gets a second reset",
			input: "hello\x1b[0m",
			want:  "hello\x1b[0m\x1b[0m",
		},
		{
			name:  "empty line in middle stays empty",
			input: "foo\n\nbar",
			want:  "foo\x1b[0m\n\nbar\x1b[0m",
		},
		{
			name:  "whitespace-only line with embedded SGR gets reset",
			input: " \x1b[42m ",
			want:  " \x1b[42m \x1b[0m",
		},
		{
			name:  "trailing newline produces empty trailing element which is ignored",
			input: "foo\n",
			want:  "foo\x1b[0m\n",
		},
		{
			name:  "fully empty input returns fully empty output",
			input: "",
			want:  "",
		},
		{
			name:  "multi-line input gets reset on every non-empty line",
			input: "a\nb\nc",
			want:  "a\x1b[0m\nb\x1b[0m\nc\x1b[0m",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := injectSGRResets(tc.input)
			if got != tc.want {
				t.Errorf("injectSGRResets(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
