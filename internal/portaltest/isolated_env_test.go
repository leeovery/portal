package portaltest_test

// Tests for the per-test state-directory isolation helper.
// These pin the env-slice contract that callers will pass to
// exec.Cmd.Env when spawning `portal state daemon` subprocesses.
// The helper is the structural mechanism preventing leaked
// daemons from writing to the developer's real
// ~/.config/portal/state/.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portaltest"
)

// TestMain redirects HOME and XDG_CONFIG_HOME at process start so
// the fingerprint backstop registered by NewIsolatedStateEnv targets
// a hermetic temp directory, never the developer's real
// ~/.config/portal/state/. Without this hook, running this package's
// own test suite on a machine with a live `portal state daemon`
// would race the daemon's tick writes and the backstop would flag
// legitimate dev-install mutation as a test failure.
//
// Per-test t.Setenv would also work but is fragile — TestMain
// applies the redirect once for the whole binary, including any
// future tests added to the package.
func TestMain(m *testing.M) {
	sandbox, err := os.MkdirTemp("", "portaltest-self-sandbox-*")
	if err != nil {
		panic("portaltest: mkdir sandbox: " + err.Error())
	}
	defer func() { _ = os.RemoveAll(sandbox) }()

	_ = os.Setenv("HOME", sandbox)
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(sandbox, "config"))

	os.Exit(m.Run())
}

// envValue extracts the value for KEY from an env slice, returning
// ("", false) when the key is absent. Used to assert presence/absence
// and exact value of XDG_CONFIG_HOME without leaking ordering details.
func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix), true
		}
	}
	return "", false
}

// envCount returns the number of entries in env whose key equals KEY.
// Used to assert XDG_CONFIG_HOME appears exactly once (no duplicate
// from a pre-existing inherited entry).
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

// TestSetsXDGConfigHomeInsideTempDir asserts the helper writes
// XDG_CONFIG_HOME into the returned env and the value resolves under
// the per-test t.TempDir() — the core isolation guarantee.
func TestSetsXDGConfigHomeInsideTempDir(t *testing.T) {
	env, _ := portaltest.NewIsolatedStateEnv(t)

	got, ok := envValue(env, "XDG_CONFIG_HOME")
	if !ok {
		t.Fatalf("XDG_CONFIG_HOME absent from returned env")
	}
	// Value must point inside a t.TempDir(); the helper uses
	// <tempDir>/config so the path must end with /config and exist.
	if filepath.Base(got) != "config" {
		t.Fatalf("XDG_CONFIG_HOME does not end in /config: %q", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("XDG_CONFIG_HOME path does not exist: %v", err)
	}
}

// TestRemovesPreExistingXDGConfigHome asserts that when the test
// process already has XDG_CONFIG_HOME set, the returned env contains
// exactly one XDG_CONFIG_HOME entry — the helper's value, not the
// inherited one. This pins the dedupe contract.
func TestRemovesPreExistingXDGConfigHome(t *testing.T) {
	decoy := "/decoy/should/not/leak"
	t.Setenv("XDG_CONFIG_HOME", decoy)

	env, _ := portaltest.NewIsolatedStateEnv(t)

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

// TestRemovesEmptyPreExistingXDGConfigHome covers the edge case of
// XDG_CONFIG_HOME="" — must still be replaced, not duplicated.
func TestRemovesEmptyPreExistingXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	env, _ := portaltest.NewIsolatedStateEnv(t)

	if got := envCount(env, "XDG_CONFIG_HOME"); got != 1 {
		t.Fatalf("expected exactly 1 XDG_CONFIG_HOME entry, got %d", got)
	}
	got, _ := envValue(env, "XDG_CONFIG_HOME")
	if got == "" {
		t.Fatalf("XDG_CONFIG_HOME is empty; helper did not replace inherited empty value")
	}
}

// TestPreservesHomeAndPath spot-checks that unrelated env entries
// (HOME, PATH) are preserved from os.Environ(). The helper must only
// touch XDG_CONFIG_HOME.
func TestPreservesHomeAndPath(t *testing.T) {
	wantHome := os.Getenv("HOME")
	wantPath := os.Getenv("PATH")

	env, _ := portaltest.NewIsolatedStateEnv(t)

	gotHome, okHome := envValue(env, "HOME")
	if !okHome {
		t.Fatalf("HOME missing from returned env")
	}
	if gotHome != wantHome {
		t.Errorf("HOME mutated: got %q want %q", gotHome, wantHome)
	}
	gotPath, okPath := envValue(env, "PATH")
	if !okPath {
		t.Fatalf("PATH missing from returned env")
	}
	if gotPath != wantPath {
		t.Errorf("PATH mutated: got %q want %q", gotPath, wantPath)
	}
}

// TestStateDirUnderXDGConfigHome asserts the returned stateDir
// resolves to <XDG_CONFIG_HOME>/portal/state and exists on disk.
// Pins the path layout daemon/saver tests depend on.
func TestStateDirUnderXDGConfigHome(t *testing.T) {
	env, stateDir := portaltest.NewIsolatedStateEnv(t)

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

// TestEnvUsableAsExecCmdEnv verifies the contract that callers can
// assign the returned env directly to exec.Cmd.Env and the spawned
// subprocess sees the helper's XDG_CONFIG_HOME. This is the end-to-end
// integration the daemon tests rely on.
func TestEnvUsableAsExecCmdEnv(t *testing.T) {
	env, _ := portaltest.NewIsolatedStateEnv(t)
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

// TestDistinctStateDirPerCall asserts that two independent calls
// (here modeled as subtests, each with its own *testing.T) produce
// non-overlapping stateDir paths. This is the structural guarantee
// that prevents cross-test leakage when many daemon tests run in the
// same process.
func TestDistinctStateDirPerCall(t *testing.T) {
	var a, b string
	t.Run("first", func(t *testing.T) {
		_, a = portaltest.NewIsolatedStateEnv(t)
	})
	t.Run("second", func(t *testing.T) {
		_, b = portaltest.NewIsolatedStateEnv(t)
	})
	if a == "" || b == "" {
		t.Fatalf("empty stateDir(s): a=%q b=%q", a, b)
	}
	if a == b {
		t.Fatalf("expected distinct stateDirs across subtests, both got %q", a)
	}
}

// TestConfigDirPermissions asserts the configDir (XDG_CONFIG_HOME
// itself) is created with 0o700 perms — the helper MkdirAlls it
// before returning. This matches the perm pattern used elsewhere in
// portal for sensitive state directories.
func TestConfigDirPermissions(t *testing.T) {
	env, _ := portaltest.NewIsolatedStateEnv(t)
	configDir, _ := envValue(env, "XDG_CONFIG_HOME")
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("stat configDir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("configDir perm = %#o, want %#o", perm, 0o700)
	}
}
