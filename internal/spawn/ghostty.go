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

// ghosttyAdapter is the native window-spawning driver for the Ghostty terminal.
//
// Placeholder — real osascript driver lands in Tasks 2.4/2.5.
type ghosttyAdapter struct{}

// newGhosttyAdapter builds the native Ghostty adapter.
//
// Placeholder — real osascript driver lands in Tasks 2.4/2.5.
func newGhosttyAdapter() *ghosttyAdapter {
	return &ghosttyAdapter{}
}

// OpenWindow satisfies the Adapter interface.
//
// Placeholder — real osascript driver lands in Tasks 2.4/2.5. It performs no
// osascript call and builds no argv; it exists only so the resolver can
// construct and type-assert the native Ghostty adapter.
func (g *ghosttyAdapter) OpenWindow(command []string) Result {
	return Unsupported("ghostty driver not yet implemented (Task 2.5)")
}

// Compile-time assertion that *ghosttyAdapter satisfies the Adapter contract.
var _ Adapter = (*ghosttyAdapter)(nil)
