package bootstrapadapter_test

// Integration tests for the production-shape bootstrap adapters. The two
// adapters are pure pass-throughs to *tmux.Client / tmux.RegisterPortalHooks,
// so the goal here is a thin smoke layer: prove that Set/Clear toggle the
// expected server option on a live tmux server and that RegisterPortalHooks
// runs to completion. Heavy hook-table semantics are owned by
// internal/tmux/hooks_register_test.go; the orchestrator-level wiring is
// owned by cmd/bootstrap/phase5_integration_test.go. This file only proves
// the adapters' shaping.

import (
	"testing"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestRestoringMarker_SetClearsTogglesServerOption proves that Set writes
// @portal-restoring="1" and Clear removes it, both observable on the live
// tmux server. The literal name comes from state.RestoringMarkerName so the
// adapter cannot drift from the canonical constant.
func TestRestoringMarker_SetClearsTogglesServerOption(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-bsa-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	m := &bootstrapadapter.RestoringMarker{Client: client}

	// Pre-condition: marker MUST be absent.
	if _, found, err := client.TryGetServerOption(state.RestoringMarkerName); err != nil {
		t.Fatalf("TryGetServerOption pre-Set: %v", err)
	} else if found {
		t.Fatal("@portal-restoring unexpectedly set before Set()")
	}

	// Set: marker MUST be "1".
	if err := m.Set(); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, found, err := client.TryGetServerOption(state.RestoringMarkerName)
	if err != nil {
		t.Fatalf("TryGetServerOption post-Set: %v", err)
	}
	if !found || val != "1" {
		t.Errorf("post-Set: found=%v value=%q; want found=true value=%q", found, val, "1")
	}

	// Clear: marker MUST be absent again.
	if err := m.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, found, err := client.TryGetServerOption(state.RestoringMarkerName); err != nil {
		t.Fatalf("TryGetServerOption post-Clear: %v", err)
	} else if found {
		t.Error("@portal-restoring still present after Clear()")
	}

	// Clear is idempotent: a second invocation MUST NOT error.
	if err := m.Clear(); err != nil {
		t.Errorf("Clear (second invocation): %v", err)
	}
}

// TestHookRegistrar_RegistersPortalHooks proves that RegisterPortalHooks
// runs without error against a live tmux server. The hook-table contents
// are exercised in detail by internal/tmux/hooks_register_test.go and at
// the orchestrator level by phase5_integration_test.go; this test only
// confirms the adapter shape.
func TestHookRegistrar_RegistersPortalHooks(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-bsa-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	r := &bootstrapadapter.HookRegistrar{Client: client}
	if err := r.RegisterPortalHooks(); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	// Idempotent: a second invocation MUST NOT error or duplicate entries.
	if err := r.RegisterPortalHooks(); err != nil {
		t.Errorf("RegisterPortalHooks (second invocation): %v", err)
	}
}
