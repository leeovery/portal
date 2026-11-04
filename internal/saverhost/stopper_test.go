package saverhost_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
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

// tornDown is the Fake whose saver session goes as its kill fails, as zmx's
// does when it tears the session down under the kill's connection.
type tornDown struct{ *backendtest.Fake }

func (b *tornDown) Kill(ctx context.Context, name string) error {
	if err := b.Fake.Kill(ctx, name); err != nil {
		return err
	}
	return syscall.ECONNRESET
}

func TestStop(t *testing.T) {
	t.Run("it proves the daemon, sends it SIGTERM, and kills the session once it has exited", func(t *testing.T) {
		sink := logtest.Install(t)
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); err != nil {
			t.Fatalf("Stop: %v", err)
		}

		if want := []string{"identify 4242", "args 4242", "SIGTERM 4242"}; !slices.Equal(p.events, want) {
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

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); err != nil {
			t.Fatalf("Stop: %v", err)
		}

		if took := time.Since(start); took < termWait {
			t.Errorf("Stop took %s, want SIGKILL only once TermWait (%s) passed", took, termWait)
		}
		want := []string{"identify 4242", "args 4242", "SIGTERM 4242", "identify 4242", "args 4242", "SIGKILL 4242"}
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
		"runs another program's argv":        func(p *proc) { p.argv = []string{"/bin/sleep", "1000"} },
		"lacks the saver session's marks":    func(p *proc) { p.env = []string{"PATH=/usr/bin"} },
		"has its environment hidden from it": func(p *proc) { p.argsErr = proctable.ErrUnsupported },
	} {
		t.Run("a daemon that outlives TermWait and then "+name+" is sent no SIGKILL, and its session is left", func(t *testing.T) {
			b := withSaver(daemonPID)
			p := newProcs(map[int]*proc{daemonPID: daemon(func(p *proc, sig syscall.Signal) {
				if sig == syscall.SIGTERM {
					change(p)
				}
			})})

			err := newStopper(t, b, p).Stop(t.Context(), daemonPID)

			if err == nil || !strings.Contains(err.Error(), "sent SIGTERM") || !strings.Contains(err.Error(), "wasn't sent SIGKILL") {
				t.Errorf("Stop = %v, want a failure saying it was sent SIGTERM and not SIGKILL", err)
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
		cfg := config(b, p)
		cfg.Identify = func(pid int) (state.IdentifyResult, error) {
			// Gone between the wait's last read and the re-proof.
			if reads++; reads > 1 {
				p.table[daemonPID].exited = true
			}
			return p.identify(pid)
		}

		if err := stopperOf(t, cfg).Stop(t.Context(), daemonPID); err != nil {
			t.Fatalf("Stop: %v", err)
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
		"is another program": {func(p *proc) { p.verdict = state.IdentifyNotPortalDaemon }, saverhost.ErrNotSaverDaemon},
		"was reused for another program since its identity's read": {func(p *proc) { p.argv = []string{"/bin/sleep", "1000"} }, saverhost.ErrNotSaverDaemon},
		"lacks the session's name":                                 {func(p *proc) { p.env = []string{marks[1]} }, saverhost.ErrNotSaverDaemon},
		"lacks the backend's directory":                            {func(p *proc) { p.env = []string{marks[0]} }, saverhost.ErrNotSaverDaemon},
		"runs in another zmx directory":                            {func(p *proc) { p.env = []string{marks[0], "ZMX_DIR=/tmp/zx-elsewhere"} }, saverhost.ErrNotSaverDaemon},
		"names the directory twice, differently":                   {func(p *proc) { p.env = append(slices.Clone(marks), "ZMX_DIR=/tmp/zx-elsewhere") }, saverhost.ErrNotSaverDaemon},
		"can't be identified":                                      {func(p *proc) { p.identifyErr = errors.New("ps: malformed output") }, saverhost.ErrUnreadable},
		"has an environment that can't be read":                    {func(p *proc) { p.argsErr = errors.New("sysctl: EPERM") }, saverhost.ErrUnreadable},
		"has its environment unread off darwin":                    {func(p *proc) { p.argsErr = proctable.ErrUnsupported }, saverhost.ErrUnreadable},
		"is a portal daemon with no marks at all":                  {func(p *proc) { p.env = nil }, saverhost.ErrNotSaverDaemon},
	} {
		t.Run("a saver session's process that "+name+" is sent nothing, and its session is left", func(t *testing.T) {
			sink := logtest.Install(t)
			b := withSaver(daemonPID)
			d := daemon(exitsOn(syscall.SIGTERM, syscall.SIGKILL))
			tc.change(d)
			p := newProcs(map[int]*proc{daemonPID: d})

			err := newStopper(t, b, p).Stop(t.Context(), daemonPID)

			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), "nothing was sent") {
				t.Errorf("Stop = %v, want %v, saying nothing was sent", err, tc.want)
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

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); err != nil {
			t.Errorf("Stop: %v", err)
		}
		if got := p.signals(); len(got) != 0 {
			t.Errorf("signals = %q, want none", got)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a daemon found gone while still exiting is waited for before its session is killed", func(t *testing.T) {
		b := withSaver(daemonPID)
		d := exiting(3)
		p := newProcs(map[int]*proc{daemonPID: d})

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); err != nil {
			t.Fatalf("Stop: %v", err)
		}

		if d.lingers != 0 {
			t.Errorf("the daemon had %d reads of its exit left, want the stop to wait them out", d.lingers)
		}
		if got := p.signals(); len(got) != 0 {
			t.Errorf("signals = %q, want none", got)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a daemon found gone that doesn't finish exiting fails the stop, and its session is left", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: exiting(1 << 30)})

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); err == nil {
			t.Error("Stop = nil, want a failure")
		}
		if got := p.signals(); len(got) != 0 {
			t.Errorf("signals = %q, want none", got)
		}
		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want the session left", got)
		}
	})

	t.Run("a daemon gone as its SIGTERM is sent is stopped", func(t *testing.T) {
		b := withSaver(daemonPID)
		d := daemon(nil)
		p := newProcs(map[int]*proc{daemonPID: d})
		cfg := config(b, p)
		cfg.Signal = func(pid int, sig syscall.Signal) error {
			d.exited = true
			return p.signal(pid, sig)
		}

		if err := stopperOf(t, cfg).Stop(t.Context(), daemonPID); err != nil {
			t.Errorf("Stop: %v", err)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a daemon that outlives its SIGKILL fails the stop, and its session is left", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); err == nil {
			t.Error("Stop = nil, want a failure")
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
		cfg := config(b, p)
		cfg.Signal = func(int, syscall.Signal) error { return syscall.EPERM }

		if err := stopperOf(t, cfg).Stop(t.Context(), daemonPID); !errors.Is(err, syscall.EPERM) {
			t.Errorf("Stop = %v, want EPERM", err)
		}
		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want the session left", got)
		}
	})

	t.Run("a pid the saver session no longer runs is sent nothing", func(t *testing.T) {
		b := withSaver(daemonPID + 1)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM)), daemonPID + 1: daemon(exitsOn(syscall.SIGTERM))})

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); !errors.Is(err, saverhost.ErrReplaced) {
			t.Errorf("Stop = %v, want ErrReplaced", err)
		}
		if len(p.events) != 0 || len(kills(b)) != 0 {
			t.Errorf("events = %q, kills = %q; want none", p.events, kills(b))
		}
	})

	t.Run("this process is sent nothing", func(t *testing.T) {
		self := os.Getpid()
		b := withSaver(self)
		p := newProcs(map[int]*proc{self: daemon(exitsOn(syscall.SIGTERM))})

		if err := newStopper(t, b, p).Stop(t.Context(), self); err == nil {
			t.Error("Stop of this process = nil, want a refusal")
		}
		if len(p.events) != 0 || len(kills(b)) != 0 {
			t.Errorf("events = %q, kills = %q; want none", p.events, kills(b))
		}
	})

	t.Run("a saver session gone since its pid was read leaves the pid to its proof", func(t *testing.T) {
		b := backendtest.New()
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); err != nil {
			t.Fatalf("Stop: %v", err)
		}

		if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
			t.Errorf("signals = %q, want the proved daemon's SIGTERM", got)
		}
		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want none: there is no session", got)
		}
	})

	t.Run("a saver session that doesn't answer is sent nothing", func(t *testing.T) {
		b := withSaver(daemonPID)
		b.StopAnswering(saver.SessionName)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Stop = %v, want ErrTimeout", err)
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

		if err := newStopper(t, b, p).Stop(ctx, daemonPID); !errors.Is(err, context.Canceled) {
			t.Errorf("Stop = %v, want the context's cause", err)
		}
		if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
			t.Errorf("signals = %q, want the SIGTERM alone", got)
		}
	})

	t.Run("a kill that fails as zmx tears the session down succeeds", func(t *testing.T) {
		fake := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		if err := newStopper(t, &tornDown{fake}, p).Stop(t.Context(), daemonPID); err != nil {
			t.Errorf("Stop = %v, want success: the session is gone", err)
		}
	})

	t.Run("a kill that fails with the session still there fails the stop", func(t *testing.T) {
		b := withSaver(daemonPID)
		boom := errors.New("write: broken pipe")
		b.Fail(backendtest.OpKill, boom)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); !errors.Is(err, boom) {
			t.Errorf("Stop = %v, want %v", err, boom)
		}
	})

	t.Run("a saver made since its daemon exited is left", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(func(p *proc, sig syscall.Signal) {
			p.exited = true
			b.Add(backend.Session{Name: saver.SessionName, PID: daemonPID + 1})
		})})

		if err := newStopper(t, b, p).Stop(t.Context(), daemonPID); err != nil {
			t.Fatalf("Stop: %v", err)
		}

		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want the new saver left", got)
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

	t.Run("it reads the identity first, and the argv and environment only of a portal state daemon", func(t *testing.T) {
		other := daemon(nil)
		other.verdict = state.IdentifyNotPortalDaemon
		p := newProcs(map[int]*proc{daemonPID: other})

		_ = newStopper(t, backendtest.New(), p).Prove(daemonPID)

		if want := []string{"identify 4242"}; !slices.Equal(p.events, want) {
			t.Errorf("events = %q, want %q", p.events, want)
		}
	})

	t.Run("the environment's argv must name a daemon too, since the pid may be reused between the reads", func(t *testing.T) {
		for _, argv := range [][]string{
			{"/bin/sleep", "1000"},
			{"/bin/sh", "-c", "/opt/homebrew/bin/portal state daemon"},
			{"/opt/homebrew/bin/portal", "state", "hydrate"},
			nil,
		} {
			d := daemon(nil)
			d.argv = argv
			p := newProcs(map[int]*proc{daemonPID: d})

			if err := newStopper(t, backendtest.New(), p).Prove(daemonPID); !errors.Is(err, saverhost.ErrNotSaverDaemon) {
				t.Errorf("Prove of argv %q = %v, want ErrNotSaverDaemon", argv, err)
			}
		}
	})

	t.Run("a process gone at either read is ErrGone", func(t *testing.T) {
		if err := newStopper(t, backendtest.New(), newProcs(nil)).Prove(daemonPID); !errors.Is(err, saverhost.ErrGone) {
			t.Errorf("Prove of no process = %v, want ErrGone", err)
		}
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		cfg := config(backendtest.New(), p)
		cfg.Args = func(pid int) (proctable.Args, error) {
			p.table[pid].exited = true
			return p.args(pid)
		}
		if err := stopperOf(t, cfg).Prove(daemonPID); !errors.Is(err, saverhost.ErrGone) {
			t.Errorf("Prove of a process gone before its arguments' read = %v, want ErrGone", err)
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
		"no backend":                    {Marks: marks, Signal: signal},
		"no signal":                     {Backend: backendtest.New(), Marks: marks},
		"no marks":                      {Backend: backendtest.New(), Signal: signal},
		"marks naming no saver session": {Backend: backendtest.New(), Marks: []string{marks[1]}, Signal: signal},
		"marks naming the saver session alone, which every zmx directory's carries": {Backend: backendtest.New(), Marks: []string{marks[0]}, Signal: signal},
		"an empty directory":        {Backend: backendtest.New(), Marks: []string{marks[0], "ZMX_DIR="}, Signal: signal},
		"a mark naming no variable": {Backend: backendtest.New(), Marks: []string{marks[0], "=x"}, Signal: signal},
		"a mark that is no entry":   {Backend: backendtest.New(), Marks: []string{marks[0], "ZMX_DIR"}, Signal: signal},
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if s, err := saverhost.NewStopper(cfg); err == nil {
				t.Errorf("NewStopper = %v, want a refusal", s)
			}
		})
	}
}
