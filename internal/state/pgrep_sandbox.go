//go:build integration

package state

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Daemon-pgrep test sandbox, integration builds only — pgrep_sandbox_prod.go
// supplies inert stubs for every other build. The orphan sweep only SIGKILLs
// pids PgrepPortalDaemons returns, so once the sandbox is on, by
// EnableDaemonSandbox in this process or SandboxRegistryEnv naming a registry
// for a test's subprocess, it filters default-deny, and no sweep can reach the
// developer's live daemon. Until then it filters nothing, so a test must turn
// it on, as IsolateStateForTest does, before anything enumerates daemons.
// Ownership is keyed on the state directory, not the pid: the saver respawns
// during bootstrap, but each incarnation rewrites <stateDir>/daemon.pid.

var (
	sandboxMu        sync.Mutex
	sandboxEnabled   bool
	sandboxOwnedPID  map[int]bool
	sandboxOwnedDirs map[string]bool
	sandboxSources   []func() (int, bool)
)

// EnableDaemonSandbox turns on default-deny filtering for this process, and is
// idempotent.
func EnableDaemonSandbox() {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	sandboxEnabled = true
	if sandboxOwnedPID == nil {
		sandboxOwnedPID = make(map[int]bool)
	}
	if sandboxOwnedDirs == nil {
		sandboxOwnedDirs = make(map[string]bool)
	}
}

// RegisterSandboxStateDir marks dir as test-owned: its current daemon.pid
// counts as owned on every enumeration, so a respawn needs no re-registration.
func RegisterSandboxStateDir(dir string) {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	if sandboxOwnedDirs == nil {
		sandboxOwnedDirs = make(map[string]bool)
	}
	sandboxOwnedDirs[dir] = true
}

// RegisterSandboxDaemon records an explicit test-owned pid, for a daemon that
// never owns a registered state dir's daemon.pid.
func RegisterSandboxDaemon(pid int) {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	if sandboxOwnedPID == nil {
		sandboxOwnedPID = make(map[int]bool)
	}
	sandboxOwnedPID[pid] = true
}

// RegisterSandboxDaemonSource takes a callback yielding a currently-live
// test-owned pid, or false. It lives in the test package so this one need not
// know where the pid comes from.
func RegisterSandboxDaemonSource(fn func() (int, bool)) {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	sandboxSources = append(sandboxSources, fn)
}

// ResetDaemonSandbox clears the registry so sandbox state cannot bleed tests.
func ResetDaemonSandbox() {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	sandboxEnabled = false
	sandboxOwnedPID = nil
	sandboxOwnedDirs = nil
	sandboxSources = nil
}

func sandboxFilterPgrep(pids []int) []int {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	registryDirs, registryActive := registrySandboxDirs()
	if !sandboxEnabled && !registryActive {
		return pids
	}
	owned := make(map[int]bool, len(sandboxOwnedPID)+len(sandboxOwnedDirs)+len(registryDirs))
	for p := range sandboxOwnedPID {
		owned[p] = true
	}
	for dir := range sandboxOwnedDirs {
		if p, ok := readDaemonPIDFile(dir); ok {
			owned[p] = true
		}
	}
	for _, dir := range registryDirs {
		if p, ok := readDaemonPIDFile(dir); ok {
			owned[p] = true
		}
	}
	for _, src := range sandboxSources {
		if p, ok := src(); ok {
			owned[p] = true
		}
	}
	out := make([]int, 0, len(pids))
	for _, p := range pids {
		if owned[p] {
			out = append(out, p)
		}
	}
	return out
}

// A set env var naming an unreadable file yields active with zero owned dirs, so
// a subprocess sweep kills nothing. Re-read on every enumeration, so dirs
// appended after a subprocess was spawned are honoured.
func registrySandboxDirs() (dirs []string, active bool) {
	path := os.Getenv(SandboxRegistryEnv)
	if path == "" {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, true
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		dir := strings.TrimSpace(line)
		if dir == "" {
			continue
		}
		dirs = append(dirs, dir)
	}
	return dirs, true
}

func readDaemonPIDFile(stateDir string) (int, bool) {
	b, err := os.ReadFile(filepath.Join(stateDir, "daemon.pid"))
	if err != nil {
		return 0, false
	}
	p, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || p <= 0 {
		return 0, false
	}
	return p, true
}
