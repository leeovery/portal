package spawn

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/runnerreg"
)

// fakeSessions is the backend as detection reads it: the session this
// process runs in, and the leader's environment of each session.
type fakeSessions struct {
	current string
	leaders map[string]backend.ClientEnv
	err     error
}

func (f fakeSessions) Current() string { return f.current }

func (f fakeSessions) LeaderEnv(_ context.Context, name string) (backend.ClientEnv, error) {
	return f.leaders[name], f.err
}

// fakeRunners answers ConfirmSession for the runners it holds, by token, each on its
// session, and records each ask.
type fakeRunners struct {
	pids     map[string]int
	sessions map[string]string
	err      error
	asked    []string
}

func (f *fakeRunners) ConfirmSession(_ context.Context, token, session string) (int, error) {
	f.asked = append(f.asked, token+"@"+session)
	if f.err != nil {
		return 0, f.err
	}
	pid, ok := f.pids[token]
	if !ok || f.sessions[token] != session {
		return 0, runnerreg.ErrNoRunner
	}
	return pid, nil
}

// fakeClients answers AttachClients with each session's zmx attach pids.
type fakeClients struct {
	pids  map[string][]int
	err   error
	asked []string
}

func (f *fakeClients) AttachClients(session string) ([]int, error) {
	f.asked = append(f.asked, session)
	return f.pids[session], f.err
}

var (
	ghosttyCommand  = "/Applications/Ghostty.app/Contents/MacOS/ghostty"
	ghosttyAppPath  = "/Applications/Ghostty.app"
	terminalCommand = "/Applications/Terminal.app/Contents/MacOS/Terminal"
	terminalAppPath = "/Applications/Terminal.app"
)

// localWalkSeams walk 501 to Ghostty, 502 to Terminal, and 601, a remote or
// mosh client, to nothing.
func localWalkSeams() (*fakeWalker, *fakeReader) {
	walker := &fakeWalker{procs: map[int]fakeProc{
		501: {ppid: 1, command: ghosttyCommand},
		502: {ppid: 1, command: terminalCommand},
		601: {ppid: 1, command: "mosh-server"},
	}}
	reader := &fakeReader{bundles: map[string]fakeBundle{
		ghosttyAppPath:  {bundleID: "com.mitchellh.ghostty", name: "Ghostty"},
		terminalAppPath: {bundleID: "com.apple.Terminal", name: "Terminal"},
	}}
	return walker, reader
}

// leaderWith is a leader's tracked environment: zmx's default list, unset,
// with any vars given.
func leaderWith(vars ...backend.EnvVar) backend.ClientEnv {
	return append(backend.ClientEnv{{Name: "DISPLAY", Unset: true}, {Name: "SSH_CONNECTION", Unset: true}}, vars...)
}

func runnerLeader(token string) backend.ClientEnv {
	return leaderWith(backend.EnvVar{Name: runnerreg.TokenVar, Value: token})
}

// insideDetector detects inside session dev, whose leader is leader, with
// runner tok123 on dev as pid 501 and the given raw clients.
func insideDetector(leader backend.ClientEnv, raw ...int) (*Detector, *fakeRunners, *fakeClients) {
	walker, reader := localWalkSeams()
	runners := &fakeRunners{pids: map[string]int{"tok123": 501, "elsewh": 502}, sessions: map[string]string{"tok123": "dev", "elsewh": "api"}}
	clients := &fakeClients{pids: map[string][]int{"dev": raw}}
	return &Detector{
		sessions: fakeSessions{current: "dev", leaders: map[string]backend.ClientEnv{"dev": leader}},
		runners:  runners,
		clients:  clients,
		getenv:   func(string) string { panic("detection inside a session read the environment") },
		walker:   walker,
		reader:   reader,
	}, runners, clients
}

func TestDetectInsideSession(t *testing.T) {
	t.Run("it walks from the leading runner, and looks for no raw client", func(t *testing.T) {
		d, runners, clients := insideDetector(runnerLeader("tok123"), 502)

		got, route, err := d.resolve()

		if err != nil || got.BundleID != "com.mitchellh.ghostty" || route != routeInsideRunner {
			t.Errorf("resolve = %+v, %q, %v; want Ghostty through the runner", got, route, err)
		}
		if len(runners.asked) != 1 || runners.asked[0] != "tok123@dev" {
			t.Errorf("runners asked %q, want tok123 on dev", runners.asked)
		}
		if len(clients.asked) != 0 {
			t.Errorf("the process table was searched for %q with a runner leading", clients.asked)
		}
	})

	t.Run("it walks from the session's one raw client when the leader carries no token", func(t *testing.T) {
		d, runners, _ := insideDetector(leaderWith(), 502)

		got, route, err := d.resolve()

		if err != nil || got.BundleID != "com.apple.Terminal" || route != routeInsideRaw {
			t.Errorf("resolve = %+v, %q, %v; want Terminal through the raw client", got, route, err)
		}
		if len(runners.asked) != 0 {
			t.Errorf("runners asked %q with a raw client leading", runners.asked)
		}
	})

	t.Run("a remote leader walks to no terminal, cleanly", func(t *testing.T) {
		d, _, _ := insideDetector(leaderWith(), 601)

		if got, _, err := d.resolve(); err != nil || !got.IsNull() {
			t.Errorf("resolve = %+v, %v; want a clean NULL", got, err)
		}
	})

	for name, tc := range map[string]struct {
		leader backend.ClientEnv
		raw    []int
	}{
		"no client leading":                      {leader: nil, raw: []int{501}},
		"a token the registry doesn't hold":      {leader: runnerLeader("nosuch"), raw: []int{501}},
		"a token naming a runner elsewhere":      {leader: runnerLeader("elsewh"), raw: []int{501}},
		"an empty token":                         {leader: runnerLeader(""), raw: []int{501}},
		"no raw client in the process table":     {leader: leaderWith()},
		"two raw clients in the process table":   {leader: leaderWith(), raw: []int{501, 502}},
		"three raw clients in the process table": {leader: leaderWith(), raw: []int{501, 502, 601}},
	} {
		t.Run(name+" is uncertain, and walks nothing", func(t *testing.T) {
			d, _, _ := insideDetector(tc.leader, tc.raw...)
			walker := &fakeWalker{}
			d.walker = walker

			got, _, err := d.resolve()

			if !errors.Is(err, errUncertain) || errors.Is(err, ErrDetectTransient) || !got.IsNull() {
				t.Errorf("resolve = %+v, %v; want NULL, uncertain", got, err)
			}
			if len(walker.calls) != 0 {
				t.Errorf("walked %v", walker.calls)
			}
		})
	}

	t.Run("an unsure runner's refusal is uncertain, and walks nothing", func(t *testing.T) {
		d, runners, _ := insideDetector(runnerLeader("tok123"), 502)
		runners.err = fmt.Errorf("ask runner 501 to confirm-session: %w: unsure", runnerreg.ErrRefused)

		if got, _, err := d.resolve(); !errors.Is(err, errUncertain) || !got.IsNull() {
			t.Errorf("resolve = %+v, %v; want NULL, uncertain", got, err)
		}
	})

	for name, setUp := range map[string]func(*Detector, *fakeRunners, *fakeClients) error{
		"a leader read that fails": func(d *Detector, _ *fakeRunners, _ *fakeClients) error {
			failure := errors.New("no answer in 1s")
			d.sessions = fakeSessions{current: "dev", err: failure}
			return failure
		},
		"a runner that can't be asked": func(_ *Detector, r *fakeRunners, _ *fakeClients) error {
			r.err = errors.New("reach runner 501: connection refused")
			return r.err
		},
		"a process table that can't be read": func(d *Detector, _ *fakeRunners, c *fakeClients) error {
			d.sessions = fakeSessions{current: "dev", leaders: map[string]backend.ClientEnv{"dev": leaderWith()}}
			c.err = errors.New("sysctl failed")
			return c.err
		},
	} {
		t.Run(name+" is transient, keeping its cause", func(t *testing.T) {
			d, runners, clients := insideDetector(runnerLeader("tok123"), 502)
			cause := setUp(d, runners, clients)

			got, _, err := d.resolve()

			if !errors.Is(err, ErrDetectTransient) || !errors.Is(err, cause) || !got.IsNull() {
				t.Errorf("resolve = %+v, %v; want NULL, transient, wrapping %v", got, err, cause)
			}
		})
	}

	t.Run("a walk that fails from the leader is transient", func(t *testing.T) {
		psFailure := errors.New("ps: operation not permitted")
		d, _, _ := insideDetector(runnerLeader("tok123"))
		d.walker = &fakeWalker{procs: map[int]fakeProc{501: {err: psFailure}}}

		if got, _, err := d.resolve(); !errors.Is(err, ErrDetectTransient) || !errors.Is(err, psFailure) || !got.IsNull() {
			t.Errorf("resolve = %+v, %v; want NULL, transient, wrapping the ps failure", got, err)
		}
	})
}
