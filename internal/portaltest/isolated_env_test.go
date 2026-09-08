package portaltest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
)

func TestMain(m *testing.M) {
	os.Exit(runInSelfSandbox(m.Run))
}

// runInSelfSandbox redirects HOME and XDG_CONFIG_HOME at a temp sandbox for the
// whole binary so the backstop under test targets a hermetic temp dir: on a
// machine with a live `portal state daemon` it would otherwise race that
// daemon's tick writes and flag them. It removes the sandbox and returns run's
// own code, so no cleanup rides on a deferred function os.Exit would skip.
func runInSelfSandbox(run func() int) int {
	sandbox, err := os.MkdirTemp("", "portaltest-self-sandbox-*")
	if err != nil {
		panic("portaltest: mkdir sandbox: " + err.Error())
	}

	_ = os.Setenv("HOME", sandbox)
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(sandbox, "config"))

	code := run()

	_ = os.RemoveAll(sandbox)
	return code
}

func TestRunInSelfSandbox(t *testing.T) {
	t.Run("it removes the self-test sandbox directory and preserves the run's exit code", func(t *testing.T) {
		// The helper sets these with os.Setenv; t.Setenv puts the binary-wide
		// values TestMain established back when this test ends.
		t.Setenv("HOME", os.Getenv("HOME"))
		t.Setenv("XDG_CONFIG_HOME", os.Getenv("XDG_CONFIG_HOME"))

		const wantCode = 7
		var sandbox string

		code := runInSelfSandbox(func() int {
			sandbox = os.Getenv("HOME")
			return wantCode
		})

		if code != wantCode {
			t.Errorf("exit code = %d, want the run's own %d", code, wantCode)
		}
		if sandbox == "" {
			t.Fatalf("the run saw no HOME; the sandbox was not installed")
		}
		if _, err := os.Stat(sandbox); !os.IsNotExist(err) {
			t.Errorf("sandbox %q survived the run: stat err = %v, want not-exist", sandbox, err)
		}
	})
}

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if after, ok := strings.CutPrefix(e, prefix); ok {
			return after, true
		}
	}
	return "", false
}

func envCount(env []string, key string) int {
	prefix := key + "="
	n := 0
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func TestSetsXDGConfigHomeInsideTempDir(t *testing.T) {
	env, _ := portaltest.IsolateStateForTest(t)

	got, ok := envValue(env, "XDG_CONFIG_HOME")
	if !ok {
		t.Fatalf("XDG_CONFIG_HOME absent from returned env")
	}
	if filepath.Base(got) != "config" {
		t.Fatalf("XDG_CONFIG_HOME does not end in /config: %q", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("XDG_CONFIG_HOME path does not exist: %v", err)
	}
}

func TestRemovesPreExistingXDGConfigHome(t *testing.T) {
	decoy := "/decoy/should/not/leak"
	t.Setenv("XDG_CONFIG_HOME", decoy)

	env, _ := portaltest.IsolateStateForTest(t)

	if got := envCount(env, "XDG_CONFIG_HOME"); got != 1 {
		t.Fatalf("expected exactly 1 XDG_CONFIG_HOME entry, got %d", got)
	}
	got, _ := envValue(env, "XDG_CONFIG_HOME")
	if got == decoy {
		t.Fatalf("XDG_CONFIG_HOME leaked decoy value %q", decoy)
	}
	if strings.Contains(got, decoy) {
		t.Fatalf("XDG_CONFIG_HOME contains decoy fragment %q in %q", decoy, got)
	}
}

func TestRemovesEmptyPreExistingXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	env, _ := portaltest.IsolateStateForTest(t)

	if got := envCount(env, "XDG_CONFIG_HOME"); got != 1 {
		t.Fatalf("expected exactly 1 XDG_CONFIG_HOME entry, got %d", got)
	}
	got, _ := envValue(env, "XDG_CONFIG_HOME")
	if got == "" {
		t.Fatalf("XDG_CONFIG_HOME is empty; helper did not replace inherited empty value")
	}
}

// HOME is deliberately not preserved: the helper re-points it at a fresh
// tempdir.
func TestPreservesPath(t *testing.T) {
	wantPath := os.Getenv("PATH")

	env, _ := portaltest.IsolateStateForTest(t)

	gotPath, okPath := envValue(env, "PATH")
	if !okPath {
		t.Fatalf("PATH missing from returned env")
	}
	if gotPath != wantPath {
		t.Errorf("PATH mutated: got %q want %q", gotPath, wantPath)
	}
}

func TestNeutralizesHomeAndXDGConfigHome(t *testing.T) {
	priorHome := os.Getenv("HOME")

	_, _ = portaltest.IsolateStateForTest(t)

	gotHome := os.Getenv("HOME")
	if gotHome == priorHome {
		t.Fatalf("HOME not scrubbed: still %q (helper must re-point at a fresh tempdir)", gotHome)
	}
	if gotHome == "" {
		t.Fatalf("HOME scrubbed to empty; helper must point at a tempdir, not unset")
	}
	if got := os.Getenv("XDG_CONFIG_HOME"); got != "" {
		t.Fatalf("XDG_CONFIG_HOME not cleared on test process env: got %q", got)
	}
}

func TestStateDirUnderXDGConfigHome(t *testing.T) {
	env, stateDir := portaltest.IsolateStateForTest(t)

	xdg, ok := envValue(env, "XDG_CONFIG_HOME")
	if !ok {
		t.Fatalf("XDG_CONFIG_HOME absent")
	}
	want := filepath.Join(xdg, "portal", "state")
	if stateDir != want {
		t.Fatalf("stateDir mismatch: got %q want %q", stateDir, want)
	}
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("stateDir not on disk: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("stateDir is not a directory: %s", stateDir)
	}
}

func TestEnvUsableAsExecCmdEnv(t *testing.T) {
	env, _ := portaltest.IsolateStateForTest(t)
	wantXDG, _ := envValue(env, "XDG_CONFIG_HOME")

	cmd := exec.Command("sh", "-c", "echo $XDG_CONFIG_HOME")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sh exec: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != wantXDG {
		t.Fatalf("subprocess saw XDG_CONFIG_HOME=%q, want %q", got, wantXDG)
	}
}

func TestDistinctStateDirPerCall(t *testing.T) {
	var a, b string
	t.Run("first", func(t *testing.T) {
		_, a = portaltest.IsolateStateForTest(t)
	})
	t.Run("second", func(t *testing.T) {
		_, b = portaltest.IsolateStateForTest(t)
	})
	if a == "" || b == "" {
		t.Fatalf("empty stateDir(s): a=%q b=%q", a, b)
	}
	if a == b {
		t.Fatalf("expected distinct stateDirs across subtests, both got %q", a)
	}
}

func TestConfigDirPermissions(t *testing.T) {
	env, _ := portaltest.IsolateStateForTest(t)
	configDir, _ := envValue(env, "XDG_CONFIG_HOME")
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("stat configDir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("configDir perm = %#o, want %#o", perm, 0o700)
	}
}

func TestPointsHISTFILEAtNullDevice(t *testing.T) {
	t.Setenv("HISTFILE", "/decoy/should/not/leak")

	env, _ := portaltest.IsolateStateForTest(t)

	if got := os.Getenv("HISTFILE"); got != os.DevNull {
		t.Errorf("process HISTFILE = %q, want %q", got, os.DevNull)
	}
	if got := envCount(env, "HISTFILE"); got != 1 {
		t.Fatalf("expected exactly 1 HISTFILE entry in the returned env, got %d", got)
	}
	got, _ := envValue(env, "HISTFILE")
	if got != os.DevNull {
		t.Errorf("returned env HISTFILE = %q, want %q", got, os.DevNull)
	}
}

func TestStateDirEnv(t *testing.T) {
	t.Run("it sets PORTAL_STATE_DIR to the returned state dir", func(t *testing.T) {
		_, stateDir := portaltest.IsolateStateForTest(t)

		if got := os.Getenv("PORTAL_STATE_DIR"); got != stateDir {
			t.Fatalf("process PORTAL_STATE_DIR = %q, want %q", got, stateDir)
		}
	})

	t.Run("it carries PORTAL_STATE_DIR in the returned env slice", func(t *testing.T) {
		t.Setenv("PORTAL_STATE_DIR", "/decoy/should/not/leak")

		env, stateDir := portaltest.IsolateStateForTest(t)

		if got := envCount(env, "PORTAL_STATE_DIR"); got != 1 {
			t.Fatalf("expected exactly 1 PORTAL_STATE_DIR entry, got %d", got)
		}
		got, _ := envValue(env, "PORTAL_STATE_DIR")
		if got != stateDir {
			t.Fatalf("returned env PORTAL_STATE_DIR = %q, want %q", got, stateDir)
		}
	})

	t.Run("it registers the same directory with the sandbox registry as the slice names", func(t *testing.T) {
		env, _ := portaltest.IsolateStateForTest(t)

		registryPath, ok := envValue(env, state.SandboxRegistryEnv)
		if !ok {
			t.Fatalf("%s absent from returned env", state.SandboxRegistryEnv)
		}
		body, err := os.ReadFile(registryPath)
		if err != nil {
			t.Fatalf("read sandbox registry: %v", err)
		}
		named, _ := envValue(env, "PORTAL_STATE_DIR")
		if got := strings.TrimSpace(string(body)); got != named {
			t.Fatalf("sandbox registry names %q, env slice names %q", got, named)
		}
	})

	t.Run("it lets a caller override the state dir after the call", func(t *testing.T) {
		env, _ := portaltest.IsolateStateForTest(t)

		override := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", override)
		env = append(env, "PORTAL_STATE_DIR="+override)

		if got := os.Getenv("PORTAL_STATE_DIR"); got != override {
			t.Fatalf("process PORTAL_STATE_DIR = %q, want the override %q", got, override)
		}

		cmd := exec.Command("sh", "-c", "echo $PORTAL_STATE_DIR")
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("sh exec: %v", err)
		}
		if got := strings.TrimSpace(string(out)); got != override {
			t.Fatalf("subprocess saw PORTAL_STATE_DIR=%q, want the override %q", got, override)
		}
	})
}

func TestPointsZDOTDIROutsideTheFrameworkTempTree(t *testing.T) {
	t.Setenv("ZDOTDIR", "/decoy/should/not/leak")

	env, _ := portaltest.IsolateStateForTest(t)
	frameworkTree := filepath.Dir(t.TempDir())

	t.Run("it points ZDOTDIR outside the framework temp tree", func(t *testing.T) {
		zdotdir := os.Getenv("ZDOTDIR")
		if zdotdir == "" {
			t.Fatalf("ZDOTDIR unset on the test process env; the shell would resolve it to the temp HOME")
		}
		if under(zdotdir, frameworkTree) {
			t.Errorf("ZDOTDIR %q is inside the framework temp tree %q; the shell's writes land in the directory the framework removes", zdotdir, frameworkTree)
		}
		if info, err := os.Stat(zdotdir); err != nil {
			t.Errorf("ZDOTDIR %q not on disk: %v", zdotdir, err)
		} else if !info.IsDir() {
			t.Errorf("ZDOTDIR %q is not a directory", zdotdir)
		}
	})

	t.Run("it carries that ZDOTDIR in the returned env slice", func(t *testing.T) {
		if got := envCount(env, "ZDOTDIR"); got != 1 {
			t.Fatalf("expected exactly 1 ZDOTDIR entry in the returned env, got %d", got)
		}
		if got, _ := envValue(env, "ZDOTDIR"); got != os.Getenv("ZDOTDIR") {
			t.Errorf("returned env ZDOTDIR = %q, want the process value %q", got, os.Getenv("ZDOTDIR"))
		}
	})
}

func under(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
