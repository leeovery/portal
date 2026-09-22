package spawn

import (
	"fmt"
	"os"
	"strings"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/shellquote"
)

// err reports a non-exit execution failure (e.g. the binary missing on PATH),
// distinct from a non-zero exit, which is reported via exitCode alone.
type recipeRunner interface {
	Run(argv []string) (out string, exitCode int, err error)
}

type execRecipeRunner struct{}

var _ recipeRunner = execRecipeRunner{}

func (execRecipeRunner) Run(argv []string) (string, int, error) {
	return runArgvCombined(argv)
}

func substituteCommand(template []string, commandStr string) []string {
	final := make([]string, len(template))
	for i, el := range template {
		final[i] = strings.ReplaceAll(el, "{command}", commandStr)
	}
	return final
}

// No permission-required branch: a config recipe is a generic argv carrying no
// AppleEvent codes, so even permission-shaped output folds to spawn-failed.
func mapRecipeResult(out string, exitCode int, err error) Result {
	if err == nil && exitCode == 0 {
		return Success(strings.TrimSpace(out))
	}
	return SpawnFailed(recipeFailureDetail(out, exitCode, err))
}

func recipeFailureDetail(out string, exitCode int, err error) string {
	return execFailureDetail(out, exitCode, err, "recipe exit %d")
}

type argvRecipeAdapter struct {
	template []string
	runner   recipeRunner
}

func (a *argvRecipeAdapter) OpenWindow(command []string) Result {
	final := substituteCommand(a.template, shellquote.Join(command))
	out, code, err := a.runner.Run(final)
	return mapRecipeResult(out, code, err)
}

var _ Adapter = (*argvRecipeAdapter)(nil)

// Validated at resolve time: Portal execs the file directly (its own shebang and
// exec bit), so a missing or non-executable script falls through to native. The
// check is a mode-bit test, not an access probe, so it is root-safe.
func newScriptRecipeAdapter(key, rawPath string, runner recipeRunner) (Adapter, bool) {
	p := resolver.ExpandTilde(rawPath)
	info, err := os.Stat(p)
	if err != nil {
		spawnLogger.Warn("terminals.json entry rejected", "detail", fmt.Sprintf("%q: script %q not found: %v", key, p, err))
		return nil, false
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		spawnLogger.Warn("terminals.json entry rejected", "detail", fmt.Sprintf("%q: script %q is not executable", key, p))
		return nil, false
	}
	return &scriptRecipeAdapter{scriptPath: p, runner: runner}, true
}

// The script path is argv[0] and the composed command is delivered structurally
// as the single positional arg $1 — never an embedded {command} token.
type scriptRecipeAdapter struct {
	scriptPath string
	runner     recipeRunner
}

func (a *scriptRecipeAdapter) OpenWindow(command []string) Result {
	final := []string{a.scriptPath, shellquote.Join(command)}
	out, code, err := a.runner.Run(final)
	return mapRecipeResult(out, code, err)
}

var _ Adapter = (*scriptRecipeAdapter)(nil)
