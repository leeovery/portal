package state_test

import (
	"errors"
	"os"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/state"
)

const emptyCapturePaneKey = "work__0_0"

// emptyWorldClient is a tmux server holding no sessions. It answers the
// cycle's own confirmation as ownServerPID and every confirmation after it as
// laterPID, counting the later ones.
type emptyWorldClient struct {
	laterPID   int
	cycleReads int
	laterReads int
}

func (c *emptyWorldClient) ShowAllServerOptions() (string, error)         { return "", nil }
func (c *emptyWorldClient) ListSessionNamesProbe() ([]string, error)      { return nil, nil }
func (c *emptyWorldClient) ListAllPanesWithFormat(string) (string, error) { return "", nil }
func (c *emptyWorldClient) ShowEnvironment(string) (string, error)        { return "", nil }

func (c *emptyWorldClient) ConfirmAnswering() (int, error) {
	if c.cycleReads == 0 {
		c.cycleReads++
		return ownServerPID, nil
	}
	c.laterReads++
	return c.laterPID, nil
}

// cycleWrite is one write through the scrollback writer a commit cycle hands
// its dump.
type cycleWrite struct {
	written    bool
	err        error
	laterReads int
}

// errEndBeforeCommit ends the cycle uncommitted, so its housekeeping pass
// cannot collect the transcript the write left.
var errEndBeforeCommit = errors.New("end the cycle before its commit")

func writeThroughCycle(t *testing.T, dir string, hm state.HashMap, laterPID int, data []byte) cycleWrite {
	t.Helper()
	client := &emptyWorldClient{laterPID: laterPID}
	var got cycleWrite
	_, err := state.RunCommitCycle(state.CommitCycle{
		Client:    client,
		OwnServer: ownServerPID,
		Dir:       dir,
		LoadPrev:  func() *state.Index { return nil },
		HashMap:   hm,
		Dump: func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
			got.written, got.err = w.Write(emptyCapturePaneKey, data, xxhash.Sum64(data))
			return got.written, errEndBeforeCommit
		},
	})
	if !errors.Is(err, errEndBeforeCommit) {
		t.Fatalf("RunCommitCycle = %v, want the dump's own error", err)
	}
	got.laterReads = client.laterReads
	return got
}

func readTranscript(t *testing.T, dir string) (string, bool) {
	t.Helper()
	body, err := os.ReadFile(state.ScrollbackFile(dir, emptyCapturePaneKey))
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	return string(body), true
}

func seedTranscript(t *testing.T, dir, paneKey, body string) {
	t.Helper()
	if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
		t.Fatalf("create scrollback dir: %v", err)
	}
	if err := os.WriteFile(state.ScrollbackFile(dir, paneKey), []byte(body), 0o600); err != nil {
		t.Fatalf("seed transcript: %v", err)
	}
}

func TestCycleScrollbackWriterWritesWithoutConfirmingWhenNothingSavedCanBeLost(t *testing.T) {
	cases := []struct {
		name  string
		saved *string
		data  string
	}{
		{"a non-empty capture over a saved transcript", new("saved"), "captured"},
		{"an empty capture with no saved transcript", nil, ""},
		{"an empty capture over an empty saved transcript", new(""), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.saved != nil {
				seedTranscript(t, dir, emptyCapturePaneKey, *tc.saved)
			}

			got := writeThroughCycle(t, dir, state.HashMap{}, ownServerPID+1, []byte(tc.data))

			if got.err != nil || !got.written {
				t.Errorf("Write = %t, %v; want a write", got.written, got.err)
			}
			if body, present := readTranscript(t, dir); !present || body != tc.data {
				t.Errorf("transcript = %q (present %t), want %q", body, present, tc.data)
			}
			if got.laterReads != 0 {
				t.Errorf("confirmation reads beside the cycle's = %d, want none", got.laterReads)
			}
		})
	}
}

func TestCycleScrollbackWriterSkipsACaptureMatchingTheCyclesDedupEntry(t *testing.T) {
	dir := t.TempDir()
	data := []byte("unchanged")
	hm := state.HashMap{emptyCapturePaneKey: xxhash.Sum64(data)}

	got := writeThroughCycle(t, dir, hm, ownServerPID, data)

	if got.err != nil || got.written {
		t.Errorf("Write = %t, %v; want no change", got.written, got.err)
	}
	if body, present := readTranscript(t, dir); present {
		t.Errorf("transcript written despite the dedup entry: %q", body)
	}
}

func TestCycleScrollbackWriterRefusesAnEmptyCaptureOverAnUninspectableTranscript(t *testing.T) {
	dir := t.TempDir()
	seedTranscript(t, dir, emptyCapturePaneKey, "saved")
	sbDir := state.ScrollbackDir(dir)
	if err := os.Chmod(sbDir, 0o000); err != nil {
		t.Fatalf("deny scrollback dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sbDir, 0o700) })

	got := writeThroughCycle(t, dir, state.HashMap{}, ownServerPID+1, nil)
	if err := os.Chmod(sbDir, 0o700); err != nil {
		t.Fatalf("restore scrollback dir: %v", err)
	}

	if got.written {
		t.Error("Write reported a write over an unconfirmed empty capture")
	}
	if !errors.Is(got.err, state.ErrUnconfirmedEmptyCapture) || !errors.Is(got.err, state.ErrNotOwnServer) {
		t.Errorf("Write error = %v, want ErrUnconfirmedEmptyCapture wrapping ErrNotOwnServer", got.err)
	}
	if got.laterReads != 1 {
		t.Errorf("confirmation reads beside the cycle's = %d, want 1", got.laterReads)
	}
	if body, present := readTranscript(t, dir); !present || body != "saved" {
		t.Errorf("transcript = %q (present %t), want unchanged %q", body, present, "saved")
	}
}
