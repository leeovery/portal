package spawn

import (
	"fmt"
	"strings"
)

// ghosttyScriptTemplate is the sdef-correct AppleScript that opens a new Ghostty
// window running an embedded command. It passes a `surface configuration` record
// literal directly to `new window`'s `with configuration` parameter — the only
// form Ghostty's scripting dictionary defines (there is no `make` command and no
// `with properties` terminology). The record carries a SINGLE field: `command`
// (text).
//
// `wait after command` is deliberately NOT set: the window is kept alive not by
// that flag but by the shell-fallback wrapper ghosttyEmbed builds — after the
// session's exec chain finishes, `exec "$SHELL" -il` replaces the wrapper with
// the user's interactive login shell, so the window stays visible AND usable.
// The dropped flag is what produced the "Process exited. Press any key to close
// the terminal." dead-end. Omitting this optional field does not alter the
// terminology the template resolves against Ghostty's dictionary.
//
// The single %s is the AppleScript-escaped, per-element-shell-quoted composed
// argv from ghosttyEmbed; it is a fmt.Sprintf format ARGUMENT (never a verb
// source), so a `%` in the payload is inert.
const ghosttyScriptTemplate = `tell application "Ghostty"
	new window with configuration {command:"%s"}
end tell`

// wrapWithShellFallback wraps the composed open argv in an explicit
// `bash -lc '<composed open argv>; exec "$SHELL" -il'` so a burst-spawned
// (N−1 external) Ghostty window lands at the user's interactive login shell
// after its session's exec chain finishes, instead of dead-ending on Ghostty's
// "Process exited. Press any key to close the terminal." keypress.
//
// It returns the wrapper as a REAL 3-element argv ["bash", "-lc", PAYLOAD] and
// leaves PAYLOAD un-quoted here on purpose: the caller renders the whole argv
// through the shared shell-quote helper (renderCommandString), which owns the
// nesting — re-single-quoting PAYLOAD so every single quote the inner
// per-element quoting already emitted is re-escaped via the POSIX
// close-escape-reopen idiom ('\''). Hand-rolled concatenation of the schematic
// form would let the first inner single quote terminate the outer quote and
// corrupt the command — the exact failure class this fix removes.
//
// PAYLOAD is the rendered composed argv (renderCommandString(command), which
// keeps the /usr/bin/env … PATH=<…> -u TMUX -u TMUX_PANE prefix intact so tmux
// still resolves) followed by the literal `; exec "$SHELL" -il`. Ghostty runs a
// window command by prepending `exec -l`, replacing the outer bash with OUR
// inner bash, which runs the session command as a child then execs the user's
// login+interactive shell. The explicit wrapper form is required for that reason
// (an implicit `<argv>; exec "$SHELL"` append would be unreachable under
// `exec -l`). $SHELL is populated by /usr/bin/login, so no $SHELL fallback is
// specified; a degenerate exec failure closes the window cleanly (an accepted
// fallback, no dead-end). The wrap is argv-agnostic — it applies identically to
// mint (`open --path <dir>`) and attach (`open --session <name>`) surfaces.
func wrapWithShellFallback(command []string) []string {
	payload := renderCommandString(command) + `; exec "$SHELL" -il`
	return []string{"bash", "-lc", payload}
}

// ghosttyEmbed renders the composed argv into the single string Ghostty's
// `command` property expects. Ghostty runs that string via `bash -c`, which
// word-splits it, so it FIRST wraps the composed argv in the shell-fallback
// layer (wrapWithShellFallback → ["bash", "-lc", "<rendered argv>; exec
// \"$SHELL\" -il"]) so a spawned window lands at an interactive login shell
// after its session exits rather than dead-ending. It then renders THAT wrapped
// argv through renderCommandString — each element POSIX-single-quoted, preserving
// element boundaries so a session name or path containing a space is reproduced
// intact rather than shredded — which also re-escapes the inner per-element
// single quotes for the outer `-lc` payload's single-quote layer via the shared
// helper's '\'' close-escape-reopen idiom (correct nesting, not naive
// concatenation). Finally it AppleScript-string-escapes the result so it embeds
// safely inside the double-quoted AppleScript literal.
//
// Escape ORDER is load-bearing: backslash (`\` -> `\\`) MUST run before quote
// (`"` -> `\"`). Escaping the quote first would then double the escaping
// backslash the quote-escape introduced, corrupting the literal. The order holds
// even over the EXTRA backslashes the wrapper's second single-quote layer
// introduces for each inner embedded single quote.
func ghosttyEmbed(command []string) string {
	embedded := renderCommandString(wrapWithShellFallback(command))
	embedded = strings.ReplaceAll(embedded, `\`, `\\`)
	embedded = strings.ReplaceAll(embedded, `"`, `\"`)
	return embedded
}

// ghosttyOpenScript builds the full AppleScript that opens a Ghostty window
// running command. It is pure: identical input yields identical output and it
// performs no I/O or process exec.
func ghosttyOpenScript(command []string) string {
	return fmt.Sprintf(ghosttyScriptTemplate, ghosttyEmbed(command))
}

// ghosttyOpenArgv wraps the built script into the `osascript -e <script>` argv
// the exec boundary (Task 2.5) runs. It performs no execution itself.
func ghosttyOpenArgv(command []string) []string {
	return []string{"osascript", "-e", ghosttyOpenScript(command)}
}

// osascriptRunner is the 1-method DI seam over the real osascript exec, so the
// Ghostty driver's exec boundary and outcome mapping are unit-testable with a
// fabricated outcome and no real osascript / no real window. out is the
// combined stdout+stderr, exitCode is the process exit status (0 on a clean
// run), and err is a non-exit execution error (e.g. osascript not found on
// PATH) — distinct from a non-zero exit, which is reported via exitCode alone.
type osascriptRunner interface {
	Run(argv []string) (out string, exitCode int, err error)
}

// execOsascriptRunner is the production osascriptRunner backed by real
// osascript. The real osascript boundary is manual/live-Mac only — no automated
// test executes it (the fake runner covers the mapping).
type execOsascriptRunner struct{}

var _ osascriptRunner = execOsascriptRunner{}

// Run execs the osascript argv through the shared exec boundary
// (runArgvCombined), which maps a clean run to (stdout, 0, nil), a non-zero
// exit to (combined output, code, nil), and a non-exit failure to a surfaced
// err. The osascriptRunner seam stays separate from recipeRunner — only the
// identical plumbing behind them is shared.
func (execOsascriptRunner) Run(argv []string) (string, int, error) {
	return runArgvCombined(argv)
}

// ghosttyAdapter is the native window-spawning driver for the Ghostty terminal.
// It owns the thin osascript exec boundary; every Ghostty/AppleScript/osascript
// specific concern stays quarantined here behind the generic Result taxonomy.
type ghosttyAdapter struct {
	runner osascriptRunner
}

// newGhosttyAdapter builds the native Ghostty adapter wired with the real
// osascript runner. It touches no osascript itself — only OpenWindow does — so
// the resolver can construct it freely.
func newGhosttyAdapter() *ghosttyAdapter {
	return &ghosttyAdapter{runner: execOsascriptRunner{}}
}

// OpenWindow builds the osascript argv (Task 2.4), runs it through the runner
// seam, and maps the outcome to a generic typed Result. It never inspects the
// OS-specific output to classify — that is mapGhosttyResult's job.
func (g *ghosttyAdapter) OpenWindow(command []string) Result {
	out, code, err := g.runner.Run(ghosttyOpenArgv(command))
	return mapGhosttyResult(out, code, err)
}

// mapGhosttyResult is the pure outcome mapping from a raw osascript outcome to
// the generic typed Result. A clean run (no execution error and a zero exit)
// is Success; an AppleEvent permission signal (-1743 not-permitted/denied or
// -1712 timeout) in the output is PermissionRequired, carrying the opaque
// output as Detail plus the driver-composed Guidance; every other non-clean
// outcome is SpawnFailed, carrying the opaque combined output / error text in
// Detail.
//
// The -1743/-1712 recognition is quarantined HERE, in the driver: general code
// switches on the generic Outcome alone and never sees an AppleEvent number.
func mapGhosttyResult(out string, exitCode int, err error) Result {
	if err == nil && exitCode == 0 {
		return Success(successDetail(out))
	}
	if strings.Contains(out, "-1743") || strings.Contains(out, "-1712") {
		return PermissionRequired(out, ghosttyPermissionGuidance())
	}
	return SpawnFailed(failureDetail(out, exitCode, err))
}

// ghosttyPermissionGuidance is the driver-composed, user-readable guidance for a
// Ghostty Automation-permission wall. It names the target terminal (Ghostty) and
// the macOS Automation-settings location plus deep-link, so the orchestrator can
// surface it verbatim without ever parsing the AppleEvent code or the deep-link —
// the whole string is opaque above the driver boundary.
func ghosttyPermissionGuidance() string {
	return "Ghostty needs permission to open new windows. Grant it under " +
		"System Settings → Privacy & Security → Automation " +
		"(x-apple.systempreferences:com.apple.preference.security?Privacy_Automation), " +
		"then try again."
}

// successDetail is the opaque Detail for a clean exit: the trimmed osascript
// output when present, else a terse marker so Detail is never empty.
func successDetail(out string) string {
	if trimmed := strings.TrimSpace(out); trimmed != "" {
		return trimmed
	}
	return "ghostty osascript exit 0"
}

// failureDetail is the opaque Detail for a non-clean Ghostty exit: it delegates
// to the shared execFailureDetail formatter, supplying only the Ghostty-specific
// never-empty fallback label.
func failureDetail(out string, exitCode int, err error) string {
	return execFailureDetail(out, exitCode, err, "ghostty osascript exit %d")
}

// Compile-time assertion that *ghosttyAdapter satisfies the Adapter contract.
var _ Adapter = (*ghosttyAdapter)(nil)
