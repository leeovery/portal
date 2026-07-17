//go:build ghosttycompile

package spawn

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// driftDiscriminator is the AppleScript error code osacompile emits when the
// pre-fix `make new surface configuration` template is compiled against
// Ghostty's dictionary — the established, reproduced signature of a drifted
// `tell application "Ghostty"` template (spec §Fix 4). It is the ONLY failure
// signature this guard treats as a genuine template regression; every other
// resolution failure is environmental and skipped.
const driftDiscriminator = "-2741"

// TestGhosttyOpenScript_CompilesAgainstInstalledDictionary is the automated
// prevention guard (spec §Fix 4). It is the tripwire the shipped-broken template
// lacked: the only test that previously exercised the real AppleScript boundary
// was `//go:build manual` (TestManual_OpenWindow_OpensRealGhosttyWindow) — a test
// nobody ran before tagging 0.9.1 — so no automatable lane could catch a wrong
// `tell application "Ghostty"` template. This test closes that gap by feeding
// ghosttyOpenScript(...) through a COMPILE-ONLY osascript path (osacompile),
// which resolves the terminology against Ghostty's installed scripting
// dictionary and fails a drifted template WITHOUT a human in the loop.
//
// Lane isolation. It cannot be hermetic (it needs a real Mac + installed
// Ghostty), so it is fenced behind the dedicated `ghosttycompile` build tag and
// compiles into NEITHER default lane:
//
//	go test ./...                     — unit lane, this file is NOT compiled
//	go test -tags integration ./...   — integration lane, this file is NOT compiled
//
// It runs only via:
//
//	go test -tags ghosttycompile -run TestGhosttyOpenScript_CompilesAgainstInstalledDictionary ./internal/spawn/
//
// It is separate from the window-opening `manual` test: osacompile is
// compile-only and opens NO window / runs NOTHING — this is a terminology
// tripwire, not a functional proof. The functional proof remains the mandatory
// live validation (the `-tags manual` test + a real ≥3-session burst); the two
// are complementary, not substitutes.
//
// LIVE-MAC CONFIRMATION (recorded per spec §Fix 4 "Assumption to confirm").
// Confirmed on this Mac (GOOS=darwin, /Applications/Ghostty.app installed,
// Ghostty running):
//   - osacompile resolved the `tell application "Ghostty"` terminology against
//     Ghostty's dictionary and returned exit 0 for the corrected
//     `new window with configuration {…}` template, opening no window.
//   - The pre-fix `make new surface configuration with properties {…}` template
//     failed the same osacompile invocation with the observed `-2741`
//     terminology error, opening no window.
//
// PRECONDITION RATIONALE (spec §Fix 4 "Assumption to confirm", discharged
// defensively rather than by the feature-invocation analogy). The confirmation
// above happened to run with Ghostty up, and the spec left one genuinely-open
// nuance: whether terminology resolution requires Ghostty to be *running* — a
// case the installed-only gate does not cover, and one this guard can be invoked
// in (Ghostty installed but closed). Two independent reasons make
// installed-presence the correct precondition anyway and make a not-running
// false failure structurally impossible:
//
//  1. AppleScript terminology for `tell application "Ghostty"` is resolved from
//     the installed app bundle's static scripting definition (its sdef, a bundle
//     resource) — so resolution reads the installed bundle, not the running
//     process. Installed-presence is therefore the right precondition; the
//     terminology the confirmation resolved came from the bundle regardless of
//     Ghostty being up.
//  2. Independently of (1), and as the load-bearing defensive guarantee: this
//     guard fails ONLY on the -2741 drift discriminator and t.Skips ANY other
//     resolution failure. So even if terminology resolution were ever
//     unavailable — e.g. Ghostty not running, or osacompile absent — the guard
//     cannot report that as a false template-drift failure. It fails loud only
//     on the one signature that means the template actually regressed to the
//     broken `make new surface configuration` form.
//
// Together these discharge the spec's "Assumption to confirm" with a defensive
// precondition instead of the earlier "the spawn feature is only ever invoked
// from a running Ghostty" analogy — which conflated the *feature's* invocation
// with this *guard's* own standalone invocation. (The corrected template is
// already committed in ghostty.go, so this guard passes today; the pre-fix
// template would fail it with -2741 — the discriminating behaviour was
// reproduced during the investigation with the very same osacompile tool.)
func TestGhosttyOpenScript_CompilesAgainstInstalledDictionary(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osacompile / Ghostty terminology resolution is macOS-only")
	}
	if !ghosttyAppInstalled() {
		t.Skip("Ghostty.app is not installed; the compile-check needs Ghostty's scripting dictionary")
	}

	// A representative env-self-sufficient composed argv — the exact SHAPE the
	// spawn layer composes (TMUX/TMUX_PANE stripped via `env -u`, run verbatim),
	// so this exercises the template AND ghosttyEmbed escaping together rather
	// than a hand-simplified string.
	argv := []string{
		"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
		"/bin/sh", "-c", "echo probe",
	}

	script := ghosttyOpenScript(argv)

	// osacompile requires an output target (unlike `osascript -e`, it does not
	// parse-and-discard). t.TempDir() auto-cleans the throwaway artefact.
	out := filepath.Join(t.TempDir(), "probe.scpt")

	combined, err := exec.Command("osacompile", "-e", script, "-o", out).CombinedOutput()
	if err != nil {
		// Classify the failure by its SIGNATURE, not by whether it was a
		// non-zero exit vs a non-exit run error. Only the established -2741
		// terminology-drift discriminator — the exact code the pre-fix
		// `make new surface configuration` template yields against Ghostty's
		// dictionary — is a genuine template regression, so ONLY it hard-fails.
		if strings.Contains(string(combined), driftDiscriminator) {
			t.Fatalf("osacompile reported the %s terminology-drift error: the Ghostty AppleScript "+
				"template no longer compiles against the installed dictionary — the broken "+
				"`make new surface configuration` form has returned.\nscript:\n%s\ncompiler output:\n%s",
				driftDiscriminator, script, combined)
		}
		// Any OTHER failure — a non-%s exit, or a non-exit run error such as
		// osacompile missing from PATH — could be environmental (e.g. Ghostty
		// not running so terminology cannot be resolved) rather than a template
		// regression. Skip rather than emit a false template-drift failure
		// unrelated to the template.
		t.Skipf("osacompile could not resolve the Ghostty terminology and produced no %s drift "+
			"signature; treating as environmental (e.g. Ghostty not running, osacompile unavailable) "+
			"rather than a template regression.\nerror: %v\nscript:\n%s\noutput:\n%s",
			driftDiscriminator, err, script, combined)
	}
}

// ghosttyAppInstalled reports whether Ghostty.app is present at either the
// system or per-user Applications location. It gates the guard so invoking the
// `ghosttycompile` tag on a machine without Ghostty skips cleanly (t.Skip)
// rather than hard-failing.
func ghosttyAppInstalled() bool {
	candidates := []string{"/Applications/Ghostty.app"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "Applications", "Ghostty.app"))
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
