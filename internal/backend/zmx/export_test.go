package zmx

import (
	"fmt"
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
type Process struct {
	proctable.Process
	Args proctable.Args
	Err  error
}

// FakeProcesses has the Backend read procs in place of the system's process
// table, or fail the listing with err.
func FakeProcesses(b *Backend, procs []Process, err error) {
	FakeProcessesFrom(b, func() ([]Process, error) { return procs, err })
}

// FakeProcessesFrom has the Backend read, at each listing, the processes
// table returns, or fail the listing with table's error. A process's
// arguments are read as the listing last taken shows them, and a process it
// doesn't show is gone.
func FakeProcessesFrom(b *Backend, table func() ([]Process, error)) {
	var (
		mu   sync.Mutex
		last []Process
	)
	b.processes = processTable{
		list: func() ([]proctable.Process, error) {
			procs, err := table()
			if err != nil {
				return nil, err
			}
			mu.Lock()
			last = procs
			mu.Unlock()
			list := make([]proctable.Process, len(procs))
			for i, p := range procs {
				list[i] = p.Process
			}
			return list, nil
		},
		readArgs: func(pid int) (proctable.Args, error) {
			mu.Lock()
			defer mu.Unlock()
			for _, p := range last {
				if p.PID == pid {
					return p.Args, p.Err
				}
			}
			return proctable.Args{}, fmt.Errorf("read arguments of process %d: %w", pid, proctable.ErrGone)
		},
	}
}
