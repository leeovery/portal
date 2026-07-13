package spawn

import (
	"fmt"
	"strings"
)

// ghosttyScriptTemplate is the validated (Ghostty 1.3.1) AppleScript that opens
// a new Ghostty window running an embedded command. It makes a `surface
// configuration` record carrying the composed argv as its `command` property
// plus `wait after command:true` — the latter governs whether the window
// persists after its command exits (the normal-detach window lifecycle for a
// spawned session) — then opens a `new window` using that configuration.
//
// The single %s is the AppleScript-escaped, space-joined composed argv from
// ghosttyEmbed; it is a format argument (never a verb source), so a `%` in the
// payload is inert.
const ghosttyScriptTemplate = `tell application "Ghostty"
	set surfaceConfig to make new surface configuration with properties {command:"%s", wait after command:true}
	make new window with properties {configuration:surfaceConfig}
end tell`

// ghosttyEmbed renders the composed argv into the single string Ghostty's
// `command` property expects: it joins the argv elements with single spaces,
// then AppleScript-string-escapes the result so it embeds safely inside the
// double-quoted AppleScript literal.
//
// Escape ORDER is load-bearing: backslash (`\` -> `\\`) MUST run before quote
// (`"` -> `\"`). Escaping the quote first would then double the escaping
// backslash the quote-escape introduced, corrupting the literal.
func ghosttyEmbed(command []string) string {
	embedded := strings.Join(command, " ")
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
