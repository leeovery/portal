package zmx_test

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/proctable"
)

// recordedDir is the directory the sessions of these tests record.
const recordedDir = "/srv/recorded"

// leading is the session's program in the fake table: pid, in its own
// group, on the session's terminal, whose foreground group is foreground,
// running command in cwd.
func leading(pid, foreground int, command, cwd string) zmx.Process {
	return zmx.Process{
		Process: proctable.Process{PID: pid, PPID: 1, Group: pid, Terminal: true, ForegroundGroup: foreground, Command: command},
		Cwd:     cwd,
	}
}

// child is a process under parent on the session's terminal, in group,
// running command in cwd, "" for one whose directory can't be read.
func child(pid, parent, group int, command, cwd string) zmx.Process {
	return zmx.Process{
		Process: proctable.Process{PID: pid, PPID: parent, Group: group, Terminal: true, Command: command},
		Cwd:     cwd,
	}
}

// exiting is p gone: a zombie, or a process working on exiting.
func exiting(p zmx.Process) zmx.Process {
	p.Gone = true
	return p
}

func TestWorkDir(t *testing.T) {
	info := withCwd(infoFrame(t), recordedDir)
	pid := fixtureListing(info, "work").PID
	sh, shC, claude, vim := pid+1, pid+2, pid+3, pid+4
	for _, tt := range []struct {
		name  string
		procs []zmx.Process
		want  string
	}{
		{
			name:  "a login shell at its prompt works in its own directory",
			procs: []zmx.Process{leading(pid, pid, "zsh", "/srv/shell")},
			want:  "/srv/shell",
		},
		{
			name: "Claude under Portal's wrappers is followed to, through each sh, and no further",
			procs: []zmx.Process{
				leading(pid, pid, "sh", "/srv/api"),
				child(shC, pid, pid, "sh", "/srv/api"),
				child(claude, shC, pid, "claude", "/srv/api/.claude/worktrees/feature"),
				child(claude+10, claude, pid, "bun", "/srv/mcp"),
			},
			want: "/srv/api/.claude/worktrees/feature",
		},
		{
			name: "Claude running a lone MCP server, or its Bash tool's command, is no shell, so not passed",
			procs: []zmx.Process{
				leading(pid, pid, "sh", "/srv/api"),
				child(claude, pid, pid, "claude", "/srv/api/worktree"),
				child(claude+10, claude, pid, "zsh", "/tmp"),
			},
			want: "/srv/api/worktree",
		},
		{
			name: "the resume panel's waiter under its sh is where the session works",
			procs: []zmx.Process{
				leading(pid, pid, "sh", "/srv/api"),
				child(sh, pid, pid, "portal", "/srv/api/project"),
			},
			want: "/srv/api/project",
		},
		{
			name: "a shell's foreground job, in a group of its own, is followed",
			procs: []zmx.Process{
				leading(pid, vim, "zsh", "/srv/shell"),
				child(vim, pid, vim, "vim", "/srv/edited"),
			},
			want: "/srv/edited",
		},
		{
			name: "a shell's suspended job is not followed, the shell having moved on",
			procs: []zmx.Process{
				leading(pid, pid, "zsh", "/srv/moved"),
				child(vim, pid, vim, "vim", "/srv/edited"),
			},
			want: "/srv/moved",
		},
		{
			name: "a background job and a helper, each in a group of its own, are not followed",
			procs: []zmx.Process{
				leading(pid, pid, "zsh", "/srv/shell"),
				child(vim, pid, vim, "sleep", "/srv/job"),
				child(vim+1, pid, vim+1, "gitstatusd", "/"),
			},
			want: "/srv/shell",
		},
		{
			name: "a shell nested in another's job, at its prompt, is followed",
			procs: []zmx.Process{
				leading(pid, sh, "zsh", "/srv/outer"),
				child(sh, pid, sh, "bash", "/srv/inner"),
			},
			want: "/srv/inner",
		},
		{
			name: "a shell nested in another's job is followed to its own foreground job",
			procs: []zmx.Process{
				leading(pid, vim, "zsh", "/srv/outer"),
				child(sh, pid, sh, "bash", "/srv/inner"),
				child(vim, sh, vim, "vim", "/srv/edited"),
			},
			want: "/srv/edited",
		},
		{
			name: "Claude a minted session's interactive shell runs, its job its own, is followed to through that shell",
			procs: []zmx.Process{
				leading(pid, claude, "sh", "/srv/api"),
				child(shC, pid, shC, "zsh", "/srv/api"),
				child(claude, shC, claude, "claude", "/srv/api/.claude/worktrees/feature"),
			},
			want: "/srv/api/.claude/worktrees/feature",
		},
		{
			name: "of a shell's children, only the one on the way to the foreground counts, a background job beside it",
			procs: []zmx.Process{
				leading(pid, vim, "zsh", "/srv/outer"),
				child(vim+1, pid, vim+1, "sleep", "/srv/job"),
				child(sh, pid, sh, "bash", "/srv/inner"),
				child(vim, sh, vim, "vim", "/srv/edited"),
			},
			want: "/srv/edited",
		},
		{
			name: "a nested shell whose job is suspended is where the walk stops",
			procs: []zmx.Process{
				leading(pid, sh, "zsh", "/srv/outer"),
				child(sh, pid, sh, "bash", "/srv/inner"),
				child(vim, sh, vim, "vim", "/srv/edited"),
			},
			want: "/srv/inner",
		},
		{
			name: "a foreground pipeline stops the walk at its shell",
			procs: []zmx.Process{
				leading(pid, vim, "zsh", "/srv/shell"),
				child(vim, pid, vim, "grep", "/srv/a"),
				child(vim+1, pid, vim, "less", "/srv/a"),
			},
			want: "/srv/shell",
		},
		{
			name: "a lone foreground child that is a zombie or exiting is not followed",
			procs: []zmx.Process{
				leading(pid, vim, "zsh", "/srv/shell"),
				exiting(child(vim, pid, vim, "vim", "/srv/edited")),
			},
			want: "/srv/shell",
		},
		{
			name: "a program on no terminal is not walked",
			procs: []zmx.Process{
				{Process: proctable.Process{PID: pid, PPID: 1, Group: pid, Command: "sh"}, Cwd: "/srv/detached"},
				child(sh, pid, 0, "sleep", "/srv/child"),
			},
			want: "/srv/detached",
		},
		{
			name: "where the process it stops at can't be read, the deepest on the way that can is the directory",
			procs: []zmx.Process{
				leading(pid, pid, "sh", "/srv/wrapper"),
				child(shC, pid, pid, "sh", "/srv/worktree"),
				child(claude, shC, pid, "claude", ""),
			},
			want: "/srv/worktree",
		},
		{
			name: "where no process on the way can be read, the directory recorded",
			procs: []zmx.Process{
				leading(pid, pid, "sh", ""),
				child(claude, pid, pid, "claude", ""),
			},
			want: recordedDir,
		},
		{
			name:  "a program the table doesn't list, gone, falls back to the directory recorded",
			procs: []zmx.Process{child(sh, 1, sh, "sh", "/srv/other")},
			want:  recordedDir,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := socketDir(t)
			serve(t, dir, "work", anyRequest(info, []byte("screen")))
			b := newBackend(t, dir)
			zmx.FakeProcesses(b, tt.procs, nil)

			if got, err := b.WorkDir(t.Context(), "work"); err != nil || got != tt.want {
				t.Errorf("WorkDir = %q, %v; want %q", got, err, tt.want)
			}
		})
	}

	t.Run("a recorded spelling equal to the one found is taken with nothing read from disk", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", anyRequest(withCwd(infoFrame(t), "/srv/same"), []byte("screen")))
		b := newBackend(t, dir)
		zmx.FakeProcesses(b, []zmx.Process{leading(pid, pid, "zsh", "/srv/same")}, nil)
		zmx.FakeStat(b, func(path string) (fs.FileInfo, error) {
			t.Errorf("stat(%q) read from disk", path)
			return nil, fs.ErrNotExist
		})

		if got, err := b.WorkDir(t.Context(), "work"); err != nil || got != "/srv/same" {
			t.Errorf("WorkDir = %q, %v; want /srv/same", got, err)
		}
	})

	t.Run("a comparison that never answers, as on a hung mount, gives up within the ceiling, keeping the path found", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", anyRequest(withCwd(infoFrame(t), "/net/hung"), []byte("screen")))
		b := newBackend(t, dir)
		zmx.FakeProcesses(b, []zmx.Process{leading(pid, pid, "zsh", "/srv/found")}, nil)
		hung := make(chan struct{})
		t.Cleanup(func() { close(hung) })
		zmx.FakeStat(b, func(string) (fs.FileInfo, error) {
			<-hung
			return nil, fs.ErrNotExist
		})

		start := time.Now()
		got, err := b.WorkDir(t.Context(), "work")
		if took := time.Since(start); err != nil || got != "/srv/found" || took > 5*ceiling {
			t.Errorf("WorkDir = %q, %v after %v; want /srv/found within the %v ceiling", got, err, took, ceiling)
		}
	})

	t.Run("a table that can't be read falls back to the directory recorded", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", anyRequest(info, []byte("screen")))
		b := newBackend(t, dir)
		zmx.FakeProcesses(b, nil, errors.New("sysctl refused"))

		if got, err := b.WorkDir(t.Context(), "work"); err != nil || got != recordedDir {
			t.Errorf("WorkDir = %q, %v; want %q", got, err, recordedDir)
		}
	})

	t.Run("a session naming no process is the directory it records, the table unread", func(t *testing.T) {
		noProgram := withPID(info, 0)
		dir := socketDir(t)
		serve(t, dir, "work", anyRequest(noProgram, []byte("screen")))
		b := newBackend(t, dir)
		var lists atomic.Int32
		zmx.FakeProcessesFrom(b, func() ([]zmx.Process, error) {
			lists.Add(1)
			return []zmx.Process{leading(1, 1, "launchd", "/srv/launchd")}, nil
		})

		if got, err := b.WorkDir(t.Context(), "work"); err != nil || got != recordedDir {
			t.Errorf("WorkDir = %q, %v; want %q", got, err, recordedDir)
		}
		if n := lists.Load(); n != 0 {
			t.Errorf("the table was read %d times, want none", n)
		}
	})
}

func TestReportDir(t *testing.T) {
	const want = "\x1b]7;file://" + hostname + "/srv/a%20dir%C2%B7x/100%25%3B%23%3F~-_.:@\x1b\\"

	t.Run("it sends the OSC 7 zmx's toOsc7Cwd writes, as one Output frame on a connection after the read", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "work", anyRequest(infoFrame(t), []byte("screen")))

		if err := newBackend(t, dir).ReportDir(t.Context(), "work", "/srv/a dir·x/100%;#?~-_.:@"); err != nil {
			t.Fatalf("ReportDir = %v", err)
		}

		reqs := d.received()
		if len(reqs) != 2 || reqs[0].tag != 6 || reqs[1].tag != 1 || string(reqs[1].payload) != want {
			t.Errorf("ReportDir sent %+v; want Info, then one Output of %q", reqs, want)
		}
	})

	t.Run("it waits for its own Output past the session's", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", broadcastingFirst(infoFrame(t), []byte("prompt$ ")))

		if err := newBackend(t, dir).ReportDir(t.Context(), "work", "/srv/app"); err != nil {
			t.Errorf("ReportDir = %v, want the echo found past the session's output", err)
		}
	})

	t.Run("a daemon that broadcasts the session's Output but never the report's is not answering", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", neverEchoing(infoFrame(t), []byte("prompt$ ")))

		if err := newBackend(t, dir).ReportDir(t.Context(), "work", "/srv/app"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("ReportDir = %v, want ErrTimeout: its own Output never came back", err)
		}
	})

	// Info's cwd is also the directory the session was made in, which its
	// terminal, the state an attach is sent, never took.
	t.Run("it sends the report to a session whose Info already records the directory", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "work", anyRequest(withCwd(infoFrame(t), "file://"+hostname+"/srv/my%20app"), []byte("screen")))

		if err := newBackend(t, dir).ReportDir(t.Context(), "work", "/srv/my app"); err != nil {
			t.Fatalf("ReportDir = %v", err)
		}
		if reqs := d.received(); len(reqs) != 2 || reqs[0].tag != 6 || reqs[1].tag != 1 {
			t.Errorf("ReportDir sent %+v; want Info, then the Output", reqs)
		}
	})

	t.Run("a session answering Info in another protocol is sent no Output", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "work", anyRequest(frame(6, 2, make([]byte, 552)), []byte("screen")))

		if err := newBackend(t, dir).ReportDir(t.Context(), "work", "/srv/app"); err == nil {
			t.Error("ReportDir = nil, want the other protocol refused")
		}
		if reqs := d.received(); slices.ContainsFunc(reqs, func(r request) bool { return r.tag == 1 }) {
			t.Errorf("ReportDir sent %+v; want no Output", reqs)
		}
	})

	t.Run("it refuses a directory that is not absolute, sending nothing", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "work", anyRequest(infoFrame(t), []byte("screen")))

		if err := newBackend(t, dir).ReportDir(t.Context(), "work", "srv/app"); err == nil {
			t.Error("ReportDir(srv/app) = nil, want a refusal")
		}
		if reqs := d.received(); len(reqs) != 0 {
			t.Errorf("ReportDir sent %+v; want nothing", reqs)
		}
	})

	t.Run("a daemon that takes the report and never broadcasts it is not answering", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", answeringOnce(infoFrame(t)))

		if err := newBackend(t, dir).ReportDir(t.Context(), "work", "/srv/app"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("ReportDir = %v, want ErrTimeout", err)
		}
	})
}

// broadcastingFirst answers as anyRequest does, after broadcasting output,
// as the session's own, on every connection.
func broadcastingFirst(info, output []byte) func(*daemon, net.Conn) {
	answer := anyRequest(info, nil)
	return func(d *daemon, c net.Conn) {
		_, _ = c.Write(frame(1, 1, output))
		answer(d, c)
	}
}

// neverEchoing answers Info with info, and Output with output, the session's,
// never broadcasting the Output it was sent.
func neverEchoing(info, output []byte) func(*daemon, net.Conn) {
	return func(d *daemon, c net.Conn) {
		r, err := d.read(c)
		if err != nil {
			return
		}
		switch r.tag {
		case 6:
			_, _ = c.Write(info)
		case 1:
			_, _ = c.Write(frame(1, 1, output))
		}
		_, _ = io.Copy(io.Discard, c)
	}
}

// withPID is an Info frame with its pid replaced.
func withPID(f []byte, pid int32) []byte {
	f = slices.Clone(f)
	binary.NativeEndian.PutUint32(f[8+8:], uint32(pid))
	return f
}

func TestPreferRecordedGivesTheFoundPathOnceItsContextHasEnded(t *testing.T) {
	found := realDir(t, t.TempDir())
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(found, link); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if got := zmx.PreferRecorded(ctx, link, found); got != found {
		t.Errorf("PreferRecorded(ended, %q, %q) = %q, want the path found", link, found, got)
	}
}
