package migrate_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/nanoid"
)

// v1Index is sessions.json as the tmux version wrote it, every member of its
// schema included, so a fixture reads as one of its files does.
type v1Index struct {
	Version  int         `json:"version"`
	SavedAt  time.Time   `json:"saved_at"`
	Sessions []v1Session `json:"sessions"`
}

type v1Session struct {
	Name        string            `json:"name"`
	Environment map[string]string `json:"environment"`
	Windows     []v1Window        `json:"windows"`
}

type v1Window struct {
	Index  int      `json:"index"`
	Name   string   `json:"name"`
	Layout string   `json:"layout"`
	Zoomed bool     `json:"zoomed"`
	Active bool     `json:"active"`
	Panes  []v1Pane `json:"panes"`
}

type v1Pane struct {
	Index          int    `json:"index"`
	CWD            string `json:"cwd"`
	Active         bool   `json:"active"`
	CurrentCommand string `json:"current_command"`
	ScrollbackFile string `json:"scrollback_file"`
	PortalPaneID   string `json:"portal_pane_id"`
}

// savedAt is when the fixtures' tmux version last saved.
var savedAt = time.Date(2026, 11, 4, 22, 15, 3, 123456789, time.UTC)

// pane is a saved pane at index, working in cwd.
func pane(index int, cwd string) v1Pane {
	return v1Pane{Index: index, CWD: cwd, Active: index == 0, CurrentCommand: "zsh"}
}

func (p v1Pane) token(token string) v1Pane {
	p.PortalPaneID = token
	return p
}

func (p v1Pane) scrollback(file string) v1Pane {
	p.ScrollbackFile = file
	return p
}

// window is a saved window at index holding panes.
func window(index int, panes ...v1Pane) v1Window {
	return v1Window{Index: index, Name: "zsh", Layout: `{"V":2,"L":{"t":"p","w":80,"h":24,"x":0,"y":0}}`, Active: index == 0, Panes: panes}
}

// session is a saved tmux session of one window, index 0, holding panes.
func session(name string, panes ...v1Pane) v1Session {
	return sessionOf(name, window(0, panes...))
}

func sessionOf(name string, windows ...v1Window) v1Session {
	return v1Session{Name: name, Environment: map[string]string{"TERM": "xterm-256color"}, Windows: windows}
}

// legacyDir is a fresh tmux version's state dir holding sessions.json, saving
// sessions, and each of files, by its name relative to the dir, holding its
// content.
func legacyDir(t *testing.T, sessions []v1Session, files map[string]string) string {
	t.Helper()
	data, err := json.MarshalIndent(v1Index{Version: 1, SavedAt: savedAt, Sessions: sessions}, "", "  ")
	if err != nil {
		t.Fatalf("encode sessions.json: %v", err)
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sessions.json"), string(data))
	for name, content := range files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), content)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("make %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readLegacy(t *testing.T, dir string) migrate.Legacy {
	t.Helper()
	legacy, err := migrate.ReadLegacy(dir)
	if err != nil {
		t.Fatalf("ReadLegacy: %v", err)
	}
	return legacy
}

// zmxDir is the zmx dir the fixtures' names are judged against, which sets
// how long a name may be. Nothing reads or writes it.
const zmxDir = "/home/user/.local/state/zmx"

func zmxBackend(t *testing.T) *zmx.Backend {
	t.Helper()
	b, err := zmx.New(zmx.Config{Dir: zmxDir})
	if err != nil {
		t.Fatalf("bind a zmx backend: %v", err)
	}
	return b
}

// mints is a generator minting Gen001, Gen002 and so on.
func mints() nanoid.Generator {
	n := 0
	return func() (string, error) {
		n++
		return fmt.Sprintf("Gen%03d", n), nil
	}
}

// codeDir holds the fixtures' git repositories, one per directory below it.
const codeDir = "/home/user/code/"

// repoRoot stands in for session.GroupingDir: a working directory below
// codeDir groups under the repository holding it, and any other under none.
func repoRoot(cwd string) (string, bool) {
	rest, ok := strings.CutPrefix(cwd, codeDir)
	if !ok || rest == "" {
		return "", false
	}
	repo, _, _ := strings.Cut(rest, "/")
	return codeDir + repo, true
}

// naming judges names as the zmx backend does over zmxDir, with taken held
// already, minting from mints, grouping by repoRoot.
func naming(t *testing.T, taken ...string) migrate.Naming {
	t.Helper()
	return migrate.Naming{Taken: taken, Validate: zmxBackend(t).ValidateName, Mint: mints(), GroupingDir: repoRoot}
}

// planOf plans migrating the tmux version's state dir dir with hookKeys under
// n, failing the test on an error.
func planOf(t *testing.T, dir string, hookKeys []string, n migrate.Naming) migrate.Plan {
	t.Helper()
	plan, err := migrate.NewPlan(readLegacy(t, dir), hookKeys, n)
	if err != nil {
		t.Fatalf("NewPlan: %v", err)
	}
	return plan
}

// planSessions plans migrating sessions, with no hooks and nothing taken.
func planSessions(t *testing.T, sessions ...v1Session) migrate.Plan {
	t.Helper()
	return planOf(t, legacyDir(t, sessions, nil), nil, naming(t))
}

func namesOf(plan migrate.Plan) []string {
	names := make([]string, 0, len(plan.Sessions))
	for _, s := range plan.Sessions {
		names = append(names, s.Name)
	}
	return names
}

func labelsOf(plan migrate.Plan) []string {
	labels := make([]string, 0, len(plan.Sessions))
	for _, s := range plan.Sessions {
		labels = append(labels, s.Label)
	}
	return labels
}
