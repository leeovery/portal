//go:build integration

package spawn

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/runnertoken"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

// settle bounds each wait on Portal, zmx or a session's shell.
const settle = 20 * time.Second

// sessionIn is b as a process inside session reads it.
type sessionIn struct {
	*zmx.Backend
	session string
}

func (s sessionIn) Current() string { return s.session }

// zmxWorld is a zmx directory and a state directory of the test's own, and
// the environment a process attached to its sessions runs in.
type zmxWorld struct {
	b        *zmx.Backend
	stateDir string
	env      []string
}

func newZmxWorld(t *testing.T) *zmxWorld {
	t.Helper()
	env, _ := portaltest.IsolateStateForTest(t)
	// A runner's socket lives in the state directory, and one under macOS's
	// $TMPDIR is too long for a socket's address.
	stateDir, err := os.MkdirTemp("/tmp", "ps-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateDir) })
	portaltest.OwnStateDir(t, stateDir)
	b := zmxtest.New(t)
	env = append(env,
		"PORTAL_STATE_DIR="+stateDir,
		"ZMX_DIR="+b.Dir(),
		"PATH="+filepath.Dir(zmxtest.Binary(t))+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PORTAL_PROJECTS_FILE="+filepath.Join(t.TempDir(), "projects.json"),
		"PORTAL_PREFS_FILE="+filepath.Join(t.TempDir(), "prefs.json"),
		"SHELL=/bin/sh",
		"TERM=xterm-256color",
	)
	return &zmxWorld{b: b, stateDir: stateDir, env: env}
}

func (w *zmxWorld) create(t *testing.T, name string) {
	t.Helper()
	if _, err := w.b.Create(t.Context(), name, t.TempDir(), nil); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

// onTerminal runs argv on a pseudo-terminal of its own, in the world's
// environment, until the test ends, and returns its pid and terminal.
func (w *zmxWorld) onTerminal(t *testing.T, argv ...string) (int, *os.File) {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = w.env
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		t.Fatalf("start %s on a terminal: %v", argv[0], err)
	}
	go drain(tty)
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = tty.Close()
	})
	return cmd.Process.Pid, tty
}

func drain(tty *os.File) {
	buf := make([]byte, 4096)
	for {
		if _, err := tty.Read(buf); err != nil {
			return
		}
	}
}

// detector detects inside session as a process there would, through the
// world's zmx, registry and process table, walking every process in walks
// to Ghostty and failing a walk from any other.
func (w *zmxWorld) detector(session string, walks ...int) *Detector {
	walker := &fakeWalker{procs: map[int]fakeProc{}}
	for _, pid := range walks {
		walker.procs[pid] = fakeProc{ppid: 1, command: ghosttyCommand}
	}
	_, reader := localWalkSeams()
	return &Detector{
		sessions: sessionIn{Backend: w.b, session: session},
		runners:  runnerreg.New(w.stateDir),
		clients:  w.b,
		walker:   walker,
		reader:   reader,
	}
}

// awaitDetection waits for detection to find what want accepts, and returns
// why it found no terminal, nil when it found one.
func awaitDetection(t *testing.T, d *Detector, want func(Identity, string, error) bool) error {
	t.Helper()
	var (
		id    Identity
		route string
		err   error
	)
	if !harnesstest.PollUntil(t, settle, 50*time.Millisecond, func() bool {
		id, route, err = d.resolve()
		return want(id, route, err)
	}) {
		t.Fatalf("detection = %+v, %q, %v; never what the test waits for", id, route, err)
	}
	return err
}

// awaitResolved waits for detection to find Ghostty through route.
func awaitResolved(t *testing.T, d *Detector, route string) {
	t.Helper()
	_ = awaitDetection(t, d, func(id Identity, got string, err error) bool {
		return err == nil && got == route && id.BundleID == "com.mitchellh.ghostty"
	})
}

// awaitUncertain waits for detection through route to find no one terminal
// leading the session, and returns why.
func awaitUncertain(t *testing.T, d *Detector, route string) error {
	t.Helper()
	return awaitDetection(t, d, func(_ Identity, got string, err error) bool {
		return errors.Is(err, errUncertain) && got == route
	})
}

func TestDetectInsideSession_OverZmx(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "portal")
	if err := portalbintest.BuildPortalBinary(filepath.Dir(binary)); err != nil {
		t.Fatalf("build portal: %v", err)
	}

	t.Run("it walks from the runner whose client leads the session", func(t *testing.T) {
		w := newZmxWorld(t)
		w.create(t, "api-x7Kd9a")
		pid, _ := w.onTerminal(t, binary, "open", "-s", "api-x7Kd9a")

		awaitResolved(t, w.detector("api-x7Kd9a", pid), routeInsideRunner)
	})

	t.Run("a runner switched outside Portal and back, unsure, confirms through the leader and is walked from", func(t *testing.T) {
		w := newZmxWorld(t)
		w.env = append(w.env, "PORTAL_LOG_LEVEL=debug")
		w.create(t, "api-x7Kd9a")
		w.create(t, "blog-c3d4")
		pid, tty := w.onTerminal(t, binary, "open", "-s", "api-x7Kd9a")
		awaitResolved(t, w.detector("api-x7Kd9a", pid), routeInsideRunner)

		typeZmx(t, tty, "attach blog-c3d4")
		awaitLeader(t, w.b, "blog-c3d4")
		typeZmx(t, tty, "attach api-x7Kd9a")
		awaitLeader(t, w.b, "api-x7Kd9a")
		if !harnesstest.PollUntil(t, settle, 50*time.Millisecond, func() bool {
			return strings.Contains(portaltest.ReadPortalLogSafe(w.stateDir), "switch seen")
		}) {
			t.Fatalf("the runner never saw the switch:\n%s", portaltest.ReadPortalLogSafe(w.stateDir))
		}
		portaltest.AssertLogLevelResolved(t, state.PortalLog(w.stateDir), pid, "debug")

		awaitResolved(t, w.detector("api-x7Kd9a", pid), routeInsideRunner)
	})

	t.Run("it walks from a raw zmx client, the session's only one, and never from zmx's daemon", func(t *testing.T) {
		w := newZmxWorld(t)
		w.create(t, "api-x7Kd9a")
		pid, _ := w.onTerminal(t, zmxtest.Binary(t), "attach", "api-x7Kd9a")

		awaitResolved(t, w.detector("api-x7Kd9a", pid), routeInsideRaw)
	})

	t.Run("two raw clients refuse", func(t *testing.T) {
		w := newZmxWorld(t)
		w.create(t, "api-x7Kd9a")
		first, _ := w.onTerminal(t, zmxtest.Binary(t), "attach", "api-x7Kd9a")
		awaitResolved(t, w.detector("api-x7Kd9a", first), routeInsideRaw)
		second, _ := w.onTerminal(t, zmxtest.Binary(t), "attach", "api-x7Kd9a")

		if err := awaitUncertain(t, w.detector("api-x7Kd9a", first, second), routeInsideRaw); !strings.Contains(err.Error(), "2 of 2 zmx attach processes") {
			t.Errorf("detection = %v, want two raw clients", err)
		}
	})

	// zmx switches a raw client in-process, so the process table still shows
	// it under the session it attached: the one zmx attach naming the session
	// it now leads is the runner's own, which is never the leader.
	t.Run("a raw client switched in to lead a runner's session refuses, never walking from the runner's client", func(t *testing.T) {
		w := newZmxWorld(t)
		w.create(t, "api-x7Kd9a")
		w.create(t, "blog-c3d4")
		runner, _ := w.onTerminal(t, binary, "open", "-s", "blog-c3d4")
		awaitResolved(t, w.detector("blog-c3d4", runner), routeInsideRunner)
		raw, tty := w.onTerminal(t, zmxtest.Binary(t), "attach", "api-x7Kd9a")
		awaitResolved(t, w.detector("api-x7Kd9a", raw), routeInsideRaw)

		typeZmx(t, tty, "attach blog-c3d4")
		awaitRawLeader(t, w.b, tty, "blog-c3d4")

		if err := awaitUncertain(t, w.detector("blog-c3d4", runner, raw), routeInsideRaw); !strings.Contains(err.Error(), "0 of 1 zmx attach processes") {
			t.Errorf("detection = %v, want the runner's client alone, passed over", err)
		}
	})

	t.Run("a session nobody leads refuses", func(t *testing.T) {
		w := newZmxWorld(t)
		w.create(t, "api-x7Kd9a")

		if _, route, err := w.detector("api-x7Kd9a").resolve(); !errors.Is(err, errUncertain) || route != routeInside {
			t.Errorf("detection = %q, %v; want uncertain, no leader", route, err)
		}
	})
}

// typeZmx types a zmx command into the session tty shows, putting the pinned
// zmx first on the login shell's PATH, which macOS's path_helper reorders.
func typeZmx(t *testing.T, tty *os.File, args string) {
	t.Helper()
	if _, err := tty.WriteString("PATH=" + filepath.Dir(zmxtest.Binary(t)) + ":$PATH zmx " + args + "\r"); err != nil {
		t.Fatalf("type zmx %s: %v", args, err)
	}
}

// awaitLeader waits for a client to lead session.
func awaitLeader(t *testing.T, b *zmx.Backend, session string) {
	t.Helper()
	var env backend.ClientEnv
	if !harnesstest.PollUntil(t, settle, 50*time.Millisecond, func() bool {
		var err error
		env, err = b.LeaderEnv(context.WithoutCancel(t.Context()), session)
		return err == nil && len(env) > 0
	}) {
		t.Fatalf("no client leads %s: %v", session, env)
	}
}

// awaitRawLeader types into tty until a client with no runner's token leads
// session.
func awaitRawLeader(t *testing.T, b *zmx.Backend, tty *os.File, session string) {
	t.Helper()
	var env backend.ClientEnv
	if !harnesstest.PollUntil(t, settle, 100*time.Millisecond, func() bool {
		if _, err := tty.WriteString(" "); err != nil {
			t.Fatalf("type into the raw client: %v", err)
		}
		var err error
		env, err = b.LeaderEnv(context.WithoutCancel(t.Context()), session)
		_, runner := env.Lookup(runnertoken.Var)
		return err == nil && len(env) > 0 && !runner
	}) {
		t.Fatalf("no raw client leads %s: %v", session, env)
	}
}
