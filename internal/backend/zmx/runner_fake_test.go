package zmx_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

const testDir = "/tmp/zx-unit"

// scriptedRunner answers each zmx subcommand (Argv[1]) from a script and
// records every run. An unscripted subcommand fails the test.
type scriptedRunner struct {
	t       *testing.T
	replies map[string]reply
	calls   []runCall
}

type reply struct {
	out string
	err error
}

type runCall struct {
	raw bool
	cmd backend.Command
}

func newRunner(t *testing.T, replies map[string]reply) *scriptedRunner {
	return &scriptedRunner{t: t, replies: replies}
}

func (r *scriptedRunner) Run(cmd backend.Command) (string, error) {
	out, err := r.answer(false, cmd)
	return strings.TrimSpace(out), err
}

func (r *scriptedRunner) RunRaw(cmd backend.Command) (string, error) {
	return r.answer(true, cmd)
}

func (r *scriptedRunner) answer(raw bool, cmd backend.Command) (string, error) {
	r.calls = append(r.calls, runCall{raw: raw, cmd: cmd})
	rep, ok := r.replies[cmd.Argv[1]]
	if !ok {
		r.t.Errorf("unscripted zmx run %v", cmd.Argv)
		return "", fmt.Errorf("unscripted zmx run %v", cmd.Argv)
	}
	if rep.err != nil {
		return "", rep.err
	}
	return rep.out, nil
}

// callsTo returns the recorded runs of one subcommand.
func (r *scriptedRunner) callsTo(sub string) []runCall {
	var out []runCall
	for _, c := range r.calls {
		if c.cmd.Argv[1] == sub {
			out = append(out, c)
		}
	}
	return out
}

// zmxFailure is a run zmx ended with exit 1 and stderr.
func zmxFailure(stderr string) error {
	return &zmx.CommandError{Stderr: stderr, Err: errors.New("exit status 1")}
}

// fixedEnviron is an environment as a session's shell would inherit it.
func fixedEnviron(kv ...string) func() []string {
	return func() []string { return slices.Clone(kv) }
}

func newBackend(r zmx.Runner, environ ...string) *zmx.Backend {
	return newBackendWith(r, fixedEnviron(environ...))
}

func newBackendWith(r zmx.Runner, environ func() []string) *zmx.Backend {
	return zmx.New(testDir, environ, r)
}

// envValues returns every value env carries for key, in order.
func envValues(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			out = append(out, v)
		}
	}
	return out
}
