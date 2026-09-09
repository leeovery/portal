package hooks_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// assertNoHook pins the whole no-hook answer: the lookup degrades rather than
// erroring, reports no hit, and yields no command.
func assertNoHook(t *testing.T, cmd string, ok bool, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("got ok=true, want false")
	}
	if cmd != "" {
		t.Errorf("got cmd=%q, want empty", cmd)
	}
}

func TestLookupOnResume(t *testing.T) {
	t.Run("returns no-hook when hooks.json is missing", func(t *testing.T) {
		store := hooks.NewStore(filepath.Join(t.TempDir(), "hooks.json"))

		cmd, ok, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, cmd, ok, err)
	})

	t.Run("returns no-hook when hooks.json is malformed JSON", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: "{not json"})

		cmd, ok, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, cmd, ok, err)
	})

	t.Run("returns no-hook when the hook-key is absent", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"other-session:0.0":{"on-resume":"echo hi"}}`})

		cmd, ok, err := store.LookupOnResume("missing-session:0.0", hooks.ViaHydrate)
		assertNoHook(t, cmd, ok, err)
	})

	t.Run("returns no-hook when the key has no on-resume event", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-attach":"echo attached"}}`})

		cmd, ok, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, cmd, ok, err)
	})

	t.Run("returns no-hook when the on-resume command is empty string", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-resume":""}}`})

		cmd, ok, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		assertNoHook(t, cmd, ok, err)
	})

	t.Run("it returns the registered on-resume command through the store method", func(t *testing.T) {
		const want = "echo hello world; ls -la"
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"session:0.0":{"on-resume":"echo hello world; ls -la"}}`})

		cmd, ok, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Errorf("got ok=false, want true")
		}
		if cmd != want {
			t.Errorf("got cmd=%q, want %q", cmd, want)
		}
	})

	t.Run("round-trips hook keys containing colons in the session name", func(t *testing.T) {
		const want = "ls"
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"work:foo:0.0":{"on-resume":"ls"}}`})

		cmd, ok, err := store.LookupOnResume("work:foo:0.0", hooks.ViaHydrate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Errorf("got ok=false, want true")
		}
		if cmd != want {
			t.Errorf("got cmd=%q, want %q", cmd, want)
		}
	})

	t.Run("returns no hook for an empty key even when hooks.json holds an empty-key entry", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"":{"on-resume":"rm -rf /"}}`})

		cmd, ok, err := store.LookupOnResume("", hooks.ViaHydrate)
		assertNoHook(t, cmd, ok, err)
	})

	t.Run("does not trim a whitespace-only key", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"":{"on-resume":"rm -rf /"}}`})

		cmd, ok, err := store.LookupOnResume(" ", hooks.ViaHydrate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || cmd != "" {
			t.Errorf("got cmd=%q ok=%v, want a miss: a whitespace key must not collapse to the empty key", cmd, ok)
		}

		seeded, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{" ":{"on-resume":"echo spaced"}}`})
		cmd, ok, err = seeded.LookupOnResume(" ", hooks.ViaHydrate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok || cmd != "echo spaced" {
			t.Errorf("got cmd=%q ok=%v, want the literally-seeded whitespace key to hit", cmd, ok)
		}
	})

	t.Run("surfaces a wrapped I/O error distinct from the no-hook case", func(t *testing.T) {
		// An unreadable hooks.json reads as EISDIR, not ErrNotExist, so the
		// error propagates rather than degrading to "no hook".
		store, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		cmd, ok, err := store.LookupOnResume("session:0.0", hooks.ViaHydrate)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if ok {
			t.Errorf("got ok=true, want false")
		}
		if cmd != "" {
			t.Errorf("got cmd=%q, want empty", cmd)
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
				if _, _, err := store.LookupOnResume(hookstest.SubjectSeedA, via); err != nil {
					t.Fatalf("LookupOnResume: %v", err)
				}

				hookstest.AssertDegradedRead(t, sink, via.String())
			})
		}
	})

	t.Run("it refuses an empty hook key before reading the file", func(t *testing.T) {
		// An unreadable hooks.json would surface EISDIR out of the store's read,
		// so a clean miss here proves the file was never consulted.
		store, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		cmd, ok, err := store.LookupOnResume("", hooks.ViaHydrate)
		assertNoHook(t, cmd, ok, err)
	})
}
