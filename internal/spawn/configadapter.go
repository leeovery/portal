package spawn

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/leeovery/portal/internal/log"
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

// Run execs the recipe argv through log.CombinedOutputWithContext (the
// stderr-preserving boundary helper) and derives exitCode from an
// *exec.ExitError. A clean run returns (stdout, 0, nil); a non-zero (or signal)
// exit returns the combined output plus the exit code with a nil err (it ran
// but failed); a non-exit failure (binary missing on PATH — no exit status)
// surfaces as err so the mapping folds it to spawn-failed.
func (execRecipeRunner) Run(argv []string) (string, int, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	out, err := log.CombinedOutputWithContext(cmd)
	if err == nil {
		return string(out), 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return combineOutput(out, err), exitErr.ExitCode(), nil
	}
	return string(out), 0, err
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

// recipeFailureDetail is the opaque Detail for a non-clean recipe exit: the
// combined output and/or execution-error text, falling back to the bare exit
// code so Detail is never empty.
func recipeFailureDetail(out string, exitCode int, err error) string {
	detail := strings.TrimSpace(out)
	if err != nil {
		if detail == "" {
			return err.Error()
		}
		return detail + ": " + err.Error()
	}
	if detail == "" {
		return fmt.Sprintf("recipe exit %d", exitCode)
	}
	return detail
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
