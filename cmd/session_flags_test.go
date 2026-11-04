package cmd

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/state"
)

// stageFlags points the state directory at a fresh one holding flag raised
// for each of names.
func stageFlags(t *testing.T, flag state.SessionFlag, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	for _, name := range names {
		if err := flag.Set(dir, name); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func flagNames(t *testing.T, read func() (map[string]struct{}, error)) []string {
	t.Helper()
	raised, err := read()
	if err != nil {
		t.Fatalf("read flags: %v", err)
	}
	return slices.Sorted(maps.Keys(raised))
}

func TestPickerFlagReaders(t *testing.T) {
	t.Run("the dots read the resume-pending flags, and the settle the hydrating ones", func(t *testing.T) {
		dir := stageFlags(t, state.ResumePending, "alpha-a1b2")
		if err := state.Hydrating.Set(dir, "bravo-c3d4"); err != nil {
			t.Fatal(err)
		}

		pending, hydrating := pickerFlagReaders()

		if got := flagNames(t, pending.ListPendingResumeSessions); !slices.Equal(got, []string{"alpha-a1b2"}) {
			t.Errorf("pending = %q, want [alpha-a1b2]", got)
		}
		if got := flagNames(t, hydrating.ListHydratingSessions); !slices.Equal(got, []string{"bravo-c3d4"}) {
			t.Errorf("hydrating = %q, want [bravo-c3d4]", got)
		}
	})

	t.Run("a state dir not made yet raises no flag", func(t *testing.T) {
		t.Setenv("PORTAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))

		pending, hydrating := pickerFlagReaders()

		if got := flagNames(t, pending.ListPendingResumeSessions); len(got) != 0 {
			t.Errorf("pending = %q, want none", got)
		}
		if got := flagNames(t, hydrating.ListHydratingSessions); len(got) != 0 {
			t.Errorf("hydrating = %q, want none", got)
		}
	})

	t.Run("a flag directory that can't be read is an error, never no flag", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads through a directory's permissions")
		}
		dir := stageFlags(t, state.ResumePending, "alpha-a1b2")
		flags := filepath.Join(dir, string(state.ResumePending))
		if err := os.Chmod(flags, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(flags, 0o700) })

		pending, _ := pickerFlagReaders()

		if raised, err := pending.ListPendingResumeSessions(); err == nil {
			t.Errorf("pending = %v, want an error", raised)
		}
	})
}

// waitingRows is the picker's rows once its first listing lands, read
// through pickerFlagReaders.
func waitingRows(t *testing.T, noColor bool, sessions ...backend.Session) string {
	t.Helper()
	cfg := defaultTestTUIConfig()
	cfg.lister = fakePickerLister(sessions...)
	cfg.noColor = noColor
	cfg.pendingReader, cfg.hydrating = pickerFlagReaders()
	m := update(t, buildTUIModel(cfg, pickerLanding{}, nil), tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, msg := range drainCmd(t, m.Init()) {
		m = update(t, m, msg)
	}
	return ansi.Strip(m.View().Content)
}

func rowOf(t *testing.T, view, name string) string {
	t.Helper()
	for line := range strings.Lines(view) {
		if strings.Contains(line, name) {
			return strings.TrimRight(line, " \n")
		}
	}
	t.Fatalf("no row for %s in:\n%s", name, view)
	return ""
}

func TestPicker_DotsTheLiveSessionsWaitingOnTheirPanel(t *testing.T) {
	for _, tc := range []struct {
		name    string
		noColor bool
		dot     string
	}{
		{"in colour, by the dot", false, "●"},
		{"under NO_COLOR, by its letter", true, "P"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stageFlags(t, state.ResumePending, "blog-c3d4", "gone-e5f6")

			view := waitingRows(t, tc.noColor, backend.Session{Name: "portal-a1b2"}, backend.Session{Name: "blog-c3d4"})

			if row := rowOf(t, view, "blog-c3d4"); !strings.HasSuffix(row, tc.dot) {
				t.Errorf("the waiting session's row ends %q, want %q: %q", row[max(0, len(row)-4):], tc.dot, row)
			}
			if row := rowOf(t, view, "portal-a1b2"); strings.HasSuffix(row, tc.dot) {
				t.Errorf("a session not waiting carries the dot: %q", row)
			}
			if strings.Contains(view, "gone-e5f6") {
				t.Errorf("a flag whose session has ended got a row:\n%s", view)
			}
		})
	}
}
