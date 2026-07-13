package spawn

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// fakeRecipeRunner is a test double for recipeRunner: it records the final argv
// it is handed and returns a fabricated (out, exitCode, err) outcome, so the
// argvRecipeAdapter exec boundary and mapRecipeResult are unit-testable without
// running any real program or opening a real window.
type fakeRecipeRunner struct {
	gotArgv  []string
	out      string
	exitCode int
	err      error
}

func (f *fakeRecipeRunner) Run(argv []string) (string, int, error) {
	f.gotArgv = append([]string(nil), argv...)
	return f.out, f.exitCode, f.err
}

// spacedCommand is a composed attach argv whose space-join yields a
// multi-space {command} string, so a substitution that shell-split would
// balloon the element count and mangle the command.
func spacedCommand() []string {
	return []string{
		"/usr/bin/env", "PATH=/opt/homebrew/bin:/usr/bin",
		"/abs/portal", "attach", "proj-abc123",
	}
}

func TestSubstituteCommand(t *testing.T) {
	t.Run("it substitutes {command} as one literal element and leaves other elements verbatim", func(t *testing.T) {
		template := []string{
			"osascript",
			"-e",
			`tell app "Warp" to create window with command "{command}"`,
		}
		commandStr := renderCommandString(spacedCommand())

		final := substituteCommand(template, commandStr)

		if len(final) != len(template) {
			t.Fatalf("len(final) = %d, want %d (element count must stay fixed — never shell-split)", len(final), len(template))
		}
		if final[0] != "osascript" {
			t.Errorf("final[0] = %q, want the verbatim element %q", final[0], "osascript")
		}
		if final[1] != "-e" {
			t.Errorf("final[1] = %q, want the verbatim element %q", final[1], "-e")
		}
		want := `tell app "Warp" to create window with command "` + commandStr + `"`
		if final[2] != want {
			t.Errorf("final[2] = %q, want %q ({command} replaced by the whole command string in place)", final[2], want)
		}
	})

	t.Run("it substitutes a standalone {command} element as the whole command string", func(t *testing.T) {
		template := []string{"kitty", "@", "launch", "{command}"}
		commandStr := renderCommandString(spacedCommand())

		final := substituteCommand(template, commandStr)

		if len(final) != len(template) {
			t.Fatalf("len(final) = %d, want %d (a multi-space command must stay ONE element)", len(final), len(template))
		}
		for i, want := range []string{"kitty", "@", "launch"} {
			if final[i] != want {
				t.Errorf("final[%d] = %q, want the verbatim element %q", i, final[i], want)
			}
		}
		if final[3] != commandStr {
			t.Errorf("final[3] = %q, want exactly the whole command string %q as one element", final[3], commandStr)
		}
	})

	t.Run("it returns a new slice and does not mutate the template", func(t *testing.T) {
		template := []string{"kitty", "@", "launch", "{command}"}
		commandStr := renderCommandString(spacedCommand())

		_ = substituteCommand(template, commandStr)

		if template[3] != "{command}" {
			t.Errorf("template[3] = %q, want it left unmutated as %q", template[3], "{command}")
		}
	})
}

func TestArgvRecipeAdapterOpenWindow(t *testing.T) {
	t.Run("it runs the substituted final argv through the runner", func(t *testing.T) {
		template := []string{"kitty", "@", "launch", "{command}"}
		command := spacedCommand()
		fake := &fakeRecipeRunner{}
		adapter := &argvRecipeAdapter{template: template, runner: fake}

		adapter.OpenWindow(command)

		want := substituteCommand(template, renderCommandString(command))
		if !slices.Equal(fake.gotArgv, want) {
			t.Errorf("runner received argv %#v, want the substituted final argv %#v", fake.gotArgv, want)
		}
	})

	t.Run("it maps a clean exit to success and a non-zero exit to spawn-failed with opaque detail", func(t *testing.T) {
		template := []string{"kitty", "@", "launch", "{command}"}

		clean := &fakeRecipeRunner{out: "", exitCode: 0, err: nil}
		cleanResult := (&argvRecipeAdapter{template: template, runner: clean}).OpenWindow(spacedCommand())
		if cleanResult.Outcome != OutcomeSuccess {
			t.Errorf("clean exit Outcome = %v, want OutcomeSuccess", cleanResult.Outcome)
		}

		const body = "kitty: could not open display"
		failed := &fakeRecipeRunner{out: body, exitCode: 1, err: nil}
		failedResult := (&argvRecipeAdapter{template: template, runner: failed}).OpenWindow(spacedCommand())
		if failedResult.Outcome != OutcomeSpawnFailed {
			t.Errorf("non-zero exit Outcome = %v, want OutcomeSpawnFailed", failedResult.Outcome)
		}
		if !strings.Contains(failedResult.Detail, body) {
			t.Errorf("Detail = %q, want it to carry the opaque output %q", failedResult.Detail, body)
		}
	})

	t.Run("it maps an execution error to spawn-failed", func(t *testing.T) {
		template := []string{"kitty", "@", "launch", "{command}"}
		execErr := errors.New(`exec: "kitty": executable file not found in $PATH`)
		fake := &fakeRecipeRunner{out: "", exitCode: 0, err: execErr}
		adapter := &argvRecipeAdapter{template: template, runner: fake}

		result := adapter.OpenWindow(spacedCommand())

		if result.Outcome != OutcomeSpawnFailed {
			t.Errorf("Outcome = %v, want OutcomeSpawnFailed (never a panic, never Success)", result.Outcome)
		}
		if strings.TrimSpace(result.Detail) == "" {
			t.Errorf("Detail = %q, want it to carry the execution-error text", result.Detail)
		}
	})
}

// TestMapRecipeResult pins the pure mapping directly, notably that a config
// recipe can NEVER surface OutcomePermissionRequired — there is no AppleEvent
// code for Portal to read from a generic argv, so that path is structurally
// unreachable here (permission-required stays native-adapter-only).
func TestMapRecipeResult(t *testing.T) {
	t.Run("it maps a clean exit to success with a trimmed opaque detail", func(t *testing.T) {
		result := mapRecipeResult("  ok  ", 0, nil)

		if result.Outcome != OutcomeSuccess {
			t.Fatalf("Outcome = %v, want OutcomeSuccess", result.Outcome)
		}
		if result.Detail != "ok" {
			t.Errorf("Detail = %q, want the trimmed output %q", result.Detail, "ok")
		}
	})

	t.Run("it maps a non-zero exit to spawn-failed carrying the opaque output", func(t *testing.T) {
		const body = "some failure text"
		result := mapRecipeResult(body, 3, nil)

		if result.Outcome != OutcomeSpawnFailed {
			t.Fatalf("Outcome = %v, want OutcomeSpawnFailed", result.Outcome)
		}
		if !strings.Contains(result.Detail, body) {
			t.Errorf("Detail = %q, want it to carry the opaque output %q", result.Detail, body)
		}
	})

	t.Run("it maps an execution error to spawn-failed", func(t *testing.T) {
		result := mapRecipeResult("", 0, errors.New("boom"))

		if result.Outcome != OutcomeSpawnFailed {
			t.Errorf("Outcome = %v, want OutcomeSpawnFailed for an execution error", result.Outcome)
		}
	})

	t.Run("it never returns permission-required from a config recipe", func(t *testing.T) {
		// A config recipe is a generic argv Portal cannot read AppleEvent codes
		// from — even output that LOOKS like a permission signal must fold to
		// spawn-failed, never permission-required.
		cases := []struct {
			name     string
			out      string
			exitCode int
			err      error
		}{
			{name: "clean exit", out: "", exitCode: 0, err: nil},
			{name: "non-zero exit", out: "generic failure", exitCode: 1, err: nil},
			{name: "execution error", out: "", exitCode: 0, err: errors.New("not found")},
			{name: "output contains -1743", out: "AppleScript error (-1743)", exitCode: 1, err: nil},
			{name: "output contains -1712", out: "AppleEvent timed out (-1712)", exitCode: 1, err: nil},
			{name: "-1743 on a clean-looking exit code", out: "-1743", exitCode: 0, err: errors.New("weird")},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				result := mapRecipeResult(tc.out, tc.exitCode, tc.err)
				if result.Outcome == OutcomePermissionRequired {
					t.Errorf("Outcome = OutcomePermissionRequired, want it NEVER produced by a config recipe (got out=%q code=%d err=%v)", tc.out, tc.exitCode, tc.err)
				}
				if strings.TrimSpace(result.Guidance) != "" {
					t.Errorf("Guidance = %q, want empty — config recipes have no permission guidance", result.Guidance)
				}
			})
		}
	})
}
