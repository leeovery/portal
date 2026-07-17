//go:build ghosttycompile

package spawn

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

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
// Confirmed on this Mac (GOOS=darwin, /Applications/Ghostty.app installed, and
// Ghostty RUNNING — it is the host terminal for the spawn feature, so it is
// inherently running whenever the feature is used):
//   - osacompile resolved the `tell application "Ghostty"` terminology against
//     Ghostty's dictionary and returned exit 0 for the corrected
//     `new window with configuration {…}` template, opening no window.
//   - The pre-fix `make new surface configuration with properties {…}` template
//     failed the same osacompile invocation with the observed `-2741`
//     terminology error, opening no window.
//
// The observed behaviour needed no stricter precondition than installed-presence:
// because the spawn feature is only ever invoked from a running Ghostty, gating
// on installed Ghostty (t.Skip otherwise) is sufficient and the guard cannot
// produce a false failure unrelated to the template. (The corrected template is
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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// A non-zero exit (e.g. the -2741 terminology error the pre-fix
			// template produces) means the emitted AppleScript no longer
			// compiles against Ghostty's dictionary — the template has drifted.
			t.Fatalf("osacompile exited %d (want 0): the Ghostty AppleScript template no longer "+
				"compiles against the installed dictionary (a -2741 error indicates the broken "+
				"`make new surface configuration` form).\nscript:\n%s\ncompiler output:\n%s",
				exitErr.ExitCode(), script, combined)
		}
		// A non-exit failure (e.g. osacompile missing from PATH) — surface it
		// rather than masking a real compile regression behind it.
		t.Fatalf("osacompile failed to run: %v\nscript:\n%s\noutput:\n%s", err, script, combined)
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
