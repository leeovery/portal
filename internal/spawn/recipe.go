package spawn

import (
	"errors"
	"fmt"
	"strings"
)

// RecipeKind's zero value is an explicit invalid sentinel, never a valid form.
type RecipeKind int

const (
	// RecipeArgv substitutes the composed command into the {command} placeholder
	// of at least one argv element.
	RecipeArgv RecipeKind = iota + 1
	// RecipeScript execs the user's script file with the composed command
	// delivered structurally as $1, never an embedded {command}.
	RecipeScript
)

// The {command}-presence rule is argv-only: a script recipe always receives the
// command as $1, so it cannot structurally lack it.
func validateRecipe(r Recipe) (RecipeKind, error) {
	hasArgv := len(r.Argv) > 0
	hasScript := strings.TrimSpace(r.Script) != ""

	switch {
	case hasArgv && hasScript:
		return 0, errors.New("recipe declares both argv and script (exactly one required)")
	case !hasArgv && !hasScript:
		return 0, errors.New("recipe declares neither argv nor script (exactly one required)")
	case hasArgv:
		if !argvHasCommandPlaceholder(r.Argv) {
			return 0, errors.New("argv recipe omits the {command} placeholder")
		}
		return RecipeArgv, nil
	default:
		return RecipeScript, nil
	}
}

func argvHasCommandPlaceholder(argv []string) bool {
	for _, el := range argv {
		if strings.Contains(el, "{command}") {
			return true
		}
	}
	return false
}

// An entry with no `open` capability is forward-compat rather than a typo, so it
// is rejected silently; only a configured-but-invalid recipe warns.
func validRecipeForEntry(key string, e TerminalEntry) (Recipe, RecipeKind, bool) {
	if e.Commands.Open == nil {
		return Recipe{}, 0, false
	}
	kind, err := validateRecipe(*e.Commands.Open)
	if err != nil {
		spawnLogger.Warn("terminals.json entry rejected", "detail", fmt.Sprintf("%q: %v", key, err))
		return Recipe{}, 0, false
	}
	return *e.Commands.Open, kind, true
}
