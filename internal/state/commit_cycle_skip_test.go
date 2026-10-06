package state_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/state"
)

// skipWorld is pane X's live state in a world where the cycle skips it: named
// by a skeleton marker, carrying the resume pending marker, or in a session
// whose environment read fails while X waits, which carries it forward. A
// second session the capture always reaches keeps the capture from failing
// whole. Every confirmation is answered by the committer's own server, and
// counted.
type skipWorld struct {
	skeleton bool
	pending  bool
	envErr   error
	confirms int
}

func (w *skipWorld) ShowAllServerOptions() (string, error) {
	if !w.skeleton {
		return "", nil
	}
	return state.SkeletonMarkerPrefix + handOverKey + ` "1"`, nil
}

func (w *skipWorld) ListSessionNamesProbe() ([]string, error) { return []string{"work", "other"}, nil }

func (w *skipWorld) ListAllPanesWithFormat(string) (string, error) {
	flag := ""
	if w.pending {
		flag = "1"
	}
	return paneLineWithPending("work", 0, "main", "tiled", false, true, 1, "/tmp", true, "zsh", waitingPaneToken, flag) + "\n" + otherRow(), nil
}

func (w *skipWorld) ShowEnvironment(session string) (string, error) {
	if session == "work" {
		return "", w.envErr
	}
	return "", nil
}

func (w *skipWorld) ConfirmAnswering() (int, error) {
	w.confirms++
	return ownServerPID, nil
}

var skipWorlds = []struct {
	name  string
	world func() *skipWorld
	set   func(state.CaptureCycle) map[string]struct{}
}{
	{
		name:  "skeleton-marked",
		world: func() *skipWorld { return &skipWorld{skeleton: true} },
		set:   func(c state.CaptureCycle) map[string]struct{} { return c.Skeleton },
	},
	{
		name:  "waiting",
		world: func() *skipWorld { return &skipWorld{pending: true} },
		set:   func(c state.CaptureCycle) map[string]struct{} { return c.Pending },
	},
	{
		name:  "carried",
		world: func() *skipWorld { return &skipWorld{pending: true, envErr: errors.New("environment read failed")} },
		set:   func(c state.CaptureCycle) map[string]struct{} { return c.Carried },
	},
}

// skippedWrite is what one write of X's capture through the cycle's writer
// answered, and what it touched.
type skippedWrite struct {
	written      bool
	err          error
	confirms     int
	entryBefore  uint64
	hadBefore    bool
	entryAfter   uint64
	hadAfter     bool
	inSkipSet    bool
	skipsAnswers bool
}

// skipSeed commits the index X was saved in, its record naming its positional
// file, which holds its transcript.
func skipSeed(t *testing.T, dir string) state.Index {
	t.Helper()
	idx := waitingIndex(waitingPaneToken, "scrollback/"+handOverPositionalFile())
	seedScrollback(t, dir, handOverPositionalFile(), handOverTranscript)
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	idx.Canonicalize()
	return idx
}

// runSkipCycle commits one cycle over world. A nil data dumps nothing; any
// other hands the writer that capture for X.
func runSkipCycle(t *testing.T, dir string, world *skipWorld, set func(state.CaptureCycle) map[string]struct{}, data []byte) skippedWrite {
	t.Helper()
	prev := skipSeed(t, dir)
	hm := state.HashMap{handOverKey: xxhash.Sum64String(handOverTranscript)}
	var got skippedWrite
	_, err := state.RunCommitCycle(state.CommitCycle{
		OwnServer: ownServerPID,
		Client:    world,
		Dir:       dir,
		LoadPrev:  func() *state.Index { return &prev },
		HashMap:   hm,
		Dump: func(c state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
			_, got.inSkipSet = set(c)[handOverKey]
			got.skipsAnswers = c.SkipsScrollback(handOverKey)
			if data == nil {
				return false, nil
			}
			got.entryBefore, got.hadBefore = hm[handOverKey]
			confirmsBefore := world.confirms
			got.written, got.err = w.Write(handOverKey, data, xxhash.Sum64(data))
			got.confirms = world.confirms - confirmsBefore
			got.entryAfter, got.hadAfter = hm[handOverKey]
			return got.written, nil
		},
	})
	if err != nil {
		t.Fatalf("RunCommitCycle: %v", err)
	}
	return got
}

func TestCycleScrollbackWriterRefusesAPaneTheCycleSkips(t *testing.T) {
	captures := []struct {
		name string
		data []byte
	}{
		{"a live capture", []byte("live-capture")},
		{"an empty capture its own server confirms", []byte{}},
	}
	for _, sw := range skipWorlds {
		for _, capture := range captures {
			t.Run(sw.name+" pane handed "+capture.name, func(t *testing.T) {
				dir := t.TempDir()
				got := runSkipCycle(t, dir, sw.world(), sw.set, capture.data)

				if !got.inSkipSet || !got.skipsAnswers {
					t.Fatalf("X in the cycle's %s set = %v, SkipsScrollback = %v; want both true", sw.name, got.inSkipSet, got.skipsAnswers)
				}
				if got.written || got.err != nil {
					t.Errorf("Write = (%v, %v), want (false, nil) for a pane the cycle skips", got.written, got.err)
				}
				if got.confirms != 0 {
					t.Errorf("Write sent %d confirmation reads, want none", got.confirms)
				}
				if got.hadAfter != got.hadBefore || got.entryAfter != got.entryBefore {
					t.Errorf("dedup entry for X = (%d, present %v), want it untouched at (%d, present %v)",
						got.entryAfter, got.hadAfter, got.entryBefore, got.hadBefore)
				}

				control := t.TempDir()
				runSkipCycle(t, control, sw.world(), sw.set, nil)
				want := recordFor(t, onDiskIndex(t, control), "work").ScrollbackFile
				named := recordFor(t, onDiskIndex(t, dir), "work").ScrollbackFile
				if named != want {
					t.Errorf("sessions.json names %q for X, want %q as a cycle whose dump never handed it over", named, want)
				}
				if body := readScrollback(t, dir, strings.TrimPrefix(named, "scrollback/")); body != handOverTranscript {
					t.Errorf("%s = %q, want the saved transcript %q", named, body, handOverTranscript)
				}
			})
		}
	}
}
