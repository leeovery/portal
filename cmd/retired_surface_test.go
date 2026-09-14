package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var retiredVerbs = []string{"attach", "spawn"}

// Hidden descendants are included, so a back-compat shim hidden anywhere in the
// tree is still enumerated.
func allCommandsInTree(c *cobra.Command) []*cobra.Command {
	out := []*cobra.Command{c}
	for _, child := range c.Commands() {
		out = append(out, allCommandsInTree(child)...)
	}
	return out
}

func TestRetiredSurface_NoChildNamedAttachOrSpawn(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		for _, verb := range retiredVerbs {
			if c.Name() == verb {
				t.Errorf("%q must be retired — found a child command named %q on rootCmd", verb, verb)
			}
		}
	}
}

func TestRetiredSurface_NoAliasResolvesToRetiredVerbs(t *testing.T) {
	for _, c := range allCommandsInTree(rootCmd) {
		for _, alias := range c.Aliases {
			for _, verb := range retiredVerbs {
				if alias == verb {
					t.Errorf("command %q carries a back-compat alias %q — attach/spawn must take no alias", c.Name(), verb)
				}
			}
		}
	}

	// For an unmatched leading token, Find stops at rootCmd and leaves the token
	// in the remaining args; a silent alias would resolve to the aliased command
	// instead. That difference is what distinguishes deleted from aliased.
	for _, verb := range retiredVerbs {
		found, rest, _ := rootCmd.Find([]string{verb})
		if found != rootCmd {
			t.Errorf("Find([%q]) resolved to command %q — expected the unmatched-token fall-through to rootCmd (verb is deleted, not aliased)", verb, found.Name())
		}
		if len(rest) != 1 || rest[0] != verb {
			t.Errorf("Find([%q]) consumed the token (rest=%v) — an unmatched retired verb must survive as a remaining arg", verb, rest)
		}
	}
}

func TestRetiredSurface_AbsentFromHelp(t *testing.T) {
	names := availableCommandNames(rootCmd.UsageString())
	if len(names) == 0 {
		t.Fatal("no commands parsed from Available Commands section — parser or usage layout changed")
	}
	for _, verb := range retiredVerbs {
		if names[verb] {
			t.Errorf("retired verb %q listed as an Available Command in help", verb)
		}
	}
}

// Keyed on the subcommand-offering form the generator emits rather than a loose
// substring, so an unrelated mention of the word cannot false-positive.
func TestRetiredSurface_AbsentFromCompletion(t *testing.T) {
	buf := new(bytes.Buffer)
	if err := rootCmd.GenBashCompletion(buf); err != nil {
		t.Fatalf("GenBashCompletion: %v", err)
	}
	out := buf.String()
	for _, verb := range retiredVerbs {
		offering := fmt.Sprintf("commands+=(%q)", verb)
		if strings.Contains(out, offering) {
			t.Errorf("bash completion offers retired verb as a subcommand (%s)", offering)
		}
	}
}

func TestRetiredSurface_AbsorbedBehavioursReachableViaOpen(t *testing.T) {
	if openCmd.Flags().Lookup("session") == nil {
		t.Error("open must expose --session (absorbs attach's exact/no-guess attach)")
	}

	ack := openCmd.Flags().Lookup("ack")
	if ack == nil {
		t.Fatal("open must expose --ack (spawned host-window receipt flag)")
	}
	if !ack.Hidden {
		t.Error("open --ack must be hidden (internal receipt flag, absent from --help/completion)")
	}

	// Behaviour, not validator identity, so a wrapper validator stays acceptable.
	if openCmd.Args != nil {
		if err := openCmd.Args(openCmd, []string{"a", "b"}); err != nil {
			t.Errorf("open must admit >=2 positional targets (multi-target burst); Args rejected 2 positionals: %v", err)
		}
	}
}

// Keywords rather than a golden string, so accurate copy edits do not churn
// this.
func TestOpenHelpMetadata_DescribesRedesignedVerb(t *testing.T) {
	if strings.Contains(openCmd.Use, "destination") {
		t.Errorf("openCmd.Use still implies a single [destination]: %q", openCmd.Use)
	}

	short := strings.ToLower(openCmd.Short)
	if strings.Contains(short, "at a path") {
		t.Errorf("openCmd.Short still reads like the single-path pre-redesign command: %q", openCmd.Short)
	}
	if !strings.Contains(short, "target") && !strings.Contains(short, "portal") {
		t.Errorf("openCmd.Short does not name the multi-target surface: %q", openCmd.Short)
	}
	if !strings.Contains(short, "picker") {
		t.Errorf("openCmd.Short does not mention the interactive picker: %q", openCmd.Short)
	}

	if openCmd.Long == "" {
		t.Fatal("openCmd.Long is empty; expected a description of the redesigned surface")
	}
	long := strings.ToLower(openCmd.Long)
	for _, want := range []string{
		"picker",
		"session",
		"attach",
		"mint",
		"-s",
		"-p",
		"-a",
		"-z",
		"-f",
		"-e",
		"--",
		"precedence",
		"/term",
		"search",
	} {
		if !strings.Contains(long, want) {
			t.Errorf("openCmd.Long omits %q; full surface not described:\n%s", want, openCmd.Long)
		}
	}
	if !strings.Contains(long, "window") && !strings.Contains(long, "surface") {
		t.Errorf("openCmd.Long does not describe multi-target opening:\n%s", openCmd.Long)
	}
}
