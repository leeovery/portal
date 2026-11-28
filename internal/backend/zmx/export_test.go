package zmx

import (
	"fmt"
	"io/fs"
	"sync"
	"time"

	"github.com/leeovery/portal/internal/proctable"
)

// ShortenCeilings sets the ceiling of every socket operation to d, so tests
// of what happens past one need not wait out the real ones. Create keeps its
// own: a script's first run on macOS can take a few hundred milliseconds.
func ShortenCeilings(b *Backend, d time.Duration) {
	b.limits.probe, b.limits.kill, b.limits.detach, b.limits.history = d, d, d, d
}

// CreateCeiling is Create's own ceiling, which ShortenCeilings leaves as it
// is.
var CreateCeiling = defaultCeilings.create

// ShortenCreateCeiling sets Create's ceiling to d.
func ShortenCreateCeiling(b *Backend, d time.Duration) { b.limits.create = d }

// DefaultTracked is Portal's copy of the variables zmx tracks for a client
// whose environment has no ZMX_TRACK_ENV.
const DefaultTracked = defaultTracked

// Process is one process of a fake process table: its place in the list,
// and the arguments it was exec'd with, or the error reading them fails with.
// Reads, when set, fails its first reads of them. Cwd is the directory it
// works in; a read of an empty one fails.
type Process struct {
	proctable.Process
	Args  proctable.Args
	Err   error
	Reads *FailingReads
	Cwd   string
}

// FailingReads fails a process's reads of its arguments with each of its
// errors in turn, then lets them be read.
type FailingReads struct {
	mu   sync.Mutex
	errs []error
}

// FailReads fails the first reads of a process's arguments with errs.
func FailReads(errs ...error) *FailingReads { return &FailingReads{errs: errs} }

// next is the failure of the read it is asked for, nil once errs are spent.
func (r *FailingReads) next() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.errs) == 0 {
		return nil
	}
	err := r.errs[0]
	r.errs = r.errs[1:]
	return err
}

// FakeProcesses has the Backend read procs in place of the system's process
// table, or fail the listing with err.
func FakeProcesses(b *Backend, procs []Process, err error) {
	FakeProcessesFrom(b, func() ([]Process, error) { return procs, err })
}

// FakeProcessesFrom has the Backend read, at each listing and each read of
// one process, the processes table returns, or fail with table's error. A
// process's arguments and directory are read as the listing or read last
// taken shows them, and a process it doesn't show is gone.
func FakeProcessesFrom(b *Backend, table func() ([]Process, error)) {
	var (
		mu   sync.Mutex
		last []Process
	)
	take := func() ([]Process, error) {
		procs, err := table()
		if err != nil {
			return nil, err
		}
		mu.Lock()
		last = procs
		mu.Unlock()
		return procs, nil
	}
	gone := func(pid int) error { return fmt.Errorf("read process %d: %w", pid, proctable.ErrGone) }
	listed := func(pid int) (Process, bool) {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range last {
			if p.PID == pid {
				return p, true
			}
		}
		return Process{}, false
	}
	b.processes = processTable{
		list: func() ([]proctable.Process, error) {
			procs, err := take()
			if err != nil {
				return nil, err
			}
			list := make([]proctable.Process, len(procs))
			for i, p := range procs {
				list[i] = p.Process
			}
			return list, nil
		},
		read: func(pid int) (proctable.Process, error) {
			procs, err := take()
			if err != nil {
				return proctable.Process{}, err
			}
			for _, p := range procs {
				if p.PID == pid {
					return p.Process, nil
				}
			}
			return proctable.Process{}, gone(pid)
		},
		readArgs: func(pid int) (proctable.Args, error) {
			p, ok := listed(pid)
			if !ok {
				return proctable.Args{}, gone(pid)
			}
			if err := p.Reads.next(); err != nil {
				return proctable.Args{}, err
			}
			return p.Args, p.Err
		},
		cwd: func(pid int) (string, error) {
			p, ok := listed(pid)
			switch {
			case !ok:
				return "", gone(pid)
			case p.Cwd == "":
				return "", fmt.Errorf("read the directory of process %d: unreadable", pid)
			}
			return p.Cwd, nil
		},
	}
}

// FakeStat has WorkDir read the directories it compares with stat.
func FakeStat(b *Backend, stat func(string) (fs.FileInfo, error)) { b.stat = stat }

// OSC7 is the OSC 7 ReportDir sends for dir on the machine hostname.
func OSC7(hostname, dir string) []byte { return osc7(hostname, dir) }
