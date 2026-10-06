package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/leeovery/portal/internal/state"
)

const (
	lagSiblingToken      = "tokyyy"
	lagSiblingTranscript = "y-transcript"
)

func yRow(session string, pending bool) string {
	flag := ""
	if pending {
		flag = "1"
	}
	return paneLineWithPending(session, 0, "main", "L", false, true, 1, "/live", false, "zsh", lagSiblingToken, flag)
}

// laggingPrev is the index the daemon holds in memory: session foo's panes X
// and Y, both stamped, each named at its positional transcript, beside a
// session other. It is committed as sessions.json before a commit-now runs.
func laggingPrev() state.Index {
	idx := prevIndexOf(
		prevPane{"foo", 0, state.Pane{Index: 0, CWD: "/x", Active: true, CurrentCommand: "claude", ScrollbackFile: "scrollback/foo__0.0.bin", PortalPaneID: waitingPaneToken}},
		prevPane{"foo", 0, state.Pane{Index: 1, CWD: "/y", CurrentCommand: "claude", ScrollbackFile: "scrollback/foo__0.1.bin", PortalPaneID: lagSiblingToken}},
		prevPane{"other", 0, state.Pane{Index: 0, CWD: "/o", Active: true, CurrentCommand: "zsh", ScrollbackFile: "scrollback/other__0.0.bin"}},
	)
	idx.Canonicalize()
	return idx
}

// commitNowFilesBothWaitingPanes commits laggingPrev, then runs a commit-now
// over X and Y both waiting, which files each under its token and removes
// their positional files. It returns laggingPrev, which now lags sessions.json.
func commitNowFilesBothWaitingPanes(t *testing.T, dir string) state.Index {
	t.Helper()
	prev := laggingPrev()
	seedScrollback(t, dir, "foo__0.0.bin", carryTranscript)
	seedScrollback(t, dir, "foo__0.1.bin", lagSiblingTranscript)
	seedScrollback(t, dir, "other__0.0.bin", "o-body")
	if err := state.Commit(dir, prev, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	bothWaiting := carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", true), yRow("foo", true), otherRow()}}
	mustCommit(t, committers[1], dir, bothWaiting, state.Index{})

	if got := readScrollback(t, dir, carryTokenFile()); got != carryTranscript {
		t.Fatalf("after the commit-now %s = %q, want %q", carryTokenFile(), got, carryTranscript)
	}
	if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), "foo__0.0.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("after the commit-now X's positional file stat err = %v, want it removed", err)
	}
	if p := findPane(onDiskIndex(t, dir), "foo", 0, 0); p == nil || p.ScrollbackFile != carryTokenPath() {
		t.Fatalf("after the commit-now sessions.json names %+v for X, want %q", p, carryTokenPath())
	}
	return prev
}

func assertEveryNamedScrollbackOnDisk(t *testing.T, dir string) {
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

// answeredInACarriedSession are the captures in which X has been answered and
// Y still waits while their session misses the capture.
var answeredInACarriedSession = map[string]carryWorld{
	"a rename landing after the pane read": {names: []string{"foo", "other"}, rows: []string{xRow("foo", false), yRow("foo", true), otherRow()}, envErrs: noSuchFoo()},
	"an environment read failing":          {names: []string{"foo", "other"}, rows: []string{xRow("foo", false), yRow("foo", true), otherRow()}, envErrs: map[string]error{"foo": errors.New("boom")}},
}

func TestDaemonCycleCarriesAnAnsweredPaneFromTheIndexACommitNowLeft(t *testing.T) {
	for name, missed := range answeredInACarriedSession {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			memPrev := commitNowFilesBothWaitingPanes(t, dir)

			capture := mustCommit(t, committers[0], dir, missed, memPrev)

			if _, carried := capture.Carried[state.SanitizePaneKey("foo", 0, 0)]; !carried {
				t.Fatalf("carried set = %v, want foo carried", capture.Carried)
			}
			assertXFiledAt(t, dir, "foo")
			assertEveryNamedScrollbackOnDisk(t, dir)
		})
	}
}

func committedIgnoringSaveTime(t *testing.T, dir string) state.Index {
	t.Helper()
	idx := onDiskIndex(t, dir)
	idx.SavedAt = time.Time{}
	return idx
}

func TestDaemonCycleAfterACommitNowMatchesOneWhoseIndexIsCurrent(t *testing.T) {
	worlds := map[string]carryWorld{
		"a skeleton-marked pane": {
			names:   []string{"foo", "other"},
			rows:    []string{xRow("foo", false), yRow("foo", true), otherRow()},
			markers: state.SkeletonMarkerPrefix + state.SanitizePaneKey("foo", 0, 0) + ` "1"`,
		},
		"a waiting pane beside an answered one": {
			names: []string{"foo", "other"},
			rows:  []string{xRow("foo", false), yRow("foo", true), otherRow()},
		},
		"both panes still waiting": {
			names: []string{"foo", "other"},
			rows:  []string{xRow("foo", true), yRow("foo", true), otherRow()},
		},
	}
	for name, w := range answeredInACarriedSession {
		worlds["a carried session: "+name] = w
	}
	for name, w := range worlds {
		t.Run(name, func(t *testing.T) {
			lagging := t.TempDir()
			memPrev := commitNowFilesBothWaitingPanes(t, lagging)
			current := t.TempDir()
			commitNowFilesBothWaitingPanes(t, current)

			mustCommit(t, committers[0], lagging, w, memPrev)
			mustCommit(t, committers[0], current, w, onDiskIndex(t, current))

			if got, want := committedIgnoringSaveTime(t, lagging), committedIgnoringSaveTime(t, current); !reflect.DeepEqual(got, want) {
				t.Errorf("committed index with a lagging in-memory index =\n%+v\nwant the one with a current index\n%+v", got, want)
			}
			if got, want := scrollbackContents(t, lagging), scrollbackContents(t, current); !reflect.DeepEqual(got, want) {
				t.Errorf("scrollback with a lagging in-memory index = %v, want the one with a current index %v", got, want)
			}
			assertEveryNamedScrollbackOnDisk(t, lagging)
		})
	}
}

func TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON(t *testing.T) {
	for _, c := range []struct {
		name string
		prev func(dir string) state.Index
	}{
		{name: "daemon tick", prev: func(string) state.Index { return laggingPrev() }},
		{name: "commit-now", prev: func(dir string) state.Index { idx, _, _ := state.ReadIndex(dir); return idx }},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			commitNowFilesBothWaitingPanes(t, dir)
			w := carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", false), yRow("foo", true), otherRow()}, envErrs: noSuchFoo()}

			loads := 0
			_, err := state.RunCommitCycle(state.CommitCycle{
				OwnServer: ownServerPID,
				Client:    w.client(t),
				Dir:       dir,
				LoadPrev:  func() *state.Index { loads++; idx := c.prev(dir); return &idx },
				HashMap:   state.HashMap{},
			})
			if err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}
			if loads != 0 {
				t.Errorf("LoadPrev called %d times over a readable sessions.json, want 0", loads)
			}
		})
	}
}

// commitLockHeldElsewhere reports whether a second open file description is
// refused the commit lock, as another committer's would be.
func commitLockHeldElsewhere(t *testing.T, dir string) bool {
	t.Helper()
	f, err := os.OpenFile(state.CommitLock(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open commit lock: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.Is(err, unix.EWOULDBLOCK)
	}
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return false
}

func TestRunCommitCycleFallsBackToLoadPrevWhenSessionsJSONCannotBeRead(t *testing.T) {
	unreadable := map[string]func(t *testing.T, dir string){
		"absent": func(*testing.T, string) {},
		"not decodable": func(t *testing.T, dir string) {
			if err := os.WriteFile(state.SessionsJSON(dir), []byte("{not valid json"), 0o600); err != nil {
				t.Fatalf("seed corrupt sessions.json: %v", err)
			}
		},
	}
	xKey := state.SanitizePaneKey("foo", 0, 0)
	// cwd and command are what X's committed record carries: LoadPrev's for a
	// merged or carried record, the live pane's for a fresh one.
	worlds := map[string]struct {
		world        carryWorld
		cwd, command string
	}{
		"the carry":              {carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", true), siblingRow("foo"), otherRow()}, envErrs: map[string]error{"foo": errors.New("boom")}}, "/x", "claude"},
		"the waiting-pane merge": {carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", true), siblingRow("foo"), otherRow()}}, "/x", "claude"},
		"the skeleton merge":     {carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", false), siblingRow("foo"), otherRow()}, markers: state.SkeletonMarkerPrefix + xKey + ` "1"`}, "/x", "claude"},
		"the answered-pane hold": {carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", false), siblingRow("foo"), otherRow()}}, "/live", "zsh"},
	}
	for shape, stage := range unreadable {
		for site, row := range worlds {
			w := row.world
			t.Run(site+" with sessions.json "+shape, func(t *testing.T) {
				dir := t.TempDir()
				prev := carryPrev()
				seedScrollback(t, dir, carryTokenFile(), carryTranscript)
				seedScrollback(t, dir, "foo__0.1.bin", "y-body")
				seedScrollback(t, dir, "other__0.0.bin", "o-body")
				stage(t, dir)

				loads := 0
				_, err := state.RunCommitCycle(state.CommitCycle{
					OwnServer: ownServerPID,
					Client:    w.client(t),
					Dir:       dir,
					LoadPrev: func() *state.Index {
						loads++
						if !commitLockHeldElsewhere(t, dir) {
							t.Error("LoadPrev called without the commit lock held")
						}
						return &prev
					},
					HashMap: state.HashMap{},
					Dump:    func(state.CaptureCycle, state.ScrollbackWriter) (bool, error) { return false, nil },
				})
				if err != nil {
					t.Fatalf("RunCommitCycle: %v", err)
				}
				if loads != 1 {
					t.Errorf("LoadPrev called %d times, want 1", loads)
				}
				assertXFiledAt(t, dir, "foo")
				if p := findPane(onDiskIndex(t, dir), "foo", 0, 0); p == nil || p.CWD != row.cwd || p.CurrentCommand != row.command {
					t.Errorf("X's committed record = %+v, want CWD %q and command %q", p, row.cwd, row.command)
				}
			})
		}
	}
}
