package cmd

import (
	"context"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
)

const (
	workSession = "api-x7Kd9a"
	workDir     = "/srv/api/.claude/worktrees/feature"
)

// stageWorkingAttach stages an attach of workSession, which records
// /srv/api while its work runs in workDir.
func stageWorkingAttach(t *testing.T) (*backendtest.Fake, *attachRecorder) {
	t.Helper()
	fake, rec := stageAttach(t, workSession)
	fake.Add(backend.Session{Name: workSession, Dir: "/srv/api"})
	fake.SetWorkDir(workSession, workDir)
	return fake, rec
}

func TestAttach_ReportsTheWorkDir(t *testing.T) {
	t.Run("the session is told the directory its work runs in before the attach runs", func(t *testing.T) {
		fake, rec := stageWorkingAttach(t)
		var atRun []string
		rec.answer(runResult{ended: func() { atRun = fake.Reported(workSession) }})

		if err := openSession(contextCommand(), workSession); err != nil {
			t.Fatalf("openSession = %v", err)
		}

		rec.onlyRun(t)
		if want := []string{workDir}; !slices.Equal(atRun, want) {
			t.Errorf("reported %q as the attach ran, want %q", atRun, want)
		}
	})

	t.Run("an attach with no terminal is told it before the exec takes the attach's place", func(t *testing.T) {
		fake, rec := stageWorkingAttach(t)
		rec.answer(runResult{err: runner.ErrNoTerminal})
		var atExec []string
		rec.onExec = func() { atExec = fake.Reported(workSession) }

		if err := openSession(contextCommand(), workSession); err != nil {
			t.Fatalf("openSession = %v", err)
		}

		if len(rec.execs) != 1 {
			t.Fatalf("execs = %+v, want the one exec", rec.execs)
		}
		if want := []string{workDir}; !slices.Equal(atExec, want) {
			t.Errorf("reported %q as the exec ran, want %q", atExec, want)
		}
	})

	t.Run("a switch from inside a session tells the session nothing: this process attaches nothing", func(t *testing.T) {
		fake, rec := stageWorkingAttach(t)
		fake.Add(backend.Session{Name: "blog-c3d4"})
		fake.SetCurrent("blog-c3d4")

		if err := openSession(contextCommand(), workSession); err != nil {
			t.Fatalf("openSession = %v", err)
		}

		if len(rec.runs) != 0 {
			t.Fatalf("runs = %+v, want the switch alone", rec.runs)
		}
		if calls := append(fake.CallsTo(backendtest.OpWorkDir), fake.CallsTo(backendtest.OpReportDir)...); len(calls) != 0 {
			t.Errorf("calls = %+v, want no directory found or told", calls)
		}
	})

	t.Run("a failure to find it is logged at DEBUG and the attach goes ahead", func(t *testing.T) {
		sink := logtest.Install(t)
		fake, rec := stageWorkingAttach(t)
		fake.Fail(backendtest.OpWorkDir, backend.ErrTimeout)

		if err := openSession(contextCommand(), workSession); err != nil {
			t.Fatalf("openSession = %v", err)
		}

		rec.onlyRun(t)
		r := sink.Records().Matching("attach", "dir report failed").AtExactLevel(slog.LevelDebug).Only(t, "the find's failure")
		if detail := r.AttrString(t, "detail"); detail != "read" {
			t.Errorf("detail = %q, want read", detail)
		}
		if reported := fake.Reported(workSession); len(reported) != 0 {
			t.Errorf("reported %q, want nothing told", reported)
		}
	})

	t.Run("a failure to tell it is logged at DEBUG and the attach goes ahead", func(t *testing.T) {
		sink := logtest.Install(t)
		fake, rec := stageWorkingAttach(t)
		fake.Fail(backendtest.OpReportDir, backend.ErrTimeout)

		if err := openSession(contextCommand(), workSession); err != nil {
			t.Fatalf("openSession = %v", err)
		}

		rec.onlyRun(t)
		r := sink.Records().Matching("attach", "dir report failed").AtExactLevel(slog.LevelDebug).Only(t, "the report's failure")
		if detail := r.AttrString(t, "detail"); detail != "report" {
			t.Errorf("detail = %q, want report", detail)
		}
	})
}

func TestReportWorkDir(t *testing.T) {
	t.Run("it returns the directory found, told or not", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: workSession, Dir: "/srv/api"})
		fake.SetWorkDir(workSession, workDir)
		fake.Fail(backendtest.OpReportDir, backend.ErrTimeout)

		if got := reportWorkDir(t.Context(), fake, workSession); got != workDir {
			t.Errorf("reportWorkDir = %q, want %q", got, workDir)
		}
	})

	t.Run("a session with no directory to find is told nothing", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: workSession})

		if got := reportWorkDir(t.Context(), fake, workSession); got != "" {
			t.Errorf("reportWorkDir = %q, want none", got)
		}
		if calls := fake.CallsTo(backendtest.OpReportDir); len(calls) != 0 {
			t.Errorf("ReportDir calls = %+v, want none", calls)
		}
	})

	t.Run("the find and the report run within reportDirWait", func(t *testing.T) {
		b := &deadlines{Fake: backendtest.New(backend.Session{Name: workSession})}
		b.SetWorkDir(workSession, workDir)

		reportWorkDir(t.Context(), b, workSession)
		returned := time.Now()

		if len(b.seen) != 2 {
			t.Fatalf("deadlines seen = %v, want one for the find and one for the report", b.seen)
		}
		for i, deadline := range b.seen {
			if deadline.IsZero() || deadline.After(returned.Add(reportDirWait)) {
				t.Errorf("call %d ran with deadline %v, want one no later than %v past its return at %v", i, deadline, reportDirWait, returned)
			}
		}
	})
}

// deadlines is a Fake that records the deadline each WorkDir and ReportDir
// runs under, zero for none.
type deadlines struct {
	*backendtest.Fake
	seen []time.Time
}

func (d *deadlines) WorkDir(ctx context.Context, name string) (string, error) {
	d.record(ctx)
	return d.Fake.WorkDir(ctx, name)
}

func (d *deadlines) ReportDir(ctx context.Context, name, dir string) error {
	d.record(ctx)
	return d.Fake.ReportDir(ctx, name, dir)
}

func (d *deadlines) record(ctx context.Context) {
	deadline, _ := ctx.Deadline()
	d.seen = append(d.seen, deadline)
}
