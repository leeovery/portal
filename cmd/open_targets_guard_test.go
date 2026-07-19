package cmd

// Drift guard for openTargetPins. orderedOpenTargets (open_targets.go) recovers
// left-to-right target order from raw argv by classifying each flag token against
// the static openTargetPins map, which is structurally decoupled from openCmd's
// live cobra flag set. A value-taking flag added to openCmd but not mirrored into
// openTargetPins would be treated as arity-0, misrouting its VALUE as a bare
// positional target. This file walks the live flag set and fails loudly on that
// drift. No package-level state, no cobra Execute, no tmux — but package cmd, so
// per CLAUDE.md it MUST NOT use t.Parallel.

import (
	"slices"
	"testing"

	"github.com/spf13/pflag"
)

// valueTakingFlagMissingPins returns the openTargetPins keys a value-taking flag
// REQUIRES but that `pins` lacks (its "--long" form, plus its "-short" form when
// it has a shorthand). A pflag takes a value unless it is a bool or carries a
// non-empty NoOptDefVal (arity-0 / optional-value); such a flag is correctly
// skipped by orderedOpenTargets, so this predicate returns nil for it. A
// value-taking flag already fully covered by `pins` also returns nil. It is the
// shared predicate driving both the live-openCmd guard and its drift unit below.
func valueTakingFlagMissingPins(f *pflag.Flag, pins map[string]string) []string {
	if f.Value.Type() == "bool" || f.NoOptDefVal != "" {
		return nil
	}
	var missing []string
	if _, ok := pins["--"+f.Name]; !ok {
		missing = append(missing, "--"+f.Name)
	}
	if f.Shorthand != "" {
		if _, ok := pins["-"+f.Shorthand]; !ok {
			missing = append(missing, "-"+f.Shorthand)
		}
	}
	return missing
}

func TestValueTakingFlagMissingPins_DetectsDrift(t *testing.T) {
	// A crafted flag set — the real openCmd is never mutated (a pflag.FlagSet can
	// not cleanly un-register a flag). "zzz"/"Z" is value-taking and absent from
	// openTargetPins, so the predicate must report BOTH forms missing.
	fs := pflag.NewFlagSet("crafted", pflag.ContinueOnError)
	fs.StringP("zzz", "Z", "", "throwaway value-taking flag absent from openTargetPins")

	got := valueTakingFlagMissingPins(fs.Lookup("zzz"), openTargetPins)
	want := []string{"--zzz", "-Z"}
	if !slices.Equal(got, want) {
		t.Errorf("valueTakingFlagMissingPins(--zzz/-Z) = %#v, want %#v — a value-taking flag missing from openTargetPins must be flagged", got, want)
	}
}

func TestValueTakingFlagMissingPins_SkipsAndCovers(t *testing.T) {
	fs := pflag.NewFlagSet("crafted", pflag.ContinueOnError)
	fs.BoolP("verbose", "v", false, "bool flag — arity-0, correctly skipped")
	fs.String("opt", "", "optional-value flag — skipped via NoOptDefVal")
	fs.Lookup("opt").NoOptDefVal = "sentinel"
	fs.StringP("session", "s", "", "value-taking flag already present in openTargetPins")

	// A bool flag is arity-0 and must be skipped (nil), never flagged.
	if got := valueTakingFlagMissingPins(fs.Lookup("verbose"), openTargetPins); got != nil {
		t.Errorf("bool flag --verbose should be skipped, got %#v", got)
	}
	// An optional-value flag (NoOptDefVal set) is likewise skipped.
	if got := valueTakingFlagMissingPins(fs.Lookup("opt"), openTargetPins); got != nil {
		t.Errorf("optional-value flag --opt should be skipped, got %#v", got)
	}
	// A value-taking flag fully covered by openTargetPins must NOT false-positive.
	if got := valueTakingFlagMissingPins(fs.Lookup("session"), openTargetPins); got != nil {
		t.Errorf("fully-pinned flag --session/-s should report nothing missing, got %#v", got)
	}
}

// TestOpenTargetPinsCoverValueTakingFlags is the live drift guard: it walks
// openCmd's real cobra flag set and fails if any value-taking flag is missing
// from openTargetPins. It passes for the current 7 flags (exec/filter/session/
// path/alias/zoxide/ack) and fails loudly the moment a value-taking flag is
// added to openCmd without a matching openTargetPins entry.
func TestOpenTargetPinsCoverValueTakingFlags(t *testing.T) {
	openCmd.Flags().VisitAll(func(f *pflag.Flag) {
		for _, key := range valueTakingFlagMissingPins(f, openTargetPins) {
			t.Errorf("openCmd flag --%s is value-taking but %q is absent from openTargetPins — orderedOpenTargets would misroute its value as a positional target; add it to openTargetPins", f.Name, key)
		}
	})
}
