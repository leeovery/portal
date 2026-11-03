package proctable

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestReadsThisProcess(t *testing.T) {
	t.Run("it reads this process's own argv and environment", func(t *testing.T) {
		got, err := ReadArgs(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got.Argv, os.Args) {
			t.Errorf("argv = %q, want %q", got.Argv, os.Args)
		}
		if want := "PROCTABLE_PROBE=1"; !slices.Contains(argsOfChild(t, want).Env, want) {
			t.Errorf("a child's environment lacks %s", want)
		}
	})

	t.Run("it lists a process under the name of the file it exec'd, whatever its argv[0]", func(t *testing.T) {
		child := &exec.Cmd{Path: "/bin/sleep", Args: []string{"zmx", "30"}}
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		reap(t, child)
		list, err := List()
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(list, func(p Process) bool { return p.PID == child.Process.Pid })
		if i < 0 {
			t.Fatalf("List lacks the child %d", child.Process.Pid)
		}
		if list[i].Command != "sleep" {
			t.Errorf("Command = %q, want sleep", list[i].Command)
		}
	})

	t.Run("it lists this process with its parent", func(t *testing.T) {
		list, err := List()
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(list, func(p Process) bool { return p.PID == os.Getpid() })
		if i < 0 {
			t.Fatalf("List lacks this process %d", os.Getpid())
		}
		if list[i].PPID != os.Getppid() {
			t.Errorf("PPID = %d, want %d", list[i].PPID, os.Getppid())
		}
		if parent, err := Parent(os.Getpid()); err != nil || parent != os.Getppid() {
			t.Errorf("Parent = %d, %v; want %d", parent, err, os.Getppid())
		}
	})
}

func TestReadArgsKeepsAnEmptyArgvZero(t *testing.T) {
	dir := t.TempDir()
	// Executable paths of each length modulo the padding, so argv starts at
	// every offset the padding can leave it at.
	for n := 1; n <= pathAlign; n++ {
		link := filepath.Join(dir, strings.Repeat("s", n))
		if err := os.Symlink("/bin/sleep", link); err != nil {
			t.Fatal(err)
		}
		child := &exec.Cmd{Path: link, Args: []string{"", "30"}}
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		reap(t, child)

		if got, err := ReadArgs(child.Process.Pid); err != nil || !slices.Equal(got.Argv, child.Args) {
			t.Errorf("ReadArgs(exec'd from %s) = %q, %v; want argv %q", link, got.Argv, err, child.Args)
		}
	}
}

func TestReadArgsTellsAGoneProcess(t *testing.T) {
	t.Run("a zombie is gone", func(t *testing.T) {
		child := exec.Command("/usr/bin/true")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = child.Wait() }()
		var err error
		for range 200 {
			if _, err = ReadArgs(child.Process.Pid); err != nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !errors.Is(err, ErrGone) {
			t.Errorf("ReadArgs(zombie) = %v, want ErrGone", err)
		}
	})

	t.Run("a reaped process is gone", func(t *testing.T) {
		child := exec.Command("/usr/bin/true")
		if err := child.Run(); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadArgs(child.Process.Pid); !errors.Is(err, ErrGone) {
			t.Errorf("ReadArgs(reaped) = %v, want ErrGone", err)
		}
	})

	t.Run("another user's running process is not", func(t *testing.T) {
		if _, err := ReadArgs(1); err == nil || errors.Is(err, ErrGone) {
			t.Errorf("ReadArgs(launchd) = %v, want an error other than ErrGone", err)
		}
	})
}

func TestListTellsWhetherAProcessHasATerminal(t *testing.T) {
	onTerminal := exec.Command("sleep", "30")
	tty, err := pty.Start(onTerminal)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	reap(t, onTerminal)

	detached := exec.Command("sleep", "30")
	detached.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := detached.Start(); err != nil {
		t.Fatal(err)
	}
	reap(t, detached)

	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	for pid, want := range map[int]bool{onTerminal.Process.Pid: true, detached.Process.Pid: false} {
		i := slices.IndexFunc(list, func(p Process) bool { return p.PID == pid })
		if i < 0 {
			t.Fatalf("List lacks process %d", pid)
		}
		if list[i].Terminal != want {
			t.Errorf("process %d: Terminal = %v, want %v", pid, list[i].Terminal, want)
		}
	}
}

// argsOfChild starts a child with env added to this process's environment,
// and reads its arguments.
func argsOfChild(t *testing.T, env string) Args {
	t.Helper()
	child := exec.Command("sleep", "30")
	child.Env = append(os.Environ(), env)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	reap(t, child)
	got, err := ReadArgs(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func reap(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
}
