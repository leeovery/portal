package tui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// Compile-time assertions that the production seams remain satisfied. These
// are duplicated here (alongside the package-level assertions in
// preview_adapter.go) so a regression in either direction trips the test
// build immediately.
var (
	_ TmuxEnumerator   = (*tmux.Client)(nil)
	_ ScrollbackReader = scrollbackReaderAdapter{}
)

// writeBinFile is a small helper that writes a known scrollback `.bin` file
// for paneKey under stateDir's scrollback subdirectory, creating intermediate
// dirs as needed. It uses 0o644 so the default-readable file shape mirrors
// production.
func writeBinFile(t *testing.T, stateDir, paneKey string, content []byte) string {
	t.Helper()
	dir := filepath.Join(stateDir, "scrollback")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir scrollback: %v", err)
	}
	path := state.ScrollbackFile(stateDir, paneKey)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestScrollbackReaderAdapter_TailReturnsBytesForValidPaneKey(t *testing.T) {
	stateDir := t.TempDir()
	paneKey := state.SanitizePaneKey("work", 0, 1)
	// Two complete lines plus a trailing line — TailScrollback returns
	// every terminated line when fewer than n exist, so the entire content
	// (through the final '\n') is the expected output.
	content := []byte("line-one\nline-two\nline-three\n")
	writeBinFile(t, stateDir, paneKey, content)

	adapter := scrollbackReaderAdapter{stateDir: stateDir, n: previewTailLines}
	got, err := adapter.Tail(paneKey)
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Tail bytes = %q, want %q", got, content)
	}
}

func TestScrollbackReaderAdapter_TailReturnsNilNilForMissingBin(t *testing.T) {
	stateDir := t.TempDir()
	adapter := scrollbackReaderAdapter{stateDir: stateDir, n: previewTailLines}

	got, err := adapter.Tail("nonexistent-pane-key")

	if err != nil {
		t.Errorf("expected nil error for missing file, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil bytes for missing file, got %q", got)
	}
}

func TestScrollbackReaderAdapter_TailReturnsErrForPermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode 0o000 has no equivalent on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses unix file permissions; skipping permission-denied test")
	}

	stateDir := t.TempDir()
	paneKey := state.SanitizePaneKey("work", 0, 0)
	path := writeBinFile(t, stateDir, paneKey, []byte("payload\n"))
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod 0o000: %v", err)
	}
	t.Cleanup(func() {
		// Restore mode so TempDir cleanup can remove the file.
		_ = os.Chmod(path, 0o600)
	})

	adapter := scrollbackReaderAdapter{stateDir: stateDir, n: previewTailLines}
	got, err := adapter.Tail(paneKey)

	if err == nil {
		t.Fatalf("expected non-nil error for permission-denied read, got nil (bytes=%q)", got)
	}
	if got != nil {
		t.Errorf("expected nil bytes on permission-denied error, got %q", got)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("expected error to wrap fs.ErrPermission, got %v", err)
	}
}

func TestPreviewTailLinesIsOneThousand(t *testing.T) {
	if previewTailLines != 1000 {
		t.Errorf("previewTailLines = %d, want 1000", previewTailLines)
	}
}
