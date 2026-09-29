package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func TestCaptureAndRefile(t *testing.T) {
	t.Run("it re-files every frozen pane before it returns", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__0.1.bin", "frozen-body")
		prev := waitingIndex(waitingPaneToken, "scrollback/work__0.1.bin")
		hm := state.HashMap{"work__0.1": 42}
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLineWithPending("work", 0, "main", "tiled", false, true, 1, "/tmp", true, "zsh", waitingPaneToken, "1"),
			t:            t,
		}
		logger, _ := openTempLogger(t)

		capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), dir, &prev, hm, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertPaneKeySet(t, capture.Pending, waitingSet())
		want := "scrollback/pane-" + waitingPaneToken + ".bin"
		if got := waitingPaneOf(t, capture.Index).ScrollbackFile; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		if got := readScrollback(t, dir, "pane-"+waitingPaneToken+".bin"); got != "frozen-body" {
			t.Errorf("token-named file = %q, want %q", got, "frozen-body")
		}
		if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), "work__0.1.bin")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("positional file stat err = %v, want not-exist", err)
		}
		if _, held := hm["work__0.1"]; held {
			t.Errorf("hash map still holds the vacated key: %v", hm)
		}
	})

	t.Run("it returns the capture's error with nothing re-filed", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__0.1.bin", "frozen-body")
		prev := waitingIndex(waitingPaneToken, "scrollback/work__0.1.bin")
		captureErr := errors.New("exec: tmux broken")
		client := &failFastCaptureClient{t: t, listSessionNamesErr: captureErr}
		logger, _ := openTempLogger(t)

		capture, err := state.CaptureAndRefile(client, dir, &prev, state.HashMap{}, logger)
		if !errors.Is(err, captureErr) {
			t.Fatalf("error = %v, want %v", err, captureErr)
		}
		if len(capture.Index.Sessions) != 0 {
			t.Errorf("Sessions = %d, want 0 on a failed capture", len(capture.Index.Sessions))
		}
		if capture.Pending == nil {
			t.Fatal("pending set is nil; want a non-nil empty map")
		}
		assertPaneKeySet(t, capture.Pending, map[string]struct{}{})
		if got := readScrollback(t, dir, "work__0.1.bin"); got != "frozen-body" {
			t.Errorf("positional file = %q, want it untouched", got)
		}
		if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), "pane-"+waitingPaneToken+".bin")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("token-named file stat err = %v, want not-exist", err)
		}
	})
	t.Run("it reads the skeleton markers itself and hands them back", func(t *testing.T) {
		skeletonKey := state.SanitizePaneKey("work", 0, 0)
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "main", "tiled", false, true, 0, "/tmp", true, "zsh"),
			markers:      state.SkeletonMarkerPrefix + skeletonKey + ` "1"`,
			t:            t,
		}

		capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), t.TempDir(), nil, state.HashMap{}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertPaneKeySet(t, capture.Skeleton, map[string]struct{}{skeletonKey: {}})
		assertPaneKeySet(t, capture.Pending, map[string]struct{}{})
	})

	t.Run("it takes no capture when the skeleton-marker read fails", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__0.1.bin", "frozen-body")
		prev := waitingIndex(waitingPaneToken, "scrollback/work__0.1.bin")
		markerErr := errors.New("show-options blew up")
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLineWithPending("work", 0, "main", "tiled", false, true, 1, "/tmp", true, "zsh", waitingPaneToken, "1"),
			markersE:     markerErr,
			t:            t,
		}

		capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), dir, &prev, state.HashMap{}, nil)
		if !errors.Is(err, markerErr) {
			t.Fatalf("error = %v, want %v", err, markerErr)
		}
		if mock.listSessionsCalls != 0 || mock.listPanesCalls != 0 {
			t.Errorf("list-sessions calls = %d, list-panes calls = %d; want no capture after a failed marker read",
				mock.listSessionsCalls, mock.listPanesCalls)
		}
		if len(capture.Index.Sessions) != 0 {
			t.Errorf("Sessions = %d, want 0 on a failed marker read", len(capture.Index.Sessions))
		}
		if got := readScrollback(t, dir, "work__0.1.bin"); got != "frozen-body" {
			t.Errorf("positional file = %q, want it untouched", got)
		}
	})
}

// A waiting pane restored away from its saved address is carried through the
// three stages the helper hands it over in: armed, marked pending while still
// armed, and pending alone.
func TestCaptureAndRefileKeepsARestoredWaitingPaneTranscript(t *testing.T) {
	tokenPath := "scrollback/pane-" + waitingPaneToken + ".bin"
	liveKey := state.SanitizePaneKey("work", 1, 0)
	skeletonMarker := state.SkeletonMarkerPrefix + liveKey + ` "1"`

	stages := []struct {
		name    string
		markers string
		pending string
	}{
		{name: "skeleton marker only", markers: skeletonMarker, pending: ""},
		{name: "both markers", markers: skeletonMarker, pending: "1"},
		{name: "pending marker only", markers: "", pending: "1"},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			dir := t.TempDir()
			seedScrollback(t, dir, "pane-"+waitingPaneToken+".bin", "frozen-body")
			prev := waitingIndex(waitingPaneToken, tokenPath)
			if err := state.Commit(dir, prev, false, nil); err != nil {
				t.Fatalf("seed sessions.json: %v", err)
			}
			mock := &captureMock{
				listSessions: listSessionsFor("work"),
				listPanes:    paneLineWithPending("work", 1, "main", "tiled", false, true, 0, "/fresh", true, "sleep", waitingPaneToken, stage.pending),
				markers:      stage.markers,
				t:            t,
			}

			capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), dir, &prev, state.HashMap{}, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := state.Commit(dir, capture.Index, false, nil); err != nil {
				t.Fatalf("Commit: %v", err)
			}

			committed, _, err := state.ReadIndex(dir)
			if err != nil {
				t.Fatalf("ReadIndex: %v", err)
			}
			p := findPane(committed, "work", 1, 0)
			if p == nil {
				t.Fatalf("committed index missing work:1.0: %+v", committed.Sessions)
			}
			if p.ScrollbackFile != tokenPath || p.CWD != "/tmp" || p.CurrentCommand != "zsh" {
				t.Errorf("committed pane = %+v, want the saved /tmp + zsh naming %q", *p, tokenPath)
			}
			if got := readScrollback(t, dir, "pane-"+waitingPaneToken+".bin"); got != "frozen-body" {
				t.Errorf("token-named file = %q, want %q", got, "frozen-body")
			}
		})
	}
}
