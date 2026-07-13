package spawn

import (
	"fmt"
	"os"
	"strings"

	"github.com/leeovery/portal/internal/resolver"
)

// recipeRunner is the 1-method DI seam over a real recipe exec, so a config
// recipe adapter's exec boundary and outcome mapping are unit-testable with a
// fabricated outcome and no real process / no real window. out is the combined
// stdout+stderr, exitCode is the process exit status (0 on a clean run), and
// err is a non-exit execution error (e.g. the recipe binary missing on PATH) —
// distinct from a non-zero exit, which is reported via exitCode alone.
//
// It is deliberately a SEPARATE seam from Phase 2's osascriptRunner: a config
// recipe is a generic argv, not osascript-specific, so it carries a generic
// name even though the exec shape mirrors the native driver's.
type recipeRunner interface {
	Run(argv []string) (out string, exitCode int, err error)
}

// execRecipeRunner is the production recipeRunner backed by a real exec of the
// recipe's final argv. The real-exec boundary is exercised only by the
// integration-tagged test (the fake runner covers the mapping on the unit lane).
type execRecipeRunner struct{}

var _ recipeRunner = execRecipeRunner{}

// Run execs the recipe argv through the shared exec boundary (runArgvCombined),
// which maps a clean run to (stdout, 0, nil), a non-zero exit to (combined
// output, code, nil), and a non-exit failure to a surfaced err. The recipeRunner
// seam stays separate from osascriptRunner — only the identical plumbing behind
// them is shared.
func (execRecipeRunner) Run(argv []string) (string, int, error) {
	return runArgvCombined(argv)
}

// substituteCommand renders the recipe's final argv by dropping commandStr into
// the {command} placeholder of every template element that carries the token.
// It returns a NEW slice (the template is never mutated); each element is
// strings.ReplaceAll(el, "{command}", commandStr), so the composed command lands
// as ONE literal string wherever {command} appears — the element COUNT is fixed
// (never shell-split) and elements without the token are byte-for-byte unchanged.
func substituteCommand(template []string, commandStr string) []string {
	final := make([]string, len(template))
	for i, el := range template {
		final[i] = strings.ReplaceAll(el, "{command}", commandStr)
	}
	return final
}

// mapRecipeResult is the pure outcome mapping from a raw recipe exec outcome to
// the generic typed Result. A clean run (no execution error and a zero exit) is
// Success carrying the trimmed opaque output; every other outcome is
// SpawnFailed, carrying the opaque combined output / error text in Detail.
//
// There is deliberately NO permission-required branch: a config recipe is a
// generic argv Portal cannot read AppleEvent codes from, so permission-required
// is structurally unreachable here — it stays native-adapter-only. Even output
// that resembles a permission signal (e.g. an embedded -1743/-1712) folds to
// spawn-failed.
func mapRecipeResult(out string, exitCode int, err error) Result {
	if err == nil && exitCode == 0 {
		return Success(strings.TrimSpace(out))
	}
	return SpawnFailed(recipeFailureDetail(out, exitCode, err))
}

// recipeFailureDetail is the opaque Detail for a non-clean recipe exit: it
// delegates to the shared execFailureDetail formatter, supplying only the
// recipe-specific never-empty fallback label.
func recipeFailureDetail(out string, exitCode int, err error) string {
	return execFailureDetail(out, exitCode, err, "recipe exit %d")
}

// argvRecipeAdapter is the config-escape-hatch Adapter for a validated argv
// recipe: it substitutes the composed attach command into the recipe's argv
// template and runs the result through the recipeRunner seam. The constructor
// wiring (matchConfig winner + RecipeArgv → &argvRecipeAdapter{recipe.Argv,
// r.runner}) lives in the resolver.
type argvRecipeAdapter struct {
	template []string
	runner   recipeRunner
}

// OpenWindow renders the composed attach argv to the {command} string
// (renderCommandString), substitutes it into the argv template, runs the final
// argv through the runner seam, and maps the outcome to a generic typed Result.
func (a *argvRecipeAdapter) OpenWindow(command []string) Result {
	final := substituteCommand(a.template, renderCommandString(command))
	out, code, err := a.runner.Run(final)
	return mapRecipeResult(out, code, err)
}

// Compile-time assertion that *argvRecipeAdapter satisfies the Adapter contract.
var _ Adapter = (*argvRecipeAdapter)(nil)

// newScriptRecipeAdapter builds a script-recipe Adapter for a matched script
// entry, applying the resolution-time validity gate a script recipe needs (an
// argv recipe's validation is purely structural — Task 4.2 — and needs no
// filesystem access; a script recipe additionally requires the file to exist and
// be executable, which can only be checked here at resolve time).
//
// It expands a leading ~ via resolver.ExpandTilde (the single source of truth for
// tilde expansion), then stats the resolved path. A missing OR non-executable
// script is an invalid entry: it emits exactly one spawn-component WARN naming
// the entry key (the key + reason ride in the opaque `detail` attr — the closed
// spawn attr set has no dedicated entry-key attr) and returns ok=false, so the
// resolver falls through to native (Task 4.6). Per spec the escape-hatch script
// carries its OWN exec bit + shebang and Portal execs it DIRECTLY (never via
// `sh <path>`), so a file with no exec bit could never run and is rejected here;
// the check is a Perm() mode-bit test, not an access probe, so it is root-safe.
func newScriptRecipeAdapter(key, rawPath string, runner recipeRunner) (Adapter, bool) {
	p := resolver.ExpandTilde(rawPath)
	info, err := os.Stat(p)
	if err != nil {
		detectLogger.Warn("terminals.json entry rejected", "detail", fmt.Sprintf("%q: script %q not found: %v", key, p, err))
		return nil, false
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		detectLogger.Warn("terminals.json entry rejected", "detail", fmt.Sprintf("%q: script %q is not executable", key, p))
		return nil, false
	}
	return &scriptRecipeAdapter{scriptPath: p, runner: runner}, true
}

// scriptRecipeAdapter is the config-escape-hatch Adapter for a validated script
// recipe: Portal execs the user's script file DIRECTLY (the resolved path is
// argv[0], carrying its own shebang + exec bit) with the composed attach command
// delivered structurally as the single positional arg $1 — never an embedded
// {command} token. The constructor wiring (matchConfig winner + RecipeScript →
// newScriptRecipeAdapter) lives in the resolver (Task 4.6).
type scriptRecipeAdapter struct {
	scriptPath string
	runner     recipeRunner
}

// OpenWindow renders the composed attach argv to the {command} string
// (renderCommandString, the same space-join the argv recipe and native path
// use), delivers it as the single positional arg $1 after the script path, runs
// the final argv through the runner seam, and maps the outcome to a generic typed
// Result. mapRecipeResult never yields PermissionRequired — permission-required
// stays native-adapter-only.
func (a *scriptRecipeAdapter) OpenWindow(command []string) Result {
	final := []string{a.scriptPath, renderCommandString(command)}
	out, code, err := a.runner.Run(final)
	return mapRecipeResult(out, code, err)
}

// Compile-time assertion that *scriptRecipeAdapter satisfies the Adapter contract.
var _ Adapter = (*scriptRecipeAdapter)(nil)
