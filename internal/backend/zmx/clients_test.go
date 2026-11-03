package zmx_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/proctable"
)

// client is a process on a terminal exec'd from zmx's executable with argv
// and env.
func client(pid int, argv []string, env ...string) zmx.Process {
	return zmx.Process{
		Process: proctable.Process{PID: pid, PPID: 1, Terminal: true, Command: "zmx"},
		Args:    proctable.Args{Argv: argv, Env: env},
	}
}

// otherProgram is client exec'd from another program's executable.
func otherProgram(pid int, argv []string, env ...string) zmx.Process {
	p := client(pid, argv, env...)
	p.Command = "zsh"
	return p
}

func TestAttachClients(t *testing.T) {
	const dir = "/tmp/zx-clients"
	inDir := "ZMX_DIR=" + dir
	daemon := client(9, []string{"/opt/bin/zmx", "attach", "dev"}, inDir)
	daemon.Terminal = false
	exited := client(8, []string{"zmx", "attach", "dev"}, inDir)
	exited.Err = fmt.Errorf("read arguments of process 8: %w", proctable.ErrGone)

	procs := []zmx.Process{
		client(10, []string{"/opt/bin/zmx", "attach", "dev"}, inDir),
		client(11, []string{"zmx", "a", "dev"}, inDir),
		client(12, []string{"zmx", "attach", "--labels", "k=v", "dev", "vim"}, inDir),
		client(13, []string{"zmx", "attach", "--labels=k=v", "dev"}, inDir),
		client(14, []string{"zmx", "attach", "--labels", "a=1", "--labels=b=2", "dev"}, inDir),
		client(15, []string{"zmx", "attach", "v"}, inDir, "ZMX_SESSION_PREFIX=de"),
		client(16, []string{"zmx", "attach"}, inDir, "ZMX_SESSION_PREFIX=dev"),
		client(17, []string{"zmx", "attach", "dev", "--labels", "k=v"}, inDir),
		daemon,
		exited,
		client(20, []string{"zmx", "attach", "dev"}, "ZMX_DIR=/tmp/zx-other"),
		client(21, []string{"zmx", "attach", "dev"}, "HOME=/h", "TMPDIR=/var/t"),
		client(22, []string{"zmx", "attach", "dev"}, inDir, "ZMX_SESSION_PREFIX=p-"),
		client(23, []string{"zmx", "attach", "devx"}, inDir),
		client(24, []string{"zmx", "kill", "dev"}, inDir),
		client(25, []string{"zmxd", "attach", "dev"}, inDir),
		client(26, []string{"zmx", "attach", "--labels"}, inDir),
		client(27, []string{"zmx", "attach"}, inDir),
		client(28, []string{"zmx", "attach", "-h", "dev"}, inDir),
		client(29, []string{"zmx", "attach", "--labels", "dev"}, inDir),
		client(30, []string{"zmx", "attach", "x", "dev"}, inDir),
		otherProgram(31, []string{"zmx", "attach", "dev"}, inDir),
	}

	b := newBackend(t, dir)
	zmx.FakeProcesses(b, procs, nil)

	got, err := b.AttachClients("dev")
	if err != nil {
		t.Fatalf("AttachClients(dev): %v", err)
	}

	// 10–14 name dev in the directory, among --labels in either form, once or
	// more; 15 reaches dev through its prefix and 16 by its prefix alone; 17's
	// --labels comes after the name, so belongs to its command. The daemon has
	// no terminal and 8 exited as it was read. 20–23 attach elsewhere; 24–28
	// don't attach, or get zmx's help or an error; 29's --labels takes dev as
	// its value, naming no session; 30 names x. 31 calls itself zmx, but the
	// kernel names another program.
	pids := make([]int, len(got))
	for i, c := range got {
		pids[i] = c.PID
		if want := procs[slices.IndexFunc(procs, func(p zmx.Process) bool { return p.PID == c.PID })].Args.Env; !slices.Equal(c.Env, want) {
			t.Errorf("client %d's environment = %q, want %q", c.PID, c.Env, want)
		}
	}
	if want := []int{10, 11, 12, 13, 14, 15, 16, 17}; !slices.Equal(pids, want) {
		t.Errorf("AttachClients(dev) = %v, want %v", pids, want)
	}
}

func TestAttachClients_RefusesAProcessItCannotRead(t *testing.T) {
	const dir = "/tmp/zx-clients"
	unreadable := client(10, nil)
	unreadable.Err = errors.New("read arguments of process 10: invalid argument")
	detachedUnreadable := client(11, nil)
	detachedUnreadable.Terminal = false
	detachedUnreadable.Err = unreadable.Err
	otherUnreadable := otherProgram(14, nil)
	otherUnreadable.Err = unreadable.Err
	attachHidingItsEnvironment := client(12, []string{"zmx", "attach", "dev"})
	shellHidingItsEnvironment := client(13, []string{"zmx", "kill", "dev"})

	for _, tt := range []struct {
		name  string
		procs []zmx.Process
		want  error
	}{
		{"a zmx on a terminal it can't read", []zmx.Process{unreadable}, backend.ErrUnreadableClient},
		{"a zmx attach whose environment the kernel doesn't show", []zmx.Process{attachHidingItsEnvironment}, backend.ErrUnreadableClient},
		{"not a zmx with no terminal", []zmx.Process{detachedUnreadable}, nil},
		{"not another program", []zmx.Process{otherUnreadable}, nil},
		{"not another zmx command whose environment the kernel doesn't show", []zmx.Process{shellHidingItsEnvironment}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newBackend(t, dir)
			zmx.FakeProcesses(b, tt.procs, nil)

			if got, err := b.AttachClients("dev"); !errors.Is(err, tt.want) {
				t.Errorf("AttachClients = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestAttachClients_FailsWithTheProcessListing(t *testing.T) {
	b := newBackend(t, "/tmp/zx-clients")
	listing := errors.New("sysctl failed")
	zmx.FakeProcesses(b, nil, listing)

	if got, err := b.AttachClients("dev"); !errors.Is(err, listing) || got != nil {
		t.Errorf("AttachClients = %v, %v; want the listing's failure", got, err)
	}
}
