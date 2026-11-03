package zmx_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/proctable"
)

// client is a process on a terminal exec'd with argv and env.
func client(pid int, argv []string, env ...string) zmx.Process {
	return zmx.Process{
		Process: proctable.Process{PID: pid, PPID: 1, Terminal: true},
		Args:    proctable.Args{Argv: argv, Env: env},
	}
}

func TestAttachClients(t *testing.T) {
	const dir = "/tmp/zx-clients"
	inDir := "ZMX_DIR=" + dir
	daemon := client(9, []string{"/opt/bin/zmx", "attach", "dev"}, inDir)
	daemon.Terminal = false
	unreadable := client(8, []string{"zmx", "attach", "dev"})
	unreadable.Err = errors.New("exited")

	procs := []zmx.Process{
		client(10, []string{"/opt/bin/zmx", "attach", "dev"}, inDir),
		client(11, []string{"zmx", "a", "dev"}, inDir),
		client(12, []string{"zmx", "attach", "--labels", "k=v", "dev", "vim"}, inDir),
		client(13, []string{"zmx", "attach", "dev"}),
		client(14, []string{"zmx", "attach", "v"}, inDir, "ZMX_SESSION_PREFIX=de"),
		daemon,
		unreadable,
		client(20, []string{"zmx", "attach", "dev"}, "ZMX_DIR=/tmp/zx-other"),
		client(21, []string{"zmx", "attach", "dev"}, "HOME=/h", "TMPDIR=/var/t"),
		client(22, []string{"zmx", "attach", "dev"}, inDir, "ZMX_SESSION_PREFIX=p-"),
		client(23, []string{"zmx", "attach", "devx"}, inDir),
		client(24, []string{"zmx", "kill", "dev"}, inDir),
		client(25, []string{"zmxd", "attach", "dev"}, inDir),
		client(26, []string{"zmx", "attach", "--labels"}, inDir),
		client(27, []string{"zmx", "attach"}, inDir),
	}

	b := newBackend(t, dir)
	zmx.FakeProcesses(b, procs, nil)

	got, err := b.AttachClients("dev")

	// 10–12 name dev in the directory, 13 shows no environment to judge, and
	// 14 reaches dev through its prefix. The daemon has no terminal; 20–27
	// attach elsewhere or don't attach.
	if want := []int{10, 11, 12, 13, 14}; err != nil || !slices.Equal(got, want) {
		t.Errorf("AttachClients(dev) = %v, %v; want %v", got, err, want)
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
