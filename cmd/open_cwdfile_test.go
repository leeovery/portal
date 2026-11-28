package cmd

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
)

// unwritten is what a cwd file holds until Portal writes it, so a test can
// tell a file left alone from one written.
const unwritten = "unwritten"

// leaving is an open line's attaches of api-x7Kd9a and blog-c3d4, each with
// its work in a directory of its own, and the file it hands --cwd-file.
type leaving struct {
	fake            *backendtest.Fake
	rec             *attachRecorder
	reads           *workDirReads
	apiDir, blogDir string
	cwdFile         string
	stats           int
	home            string
}

// workDirReads is the staged backend, recording the session and the deadline
// of each WorkDir, and finding none once blank is set.
type workDirReads struct {
	envBackend
	names     []string
	deadlines []time.Time
	blank     bool
}

func (r *workDirReads) WorkDir(ctx context.Context, name string) (string, error) {
	deadline, _ := ctx.Deadline()
	r.names = append(r.names, name)
	r.deadlines = append(r.deadlines, deadline)
	if r.blank {
		return "", nil
	}
	return r.Fake.WorkDir(ctx, name)
}

func stageLeaving(t *testing.T) *leaving {
	t.Helper()
	fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
	reads := &workDirReads{envBackend: envBackend{Fake: fake, env: attachEnv}}
	withBackendDeps(t, BackendDeps{Backend: reads})
	l := &leaving{
		fake:    fake,
		rec:     rec,
		reads:   reads,
		apiDir:  t.TempDir(),
		blogDir: t.TempDir(),
		cwdFile: filepath.Join(t.TempDir(), "cwd"),
		home:    t.TempDir(),
	}
	fake.SetWorkDir("api-x7Kd9a", l.apiDir)
	fake.SetWorkDir("blog-c3d4", l.blogDir)
	t.Setenv("HOME", l.home)
	if err := os.WriteFile(l.cwdFile, []byte(unwritten), 0o600); err != nil {
		t.Fatal(err)
	}
	withFuncSeam(t, &cwdFileStat, func(path string) (os.FileInfo, error) {
		l.stats++
		return os.Stat(path)
	})
	return l
}

// open runs an open line handing --cwd-file the staged file.
func (l *leaving) open(args ...string) error {
	return runOpen(append([]string{"--cwd-file", l.cwdFile}, args...)...)
}

func (l *leaving) written(t *testing.T) string {
	t.Helper()
	got, err := os.ReadFile(l.cwdFile)
	if err != nil {
		t.Fatalf("reading the cwd file: %v", err)
	}
	return string(got)
}

func TestOpen_CwdFile_NamesTheDirectoryOfTheLastSessionLeft(t *testing.T) {
	t.Run("across a detach to the picker and the attach of another", func(t *testing.T) {
		l := stageLeaving(t)
		stagePicker(t, "blog-c3d4")
		l.rec.answer(outcome(attachkeys.Detach))

		if err := l.open("api-x7Kd9a"); err != nil {
			t.Fatalf("open = %v", err)
		}

		if got := l.written(t); got != l.blogDir {
			t.Errorf("cwd file = %q, want blog-c3d4's %q", got, l.blogDir)
		}
	})

	t.Run("read as Portal exits, not as the attach began", func(t *testing.T) {
		l := stageLeaving(t)
		moved := t.TempDir()
		l.rec.answer(runResult{ended: func() { l.fake.SetWorkDir("api-x7Kd9a", moved) }})

		if err := l.open("api-x7Kd9a"); err != nil {
			t.Fatalf("open = %v", err)
		}

		if got := l.written(t); got != moved {
			t.Errorf("cwd file = %q, want %q, where the work ran as Portal exited", got, moved)
		}
	})

	t.Run("an attach that began, then failed, is left", func(t *testing.T) {
		l := stageLeaving(t)
		boom := errors.New("pty: boom after the attach began")
		l.rec.answer(runResult{err: boom, began: true})

		if err := l.open("api-x7Kd9a"); !errors.Is(err, boom) {
			t.Fatalf("open = %v, want %v", err, boom)
		}

		if got := l.written(t); got != l.apiDir {
			t.Errorf("cwd file = %q, want %q", got, l.apiDir)
		}
	})

	t.Run("home, when the directory has gone", func(t *testing.T) {
		l := stageLeaving(t)
		l.fake.SetWorkDir("api-x7Kd9a", filepath.Join(l.apiDir, "removed"))

		if err := l.open("api-x7Kd9a"); err != nil {
			t.Fatalf("open = %v", err)
		}

		if got := l.written(t); got != l.home {
			t.Errorf("cwd file = %q, want home %q", got, l.home)
		}
	})

	t.Run("home, when the path is no longer a directory", func(t *testing.T) {
		l := stageLeaving(t)
		file := filepath.Join(l.apiDir, "now-a-file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		l.fake.SetWorkDir("api-x7Kd9a", file)

		if err := l.open("api-x7Kd9a"); err != nil {
			t.Fatalf("open = %v", err)
		}

		if got := l.written(t); got != l.home {
			t.Errorf("cwd file = %q, want home %q", got, l.home)
		}
	})

	t.Run("created owner-only when it is missing", func(t *testing.T) {
		l := stageLeaving(t)
		if err := os.Remove(l.cwdFile); err != nil {
			t.Fatal(err)
		}

		if err := l.open("api-x7Kd9a"); err != nil {
			t.Fatalf("open = %v", err)
		}

		info, err := os.Stat(l.cwdFile)
		if err != nil {
			t.Fatalf("cwd file: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("cwd file mode = %v, want 0600", perm)
		}
		if got := l.written(t); got != l.apiDir {
			t.Errorf("cwd file = %q, want %q", got, l.apiDir)
		}
	})
}

func TestOpen_CwdFile_TheExitReadFallsBackToTheDirectoryFoundAsTheAttachBegan(t *testing.T) {
	tests := []struct {
		name string
		end  func(*leaving)
	}{
		{name: "a read that fails, as for a session that has ended", end: func(l *leaving) { l.fake.Remove("api-x7Kd9a") }},
		{name: "a read that finds none", end: func(l *leaving) { l.reads.blank = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := stageLeaving(t)
			l.rec.answer(runResult{ended: func() { tt.end(l) }})

			if err := l.open("api-x7Kd9a"); err != nil {
				t.Fatalf("open = %v", err)
			}

			if got := l.written(t); got != l.apiDir {
				t.Errorf("cwd file = %q, want %q, found as the attach began", got, l.apiDir)
			}
		})
	}
}

func TestOpen_CwdFile_TheDirectoryIsReadOnceAsPortalExitsForTheLastSessionAlone(t *testing.T) {
	l := stageLeaving(t)
	stagePicker(t, "blog-c3d4")
	l.rec.answer(outcome(attachkeys.Detach))

	if err := l.open("api-x7Kd9a"); err != nil {
		t.Fatalf("open = %v", err)
	}
	exited := time.Now()

	// Each attach's own find as it began, then the one read as Portal exits.
	if want := []string{"api-x7Kd9a", "blog-c3d4", "blog-c3d4"}; !slices.Equal(l.reads.names, want) {
		t.Fatalf("WorkDir reads = %q, want %q", l.reads.names, want)
	}
	if deadline := l.reads.deadlines[2]; deadline.IsZero() || deadline.After(exited.Add(reportDirWait)) {
		t.Errorf("the exit read ran with deadline %v, want one within %v of the exit at %v", deadline, reportDirWait, exited)
	}
}

func TestOpen_WithoutACwdFile_NoDirectoryIsReadAsAnAttachEnds(t *testing.T) {
	l := stageLeaving(t)
	stagePicker(t, "blog-c3d4")
	l.rec.answer(outcome(attachkeys.Detach))

	if err := runOpen("api-x7Kd9a"); err != nil {
		t.Fatalf("open = %v", err)
	}

	if want := []string{"api-x7Kd9a", "blog-c3d4"}; !slices.Equal(l.reads.names, want) {
		t.Errorf("WorkDir reads = %q, want %q, each attach's find alone", l.reads.names, want)
	}
}

func TestOpen_CwdFile_IsWrittenOnceWhateverTheExit(t *testing.T) {
	t.Run("an attach ending with a non-zero code keeps its code", func(t *testing.T) {
		l := stageLeaving(t)
		l.rec.answer(runResult{out: runner.Outcome{ExitCode: 3}})

		err := l.open("api-x7Kd9a")

		if exit, ok := errors.AsType[*ExitCodeError](err); !ok || exit.Code != 3 {
			t.Errorf("open = %v, want exit status 3", err)
		}
		if got := l.written(t); got != l.apiDir {
			t.Errorf("cwd file = %q, want %q", got, l.apiDir)
		}
		if l.stats != 1 {
			t.Errorf("directory checked %d times, want once: one write", l.stats)
		}
	})

	t.Run("an error after an attach is returned as it was", func(t *testing.T) {
		l := stageLeaving(t)
		boom := errors.New("picker: boom")
		stagePicker(t).err = boom
		l.rec.answer(outcome(attachkeys.Detach))

		err := l.open("api-x7Kd9a")

		if !errors.Is(err, boom) {
			t.Errorf("open = %v, want %v", err, boom)
		}
		if got := l.written(t); got != l.apiDir {
			t.Errorf("cwd file = %q, want %q", got, l.apiDir)
		}
		if l.stats != 1 {
			t.Errorf("directory checked %d times, want once: one write", l.stats)
		}
	})

	t.Run("an exec after an attach writes it first, since the exec runs no defers", func(t *testing.T) {
		l := stageLeaving(t)
		stagePicker(t, "blog-c3d4")
		l.rec.answer(outcome(attachkeys.Detach), runResult{err: runner.ErrNoTerminal})
		var atExec string
		l.rec.onExec = func() { atExec = l.written(t) }

		if err := l.open("api-x7Kd9a"); err != nil {
			t.Fatalf("open = %v", err)
		}

		if len(l.rec.execs) != 1 {
			t.Fatalf("execs = %+v, want the one exec", l.rec.execs)
		}
		if atExec != l.apiDir {
			t.Errorf("cwd file as the exec ran = %q, want api-x7Kd9a's %q", atExec, l.apiDir)
		}
		if l.stats != 1 {
			t.Errorf("directory checked %d times, want once: one write", l.stats)
		}
	})
}

func TestOpen_CwdFile_IsLeftAloneWhenNoSessionWasLeft(t *testing.T) {
	boom := errors.New("pty: boom")
	tests := []struct {
		name  string
		args  []string
		stage func(*leaving)
		// reached fails the case unless it got as far as it says.
		reached func(t *testing.T, l *leaving, err error)
	}{
		{
			name: "help",
			args: []string{"--help"},
			reached: func(t *testing.T, l *leaving, err error) {
				if err != nil || len(l.rec.runs) != 0 {
					t.Errorf("open --help = %v with runs %+v, want help alone", err, l.rec.runs)
				}
			},
		},
		{
			name: "an error before any attach",
			args: []string{"--ack", "notcolon", "api-x7Kd9a"},
			reached: func(t *testing.T, _ *leaving, err error) {
				if _, ok := errors.AsType[*UsageError](err); !ok {
					t.Errorf("open = %v, want the usage error", err)
				}
			},
		},
		{
			name: "an attach that never began",
			args: []string{"api-x7Kd9a"},
			stage: func(l *leaving) {
				l.rec.answer(runResult{err: boom})
			},
			reached: func(t *testing.T, l *leaving, err error) {
				if !errors.Is(err, boom) || len(l.rec.runs) != 1 {
					t.Errorf("open = %v with runs %+v, want the one run's %v", err, l.rec.runs, boom)
				}
			},
		},
		{
			name: "a switch from inside a session",
			args: []string{"api-x7Kd9a"},
			stage: func(l *leaving) {
				l.fake.SetCurrent("blog-c3d4")
				l.rec.runners.switchErr = nil
			},
			reached: func(t *testing.T, l *leaving, err error) {
				if err != nil || len(l.rec.runners.switches) != 1 {
					t.Errorf("open = %v with switches %+v, want the one switch", err, l.rec.runners.switches)
				}
			},
		},
		{
			name: "the exec that takes the attach's place",
			args: []string{"api-x7Kd9a"},
			stage: func(l *leaving) {
				l.rec.answer(runResult{err: runner.ErrNoTerminal})
			},
			reached: func(t *testing.T, l *leaving, err error) {
				if err != nil || len(l.rec.execs) != 1 {
					t.Errorf("open = %v with execs %+v, want the one exec", err, l.rec.execs)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := stageLeaving(t)
			if tt.stage != nil {
				tt.stage(l)
			}

			err := l.open(tt.args...)

			tt.reached(t, l, err)
			if got := l.written(t); got != unwritten {
				t.Errorf("cwd file = %q, want it left alone", got)
			}
		})
	}
}

func TestOpen_CwdFile_IsLeftAloneWhenThePickerQuitsAtOnce(t *testing.T) {
	l := stageLeaving(t)
	stageRealPickers(t, l.fake, []pickerStep{})

	if err := l.open(); err != nil {
		t.Fatalf("open = %v", err)
	}

	if len(l.rec.runs) != 0 {
		t.Fatalf("runs = %+v, want none", l.rec.runs)
	}
	if got := l.written(t); got != unwritten {
		t.Errorf("cwd file = %q, want it left alone", got)
	}
}

func TestOpen_CwdFile_AWriteThatFailsIsLoggedAndChangesNothingElse(t *testing.T) {
	sink := logtest.Install(t)
	l := stageLeaving(t)
	l.cwdFile = filepath.Join(t.TempDir(), "missing", "cwd")
	l.rec.answer(runResult{out: runner.Outcome{ExitCode: 3}})

	err := l.open("api-x7Kd9a")

	if exit, ok := errors.AsType[*ExitCodeError](err); !ok || exit.Code != 3 {
		t.Errorf("open = %v, want exit status 3", err)
	}
	r := sink.Records().Matching("attach", "cwd file write failed").AtExactLevel(slog.LevelWarn).Only(t, "the failed write")
	if path := r.AttrString(t, "path"); path != l.cwdFile {
		t.Errorf("path = %q, want %q", path, l.cwdFile)
	}
}

func TestLeftDir_WritesNothingWhenTheCheckCannotAnswerInTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		withFuncSeam(t, &cwdFileStat, func(string) (os.FileInfo, error) {
			<-release
			return nil, os.ErrNotExist
		})
		path := filepath.Join(t.TempDir(), "cwd")
		fake := backendtest.New(backend.Session{Name: workSession})
		fake.SetWorkDir(workSession, "/Volumes/hung/work")
		left := &leftDir{path: path}
		left.leave(fake, workSession, "")
		start := time.Now()

		left.write(t.Context())

		if took := time.Since(start); took != cwdStatWait {
			t.Errorf("write took %v, want %v, the check's bound", took, cwdStatWait)
		}
		close(release)
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("cwd file stat = %v, want nothing written", err)
		}
	})
}

func TestOpen_CwdFile_IsLeftAloneWhenTheCheckFailsOtherwise(t *testing.T) {
	t.Run("a symlink loop", func(t *testing.T) {
		l := stageLeaving(t)
		loop := filepath.Join(t.TempDir(), "loop")
		if err := os.Symlink(loop, loop); err != nil {
			t.Fatal(err)
		}
		l.fake.SetWorkDir("api-x7Kd9a", loop)

		if err := l.open("api-x7Kd9a"); err != nil {
			t.Fatalf("open = %v", err)
		}

		if got := l.written(t); got != unwritten {
			t.Errorf("cwd file = %q, want it left alone", got)
		}
	})

	t.Run("permission denied", func(t *testing.T) {
		l := stageLeaving(t)
		withFuncSeam(t, &cwdFileStat, func(path string) (os.FileInfo, error) {
			return nil, &fs.PathError{Op: "stat", Path: path, Err: syscall.EACCES}
		})

		if err := l.open("api-x7Kd9a"); err != nil {
			t.Fatalf("open = %v", err)
		}

		if got := l.written(t); got != unwritten {
			t.Errorf("cwd file = %q, want it left alone", got)
		}
	})
}
