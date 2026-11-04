//go:build !darwin

package proctest

import (
	"os/exec"
	"syscall"
	"testing"
)

// Held is darwin's alone: proctable reads processes only there.
type Held struct{}

// Start skips the test: a process can be held, and proved, only on darwin.
func Start(t testing.TB, _ *exec.Cmd, _ string) *Held {
	t.Helper()
	t.Skip("proctest holds processes only on darwin")
	return nil
}

func (*Held) PID() int                              { return 0 }
func (*Held) Exited() <-chan struct{}               { return nil }
func (*Held) Status() syscall.WaitStatus            { return 0 }
func (*Held) Signal(int, syscall.Signal) error      { return nil }
func (*Held) SignalGroup(syscall.Signal) error      { return nil }
func (*Held) Watch(testing.TB, int) <-chan struct{} { return nil }
