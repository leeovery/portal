package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/state"
)

const answeredCapture = "x-after-answer"

// answeredClient is the live state once X, filed under its token while it
// waited, has been answered: no skeleton marker, no resume pending marker. It
// answers the cycle's own confirmation as ownServerPID and every confirmation
// after it with later.
type answeredClient struct {
	later      func() (int, error)
	cycleReads int
	laterReads int
}

func (c *answeredClient) ShowAllServerOptions() (string, error)    { return "", nil }
func (c *answeredClient) ListSessionNamesProbe() ([]string, error) { return []string{"work"}, nil }
func (c *answeredClient) ShowEnvironment(string) (string, error)   { return "", nil }

func (c *answeredClient) ListAllPanesWithFormat(string) (string, error) {
	return paneLineWithPending("work", 0, "main", "tiled", false, true, 1, "/tmp", true, "zsh", waitingPaneToken, ""), nil
}

func (c *answeredClient) ConfirmAnswering() (int, error) {
	if c.cycleReads == 0 {
		c.cycleReads++
		return ownServerPID, nil
	}
	c.laterReads++
	if c.later == nil {
		return ownServerPID, nil
	}
	return c.later()
}

// answeredSeed is the index committed while X waited: X's record names its
// token-named file, which holds its transcript, and the housekeeping pass of
// that commit has removed the positional name.
func answeredSeed(t *testing.T, dir string) state.Index {
	t.Helper()
	idx := waitingIndex(waitingPaneToken, handOverTokenPath())
	seedScrollback(t, dir, handOverTokenFile(), handOverTranscript)
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	idx.Canonicalize()
	return idx
}

type dumpFunc func(state.CaptureCycle, state.ScrollbackWriter) (bool, error)

func answeredTick(client *answeredClient, dir string, prev state.Index, hm state.HashMap, dump dumpFunc) state.CommitCycle {
	return state.CommitCycle{
		OwnServer: ownServerPID,
		Client:    client,
		Dir:       dir,
		LoadPrev:  func() *state.Index { return &prev },
		HashMap:   hm,
		Dump:      dump,
	}
}

func answeredCommitNow(t *testing.T, client *answeredClient, dir string) state.CommitCycle {
	return state.CommitCycle{
		OwnServer: ownServerPID,
		Client:    client,
		Dir:       dir,
		LoadPrev: func() *state.Index {
			idx, _, err := state.ReadIndex(dir)
			if err != nil {
				t.Errorf("ReadIndex: %v", err)
			}
			return &idx
		},
	}
}

// writesX dumps X's capture through the cycle's writer, recording what the
// writer answered.
func writesX(data []byte, written *bool, writeErr *error) dumpFunc {
	return func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
		*written, *writeErr = w.Write(handOverKey, data, xxhash.Sum64(data))
		return *written, nil
	}
}

func dumpsNothing(state.CaptureCycle, state.ScrollbackWriter) (bool, error) { return false, nil }

func assertXKeptUnderToken(t *testing.T, dir string) {
	t.Helper()
	assertTranscriptFiledUnderToken(t, dir)
	assertSavedScrollbackPresent(t, dir)
	assertRestoreFindsXTranscript(t, dir)
}

func assertXFiledAtPositional(t *testing.T, dir, want string) {
	t.Helper()
	if got := recordFor(t, onDiskIndex(t, dir), "work").ScrollbackFile; got != "scrollback/"+handOverPositionalFile() {
		t.Errorf("sessions.json names %q for X, want its positional file", got)
	}
	if got := readScrollback(t, dir, handOverPositionalFile()); got != want {
		t.Errorf("positional file = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), handOverTokenFile())); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("token-named file stat err = %v, want not-exist once a commit naming the positional capture ran its housekeeping", err)
	}
	assertSavedScrollbackPresent(t, dir)
}

func TestRunCommitCycleKeepsAnAnsweredPanesTranscriptOverAnUnconfirmedEmptyCapture(t *testing.T) {
	refusals := []struct {
		name  string
		later func() (int, error)
	}{
		{"refused", func() (int, error) { return 0, errors.New("server exited") }},
		{"answered by another server on the socket", func() (int, error) { return ownServerPID + 1, nil }},
		{"answered naming no server", func() (int, error) { return 0, nil }},
	}
	for _, refusal := range refusals {
		t.Run("the read after the empty capture "+refusal.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := answeredSeed(t, dir)
			client := &answeredClient{later: refusal.later}
			var written bool
			var writeErr error

			if _, err := state.RunCommitCycle(answeredTick(client, dir, seed, state.HashMap{}, writesX(nil, &written, &writeErr))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if written || !errors.Is(writeErr, state.ErrUnconfirmedEmptyCapture) {
				t.Errorf("Write = %t, %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", written, writeErr)
			}
			if client.laterReads != 1 {
				t.Errorf("confirmation reads after the capture = %d, want 1", client.laterReads)
			}
			if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), handOverPositionalFile())); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("positional file stat err = %v, want nothing written", err)
			}
			assertXKeptUnderToken(t, dir)
		})
	}
}

func TestRunCommitCycleKeepsNamingAnAnsweredPanesTranscriptUntilItsCaptureIsWritten(t *testing.T) {
	failures := []struct {
		name string
		dump func(t *testing.T, dir string) dumpFunc
	}{
		{"its capture-pane is refused", func(*testing.T, string) dumpFunc { return dumpsNothing }},
		{"its write fails", func(t *testing.T, dir string) dumpFunc {
			return func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
				sb := state.ScrollbackDir(dir)
				if err := os.Chmod(sb, 0o500); err != nil {
					t.Fatalf("chmod scrollback dir: %v", err)
				}
				data := []byte(answeredCapture)
				written, err := w.Write(handOverKey, data, xxhash.Sum64(data))
				if chmodErr := os.Chmod(sb, 0o700); chmodErr != nil {
					t.Fatalf("restore scrollback dir: %v", chmodErr)
				}
				if err == nil || written {
					t.Errorf("Write = %t, %v; want the failed write", written, err)
				}
				return false, nil
			}
		}},
	}
	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := answeredSeed(t, dir)
			hm := state.HashMap{}

			capture, err := state.RunCommitCycle(answeredTick(&answeredClient{}, dir, seed, hm, failure.dump(t, dir)))
			if err != nil {
				t.Fatalf("first dumping cycle: %v", err)
			}
			assertXKeptUnderToken(t, dir)

			capture, err = state.RunCommitCycle(answeredTick(&answeredClient{}, dir, capture.Index, hm, dumpsNothing))
			if err != nil {
				t.Fatalf("later tick: %v", err)
			}
			assertXKeptUnderToken(t, dir)

			if _, err := state.RunCommitCycle(answeredCommitNow(t, &answeredClient{}, dir)); err != nil {
				t.Fatalf("later commit-now: %v", err)
			}
			assertXKeptUnderToken(t, dir)

			if _, err := state.RunCommitCycle(answeredTick(&answeredClient{}, dir, capture.Index, nil, nil)); err != nil {
				t.Fatalf("later shutdown flush with nothing dumped: %v", err)
			}
			assertXKeptUnderToken(t, dir)
		})
	}
}

func TestRunCommitCycleCommitNowAfterAnAnswerKeepsTheTranscriptForTheNextRestore(t *testing.T) {
	dir := t.TempDir()
	answeredSeed(t, dir)

	if _, err := state.RunCommitCycle(answeredCommitNow(t, &answeredClient{}, dir)); err != nil {
		t.Fatalf("commit-now: %v", err)
	}

	assertXKeptUnderToken(t, dir)
}

func TestRunCommitCycleFilesAnAnsweredPaneAtItsPositionalFileOnceItsCaptureIsWritten(t *testing.T) {
	cases := []struct {
		name       string
		data       string
		laterReads int
	}{
		{"a non-empty capture", answeredCapture, 0},
		{"an empty capture its own server confirms against the token-named transcript", "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := answeredSeed(t, dir)
			client := &answeredClient{}
			var written bool
			var writeErr error

			capture, err := state.RunCommitCycle(answeredTick(client, dir, seed, state.HashMap{}, writesX([]byte(tc.data), &written, &writeErr)))
			if err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if !written || writeErr != nil {
				t.Errorf("Write = %t, %v; want a write", written, writeErr)
			}
			if client.laterReads != tc.laterReads {
				t.Errorf("confirmation reads after the capture = %d, want %d", client.laterReads, tc.laterReads)
			}
			if got := recordFor(t, capture.Index, "work").ScrollbackFile; got != "scrollback/"+handOverPositionalFile() {
				t.Errorf("returned index names %q for X, want its positional file", got)
			}
			assertXFiledAtPositional(t, dir, tc.data)
		})
	}
}

func TestRunCommitCycleAfterAnAnswerNeverNamesAMissingPositionalFile(t *testing.T) {
	ends := []struct {
		name string
		end  func(t *testing.T, dir string) (bool, error)
	}{
		{"stood down", func(*testing.T, string) (bool, error) {
			return false, state.ErrTmuxStoppedAnswering
		}},
		{"cancelled mid-dump", func(*testing.T, string) (bool, error) {
			return false, errEndBeforeCommit
		}},
		{"with a failed sessions.json write", func(t *testing.T, dir string) (bool, error) {
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatalf("chmod state dir: %v", err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			return true, nil
		}},
	}
	for _, end := range ends {
		t.Run("a cycle that wrote X's capture and ended "+end.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := answeredSeed(t, dir)
			hm := state.HashMap{}
			data := []byte(answeredCapture)

			_, err := state.RunCommitCycle(answeredTick(&answeredClient{}, dir, seed, hm, func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
				if written, err := w.Write(handOverKey, data, xxhash.Sum64(data)); err != nil || !written {
					t.Fatalf("Write = %t, %v; want a write", written, err)
				}
				return end.end(t, dir)
			}))
			if err == nil {
				t.Fatal("cycle returned nil, want it to end uncommitted")
			}
			if err := os.Chmod(dir, 0o700); err != nil {
				t.Fatalf("restore state dir: %v", err)
			}
			assertXKeptUnderToken(t, dir)

			if _, err := state.RunCommitCycle(answeredCommitNow(t, &answeredClient{}, dir)); err != nil {
				t.Fatalf("commit-now: %v", err)
			}
			assertXKeptUnderToken(t, dir)

			if _, err := state.RunCommitCycle(answeredTick(&answeredClient{}, dir, onDiskIndex(t, dir), hm, func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
				return w.Write(handOverKey, data, xxhash.Sum64(data))
			})); err != nil {
				t.Fatalf("next tick: %v", err)
			}
			assertXFiledAtPositional(t, dir, answeredCapture)
		})
	}
}

func TestRunCommitCycleJudgesAnOrdinaryPanesEmptyCaptureAtItsPositionalFile(t *testing.T) {
	t.Run("a saved positional transcript needs the confirmation", func(t *testing.T) {
		dir := t.TempDir()
		seed := waitingIndex(waitingPaneToken, "scrollback/"+handOverPositionalFile())
		seedScrollback(t, dir, handOverPositionalFile(), handOverTranscript)
		client := &answeredClient{later: func() (int, error) { return ownServerPID + 1, nil }}
		var written bool
		var writeErr error

		if _, err := state.RunCommitCycle(answeredTick(client, dir, seed, state.HashMap{}, writesX(nil, &written, &writeErr))); err != nil {
			t.Fatalf("RunCommitCycle: %v", err)
		}

		if written || !errors.Is(writeErr, state.ErrUnconfirmedEmptyCapture) {
			t.Errorf("Write = %t, %v; want a refusal", written, writeErr)
		}
		if got := readScrollback(t, dir, handOverPositionalFile()); got != handOverTranscript {
			t.Errorf("positional file = %q, want %q", got, handOverTranscript)
		}
	})

	t.Run("a token-named file its record does not name is not its saved transcript while the named file is on disk", func(t *testing.T) {
		dir := t.TempDir()
		seed := waitingIndex(waitingPaneToken, "scrollback/"+handOverPositionalFile())
		seedScrollback(t, dir, handOverPositionalFile(), "")
		seedScrollback(t, dir, handOverTokenFile(), handOverTranscript)
		client := &answeredClient{later: func() (int, error) { return ownServerPID + 1, nil }}
		var written bool
		var writeErr error

		if _, err := state.RunCommitCycle(answeredTick(client, dir, seed, state.HashMap{}, writesX(nil, &written, &writeErr))); err != nil {
			t.Fatalf("RunCommitCycle: %v", err)
		}

		if !written || writeErr != nil {
			t.Errorf("Write = %t, %v; want a write", written, writeErr)
		}
		if client.laterReads != 0 {
			t.Errorf("confirmation reads after the capture = %d, want none", client.laterReads)
		}
		assertXFiledAtPositional(t, dir, "")
	})
}

// staleDaemonPrev is the daemon's in-memory index from before a commit-now
// filed X under its token: it still names X's positional file.
func staleDaemonPrev() state.Index {
	idx := waitingIndex(waitingPaneToken, "scrollback/"+handOverPositionalFile())
	idx.Canonicalize()
	return idx
}

func TestRunCommitCycleHoldsAnAnsweredPanesTranscriptWhenThePreviousIndexPredatesItsTokenFiling(t *testing.T) {
	cases := []struct {
		name   string
		client *answeredClient
		dump   func(written *bool, writeErr *error) dumpFunc
	}{
		{"an unconfirmed empty capture", &answeredClient{later: func() (int, error) { return ownServerPID + 1, nil }}, func(written *bool, writeErr *error) dumpFunc {
			return writesX(nil, written, writeErr)
		}},
		{"a refused capture", &answeredClient{}, func(*bool, *error) dumpFunc { return dumpsNothing }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			answeredSeed(t, dir)
			var written bool
			var writeErr error

			if _, err := state.RunCommitCycle(answeredTick(tc.client, dir, staleDaemonPrev(), state.HashMap{}, tc.dump(&written, &writeErr))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if written {
				t.Error("Write reported a write over the answered pane's transcript")
			}
			if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), handOverPositionalFile())); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("positional file stat err = %v, want nothing written", err)
			}
			assertXKeptUnderToken(t, dir)
		})
	}
}
