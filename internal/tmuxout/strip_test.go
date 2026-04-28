package tmuxout_test

import (
	"testing"

	"github.com/leeovery/portal/internal/tmuxout"
)

func TestStripMatchedOuterQuotes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strips a matched pair of double quotes",
			in:   `"hello"`,
			want: "hello",
		},
		{
			name: "strips a matched pair of single quotes",
			in:   `'hello'`,
			want: "hello",
		},
		{
			name: "preserves inner content verbatim inside double quotes",
			in:   `"a b\tc"`,
			want: `a b\tc`,
		},
		{
			name: "preserves inner content verbatim inside single quotes",
			in:   `'run-shell "portal state notify"'`,
			want: `run-shell "portal state notify"`,
		},
		{
			name: "leaves asymmetric leading double / trailing single unchanged",
			in:   `"foo'`,
			want: `"foo'`,
		},
		{
			name: "leaves asymmetric leading single / trailing double unchanged",
			in:   `'foo"`,
			want: `'foo"`,
		},
		{
			name: "leaves a value with only a leading quote unchanged",
			in:   `"foo`,
			want: `"foo`,
		},
		{
			name: "leaves a value with only a trailing quote unchanged",
			in:   `foo"`,
			want: `foo"`,
		},
		{
			name: "returns empty string unchanged",
			in:   ``,
			want: ``,
		},
		{
			name: "returns a single double-quote unchanged",
			in:   `"`,
			want: `"`,
		},
		{
			name: "returns a single single-quote unchanged",
			in:   `'`,
			want: `'`,
		},
		{
			name: "treats two double quotes as an empty quoted string",
			in:   `""`,
			want: ``,
		},
		{
			name: "treats two single quotes as an empty quoted string",
			in:   `''`,
			want: ``,
		},
		{
			name: "is idempotent on unquoted input",
			in:   `bareword`,
			want: `bareword`,
		},
		{
			name: "only strips one outer pair (nested double quotes preserved)",
			in:   `""inner""`,
			want: `"inner"`,
		},
		{
			name: "only strips one outer pair (nested single quotes preserved)",
			in:   `''inner''`,
			want: `'inner'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tmuxout.StripMatchedOuterQuotes(tt.in)
			if got != tt.want {
				t.Errorf("StripMatchedOuterQuotes(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
