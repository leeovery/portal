package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
)

// rmCase describes one `hook rm` route: the session it runs in, what the store
// holds, and what the command is handed.
type rmCase struct {
	name string
	// current is the session the command runs in, "" for none.
	current string
	seeded  map[string]map[string]string
	extra   []string
	wantErr bool
	// holdLock drives the row against a sidecar held elsewhere, with the
	// acquisition bound lowered to lockBound so the wait is one a test can sit
	// through. The hold is taken after the seed is staged and read, so the
	// before bytes are the file the timed-out mutation must leave alone.
	holdLock bool
}

// rmOutcome is what a driven case left behind for its table to assert on. The
// command's own output and error travel with it because a row whose subject is
// how a failure reaches the user asserts on both.
type rmOutcome struct {
	hooksFile string
	before    []byte
	out       string
	err       error
}

// runRmCase stages tt, drives `hook rm`, and holds it to its expected exit.
func runRmCase(t *testing.T, tt rmCase) rmOutcome {
	t.Helper()

	if tt.holdLock {
		hooks.SetLockTimeoutForTest(t, lockBound)
	}
	_, hooksFile := hooksFileInTempDir(t, tt.seeded)
	before := readFileBytes(t, hooksFile)
	if tt.holdLock {
		hookstest.HoldHooksSidecar(t, hooksFile)
	}
	inSession(t, tt.current)

	out, err := runHookRm(t, tt.extra...)
	if tt.wantErr != (err != nil) {
		t.Fatalf("hook rm error = %v, wantErr %v", err, tt.wantErr)
	}
	return rmOutcome{hooksFile: hooksFile, before: before, out: out, err: err}
}

func TestHooksRmExitsZeroOnlyWhenItRemoved(t *testing.T) {
	t.Run("it exits non-zero when the current session has no entry", func(t *testing.T) {
		got := runRmCase(t, rmCase{current: hookSessionA, wantErr: true})

		if want := fmt.Sprintf("no resume hook registered for %s", hookSessionA); got.err.Error() != want {
			t.Errorf("error = %q, want %q", got.err.Error(), want)
		}
		if got.out != "" {
			t.Errorf("output = %q, want empty", got.out)
		}
	})

	t.Run("it exits non-zero when --pane-key names no entry", func(t *testing.T) {
		got := runRmCase(t, rmCase{extra: []string{"--pane-key", hookstest.SubjectSeedB}, wantErr: true})

		if want := fmt.Sprintf("no resume hook registered for %s", hookstest.SubjectSeedB); got.err.Error() != want {
			t.Errorf("error = %q, want %q", got.err.Error(), want)
		}
	})

	t.Run("it leaves hooks.json byte-identical on every failing route", func(t *testing.T) {
		another := map[string]map[string]string{hookSessionB: {"on-resume": "npm start"}}

		for _, tt := range []rmCase{
			{
				// Seeded with an empty-key entry too: running outside a session must
				// not reach it, which it could only do by reaching the store at all.
				name: "outside a session",
				seeded: map[string]map[string]string{
					"":           {"on-resume": "an empty-key entry"},
					hookSessionB: {"on-resume": "npm start"},
				},
				wantErr: true,
			},
			{name: "current session naming no entry", current: hookSessionA, seeded: another, wantErr: true},
			{name: "--pane-key naming no entry", current: hookSessionA, seeded: another, extra: []string{"--pane-key", hookstest.SubjectSeedB}, wantErr: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				got := runRmCase(t, tt)
				assertHooksFileUnchanged(t, got.hooksFile, got.before)
			})
		}
	})

	t.Run("it reports removing nothing as a plain error, not a usage error", func(t *testing.T) {
		got := runRmCase(t, rmCase{current: hookSessionA, seeded: map[string]map[string]string{}, wantErr: true})

		// main's classify prints a non-silent, non-usage error to stderr and exits
		// 1; cobra's own SilenceUsage/SilenceErrors keep the streams clean.
		if _, ok := errors.AsType[*UsageError](got.err); ok {
			t.Errorf("error %v is a *UsageError; removing nothing is not a usage error", got.err)
		}
		if IsSilentExitError(got.err) {
			t.Error("error is a silent-exit error; its message would never reach stderr")
		}
		if strings.Contains(got.out, "Usage:") {
			t.Errorf("command output = %q, want no usage dump", got.out)
		}
	})
}
