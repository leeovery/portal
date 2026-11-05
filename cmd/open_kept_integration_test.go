//go:build integration

package cmd_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hookstest"
)

// keptLine is the row the kept session's saved transcript ends on.
const keptLine = "kept for the next restore"

// Decision 78: x <name> brings back a kept session, one the next restore
// would, under bootstrap.lock and through restore's own path: its transcript
// replayed and its eager hook run, with no bootstrap.
func TestOpenBringsBackAKeptSessionAgainstZmx(t *testing.T) {
	w, ended := loggedOut(t, 1)
	name := ended[0]
	// zmx refuses to make a session whose socket path holds anything but a
	// socket, so the bootstrap's restore fails and keeps the entry, owed.
	blocker := filepath.Join(w.dir.Backend.Dir(), name)
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if out := w.portal(t, "list", "--short"); out != "" {
		t.Fatalf("portal list = %q, want nothing listed, the restore failing", out)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	saved, held := w.entry(name)
	if !held || !saved.Owed || saved.ScrollbackFile == "" {
		t.Fatalf("entry for %s = %+v, %v; want it kept, owed, naming its transcript", name, saved, held)
	}
	transcript := filepath.Join(w.dir.StateDir, filepath.FromSlash(saved.ScrollbackFile))
	if err := os.WriteFile(transcript, []byte("$ make\r\n"+keptLine+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hookMark := filepath.Join(t.TempDir(), "hook-ran")
	hooksFile := hookstest.ResolveHooksFilePathFromEnv(t, w.env)
	hookstest.StageStore(t, hookstest.Staging{
		Dir:  filepath.Dir(hooksFile),
		Seed: `{"` + name + `": {"on-resume": {"command": "touch ` + hookMark + `", "resume": "eager"}}}`,
	})
	bootstraps := w.bootstraps()

	if out, err := w.command("open", name).CombinedOutput(); err != nil {
		t.Fatalf("portal open %s: %v\n%s\n--- portal.log ---\n%s", name, err, out, w.log())
	}

	if got := w.userSessions(t); !slices.Equal(got, []string{name}) {
		t.Fatalf("sessions = %v, want %s brought back", got, name)
	}
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		_, err := os.Stat(hookMark)
		return err == nil
	}) {
		t.Fatalf("%s's eager hook never ran\n--- portal.log ---\n%s", name, w.log())
	}
	var history string
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		raw, err := w.dir.Backend.History(t.Context(), name, 0)
		history = ansi.Strip(string(raw))
		return err == nil && strings.Contains(history, keptLine)
	}) {
		t.Fatalf("%s's history = %q, want its transcript replayed", name, history)
	}
	w.awaitNoFlags(t)
	if e, held := w.entry(name); !held || e.Owed {
		t.Errorf("entry for %s = %+v, %v once its helper finished; want it held, owed no longer", name, e, held)
	}
	if log := w.log(); !strings.Contains(log, "restore: restore complete session="+name+" restored=1 failed=0 ") {
		t.Errorf("portal.log records no bring-back of %s\n--- portal.log ---\n%s", name, log)
	}
	if n := w.bootstraps(); n != bootstraps {
		t.Errorf("bootstraps = %d, want %d: bringing a kept session back runs none", n, bootstraps)
	}
}
