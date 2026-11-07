package saverhost_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
)

// otherState is the state directory a saver not the Host's saves.
const otherState = "/srv/portal/state"

// firstMade is the process of the first session a new Fake makes.
const firstMade = 1001

// A saver is one state directory's: its identity is its zmx directory and
// its state directory, so the state directory is a third mark.
func TestProve_StateDirIsAThirdMark(t *testing.T) {
	t.Run("a daemon of another state directory is proved that state directory's saver, no saver of the Host's", func(t *testing.T) {
		dir := t.TempDir()
		p := newProcs(map[int]*proc{daemonPID: theirs(otherState, nil)})

		err := hostOf(t, config(backendtest.New(), p, dir)).Prove(daemonPID)

		other, ok := errors.AsType[*saverhost.OtherStateDirError](err)
		if !ok || !errors.Is(err, saverhost.ErrNotSaverDaemon) {
			t.Fatalf("Prove = %v, want an *OtherStateDirError, which is ErrNotSaverDaemon", err)
		}
		if want := (saverhost.OtherStateDirError{StateDir: dir, Other: otherState, PID: daemonPID}); *other != want {
			t.Errorf("Prove = %+v, want %+v", *other, want)
		}
	})

	for name, env := range map[string][]string{
		"no state directory":                      nil,
		"an empty state directory":                {"PORTAL_STATE_DIR="},
		"a relative state directory":              {"PORTAL_STATE_DIR=state"},
		"two state directories":                   {"PORTAL_STATE_DIR=" + otherState, "PORTAL_STATE_DIR=/srv/third/state"},
		"another state directory, then its own":   {"PORTAL_STATE_DIR=" + otherState, "PORTAL_STATE_DIR=<own>"},
		"its own state directory, then another's": {"PORTAL_STATE_DIR=<own>", "PORTAL_STATE_DIR=" + otherState},
	} {
		t.Run("a daemon naming "+name+" is no saver of the Host's, nor proved another's", func(t *testing.T) {
			dir := t.TempDir()
			d := daemon(nil)
			d.saves = false
			for _, entry := range env {
				if entry == "PORTAL_STATE_DIR=<own>" {
					entry = saverhost.StateDirMark(dir)
				}
				d.env = append(d.env, entry)
			}
			p := newProcs(map[int]*proc{daemonPID: d})

			err := hostOf(t, config(backendtest.New(), p, dir)).Prove(daemonPID)

			if _, theirs := errors.AsType[*saverhost.OtherStateDirError](err); theirs || !errors.Is(err, saverhost.ErrNotSaverDaemon) {
				t.Errorf("Prove with %q = %v, want ErrNotSaverDaemon, and no state directory proved", env, err)
			}
		})
	}

	t.Run("the state directory is compared as the zmx-dir claim compares directories", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(dir, link); err != nil {
			t.Fatal(err)
		}
		d := daemon(nil)
		d.saves = false
		d.env = append(d.env, saverhost.StateDirMark(link+"/"))
		p := newProcs(map[int]*proc{daemonPID: d})
		cfg := config(backendtest.New(), p, dir)
		cfg.Canonical = map[string]func(string) string{"PORTAL_STATE_DIR": zmx.CanonicalDir}

		if err := hostOf(t, cfg).Prove(daemonPID); err != nil {
			t.Errorf("Prove of a daemon naming %s = %v, want proved", link, err)
		}
	})
}

func TestCheckStateDir(t *testing.T) {
	for name, tc := range map[string]struct {
		b     *backendtest.Fake
		procs map[int]*proc
	}{
		"no saver":                                 {backendtest.New(), nil},
		"a saver of this state directory":          {withSaver(daemonPID), map[int]*proc{daemonPID: daemon(nil)}},
		"a saver session running no daemon":        {withSaver(daemonPID), map[int]*proc{daemonPID: {verdict: state.IdentifyNotPortalDaemon}}},
		"a saver whose process can't be read":      {withSaver(daemonPID), map[int]*proc{daemonPID: {identifyErr: errors.New("ps failed")}}},
		"a saver whose state directory is unnamed": {withSaver(daemonPID), map[int]*proc{daemonPID: {verdict: state.IdentifyIsPortalDaemon, argv: daemonArgv, env: marks}}},
	} {
		t.Run(name+" passes", func(t *testing.T) {
			if err := newHost(t, tc.b, newProcs(tc.procs)).CheckStateDir(t.Context()); err != nil {
				t.Errorf("CheckStateDir = %v, want nil", err)
			}
		})
	}

	t.Run("a saver of another state directory fails, naming it", func(t *testing.T) {
		p := newProcs(map[int]*proc{daemonPID: theirs(otherState, nil)})

		err := newHost(t, withSaver(daemonPID), p).CheckStateDir(t.Context())

		if other, ok := errors.AsType[*saverhost.OtherStateDirError](err); !ok || other.Other != otherState || other.PID != daemonPID {
			t.Errorf("CheckStateDir = %v, want the saver of %s, pid %d", err, otherState, daemonPID)
		}
		if signals := p.signals(); len(signals) != 0 {
			t.Errorf("signals = %q, want none", signals)
		}
	})
}

// Uninstall's stop, and the recycle's: another state directory's saver is
// sent nothing, and its session left.
func TestStop_RefusesAnotherStateDirsSaver(t *testing.T) {
	sink := logtest.Install(t)
	b := withSaver(daemonPID)
	p := newProcs(map[int]*proc{daemonPID: theirs(otherState, exitsOn(syscall.SIGTERM, syscall.SIGKILL))})

	stopped, err := newHost(t, b, p).Stop(t.Context())

	if _, ok := errors.AsType[*saverhost.OtherStateDirError](err); !ok || !stopped {
		t.Errorf("Stop = %v, %v; want true and an *OtherStateDirError", stopped, err)
	}
	if signals := p.signals(); len(signals) != 0 {
		t.Errorf("signals = %q, want none", signals)
	}
	if got := kills(b); len(got) != 0 {
		t.Errorf("kills = %q, want the session left", got)
	}
	assertRefused(t, sink, daemonPID, otherState)
}

func TestStopOr_NeverHandsOnAnotherStateDirsSaver(t *testing.T) {
	p := newProcs(map[int]*proc{daemonPID: theirs(otherState, exitsOn(syscall.SIGTERM))})

	_, err := newHost(t, withSaver(daemonPID), p).StopOr(t.Context(), func(context.Context, error) (bool, error) {
		t.Error("orElse was handed another state directory's saver, which it would end")
		return true, nil
	})

	if _, ok := errors.AsType[*saverhost.OtherStateDirError](err); !ok {
		t.Errorf("StopOr = %v, want the *OtherStateDirError", err)
	}
	if signals := p.signals(); len(signals) != 0 {
		t.Errorf("signals = %q, want none", signals)
	}
}

func TestEnsureSaver_StateDir(t *testing.T) {
	t.Run("it creates the saver with the state directory's mark in its environment", func(t *testing.T) {
		dir := t.TempDir()
		b := &startingDaemon{Fake: backendtest.New(), dir: dir, version: release}

		if err := newEnsurer(t, b, newProcs(nil), dir, release).EnsureSaver(t.Context(), nil); err != nil {
			t.Fatalf("EnsureSaver = %v", err)
		}

		creates := b.CallsTo(backendtest.OpCreate)
		if len(creates) != 1 || !slices.Equal(creates[0].Env, []string{saverhost.StateDirMark(dir)}) {
			t.Errorf("creates = %+v, want one carrying %s", creates, saverhost.StateDirMark(dir))
		}
	})

	for name, recorded := range map[string]bool{
		"daemon.pid records none of":                  false,
		"daemon.pid names, a stale pid reused by it,": true,
	} {
		t.Run("a saver of another state directory that "+name+" is refused at once, sent nothing and nothing made", func(t *testing.T) {
			dir := t.TempDir()
			if recorded {
				writeDaemon(t, dir, daemonPID, release)
			}
			b := withSaver(daemonPID)
			p := newProcs(map[int]*proc{daemonPID: theirs(otherState, exitsOn(syscall.SIGTERM, syscall.SIGKILL))})
			cfg := config(b, p, dir)
			cfg.ReadyWait = lockWait
			start := time.Now()

			err := ensurerOf(t, hostOf(t, cfg), release).EnsureSaver(t.Context(), func(context.Context) error {
				t.Error("a saver it didn't make was judged as made")
				return nil
			})

			if other, ok := errors.AsType[*saverhost.OtherStateDirError](err); !ok || other.Other != otherState {
				t.Fatalf("EnsureSaver = %v, want the saver of %s refused", err, otherState)
			}
			if took := time.Since(start); took >= cfg.ReadyWait {
				t.Errorf("EnsureSaver took %s, want no wait for a saver that never records itself here", took)
			}
			if signals := p.signals(); len(signals) != 0 {
				t.Errorf("signals = %q, want none", signals)
			}
			if creates, killed := b.CallsTo(backendtest.OpCreate), kills(b); len(creates) != 0 || len(killed) != 0 {
				t.Errorf("creates = %+v, kills = %q; want neither", creates, killed)
			}
		})
	}

	// Another state directory's Portal wins the create, and the create's own
	// check reads its session's process before the daemon has exec'd: the
	// wait for this state's daemon judges whose the saver is again, and
	// refuses it once its daemon has exec'd, never waiting out ReadyWait to
	// restore beside it.
	t.Run("a saver another state directory's Portal makes first is refused once its daemon has exec'd", func(t *testing.T) {
		b := &beatenByAnother{Fake: backendtest.New()}
		p := newProcs(map[int]*proc{firstMade: execing(theirs(otherState, nil), 2)})
		cfg := config(b, p, t.TempDir())
		cfg.ReadyWait = lockWait
		start := time.Now()

		err := ensurerOf(t, hostOf(t, cfg), release).EnsureSaver(t.Context(), nil)

		if other, ok := errors.AsType[*saverhost.OtherStateDirError](err); !ok || other.Other != otherState {
			t.Fatalf("EnsureSaver = %v, want the saver of %s refused", err, otherState)
		}
		if took := time.Since(start); took >= cfg.ReadyWait {
			t.Errorf("EnsureSaver took %s, want the refusal before ReadyWait, %s", took, cfg.ReadyWait)
		}
		if signals := p.signals(); len(signals) != 0 {
			t.Errorf("signals = %q, want none", signals)
		}
	})

	t.Run("a saver another process makes first, of another state directory, is refused, never judged as made", func(t *testing.T) {
		b := &beatenToIt{Fake: backendtest.New(), dir: t.TempDir(), version: release}
		p := newProcs(map[int]*proc{firstMade: theirs(otherState, nil)})

		err := newEnsurer(t, b, p, t.TempDir(), release).EnsureSaver(t.Context(), func(context.Context) error {
			t.Error("another state directory's saver was judged as made")
			return nil
		})

		if _, ok := errors.AsType[*saverhost.OtherStateDirError](err); !ok {
			t.Errorf("EnsureSaver = %v, want the saver of %s refused", err, otherState)
		}
	})
}

// beatenByAnother is the Fake whose saver session another state directory's
// Portal creates first, so the create finds the name held.
type beatenByAnother struct{ *backendtest.Fake }

func (b *beatenByAnother) Create(ctx context.Context, name, dir string, command []string, env ...string) (backend.Session, error) {
	if _, err := b.Fake.Create(ctx, name, dir, []string{"/usr/local/bin/portal", "state", "daemon"}, saverhost.StateDirMark(otherState)); err != nil {
		return backend.Session{}, err
	}
	return b.Fake.Create(ctx, name, dir, command, env...)
}

// Every wait on the saver proves the process its session runs once, and again
// only when that process is another or not yet proved a saver's daemon either
// way: a saver that stays unrecorded is no ps a poll.
func TestEnsureSaver_ProvesASteadySaverOnce(t *testing.T) {
	t.Run("while it waits for a saver found still starting", func(t *testing.T) {
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		cfg := config(withSaver(daemonPID), p, t.TempDir())
		cfg.ReadyWait = lockWait
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()

		if err := ensurerOf(t, hostOf(t, cfg), release).EnsureSaver(ctx, nil); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("EnsureSaver = %v, want the wait cut short by ctx", err)
		}
		if n := p.identified(daemonPID); n != 1 {
			t.Errorf("pid %d identified %d times over %d polls, want once", daemonPID, n, 100*time.Millisecond/poll)
		}
	})

	t.Run("a process proved neither way is proved again every RejudgeEvery readings, no oftener", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: {identifyErr: errors.New("ps failed")}})
		cfg := config(b, p, t.TempDir())
		cfg.ReadyWait = lockWait
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()

		if err := ensurerOf(t, hostOf(t, cfg), release).EnsureSaver(ctx, nil); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("EnsureSaver = %v, want the wait cut short by ctx", err)
		}
		readings := len(b.CallsTo(backendtest.OpSession))
		if n, want := p.identified(daemonPID), 1+(readings-1)/saverhost.RejudgeEvery; n != want {
			t.Errorf("pid %d identified %d times over %d readings, want %d", daemonPID, n, readings, want)
		}
	})

	t.Run("while it waits for the saver it made", func(t *testing.T) {
		p := newProcs(map[int]*proc{firstMade: daemon(nil)})

		if err := newEnsurer(t, backendtest.New(), p, t.TempDir(), release).EnsureSaver(t.Context(), nil); !errors.Is(err, saverhost.ErrNotReady) {
			t.Fatalf("EnsureSaver = %v, want ErrNotReady from a daemon that never records itself", err)
		}
		if n := p.identified(firstMade); n != 1 {
			t.Errorf("pid %d identified %d times over %d polls, want once", firstMade, n, readyWait/poll)
		}
	})
}

func TestRecorded(t *testing.T) {
	t.Run("no saver session is none, with nothing more read", func(t *testing.T) {
		if present, err := newHost(t, backendtest.New(), newProcs(nil)).Recorded(t.Context()); present || err != nil {
			t.Errorf("Recorded = %v, %v; want false, nil", present, err)
		}
	})

	t.Run("a saver whose process daemon.pid names is there, read without the process table", func(t *testing.T) {
		dir := t.TempDir()
		writeDaemon(t, dir, daemonPID, release)
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})

		present, err := hostOf(t, config(withSaver(daemonPID), p, dir)).Recorded(t.Context())

		if !present || err != nil {
			t.Errorf("Recorded = %v, %v; want true, nil", present, err)
		}
		if len(p.events) != 0 {
			t.Errorf("events = %q, want no process read", p.events)
		}
	})

	for name, stage := range map[string]func(t *testing.T, dir string){
		"daemon.pid names another process": func(t *testing.T, dir string) { writeDaemon(t, dir, daemonPID+1, release) },
		"no daemon.pid":                    func(*testing.T, string) {},
		"a daemon.pid that can't be read": func(t *testing.T, dir string) {
			if err := os.WriteFile(state.DaemonPID(dir), []byte("not a pid\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run("a saver session with "+name+" is unrecorded", func(t *testing.T) {
			dir := t.TempDir()
			stage(t, dir)

			if _, err := hostOf(t, config(withSaver(daemonPID), newProcs(nil), dir)).Recorded(t.Context()); !errors.Is(err, saverhost.ErrUnrecorded) {
				t.Errorf("Recorded = %v, want ErrUnrecorded", err)
			}
		})
	}

	t.Run("a saver session that doesn't answer is its error", func(t *testing.T) {
		b := withSaver(daemonPID)
		b.StopAnswering(saver.SessionName)

		if _, err := newHost(t, b, newProcs(nil)).Recorded(t.Context()); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Recorded = %v, want ErrTimeout", err)
		}
	})
}
