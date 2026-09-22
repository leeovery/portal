package spawn

import (
	"fmt"
	"strings"

	"github.com/leeovery/portal/internal/shellquote"
)

// `new window with configuration {record}` is the only form Ghostty's scripting
// dictionary defines — there is no `make` command and no `with properties`.
// `wait after command` is deliberately unset: the shell-fallback wrapper, not
// that flag, keeps the window alive and usable.
const ghosttyScriptTemplate = `tell application "Ghostty"
	new window with configuration {command:"%s"}
end tell`

// Ghostty prepends `exec -l` to a window command, so the explicit
// `bash -lc '<argv>; exec "$SHELL" -il'` wrapper is required — an appended
// `; exec "$SHELL"` would be unreachable, leaving the window on Ghostty's
// "Process exited" dead-end. The payload stays unquoted here: shellquote.Join
// owns the nested close-escape-reopen quoting.
func wrapWithShellFallback(command []string) []string {
	payload := shellquote.Join(command) + `; exec "$SHELL" -il`
	return []string{"bash", "-lc", payload}
}

// Escape order is load-bearing: backslash before quote. Escaping the quote first
// would double the backslash the quote-escape introduced.
func ghosttyEmbed(command []string) string {
	embedded := shellquote.Join(wrapWithShellFallback(command))
	embedded = strings.ReplaceAll(embedded, `\`, `\\`)
	embedded = strings.ReplaceAll(embedded, `"`, `\"`)
	return embedded
}

func ghosttyOpenScript(command []string) string {
	return fmt.Sprintf(ghosttyScriptTemplate, ghosttyEmbed(command))
}

func ghosttyOpenArgv(command []string) []string {
	return []string{"osascript", "-e", ghosttyOpenScript(command)}
}

// err reports a non-exit execution failure (e.g. osascript missing on PATH),
// distinct from a non-zero exit, which is reported via exitCode alone.
type osascriptRunner interface {
	Run(argv []string) (out string, exitCode int, err error)
}

type execOsascriptRunner struct{}

var _ osascriptRunner = execOsascriptRunner{}

func (execOsascriptRunner) Run(argv []string) (string, int, error) {
	return runArgvCombined(argv)
}

type ghosttyAdapter struct {
	runner osascriptRunner
}

func newGhosttyAdapter() *ghosttyAdapter {
	return &ghosttyAdapter{runner: execOsascriptRunner{}}
}

func (g *ghosttyAdapter) OpenWindow(command []string) Result {
	out, code, err := g.runner.Run(ghosttyOpenArgv(command))
	return mapGhosttyResult(out, code, err)
}

// -1743 (not permitted) and -1712 (timeout) are the AppleEvent permission
// signals. Recognising them is quarantined here in the driver: general code
// switches on Outcome and never sees an AppleEvent number.
func mapGhosttyResult(out string, exitCode int, err error) Result {
	if err == nil && exitCode == 0 {
		return Success(successDetail(out))
	}
	if strings.Contains(out, "-1743") || strings.Contains(out, "-1712") {
		return PermissionRequired(out, ghosttyPermissionGuidance())
	}
	return SpawnFailed(failureDetail(out, exitCode, err))
}

func ghosttyPermissionGuidance() string {
	return "Ghostty needs permission to open new windows. Grant it under " +
		"System Settings → Privacy & Security → Automation " +
		"(x-apple.systempreferences:com.apple.preference.security?Privacy_Automation), " +
		"then try again."
}

func successDetail(out string) string {
	if trimmed := strings.TrimSpace(out); trimmed != "" {
		return trimmed
	}
	return "ghostty osascript exit 0"
}

func failureDetail(out string, exitCode int, err error) string {
	return execFailureDetail(out, exitCode, err, "ghostty osascript exit %d")
}

var _ Adapter = (*ghosttyAdapter)(nil)
