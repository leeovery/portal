package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

const occupantTranscript = "killed-pane-old-transcript"

// occupyPositional leaves another pane's old transcript at the file X's live
// address names, the way a renumbered window hands X a dead sibling's path.
func occupyPositional(t *testing.T, dir string) {
	t.Helper()
	seedScrollback(t, dir, handOverPositionalFile(), occupantTranscript)
}

type positionalState struct {
	name  string
	stage func(t *testing.T, dir string)
}

var positionalStates = []positionalState{
	{"its positional path absent", func(*testing.T, string) {}},
	{"its positional path holding another pane's file", occupyPositional},
}

// assertPositionalNotOverwritten fails when X's positional file holds anything
// but the other pane's old transcript. Housekeeping may remove that file once
// no record names it.
func assertPositionalNotOverwritten(t *testing.T, dir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(state.ScrollbackDir(dir), handOverPositionalFile()))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil || string(data) != occupantTranscript {
		t.Errorf("positional file = %q, %v; want the other pane's %q or nothing", data, err, occupantTranscript)
	}
}

type unansweredDump struct {
	name   string
	client func() *answeredClient
	dump   func(written *bool, writeErr *error) dumpFunc
}

var unansweredDumps = []unansweredDump{
	{"an empty capture whose confirmation is refused",
		func() *answeredClient {
			return &answeredClient{later: func() (int, error) { return 0, errors.New("server exited") }}
		},
		func(written *bool, writeErr *error) dumpFunc { return writesX(nil, written, writeErr) }},
	{"an empty capture confirmed by another server",
		func() *answeredClient {
			return &answeredClient{later: func() (int, error) { return ownServerPID + 1, nil }}
		},
		func(written *bool, writeErr *error) dumpFunc { return writesX(nil, written, writeErr) }},
	{"an empty capture confirmed naming no server",
		func() *answeredClient { return &answeredClient{later: func() (int, error) { return 0, nil }} },
		func(written *bool, writeErr *error) dumpFunc { return writesX(nil, written, writeErr) }},
	{"a refused capture",
		func() *answeredClient { return &answeredClient{} },
		func(*bool, *error) dumpFunc { return dumpsNothing }},
}

func TestRunCommitCycleHoldsAnAnsweredPanesTranscriptFromTheLastCommitWhenItsPositionalPathHoldsAnotherPanesFile(t *testing.T) {
	for _, tc := range unansweredDumps {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			answeredSeed(t, dir)
			occupyPositional(t, dir)
			var written bool
			var writeErr error

			if _, err := state.RunCommitCycle(answeredTick(tc.client(), dir, staleDaemonPrev(), state.HashMap{}, tc.dump(&written, &writeErr))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if written {
				t.Error("Write reported a write over the answered pane's transcript")
			}
			assertPositionalNotOverwritten(t, dir)
			assertXKeptUnderToken(t, dir)
		})
	}
}

func TestRunCommitCycleFilesAnAnsweredPaneAtItsOccupiedPositionalFileOnceItsConfirmedCaptureIsWritten(t *testing.T) {
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
			answeredSeed(t, dir)
			occupyPositional(t, dir)
			client := &answeredClient{}
			var written bool
			var writeErr error

			if _, err := state.RunCommitCycle(answeredTick(client, dir, staleDaemonPrev(), state.HashMap{}, writesX([]byte(tc.data), &written, &writeErr))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if !written || writeErr != nil {
				t.Errorf("Write = %t, %v; want a write", written, writeErr)
			}
			if client.laterReads != tc.laterReads {
				t.Errorf("confirmation reads after the capture = %d, want %d", client.laterReads, tc.laterReads)
			}
			assertXFiledAtPositional(t, dir, tc.data)
		})
	}
}

// committedOutcome is what a cycle left behind for X: the record sessions.json
// names for it and the scrollback directory's files with their contents.
type committedOutcome struct {
	record state.Pane
	files  map[string]string
}

func outcomeOf(t *testing.T, dir string) committedOutcome {
	t.Helper()
	entries, err := os.ReadDir(state.ScrollbackDir(dir))
	if err != nil {
		t.Fatalf("read scrollback dir: %v", err)
	}
	files := map[string]string{}
	for _, e := range entries {
		files[e.Name()] = readScrollback(t, dir, e.Name())
	}
	return committedOutcome{record: recordFor(t, onDiskIndex(t, dir), "work"), files: files}
}

func TestRunCommitCycleDecidesTheAnsweredPaneHoldTheSameWhicheverIndexTheCommitterHolds(t *testing.T) {
	committers := []struct {
		name  string
		cycle func(t *testing.T, dir string, seed state.Index) state.CommitCycle
	}{
		{"a tick whose previous index names the token-named transcript", func(_ *testing.T, dir string, seed state.Index) state.CommitCycle {
			var written bool
			var writeErr error
			return answeredTick(&answeredClient{later: func() (int, error) { return ownServerPID + 1, nil }}, dir, seed, state.HashMap{}, writesX(nil, &written, &writeErr))
		}},
		{"a tick whose previous index predates the token filing", func(_ *testing.T, dir string, _ state.Index) state.CommitCycle {
			var written bool
			var writeErr error
			return answeredTick(&answeredClient{later: func() (int, error) { return ownServerPID + 1, nil }}, dir, staleDaemonPrev(), state.HashMap{}, writesX(nil, &written, &writeErr))
		}},
		{"a commit-now", func(t *testing.T, dir string, _ state.Index) state.CommitCycle {
			return answeredCommitNow(t, &answeredClient{}, dir)
		}},
	}
	for _, pos := range positionalStates {
		t.Run(pos.name, func(t *testing.T) {
			var want *committedOutcome
			for _, c := range committers {
				dir := t.TempDir()
				seed := answeredSeed(t, dir)
				pos.stage(t, dir)

				if _, err := state.RunCommitCycle(c.cycle(t, dir, seed)); err != nil {
					t.Fatalf("%s: RunCommitCycle: %v", c.name, err)
				}

				assertXKeptUnderToken(t, dir)
				assertPositionalNotOverwritten(t, dir)
				got := outcomeOf(t, dir)
				if want == nil {
					want = &got
					continue
				}
				if got.record != want.record {
					t.Errorf("%s committed %+v for X, want %+v", c.name, got.record, want.record)
				}
				if len(got.files) != len(want.files) {
					t.Errorf("%s left files %v, want %v", c.name, got.files, want.files)
				}
				for name, body := range want.files {
					if got.files[name] != body {
						t.Errorf("%s left %s = %q, want %q", c.name, name, got.files[name], body)
					}
				}
			}
		})
	}
}

func TestRunCommitCycleFallsBackToThePreviousIndexForTheHoldWhenSessionsJSONIsUnreadable(t *testing.T) {
	unreadable := []struct {
		name  string
		stage func(t *testing.T, dir string)
	}{
		{"sessions.json absent", func(t *testing.T, dir string) {
			if err := os.Remove(state.SessionsJSON(dir)); err != nil {
				t.Fatalf("remove sessions.json: %v", err)
			}
		}},
		{"sessions.json not decodable", func(t *testing.T, dir string) {
			if err := os.WriteFile(state.SessionsJSON(dir), []byte("{not json"), 0o600); err != nil {
				t.Fatalf("corrupt sessions.json: %v", err)
			}
		}},
	}
	for _, u := range unreadable {
		t.Run(u.name+"/a previous record naming the token-named transcript keeps it", func(t *testing.T) {
			dir := t.TempDir()
			seed := answeredSeed(t, dir)
			occupyPositional(t, dir)
			u.stage(t, dir)
			var written bool
			var writeErr error
			client := &answeredClient{later: func() (int, error) { return ownServerPID + 1, nil }}

			if _, err := state.RunCommitCycle(answeredTick(client, dir, seed, state.HashMap{}, writesX(nil, &written, &writeErr))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if written || !errors.Is(writeErr, state.ErrUnconfirmedEmptyCapture) {
				t.Errorf("Write = %t, %v; want a refusal judged at the token-named transcript", written, writeErr)
			}
			assertTranscriptFiledUnderToken(t, dir)
			assertRestoreFindsXTranscript(t, dir)
		})

		t.Run(u.name+"/a previous record naming the positional path judges the empty capture there", func(t *testing.T) {
			dir := t.TempDir()
			answeredSeed(t, dir)
			seedScrollback(t, dir, handOverPositionalFile(), "")
			u.stage(t, dir)
			var written bool
			var writeErr error
			client := &answeredClient{later: func() (int, error) { return ownServerPID + 1, nil }}

			if _, err := state.RunCommitCycle(answeredTick(client, dir, staleDaemonPrev(), state.HashMap{}, writesX(nil, &written, &writeErr))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if !written || writeErr != nil {
				t.Errorf("Write = %t, %v; want the empty capture written over the empty positional file", written, writeErr)
			}
			if client.laterReads != 0 {
				t.Errorf("confirmation reads after the capture = %d, want none", client.laterReads)
			}
			assertXFiledAtPositional(t, dir, "")
		})
	}
}
