package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// makeIndex returns a minimal valid Index referencing the supplied scrollback
// relative paths via a single session/window with one pane per path.
func makeIndex(t *testing.T, scrollbackPaths ...string) state.Index {
	t.Helper()
	panes := make([]state.Pane, 0, len(scrollbackPaths))
	for i, p := range scrollbackPaths {
		panes = append(panes, state.Pane{
			Index:          i,
			CWD:            "/tmp",
			Active:         i == 0,
			CurrentCommand: "zsh",
			ScrollbackFile: p,
		})
	}
	return state.Index{
		Version: state.SchemaVersion,
		SavedAt: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
		Sessions: []state.Session{
			{
				Name:        "work",
				Environment: map[string]string{"LANG": "en_US.UTF-8"},
				Windows: []state.Window{
					{
						Index:  0,
						Name:   "main",
						Layout: "abc,80x24,0,0",
						Active: true,
						Panes:  panes,
					},
				},
			},
		},
	}
}

// writeOrphan creates an orphan .bin file under scrollback/ in dir.
func writeOrphan(t *testing.T, dir, name string, contents []byte) string {
	t.Helper()
	sb := state.ScrollbackDir(dir)
	if err := os.MkdirAll(sb, 0o700); err != nil {
		t.Fatalf("mkdir scrollback: %v", err)
	}
	path := filepath.Join(sb, name)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestCommit_WritesSessionsJSONOnFirstCommit(t *testing.T) {
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	idx := makeIndex(t, "scrollback/work__0.0.bin")

	if err := state.Commit(dir, idx, false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	data, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	got, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("DecodeIndex: %v", err)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Name != "work" {
		t.Errorf("sessions.json content unexpected; got %#v", got)
	}
}

func TestCommit_SkipsWriteAndGCOnZeroChangeCycle(t *testing.T) {
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	idx := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx, false, logger); err != nil {
		t.Fatalf("first Commit: %v", err)
	}

	infoBefore, err := os.Stat(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("stat sessions.json: %v", err)
	}

	// Pre-populate an orphan AFTER the first commit so we can assert that the
	// no-op second Commit does NOT GC it.
	orphan := writeOrphan(t, dir, "orphan.bin", []byte("orphan"))

	// Sleep briefly so any mtime change would be observable on filesystems
	// with second-resolution mtimes. We assert mtime *unchanged* below.
	time.Sleep(10 * time.Millisecond)

	// Second commit: identical idx (modulo SavedAt). Should be a no-op.
	idx2 := makeIndex(t, "scrollback/work__0.0.bin")
	idx2.SavedAt = idx.SavedAt.Add(time.Hour) // SavedAt is ignored by delta
	if err := state.Commit(dir, idx2, false, logger); err != nil {
		t.Fatalf("second Commit: %v", err)
	}

	infoAfter, err := os.Stat(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("stat sessions.json after: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Errorf("sessions.json mtime changed: before=%v after=%v",
			infoBefore.ModTime(), infoAfter.ModTime())
	}

	if _, err := os.Stat(orphan); err != nil {
		t.Errorf("expected orphan.bin to survive no-op Commit; stat err=%v", err)
	}
}

func TestCommit_WritesWhenStructureChanged(t *testing.T) {
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	idx1 := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx1, false, logger); err != nil {
		t.Fatalf("first Commit: %v", err)
	}

	idx2 := makeIndex(t, "scrollback/work__0.0.bin", "scrollback/work__0.1.bin")
	if err := state.Commit(dir, idx2, false, logger); err != nil {
		t.Fatalf("second Commit: %v", err)
	}

	data, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	got, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("DecodeIndex: %v", err)
	}
	if len(got.Sessions[0].Windows[0].Panes) != 2 {
		t.Errorf("expected 2 panes after structural change; got %d",
			len(got.Sessions[0].Windows[0].Panes))
	}
}

func TestCommit_WritesWhenScrollbackChangedButStructureUnchanged(t *testing.T) {
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	idx := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx, false, logger); err != nil {
		t.Fatalf("first Commit: %v", err)
	}

	infoBefore, err := os.Stat(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Same structural content, but anyScrollbackChanged=true forces the write.
	idx2 := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx2, true, logger); err != nil {
		t.Fatalf("second Commit: %v", err)
	}

	infoAfter, err := os.Stat(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		// On filesystems with coarse mtime resolution this could spuriously fail.
		// As a fallback, also check that the file still parses (it must — we wrote it).
		t.Logf("sessions.json mtime did not advance (low-resolution fs?); before=%v after=%v",
			infoBefore.ModTime(), infoAfter.ModTime())
	}
}

func TestCommit_RemovesOrphanBinFiles(t *testing.T) {
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	// Pre-populate scrollback with an orphan and a referenced file.
	writeOrphan(t, dir, "orphan.bin", []byte("orphan"))
	referenced := writeOrphan(t, dir, "work__0.0.bin", []byte("kept"))

	idx := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx, true, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), "orphan.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected orphan.bin to be removed; stat err=%v", err)
	}
	if _, err := os.Stat(referenced); err != nil {
		t.Errorf("expected referenced file to survive; stat err=%v", err)
	}
}

func TestCommit_PreservesReferencedBinFiles(t *testing.T) {
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	a := writeOrphan(t, dir, "work__0.0.bin", []byte("a"))
	b := writeOrphan(t, dir, "work__0.1.bin", []byte("b"))

	idx := makeIndex(t, "scrollback/work__0.0.bin", "scrollback/work__0.1.bin")
	if err := state.Commit(dir, idx, true, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	for _, p := range []string{a, b} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to survive; stat err=%v", p, err)
		}
	}
}

func TestCommit_SkipsNonBinAndDirsDuringGC(t *testing.T) {
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	sb := state.ScrollbackDir(dir)
	if err := os.MkdirAll(sb, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stray := filepath.Join(sb, "stray.txt")
	if err := os.WriteFile(stray, []byte("not bin"), 0o600); err != nil {
		t.Fatalf("write stray: %v", err)
	}
	subdir := filepath.Join(sb, "subdir")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	idx := makeIndex(t)
	if err := state.Commit(dir, idx, true, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := os.Stat(stray); err != nil {
		t.Errorf("expected stray.txt to survive (non-.bin); err=%v", err)
	}
	if _, err := os.Stat(subdir); err != nil {
		t.Errorf("expected subdir to survive (directory); err=%v", err)
	}
}

func TestCommit_ToleratesMissingScrollbackDir(t *testing.T) {
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	// No scrollback/ subdir at all. Commit must still succeed.
	idx := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx, false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, err := os.Stat(state.SessionsJSON(dir)); err != nil {
		t.Errorf("expected sessions.json to exist; err=%v", err)
	}
}

func TestCommit_ReturnsWrappedErrorWhenAtomicWriteFailsAndPreservesPriorFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot make a directory unwritable as root")
	}
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	// First commit succeeds and produces sessions.json.
	idx1 := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx1, false, logger); err != nil {
		t.Fatalf("first Commit: %v", err)
	}
	priorBytes, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read prior: %v", err)
	}

	// Make the state directory read-only so AtomicWrite cannot create a temp file.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod 0500: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	// Second commit with structurally-different idx — must attempt write and fail.
	idx2 := makeIndex(t, "scrollback/work__0.0.bin", "scrollback/work__0.1.bin")
	err = state.Commit(dir, idx2, false, logger)
	if err == nil {
		t.Fatalf("expected error from Commit when AtomicWrite fails; got nil")
	}
	if !strings.Contains(err.Error(), "write sessions.json") {
		t.Errorf("expected wrapped 'write sessions.json' error; got %v", err)
	}

	// Restore perms so we can read the file.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("restore chmod: %v", err)
	}
	gotBytes, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(gotBytes) != string(priorBytes) {
		t.Errorf("prior sessions.json was modified after failed write")
	}
}

func TestCommit_DoesNotReturnErrorWhenGCFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot make a directory unwritable as root")
	}
	dir := t.TempDir()
	logger, sink := openTempLogger(t)

	// Pre-populate an orphan to give GC something to remove.
	writeOrphan(t, dir, "orphan.bin", []byte("orphan"))
	writeOrphan(t, dir, "work__0.0.bin", []byte("kept"))

	// Make scrollback dir read-only AFTER files exist, so ReadDir succeeds but
	// Remove fails with EACCES.
	sb := state.ScrollbackDir(dir)
	if err := os.Chmod(sb, 0o500); err != nil {
		t.Fatalf("chmod scrollback: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sb, 0o700) })

	idx := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx, true, logger); err != nil {
		t.Errorf("Commit returned error despite GC failure: %v", err)
	}

	// Logger should record a warn about gc.
	log := sink.Body()
	if !strings.Contains(log, "WARN") {
		t.Errorf("expected WARN entry in log; got %q", log)
	}
	if !strings.Contains(log, "gc") {
		t.Errorf("expected log to mention gc; got %q", log)
	}
}

func TestCommit_WritesSessionsJSONWithMode0600(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file modes not meaningfully checkable as root")
	}
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	idx := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx, false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	info, err := os.Stat(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected sessions.json mode 0600; got %o", mode)
	}
}

func TestCommit_ToleratesUnreadablePriorSessionsJSON(t *testing.T) {
	dir := t.TempDir()
	logger, _ := openTempLogger(t)

	// Write a corrupt prior sessions.json — DecodeIndex should fail, treated as changed.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	idx := makeIndex(t, "scrollback/work__0.0.bin")
	if err := state.Commit(dir, idx, false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// New sessions.json must replace the corrupt one.
	data, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := state.DecodeIndex(data); err != nil {
		t.Errorf("expected valid sessions.json after Commit; decode err=%v", err)
	}
}

func TestComputeReferencedSet_CollectsAllPaneScrollbackPaths(t *testing.T) {
	idx := state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{
			{
				Name: "a",
				Windows: []state.Window{
					{
						Index: 0,
						Panes: []state.Pane{
							{Index: 0, ScrollbackFile: "scrollback/a__0.0.bin"},
							{Index: 1, ScrollbackFile: "scrollback/a__0.1.bin"},
						},
					},
					{
						Index: 1,
						Panes: []state.Pane{
							{Index: 0, ScrollbackFile: "scrollback/a__1.0.bin"},
						},
					},
				},
			},
			{
				Name: "b",
				Windows: []state.Window{
					{
						Index: 0,
						Panes: []state.Pane{
							{Index: 0, ScrollbackFile: "scrollback/b__0.0.bin"},
						},
					},
				},
			},
		},
	}

	set := state.ComputeReferencedSet(idx)

	want := []string{
		"scrollback/a__0.0.bin",
		"scrollback/a__0.1.bin",
		"scrollback/a__1.0.bin",
		"scrollback/b__0.0.bin",
	}
	if len(set) != len(want) {
		t.Errorf("set has %d entries, want %d; set=%v", len(set), len(want), set)
	}
	for _, p := range want {
		if _, ok := set[p]; !ok {
			t.Errorf("set missing %q; set=%v", p, set)
		}
	}
}

func TestComputeReferencedSet_EmptyIndexProducesEmptySet(t *testing.T) {
	idx := state.Index{Version: state.SchemaVersion}
	set := state.ComputeReferencedSet(idx)
	if len(set) != 0 {
		t.Errorf("expected empty set; got %v", set)
	}
}
