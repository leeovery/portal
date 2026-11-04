package saverhost_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
)

func TestStop(t *testing.T) {
	t.Run("no saver is nothing to stop, and nothing is read or signalled", func(t *testing.T) {
		b := backendtest.New()
		p := newProcs(nil)

		stopped, err := newStopper(t, b, p).Stop(t.Context())

		if stopped || err != nil {
			t.Errorf("Stop = %v, %v; want false, nil", stopped, err)
		}
		if len(p.events) != 0 || len(kills(b)) != 0 {
			t.Errorf("events = %q, kills = %q; want none", p.events, kills(b))
		}
	})

	t.Run("it proves the daemon, sends it SIGTERM, and kills the session once it has exited", func(t *testing.T) {
		sink := logtest.Install(t)
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		stopped, err := newStopper(t, b, p).Stop(t.Context())

		if !stopped || err != nil {
			t.Fatalf("Stop = %v, %v; want true, nil", stopped, err)
		}
		if want := []string{"identify 4242", "environ 4242", "SIGTERM 4242"}; !slices.Equal(p.events, want) {
			t.Errorf("events = %q, want %q: the proof, then the signal", p.events, want)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
		started := sink.Records().Matching("saver", "stop started").Only(t, "the stop's start")
		if pid := started.IntAttr(t, "target_pid"); pid != daemonPID {
			t.Errorf("stop started target_pid = %d, want %d", pid, daemonPID)
		}
		sink.Records().Matching("saver", "daemon exited").Only(t, "the daemon's exit").RequireDuration(t, "took")
	})

	t.Run("it waits out TermWait, proves the daemon again and sends it SIGKILL", func(t *testing.T) {
		sink := logtest.Install(t)
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGKILL))})
		start := time.Now()

		stopped, err := newStopper(t, b, p).Stop(t.Context())

		if !stopped || err != nil {
			t.Fatalf("Stop = %v, %v; want true, nil", stopped, err)
		}
		if took := time.Since(start); took < termWait {
			t.Errorf("Stop took %s, want SIGKILL only once TermWait (%s) passed", took, termWait)
		}
		want := []string{"identify 4242", "environ 4242", "SIGTERM 4242", "identify 4242", "environ 4242", "SIGKILL 4242"}
		if !slices.Equal(p.events, want) {
			t.Errorf("events = %q, want %q: a fresh proof right before the SIGKILL", p.events, want)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's, once the daemon is gone", got)
		}
		escalated := sink.Records().Matching("saver", "stop escalated").Only(t, "the escalation")
		if reason := escalated.AttrString(t, "reason"); reason != "term-timeout" {
			t.Errorf("stop escalated reason = %q, want term-timeout", reason)
		}
	})

	for name, change := range map[string]func(p *proc){
		"can't be read":                      func(p *proc) { p.identifyErr = errors.New("ps: malformed output") },
		"is another program":                 func(p *proc) { p.verdict = state.IdentifyNotPortalDaemon },
		"lacks the saver session's marks":    func(p *proc) { p.env = []string{"PATH=/usr/bin"} },
		"has its environment hidden from it": func(p *proc) { p.envErr = proctable.ErrUnsupported },
	} {
		t.Run("a daemon that outlives TermWait and then "+name+" is sent no SIGKILL, and its session is left", func(t *testing.T) {
			b := withSaver(daemonPID)
			p := newProcs(map[int]*proc{daemonPID: daemon(func(p *proc, sig syscall.Signal) {
				if sig == syscall.SIGTERM {
					change(p)
				}
			})})

			stopped, err := newStopper(t, b, p).Stop(t.Context())

			if stopped || err == nil {
				t.Errorf("Stop = %v, %v; want a failure", stopped, err)
			}
			if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
				t.Errorf("signals = %q, want the SIGTERM alone", got)
			}
			if got := kills(b); len(got) != 0 {
				t.Errorf("kills = %q, want the session left while its daemon may run", got)
			}
		})
	}

	t.Run("a daemon gone by its re-proof is stopped, sent no SIGKILL", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		reads := 0
		s, err := saverhost.NewStopper(saverhost.Config{
			Backend: b, Marks: marks, Signal: p.signal,
			Identify: func(pid int) (state.IdentifyResult, error) {
				// Gone between the wait's last read and the re-proof.
				if reads++; reads > 1 {
					p.table[daemonPID].exited = true
				}
				return p.identify(pid)
			},
			Environ: p.environ, Alive: p.alive, TermWait: termWait, KillWait: killWait, Poll: poll,
		})
		if err != nil {
			t.Fatal(err)
		}

		stopped, err := s.Stop(t.Context())

		if !stopped || err != nil {
			t.Fatalf("Stop = %v, %v; want true, nil", stopped, err)
		}
		if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
			t.Errorf("signals = %q, want the SIGTERM alone", got)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	for name, tc := range map[string]struct {
		change func(p *proc)
		want   error
	}{
		"is another program":                      {func(p *proc) { p.verdict = state.IdentifyNotPortalDaemon }, saverhost.ErrNotSaverDaemon},
		"lacks the session's name":                {func(p *proc) { p.env = []string{marks[1]} }, saverhost.ErrNotSaverDaemon},
		"lacks the backend's directory":           {func(p *proc) { p.env = []string{marks[0]} }, saverhost.ErrNotSaverDaemon},
		"runs in another zmx directory":           {func(p *proc) { p.env = []string{marks[0], "ZMX_DIR=/tmp/zx-elsewhere"} }, saverhost.ErrNotSaverDaemon},
		"names the directory twice, differently":  {func(p *proc) { p.env = append(slices.Clone(marks), "ZMX_DIR=/tmp/zx-elsewhere") }, saverhost.ErrNotSaverDaemon},
		"can't be identified":                     {func(p *proc) { p.identifyErr = errors.New("ps: malformed output") }, saverhost.ErrUnreadable},
		"has an environment that can't be read":   {func(p *proc) { p.envErr = errors.New("sysctl: EPERM") }, saverhost.ErrUnreadable},
		"has its environment unread off darwin":   {func(p *proc) { p.envErr = proctable.ErrUnsupported }, saverhost.ErrUnreadable},
		"is a portal daemon with no marks at all": {func(p *proc) { p.env = nil }, saverhost.ErrNotSaverDaemon},
	} {
		t.Run("a saver session's process that "+name+" is sent nothing, and its session is left", func(t *testing.T) {
			sink := logtest.Install(t)
			b := withSaver(daemonPID)
			d := daemon(exitsOn(syscall.SIGTERM, syscall.SIGKILL))
			tc.change(d)
			p := newProcs(map[int]*proc{daemonPID: d})

			stopped, err := newStopper(t, b, p).Stop(t.Context())

			if stopped || !errors.Is(err, tc.want) {
				t.Errorf("Stop = %v, %v; want false and %v", stopped, err, tc.want)
			}
			if got := p.signals(); len(got) != 0 {
				t.Errorf("signals = %q, want none", got)
			}
			if got := kills(b); len(got) != 0 {
				t.Errorf("kills = %q, want the session left", got)
			}
			refused := sink.Records().Matching("saver", "stop refused").AtExactLevel(slog.LevelWarn).Only(t, "the refusal")
			if pid := refused.IntAttr(t, "target_pid"); pid != daemonPID {
				t.Errorf("stop refused target_pid = %d, want %d", pid, daemonPID)
			}
		})
	}

	t.Run("a daemon gone before its proof is sent nothing, and its session is killed", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(nil)

		stopped, err := newStopper(t, b, p).Stop(t.Context())

		if !stopped || err != nil {
			t.Errorf("Stop = %v, %v; want true, nil", stopped, err)
		}
		if got := p.signals(); len(got) != 0 {
			t.Errorf("signals = %q, want none", got)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a daemon gone as its SIGTERM is sent is stopped", func(t *testing.T) {
		b := withSaver(daemonPID)
		d := daemon(nil)
		p := newProcs(map[int]*proc{daemonPID: d})
		s, err := saverhost.NewStopper(saverhost.Config{
			Backend: b, Marks: marks, Identify: p.identify, Environ: p.environ, Alive: p.alive,
			Signal: func(pid int, sig syscall.Signal) error {
				d.exited = true
				return p.signal(pid, sig)
			},
			TermWait: termWait, KillWait: killWait, Poll: poll,
		})
		if err != nil {
			t.Fatal(err)
		}

		if stopped, err := s.Stop(t.Context()); !stopped || err != nil {
			t.Errorf("Stop = %v, %v; want true, nil", stopped, err)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a daemon that outlives its SIGKILL fails the stop, and its session is left", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})

		stopped, err := newStopper(t, b, p).Stop(t.Context())

		if stopped || err == nil {
			t.Errorf("Stop = %v, %v; want a failure", stopped, err)
		}
		if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242", "SIGKILL 4242"}) {
			t.Errorf("signals = %q, want SIGTERM then SIGKILL", got)
		}
		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want the session left", got)
		}
	})

	t.Run("a signal that fails fails the stop", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		s, err := saverhost.NewStopper(saverhost.Config{
			Backend: b, Marks: marks, Identify: p.identify, Environ: p.environ, Alive: p.alive,
			Signal:   func(int, syscall.Signal) error { return syscall.EPERM },
			TermWait: termWait, KillWait: killWait, Poll: poll,
		})
		if err != nil {
			t.Fatal(err)
		}

		if stopped, err := s.Stop(t.Context()); stopped || !errors.Is(err, syscall.EPERM) {
			t.Errorf("Stop = %v, %v; want false and EPERM", stopped, err)
		}
		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want the session left", got)
		}
	})

	t.Run("a saver session that doesn't answer is read as nothing, and sent nothing", func(t *testing.T) {
		b := withSaver(daemonPID)
		b.StopAnswering(saver.SessionName)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		stopped, err := newStopper(t, b, p).Stop(t.Context())

		if stopped || !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Stop = %v, %v; want false and ErrTimeout", stopped, err)
		}
		if len(p.events) != 0 {
			t.Errorf("events = %q, want none", p.events)
		}
	})

	t.Run("a saver session naming no process is sent nothing", func(t *testing.T) {
		b := withSaver(0)
		p := newProcs(nil)

		stopped, err := newStopper(t, b, p).Stop(t.Context())

		if stopped || !errors.Is(err, saverhost.ErrNoProcess) {
			t.Errorf("Stop = %v, %v; want false and ErrNoProcess", stopped, err)
		}
		if len(p.events) != 0 {
			t.Errorf("events = %q, want none", p.events)
		}
	})

	t.Run("a wait the context ends fails the stop, sending no SIGKILL", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		ctx, cancel := context.WithCancel(t.Context())
		p.table[daemonPID].onSignal = func(*proc, syscall.Signal) { cancel() }

		stopped, err := newStopper(t, b, p).Stop(ctx)

		if stopped || !errors.Is(err, context.Canceled) {
			t.Errorf("Stop = %v, %v; want false and the context's cause", stopped, err)
		}
		if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
			t.Errorf("signals = %q, want the SIGTERM alone", got)
		}
	})
}

func TestProve(t *testing.T) {
	t.Run("it proves a portal state daemon carrying every mark", func(t *testing.T) {
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		if err := newStopper(t, backendtest.New(), p).Prove(daemonPID); err != nil {
			t.Errorf("Prove = %v, want proved", err)
		}
	})

	t.Run("it reads the identity first, and the environment only of a portal state daemon", func(t *testing.T) {
		other := daemon(nil)
		other.verdict = state.IdentifyNotPortalDaemon
		p := newProcs(map[int]*proc{daemonPID: other})

		_ = newStopper(t, backendtest.New(), p).Prove(daemonPID)

		if want := []string{"identify 4242"}; !slices.Equal(p.events, want) {
			t.Errorf("events = %q, want %q", p.events, want)
		}
	})

	t.Run("a process gone at either read is ErrGone", func(t *testing.T) {
		if err := newStopper(t, backendtest.New(), newProcs(nil)).Prove(daemonPID); !errors.Is(err, saverhost.ErrGone) {
			t.Errorf("Prove of no process = %v, want ErrGone", err)
		}
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		s, err := saverhost.NewStopper(saverhost.Config{
			Backend: backendtest.New(), Marks: marks, Signal: p.signal, Identify: p.identify, Alive: p.alive,
			Environ: func(pid int) ([]string, error) {
				p.table[pid].exited = true
				return p.environ(pid)
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Prove(daemonPID); !errors.Is(err, saverhost.ErrGone) {
			t.Errorf("Prove of a process gone before its environment's read = %v, want ErrGone", err)
		}
	})

	t.Run("a mark is matched whole, never as a prefix", func(t *testing.T) {
		d := daemon(nil)
		d.env = []string{marks[0], marks[1] + "0"}
		p := newProcs(map[int]*proc{daemonPID: d})

		if err := newStopper(t, backendtest.New(), p).Prove(daemonPID); !errors.Is(err, saverhost.ErrNotSaverDaemon) {
			t.Errorf("Prove = %v, want ErrNotSaverDaemon for %s", err, d.env[1])
		}
	})
}

func TestSaverPID(t *testing.T) {
	for name, tc := range map[string]struct {
		b           *backendtest.Fake
		wantPID     int
		wantPresent bool
		wantErr     error
	}{
		"the saver session's process": {withSaver(daemonPID), daemonPID, true, nil},
		"no saver session":            {backendtest.New(), 0, false, nil},
		"a session naming no process": {withSaver(0), 0, false, saverhost.ErrNoProcess},
	} {
		t.Run(name, func(t *testing.T) {
			pid, present, err := newStopper(t, tc.b, newProcs(nil)).SaverPID(t.Context())
			if pid != tc.wantPID || present != tc.wantPresent || !errors.Is(err, tc.wantErr) {
				t.Errorf("SaverPID = %d, %v, %v; want %d, %v, %v", pid, present, err, tc.wantPID, tc.wantPresent, tc.wantErr)
			}
		})
	}
}

func TestNewStopper(t *testing.T) {
	signal := func(int, syscall.Signal) error { return nil }
	for name, cfg := range map[string]saverhost.Config{
		"no backend":                {Marks: marks, Signal: signal},
		"no signal":                 {Backend: backendtest.New(), Marks: marks},
		"no marks":                  {Backend: backendtest.New(), Signal: signal},
		"a mark naming no variable": {Backend: backendtest.New(), Marks: []string{"=x"}, Signal: signal},
		"a mark that is no entry":   {Backend: backendtest.New(), Marks: []string{"ZMX_DIR"}, Signal: signal},
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if s, err := saverhost.NewStopper(cfg); err == nil {
				t.Errorf("NewStopper = %v, want a refusal", s)
			}
		})
	}
}
