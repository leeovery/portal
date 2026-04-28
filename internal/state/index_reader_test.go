package state_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

func writeSessionsJSON(t *testing.T, dir string, data []byte) {
	t.Helper()
	if err := os.WriteFile(state.SessionsJSON(dir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

func TestReadIndex_ReturnsSkipWithoutErrorWhenFileAbsent(t *testing.T) {
	dir := t.TempDir()

	idx, skip, err := state.ReadIndex(dir)
	if err != nil {
		t.Fatalf("expected nil error; got %v", err)
	}
	if !skip {
		t.Errorf("expected skip=true; got false")
	}
	if len(idx.Sessions) != 0 || idx.Version != 0 {
		t.Errorf("expected zero-value Index; got %#v", idx)
	}
}

func TestReadIndex_ReturnsParsedIndexForValidV1File(t *testing.T) {
	dir := t.TempDir()
	original := state.Index{
		Version: state.SchemaVersion,
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
		Sessions: []state.Session{
			{
				Name:        "work",
				Environment: map[string]string{"LANG": "en_US.UTF-8"},
				Windows: []state.Window{
					{
						Index:  0,
						Name:   "main",
						Layout: "abcd,80x24,0,0",
						Active: true,
						Panes: []state.Pane{
							{
								Index:          0,
								CWD:            "/tmp",
								Active:         true,
								CurrentCommand: "zsh",
								ScrollbackFile: "scrollback/work__0.0.bin",
							},
						},
					},
				},
			},
		},
	}
	data, err := state.EncodeIndex(original)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	writeSessionsJSON(t, dir, data)

	idx, skip, err := state.ReadIndex(dir)
	if err != nil {
		t.Fatalf("expected nil error; got %v", err)
	}
	if skip {
		t.Errorf("expected skip=false for valid v1 file; got true")
	}
	if idx.Version != state.SchemaVersion {
		t.Errorf("expected version=%d; got %d", state.SchemaVersion, idx.Version)
	}
	if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "work" {
		t.Errorf("expected one session named work; got %#v", idx.Sessions)
	}
	if !idx.SavedAt.Equal(original.SavedAt) {
		t.Errorf("saved_at not preserved: got %v; want %v", idx.SavedAt, original.SavedAt)
	}
}

func TestReadIndex_ReturnsSkipWithParseErrorForTruncatedJSON(t *testing.T) {
	dir := t.TempDir()
	writeSessionsJSON(t, dir, []byte("{...} broken"))

	_, skip, err := state.ReadIndex(dir)
	if err == nil {
		t.Fatalf("expected parse error; got nil")
	}
	if !skip {
		t.Errorf("expected skip=true on parse error; got false")
	}
	if !strings.Contains(err.Error(), "parse sessions.json") {
		t.Errorf("expected error wrapped with 'parse sessions.json'; got %v", err)
	}
}

func TestReadIndex_ReturnsSkipWithParseErrorForNonJSONContent(t *testing.T) {
	dir := t.TempDir()
	writeSessionsJSON(t, dir, []byte("not json at all"))

	_, skip, err := state.ReadIndex(dir)
	if err == nil {
		t.Fatalf("expected parse error; got nil")
	}
	if !skip {
		t.Errorf("expected skip=true on parse error; got false")
	}
	if !strings.Contains(err.Error(), "parse sessions.json") {
		t.Errorf("expected error wrapped with 'parse sessions.json'; got %v", err)
	}
}

func TestReadIndex_ReturnsSkipWithVersionErrorWhenSchemaVersionExceedsCurrent(t *testing.T) {
	dir := t.TempDir()
	writeSessionsJSON(t, dir, []byte(`{
  "version": 2,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": []
}`))

	_, skip, err := state.ReadIndex(dir)
	if err == nil {
		t.Fatalf("expected version error; got nil")
	}
	if !skip {
		t.Errorf("expected skip=true on version error; got false")
	}
	if !strings.Contains(err.Error(), "parse sessions.json") {
		t.Errorf("expected error wrapped with 'parse sessions.json'; got %v", err)
	}
	if !strings.Contains(err.Error(), "unsupported sessions.json version") {
		t.Errorf("expected error to mention unsupported version; got %v", err)
	}
	if !strings.Contains(err.Error(), "2") {
		t.Errorf("expected error to mention the offending version; got %v", err)
	}
}

func TestReadIndex_ReturnsSkipWithVersionErrorWhenVersionIsZero(t *testing.T) {
	dir := t.TempDir()
	writeSessionsJSON(t, dir, []byte(`{
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": []
}`))

	_, skip, err := state.ReadIndex(dir)
	if err == nil {
		t.Fatalf("expected version error; got nil")
	}
	if !skip {
		t.Errorf("expected skip=true on missing-version error; got false")
	}
	if !strings.Contains(err.Error(), "parse sessions.json") {
		t.Errorf("expected error wrapped with 'parse sessions.json'; got %v", err)
	}
	if !strings.Contains(err.Error(), "missing version") {
		t.Errorf("expected error to mention missing version; got %v", err)
	}
}

func TestReadIndex_ToleratesUnknownFieldsInValidV1Document(t *testing.T) {
	dir := t.TempDir()
	writeSessionsJSON(t, dir, []byte(`{
  "version": 1,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": [],
  "future_field": "ignored",
  "another": {"nested": 42}
}`))

	idx, skip, err := state.ReadIndex(dir)
	if err != nil {
		t.Fatalf("expected nil error; got %v", err)
	}
	if skip {
		t.Errorf("expected skip=false; got true")
	}
	if idx.Version != state.SchemaVersion {
		t.Errorf("expected version=%d; got %d", state.SchemaVersion, idx.Version)
	}
}

func TestReadIndex_HandlesEmptySessionsArray(t *testing.T) {
	dir := t.TempDir()
	writeSessionsJSON(t, dir, []byte(`{
  "version": 1,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": []
}`))

	idx, skip, err := state.ReadIndex(dir)
	if err != nil {
		t.Fatalf("expected nil error; got %v", err)
	}
	if skip {
		t.Errorf("expected skip=false; got true")
	}
	if len(idx.Sessions) != 0 {
		t.Errorf("expected empty sessions slice; got %#v", idx.Sessions)
	}
}

func TestReadIndex_ReturnsSkipWithWrappedPermissionErrorWhenUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode 0o000 does not block reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}

	dir := t.TempDir()
	path := state.SessionsJSON(dir)
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod 0o000: %v", err)
	}
	t.Cleanup(func() {
		// Restore mode so t.TempDir cleanup can remove the file.
		_ = os.Chmod(path, 0o600)
	})

	_, skip, err := state.ReadIndex(dir)
	if err == nil {
		t.Fatalf("expected read error; got nil")
	}
	if !skip {
		t.Errorf("expected skip=true on read error; got false")
	}
	if !strings.Contains(err.Error(), "read sessions.json") {
		t.Errorf("expected error wrapped with 'read sessions.json'; got %v", err)
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("permission error must not be reported as fs.ErrNotExist; got %v", err)
	}
}

func TestReadIndex_WrapsParseErrorWithErrCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	writeSessionsJSON(t, dir, []byte("{not json"))

	_, _, err := state.ReadIndex(dir)
	if err == nil {
		t.Fatal("expected parse error; got nil")
	}
	if !errors.Is(err, state.ErrCorruptIndex) {
		t.Errorf("errors.Is(err, ErrCorruptIndex) = false; want true. err=%v", err)
	}
}

func TestReadIndex_WrapsVersionErrorWithErrCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	writeSessionsJSON(t, dir, []byte(`{
  "version": 99,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": []
}`))

	_, _, err := state.ReadIndex(dir)
	if err == nil {
		t.Fatal("expected version error; got nil")
	}
	if !errors.Is(err, state.ErrCorruptIndex) {
		t.Errorf("errors.Is(err, ErrCorruptIndex) = false; want true. err=%v", err)
	}
}

func TestReadIndex_DoesNotWrapAbsentFileWithErrCorruptIndex(t *testing.T) {
	dir := t.TempDir()

	_, _, err := state.ReadIndex(dir)
	if err != nil {
		t.Fatalf("expected nil err for absent file; got %v", err)
	}
	if errors.Is(err, state.ErrCorruptIndex) {
		t.Errorf("absent file must NOT be classified as corrupt")
	}
}

func TestReadIndex_PerformsNoStdoutOrStderrWrites(t *testing.T) {
	dir := t.TempDir()

	// Cover all branches: missing file, valid file, parse error.
	cases := []struct {
		name  string
		setup func(t *testing.T, d string)
	}{
		{
			name:  "absent",
			setup: func(*testing.T, string) {},
		},
		{
			name: "valid v1",
			setup: func(t *testing.T, d string) {
				data, err := state.EncodeIndex(state.Index{
					Version: state.SchemaVersion,
					SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
				})
				if err != nil {
					t.Fatalf("EncodeIndex: %v", err)
				}
				writeSessionsJSON(t, d, data)
			},
		},
		{
			name: "parse error",
			setup: func(t *testing.T, d string) {
				writeSessionsJSON(t, d, []byte("not json"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caseDir := filepath.Join(dir, tc.name)
			if err := os.MkdirAll(caseDir, 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			tc.setup(t, caseDir)

			outR, outW, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe stdout: %v", err)
			}
			errR, errW, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe stderr: %v", err)
			}
			origStdout, origStderr := os.Stdout, os.Stderr
			os.Stdout = outW
			os.Stderr = errW

			_, _, _ = state.ReadIndex(caseDir)

			os.Stdout = origStdout
			os.Stderr = origStderr
			_ = outW.Close()
			_ = errW.Close()

			outBuf := make([]byte, 1024)
			errBuf := make([]byte, 1024)
			n, _ := outR.Read(outBuf)
			m, _ := errR.Read(errBuf)
			_ = outR.Close()
			_ = errR.Close()

			if n != 0 {
				t.Errorf("ReadIndex wrote to stdout: %q", outBuf[:n])
			}
			if m != 0 {
				t.Errorf("ReadIndex wrote to stderr: %q", errBuf[:m])
			}
		})
	}
}
