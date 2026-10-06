package state_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// uncommittedCommitter is one committer's shape of the cycle: where its
// previous index comes from, and what it hands the cycle to dump with. dump is
// what runs after the cycle's re-file and before its commit.
type uncommittedCommitter struct {
	name  string
	cycle func(t *testing.T, dir string, seed state.Index, dump func(state.CaptureCycle, state.ScrollbackWriter) (bool, error)) state.CommitCycle
}

var uncommittedCommitters = []uncommittedCommitter{
	{"the daemon's tick or shutdown flush", func(_ *testing.T, dir string, seed state.Index, dump func(state.CaptureCycle, state.ScrollbackWriter) (bool, error)) state.CommitCycle {
		return state.CommitCycle{
			OwnServer: ownServerPID,
			Client:    &worldClient{world: waitingWorld()},
			Dir:       dir,
			LoadPrev:  func() *state.Index { return &seed },
			HashMap:   state.HashMap{},
			Dump:      dump,
		}
	}},
	{"commit-now", func(t *testing.T, dir string, _ state.Index, dump func(state.CaptureCycle, state.ScrollbackWriter) (bool, error)) state.CommitCycle {
		var prevs []state.Index
		cycle := commitNowCycle(t, &worldClient{world: waitingWorld()}, dir, &prevs)
		cycle.Dump = dump
		return cycle
	}},
}

// waitingWorld is the live state in which X has just gone waiting: no longer
// skeleton-marked, its resume pending marker set.
func waitingWorld() *handOverWorld {
	world := newHandOverWorld()
	world.markPending()
	return world
}

// standsDownAfterTheRefile ends the cycle the way a stand-down does, once X's
// transcript has been filed under its token.
func standsDownAfterTheRefile(t *testing.T, dir string) func(state.CaptureCycle, state.ScrollbackWriter) (bool, error) {
	return func(state.CaptureCycle, state.ScrollbackWriter) (bool, error) {
		assertTokenFileHoldsTranscript(t, dir)
		return false, fmt.Errorf("injected: %w", state.ErrTmuxStoppedAnswering)
	}
}

// failsTheWriteAfterTheRefile lets the cycle reach its commit with the state
// directory refusing the new sessions.json, once X's transcript has been filed
// under its token.
func failsTheWriteAfterTheRefile(t *testing.T, dir string) func(state.CaptureCycle, state.ScrollbackWriter) (bool, error) {
	return func(state.CaptureCycle, state.ScrollbackWriter) (bool, error) {
		assertTokenFileHoldsTranscript(t, dir)
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatalf("chmod state dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		return true, nil
	}
}

func assertTokenFileHoldsTranscript(t *testing.T, dir string) {
	t.Helper()
	if got := readScrollback(t, dir, handOverTokenFile()); got != handOverTranscript {
		t.Errorf("token-named file = %q, want %q", got, handOverTranscript)
	}
}

// assertSavedScrollbackPresent fails for every scrollback path sessions.json
// names that is not on disk.
func assertSavedScrollbackPresent(t *testing.T, dir string) {
	t.Helper()
	for _, s := range onDiskIndex(t, dir).Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.ScrollbackFile == "" {
					continue
				}
				if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p.ScrollbackFile))); err != nil {
					t.Errorf("sessions.json names %q for %s:%d.%d, which is not on disk: %v", p.ScrollbackFile, s.Name, w.Index, p.Index, err)
				}
			}
		}
	}
}

// assertRestoreFindsXTranscript reads X's transcript at the path its saved
// record names, the path the next restore replays from.
func assertRestoreFindsXTranscript(t *testing.T, dir string) {
	t.Helper()
	named := recordFor(t, onDiskIndex(t, dir), "work").ScrollbackFile
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(named)))
	if err != nil {
		t.Fatalf("read X's transcript at %q: %v", named, err)
	}
	if string(data) != handOverTranscript {
		t.Errorf("X's saved record names %q holding %q, want %q", named, data, handOverTranscript)
	}
}

func TestRunCommitCycleEndingUncommittedLeavesEverySavedScrollbackPathOnDisk(t *testing.T) {
	ends := []struct {
		name  string
		after func(t *testing.T, dir string) func(state.CaptureCycle, state.ScrollbackWriter) (bool, error)
		check func(t *testing.T, err error)
	}{
		{"a stand-down after the re-file", standsDownAfterTheRefile, func(t *testing.T, err error) {
			if !errors.Is(err, state.ErrTmuxStoppedAnswering) {
				t.Fatalf("error = %v, want the injected stand-down", err)
			}
		}},
		{"a failed sessions.json write after the re-file", failsTheWriteAfterTheRefile, func(t *testing.T, err error) {
			if err == nil {
				t.Fatal("cycle returned nil, want the failed sessions.json write")
			}
		}},
	}
	for _, committer := range uncommittedCommitters {
		for _, end := range ends {
			t.Run(committer.name+" ending on "+end.name, func(t *testing.T) {
				dir := t.TempDir()
				seed := handOverSeed(t, dir)
				sessionsBefore, err := os.ReadFile(state.SessionsJSON(dir))
				if err != nil {
					t.Fatalf("read seeded sessions.json: %v", err)
				}

				_, err = state.RunCommitCycle(committer.cycle(t, dir, seed, end.after(t, dir)))

				end.check(t, err)
				sessionsAfter, readErr := os.ReadFile(state.SessionsJSON(dir))
				if readErr != nil {
					t.Fatalf("read sessions.json: %v", readErr)
				}
				if string(sessionsAfter) != string(sessionsBefore) {
					t.Fatalf("sessions.json rewritten by a cycle that ended uncommitted:\nbefore %s\nafter  %s", sessionsBefore, sessionsAfter)
				}
				assertSavedScrollbackPresent(t, dir)
				assertRestoreFindsXTranscript(t, dir)
			})
		}
	}
}

func TestRunCommitCycleCommittingAfterAnUncommittedEndFilesXUnderItsTokenAlone(t *testing.T) {
	dir := t.TempDir()
	seed := handOverSeed(t, dir)
	cycle := uncommittedCommitters[0].cycle
	if _, err := state.RunCommitCycle(cycle(t, dir, seed, standsDownAfterTheRefile(t, dir))); err == nil {
		t.Fatal("first cycle returned nil, want the injected stand-down")
	}

	if _, err := state.RunCommitCycle(cycle(t, dir, seed, nil)); err != nil {
		t.Fatalf("committing cycle: %v", err)
	}

	assertTranscriptFiledUnderToken(t, dir)
	if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), handOverPositionalFile())); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("vacated positional file stat err = %v, want not-exist once the committed housekeeping pass ran", err)
	}
	assertSavedScrollbackPresent(t, dir)
}
