package hooks_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resumemode"
)

// assertNoHook pins the whole no-hook answer: the lookup degrades rather than
// erroring, and reports the zero result — no command, no mode, no hit.
func assertNoHook(t *testing.T, got hooks.OnResume, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (hooks.OnResume{}) {
		t.Errorf("got %+v, want the zero result", got)
	}
}

func assertHook(t *testing.T, got hooks.OnResume, err error, wantCommand string, wantMode resumemode.Mode) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := hooks.OnResume{Command: wantCommand, Mode: wantMode, Found: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLookupOnResume(t *testing.T) {
	t.Run("returns no-hook when hooks.json is missing", func(t *testing.T) {
		store := hooks.NewStore(filepath.Join(t.TempDir(), "hooks.json"))

		got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, got, err)
	})

	t.Run("returns no-hook when hooks.json is malformed JSON", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: "{not json"})

		got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, got, err)
	})

	t.Run("returns no-hook when the hook-key is absent", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"other-session:0.0":{"on-resume":"echo hi"}}`})

		got, err := store.LookupOnResume("missing-session:0.0", hooks.ViaHydrate)
		assertNoHook(t, got, err)
	})

	t.Run("returns no-hook when the key has no on-resume event", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-attach":"echo attached"}}`})

		got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, got, err)
	})

	t.Run("returns no-hook when the on-resume command is empty string", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-resume":""}}`})

		got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, got, err)
	})

	t.Run("it reports the command and no mode for a string-form entry", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-resume":"echo hello world; ls -la"}}`})

		got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertHook(t, got, err, "echo hello world; ls -la", resumemode.Unset)
	})

	t.Run("it reports the command and the mode for an object-form entry", func(t *testing.T) {
		for _, mode := range []resumemode.Mode{resumemode.Eager, resumemode.Lazy} {
			t.Run(mode.String(), func(t *testing.T) {
				store, _ := hookstest.StageStore(t, hookstest.Staging{
					Seed: `{"session:0.0":{"on-resume":{"command":"claude --resume abc","resume":"` + mode.String() + `"}}}`,
				})

				got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
				assertHook(t, got, err, "claude --resume abc", mode)
			})
		}
	})

	t.Run("it reports no mode when the stored resume is absent, empty or unrecognised", func(t *testing.T) {
		tests := []struct {
			name  string
			entry string
		}{
			{name: "absent", entry: `{"command":"echo hi"}`},
			{name: "empty", entry: `{"command":"echo hi","resume":""}`},
			{name: "unrecognised word", entry: `{"command":"echo hi","resume":"sometimes"}`},
			{name: "capitalised", entry: `{"command":"echo hi","resume":"Lazy"}`},
			{name: "not a string", entry: `{"command":"echo hi","resume":7}`},
			{name: "null", entry: `{"command":"echo hi","resume":null}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-resume":` + tt.entry + `}}`})

				got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
				assertHook(t, got, err, "echo hi", resumemode.Unset)
			})
		}
	})

	t.Run("it reports a miss for an object carrying no command key", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-resume":{"resume":"eager"}}}`})

		got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, got, err)
	})

	t.Run("it reports a miss for an object whose command is empty", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-resume":{"command":"","resume":"eager"}}}`})

		got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, got, err)
	})

	t.Run("it reports a miss for a value that is neither a string nor an object", func(t *testing.T) {
		for _, value := range []string{"7", "true", "null", `["echo hi"]`} {
			t.Run(value, func(t *testing.T) {
				store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-resume":` + value + `}}`})

				got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
				assertNoHook(t, got, err)
			})
		}
	})

	t.Run("round-trips hook keys containing colons in the session name", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"work:foo:0.0":{"on-resume":"ls"}}`})

		got, err := store.LookupOnResume("work:foo:0.0", hooks.ViaHydrate)
		assertHook(t, got, err, "ls", resumemode.Unset)
	})

	t.Run("returns no hook for an empty key even when hooks.json holds an empty-key entry", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"":{"on-resume":"rm -rf /"}}`})

		got, err := store.LookupOnResume("", hooks.ViaHydrate)
		assertNoHook(t, got, err)
	})

	t.Run("does not trim a whitespace-only key", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"":{"on-resume":"rm -rf /"}}`})

		got, err := store.LookupOnResume(" ", hooks.ViaHydrate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != (hooks.OnResume{}) {
			t.Errorf("got %+v, want a miss: a whitespace key must not collapse to the empty key", got)
		}

		seeded, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{" ":{"on-resume":"echo spaced"}}`})
		got, err = seeded.LookupOnResume(" ", hooks.ViaHydrate)
		assertHook(t, got, err, "echo spaced", resumemode.Unset)
	})

	t.Run("it returns the zero result alongside a read error", func(t *testing.T) {
		// An unreadable hooks.json reads as EISDIR, not ErrNotExist, so the
		// error propagates rather than degrading to "no hook".
		store, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		got, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if got != (hooks.OnResume{}) {
			t.Errorf("got %+v, want the zero result", got)
		}
		if !strings.Contains(err.Error(), "load hooks") {
			t.Errorf("error %q does not contain %q", err.Error(), "load hooks")
		}
	})

	t.Run("it records the caller's via on a degraded lookup rather than a hardcoded one", func(t *testing.T) {
		for _, via := range []hooks.Via{hooks.ViaHydrate, hooks.ViaDoctor} {
			t.Run(via.String(), func(t *testing.T) {
				hooks.SetLockTimeoutForTest(t, 20*time.Millisecond)
				store, path := hookstest.StageStore(t, hookstest.Staging{Entries: map[string]string{hookstest.SubjectSeedA: "echo hi"}})
				hookstest.HoldHooksSidecar(t, path)

				sink := logtest.Install(t)
				if _, err := store.LookupOnResume(hookstest.SubjectSeedA, via); err != nil {
					t.Fatalf("LookupOnResume: %v", err)
				}

				hookstest.AssertDegradedRead(t, sink, via.String())
			})
		}
	})

	t.Run("it reports a miss for an empty hook key without reading the file", func(t *testing.T) {
		// An unreadable hooks.json would surface EISDIR out of the store's read,
		// so a clean miss here proves the file was never consulted.
		store, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		got, err := store.LookupOnResume("", hooks.ViaHydrate)
		assertNoHook(t, got, err)
	})
}
