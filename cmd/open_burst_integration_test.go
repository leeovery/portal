//go:build integration

package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/shellquote"
)

// recipeTerminal is the identity of a terminal of the test's own: a
// terminals.json recipe whose script hands the test each command Portal wants
// a window for, which the test runs on a pseudo-terminal of its own, as a
// terminal app runs it in a new window.
const recipeTerminal = "com.example.PortalTestTerm"

// withRecipeTerminal makes the world's portal run under recipeTerminal, and
// returns the directory its script leaves each window's command in.
func (w *portalWorld) withRecipeTerminal(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	requests := filepath.Join(dir, "requests")
	if err := os.Mkdir(requests, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "open-window.sh")
	body := "#!/bin/sh\nprintf '%s' \"$1\" > " + shellquote.Single(requests) + "/$$.tmp && mv " +
		shellquote.Single(requests) + "/$$.tmp " + shellquote.Single(requests) + "/$$.cmd\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	recipes, err := json.Marshal(map[string]any{recipeTerminal: map[string]any{"commands": map[string]any{"open": map[string]string{"script": script}}}})
	if err != nil {
		t.Fatal(err)
	}
	terminals := filepath.Join(dir, "terminals.json")
	if err := os.WriteFile(terminals, recipes, 0o600); err != nil {
		t.Fatal(err)
	}
	w.env = append(w.env, "PORTAL_TERMINALS_FILE="+terminals, "__CFBundleIdentifier="+recipeTerminal)
	return requests
}

// awaitWindow waits for the recipe to be asked for a window, and returns the
// command it was asked to run.
func awaitWindow(t *testing.T, requests string) string {
	t.Helper()
	var asked []string
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		asked, _ = filepath.Glob(filepath.Join(requests, "*.cmd"))
		return len(asked) > 0
	}) {
		t.Fatal("the burst never asked the terminal for a window")
	}
	if len(asked) != 1 {
		t.Fatalf("the terminal was asked for windows %q, want one", asked)
	}
	command, err := os.ReadFile(asked[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(command)
}

func TestOpenBurst_AWindowAcksThroughTheStateDirectory(t *testing.T) {
	binary := buildPortalBinary(t)
	w := newPortalWorld(t, binary)
	requests := w.withRecipeTerminal(t)
	w.create(t, "api-x7Kd9a")
	w.create(t, "blog-c3d4")

	opener := w.portal(t, "open", "-s", "api-x7Kd9a", "-s", "blog-c3d4")
	command := awaitWindow(t, requests)
	for _, want := range []string{"'-u' 'ZMX_SESSION'", "'open' '--session' 'blog-c3d4' '--ack' "} {
		if !strings.Contains(command, want) {
			t.Fatalf("the window runs %s; want it to carry %s", command, want)
		}
	}
	// A recipe runs as a child of its opener, so a window can inherit the
	// opener's session; the command strips it, so the window attaches in its
	// own terminal rather than switching that session.
	w.env = append(w.env, "ZMX_SESSION=api-x7Kd9a")
	window := w.start(t, "/bin/sh", "-c", command)

	w.awaitClients(t, "blog-c3d4", 1)
	w.awaitClients(t, "api-x7Kd9a", 1)
	window.awaitRunner(t, "blog-c3d4")
	opener.awaitRunner(t, "api-x7Kd9a")

	log := portaltest.ReadPortalLogSafe(w.stateDir)
	if !strings.Contains(log, "spawn: opened 2/2 ") || strings.Contains(log, "external window failed") {
		t.Errorf("the opener did not count the window's ack; portal.log:\n%s", log)
	}
	acks, err := os.ReadDir(filepath.Join(w.stateDir, "spawn-acks"))
	if err != nil || len(acks) != 0 {
		t.Errorf("spawn-acks holds %v, %v; want the burst's ack written and cleaned", acks, err)
	}
}

// A window's shell starts from the terminal's environment, not its opener's:
// a terminal app's login shell need not read the file that sets ZMX_DIR or
// Portal's paths. The window here is given none of the opener's — its own
// zmx and state directories are decoys, so a window that resolved by them
// would touch nothing real — and still attaches the opener's session and
// acks into the opener's state directory.
func TestOpenBurst_AWindowResolvesWhatItsOpenerDid(t *testing.T) {
	binary := buildPortalBinary(t)
	w := newPortalWorld(t, binary)
	requests := w.withRecipeTerminal(t)
	w.create(t, "api-x7Kd9a")
	w.create(t, "blog-c3d4")

	opener := w.portal(t, "open", "-s", "api-x7Kd9a", "-s", "blog-c3d4")
	command := awaitWindow(t, requests)
	decoyZmx, decoyState := t.TempDir(), t.TempDir()
	window := w.startIn(t, []string{
		"HOME=" + t.TempDir(),
		"PATH=/usr/bin:/bin",
		"TERM=xterm-256color",
		"ZMX_DIR=" + decoyZmx,
		"PORTAL_STATE_DIR=" + decoyState,
	}, "/bin/sh", "-c", command)

	window.awaitRunner(t, "blog-c3d4")
	opener.awaitRunner(t, "api-x7Kd9a")
	w.awaitClients(t, "blog-c3d4", 1)
	if log := portaltest.ReadPortalLogSafe(w.stateDir); !strings.Contains(log, "spawn: opened 2/2 ") {
		t.Errorf("the opener did not count the window's ack; portal.log:\n%s", log)
	}
	for _, decoy := range []string{decoyZmx, decoyState} {
		if entries, err := os.ReadDir(decoy); err != nil || len(entries) != 0 {
			t.Errorf("the window resolved by its own environment: %s holds %v, %v", decoy, entries, err)
		}
	}
}

func TestPickerMultiSelect_OpensTheOthersInWindowsAndAttachesTheLast(t *testing.T) {
	binary := buildPortalBinary(t)
	w := newPortalWorld(t, binary)
	requests := w.withRecipeTerminal(t)
	w.create(t, "api-x7Kd9a")
	w.create(t, "blog-c3d4")

	picker := w.portal(t, "open")
	picker.awaitAfter(t, 0, "blog-c3d4")
	// m marks the highlighted api-x7Kd9a as it enters multi-select; the second
	// m marks blog-c3d4.
	picker.typeIn(t, "m")
	picker.typeIn(t, "\x1b[B")
	picker.typeIn(t, "m")
	picker.typeIn(t, "\r")

	command := awaitWindow(t, requests)
	if !strings.Contains(command, "'open' '--session' 'api-x7Kd9a' '--ack' ") {
		t.Fatalf("the window runs %s; want it to open api-x7Kd9a", command)
	}
	window := w.start(t, "/bin/sh", "-c", command)

	window.awaitRunner(t, "api-x7Kd9a")
	picker.awaitRunner(t, "blog-c3d4")
	w.awaitClients(t, "api-x7Kd9a", 1)
	w.awaitClients(t, "blog-c3d4", 1)
	if log := portaltest.ReadPortalLogSafe(w.stateDir); !strings.Contains(log, "spawn: opened 2/2 ") {
		t.Errorf("the picker did not count the window's ack; portal.log:\n%s", log)
	}
}
