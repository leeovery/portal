package spawn

import (
	"errors"
	"testing"
)

// failWalker is a ProcessWalker that fails the test the moment it is invoked.
// It guards the env fast-path branches, where detection must resolve from the
// environment alone and never touch the process tree.
type failWalker struct{ t *testing.T }

func (f failWalker) ProcessInfo(pid int) (int, string, error) {
	f.t.Helper()
	f.t.Fatalf("ProcessInfo(%d) called: the env fast-path must not walk the process tree", pid)
	return 0, "", nil
}

// failReader is a BundleReader that fails the test the moment it is invoked —
// the env fast-path never reads a bundle.
type failReader struct{ t *testing.T }

func (f failReader) Read(appPath string) (string, string, error) {
	f.t.Helper()
	f.t.Fatalf("Read(%q) called: the env fast-path must not read a bundle", appPath)
	return "", "", nil
}

// mapGetenv builds a getenv seam backed by a map; unset keys read as "".
func mapGetenv(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

func TestDetectOutsideTmux(t *testing.T) {
	t.Run("it resolves directly from __CFBundleIdentifier without walking", func(t *testing.T) {
		getenv := mapGetenv(map[string]string{
			"__CFBundleIdentifier": "com.apple.Terminal",
		})

		got, err := detectOutsideTmux(getenv, 100, failWalker{t}, failReader{t})
		if err != nil {
			t.Fatalf("detectOutsideTmux returned error: %v, want nil", err)
		}
		if got.BundleID != "com.apple.Terminal" {
			t.Errorf("BundleID = %q, want %q", got.BundleID, "com.apple.Terminal")
		}
		if got.Name != "Terminal" {
			t.Errorf("Name = %q, want %q", got.Name, "Terminal")
		}
	})

	t.Run("it resolves to Ghostty from a GHOSTTY_* var when __CFBundleIdentifier is absent", func(t *testing.T) {
		for _, key := range []string{"GHOSTTY_RESOURCES_DIR", "GHOSTTY_BIN_DIR"} {
			t.Run(key, func(t *testing.T) {
				getenv := mapGetenv(map[string]string{
					key: "/Applications/Ghostty.app/Contents/Resources",
				})

				got, err := detectOutsideTmux(getenv, 100, failWalker{t}, failReader{t})
				if err != nil {
					t.Fatalf("detectOutsideTmux returned error: %v, want nil", err)
				}
				if got.BundleID != "com.mitchellh.ghostty" {
					t.Errorf("BundleID = %q, want %q", got.BundleID, "com.mitchellh.ghostty")
				}
				if got.Name != "Ghostty" {
					t.Errorf("Name = %q, want %q", got.Name, "Ghostty")
				}
			})
		}
	})

	t.Run("it falls back to the walk when both env vars are absent", func(t *testing.T) {
		getenv := mapGetenv(nil)
		walker := &fakeWalker{procs: map[int]fakeProc{
			777: {ppid: 1, command: "/Applications/Ghostty.app/Contents/MacOS/ghostty"},
		}}
		reader := &fakeReader{bundles: map[string]fakeBundle{
			"/Applications/Ghostty.app": {bundleID: "com.mitchellh.ghostty", name: "Ghostty"},
		}}

		got, err := detectOutsideTmux(getenv, 777, walker, reader)
		if err != nil {
			t.Fatalf("detectOutsideTmux returned error: %v, want nil", err)
		}
		if got.BundleID != "com.mitchellh.ghostty" || got.Name != "Ghostty" {
			t.Errorf("identity = %+v, want the walk's resolved Ghostty identity", got)
		}
		if len(walker.calls) == 0 || walker.calls[0] != 777 {
			t.Errorf("walker first call = %v, want the walk to start at selfPID 777", walker.calls)
		}
	})

	t.Run("it falls back to the walk for an empty or malformed __CFBundleIdentifier", func(t *testing.T) {
		cases := []struct {
			name  string
			value string
		}{
			{"empty", ""},
			{"whitespace only", "  "},
			{"no dot", "Terminal"},
			{"internal whitespace", "com.apple Terminal"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				getenv := mapGetenv(map[string]string{
					"__CFBundleIdentifier": tc.value,
				})
				walker := &fakeWalker{procs: map[int]fakeProc{
					100: {ppid: 1, command: "/Applications/Ghostty.app/Contents/MacOS/ghostty"},
				}}
				reader := &fakeReader{bundles: map[string]fakeBundle{
					"/Applications/Ghostty.app": {bundleID: "com.mitchellh.ghostty", name: "Ghostty"},
				}}

				got, err := detectOutsideTmux(getenv, 100, walker, reader)
				if err != nil {
					t.Fatalf("detectOutsideTmux returned error: %v, want nil", err)
				}
				// The result must come from the walk, not the bogus env value.
				if got.BundleID != "com.mitchellh.ghostty" {
					t.Errorf("BundleID = %q, want the walk's %q (env value %q must not be trusted)", got.BundleID, "com.mitchellh.ghostty", tc.value)
				}
				if len(walker.calls) == 0 {
					t.Errorf("walk was not invoked for malformed env value %q, want the walk fallback", tc.value)
				}
			})
		}
	})

	t.Run("it propagates the walk's clean NULL and transient error unchanged", func(t *testing.T) {
		t.Run("clean NULL", func(t *testing.T) {
			getenv := mapGetenv(nil)
			walker := &fakeWalker{procs: map[int]fakeProc{
				100: {ppid: 1, command: "/usr/bin/login"},
			}}
			reader := &fakeReader{bundles: map[string]fakeBundle{}}

			got, err := detectOutsideTmux(getenv, 100, walker, reader)
			if err != nil {
				t.Fatalf("detectOutsideTmux returned error: %v, want nil", err)
			}
			if !got.IsNull() {
				t.Errorf("identity = %+v, want NULL propagated from the walk", got)
			}
		})

		t.Run("transient error", func(t *testing.T) {
			getenv := mapGetenv(nil)
			psFailure := errors.New("ps: operation not permitted")
			walker := &fakeWalker{procs: map[int]fakeProc{
				100: {err: psFailure},
			}}
			reader := &fakeReader{bundles: map[string]fakeBundle{}}

			got, err := detectOutsideTmux(getenv, 100, walker, reader)
			if !errors.Is(err, ErrDetectTransient) {
				t.Errorf("errors.Is(err, ErrDetectTransient) = false, want true; err = %v", err)
			}
			if !errors.Is(err, psFailure) {
				t.Errorf("underlying ps failure not preserved; err = %v", err)
			}
			if !got.IsNull() {
				t.Errorf("identity = %+v, want NULL alongside the transient error", got)
			}
		})
	})
}
