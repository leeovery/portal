package proctest

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/proctable"
)

// tree fakes the Reader: each pid's environment and its parent. A pid missing
// from either map fails that read.
type tree struct {
	envs    map[int][]string
	parents map[int]int
}

func (r tree) List() ([]proctable.Process, error) { return nil, errors.New("not listed") }

func (r tree) Args(pid int) (proctable.Args, error) {
	env, ok := r.envs[pid]
	if !ok {
		return proctable.Args{}, errors.New("no such process")
	}
	return proctable.Args{Argv: []string{"/bin/sh"}, Env: env}, nil
}

func (r tree) Parent(pid int) (int, error) {
	ppid, ok := r.parents[pid]
	if !ok {
		return 0, errors.New("no such process")
	}
	return ppid, nil
}

func TestProved(t *testing.T) {
	const held = 20
	world := tree{
		envs:    map[int][]string{10: nil, 11: {"MARK=mine"}, 20: nil, 30: nil, 40: nil},
		parents: map[int]int{10: 20, 11: 1, 20: 30, 30: 1, 40: 1},
	}
	for name, c := range map[string]struct {
		pid   int
		proof Proof
		want  bool
	}{
		"the held process itself":                    {20, PIDProof(held), true},
		"a process whose chain reaches the held one": {10, PIDProof(held), true},
		"the held process's parent":                  {30, PIDProof(held), false},
		"a stranger":                                 {40, PIDProof(held), false},
		"an orphan carrying the test's marker":       {11, EnvProof("MARK=mine"), true},
		"an orphan carrying another test's marker":   {11, EnvProof("MARK=theirs"), false},
		"an orphan the held process can't prove":     {11, PIDProof(held), false},
		"an orphan either proof proves":              {11, AnyProof(PIDProof(held), EnvProof("MARK=mine")), true},
		"a descendant either proof proves":           {10, AnyProof(PIDProof(held), EnvProof("MARK=mine")), true},
		"a pid whose args can't be read":             {99, PIDProof(99), false},
		"pid 1, whatever the proof":                  {1, PIDProof(1), false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Proved(c.pid, c.proof, world); got != c.want {
				t.Errorf("Proved(%d) = %v, want %v", c.pid, got, c.want)
			}
		})
	}
}
