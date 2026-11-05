package zmx

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/proctable"
)

// readsOf is a processTable whose reads of a process's arguments meet errs in
// turn and then answer argv, counting the reads.
func readsOf(argv []string, errs ...error) (processTable, *int) {
	reads := 0
	return processTable{readArgs: func(int) (proctable.Args, error) {
		reads++
		if len(errs) > 0 {
			err := errs[0]
			errs = errs[1:]
			return proctable.Args{}, err
		}
		return proctable.Args{Argv: argv}, nil
	}}, &reads
}

func TestProcessTableArgs(t *testing.T) {
	midExec := fmt.Errorf("read arguments of process 40: %w", syscall.EINVAL)
	argv := []string{"zmx", "attach", "dev"}

	t.Run("a read that answers is read once", func(t *testing.T) {
		table, reads := readsOf(argv)

		if got, err := table.args(t.Context(), 40); err != nil || !slices.Equal(got.Argv, argv) || *reads != 1 {
			t.Errorf("args = %q, %v after %d reads; want %q after one", got.Argv, err, *reads, argv)
		}
	})

	t.Run("a process whose reads fail a while is read until it answers", func(t *testing.T) {
		table, reads := readsOf(argv, midExec, fmt.Errorf("read arguments of process 40: %w", syscall.EIO), midExec)

		if got, err := table.args(t.Context(), 40); err != nil || !slices.Equal(got.Argv, argv) || *reads != 4 {
			t.Errorf("args = %q, %v after %d reads; want %q after four", got.Argv, err, *reads, argv)
		}
	})

	t.Run("a process gone between reads that fail is gone", func(t *testing.T) {
		table, reads := readsOf(argv, midExec, fmt.Errorf("read arguments of process 40: %w", proctable.ErrGone), midExec)

		if _, err := table.args(t.Context(), 40); !errors.Is(err, proctable.ErrGone) || *reads != 2 {
			t.Errorf("args = %v after %d reads; want ErrGone after two", err, *reads)
		}
	})

	t.Run("a process that never answers fails with its read's failure once the bound has passed", func(t *testing.T) {
		reads := 0
		table := processTable{readArgs: func(int) (proctable.Args, error) {
			reads++
			return proctable.Args{}, midExec
		}}
		start := time.Now()

		_, err := table.args(t.Context(), 40)

		took := time.Since(start)
		if !errors.Is(err, syscall.EINVAL) {
			t.Errorf("args = %v, want the read's failure", err)
		}
		if took < settleWithin {
			t.Errorf("args gave up after %s, want it reading for %s", took, settleWithin)
		}
		if most := int(settleWithin/settleEvery) + 1; reads < 2 || reads > most {
			t.Errorf("args read %d times, want more than once and at most %d, %s apart", reads, most, settleEvery)
		}
	})

	t.Run("a process that never answers is read no more once ctx ends", func(t *testing.T) {
		reads := 0
		ctx, cancel := context.WithCancel(t.Context())
		table := processTable{readArgs: func(int) (proctable.Args, error) {
			reads++
			if reads == 2 {
				cancel()
			}
			return proctable.Args{}, midExec
		}}

		_, err := table.args(ctx, 40)

		if !errors.Is(err, syscall.EINVAL) || reads != 2 {
			t.Errorf("args = %v after %d reads; want the read's failure after the two before ctx ended", err, reads)
		}
	})
}

// A process whose environment the kernel doesn't show is no session's daemon,
// though its argv names the session and the directory it names, none, is
// zmx's default one, the Backend's here.
func TestSessionDaemon_ProvesNoneWhoseEnvironmentTheKernelDoesntShow(t *testing.T) {
	b := &Backend{dir: DirFor(nil)}
	hidden := proctable.Process{PID: 100, PPID: 1, Command: "zmx"}
	b.processes = processTable{
		list: func() ([]proctable.Process, error) { return []proctable.Process{hidden}, nil },
		readArgs: func(int) (proctable.Args, error) {
			return proctable.Args{Argv: []string{"zmx", "attach", "work"}}, nil
		},
	}

	if d, err := b.sessionDaemon(t.Context(), "work"); !errors.Is(err, errUnproved) {
		t.Errorf("sessionDaemon(work) = %+v, %v; want errUnproved", d, err)
	}
}
