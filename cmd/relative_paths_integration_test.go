//go:build integration

package cmd_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/state"
)

// A relative path from the environment means what it means where Portal was
// run: the saver's daemon, which runs in the state directory, inherits it
// made absolute, so it saves into the state Portal resolved and reads the
// hooks.json Portal named.
func TestRelativePathsReachTheSaverAbsoluteAgainstZmx(t *testing.T) {
	w := newBootWorld(t)
	portalDir := filepath.Dir(w.dir.StateDir)
	hooksFile := filepath.Join(portalDir, "hooks.json")
	list := w.command("list", "--short")
	list.Dir = portalDir
	list.Env = append(slices.Clone(w.env), "PORTAL_STATE_DIR="+filepath.Base(w.dir.StateDir), "PORTAL_HOOKS_FILE=hooks.json")

	if out, err := list.CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("portal list = %v, printing %q; want it quiet\n--- portal.log ---\n%s", err, out, w.log())
	}

	pid := w.saverPID(t)
	if recorded, err := state.ReadPIDFile(w.dir.StateDir); recorded != pid || err != nil {
		t.Errorf("daemon.pid in %s = %d, %v; want the saver's daemon, %d", w.dir.StateDir, recorded, err, pid)
	}
	args, err := proctable.ReadArgs(pid)
	if err != nil {
		t.Fatalf("read the saver's daemon: %v", err)
	}
	for name, want := range map[string]string{"PORTAL_STATE_DIR": w.dir.StateDir, "PORTAL_HOOKS_FILE": hooksFile} {
		values := envValues(args.Env, name)
		if len(values) == 0 {
			t.Errorf("the saver's daemon has no %s", name)
		}
		for _, got := range values {
			if !filepath.IsAbs(got) || zmx.CanonicalDir(got) != zmx.CanonicalDir(want) {
				t.Errorf("the saver's daemon has %s=%s, want %s", name, got, want)
			}
		}
	}
}

// envValues is every value environ sets name to.
func envValues(environ []string, name string) []string {
	var values []string
	for _, kv := range environ {
		if value, ok := strings.CutPrefix(kv, name+"="); ok {
			values = append(values, value)
		}
	}
	return values
}
