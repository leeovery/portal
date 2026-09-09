package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/tmux"
)

// rmCase describes one `hook rm` route: how the pane resolves, what the store
// holds, and what the command is handed.
type rmCase struct {
	name   string
	paneID string
	// resolver answers the pane read on the resolved-token path. A paneKeyPath
	// row leaves it nil: rmPaneSeams builds the poisoned pair for it instead, and
	// a row naming both is refused rather than silently dropping this one.
	resolver    *mockKeyResolver
	paneKeyPath bool
	seeded      map[string]map[string]string
	extra       []string
	wantErr     bool
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
	stamper   *recordingPaneStamper
	minted    int
}

// rmPaneSeams decides which pane pair a row is driven against, and refuses a
// row whose fields contradict each other.
//
// A --pane-key row gets the poisoned pair built here, per row, so a second such
// row cannot count against another's calls; it comes back as poisoned too, so
// the caller knows to guard it. A row naming its own resolver gets that one. A
// row naming neither leaves the seam nil, so hookSeams fills in the production
// resolver — handing back tt.resolver there would pass a typed-nil through the
// interface, which is non-nil to hookSeams and panics inside the mock on the
// first read.
//
// It reports through the *testing.T subset so its own refusal is testable.
func rmPaneSeams(t harnesstest.TestingT, tt rmCase) (resolver HookKeyResolver, poisoned *mockKeyResolver, stamper *recordingPaneStamper) {
	t.Helper()

	switch {
	case tt.paneKeyPath && tt.resolver != nil:
		t.Fatalf("rmCase %q names both a pane key and a resolver: the --pane-key path is driven against the poisoned pair, so the resolver would be discarded", tt.name)
		return nil, nil, nil
	case tt.paneKeyPath:
		poisoned, stamper = paneKeyPathSeams()
		return poisoned, poisoned, stamper
	case tt.resolver != nil:
		return tt.resolver, nil, &recordingPaneStamper{}
	default:
		return nil, nil, &recordingPaneStamper{}
	}
}

// runRmCase stages tt, drives `hook rm`, and holds it to its expected exit.
func runRmCase(t *testing.T, tt rmCase) rmOutcome {
	t.Helper()

	if tt.holdLock {
		hooks.SetLockTimeoutForTest(t, lockBound)
	}
	_, hooksFile := hooksFileInTempDir(t, tt.seeded)
	t.Setenv("TMUX_PANE", tt.paneID)
	before := readFileBytes(t, hooksFile)
	if tt.holdLock {
		hookstest.HoldHooksSidecar(t, hooksFile)
	}

	resolver, poisoned, stamper := rmPaneSeams(t, tt)
	minted := 0
	withHooksDeps(t, HooksDeps{
		KeyResolver: resolver,
		PaneStamper: stamper,
		TokenMinter: func() (string, error) { minted++; return hookstest.SubjectSeedC, nil },
	})

	out, err := runHookRm(t, tt.extra...)
	if tt.wantErr != (err != nil) {
		t.Fatalf("hook rm error = %v, wantErr %v", err, tt.wantErr)
	}
	if poisoned != nil {
		assertNoPaneTmuxCalls(t, poisoned, stamper)
	}

	return rmOutcome{hooksFile: hooksFile, before: before, out: out, err: err, stamper: stamper, minted: minted}
}

func TestRmCaseRows(t *testing.T) {
	t.Run("it rejects a case naming both a pane key and a resolver", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		rec.Run(func() {
			rmPaneSeams(rec, rmCase{
				name:        "contradictory row",
				paneKeyPath: true,
				resolver:    &mockKeyResolver{key: hookstest.SubjectSeedA},
			})
		})

		if len(rec.Fatals) != 1 {
			t.Fatalf("got %d fatals, want exactly 1: %v", len(rec.Fatals), rec.Fatals)
		}
		if want := "names both a pane key and a resolver"; !strings.Contains(rec.Fatals[0], want) {
			t.Errorf("refusal %q does not say %q", rec.Fatals[0], want)
		}
		if !strings.Contains(rec.Fatals[0], "contradictory row") {
			t.Errorf("refusal %q does not name the row it refused", rec.Fatals[0])
		}
	})

	t.Run("it falls back to the production resolver for a case naming neither", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		var resolver HookKeyResolver
		var poisoned *mockKeyResolver
		var stamper *recordingPaneStamper
		rec.Run(func() {
			resolver, poisoned, stamper = rmPaneSeams(rec, rmCase{name: "neither"})
		})

		if rec.Failed() {
			t.Fatalf("a row naming neither was refused: %s", rec.Report())
		}
		// A nil seam, not a typed-nil one: hookSeams fills a nil KeyResolver with
		// the production client, and would call straight through a typed-nil.
		if resolver != nil {
			t.Errorf("KeyResolver seam = %#v, want nil so hookSeams fills its production default", resolver)
		}
		if poisoned != nil {
			t.Errorf("poisoned resolver = %#v, want nil off the --pane-key path", poisoned)
		}
		if stamper == nil {
			t.Error("stamper = nil, want the recording stamper every row is watched by")
		}
	})
}

func TestHooksRmExitsZeroOnlyWhenItRemoved(t *testing.T) {
	t.Run("it exits non-zero for a pane no live pane answers to", func(t *testing.T) {
		const stderr = "no such pane: %999"

		hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%999")

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{err: &tmux.CommandError{Stderr: stderr}}})

		_, err := runHookRm(t)
		if err == nil {
			t.Fatal("expected an error for a pane no live pane answers to, got nil")
		}
		if err.Error() != stderr {
			t.Errorf("error = %q, want tmux's own words %q unaltered", err.Error(), stderr)
		}
		if _, ok := errors.AsType[*tmux.CommandError](err); !ok {
			t.Errorf("errors.AsType[*tmux.CommandError](err) = false; want a recoverable *tmux.CommandError; err = %v", err)
		}
	})

	t.Run("it exits non-zero with its own words for a live pane carrying no token", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: ""}})

		_, err := runHookRm(t)
		if err == nil {
			t.Fatal("expected an error for a live pane carrying no token, got nil")
		}
		if err.Error() != "no resume hook registered for this pane" {
			t.Errorf("error = %q, want %q", err.Error(), "no resume hook registered for this pane")
		}
	})

	t.Run("it consults the store for nothing when the pane carries no token", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: ""}})

		if _, err := runHookRm(t); err == nil {
			t.Fatal("expected an error for a live pane carrying no token, got nil")
		}
		if _, err := os.Stat(hooksFile); err == nil {
			t.Error("hooks.json was created for a pane carrying no token: the store must not be consulted")
		}
	})

	t.Run("it exits non-zero when the resolved token has no entry", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		out, err := runHookRm(t)
		if err == nil {
			t.Fatal("expected an error when the resolved token has no entry, got nil")
		}
		want := fmt.Sprintf("no resume hook registered for %s", hookstest.SubjectSeedA)
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
		if out != "" {
			t.Errorf("output = %q, want empty string", out)
		}
	})

	t.Run("it exits non-zero when --pane-key names no entry", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		// The poisoned resolver doubles as an assertion on the message: a body
		// that resolved the pane would surface the seam's error in place of it.
		resolver, stamper := paneKeyPathSeams()
		withHooksDeps(t, HooksDeps{KeyResolver: resolver, PaneStamper: stamper})

		_, err := runHookRm(t, "--pane-key", hookstest.UnjudgeableSeedA)
		if err == nil {
			t.Fatal("expected an error when --pane-key names no entry, got nil")
		}
		want := fmt.Sprintf("no resume hook registered for %s", hookstest.UnjudgeableSeedA)
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
		assertNoPaneTmuxCalls(t, resolver, stamper)
	})

	t.Run("it exits 0 and removes on the resolved-token path", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "claude --resume abc"},
			hookstest.SubjectSeedB: {"on-resume": "npm start"},
		})
		// Driven from a pane ID that is not the resolved key, so the entry the
		// removal lands on is the key the resolver answered with and never the
		// raw pane ID.
		t.Setenv("TMUX_PANE", "%42")

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		if _, err := runHookRm(t); err != nil {
			t.Fatalf("hook rm: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data[hookstest.SubjectSeedA]; ok {
			t.Error("expected the resolved token's entry to be removed")
		}
		if _, ok := data["%42"]; ok {
			t.Error("raw pane ID %42 should not be used as key")
		}
		if data[hookstest.SubjectSeedB]["on-resume"] != "npm start" {
			t.Errorf("hooks.json = %v, want the other pane's entry left in place", data)
		}
	})

	t.Run("it exits 0 and removes on the --pane-key path", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.UnjudgeableSeedA: {"on-resume": "claude --resume abc"},
			hookstest.SubjectSeedB:     {"on-resume": "npm start"},
		})
		t.Setenv("TMUX_PANE", "")

		resolver, stamper := paneKeyPathSeams()
		withHooksDeps(t, HooksDeps{KeyResolver: resolver, PaneStamper: stamper})

		// An old-format key: the pass-through validates nothing, so removing one
		// by hand keeps working and keeps exiting 0.
		if _, err := runHookRm(t, "--pane-key", hookstest.UnjudgeableSeedA); err != nil {
			t.Fatalf("hook rm --pane-key: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data[hookstest.UnjudgeableSeedA]; ok {
			t.Error("expected the verbatim key's entry to be removed")
		}
		assertNoPaneTmuxCalls(t, resolver, stamper)
	})

	t.Run("it leaves hooks.json byte-identical on every failing route", func(t *testing.T) {
		anotherPane := map[string]map[string]string{
			hookstest.SubjectSeedB: {"on-resume": "npm start"},
		}

		tests := []rmCase{
			{
				name:     "gone pane",
				paneID:   "%999",
				resolver: &mockKeyResolver{err: &tmux.CommandError{Stderr: "no such pane: %999"}},
				seeded:   anotherPane,
				wantErr:  true,
			},
			{
				name:     "unset TMUX_PANE",
				paneID:   "",
				resolver: &mockKeyResolver{key: hookstest.SubjectSeedA},
				seeded:   anotherPane,
				wantErr:  true,
			},
			{
				// Seeded with an empty-key entry too: the unresolved key must not
				// match one, which it could only do by reaching the store at all.
				name:     "live pane carrying no token",
				paneID:   "%3",
				resolver: &mockKeyResolver{key: ""},
				seeded: map[string]map[string]string{
					"":                     {"on-resume": "an empty-key entry"},
					hookstest.SubjectSeedB: {"on-resume": "npm start"},
				},
				wantErr: true,
			},
			{
				name:     "resolved token naming no entry",
				paneID:   "%3",
				resolver: &mockKeyResolver{key: hookstest.SubjectSeedA},
				seeded:   anotherPane,
				wantErr:  true,
			},
			{
				name:        "--pane-key naming no entry",
				paneID:      "%3",
				paneKeyPath: true,
				seeded:      anotherPane,
				extra:       []string{"--pane-key", hookstest.UnjudgeableSeedA},
				wantErr:     true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := runRmCase(t, tt)
				assertHooksFileUnchanged(t, got.hooksFile, got.before)
			})
		}
	})

	t.Run("it mints and stamps nothing on either path", func(t *testing.T) {
		onePane := map[string]map[string]string{hookstest.SubjectSeedA: {"on-resume": "npm start"}}

		tests := []rmCase{
			{
				name:     "successful resolved-token removal",
				paneID:   "%3",
				resolver: &mockKeyResolver{key: hookstest.SubjectSeedA},
				seeded:   onePane,
			},
			{
				name:     "live pane carrying no token",
				paneID:   "%3",
				resolver: &mockKeyResolver{key: ""},
				seeded:   onePane,
				wantErr:  true,
			},
			{
				name:        "--pane-key",
				paneID:      "",
				paneKeyPath: true,
				seeded:      onePane,
				extra:       []string{"--pane-key", hookstest.SubjectSeedA},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := runRmCase(t, tt)

				if got.minted != 0 {
					t.Errorf("mint count = %d, want 0: hook rm mints nothing", got.minted)
				}
				if len(got.stamper.calls) != 0 {
					t.Errorf("set-option call count = %d, want 0: hook rm neither stamps nor unstamps: %+v",
						len(got.stamper.calls), got.stamper.calls)
				}
			})
		}
	})

	t.Run("it touches no dirty flag on either path", func(t *testing.T) {
		onePane := map[string]map[string]string{hookstest.SubjectSeedA: {"on-resume": "npm start"}}

		tests := []rmCase{
			{
				name:     "successful resolved-token removal",
				paneID:   "%3",
				resolver: &mockKeyResolver{key: hookstest.SubjectSeedA},
				seeded:   onePane,
			},
			{
				name:     "resolved token removing nothing",
				paneID:   "%3",
				resolver: &mockKeyResolver{key: hookstest.SubjectSeedD},
				seeded:   onePane,
				wantErr:  true,
			},
			{
				name:        "--pane-key removal",
				paneID:      "",
				paneKeyPath: true,
				seeded:      onePane,
				extra:       []string{"--pane-key", hookstest.SubjectSeedA},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stateDir := t.TempDir()
				t.Setenv("PORTAL_STATE_DIR", stateDir)

				runRmCase(t, tt)

				if saveRequestedExists(t, stateDir) {
					t.Error("hook rm touched save.requested")
				}
			})
		}
	})

	t.Run("it reports removing nothing as a plain error, not a usage error", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{})
		t.Setenv("TMUX_PANE", "%3")

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		out, err := runHookRm(t)
		if err == nil {
			t.Fatal("expected a non-zero exit, got nil")
		}

		// main's classify prints a non-silent, non-usage error to stderr and exits
		// 1; cobra's own SilenceUsage/SilenceErrors keep the streams clean.
		if _, ok := errors.AsType[*UsageError](err); ok {
			t.Errorf("error %v is a *UsageError; removing nothing is not a usage error", err)
		}
		if IsSilentExitError(err) {
			t.Error("error is a silent-exit error; its message would never reach stderr")
		}
		if strings.Contains(out, "Usage:") {
			t.Errorf("command output = %q, want no usage dump", out)
		}
	})
}
