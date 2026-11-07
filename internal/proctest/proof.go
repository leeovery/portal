// Package proctest proves a process is a test's own before the test signals
// it. A pid proved even a moment earlier may since have been reused, so every
// proof is read from the kernel as the signal is sent. Test-only: production
// code must not import this package.
package proctest

import (
	"slices"

	"github.com/leeovery/portal/internal/proctable"
)

// Reader reads this user's processes as each read is made: List, and a
// process's args and parent. A proof reads a process's parent only after its
// args, from the kernel as it then stands, so a pid reused between the two
// reads never borrows the parent its last holder had.
type Reader interface {
	List() ([]proctable.Process, error)
	Args(pid int) (proctable.Args, error)
	Parent(pid int) (int, error)
}

// Kernel is the Reader over the kernel's process table.
type Kernel struct{}

func (Kernel) List() ([]proctable.Process, error)   { return proctable.List() }
func (Kernel) Args(pid int) (proctable.Args, error) { return proctable.ReadArgs(pid) }
func (Kernel) Parent(pid int) (int, error)          { return proctable.Parent(pid) }

// Proof says whether one process, read as it now stands, is the test's own,
// and with it every process below it on the chain a walk took to reach it.
type Proof func(pid int, args proctable.Args) bool

// EnvProof proves a process whose environment holds entry, KEY=value
// exactly. macOS hides a platform binary's environment, /bin/sh's among
// them, so only a binary of the test's own carries one it can read.
func EnvProof(entry string) Proof {
	return func(_ int, args proctable.Args) bool { return slices.Contains(args.Env, entry) }
}

// PIDProof proves the process pid. It proves only while pid can't pass to
// another process: one the test started and has not reaped.
func PIDProof(pid int) Proof {
	return func(p int, _ proctable.Args) bool { return p == pid }
}

// AnyProof proves a process any of proofs proves.
func AnyProof(proofs ...Proof) Proof {
	return func(pid int, args proctable.Args) bool {
		return slices.ContainsFunc(proofs, func(p Proof) bool { return p(pid, args) })
	}
}

// Proved reports whether proof proves pid, or a process its parent chain
// reaches, read through r. It reads each process's args, then its parent, and
// once proof holds checks the chain it walked still stands. A walk that
// reaches pid 1, fails a read or repeats a pid proves nothing, so an orphan
// reparented to launchd is never proved: it leaks rather than risk a wrong
// signal.
func Proved(pid int, proof Proof, r Reader) bool {
	var chain []int
	for pid > 1 && !slices.Contains(chain, pid) {
		chain = append(chain, pid)
		args, err := r.Args(pid)
		if err != nil {
			return false
		}
		if proof(pid, args) {
			return standing(chain, r)
		}
		if pid, err = r.Parent(pid); err != nil {
			return false
		}
	}
	return false
}

// standing reports whether each process of chain still has the next as its
// parent, read after every args read the walk made. A process whose parent
// exits is reparented and never gets that parent back, so a match proves the
// parent outlived the read of its args: its pid was not taken by another
// process that the read then proved.
func standing(chain []int, r Reader) bool {
	for i := 0; i+1 < len(chain); i++ {
		if parent, err := r.Parent(chain[i]); err != nil || parent != chain[i+1] {
			return false
		}
	}
	return true
}
