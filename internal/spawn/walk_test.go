package spawn

import (
	"errors"
	"testing"
)

// fakeProc is one map-backed process record for the fake ProcessWalker seam.
type fakeProc struct {
	ppid    int
	command string
	err     error
}

// fakeWalker is a map-backed ProcessWalker: pid -> fakeProc. It records the
// pids it was asked about so a test can assert the walk order / call count.
type fakeWalker struct {
	procs map[int]fakeProc
	calls []int
}

func (f *fakeWalker) ProcessInfo(pid int) (int, string, error) {
	f.calls = append(f.calls, pid)
	p, ok := f.procs[pid]
	if !ok {
		return 0, "", errors.New("fakeWalker: no such pid")
	}
	return p.ppid, p.command, p.err
}

// fakeBundle is one map-backed bundle record for the fake BundleReader seam.
type fakeBundle struct {
	bundleID string
	name     string
	err      error
}

// fakeReader is a map-backed BundleReader: appPath -> fakeBundle. It records
// the app paths it was asked to read.
type fakeReader struct {
	bundles map[string]fakeBundle
	calls   []string
}

func (f *fakeReader) Read(appPath string) (string, string, error) {
	f.calls = append(f.calls, appPath)
	b, ok := f.bundles[appPath]
	if !ok {
		return "", "", errors.New("fakeReader: no such app")
	}
	return b.bundleID, b.name, b.err
}

// monotonicWalker is a ProcessWalker that never reaches a .app and never
// reaches ppid 1: for any pid it reports a parent of pid+1 with a shell
// command. It never repeats a pid, so only the hop bound can terminate the
// walk — this exercises the runaway/over-long guard specifically.
type monotonicWalker struct {
	calls int
}

func (m *monotonicWalker) ProcessInfo(pid int) (int, string, error) {
	m.calls++
	return pid + 1, "/bin/zsh", nil
}

func TestWalkToBundle(t *testing.T) {
	t.Run("it resolves a multi-hop local chain to the app bundle id", func(t *testing.T) {
		walker := &fakeWalker{procs: map[int]fakeProc{
			100: {ppid: 200, command: "portal"},
			200: {ppid: 300, command: "/bin/zsh"},
			300: {ppid: 1, command: "/Applications/Ghostty.app/Contents/MacOS/ghostty"},
		}}
		reader := &fakeReader{bundles: map[string]fakeBundle{
			"/Applications/Ghostty.app": {bundleID: "com.mitchellh.ghostty", name: "Ghostty"},
		}}

		got, err := walkToBundle(100, walker, reader)
		if err != nil {
			t.Fatalf("walkToBundle returned error: %v, want nil", err)
		}
		if got.IsNull() {
			t.Fatalf("walkToBundle returned NULL identity, want a resolved identity")
		}
		if got.BundleID != "com.mitchellh.ghostty" {
			t.Errorf("BundleID = %q, want %q", got.BundleID, "com.mitchellh.ghostty")
		}
		if got.Name != "Ghostty" {
			t.Errorf("Name = %q, want %q", got.Name, "Ghostty")
		}
		if len(reader.calls) != 1 || reader.calls[0] != "/Applications/Ghostty.app" {
			t.Errorf("reader called with %v, want exactly [/Applications/Ghostty.app]", reader.calls)
		}
	})

	t.Run("it returns clean NULL when ancestry reaches ppid 1 with no app bundle", func(t *testing.T) {
		walker := &fakeWalker{procs: map[int]fakeProc{
			100: {ppid: 200, command: "portal"},
			200: {ppid: 300, command: "/bin/zsh"},
			300: {ppid: 1, command: "/usr/bin/login"},
		}}
		reader := &fakeReader{bundles: map[string]fakeBundle{}}

		got, err := walkToBundle(100, walker, reader)
		if err != nil {
			t.Fatalf("walkToBundle returned error: %v, want nil", err)
		}
		if !got.IsNull() {
			t.Errorf("walkToBundle = %+v, want NULL identity", got)
		}
		if len(reader.calls) != 0 {
			t.Errorf("reader was called %v, want no calls (no .app found)", reader.calls)
		}
	})

	t.Run("it returns clean NULL for a mosh-server ancestry", func(t *testing.T) {
		walker := &fakeWalker{procs: map[int]fakeProc{
			100: {ppid: 200, command: "/bin/zsh"},
			200: {ppid: 1, command: "mosh-server"},
		}}
		reader := &fakeReader{bundles: map[string]fakeBundle{}}

		got, err := walkToBundle(100, walker, reader)
		if err != nil {
			t.Fatalf("walkToBundle returned error: %v, want nil", err)
		}
		if !got.IsNull() {
			t.Errorf("walkToBundle = %+v, want NULL identity", got)
		}
	})

	t.Run("it returns a transient error when ps fails, distinct from clean NULL", func(t *testing.T) {
		psFailure := errors.New("ps: operation not permitted")
		walker := &fakeWalker{procs: map[int]fakeProc{
			100: {ppid: 200, command: "/bin/zsh"},
			200: {err: psFailure},
		}}
		reader := &fakeReader{bundles: map[string]fakeBundle{}}

		got, err := walkToBundle(100, walker, reader)
		if err == nil {
			t.Fatalf("walkToBundle returned nil error, want a transient error")
		}
		if !errors.Is(err, ErrDetectTransient) {
			t.Errorf("errors.Is(err, ErrDetectTransient) = false, want true; err = %v", err)
		}
		if !errors.Is(err, psFailure) {
			t.Errorf("underlying ps failure not preserved in the error chain; err = %v", err)
		}
		if !got.IsNull() {
			t.Errorf("walkToBundle identity = %+v, want NULL alongside the transient error", got)
		}
	})

	t.Run("it returns a transient error when defaults read fails on a found app", func(t *testing.T) {
		readFailure := errors.New("defaults: could not read domain")
		walker := &fakeWalker{procs: map[int]fakeProc{
			100: {ppid: 1, command: "/Applications/Ghostty.app/Contents/MacOS/ghostty"},
		}}
		reader := &fakeReader{bundles: map[string]fakeBundle{
			"/Applications/Ghostty.app": {err: readFailure},
		}}

		got, err := walkToBundle(100, walker, reader)
		if err == nil {
			t.Fatalf("walkToBundle returned nil error, want a transient error")
		}
		if !errors.Is(err, ErrDetectTransient) {
			t.Errorf("errors.Is(err, ErrDetectTransient) = false, want true; err = %v", err)
		}
		if !errors.Is(err, readFailure) {
			t.Errorf("underlying defaults failure not preserved in the error chain; err = %v", err)
		}
		if !got.IsNull() {
			t.Errorf("walkToBundle identity = %+v, want NULL alongside the transient error", got)
		}
	})

	t.Run("it terminates on a cyclic ancestry via the hop bound", func(t *testing.T) {
		walker := &monotonicWalker{}
		reader := &fakeReader{bundles: map[string]fakeBundle{}}

		got, err := walkToBundle(100, walker, reader)
		if err != nil {
			t.Fatalf("walkToBundle returned error: %v, want nil (clean NULL on hop-bound)", err)
		}
		if !got.IsNull() {
			t.Errorf("walkToBundle = %+v, want NULL identity", got)
		}
		if walker.calls != maxWalkHops {
			t.Errorf("walker was called %d times, want exactly maxWalkHops (%d) — the hop bound must stop the walk", walker.calls, maxWalkHops)
		}
	})

	t.Run("it terminates on a self-referential cycle", func(t *testing.T) {
		// A 2-cycle (100 <-> 200) must terminate via the repeated-pid guard,
		// well before the hop bound, and resolve to clean NULL.
		walker := &fakeWalker{procs: map[int]fakeProc{
			100: {ppid: 200, command: "/bin/zsh"},
			200: {ppid: 100, command: "/bin/zsh"},
		}}
		reader := &fakeReader{bundles: map[string]fakeBundle{}}

		got, err := walkToBundle(100, walker, reader)
		if err != nil {
			t.Fatalf("walkToBundle returned error: %v, want nil", err)
		}
		if !got.IsNull() {
			t.Errorf("walkToBundle = %+v, want NULL identity", got)
		}
		if len(walker.calls) >= maxWalkHops {
			t.Errorf("walker was called %d times, want the repeated-pid guard to stop well before the hop bound (%d)", len(walker.calls), maxWalkHops)
		}
	})
}

func TestAppBundlePath(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantOK  bool
		want    string
	}{
		{
			name:    "it extracts the bundle dir from an executable path inside a .app",
			command: "/Applications/Ghostty.app/Contents/MacOS/ghostty",
			wantOK:  true,
			want:    "/Applications/Ghostty.app",
		},
		{
			name:    "it handles a bundle path that contains spaces",
			command: "/Applications/Visual Studio Code.app/Contents/MacOS/Electron",
			wantOK:  true,
			want:    "/Applications/Visual Studio Code.app",
		},
		{
			name:    "it reports no bundle for a plain executable path",
			command: "/usr/bin/login",
			wantOK:  false,
			want:    "",
		},
		{
			name:    "it reports no bundle for a bare process name",
			command: "mosh-server",
			wantOK:  false,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := appBundlePath(tt.command)
			if ok != tt.wantOK {
				t.Errorf("appBundlePath(%q) ok = %v, want %v", tt.command, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("appBundlePath(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestParsePSProcessInfo(t *testing.T) {
	t.Run("it parses a right-justified ppid and a full-path command", func(t *testing.T) {
		ppid, command, err := parsePSProcessInfo("  300 /Applications/Ghostty.app/Contents/MacOS/ghostty\n")
		if err != nil {
			t.Fatalf("parsePSProcessInfo returned error: %v", err)
		}
		if ppid != 300 {
			t.Errorf("ppid = %d, want 300", ppid)
		}
		if command != "/Applications/Ghostty.app/Contents/MacOS/ghostty" {
			t.Errorf("command = %q, want the full ghostty path", command)
		}
	})

	t.Run("it keeps embedded spaces in the command path", func(t *testing.T) {
		ppid, command, err := parsePSProcessInfo("42 /Applications/Visual Studio Code.app/Contents/MacOS/Electron")
		if err != nil {
			t.Fatalf("parsePSProcessInfo returned error: %v", err)
		}
		if ppid != 42 {
			t.Errorf("ppid = %d, want 42", ppid)
		}
		if command != "/Applications/Visual Studio Code.app/Contents/MacOS/Electron" {
			t.Errorf("command = %q, want the full path with embedded spaces", command)
		}
	})

	t.Run("it errors on empty ps output", func(t *testing.T) {
		if _, _, err := parsePSProcessInfo("   \n"); err == nil {
			t.Error("parsePSProcessInfo(empty) returned nil error, want an error")
		}
	})

	t.Run("it errors on a non-numeric ppid field", func(t *testing.T) {
		if _, _, err := parsePSProcessInfo("notapid /bin/zsh"); err == nil {
			t.Error("parsePSProcessInfo(non-numeric ppid) returned nil error, want an error")
		}
	})
}

func TestAppBasename(t *testing.T) {
	tests := []struct {
		name    string
		appPath string
		want    string
	}{
		{
			name:    "it strips the .app suffix from the basename",
			appPath: "/Applications/Ghostty.app",
			want:    "Ghostty",
		},
		{
			name:    "it strips .app from a name that contains spaces",
			appPath: "/Applications/Visual Studio Code.app",
			want:    "Visual Studio Code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appBasename(tt.appPath); got != tt.want {
				t.Errorf("appBasename(%q) = %q, want %q", tt.appPath, got, tt.want)
			}
		})
	}
}
