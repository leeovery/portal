package state_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/leeovery/portal/internal/state"
)

// handOverKey is pane X's address: saved there while not waiting, and restored
// there.
var handOverKey = state.SanitizePaneKey("work", 0, 1)

const (
	handOverTranscript = "x-transcript"
	closedSession      = "closed"
	heldWindow         = 150 * time.Millisecond
)

func handOverPositionalFile() string { return handOverKey + ".bin" }

func handOverTokenFile() string { return "pane-" + waitingPaneToken + ".bin" }

func handOverTokenPath() string { return "scrollback/" + handOverTokenFile() }

// handOverWorld is the live tmux state every committer in a scenario reads: pane
// X is skeleton-marked until the helper hands it over, then pending alone.
type handOverWorld struct {
	mu       sync.Mutex
	skeleton bool
	pending  bool
	sessions []string
}

func newHandOverWorld() *handOverWorld {
	return &handOverWorld{skeleton: true, sessions: []string{"work"}}
}

func (w *handOverWorld) markPending() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.skeleton = false
	w.pending = true
}

func (w *handOverWorld) snapshot() (skeleton, pending bool, sessions []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.skeleton, w.pending, append([]string(nil), w.sessions...)
}

// worldClient is one committer's view of the world. A non-nil afterStructure
// blocks the capture once it has read the pane rows, until it is closed.
type worldClient struct {
	world          *handOverWorld
	calls          atomic.Int32
	afterStructure chan struct{}
}

func (c *worldClient) ShowAllServerOptions() (string, error) {
	c.calls.Add(1)
	skeleton, _, _ := c.world.snapshot()
	if !skeleton {
		return "", nil
	}
	return state.SkeletonMarkerPrefix + handOverKey + ` "1"`, nil
}

func (c *worldClient) ListSessionNames() ([]string, error) {
	c.calls.Add(1)
	_, _, sessions := c.world.snapshot()
	return sessions, nil
}

func (c *worldClient) ListAllPanesWithFormat(string) (string, error) {
	c.calls.Add(1)
	_, pending, _ := c.world.snapshot()
	flag := ""
	if pending {
		flag = "1"
	}
	return paneLineWithPending("work", 0, "main", "tiled", false, true, 1, "/tmp", true, "zsh", waitingPaneToken, flag), nil
}

func (c *worldClient) ShowEnvironment(string) (string, error) {
	c.calls.Add(1)
	if c.afterStructure != nil {
		<-c.afterStructure
	}
	return "", nil
}

// handOverSeed is the index X was saved in, beside a session the user is about
// to close, with both transcripts on disk.
func handOverSeed(t *testing.T, dir string) state.Index {
	t.Helper()
	idx := waitingIndex(waitingPaneToken, "scrollback/"+handOverPositionalFile())
	idx.Sessions = append(idx.Sessions, state.Session{
		Name:        closedSession,
		Environment: map[string]string{},
		Windows: []state.Window{{
			Index: 0, Name: "main", Layout: "tiled", Active: true,
			Panes: []state.Pane{{Index: 0, CWD: "/tmp", CurrentCommand: "zsh", ScrollbackFile: "scrollback/closed__0.0.bin"}},
		}},
	})
	seedScrollback(t, dir, handOverPositionalFile(), handOverTranscript)
	seedScrollback(t, dir, "closed__0.0.bin", "closed-body")
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	idx.Canonicalize()
	return idx
}

func onDiskIndex(t *testing.T, dir string) state.Index {
	t.Helper()
	idx, skip, err := state.ReadIndex(dir)
	if err != nil || skip {
		t.Fatalf("ReadIndex = (skip %v, err %v), want the committed index", skip, err)
	}
	return idx
}

func recordFor(t *testing.T, idx state.Index, session string) state.Pane {
	t.Helper()
	for _, s := range idx.Sessions {
		if s.Name == session {
			return s.Windows[0].Panes[0]
		}
	}
	t.Fatalf("index holds no session %q: %+v", session, idx.Sessions)
	return state.Pane{}
}

func sessionNamesOf(idx state.Index) []string {
	names := make([]string, 0, len(idx.Sessions))
	for _, s := range idx.Sessions {
		names = append(names, s.Name)
	}
	return names
}

func assertTranscriptFiledUnderToken(t *testing.T, dir string) {
	t.Helper()
	if got := readScrollback(t, dir, handOverTokenFile()); got != handOverTranscript {
		t.Errorf("token-named file = %q, want %q", got, handOverTranscript)
	}
	if got := recordFor(t, onDiskIndex(t, dir), "work").ScrollbackFile; got != handOverTokenPath() {
		t.Errorf("sessions.json names %q for X, want %q", got, handOverTokenPath())
	}
}

type cycleResult struct {
	capture state.CaptureCycle
	err     error
}

func runCycleAsync(cycle state.CommitCycle) <-chan cycleResult {
	done := make(chan cycleResult, 1)
	go func() {
		capture, err := state.RunCommitCycle(cycle)
		done <- cycleResult{capture: capture, err: err}
	}()
	return done
}

func awaitCycle(t *testing.T, done <-chan cycleResult, who string) state.CaptureCycle {
	t.Helper()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("%s cycle: %v", who, r.err)
		}
		return r.capture
	case <-time.After(10 * time.Second):
		t.Fatalf("%s cycle did not finish", who)
		return state.CaptureCycle{}
	}
}

func awaitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// commitNowCycle is the cycle `portal state commit-now` runs: the previous index
// read from disk, and no dump. prevs records each index it read.
func commitNowCycle(t *testing.T, client *worldClient, dir string, prevs *[]state.Index) state.CommitCycle {
	return state.CommitCycle{
		Client: client,
		Dir:    dir,
		LoadPrev: func() *state.Index {
			idx, _, err := state.ReadIndex(dir)
			if err != nil {
				t.Errorf("ReadIndex: %v", err)
			}
			*prevs = append(*prevs, idx)
			return &idx
		},
	}
}

// heldTick is a daemon tick whose dump signals it has started and then waits to
// be released.
func heldTick(client *worldClient, dir string, prev state.Index, started, release chan struct{}) state.CommitCycle {
	return state.CommitCycle{
		Client:   client,
		Dir:      dir,
		LoadPrev: func() *state.Index { return &prev },
		HashMap:  state.HashMap{},
		Dump: func(state.CaptureCycle) (bool, error) {
			close(started)
			<-release
			return false, nil
		},
	}
}

func TestRunCommitCycleSerialisesOverlappingCommitters(t *testing.T) {
	t.Run("a commit-now started during a tick's dump captures only after the tick's housekeeping, and files X's transcript under its token", func(t *testing.T) {
		dir := t.TempDir()
		seed := handOverSeed(t, dir)
		world := newHandOverWorld()

		tickClient := &worldClient{world: world}
		dumpStarted, releaseDump := make(chan struct{}), make(chan struct{})
		tickDone := runCycleAsync(heldTick(tickClient, dir, seed, dumpStarted, releaseDump))
		awaitSignal(t, dumpStarted, "the tick's dump")

		world.markPending()
		nowClient := &worldClient{world: world}
		var prevs []state.Index
		nowDone := runCycleAsync(commitNowCycle(t, nowClient, dir, &prevs))

		time.Sleep(heldWindow)
		if n := nowClient.calls.Load(); n != 0 {
			t.Fatalf("commit-now made %d tmux reads while the tick held the cycle, want 0", n)
		}

		close(releaseDump)
		tickCapture := awaitCycle(t, tickDone, "tick")
		if got := recordFor(t, tickCapture.Index, "work").ScrollbackFile; got != "scrollback/"+handOverPositionalFile() {
			t.Fatalf("tick's index names %q for X, want the positional file", got)
		}
		awaitCycle(t, nowDone, "commit-now")

		assertTranscriptFiledUnderToken(t, dir)

		t.Run("the daemon's next tick, holding the older in-memory index, keeps the file on disk and named for X", func(t *testing.T) {
			nextTick := state.CommitCycle{
				Client:   &worldClient{world: world},
				Dir:      dir,
				LoadPrev: func() *state.Index { return &tickCapture.Index },
				HashMap:  state.HashMap{},
				Dump:     func(state.CaptureCycle) (bool, error) { return false, nil },
			}
			capture, err := state.RunCommitCycle(nextTick)
			if err != nil {
				t.Fatalf("next tick: %v", err)
			}
			if got := recordFor(t, capture.Index, "work").ScrollbackFile; got != handOverTokenPath() {
				t.Errorf("next tick's record for X names %q, want %q", got, handOverTokenPath())
			}
			assertTranscriptFiledUnderToken(t, dir)
		})
	})

	t.Run("a commit-now started while a tick holds X's re-file captures after the tick's housekeeping, sees X pending and leaves the token file named", func(t *testing.T) {
		dir := t.TempDir()
		seed := handOverSeed(t, dir)
		world := newHandOverWorld()
		world.markPending()

		tickClient := &worldClient{world: world}
		dumpStarted, releaseDump := make(chan struct{}), make(chan struct{})
		tickDone := runCycleAsync(heldTick(tickClient, dir, seed, dumpStarted, releaseDump))
		awaitSignal(t, dumpStarted, "the tick's dump")

		nowClient := &worldClient{world: world}
		var prevs []state.Index
		nowDone := runCycleAsync(commitNowCycle(t, nowClient, dir, &prevs))

		time.Sleep(heldWindow)
		if n := nowClient.calls.Load(); n != 0 {
			t.Fatalf("commit-now made %d tmux reads while the tick held the cycle, want 0", n)
		}

		close(releaseDump)
		awaitCycle(t, tickDone, "tick")
		nowCapture := awaitCycle(t, nowDone, "commit-now")

		if _, waiting := nowCapture.Pending[handOverKey]; !waiting {
			t.Errorf("commit-now's pending set = %v, want X in it", nowCapture.Pending)
		}
		assertTranscriptFiledUnderToken(t, dir)
	})

	t.Run("two commit-nows started back to back across X's pending mark run one after the other, the second reading the first's commit", func(t *testing.T) {
		dir := t.TempDir()
		handOverSeed(t, dir)
		world := newHandOverWorld()

		firstClient := &worldClient{world: world, afterStructure: make(chan struct{})}
		var firstPrevs, secondPrevs []state.Index
		firstDone := runCycleAsync(commitNowCycle(t, firstClient, dir, &firstPrevs))
		waitForCalls(t, firstClient, 3)

		world.markPending()
		secondClient := &worldClient{world: world}
		secondLoads := 0
		secondCycle := commitNowCycle(t, secondClient, dir, &secondPrevs)
		loadPrev := secondCycle.LoadPrev
		secondCycle.LoadPrev = func() *state.Index { secondLoads++; return loadPrev() }
		secondDone := runCycleAsync(secondCycle)

		time.Sleep(heldWindow)
		if n := secondClient.calls.Load(); n != 0 {
			t.Fatalf("the second commit-now made %d tmux reads while the first held the cycle, want 0", n)
		}

		close(firstClient.afterStructure)
		firstCapture := awaitCycle(t, firstDone, "first commit-now")
		awaitCycle(t, secondDone, "second commit-now")

		if secondLoads != 1 || len(secondPrevs) != 1 {
			t.Fatalf("the second commit-now read its previous index %d times, want 1", secondLoads)
		}
		if got, want := sessionNamesOf(secondPrevs[0]), sessionNamesOf(firstCapture.Index); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("second commit-now's previous index holds sessions %v, want the first's commit %v", got, want)
		}
		if got := recordFor(t, secondPrevs[0], "work").ScrollbackFile; got != "scrollback/"+handOverPositionalFile() {
			t.Errorf("second commit-now's previous index names %q for X, want the positional file the first committed", got)
		}
		assertTranscriptFiledUnderToken(t, dir)
	})
}

func waitForCalls(t *testing.T, c *worldClient, n int32) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for c.calls.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("client reached %d reads, want %d", c.calls.Load(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

// holdCommitLock takes the commit lock through an open file description of its
// own, as another process would, for the rest of the test.
func holdCommitLock(t *testing.T, dir string) {
	t.Helper()
	f, err := os.OpenFile(state.CommitLock(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open commit lock: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("flock commit lock: %v", err)
	}
}

func scrollbackNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(state.ScrollbackDir(dir))
	if err != nil {
		t.Fatalf("read scrollback dir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestRunCommitCycleLockBound(t *testing.T) {
	t.Run("it gives up after the bound with nothing captured, moved, written or deleted", func(t *testing.T) {
		state.SetCommitLockTimeoutForTest(t, 40*time.Millisecond)
		dir := t.TempDir()
		seed := handOverSeed(t, dir)
		seedScrollback(t, dir, "orphan.bin", "unreferenced")
		world := newHandOverWorld()
		world.markPending()
		holdCommitLock(t, dir)
		before, err := os.ReadFile(state.SessionsJSON(dir))
		if err != nil {
			t.Fatalf("read sessions.json: %v", err)
		}
		namesBefore := scrollbackNames(t, dir)

		client := &worldClient{world: world}
		loads, dumps := 0, 0
		_, err = state.RunCommitCycle(state.CommitCycle{
			Client:   client,
			Dir:      dir,
			LoadPrev: func() *state.Index { loads++; return &seed },
			HashMap:  state.HashMap{},
			Dump:     func(state.CaptureCycle) (bool, error) { dumps++; return true, nil },
		})

		if !errors.Is(err, state.ErrCommitLockHeld) {
			t.Fatalf("error = %v, want ErrCommitLockHeld", err)
		}
		if n := client.calls.Load(); n != 0 {
			t.Errorf("tmux reads = %d, want 0", n)
		}
		if loads != 0 || dumps != 0 {
			t.Errorf("previous-index loads = %d, dumps = %d; want neither", loads, dumps)
		}
		after, err := os.ReadFile(state.SessionsJSON(dir))
		if err != nil {
			t.Fatalf("read sessions.json: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("sessions.json changed under a held lock:\nbefore %s\nafter  %s", before, after)
		}
		if got := scrollbackNames(t, dir); strings.Join(got, ",") != strings.Join(namesBefore, ",") {
			t.Errorf("scrollback files = %v, want %v untouched", got, namesBefore)
		}
	})

	t.Run("it waits out a holder that releases inside the bound", func(t *testing.T) {
		dir := t.TempDir()
		seed := handOverSeed(t, dir)
		f, err := os.OpenFile(state.CommitLock(dir), os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("open commit lock: %v", err)
		}
		if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
			t.Fatalf("flock: %v", err)
		}
		time.AfterFunc(50*time.Millisecond, func() { _ = f.Close() })

		_, err = state.RunCommitCycle(state.CommitCycle{
			Client:   &worldClient{world: newHandOverWorld()},
			Dir:      dir,
			LoadPrev: func() *state.Index { return &seed },
		})
		if err != nil {
			t.Fatalf("RunCommitCycle: %v", err)
		}
	})
}

const holdCommitLockEnv = "PORTAL_TEST_HOLD_COMMIT_LOCK"

// TestHelperProcessHoldsCommitLock is not a test: re-executed with
// holdCommitLockEnv set, it takes the commit lock, reports it held and then
// waits to be killed.
func TestHelperProcessHoldsCommitLock(t *testing.T) {
	path := os.Getenv(holdCommitLockEnv)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(2)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		fmt.Println("flock:", err)
		os.Exit(2)
	}
	fmt.Println("held")
	time.Sleep(time.Minute)
	os.Exit(3)
}

func TestRunCommitCycleAfterAKilledHolder(t *testing.T) {
	dir := t.TempDir()
	seed := handOverSeed(t, dir)

	holder := exec.Command(os.Args[0], "-test.run=^TestHelperProcessHoldsCommitLock$")
	holder.Env = append(os.Environ(), holdCommitLockEnv+"="+state.CommitLock(dir))
	out, err := holder.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := holder.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	t.Cleanup(func() {
		_ = holder.Process.Kill()
		_ = holder.Wait()
	})
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "held" {
		t.Fatalf("holder reported %q (err %v), want held", line, err)
	}

	if err := holder.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("kill holder: %v", err)
	}
	_ = holder.Wait()

	world := newHandOverWorld()
	world.markPending()
	start := time.Now()
	_, err = state.RunCommitCycle(state.CommitCycle{
		Client:   &worldClient{world: world},
		Dir:      dir,
		LoadPrev: func() *state.Index { return &seed },
	})
	if err != nil {
		t.Fatalf("RunCommitCycle after the holder died: %v", err)
	}
	if took := time.Since(start); took >= time.Second {
		t.Errorf("acquire after a killed holder took %v, want it granted without waiting out the bound", took)
	}
	assertTranscriptFiledUnderToken(t, dir)
}

func TestRunCommitCycleWithNoOtherCommitter(t *testing.T) {
	t.Run("it commits the capture and reports the dump's change so an unchanged structure still writes", func(t *testing.T) {
		dir := t.TempDir()
		seed := handOverSeed(t, dir)
		world := newHandOverWorld()
		world.sessions = []string{"work", closedSession}
		before, err := os.ReadFile(state.SessionsJSON(dir))
		if err != nil {
			t.Fatalf("read sessions.json: %v", err)
		}

		_, err = state.RunCommitCycle(state.CommitCycle{
			Client:   &worldClient{world: world},
			Dir:      dir,
			LoadPrev: func() *state.Index { return &seed },
			HashMap:  state.HashMap{},
			Dump:     func(state.CaptureCycle) (bool, error) { return true, nil },
		})
		if err != nil {
			t.Fatalf("RunCommitCycle: %v", err)
		}
		after, err := os.ReadFile(state.SessionsJSON(dir))
		if err != nil {
			t.Fatalf("read sessions.json: %v", err)
		}
		if bytes.Equal(before, after) {
			t.Error("sessions.json unchanged; a dump reporting a change must force the write")
		}
	})

	t.Run("it commits without a dump, dropping a closed session and its transcript", func(t *testing.T) {
		dir := t.TempDir()
		seed := handOverSeed(t, dir)

		capture, err := state.RunCommitCycle(state.CommitCycle{
			Client:   &worldClient{world: newHandOverWorld()},
			Dir:      dir,
			LoadPrev: func() *state.Index { return &seed },
		})
		if err != nil {
			t.Fatalf("RunCommitCycle: %v", err)
		}
		if got := sessionNamesOf(onDiskIndex(t, dir)); strings.Join(got, ",") != "work" {
			t.Errorf("committed sessions = %v, want [work]", got)
		}
		if got := sessionNamesOf(capture.Index); strings.Join(got, ",") != "work" {
			t.Errorf("returned capture sessions = %v, want [work]", got)
		}
		if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), "closed__0.0.bin")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("closed session's transcript stat err = %v, want not-exist", err)
		}
	})

	t.Run("a dump error ends the cycle before the commit", func(t *testing.T) {
		dir := t.TempDir()
		seed := handOverSeed(t, dir)
		before, _ := os.ReadFile(state.SessionsJSON(dir))
		dumpErr := errors.New("cancelled")

		_, err := state.RunCommitCycle(state.CommitCycle{
			Client:   &worldClient{world: newHandOverWorld()},
			Dir:      dir,
			LoadPrev: func() *state.Index { return &seed },
			Dump:     func(state.CaptureCycle) (bool, error) { return true, dumpErr },
		})
		if !errors.Is(err, dumpErr) {
			t.Fatalf("error = %v, want the dump's", err)
		}
		after, _ := os.ReadFile(state.SessionsJSON(dir))
		if !bytes.Equal(before, after) {
			t.Error("sessions.json written after a failed dump")
		}
	})

	t.Run("a failed capture runs no dump and commits nothing", func(t *testing.T) {
		dir := t.TempDir()
		seed := handOverSeed(t, dir)
		before, _ := os.ReadFile(state.SessionsJSON(dir))
		captureErr := errors.New("tmux gone")
		dumps := 0

		_, err := state.RunCommitCycle(state.CommitCycle{
			Client:   &failFastCaptureClient{t: t, listSessionNamesErr: captureErr},
			Dir:      dir,
			LoadPrev: func() *state.Index { return &seed },
			Dump:     func(state.CaptureCycle) (bool, error) { dumps++; return true, nil },
		})
		if !errors.Is(err, captureErr) {
			t.Fatalf("error = %v, want the capture's", err)
		}
		if dumps != 0 {
			t.Errorf("dumps = %d, want 0", dumps)
		}
		after, _ := os.ReadFile(state.SessionsJSON(dir))
		if !bytes.Equal(before, after) {
			t.Error("sessions.json written after a failed capture")
		}
	})
}

// lockProbeHandler probes the commit lock at the moment a named message is
// emitted, which a capturing sink recording for later inspection cannot do.
type lockProbeHandler struct {
	lockPath string
	message  string
	seen     bool
	held     bool
}

func (h *lockProbeHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *lockProbeHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *lockProbeHandler) WithGroup(string) slog.Handler            { return h }

func (h *lockProbeHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message != h.message {
		return nil
	}
	h.seen = true
	f, err := os.OpenFile(h.lockPath, os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h.held = errors.Is(unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB), unix.EWOULDBLOCK)
	return nil
}

func TestRunCommitCycleHoldsTheLockThroughTheHousekeepingPass(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(state.ScrollbackDir(dir), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("stage scrollback as a file: %v", err)
	}
	prev := waitingIndex(waitingPaneToken, "scrollback/"+handOverPositionalFile())
	probe := &lockProbeHandler{lockPath: state.CommitLock(dir), message: "gc orphan scrollback failed"}

	_, err := state.RunCommitCycle(state.CommitCycle{
		Client:   &worldClient{world: newHandOverWorld()},
		Dir:      dir,
		LoadPrev: func() *state.Index { return &prev },
		HashMap:  state.HashMap{},
		Dump:     func(state.CaptureCycle) (bool, error) { return true, nil },
		Logger:   slog.New(probe),
	})
	if err != nil {
		t.Fatalf("RunCommitCycle: %v", err)
	}

	if !probe.seen {
		t.Fatal("the housekeeping pass's failure was never logged; the probe did not run")
	}
	if !probe.held {
		t.Error("the commit lock was free during the housekeeping pass, want it held to the pass's end")
	}
}
