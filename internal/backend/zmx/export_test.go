package zmx

import (
	"errors"
	"time"

	"github.com/leeovery/portal/internal/proctable"
)

// ShortenCeilings sets the ceiling of every socket operation to d, so tests
// of what happens past one need not wait out the real ones. Create keeps its
// own: a script's first run on macOS can take a few hundred milliseconds.
func ShortenCeilings(b *Backend, d time.Duration) {
	b.limits.probe, b.limits.kill, b.limits.detach, b.limits.history = d, d, d, d
}

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

// FakeProcesses has AttachClients read procs in place of the system's
// process table, or fail the listing with err.
func FakeProcesses(b *Backend, procs []Process, err error) {
	b.processes = processTable{
		list: func() ([]proctable.Process, error) {
			if err != nil {
				return nil, err
			}
			list := make([]proctable.Process, len(procs))
			for i, p := range procs {
				list[i] = p.Process
			}
			return list, nil
		},
		readArgs: func(pid int) (proctable.Args, error) {
			for _, p := range procs {
				if p.PID == pid {
					return p.Args, p.Err
				}
			}
			return proctable.Args{}, errors.New("no such process")
		},
	}
}
