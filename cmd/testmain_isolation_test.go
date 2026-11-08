package cmd

// TestMain poisons the PORTAL_* config paths, ZMX_DIR and the terminal
// adapters package-wide so a test that forgets to isolate fails loudly, and HOME
// to a per-run temp directory — with XDG_CONFIG_HOME emptied so HOME is what
// decides — so one that resolves a default config path lands there instead of
// reading or mutating the developer's real config; subprocesses inherit the
// poison via os.Environ().
//
// A missed backend injection under the ZMX_DIR poison fails with ENOTDIR: the
// poison sits under /dev/null, so no read of it can pass for an empty listing.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

// testBoot is the boot the package's claims judge by: before every creation
// a fixture records, unless it means one saved before the boot.
var testBoot = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

// testClaimRuleRestoring is the rule the package's claims judge by: entries
// by testBoot, adopting by restoring when given, and, as state.ClaimRule
// does, the hydrating flags in dir and the names names refuses.
func testClaimRuleRestoring(dir string, names func(string) error, restoring func(string) bool) sessionindex.Rule {
	return sessionindex.NewRule(testBoot, restoring).
		ReadingHydrating(func(name string) bool { return state.Hydrating.Raised(dir, name) }).
		RefusingNames(names)
}

func testClaimRule(dir string, names func(string) error) sessionindex.Rule {
	return testClaimRuleRestoring(dir, names, nil)
}

// pinToolchainCaches re-exports the cache locations the Go toolchain would
// otherwise derive from HOME, so a subprocess `go build` keeps using the
// developer's caches once HOME is poisoned rather than re-downloading every
// module into the temp home and leaving a read-only cache there that the run's
// own cleanup cannot remove. The values come from the toolchain rather than
// being rebuilt from the pre-poison home, so an explicitly configured cache is
// honoured instead of overwritten with the default.
//
// A test binary a test re-executes inherits both, already pinned, along with
// a HOME some earlier re-execution's TestMain poisoned beneath the test's own
// temp dir. It runs no `go` then: the toolchain writes its telemetry under
// HOME, and a telemetry process outliving the command races that test's
// TempDir cleanup.
func pinToolchainCaches() error {
	names := []string{"GOMODCACHE", "GOCACHE"}
	if !slices.ContainsFunc(names, func(name string) bool { return !filepath.IsAbs(os.Getenv(name)) }) {
		return nil
	}

	out, err := exec.Command("go", append([]string{"env"}, names...)...).Output()
	if err != nil {
		return fmt.Errorf("go env %s: %w", strings.Join(names, " "), err)
	}

	values := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(values) != len(names) {
		return fmt.Errorf("go env %s: got %d values, want %d", strings.Join(names, " "), len(values), len(names))
	}

	for i, name := range names {
		value := strings.TrimSpace(values[i])
		if !filepath.IsAbs(value) {
			return fmt.Errorf("go env %s = %q is not an absolute path", name, value)
		}
		os.Setenv(name, value)
	}
	return nil
}

func TestMain(m *testing.M) {
	// Before the poison, while the toolchain still resolves against the real home.
	if err := pinToolchainCaches(); err != nil {
		fmt.Fprintf(os.Stderr, "cmd tests: pinning the toolchain caches ahead of the HOME poison: %v\n", err)
		os.Exit(1)
	}

	// A real directory rather than a /nonexistent path: a resolve against an absent
	// home succeeds and then fails at the write, so only a writable home can stand in
	// for the developer's and for what a resolve goes on to create.
	homeDir, err := os.MkdirTemp("", "portal-test-home")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmd tests: creating the package-wide temp HOME: %v\n", err)
		os.Exit(1)
	}
	os.Setenv("HOME", homeDir)
	// The home poison only decides a default resolve while XDG_CONFIG_HOME is
	// empty, so it is emptied here too: on a machine where the developer sets
	// it, a test resolving a non-overridden config path would read their real
	// config base instead of the temp home.
	os.Setenv("XDG_CONFIG_HOME", "")

	os.Setenv("PORTAL_STATE_DIR", "/nonexistent/portal-test-must-isolate-state")
	os.Setenv("PORTAL_HOOKS_FILE", "/nonexistent/portal-test-must-isolate-hooks.json")
	os.Setenv("PORTAL_PROJECTS_FILE", "/nonexistent/portal-test-must-isolate-projects.json")
	os.Setenv("PORTAL_ALIASES_FILE", "/nonexistent/portal-test-must-isolate-aliases")
	os.Setenv("PORTAL_THEMES_DIR", "/nonexistent/portal-test-must-isolate-themes")
	os.Setenv("PORTAL_PREFS_FILE", "/nonexistent/portal-test-must-isolate-prefs.json")
	os.Setenv("PORTAL_TERMINALS_FILE", "/nonexistent/portal-test-must-isolate-terminals.json")
	// ZMX_SESSION would turn an attach into a switch of a live session, and
	// ZMX_SESSION_PREFIX would rename every session a test names.
	for _, err := range []error{
		os.Setenv("ZMX_DIR", zmxtest.PoisonedDir),
		os.Unsetenv("ZMX_SESSION"),
		os.Unsetenv("ZMX_SESSION_PREFIX"),
	} {
		if err != nil {
			fmt.Fprintf(os.Stderr, "cmd tests: poisoning the zmx environment: %v\n", err)
			os.Exit(1)
		}
	}
	// Neutralised package-wide: production dispatches this on its own goroutine, so
	// a test reaching loadPrefsStore would leave a write racing its own teardown.
	persistTranslation = func(*prefs.Store, string) {}
	// Fixed package-wide, so what a fixture's dates make of an entry is the
	// same on every machine, whenever it booted. It reads the hydrating flag
	// and the names as state.ClaimRule does; that rule's own suite pins the
	// adoption by flags this one leaves out.
	claimRule, silentClaimRule = testClaimRule, testClaimRule
	// A test reaching the exit seam unfaked would end the whole test binary, its
	// cleanups unrun; it panics here instead.
	osExit = func(code int) {
		panic(fmt.Sprintf("cmd tests: osExit(%d) reached without withOsExitFake", code))
	}
	// A test's backend reports pids that are live processes on the
	// developer's machine, none of them the test's.
	signalProcess = func(pid int, sig syscall.Signal) error {
		panic(fmt.Sprintf("cmd tests: signalProcess(%d, %v) reached without withFuncSeam", pid, sig))
	}
	// The saver and restore run what this resolves, and the test binary is
	// no portal: a test reaching the real bootstrap panics here.
	bootstrapExe = func() (string, error) {
		panic("cmd tests: bootstrapExe reached without withBootstrapDeps")
	}

	// Detection reads the terminal's identity from the environment, scrubbed
	// here, and from this process's ancestry, which leads to the developer's own
	// terminal whatever the environment says. So a test that reaches a
	// production adapter panics rather than open a window there.
	for _, name := range spawn.IdentityVars() {
		if err := os.Unsetenv(name); err != nil {
			fmt.Fprintf(os.Stderr, "cmd tests: scrubbing the terminal's identity: %v\n", err)
			os.Exit(1)
		}
	}
	spawn.PoisonAdaptersForTest()

	code := m.Run()
	if err := os.RemoveAll(homeDir); err != nil {
		fmt.Fprintf(os.Stderr, "cmd tests: removing the package-wide temp HOME: %v\n", err)
	}
	os.Exit(code)
}
