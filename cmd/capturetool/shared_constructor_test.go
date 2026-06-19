package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portalbintest"
)

// TestSharedConstructorUsedByBothPaths asserts that BOTH the production TUI
// launch (cmd/open.go) and the offline capture tool (cmd/capturetool/main.go)
// construct their model through the shared tui.Build constructor.
//
// This is the anti-drift guard the visual-verification harness depends on: if
// the capture tool grew its own bespoke construction path, the captured frame
// would no longer be the production frame and every later reskin task's visual
// gate would compare against a lie. Conversely, if production stopped routing
// through tui.Build, the two could silently diverge. Pinning "both call
// tui.Build" keeps the harness honest (spec § 15.4 / tick task: "shared tui.Build
// constructor; no bespoke render path that could drift from reality").
func TestSharedConstructorUsedByBothPaths(t *testing.T) {
	root, err := portalbintest.ProjectRoot()
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}

	const sharedConstructorCall = "tui.Build("
	files := map[string]string{
		"production (cmd/open.go)":       filepath.Join(root, "cmd", "open.go"),
		"capture tool (cmd/capturetool)": filepath.Join(root, "cmd", "capturetool", "main.go"),
	}

	for label, path := range files {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s (%s): %v", label, path, readErr)
		}
		if !strings.Contains(string(data), sharedConstructorCall) {
			t.Errorf("%s does not call the shared %s constructor — the capture frame must be built the same way production builds it", label, sharedConstructorCall)
		}
	}
}
