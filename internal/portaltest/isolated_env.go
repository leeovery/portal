package portaltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// IsolateStateForTest returns an env slice and a stateDir scoped to a per-test
// t.TempDir(); assign env to exec.Cmd.Env, then append your own TMUX=<test
// socket> (see PoisonedTmuxSocket). stateDir exists on return, and both the
// calling process's PORTAL_STATE_DIR and the returned slice's name it — the same
// directory the sandbox registry is given. A fixture wanting a different one
// overrides it after the call: the process env via t.Setenv, the slice by
// appending (exec.Cmd env dedupe is last-wins). It also mutates the calling
// process's own env via t.Setenv and registers a cleanup that fails the test if
// the state dir under the scrubbed HOME changed during the run (see
// resolveDevStateDir for why the backstop's reach stops there).
func IsolateStateForTest(t *testing.T) (env []string, stateDir string) {
	t.Helper()

	// Default-deny: daemon enumeration then surfaces only what this test
	// registers, so a sweep can never reach the developer's live daemon.
	state.EnableDaemonSandbox()
	t.Cleanup(state.ResetDaemonSandbox)

	// Scrub the host env before the snapshot below: with HOME re-pointed and
	// XDG_CONFIG_HOME cleared, the snapshotted path is one no live host daemon
	// writes to, so it cannot false-trip the backstop mid-test.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	// Shells hosted by a test's tmux server flush per-session files as they exit,
	// racing the framework's RemoveAll of the temp HOME. zsh's history is that
	// writer: /etc/zshrc assigns HISTFILE=${ZDOTDIR:-$HOME}/.zsh_history for every
	// interactive shell whatever it inherited, so only ZDOTDIR moves it — and it
	// must name a directory outside the tree the framework removes, which homeDir
	// is not. zsh's session directory hangs off the same prefix and moves with it.
	// HISTFILE still buys the shells with no such override: a bash pane writes
	// .bash_history into HOME without it. Both reach the returned env slice
	// through the os.Environ() read below, so they must be set before it.
	t.Setenv("HISTFILE", os.DevNull)
	t.Setenv("ZDOTDIR", shellConfigDirOutsideTempTree(t))

	// Registered after the test's first t.TempDir, whose lone cleanup removes the
	// parent holding homeDir, so LIFO runs this wait before that RemoveAll; a
	// fixture must isolate before starting its server, so kill-server runs earlier
	// still. The writers the env pins above cannot reach get their window to finish.
	registerHomeQuiescenceGuard(t, homeDir)

	// A failed snapshot is fatal: a silently degraded backstop is worse than none.
	devStateDir := resolveDevStateDir()
	var preSnapshot map[string]Fingerprint
	if devStateDir != "" {
		snap, err := SnapshotStateDir(devStateDir)
		if err != nil {
			t.Fatalf("portaltest: snapshot dev state dir %s: %v", devStateDir, err)
		}
		preSnapshot = snap
	}

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("portaltest: mkdir configDir: %v", err)
	}

	stateDir = filepath.Join(configDir, "portal", "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("portaltest: mkdir stateDir: %v", err)
	}

	// Own the state dir in the process env as well as the registry, so a fixture
	// cannot register one directory and write to another. Set before the
	// os.Environ() read below, so the slice carries it too.
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	state.RegisterSandboxStateDir(stateDir)

	// Subprocesses run their own enumerations, where the in-process registration
	// above does not exist; the registry file carries the same ownership across
	// the process boundary. Set it before the env slice is derived from os.Environ
	// below, so the slice carries it.
	registryPath := filepath.Join(tempDir, "sandbox-registry")
	if err := os.WriteFile(registryPath, []byte(stateDir+"\n"), 0o600); err != nil {
		t.Fatalf("portaltest: write sandbox registry: %v", err)
	}
	t.Setenv(state.SandboxRegistryEnv, registryPath)

	env = filterXDGConfigHome(os.Environ())
	env = append(env, "XDG_CONFIG_HOME="+configDir)

	env = filterEnvKeys(env, "PORTAL_STATE_DIR")
	env = append(env, "PORTAL_STATE_DIR="+stateDir)

	// Poison TMUX so a subprocess cannot silently attach to the developer's real
	// server; one that needs a server appends its own TMUX after this slice
	// (exec.Cmd env dedupe is last-wins), and one that forgets fails loudly.
	env = filterEnvKeys(env, "TMUX", "TMUX_PANE")
	env = append(env, "TMUX="+PoisonedTmuxSocket+",0,0")

	if devStateDir != "" {
		installBackstop(t, devStateDir, preSnapshot)
	}

	return env, stateDir
}

// shellConfigDirOutsideTempTree creates the directory ZDOTDIR names and
// registers its removal. It is deliberately not a t.TempDir(): the point is to
// sit outside the tree the framework removes, so a shell still flushing at
// teardown cannot land a file mid-RemoveAll. Registered before the quiescence
// wait, so LIFO removes it after that wait has run.
func shellConfigDirOutsideTempTree(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "portaltest-shellconfig-*")
	if err != nil {
		t.Fatalf("portaltest: mkdir shell config dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// Narrows *testing.T so a fake recorder can drive installBackstopCleanup.
type backstopT interface {
	Cleanup(fn func())
	Errorf(format string, args ...any)
}

// A var so the installer can be substituted and the directory it is handed —
// the one resolved after the HOME scrub — observed.
var installBackstop = installBackstopCleanup

func installBackstopCleanup(t backstopT, devStateDir string, pre map[string]Fingerprint) {
	t.Cleanup(func() {
		reportStateDirDelta(t.Errorf, devStateDir, pre)
	})
}

// PoisonedTmuxSocket is the deliberately-invalid tmux socket path baked into
// IsolateStateForTest's env. A subprocess that needs a real test server must
// override TMUX by appending its own after that slice.
const PoisonedTmuxSocket = "/nonexistent/portal-test-must-set-tmux-socket"

func filterXDGConfigHome(env []string) []string {
	return filterEnvKeys(env, "XDG_CONFIG_HOME")
}

func filterEnvKeys(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		drop := false
		for _, k := range keys {
			if strings.HasPrefix(e, k+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, e)
		}
	}
	return out
}
