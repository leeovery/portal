package spawn

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeExecutableScript writes a trivial shebang script with the exec bit set,
// so newScriptRecipeAdapter's stat gate accepts it. The body is never run on the
// unit lane (the fake runner records the argv without exec'ing) — the mode bits
// are what the constructor checks.
func writeExecutableScript(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing executable script %q: %v", path, err)
	}
}

func TestNewScriptRecipeAdapter(t *testing.T) {
	const key = "com.example.MyTerm"

	t.Run("it expands a leading ~ in the script path", func(t *testing.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)
		scriptPath := filepath.Join(tmpHome, "s.sh")
		writeExecutableScript(t, scriptPath)

		fake := &fakeRecipeRunner{}
		adapter, ok := newScriptRecipeAdapter(key, "~/s.sh", fake)
		if !ok {
			t.Fatal("newScriptRecipeAdapter returned ok=false for an existing executable script, want ok=true")
		}

		command := spacedCommand()
		adapter.OpenWindow(command)

		want := []string{scriptPath, renderCommandString(command)}
		if !slices.Equal(fake.gotArgv, want) {
			t.Errorf("runner received argv %#v, want %#v (argv[0] the ~-expanded script path, argv[1] the composed command)", fake.gotArgv, want)
		}
	})

	t.Run("it delivers the composed command as $1 to the script", func(t *testing.T) {
		dir := t.TempDir()
		scriptPath := filepath.Join(dir, "s.sh")
		writeExecutableScript(t, scriptPath)

		fake := &fakeRecipeRunner{}
		adapter, ok := newScriptRecipeAdapter(key, scriptPath, fake)
		if !ok {
			t.Fatal("newScriptRecipeAdapter returned ok=false for an existing executable script, want ok=true")
		}

		command := spacedCommand()
		adapter.OpenWindow(command)

		if len(fake.gotArgv) != 2 {
			t.Fatalf("runner received %d argv elements, want 2 (script path + one positional command arg): %#v", len(fake.gotArgv), fake.gotArgv)
		}
		if fake.gotArgv[0] != scriptPath {
			t.Errorf("argv[0] = %q, want the resolved script path %q", fake.gotArgv[0], scriptPath)
		}
		want := renderCommandString(command)
		if fake.gotArgv[1] != want {
			t.Errorf("argv[1] = %q, want the composed command as a single positional arg %q", fake.gotArgv[1], want)
		}
	})

	t.Run("it skips a missing script with a WARN and no adapter", func(t *testing.T) {
		sink := installSpawnCapture(t)
		missing := filepath.Join(t.TempDir(), "nope.sh")

		adapter, ok := newScriptRecipeAdapter(key, missing, &fakeRecipeRunner{})

		if ok {
			t.Fatal("newScriptRecipeAdapter accepted a missing script, want ok=false")
		}
		if adapter != nil {
			t.Errorf("adapter = %#v, want nil for a missing script", adapter)
		}
		warns := warnRecords(sink)
		if len(warns) != 1 {
			t.Fatalf("emitted %d WARN records for a missing script, want exactly 1: %+v", len(warns), warns)
		}
		rec := warns[0]
		if v := rec.AttrString(t, "component"); v != "spawn" {
			t.Errorf("WARN component = %q, want %q", v, "spawn")
		}
		if detail := rec.AttrString(t, "detail"); !strings.Contains(detail, key) {
			t.Errorf("WARN detail = %q, want it to name the entry key %q", detail, key)
		}
	})

	t.Run("it skips a non-executable script with a WARN and no adapter", func(t *testing.T) {
		sink := installSpawnCapture(t)
		scriptPath := filepath.Join(t.TempDir(), "plain.sh")
		// 0o644: a real file with a shebang but NO exec bit. Portal execs the
		// escape-hatch script directly, so a file that cannot run is rejected —
		// the check is a mode-bit test (Perm()&0o111), root-safe (root does not
		// grant a 0o644 file exec bits).
		if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
			t.Fatalf("writing non-executable script: %v", err)
		}

		adapter, ok := newScriptRecipeAdapter(key, scriptPath, &fakeRecipeRunner{})

		if ok {
			t.Fatal("newScriptRecipeAdapter accepted a non-executable script, want ok=false")
		}
		if adapter != nil {
			t.Errorf("adapter = %#v, want nil for a non-executable script", adapter)
		}
		warns := warnRecords(sink)
		if len(warns) != 1 {
			t.Fatalf("emitted %d WARN records for a non-executable script, want exactly 1: %+v", len(warns), warns)
		}
		rec := warns[0]
		if v := rec.AttrString(t, "component"); v != "spawn" {
			t.Errorf("WARN component = %q, want %q", v, "spawn")
		}
		if detail := rec.AttrString(t, "detail"); !strings.Contains(detail, key) {
			t.Errorf("WARN detail = %q, want it to name the entry key %q", detail, key)
		}
	})

	t.Run("it maps a clean exit to success and a non-zero exit to spawn-failed", func(t *testing.T) {
		scriptPath := filepath.Join(t.TempDir(), "s.sh")
		writeExecutableScript(t, scriptPath)

		clean := &fakeRecipeRunner{out: "", exitCode: 0, err: nil}
		cleanAdapter, ok := newScriptRecipeAdapter(key, scriptPath, clean)
		if !ok {
			t.Fatal("newScriptRecipeAdapter returned ok=false for a valid script, want ok=true")
		}
		cleanResult := cleanAdapter.OpenWindow(spacedCommand())
		if cleanResult.Outcome != OutcomeSuccess {
			t.Errorf("clean exit Outcome = %v, want OutcomeSuccess", cleanResult.Outcome)
		}

		const body = "myterm: window failed to open"
		failed := &fakeRecipeRunner{out: body, exitCode: 1, err: nil}
		failedAdapter, ok := newScriptRecipeAdapter(key, scriptPath, failed)
		if !ok {
			t.Fatal("newScriptRecipeAdapter returned ok=false for a valid script, want ok=true")
		}
		failedResult := failedAdapter.OpenWindow(spacedCommand())
		if failedResult.Outcome != OutcomeSpawnFailed {
			t.Errorf("non-zero exit Outcome = %v, want OutcomeSpawnFailed", failedResult.Outcome)
		}
		if !strings.Contains(failedResult.Detail, body) {
			t.Errorf("Detail = %q, want it to carry the opaque output %q", failedResult.Detail, body)
		}

		execErr := errors.New(`exec: permission denied`)
		errRunner := &fakeRecipeRunner{out: "", exitCode: 0, err: execErr}
		errAdapter, ok := newScriptRecipeAdapter(key, scriptPath, errRunner)
		if !ok {
			t.Fatal("newScriptRecipeAdapter returned ok=false for a valid script, want ok=true")
		}
		errResult := errAdapter.OpenWindow(spacedCommand())
		if errResult.Outcome != OutcomeSpawnFailed {
			t.Errorf("exec-error Outcome = %v, want OutcomeSpawnFailed", errResult.Outcome)
		}
	})

	t.Run("it never returns permission-required from a script recipe", func(t *testing.T) {
		scriptPath := filepath.Join(t.TempDir(), "s.sh")
		writeExecutableScript(t, scriptPath)

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
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				runner := &fakeRecipeRunner{out: tc.out, exitCode: tc.exitCode, err: tc.err}
				adapter, ok := newScriptRecipeAdapter(key, scriptPath, runner)
				if !ok {
					t.Fatal("newScriptRecipeAdapter returned ok=false for a valid script, want ok=true")
				}

				result := adapter.OpenWindow(spacedCommand())

				if result.Outcome == OutcomePermissionRequired {
					t.Errorf("Outcome = OutcomePermissionRequired, want it NEVER produced by a script recipe (out=%q code=%d err=%v)", tc.out, tc.exitCode, tc.err)
				}
				if strings.TrimSpace(result.Guidance) != "" {
					t.Errorf("Guidance = %q, want empty — script recipes have no permission guidance", result.Guidance)
				}
			})
		}
	})
}
