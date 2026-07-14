package spawn

import (
	"errors"
	"fmt"
	"strings"
)

// RecipeKind classifies a validated recipe's execution form. The zero value is
// an explicit invalid/none sentinel, so a bare RecipeKind is never mistaken for
// a valid form; a well-formed recipe is exactly one of RecipeArgv / RecipeScript.
type RecipeKind int

const (
	// RecipeArgv is an argv-template recipe: the composed attach command
	// substitutes into the {command} placeholder of at least one argv element.
	RecipeArgv RecipeKind = iota + 1
	// RecipeScript is a script recipe: Portal execs the user's script file with
	// the composed command delivered structurally as $1 (never an embedded
	// {command}).
	RecipeScript
)

// validateRecipe enforces the two structural rules a terminals.json `open`
// recipe must satisfy and reports which form it is:
//
//   - exactly one of argv / script — neither or both is a config typo, and
//   - an argv recipe must reference the {command} placeholder in at least one
//     element (a window with no {command} would never run the attach).
//
// The {command}-presence rule is argv-only: a script recipe always receives
// {command} as $1 from Portal — delivered structurally, not embedded — so it can
// never structurally lack the command. Every rejection returns the zero
// RecipeKind alongside a descriptive error.
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

// argvHasCommandPlaceholder reports whether some argv element embeds the
// {command} placeholder token.
func argvHasCommandPlaceholder(argv []string) bool {
	for _, el := range argv {
		if strings.Contains(el, "{command}") {
			return true
		}
	}
	return false
}

// validRecipeForEntry extracts an entry's `open` recipe, structurally validates
// it, and distinguishes two ok=false cases:
//
//   - no `open` capability configured (Open == nil) → forward-compat, not a
//     typo (e.g. only a future introspect/place is set): ok=false with NO WARN,
//     so the resolver simply falls through to native.
//   - a configured-but-invalid recipe → exactly one spawn-component WARN naming
//     the entry key, then ok=false.
//
// The key + reason ride in the opaque `detail` attr because the closed spawn
// attr set has no dedicated entry-key attr. A valid recipe returns
// (recipe, kind, true).
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

// renderCommandString is the single canonical rendering of the composed attach
// argv into the {command} string: a plain single-space join. It is the SAME
// space-join the native Ghostty embed (ghosttyEmbed) uses, so a config recipe
// and the native path render {command} identically. Consumed by the argv/script
// recipe adapters (Tasks 4.4/4.5).
func renderCommandString(command []string) string {
	return strings.Join(command, " ")
}
