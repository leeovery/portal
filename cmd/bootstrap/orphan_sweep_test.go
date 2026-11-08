package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/saverhost"
	"golang.org/x/sys/unix"
)

// The proofs a sweep's Prove can fail with, as saverhost's wrap them.
var (
	errNotTheSaversDaemon = fmt.Errorf("pid's environment lacks ZMX_SESSION=_portal-saver: %w", saverhost.ErrNotSaverDaemon)
	errUnprovable         = fmt.Errorf("read the environment: %w", saverhost.ErrUnreadable)
	errGoneBeforeItsProof = fmt.Errorf("pid: %w", saverhost.ErrGone)
)

// sweepFakes proves, signals and waits for exits for a sweep, recording each
// in order. A pid with no entry in failed proves; one with no entry in
// exitErrs finishes exiting once sent SIGKILL.
type sweepFakes struct {
	failed     map[int]error
	signalErrs map[int]error
	exitErrs   map[int]error
	saverReads int
	events     []string
}

func (f *sweepFakes) prove(pid int) error {
	f.events = append(f.events, fmt.Sprintf("prove %d", pid))
	return f.failed[pid]
}

func (f *sweepFakes) signal(pid int, sig syscall.Signal) error {
	f.events = append(f.events, fmt.Sprintf("%s %d", unix.SignalName(sig), pid))
	return f.signalErrs[pid]
}

func (f *sweepFakes) awaitExit(_ context.Context, pid int) error {
	f.events = append(f.events, fmt.Sprintf("await %d", pid))
	return f.exitErrs[pid]
}

// killed is every pid sent SIGKILL, in order.
func (f *sweepFakes) killed() []int {
	var pids []int
	for _, e := range f.events {
		var pid int
		if _, err := fmt.Sscanf(e, "SIGKILL %d", &pid); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

// saver answers the sweep's read of the saver session's process.
func (f *sweepFakes) saver(pid int, present bool, err error) func(context.Context) (int, bool, error) {
	return func(context.Context) (int, bool, error) {
		f.saverReads++
		return pid, present, err
	}
}

func (f *sweepFakes) sweep(candidates []int, saverPID func(context.Context) (int, bool, error), logger *slog.Logger) *OrphanSweepCore {
	return &OrphanSweepCore{
		Pgrep:     func() ([]int, error) { return candidates, nil },
		SaverPID:  saverPID,
		Prove:     f.prove,
		Signal:    f.signal,
		AwaitExit: f.awaitExit,
		Logger:    logger,
	}
}

func run(t *testing.T, c *OrphanSweepCore) {
	t.Helper()
	if err := c.SweepOrphanDaemons(t.Context()); err != nil {
		t.Fatalf("SweepOrphanDaemons = %v, want nil: the sweep is best-effort", err)
	}
}

func TestSweepOrphanDaemons(t *testing.T) {
	const saverPID = 1000

	t.Run("it kills each proved orphan, proving each just before its kill and waiting for it to exit after", func(t *testing.T) {
		f := &sweepFakes{}

		run(t, f.sweep([]int{2001, 2002}, f.saver(saverPID, true, nil), nil))

		if want := []string{"prove 2001", "SIGKILL 2001", "await 2001", "prove 2002", "SIGKILL 2002", "await 2002"}; !slices.Equal(f.events, want) {
			t.Errorf("events = %q, want %q", f.events, want)
		}
	})

	t.Run("it never kills the saver session's process, nor proves it", func(t *testing.T) {
		f := &sweepFakes{}

		run(t, f.sweep([]int{saverPID, 2001}, f.saver(saverPID, true, nil), nil))

		if want := []string{"prove 2001", "SIGKILL 2001", "await 2001"}; !slices.Equal(f.events, want) {
			t.Errorf("events = %q, want %q: the saver's process passed over", f.events, want)
		}
	})

	t.Run("it never kills itself, nor proves itself", func(t *testing.T) {
		f := &sweepFakes{}

		run(t, f.sweep([]int{os.Getpid(), 2001}, f.saver(0, false, nil), nil))

		if want := []string{"prove 2001", "SIGKILL 2001", "await 2001"}; !slices.Equal(f.events, want) {
			t.Errorf("events = %q, want %q: this process passed over", f.events, want)
		}
	})

	for name, err := range map[string]error{
		"lacks the saver session's marks, as another install's daemon does": errNotTheSaversDaemon,
		"can't be read":                     errUnprovable,
		"has gone before its proof is read": errGoneBeforeItsProof,
	} {
		t.Run("a daemon whose proof "+name+" is never killed", func(t *testing.T) {
			f := &sweepFakes{failed: map[int]error{2001: err}}

			run(t, f.sweep([]int{2001, 2002}, f.saver(0, false, nil), nil))

			if got := f.killed(); !slices.Equal(got, []int{2002}) {
				t.Errorf("killed = %v, want 2002 alone", got)
			}
		})
	}

	t.Run("with no saver session every proved daemon is an orphan", func(t *testing.T) {
		f := &sweepFakes{}

		run(t, f.sweep([]int{3001, 3002, 3003}, f.saver(0, false, nil), nil))

		if got := f.killed(); !slices.Equal(got, []int{3001, 3002, 3003}) {
			t.Errorf("killed = %v, want all three", got)
		}
	})

	for name, read := range map[string]struct {
		pid     int
		present bool
		err     error
	}{
		"can't be read":             {0, false, errors.New("read session \"_portal-saver\": no answer in 1s")},
		"answers naming no process": {0, true, nil},
	} {
		t.Run("a saver session that "+name+" kills nothing, since it can't be spared", func(t *testing.T) {
			sink := &logtest.Sink{}
			f := &sweepFakes{}

			run(t, f.sweep([]int{4001, 4002}, f.saver(read.pid, read.present, read.err), slog.New(sink).With("component", "bootstrap")))

			if len(f.events) != 0 {
				t.Errorf("events = %q, want nothing proved or signalled", f.events)
			}
			sink.Records().WithMessage("sweep: saver pid unreadable, nothing killed").AtExactLevel(slog.LevelWarn).Only(t, "the WARN")
		})
	}

	t.Run("it sends SIGKILL and nothing else", func(t *testing.T) {
		f := &sweepFakes{}

		run(t, f.sweep([]int{5001}, f.saver(0, false, nil), nil))

		if want := []string{"prove 5001", "SIGKILL 5001", "await 5001"}; !slices.Equal(f.events, want) {
			t.Errorf("events = %q, want %q", f.events, want)
		}
	})

	t.Run("a pgrep that finds nothing reads no saver and kills nothing", func(t *testing.T) {
		sink := &logtest.Sink{}
		f := &sweepFakes{}

		run(t, f.sweep(nil, f.saver(saverPID, true, nil), slog.New(sink).With("component", "bootstrap")))

		if f.saverReads != 0 || len(f.events) != 0 {
			t.Errorf("saver reads = %d, events = %q; want none", f.saverReads, f.events)
		}
		if warns := sink.Records().AtOrAboveLevel(slog.LevelWarn); warns != nil {
			t.Errorf("warnings = %+v, want none", warns)
		}
	})

	t.Run("a pgrep that fails warns and kills nothing", func(t *testing.T) {
		sink := &logtest.Sink{}
		boom := errors.New("pgrep boom")
		f := &sweepFakes{}
		c := f.sweep(nil, f.saver(0, false, nil), slog.New(sink).With("component", "bootstrap"))
		c.Pgrep = func() ([]int, error) { return nil, boom }

		run(t, c)

		if len(f.events) != 0 {
			t.Errorf("events = %q, want none", f.events)
		}
		warn := sink.Records().WithMessage("sweep: pgrep failed").AtExactLevel(slog.LevelWarn).Only(t, "the pgrep WARN")
		if got := warn.ErrorAttr(t, "error"); !errors.Is(got, boom) {
			t.Errorf("error = %v, want %v", got, boom)
		}
	})

	t.Run("an unprovable pid warns; one proved no orphan is a DEBUG", func(t *testing.T) {
		sink := &logtest.Sink{}
		f := &sweepFakes{failed: map[int]error{6001: errUnprovable, 6002: errNotTheSaversDaemon}}

		run(t, f.sweep([]int{6001, 6002}, f.saver(0, false, nil), slog.New(sink).With("component", "bootstrap")))

		warn := sink.Records().WithMessage("sweep: pid unprovable, skipping").AtExactLevel(slog.LevelWarn).Only(t, "the unprovable WARN")
		if pid := warn.IntAttr(t, "target_pid"); pid != 6001 {
			t.Errorf("unprovable target_pid = %d, want 6001", pid)
		}
		debug := sink.Records().WithMessage("sweep: pid not proved an orphaned saver daemon, skipping").AtExactLevel(slog.LevelDebug).
			Only(t, "the not-proved DEBUG")
		if pid := debug.IntAttr(t, "target_pid"); pid != 6002 {
			t.Errorf("not-proved target_pid = %d, want 6002", pid)
		}
	})

	t.Run("a kill that fails warns, and the sweep goes on", func(t *testing.T) {
		sink := &logtest.Sink{}
		f := &sweepFakes{signalErrs: map[int]error{8001: syscall.EPERM}}

		run(t, f.sweep([]int{8001, 8002}, f.saver(0, false, nil), slog.New(sink).With("component", "bootstrap")))

		if got := f.killed(); !slices.Equal(got, []int{8001, 8002}) {
			t.Errorf("killed = %v, want both tried", got)
		}
		warn := sink.Records().WithMessage("sweep: kill failed").AtExactLevel(slog.LevelWarn).Only(t, "the kill WARN")
		if pid := warn.IntAttr(t, "target_pid"); pid != 8001 {
			t.Errorf("kill failed target_pid = %d, want 8001", pid)
		}
	})

	t.Run("an orphan that doesn't finish exiting warns, and the sweep goes on", func(t *testing.T) {
		sink := &logtest.Sink{}
		stillExiting := errors.New("pid 8001 still hadn't finished exiting 2s later")
		f := &sweepFakes{exitErrs: map[int]error{8001: stillExiting}}

		run(t, f.sweep([]int{8001, 8002}, f.saver(0, false, nil), slog.New(sink).With("component", "bootstrap")))

		if want := []string{"prove 8001", "SIGKILL 8001", "await 8001", "prove 8002", "SIGKILL 8002", "await 8002"}; !slices.Equal(f.events, want) {
			t.Errorf("events = %q, want %q", f.events, want)
		}
		warn := sink.Records().WithMessage("sweep: kill failed").AtExactLevel(slog.LevelWarn).Only(t, "the kill WARN")
		if pid := warn.IntAttr(t, "target_pid"); pid != 8001 {
			t.Errorf("kill failed target_pid = %d, want 8001", pid)
		}
	})

	for name, unwire := range map[string]func(*OrphanSweepCore){
		"Pgrep":     func(c *OrphanSweepCore) { c.Pgrep = nil },
		"SaverPID":  func(c *OrphanSweepCore) { c.SaverPID = nil },
		"Prove":     func(c *OrphanSweepCore) { c.Prove = nil },
		"Signal":    func(c *OrphanSweepCore) { c.Signal = nil },
		"AwaitExit": func(c *OrphanSweepCore) { c.AwaitExit = nil },
	} {
		t.Run("a sweep with no "+name+" kills nothing", func(t *testing.T) {
			sink := &logtest.Sink{}
			f := &sweepFakes{}
			c := f.sweep([]int{9001}, f.saver(0, false, nil), slog.New(sink).With("component", "bootstrap"))
			unwire(c)

			run(t, c)

			if got := f.killed(); len(got) != 0 {
				t.Errorf("killed = %v, want none", got)
			}
			sink.Records().WithMessage("sweep: not wired, nothing killed").AtExactLevel(slog.LevelWarn).Only(t, "the WARN")
		})
	}

	t.Run("a nil Logger is tolerated", func(t *testing.T) {
		f := &sweepFakes{}
		run(t, f.sweep([]int{12001}, f.saver(0, false, nil), nil))
	})
}
