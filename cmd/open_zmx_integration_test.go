//go:build integration

package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/zmxtest"
)

func TestPortalOpenPathAgainstZmx(t *testing.T) {
	env, stateDir := portaltest.IsolateStateForTest(t)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	b := zmxtest.New(t)
	binary := buildPortalBinary(t)

	workDir := filepath.Join(realPath(t, t.TempDir()), "myproj")
	if err := os.Mkdir(workDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Stdin at EOF, so the attach detaches as soon as it connects. TMUX stays
	// poisoned: opening a session must not reach tmux at all.
	cmd := exec.Command(binary, "open", "-p", workDir)
	cmd.Env = append(env,
		"ZMX_DIR="+b.Dir(),
		"PATH="+filepath.Dir(zmxtest.Binary(t))+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PORTAL_PROJECTS_FILE="+filepath.Join(t.TempDir(), "projects.json"),
		"SHELL=/bin/sh",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("portal open -p %s: %v\n%s", workDir, err, out)
	}

	sessions, err := b.List(t.Context())
	if err != nil {
		t.Fatalf("list %s: %v", b.Dir(), err)
	}
	// The bootstrap's saver is the other.
	sessions = slices.DeleteFunc(sessions, backend.Session.PortalOwned)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %+v, want the one portal open made", sessions)
	}
	made := sessions[0]

	t.Run("it creates a zmx session named for the project, in the directory", func(t *testing.T) {
		if !strings.HasPrefix(made.Name, "myproj-") {
			t.Errorf("name = %q, want myproj-<nanoid>", made.Name)
		}
		if made.Dir != workDir {
			t.Errorf("dir = %q, want %q", made.Dir, workDir)
		}
	})

	t.Run("it records the session in the state dir's index with zmx's created time", func(t *testing.T) {
		idx, err := sessionindex.Read(stateDir)
		if err != nil {
			t.Fatalf("read index: %v", err)
		}
		entry, ok := idx.Lookup(made.Name)
		if !ok {
			t.Fatalf("index = %+v, want an entry for %s", idx.Entries(), made.Name)
		}
		if entry.Dir != workDir || !entry.CreatedAt.Equal(made.Created) || entry.CreatedAt.IsZero() {
			t.Errorf("entry = %+v, want dir %s and created %v", entry, workDir, made.Created)
		}
	})
}
