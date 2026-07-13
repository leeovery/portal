package spawn

import (
	"strings"
	"testing"
)

func TestValidateRecipe(t *testing.T) {
	t.Run("it rejects a recipe declaring both argv and script", func(t *testing.T) {
		kind, err := validateRecipe(Recipe{Argv: []string{"kitty", "{command}"}, Script: "/opt/myterm/open.sh"})

		if err == nil {
			t.Fatal("validateRecipe accepted a recipe declaring both argv and script, want an error")
		}
		if kind != 0 {
			t.Errorf("kind = %d, want the zero RecipeKind on error", kind)
		}
	})

	t.Run("it rejects a recipe declaring neither argv nor script", func(t *testing.T) {
		kind, err := validateRecipe(Recipe{})

		if err == nil {
			t.Fatal("validateRecipe accepted a recipe declaring neither argv nor script, want an error")
		}
		if kind != 0 {
			t.Errorf("kind = %d, want the zero RecipeKind on error", kind)
		}
	})

	t.Run("it treats a whitespace-only script as absent", func(t *testing.T) {
		kind, err := validateRecipe(Recipe{Script: "   \t\n"})

		if err == nil {
			t.Fatal("validateRecipe accepted a whitespace-only script as a valid recipe, want a neither error")
		}
		if kind != 0 {
			t.Errorf("kind = %d, want the zero RecipeKind on error", kind)
		}
	})

	t.Run("it rejects an argv recipe that omits the {command} placeholder", func(t *testing.T) {
		kind, err := validateRecipe(Recipe{Argv: []string{"kitty", "--hold", "--"}})

		if err == nil {
			t.Fatal("validateRecipe accepted an argv recipe with no {command} placeholder, want an error")
		}
		if kind != 0 {
			t.Errorf("kind = %d, want the zero RecipeKind on error", kind)
		}
	})

	t.Run("it accepts a valid argv-only recipe and a valid script-only recipe", func(t *testing.T) {
		kind, err := validateRecipe(Recipe{Argv: []string{"kitty", "@", "launch", "{command}"}})
		if err != nil {
			t.Fatalf("validateRecipe rejected a valid argv-with-{command} recipe: %v", err)
		}
		if kind != RecipeArgv {
			t.Errorf("kind = %d, want RecipeArgv (%d) for a valid argv recipe", kind, RecipeArgv)
		}

		kind, err = validateRecipe(Recipe{Script: "/opt/myterm/open.sh"})
		if err != nil {
			t.Fatalf("validateRecipe rejected a valid script-only recipe: %v", err)
		}
		if kind != RecipeScript {
			t.Errorf("kind = %d, want RecipeScript (%d) for a valid script recipe", kind, RecipeScript)
		}
	})
}

func TestValidRecipeForEntry(t *testing.T) {
	const key = "com.example.MyTerm"

	t.Run("it warns once and skips an entry with a structurally invalid open recipe", func(t *testing.T) {
		sink := installSpawnCapture(t)
		entry := TerminalEntry{Commands: Capabilities{Open: &Recipe{
			Argv:   []string{"kitty", "{command}"},
			Script: "/opt/myterm/open.sh",
		}}}

		recipe, kind, ok := validRecipeForEntry(key, entry)

		if ok {
			t.Fatal("validRecipeForEntry accepted a structurally-invalid recipe, want ok=false")
		}
		if kind != 0 {
			t.Errorf("kind = %d, want the zero RecipeKind for a rejected entry", kind)
		}
		if recipe.Argv != nil || recipe.Script != "" {
			t.Errorf("recipe = %+v, want the zero Recipe for a rejected entry", recipe)
		}
		warns := warnRecords(sink)
		if len(warns) != 1 {
			t.Fatalf("emitted %d WARN records for an invalid recipe, want exactly 1: %+v", len(warns), warns)
		}
		rec := warns[0]
		if v := rec.AttrString(t, "component"); v != "spawn" {
			t.Errorf("WARN component = %q, want %q", v, "spawn")
		}
		detail := rec.AttrString(t, "detail")
		if !strings.Contains(detail, key) {
			t.Errorf("WARN detail = %q, want it to name the entry key %q", detail, key)
		}
	})

	t.Run("it skips an entry with no open capability without warning (forward-compat)", func(t *testing.T) {
		sink := installSpawnCapture(t)
		entry := TerminalEntry{Commands: Capabilities{Open: nil}}

		recipe, kind, ok := validRecipeForEntry(key, entry)

		if ok {
			t.Fatal("validRecipeForEntry accepted an entry with no open capability, want ok=false")
		}
		if kind != 0 {
			t.Errorf("kind = %d, want the zero RecipeKind for a no-open entry", kind)
		}
		if recipe.Argv != nil || recipe.Script != "" {
			t.Errorf("recipe = %+v, want the zero Recipe for a no-open entry", recipe)
		}
		if warns := warnRecords(sink); len(warns) != 0 {
			t.Errorf("emitted %d WARN records for a no-open entry, want 0 (forward-compat): %+v", len(warns), warns)
		}
	})

	t.Run("it returns the recipe and kind for a valid open recipe with no warning", func(t *testing.T) {
		sink := installSpawnCapture(t)
		argvEntry := TerminalEntry{Commands: Capabilities{Open: &Recipe{Argv: []string{"kitty", "{command}"}}}}

		recipe, kind, ok := validRecipeForEntry(key, argvEntry)

		if !ok {
			t.Fatal("validRecipeForEntry rejected a valid argv recipe, want ok=true")
		}
		if kind != RecipeArgv {
			t.Errorf("kind = %d, want RecipeArgv (%d)", kind, RecipeArgv)
		}
		wantArgv := []string{"kitty", "{command}"}
		if !equalStrings(recipe.Argv, wantArgv) {
			t.Errorf("recipe.Argv = %v, want %v", recipe.Argv, wantArgv)
		}

		scriptEntry := TerminalEntry{Commands: Capabilities{Open: &Recipe{Script: "/opt/myterm/open.sh"}}}
		recipe, kind, ok = validRecipeForEntry(key, scriptEntry)
		if !ok {
			t.Fatal("validRecipeForEntry rejected a valid script recipe, want ok=true")
		}
		if kind != RecipeScript {
			t.Errorf("kind = %d, want RecipeScript (%d)", kind, RecipeScript)
		}
		if recipe.Script != "/opt/myterm/open.sh" {
			t.Errorf("recipe.Script = %q, want %q", recipe.Script, "/opt/myterm/open.sh")
		}

		if warns := warnRecords(sink); len(warns) != 0 {
			t.Errorf("emitted %d WARN records for valid recipes, want 0: %+v", len(warns), warns)
		}
	})
}

func TestRenderCommandString(t *testing.T) {
	t.Run("renderCommandString space-joins the composed attach argv", func(t *testing.T) {
		command := []string{"/usr/bin/env", "-u", "TMUX", "PATH=/b", "/abs/portal", "attach", "proj-x", "--spawn-ack", "b1:t1"}

		got := renderCommandString(command)

		want := "/usr/bin/env -u TMUX PATH=/b /abs/portal attach proj-x --spawn-ack b1:t1"
		if got != want {
			t.Errorf("renderCommandString = %q, want %q", got, want)
		}
	})
}
