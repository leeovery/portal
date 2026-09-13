package resolver_test

import (
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

func TestMatchesSearchTerm(t *testing.T) {
	home := t.TempDir()

	tests := []struct {
		name        string
		term        string
		session     string
		recordedDir string
		want        bool
	}{
		{
			name:    "it matches a term contained in the session name",
			term:    "port",
			session: "portal-a1b2",
			want:    true,
		},
		{
			name:    "it matches case-insensitively with an upper-cased term",
			term:    "PORT",
			session: "portal-a1b2",
			want:    true,
		},
		{
			name:    "it matches case-insensitively with an upper-cased name",
			term:    "port",
			session: "PORTAL-A1B2",
			want:    true,
		},
		{
			name:        "it matches a term contained in the home-abbreviated recorded directory",
			term:        "portal",
			session:     "api-work",
			recordedDir: filepath.Join(home, "Code", "portal"),
			want:        true,
		},
		{
			name:        "it matches the tilde the abbreviation introduced",
			term:        "~/code",
			session:     "api-work",
			recordedDir: filepath.Join(home, "Code", "portal"),
			want:        true,
		},
		{
			name:        "it does not match the home directory's last segment the abbreviation replaced",
			term:        filepath.Base(home),
			session:     "api-work",
			recordedDir: filepath.Join(home, "Code", "portal"),
			want:        false,
		},
		{
			name:        "it does not match the Users prefix the abbreviation replaced",
			term:        "Users",
			session:     "api-work",
			recordedDir: filepath.Join(home, "Code", "portal"),
			want:        false,
		},
		{
			name:        "it leaves a directory outside the home directory unabbreviated",
			term:        "opt",
			session:     "api-work",
			recordedDir: "/opt/tools",
			want:        true,
		},
		{
			name:        "it does not match a run that spans the name and the directory",
			term:        "i /o",
			session:     "api",
			recordedDir: "/opt/tools",
			want:        false,
		},
		{
			name:        "it matches on the name alone when the recorded directory is empty",
			term:        "api",
			session:     "api-work",
			recordedDir: "",
			want:        true,
		},
		{
			name:        "it does not match on an empty recorded directory",
			term:        "tools",
			session:     "api-work",
			recordedDir: "",
			want:        false,
		},
		{
			name:    "it treats a glob metacharacter in the term as a literal character",
			term:    "po*rt",
			session: "po*rt-x",
			want:    true,
		},
		{
			name:    "it does not expand a glob metacharacter in the term",
			term:    "po*",
			session: "portal-a1b2",
			want:    false,
		},
		{
			name:    "it treats a question mark in the term as a literal character",
			term:    "po?t",
			session: "portal-a1b2",
			want:    false,
		},
		{
			name:    "it treats a bracket in the term as a literal character",
			term:    "po[rs]t",
			session: "portal-a1b2",
			want:    false,
		},
		{
			name:        "it returns false for an empty term",
			term:        "",
			session:     "portal-a1b2",
			recordedDir: filepath.Join(home, "Code", "portal"),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)

			got := resolver.MatchesSearchTerm(tt.term, tt.session, tt.recordedDir)
			if got != tt.want {
				t.Errorf("MatchesSearchTerm(%q, %q, %q) = %v, want %v", tt.term, tt.session, tt.recordedDir, got, tt.want)
			}
		})
	}
}

func TestMatchesSearchTermUnresolvableHome(t *testing.T) {
	t.Setenv("HOME", "")

	if !resolver.MatchesSearchTerm("Users", "api-work", "/Users/leeovery/Code/portal") {
		t.Error("MatchesSearchTerm should compare the raw recorded path when the home directory cannot be resolved")
	}
}
