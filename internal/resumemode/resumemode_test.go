package resumemode_test

import (
	"testing"

	"github.com/leeovery/portal/internal/resumemode"
)

func TestParse(t *testing.T) {
	t.Run("it recognises the two on-disk spellings and nothing else", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  resumemode.Mode
			ok    bool
		}{
			{name: "eager", input: "eager", want: resumemode.Eager, ok: true},
			{name: "lazy", input: "lazy", want: resumemode.Lazy, ok: true},
			{name: "capitalised", input: "Lazy"},
			{name: "upper case", input: "LAZY"},
			{name: "leading space", input: " lazy"},
			{name: "trailing space", input: "lazy "},
			{name: "prefix", input: "laz"},
			{name: "suffixed", input: "lazyy"},
			{name: "empty", input: ""},
			{name: "numeric", input: "1"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, ok := resumemode.Parse(tt.input)
				if ok != tt.ok {
					t.Fatalf("Parse(%q) ok = %v, want %v", tt.input, ok, tt.ok)
				}
				want := tt.want
				if !tt.ok {
					want = resumemode.Unset
				}
				if got != want {
					t.Errorf("Parse(%q) = %q, want %q", tt.input, got, want)
				}
			})
		}
	})
}

func TestModeString(t *testing.T) {
	t.Run("it renders an unset mode as the empty string", func(t *testing.T) {
		if got := resumemode.Unset.String(); got != "" {
			t.Errorf("Unset.String() = %q, want %q", got, "")
		}
		outOfVocabulary := resumemode.Mode(99)
		if got := outOfVocabulary.String(); got != "" {
			t.Errorf("Mode(99).String() = %q, want %q", got, "")
		}
	})

	t.Run("it round-trips each named mode through String and Parse", func(t *testing.T) {
		for _, mode := range []resumemode.Mode{resumemode.Eager, resumemode.Lazy} {
			spelling := mode.String()
			if spelling == "" {
				t.Fatalf("%q.String() = %q, want a non-empty on-disk spelling", mode, spelling)
			}
			got, ok := resumemode.Parse(spelling)
			if !ok {
				t.Fatalf("Parse(%q) refused the spelling %q renders", spelling, mode)
			}
			if got != mode {
				t.Errorf("Parse(%q) = %q, want %q", spelling, got, mode)
			}
		}
	})
}

func TestResolve(t *testing.T) {
	t.Run("it lets a registration's mode win over the install's", func(t *testing.T) {
		tests := []struct {
			name         string
			registration resumemode.Mode
			install      resumemode.Mode
			want         resumemode.Mode
		}{
			{name: "eager registration under a lazy install", registration: resumemode.Eager, install: resumemode.Lazy, want: resumemode.Eager},
			{name: "lazy registration under an eager install", registration: resumemode.Lazy, install: resumemode.Eager, want: resumemode.Lazy},
			{name: "eager registration with no install mode", registration: resumemode.Eager, install: resumemode.Unset, want: resumemode.Eager},
			{name: "lazy registration with no install mode", registration: resumemode.Lazy, install: resumemode.Unset, want: resumemode.Lazy},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := resumemode.Resolve(tt.registration, tt.install); got != tt.want {
					t.Errorf("Resolve(%q, %q) = %q, want %q", tt.registration, tt.install, got, tt.want)
				}
			})
		}
	})

	t.Run("it falls back to the install's mode when the registration names none", func(t *testing.T) {
		for _, install := range []resumemode.Mode{resumemode.Eager, resumemode.Lazy} {
			if got := resumemode.Resolve(resumemode.Unset, install); got != install {
				t.Errorf("Resolve(Unset, %q) = %q, want %q", install, got, install)
			}
		}
	})

	t.Run("it answers lazy when neither the registration nor the install names a mode", func(t *testing.T) {
		got := resumemode.Resolve(resumemode.Unset, resumemode.Unset)
		if got != resumemode.Lazy {
			t.Errorf("Resolve(Unset, Unset) = %q, want %q", got, resumemode.Lazy)
		}
		if got != resumemode.Default {
			t.Errorf("Resolve(Unset, Unset) = %q, want the shipped Default %q", got, resumemode.Default)
		}
	})
}
