package zmx_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/proctable"
)

// zmxProcess is a process with no terminal, exec'd from zmx's executable with
// argv and env, as a session's daemon is.
func zmxProcess(argv []string, env ...string) zmx.Process {
	return zmx.Process{
		Process: proctable.Process{PID: 40, PPID: 1, Command: "zmx"},
		Args:    proctable.Args{Argv: argv, Env: env},
	}
}

// existsOver is Exists of dev, whose socket in dir refuses, over a process
// table holding procs.
func existsOver(t *testing.T, dir string, procs ...zmx.Process) (bool, error) {
	t.Helper()
	b := newBackend(t, dir)
	zmx.FakeProcesses(b, procs, nil)
	return b.Exists(t.Context(), "dev")
}

func TestRefusingSocket_IsHeldByAProcessThatMayHoldItsSession(t *testing.T) {
	runtimeDir := socketDir(t)
	dir := filepath.Join(runtimeDir, "zmx")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	refusing(t, dir, "dev")
	inDir := "ZMX_DIR=" + dir
	asPrivate := "ZMX_DIR=" + filepath.Join("/private", dir)
	link := filepath.Join(socketDir(t), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	renamed := zmxProcess([]string{"zmx", "attach", "dev"}, inDir)
	renamed.Command = "zmx-0.9"

	for _, tt := range []struct {
		name string
		proc zmx.Process
	}{
		{"zmx attach on its name", zmxProcess([]string{"/opt/bin/zmx", "attach", "dev", "/bin/sh"}, inDir)},
		{"zmx a", zmxProcess([]string{"zmx", "a", "dev"}, inDir)},
		{"zmx attach past --labels", zmxProcess([]string{"zmx", "attach", "--labels", "k=v", "dev"}, inDir)},
		{"zmx run", zmxProcess([]string{"zmx", "run", "dev", "make"}, inDir)},
		{"zmx r", zmxProcess([]string{"zmx", "r", "dev", "-d", "make"}, inDir)},
		{"zmx write", zmxProcess([]string{"zmx", "write", "dev", "notes.txt"}, inDir)},
		{"zmx wr", zmxProcess([]string{"zmx", "wr", "dev", "notes.txt"}, inDir)},
		{"zmx attach naming it after a prefix", zmxProcess([]string{"zmx", "attach", "v"}, inDir, "ZMX_SESSION_PREFIX=de")},
		{"zmx run naming the prefix alone", zmxProcess([]string{"zmx", "run"}, inDir, "ZMX_SESSION_PREFIX=dev")},
		{"its directory given with a trailing slash", zmxProcess([]string{"zmx", "attach", "dev"}, inDir+"/")},
		{"its directory under /private, as macOS resolves /tmp", zmxProcess([]string{"zmx", "attach", "dev"}, asPrivate)},
		{"its directory through a symbolic link", zmxProcess([]string{"zmx", "attach", "dev"}, "ZMX_DIR="+link)},
		{"its directory as zmx's default", zmxProcess([]string{"zmx", "attach", "dev"}, "XDG_RUNTIME_DIR="+runtimeDir)},
		{"a relative directory, which may resolve to it", zmxProcess([]string{"zmx", "attach", "dev"}, "ZMX_DIR=zmx")},
		{"a client on a terminal", client(41, []string{"zmx", "attach", "dev"}, inDir)},
		{"a zmx the kernel names otherwise, renamed or wrapped", renamed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := existsOver(t, dir, tt.proc)
			if !errors.Is(err, zmx.ErrRefusing) || !errors.Is(err, backend.ErrTimeout) {
				t.Errorf("Exists(dev) = %v, %v; want ErrRefusing wrapping ErrTimeout", got, err)
			}
			if err != nil && !strings.Contains(err.Error(), fmt.Sprintf("process %d", tt.proc.PID)) {
				t.Errorf("Exists(dev) = %v, want the error to name the process", err)
			}
		})
	}
}

func TestRefusingSocket_IsStaleWhenTheTableProvesNoProcessHoldsIt(t *testing.T) {
	dir := socketDir(t)
	refusing(t, dir, "dev")
	inDir := "ZMX_DIR=" + dir
	exiting := zmxProcess([]string{"zmx", "attach", "dev"}, inDir)
	exiting.Err = fmt.Errorf("read arguments of process 40: %w", proctable.ErrGone)
	otherVerb := zmxProcess([]string{"tmux", "new-session", "dev"}, inDir)
	otherVerb.Command = "tmux"
	hardenedOnAnotherName := zmxProcess([]string{"/opt/homebrew/bin/tart", "run", "kit-vm"})
	hardenedOnAnotherName.Command = "tart"

	for _, tt := range []struct {
		name  string
		procs []zmx.Process
	}{
		{"no process", nil},
		{"zmx kill on its name", []zmx.Process{zmxProcess([]string{"zmx", "kill", "dev"}, inDir)}},
		{"zmx history on its name", []zmx.Process{zmxProcess([]string{"zmx", "history", "dev"}, inDir)}},
		{"zmx attach on another name", []zmx.Process{zmxProcess([]string{"zmx", "attach", "devx"}, inDir)}},
		{"zmx attach on its name under a prefix", []zmx.Process{zmxProcess([]string{"zmx", "attach", "dev"}, inDir, "ZMX_SESSION_PREFIX=p-")}},
		{"zmx attach on its name in another directory", []zmx.Process{zmxProcess([]string{"zmx", "attach", "dev"}, "ZMX_DIR=/tmp/zx-other")}},
		{"zmx attach on its name in zmx's default directory, another", []zmx.Process{zmxProcess([]string{"zmx", "attach", "dev"}, "TMPDIR=/var/t")}},
		{"zmx attach answered with help", []zmx.Process{zmxProcess([]string{"zmx", "attach", "-h", "dev"}, inDir)}},
		{"zmx run answered with help", []zmx.Process{zmxProcess([]string{"zmx", "run", "--help"}, inDir)}},
		{"a zmx exiting as it is read", []zmx.Process{exiting}},
		{"another program naming it after a verb that starts nothing", []zmx.Process{otherVerb}},
		{"a program of that argv shape on a name it can't end with, whose environment the kernel doesn't show", []zmx.Process{hardenedOnAnotherName}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := existsOver(t, dir, tt.procs...); got || err != nil {
				t.Errorf("Exists(dev) = %v, %v; want false, nil: a socket no process holds is stale", got, err)
			}
		})
	}
	if _, err := os.Lstat(filepath.Join(dir, "dev")); err != nil {
		t.Errorf("the stale socket after Exists: %v; only zmx removes it", err)
	}
}

func TestRefusingSocket_StaysRefusingWhenNoProofCanBeRead(t *testing.T) {
	dir := socketDir(t)
	refusing(t, dir, "dev")
	unreadable := zmxProcess(nil)
	unreadable.Err = errors.New("read arguments of process 40: invalid argument")
	unreadableOther := zmxProcess(nil)
	unreadableOther.Command = "launchd"
	unreadableOther.Err = errors.New("operation not permitted")
	hidingItsEnvironment := zmxProcess([]string{"zmx", "attach", "dev"})
	hidingItsPrefix := zmxProcess([]string{"zmx", "attach", "v"})

	for _, tt := range []struct {
		name  string
		procs []zmx.Process
		err   error
	}{
		{"a zmx it can't read", []zmx.Process{unreadable}, nil},
		{"any process it can't read, as its name proves nothing", []zmx.Process{unreadableOther}, nil},
		{"a zmx attach on its name whose environment the kernel doesn't show", []zmx.Process{hidingItsEnvironment}, nil},
		{"a zmx attach on a name it ends with, whose environment, and so prefix, the kernel doesn't show", []zmx.Process{hidingItsPrefix}, nil},
		{"a process table it can't list", nil, errors.New("sysctl failed")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newBackend(t, dir)
			zmx.FakeProcesses(b, tt.procs, tt.err)

			if got, err := b.Exists(t.Context(), "dev"); !errors.Is(err, zmx.ErrRefusing) {
				t.Errorf("Exists(dev) = %v, %v; want ErrRefusing: a proof that can't be read proves nothing", got, err)
			}
		})
	}
}

func TestProcessTable_IsReadOnlyOnARefusal(t *testing.T) {
	dir := socketDir(t)
	serve(t, dir, "live", answering(infoFrame(t)))
	stale(t, dir, "crashed")
	b := newBackend(t, dir)
	zmx.FakeProcessesFrom(b, func() ([]zmx.Process, error) {
		t.Error("the process table was read for a socket that did not refuse, or one from before this boot")
		return nil, nil
	})

	for _, name := range []string{"live", "crashed", "gone"} {
		if _, err := b.Exists(t.Context(), name); err != nil {
			t.Errorf("Exists(%s): %v", name, err)
		}
	}
	if _, err := b.List(t.Context()); err != nil {
		t.Errorf("List: %v", err)
	}
}
