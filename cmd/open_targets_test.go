package cmd

// Pure-function tests over orderedOpenTargets. No package-level state, no cobra,
// no tmux — but this file lives in package cmd, so per CLAUDE.md it MUST NOT use
// t.Parallel.

import (
	"slices"
	"testing"
)

func TestOrderedOpenTargets(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []Target
	}{
		{
			name: "space-form pin then positionals, order preserved",
			args: []string{"-s", "api", "~/new", "blog"},
			want: []Target{
				{Value: "api", Domain: "session"},
				{Value: "~/new", Domain: "bare"},
				{Value: "blog", Domain: "bare"},
			},
		},
		{
			name: "equals form short",
			args: []string{"-s=api"},
			want: []Target{{Value: "api", Domain: "session"}},
		},
		{
			name: "equals form long",
			args: []string{"--session=api"},
			want: []Target{{Value: "api", Domain: "session"}},
		},
		{
			name: "exec value excluded",
			args: []string{"-e", "claude", "~/new"},
			want: []Target{{Value: "~/new", Domain: "bare"}},
		},
		{
			// An excluded flag's value BETWEEN two positionals is consumed and
			// dropped, leaving both positionals in left-to-right order — the value
			// ("claude") is never misrouted as a third bare target.
			name: "excluded exec value between two positionals",
			args: []string{"blog", "-e", "claude", "api"},
			want: []Target{
				{Value: "blog", Domain: "bare"},
				{Value: "api", Domain: "bare"},
			},
		},
		{
			name: "everything after -- excluded",
			args: []string{"~/new", "--", "claude", "."},
			want: []Target{{Value: "~/new", Domain: "bare"}},
		},
		{
			name: "repeated pin, no dedup",
			args: []string{"-s", "a", "-s", "b"},
			want: []Target{
				{Value: "a", Domain: "session"},
				{Value: "b", Domain: "session"},
			},
		},
		{
			name: "mixed interleave, order preserved",
			args: []string{"blog", "-p", "~/x", "api", "-a", "work"},
			want: []Target{
				{Value: "blog", Domain: "bare"},
				{Value: "~/x", Domain: "path"},
				{Value: "api", Domain: "bare"},
				{Value: "work", Domain: "alias"},
			},
		},
		{
			name: "filter value excluded",
			args: []string{"-f", "text", "api"},
			want: []Target{{Value: "api", Domain: "bare"}},
		},
		{
			name: "ack value excluded",
			args: []string{"-s", "api", "--ack", "b:t"},
			want: []Target{{Value: "api", Domain: "session"}},
		},
		{
			name: "single bare target",
			args: []string{"blog"},
			want: []Target{{Value: "blog", Domain: "bare"}},
		},
		{
			name: "single session pin",
			args: []string{"-s", "api"},
			want: []Target{{Value: "api", Domain: "session"}},
		},
		{
			name: "zoxide pin space form",
			args: []string{"-z", "prj"},
			want: []Target{{Value: "prj", Domain: "zoxide"}},
		},
		{
			name: "path pin equals form long",
			args: []string{"--path=~/Code/new"},
			want: []Target{{Value: "~/Code/new", Domain: "path"}},
		},
		{
			name: "alias pin equals form short",
			args: []string{"-a=work"},
			want: []Target{{Value: "work", Domain: "alias"}},
		},
		{
			name: "exec equals form excluded",
			args: []string{"-e=claude", "~/new"},
			want: []Target{{Value: "~/new", Domain: "bare"}},
		},
		{
			name: "long exec value excluded",
			args: []string{"--exec", "npm run dev", "~/new"},
			want: []Target{{Value: "~/new", Domain: "bare"}},
		},
		{
			name: "no args yields empty list",
			args: []string{},
			want: nil,
		},
		{
			name: "unknown flag skipped, no value consumed",
			args: []string{"--dry-run", "blog"},
			want: []Target{{Value: "blog", Domain: "bare"}},
		},
		{
			// Panic-safety edge: a trailing value-pin with no following token must
			// not index-out-of-range. The guard emits an empty-Value target of the
			// pin's domain (the current, correct behaviour); cobra rejects this
			// input upstream in practice, so this only pins no-panic + emitted shape.
			name: "trailing value-pin, no following token, no panic",
			args: []string{"-p", "~/a", "-s"},
			want: []Target{
				{Value: "~/a", Domain: "path"},
				{Value: "", Domain: "session"},
			},
		},
		{
			name: "single trailing value-pin, no value, no panic",
			args: []string{"-s"},
			want: []Target{{Value: "", Domain: "session"}},
		},
		{
			name: "filter equals form excluded",
			args: []string{"-f=text", "api"},
			want: []Target{{Value: "api", Domain: "bare"}},
		},
		{
			name: "ack equals form excluded",
			args: []string{"-s", "api", "--ack=b:t"},
			want: []Target{{Value: "api", Domain: "session"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := orderedOpenTargets(tt.args)
			if !slices.Equal(got, tt.want) {
				t.Errorf("orderedOpenTargets(%q) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}
