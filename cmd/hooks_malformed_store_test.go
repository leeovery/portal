package cmd

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
)

// A hooks.json that does not parse is the one input a mutation must never
// answer: `hook set` loading it as empty and writing back would replace every
// registration with the one it was asked to add, at exit 0. Both mutations
// refuse it on the same route a lock timeout takes — a plain error, exit 1,
// the file byte-identical.
func TestHookMutationsRefuseAMalformedStore(t *testing.T) {
	stageMalformed := func(t *testing.T) string {
		t.Helper()
		_, hooksFile := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), Seed: "not json"})
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		return hooksFile
	}

	t.Run("it exits non-zero from hook set and leaves the file byte-identical", func(t *testing.T) {
		hooksFile := stageMalformed(t)
		before := readFileBytes(t, hooksFile)

		err := runHookSetForKey(t, hookstest.SubjectSeedA, "claude --resume abc")

		assertMalformedRefusalReachesStderr(t, err)
		assertHooksFileUnchanged(t, hooksFile, before)
	})

	t.Run("it exits non-zero from hook rm --pane-key and leaves the file byte-identical", func(t *testing.T) {
		hooksFile := stageMalformed(t)
		before := readFileBytes(t, hooksFile)

		_, err := runHookRm(t, "--pane-key", hookstest.SubjectSeedA)

		assertMalformedRefusalReachesStderr(t, err)
		assertHooksFileUnchanged(t, hooksFile, before)
	})
}

func assertMalformedRefusalReachesStderr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a non-zero exit over a malformed hooks.json, got nil")
	}
	if !errors.Is(err, hooks.ErrMalformed) {
		t.Errorf("error = %v, want errors.Is ErrMalformed", err)
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		t.Errorf("error %v is a *UsageError; a file that does not parse is not a usage error", err)
	}
	if IsSilentExitError(err) {
		t.Error("error is a silent-exit error; its reason would never reach stderr")
	}
}
