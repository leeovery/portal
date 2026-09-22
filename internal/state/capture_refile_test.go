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

		idx, pending, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), dir, nil, &prev, hm, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertPaneKeySet(t, pending, waitingSet())
		want := "scrollback/pane-" + waitingPaneToken + ".bin"
		if got := waitingPaneOf(t, idx).ScrollbackFile; got != want {
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

		idx, pending, err := state.CaptureAndRefile(client, dir, nil, &prev, state.HashMap{}, logger)
		if !errors.Is(err, captureErr) {
			t.Fatalf("error = %v, want %v", err, captureErr)
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("Sessions = %d, want 0 on a failed capture", len(idx.Sessions))
		}
		if pending == nil {
			t.Fatal("pending set is nil; want a non-nil empty map")
		}
		assertPaneKeySet(t, pending, map[string]struct{}{})
		if got := readScrollback(t, dir, "work__0.1.bin"); got != "frozen-body" {
			t.Errorf("positional file = %q, want it untouched", got)
		}
		if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), "pane-"+waitingPaneToken+".bin")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("token-named file stat err = %v, want not-exist", err)
		}
	})
}
