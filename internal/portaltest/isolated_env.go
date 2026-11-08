package portaltest

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/xdg"
	"github.com/leeovery/portal/internal/zmxtest"
)

// IsolateStateForTest returns an env slice and a stateDir scoped to a per-test
// t.TempDir(); assign env to exec.Cmd.Env, then append your own ZMX_DIR=<test
// dir> (see zmxtest.PoisonedDir). stateDir exists on return, and both the
// calling process's PORTAL_STATE_DIR and the returned slice's name it — the same
// directory the sandbox registry is given. Every other Portal file and directory
// variable names its place in the same scratch config, in the process and the
// slice alike, so no export of the developer's reaches a test. A fixture wanting
// a different one overrides it after the call: the process env via t.Setenv,
// the slice by appending (exec.Cmd env dedupe is last-wins). It also mutates the
// calling process's own env via t.Setenv and registers a cleanup that fails the
// test if the state dir under the scrubbed HOME changed during the run (see
// resolveDevStateDir for why the backstop's reach stops there).
//
// The slice is the process's environment with the isolation over it. A fixture
// passing a process nothing else of the developer's takes the isolation alone,
// as StartSaverDaemon does.
func IsolateStateForTest(t *testing.T) (env []string, stateDir string) {
	t.Helper()
	isolation, stateDir := isolate(t)
	return overlay(os.Environ(), isolation, unisolatedKeys...), stateDir
}

// unisolatedKeys are dropped from the environment the slice starts from: a
// subprocess runs in no tmux pane and no zmx session of the developer's.
var unisolatedKeys = []string{"TMUX", "TMUX_PANE", "ZMX_SESSION", "ZMX_SESSION_PREFIX"}

// isolate sets up IsolateStateForTest's isolation in the process and returns
// the entries that carry it to a subprocess: the scrubbed home and shell
// files, every Portal path in the scratch config, the sandbox registry, and
// zmx poisoned.
func isolate(t *testing.T) (isolation []string, stateDir string) {
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

	// Shells hosted by a test's zmx sessions flush per-session files as they exit,
	// racing the framework's RemoveAll of the temp HOME. zsh's history is that
	// writer: /etc/zshrc assigns HISTFILE=${ZDOTDIR:-$HOME}/.zsh_history for every
	// interactive shell whatever it inherited, so only ZDOTDIR moves it — and it
	// must name a directory outside the tree the framework removes, which homeDir
	// is not. zsh's session directory hangs off the same prefix and moves with it.
	// HISTFILE still buys the shells with no such override: a bash session writes
	// .bash_history into HOME without it.
	zdotdir := shellConfigDirOutsideTempTree(t)
	t.Setenv("HISTFILE", os.DevNull)
	t.Setenv("ZDOTDIR", zdotdir)

	// Registered after the test's first t.TempDir, whose lone cleanup removes the
	// parent holding homeDir, so LIFO runs this wait before that RemoveAll; a
	// fixture must isolate before starting its sessions, so their kill runs
	// earlier still. The writers the env pins above cannot reach get their window to finish.
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

	// Every Portal path names its place in the scratch config, in the process
	// env as well as the slice: an export of the developer's, such as
	// PORTAL_HOOKS_FILE, would otherwise send a test's writes to their file.
	// Owning the state dir in both, as well as the registry, keeps a fixture
	// from registering one directory and writing to another.
	paths := configPaths(t, configDir)
	for _, entry := range paths {
		key, value, _ := strings.Cut(entry, "=")
		t.Setenv(key, value)
	}
	stateDir = xdg.EnvSlice(paths)(xdg.StateDir.EnvVar)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("portaltest: mkdir stateDir: %v", err)
	}

	state.RegisterSandboxStateDir(stateDir)

	// Subprocesses run their own enumerations, where the in-process registration
	// above does not exist; the registry file carries the same ownership across
	// the process boundary.
	registryPath := filepath.Join(tempDir, "sandbox-registry")
	if err := os.WriteFile(registryPath, []byte(stateDir+"\n"), 0o600); err != nil {
		t.Fatalf("portaltest: write sandbox registry: %v", err)
	}
	t.Setenv(state.SandboxRegistryEnv, registryPath)

	if devStateDir != "" {
		installBackstop(t, devStateDir, preSnapshot)
	}

	isolation = append([]string{
		"HOME=" + homeDir,
		"XDG_CONFIG_HOME=" + configDir,
		"HISTFILE=" + os.DevNull,
		"ZDOTDIR=" + zdotdir,
		state.SandboxRegistryEnv + "=" + registryPath,
		// A subprocess that needs sessions appends its own ZMX_DIR after this
		// slice (exec.Cmd env dedupe is last-wins), and one that forgets fails
		// loudly.
		"ZMX_DIR=" + zmxtest.PoisonedDir,
	}, paths...)
	return isolation, stateDir
}

// configPaths names each Portal file and directory where the default resolve
// finds it under configDir, as KEY=value entries keyed by the variable that
// overrides it.
func configPaths(t *testing.T, configDir string) []string {
	t.Helper()
	base := xdg.EnvSlice([]string{"XDG_CONFIG_HOME=" + configDir})
	var paths []string
	for _, id := range xdg.Files() {
		file, err := xdg.ConfigFilePath(base, id)
		if err != nil {
			t.Fatalf("portaltest: resolve %s: %v", id.Filename, err)
		}
		paths = append(paths, id.EnvVar+"="+file.Path)
	}
	for _, id := range xdg.Dirs() {
		dir, err := xdg.ConfigDirPath(base, id)
		if err != nil {
			t.Fatalf("portaltest: resolve %s: %v", id.Dirname, err)
		}
		paths = append(paths, id.EnvVar+"="+dir)
	}
	return paths
}

// overlay is environ with entries in place of every variable they set, and
// without the variables drop names.
func overlay(environ, entries []string, drop ...string) []string {
	keys := slices.Clone(drop)
	for _, entry := range entries {
		key, _, _ := strings.Cut(entry, "=")
		keys = append(keys, key)
	}
	return append(filterEnvKeys(environ, keys...), entries...)
}

// OwnStateDir hands dir the ownership IsolateStateForTest gave the state dir
// it made, for a fixture whose processes use another, such as one short enough
// for unix sockets: the sandbox surfaces daemons on dir, in this process and
// in subprocesses, and this process's PORTAL_STATE_DIR names it. The fixture
// still appends PORTAL_STATE_DIR=dir to its subprocesses' env slice. The
// fingerprint backstop needs nothing: it watches the default path under the
// scrubbed HOME, wherever the state dir is.
func OwnStateDir(t *testing.T, dir string) {
	t.Helper()
	if os.Getenv(state.SandboxRegistryEnv) == "" {
		t.Fatal("portaltest: OwnStateDir runs after IsolateStateForTest")
	}
	t.Setenv("PORTAL_STATE_DIR", dir)
	state.RegisterSandboxStateDir(dir)
	appendSandboxRegistryDir(t, dir)
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

// appendSandboxRegistryDir extends a state dir's daemon ownership to
// subprocess enumerations; the registry file is re-read each time, so
// appending after the env slice was built is fine.
func appendSandboxRegistryDir(t *testing.T, dir string) {
	t.Helper()
	path := os.Getenv(state.SandboxRegistryEnv)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("portaltest: open sandbox registry for append: %v", err)
	}
	if _, err := f.WriteString(dir + "\n"); err != nil {
		_ = f.Close()
		t.Fatalf("portaltest: append sandbox registry: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("portaltest: close sandbox registry: %v", err)
	}
}
