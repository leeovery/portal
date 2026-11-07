package logtest_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// The rule reads source, so each case below is a whole miniature file, staged
// at the path the rule judges it under.
const (
	fixtureHandInstalledSink = `package x

import "testing"

func TestRogue(t *testing.T) {
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)
	_ = sink
}
`

	fixtureLevelGateHandler = `package cmd

import "testing"

func TestExecMarker_VisibleAtWARN(t *testing.T) {
	h, sink := newWARNBypassHandler()
	log.SetTestHandler(t, h)
	_ = sink
}
`

	fixtureJSONRenderingHandler = `package hooks_test

import "testing"

func TestSetEmitsOpAsJSONField(t *testing.T) {
	var buf bytes.Buffer
	log.SetTestHandler(t, slog.NewJSONHandler(&buf, nil))
}
`

	fixtureInstallItself = `package logtest

import "testing"

func Install(t *testing.T) *Sink {
	sink := &Sink{}
	log.SetTestHandler(t, sink)
	return sink
}
`

	fixtureNoInstallAtAll = `package x

import "testing"

func TestX(t *testing.T) {
	sink := logtest.Install(t)
	_ = sink
}
`
)

func TestHandlerRuleFlagsAHandInstalledCaptureHandler(t *testing.T) {
	rogue := filepath.Join("cmd", "rogue_test.go")

	scanned, defects := auditHandlerInstalls(stageHandlerInstalls(t, rogue, fixtureHandInstalledSink))

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1", scanned)
	}
	if len(defects) != 1 || !strings.Contains(defects[0], rogue) {
		t.Fatalf("defects = %v, want one naming %s", defects, rogue)
	}
	if !strings.Contains(defects[0], "logtest.Install") {
		t.Errorf("defect %q does not name the route a capture handler must be taken from", defects[0])
	}
}

func TestHandlerRuleAllowsTheTwoSanctionedHandlersAndInstall(t *testing.T) {
	var installs []handlerInstall
	for _, staged := range []struct{ rel, src string }{
		{filepath.Join("cmd", "open_test.go"), fixtureLevelGateHandler},
		{filepath.Join("internal", "hooks", "store_test.go"), fixtureJSONRenderingHandler},
		{filepath.Join("internal", "logtest", "install.go"), fixtureInstallItself},
	} {
		installs = append(installs, stageHandlerInstalls(t, staged.rel, staged.src)...)
	}

	scanned, defects := auditHandlerInstalls(installs)

	if scanned != 3 {
		t.Fatalf("scanned = %d, want 3", scanned)
	}
	if len(defects) != 0 {
		t.Fatalf("defects = %v, want none — the level gate and the JSON renderer are each sanctioned", defects)
	}
	if unmatched := unmatchedSanctions(installs); len(unmatched) != 0 {
		t.Errorf("unmatchedSanctions = %v, want none over installs at every sanctioned site", unmatched)
	}
}

func TestHandlerRuleReportsASanctionedSiteThatNoLongerInstallsAHandler(t *testing.T) {
	installs := stageHandlerInstalls(t, filepath.Join("internal", "logtest", "install.go"), fixtureInstallItself)

	unmatched := unmatchedSanctions(installs)

	if len(unmatched) != 2 {
		t.Fatalf("unmatchedSanctions = %v, want the two sanctioned sites making no install", unmatched)
	}
	for _, want := range []string{"open_test.go", "store_test.go"} {
		if !containsSubstring(unmatched, want) {
			t.Errorf("unmatchedSanctions = %v, want one naming %s", unmatched, want)
		}
	}
}

func TestHandlerRuleFailsWhenItScansNothing(t *testing.T) {
	scanned, defects := auditHandlerInstalls(stageHandlerInstalls(t, filepath.Join("cmd", "quiet_test.go"), fixtureNoInstallAtAll))

	if scanned != 0 {
		t.Fatalf("scanned = %d, want 0", scanned)
	}
	msg := handlerGuardFailure(scanned, defects)
	if msg == "" {
		t.Fatal("handlerGuardFailure = empty over zero installs; the guard must fail rather than pass having found nothing")
	}
	if !strings.Contains(msg, "stopped looking") {
		t.Errorf("handlerGuardFailure = %q; want it to say the guard has stopped looking", msg)
	}
}

// containsSubstring reports whether any of msgs carries substring.
func containsSubstring(msgs []string, substring string) bool {
	for _, msg := range msgs {
		if strings.Contains(msg, substring) {
			return true
		}
	}
	return false
}
