package state_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func TestCaptureAndRefile(t *testing.T) {
	t.Run("it re-files every frozen pane before it returns, leaving each positional name for the commit's housekeeping pass", func(t *testing.T) {
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

		capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), ownServerPID, dir, &prev, hm, logger)
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
		if got := readScrollback(t, dir, "work__0.1.bin"); got != "frozen-body" {
			t.Errorf("positional file = %q, want %q", got, "frozen-body")
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

		capture, err := state.CaptureAndRefile(client, ownServerPID, dir, &prev, state.HashMap{}, logger)
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

		capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), ownServerPID, t.TempDir(), nil, state.HashMap{}, nil)
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

		capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), ownServerPID, dir, &prev, state.HashMap{}, nil)
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

			capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), ownServerPID, dir, &prev, state.HashMap{}, nil)
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

// A skeleton-marked pane restored at work:1.0 whose saved record may name
// another address's positional file, beside any occupant rows given.
func skeletonCycle(t *testing.T, dir, token, stored string, logger *slog.Logger, occupants ...string) state.CaptureCycle {
	t.Helper()
	prev := waitingIndex(token, stored)
	prev.Sessions[0].Windows[0].Index = 2
	prev.Sessions[0].Windows[0].Panes[0].Index = 0
	liveKey := state.SanitizePaneKey("work", 1, 0)
	rows := append([]string{paneLineWithPending("work", 1, "main", "tiled", false, true, 0, "/fresh", true, "portal", token, "")}, occupants...)
	mock := &captureMock{
		listSessions: listSessionsFor("work"),
		listPanes:    strings.Join(rows, "\n"),
		markers:      state.SkeletonMarkerPrefix + liveKey + ` "1"`,
		t:            t,
	}
	capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), ownServerPID, dir, &prev, state.HashMap{}, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return capture
}

// occupantOfSavedAddress is an unmarked, unregistered pane now sitting at
// work:2.0, the address the moved pane's record is filed under.
func occupantOfSavedAddress() string {
	return paneLine("work", 2, "logs", "tiled", false, false, 0, "/logs", true, "tail")
}

func assertNoTokenFile(t *testing.T, dir, token string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), "pane-"+token+".bin")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("token-named file stat err = %v, want not-exist", err)
	}
}

func TestCaptureAndRefileLinksAMovedSkeletonPaneOntoItsToken(t *testing.T) {
	t.Run("it names the token file and leaves the baked positional path holding the same bytes", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__2.0.bin", "saved-transcript")

		capture := skeletonCycle(t, dir, waitingPaneToken, "scrollback/work__2.0.bin", nil, occupantOfSavedAddress())

		p := findPane(capture.Index, "work", 1, 0)
		if p == nil {
			t.Fatalf("index missing work:1.0: %+v", capture.Index.Sessions)
		}
		if want := state.PendingScrollbackFile(waitingPaneToken); p.ScrollbackFile != want {
			t.Errorf("ScrollbackFile = %q, want %q", p.ScrollbackFile, want)
		}
		if got := readScrollback(t, dir, "pane-"+waitingPaneToken+".bin"); got != "saved-transcript" {
			t.Errorf("token-named file = %q, want %q", got, "saved-transcript")
		}
		if got := readScrollback(t, dir, "work__2.0.bin"); got != "saved-transcript" {
			t.Errorf("baked positional file = %q, want %q", got, "saved-transcript")
		}
	})

	t.Run("it keeps a pane restored at its saved address on its own positional file", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__1.0.bin", "saved-transcript")

		capture := skeletonCycle(t, dir, waitingPaneToken, "scrollback/work__1.0.bin", nil)

		if got := findPane(capture.Index, "work", 1, 0).ScrollbackFile; got != "scrollback/work__1.0.bin" {
			t.Errorf("ScrollbackFile = %q, want %q", got, "scrollback/work__1.0.bin")
		}
		assertNoTokenFile(t, dir, waitingPaneToken)
	})

	t.Run("it keeps the positional path while nothing occupies the saved address", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__2.0.bin", "saved-transcript")

		capture := skeletonCycle(t, dir, waitingPaneToken, "scrollback/work__2.0.bin", nil)

		if got, want := findPane(capture.Index, "work", 1, 0).ScrollbackFile, "scrollback/work__2.0.bin"; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		assertNoTokenFile(t, dir, waitingPaneToken)
		if err := state.Commit(dir, capture.Index, false, nil); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if got := readScrollback(t, dir, "work__2.0.bin"); got != "saved-transcript" {
			t.Errorf("baked positional file after commit = %q, want %q", got, "saved-transcript")
		}
	})

	t.Run("it keeps the positional path while the pane at the saved address is itself frozen", func(t *testing.T) {
		dir := t.TempDir()
		const occupantToken = "xy78zw"
		seedScrollback(t, dir, "work__2.0.bin", "saved-transcript")
		seedScrollback(t, dir, "work__3.0.bin", "occupant-transcript")
		prev := waitingIndex(waitingPaneToken, "scrollback/work__2.0.bin")
		prev.Sessions[0].Windows[0].Index = 2
		prev.Sessions[0].Windows[0].Panes[0].Index = 0
		prev.Sessions[0].Windows = append(prev.Sessions[0].Windows, state.Window{
			Index: 3, Name: "logs", Layout: "tiled",
			Panes: []state.Pane{{Index: 0, CWD: "/logs", CurrentCommand: "tail", ScrollbackFile: "scrollback/work__3.0.bin", PortalPaneID: occupantToken}},
		})
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes: paneLineWithPending("work", 1, "main", "tiled", false, true, 0, "/fresh", true, "portal", waitingPaneToken, "") + "\n" +
				paneLineWithPending("work", 2, "logs", "tiled", false, false, 0, "/fresh", true, "portal", occupantToken, "1"),
			markers: state.SkeletonMarkerPrefix + state.SanitizePaneKey("work", 1, 0) + ` "1"` + "\n" +
				state.SkeletonMarkerPrefix + state.SanitizePaneKey("work", 2, 0) + ` "1"`,
			t: t,
		}

		capture, err := state.CaptureAndRefile(tmux.NewClient(mock.commander()), ownServerPID, dir, &prev, state.HashMap{}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, want := findPane(capture.Index, "work", 1, 0).ScrollbackFile, "scrollback/work__2.0.bin"; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		assertNoTokenFile(t, dir, waitingPaneToken)
		if err := state.Commit(dir, capture.Index, false, nil); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if got := readScrollback(t, dir, "work__2.0.bin"); got != "saved-transcript" {
			t.Errorf("baked positional file after commit = %q, want %q", got, "saved-transcript")
		}
	})

	t.Run("it keeps a pane already filed under its token on that file", func(t *testing.T) {
		dir := t.TempDir()
		tokenPath := state.PendingScrollbackFile(waitingPaneToken)
		seedScrollback(t, dir, "pane-"+waitingPaneToken+".bin", "saved-transcript")

		capture := skeletonCycle(t, dir, waitingPaneToken, tokenPath, nil)

		if got := findPane(capture.Index, "work", 1, 0).ScrollbackFile; got != tokenPath {
			t.Errorf("ScrollbackFile = %q, want %q", got, tokenPath)
		}
		if got := readScrollback(t, dir, "pane-"+waitingPaneToken+".bin"); got != "saved-transcript" {
			t.Errorf("token-named file = %q, want %q", got, "saved-transcript")
		}
	})

	t.Run("it keeps the positional path for a token the pane-token rule refuses", func(t *testing.T) {
		dir := t.TempDir()
		const refused = "not-a-token"
		seedScrollback(t, dir, "work__2.0.bin", "saved-transcript")

		capture := skeletonCycle(t, dir, refused, "scrollback/work__2.0.bin", nil, occupantOfSavedAddress())

		if got := findPane(capture.Index, "work", 1, 0).ScrollbackFile; got != "scrollback/work__2.0.bin" {
			t.Errorf("ScrollbackFile = %q, want %q", got, "scrollback/work__2.0.bin")
		}
		entries, err := os.ReadDir(state.ScrollbackDir(dir))
		if err != nil {
			t.Fatalf("read scrollback dir: %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != "work__2.0.bin" {
			t.Errorf("scrollback dir = %v, want only work__2.0.bin", entries)
		}
	})

	t.Run("it warns once and leaves the record alone when the link fails", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__2.0.bin", "saved-transcript")
		denyScrollbackWrites(t, dir)
		logger, sink := openTempLogger(t)

		capture := skeletonCycle(t, dir, waitingPaneToken, "scrollback/work__2.0.bin", logger, occupantOfSavedAddress())

		if got, want := findPane(capture.Index, "work", 1, 0).ScrollbackFile, "scrollback/work__2.0.bin"; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		assertNoTokenFile(t, dir, waitingPaneToken)
		rec := sink.Records().AtExactLevel(slog.LevelWarn).Only(t, "link failure warning")
		if got, want := rec.AttrOrEmpty("pane_key"), "work__1.0"; got != want {
			t.Errorf("pane_key = %q, want %q", got, want)
		}
		if got, want := rec.AttrOrEmpty("path"), "scrollback/work__2.0.bin"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if !rec.HasAttr("error") {
			t.Errorf("warning carries no error attr: %v", rec.Keys)
		}
		for _, key := range rec.Keys {
			switch key {
			case "pane_key", "path", "error":
			default:
				t.Errorf("warning carries unexpected attr key %q", key)
			}
		}
	})

	t.Run("it adopts a token-named file that already exists", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__2.0.bin", "intruder-capture")
		seedScrollback(t, dir, "pane-"+waitingPaneToken+".bin", "saved-transcript")

		capture := skeletonCycle(t, dir, waitingPaneToken, "scrollback/work__2.0.bin", nil, occupantOfSavedAddress())

		if want := state.PendingScrollbackFile(waitingPaneToken); findPane(capture.Index, "work", 1, 0).ScrollbackFile != want {
			t.Errorf("ScrollbackFile = %q, want %q", findPane(capture.Index, "work", 1, 0).ScrollbackFile, want)
		}
		if got := readScrollback(t, dir, "pane-"+waitingPaneToken+".bin"); got != "saved-transcript" {
			t.Errorf("token-named file = %q, want %q", got, "saved-transcript")
		}
	})
}
