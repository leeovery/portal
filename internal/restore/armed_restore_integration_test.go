//go:build integration

package restore_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestPhase3Integration_SaveRestoreRoundTrip(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	ts := tmuxtest.New(t, "ptl-")

	ts.Run(t, "new-session", "-d", "-s", "alpha")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	client := ts.Client()

	idx, _, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "alpha" {
		t.Fatalf("expected one captured session named alpha, got %+v", idx.Sessions)
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := writeFile(state.SessionsJSON(stateDir), data); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	restoretest.RebootServer(t, ts, client)

	o := restoretest.NewRestoreOrchestrator(t, client, stateDir, binDir)
	if err := restoretest.RestoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker: %v", err)
	}

	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	if !strings.Contains(out, "alpha") {
		t.Fatalf("expected alpha in list-sessions; got %q", out)
	}

	wantMarker := "@portal-skeleton-" + state.SanitizePaneKey("alpha", 0, 0)
	markerOut := ts.Run(t, "show-options", "-sv", wantMarker)
	if strings.TrimSpace(markerOut) == "" {
		t.Errorf("expected marker %q to be set; got empty value", wantMarker)
	}

	if out, err := ts.TryRun("show-options", "-sv", state.RestoringMarkerName); err == nil && strings.TrimSpace(out) != "" {
		t.Errorf("%s should be unset after marker block; got %q", state.RestoringMarkerName, out)
	}

	if _, err := o.Restore(); err != nil {
		t.Fatalf("second Restore: %v", err)
	}
	out2 := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	if got := strings.Count(out2, "alpha"); got != 1 {
		t.Errorf("expected exactly one alpha session after second Restore; got %d in %q", got, out2)
	}
}

func TestPhase3Integration_RestoreUsesLiveIndicesUnderBaseIndexDrift(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	ts := tmuxtest.New(t, "ptl-")

	ts.Run(t, "new-session", "-d", "-s", "alpha")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	client := ts.Client()
	idx, _, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	if len(idx.Sessions) != 1 {
		t.Fatalf("expected 1 captured session, got %d", len(idx.Sessions))
	}
	savedPane := idx.Sessions[0].Windows[0].Panes[0]
	if savedPane.Index != 0 {
		t.Fatalf("saved pane Index = %d, want 0 (capture should reflect server default)", savedPane.Index)
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := writeFile(state.SessionsJSON(stateDir), data); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	restoretest.RebootServer(t, ts, client)
	// Server-lifetime options do not survive the kill, so the drift this test is
	// about is re-applied onto the fresh server.
	tmuxtest.ApplyBaseIndices(t, ts, 1, 1)

	o := restoretest.NewRestoreOrchestrator(t, client, stateDir, binDir)
	if err := restoretest.RestoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker: %v", err)
	}

	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	if !strings.Contains(out, "alpha") {
		t.Fatalf("expected alpha in list-sessions; got %q", out)
	}

	livePanesOut := strings.Join(restoretest.LivePaneCoords(t, ts, "alpha"), " ")
	if livePanesOut != "1:1" {
		t.Fatalf("alpha live panes = %q, want %q (base-index drift)", livePanesOut, "1:1")
	}

	liveKey := state.SanitizePaneKey("alpha", 1, 1)
	liveFIFO := state.FIFOPath(stateDir, liveKey)
	if _, err := os.Lstat(liveFIFO); err != nil {
		t.Errorf("expected FIFO at live key %s, missing: %v", liveFIFO, err)
	}
	savedKey := state.SanitizePaneKey("alpha", 0, 0)
	savedFIFO := state.FIFOPath(stateDir, savedKey)
	if _, err := os.Lstat(savedFIFO); err == nil {
		t.Errorf("did not expect FIFO at saved-key path %s under index drift", savedFIFO)
	}

	wantMarker := "@portal-skeleton-" + liveKey
	markerOut := ts.Run(t, "show-options", "-sv", wantMarker)
	if strings.TrimSpace(markerOut) == "" {
		t.Errorf("expected marker %q to be set; got empty value", wantMarker)
	}
	dontWantMarker := "@portal-skeleton-" + savedKey
	if out, err := ts.TryRun("show-options", "-sv", dontWantMarker); err == nil && strings.TrimSpace(out) != "" {
		t.Errorf("did not expect marker %q (saved-key); got %q", dontWantMarker, out)
	}
}
