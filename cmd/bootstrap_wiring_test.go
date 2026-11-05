package cmd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"syscall"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
)

// zmxFake is the zmx driver's reach to the saver's daemon, answering as its
// fields say and recording what it was asked.
type zmxFake struct {
	version    int
	versionErr error
	older      bool
	olderErr   error
	killErr    error
	endErr     error

	// signals has EndDaemon send pid 4242 SIGTERM through the signaller it is
	// given, as the driver does the daemon it proves.
	signals bool

	olderAsks, kills, ends int
	names                  []string
	// dirs is what each EndDaemon was asked to prove the daemon by, beside
	// the session's own marks.
	dirs [][]string
}

func (f *zmxFake) KillByCLI(_ context.Context, name string) error {
	f.names = append(f.names, name)
	f.kills++
	return f.killErr
}

func (f *zmxFake) ProtocolVersion(_ context.Context, name string) (int, error) {
	f.names = append(f.names, name)
	return f.version, f.versionErr
}

func (f *zmxFake) OlderZmx(_ context.Context, name string) (bool, error) {
	f.names = append(f.names, name)
	f.olderAsks++
	return f.older, f.olderErr
}

func (f *zmxFake) EndDaemon(_ context.Context, name string, signal func(pid int, sig syscall.Signal) error, dirs ...string) error {
	f.names = append(f.names, name)
	f.dirs = append(f.dirs, dirs)
	f.ends++
	if f.signals {
		if err := signal(4242, syscall.SIGTERM); err != nil {
			return err
		}
	}
	return f.endErr
}

// daemonState is the state directory daemonOver's saver saves.
const daemonState = "/srv/portal/state"

func daemonOver(f *zmxFake) saverDaemon {
	return saverDaemon{stateDir: daemonState, driver: func() (saverZmx, error) { return f, nil }}
}

var errBoom = errors.New("boom")

// errNoAnswer is a probe of a daemon that took the request and answered nothing.
var errNoAnswer = fmt.Errorf("read protocol version of session %q: no answer in 1s: %w", saver.SessionName, backend.ErrTimeout)

func TestSaverDaemon_Protocol(t *testing.T) {
	for _, tc := range []struct {
		name string
		fake zmxFake
		// want is the version, wantErr what the error must wrap, nil for none.
		want      int
		wantErr   error
		notErr    error
		olderAsks int
	}{
		{name: "a daemon of Portal's protocol answers it, unasked in the older framing", fake: zmxFake{version: zmx.ProtocolVersion}, want: zmx.ProtocolVersion},
		{name: "a daemon of another version names it", fake: zmxFake{version: 2}, want: 2},
		{name: "a failure other than no answer is its own, unasked in the older framing", fake: zmxFake{versionErr: errBoom}, wantErr: errBoom, notErr: bootstrap.ErrOlderZmx},
		{name: "a saver session gone is ErrSaverGone", fake: zmxFake{versionErr: backend.ErrNoSuchSession, older: true}, wantErr: bootstrap.ErrSaverGone},
		{
			name:    "a socket that refuses is no answer to judge by, unasked in the older framing",
			fake:    zmxFake{versionErr: fmt.Errorf("%w: %w", zmx.ErrRefusing, backend.ErrTimeout), older: true},
			wantErr: zmx.ErrRefusing, notErr: bootstrap.ErrOlderZmx,
		},
		{name: "no answer, then 0.8.1's answer, is ErrOlderZmx", fake: zmxFake{versionErr: errNoAnswer, older: true}, wantErr: bootstrap.ErrOlderZmx, olderAsks: 1},
		{name: "no answer in either framing is the timeout", fake: zmxFake{versionErr: errNoAnswer}, wantErr: backend.ErrTimeout, notErr: bootstrap.ErrOlderZmx, olderAsks: 1},
		{name: "no answer, then a failed ask in 0.8.1's framing, is the timeout", fake: zmxFake{versionErr: errNoAnswer, olderErr: errBoom}, wantErr: backend.ErrTimeout, notErr: bootstrap.ErrOlderZmx, olderAsks: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := tc.fake

			got, err := daemonOver(&fake).Protocol(t.Context())

			if got != tc.want || !errors.Is(err, tc.wantErr) || tc.wantErr == nil && err != nil {
				t.Errorf("Protocol = %d, %v; want %d, %v", got, err, tc.want, tc.wantErr)
			}
			if tc.notErr != nil && errors.Is(err, tc.notErr) {
				t.Errorf("Protocol = %v, want it not %v", err, tc.notErr)
			}
			if fake.olderAsks != tc.olderAsks {
				t.Errorf("asked in 0.8.1's framing %d times, want %d", fake.olderAsks, tc.olderAsks)
			}
			for _, name := range fake.names {
				if name != saver.SessionName {
					t.Errorf("asked about session %q, want %q alone", name, saver.SessionName)
				}
			}
		})
	}

	t.Run("a driver it can't bind fails it", func(t *testing.T) {
		d := saverDaemon{driver: func() (saverZmx, error) { return nil, errBoom }}

		if _, err := d.Protocol(t.Context()); !errors.Is(err, errBoom) {
			t.Errorf("Protocol = %v, want the bind's failure", err)
		}
	})
}

func TestSaverDaemon_End(t *testing.T) {
	t.Run("it ends the saver session's daemon, signalling through signalProcess", func(t *testing.T) {
		fake := &zmxFake{signals: true}
		var sent []string
		withFuncSeam(t, &signalProcess, func(pid int, sig syscall.Signal) error {
			sent = append(sent, fmt.Sprintf("%d %v", pid, sig))
			return nil
		})

		if err := daemonOver(fake).End(t.Context()); err != nil {
			t.Fatalf("End = %v", err)
		}
		if len(fake.names) != 1 || fake.names[0] != saver.SessionName {
			t.Errorf("ended %v, want %s alone", fake.names, saver.SessionName)
		}
		if want := fmt.Sprintf("4242 %v", syscall.SIGTERM); len(sent) != 1 || sent[0] != want {
			t.Errorf("signals = %v, want %q through signalProcess", sent, want)
		}
		assertProvedByStateDir(t, fake)
	})

	t.Run("an end that fails fails it", func(t *testing.T) {
		if err := daemonOver(&zmxFake{endErr: errBoom}).End(t.Context()); !errors.Is(err, errBoom) {
			t.Errorf("End = %v, want the end's failure", err)
		}
	})

	t.Run("it never runs zmx kill: the zmx on PATH may speak another protocol than the saver found", func(t *testing.T) {
		fake := &zmxFake{}

		if err := daemonOver(fake).End(t.Context()); err != nil || fake.kills != 0 {
			t.Errorf("End = %v after %d zmx kills, want nil after none", err, fake.kills)
		}
	})
}

func TestSaverDaemon_EndMade(t *testing.T) {
	t.Run("it ends the saver made with zmx kill, signalling nothing", func(t *testing.T) {
		fake := &zmxFake{signals: true}
		withFuncSeam(t, &signalProcess, func(pid int, sig syscall.Signal) error {
			t.Errorf("signalled %d %v, want zmx kill alone", pid, sig)
			return nil
		})

		if err := daemonOver(fake).EndMade(t.Context()); err != nil {
			t.Fatalf("EndMade = %v", err)
		}
		if fake.kills != 1 || fake.ends != 0 {
			t.Errorf("zmx kills %d, proved ends %d; want one kill and no end", fake.kills, fake.ends)
		}
		if len(fake.names) != 1 || fake.names[0] != saver.SessionName {
			t.Errorf("killed %v, want %s alone", fake.names, saver.SessionName)
		}
	})

	t.Run("a zmx kill that fails is followed by End's proved signal", func(t *testing.T) {
		fake := &zmxFake{killErr: errBoom, signals: true}
		var sent []string
		withFuncSeam(t, &signalProcess, func(pid int, sig syscall.Signal) error {
			sent = append(sent, fmt.Sprintf("%d %v", pid, sig))
			return nil
		})

		if err := daemonOver(fake).EndMade(t.Context()); err != nil {
			t.Fatalf("EndMade = %v, want the signal's end", err)
		}
		if want := fmt.Sprintf("4242 %v", syscall.SIGTERM); len(sent) != 1 || sent[0] != want {
			t.Errorf("signals = %v, want %q through signalProcess", sent, want)
		}
		assertProvedByStateDir(t, fake)
	})

	t.Run("both failing fails it with both", func(t *testing.T) {
		unproved := errors.New("no one process is proved the session's zmx daemon")

		err := daemonOver(&zmxFake{killErr: errBoom, endErr: unproved}).EndMade(t.Context())

		if !errors.Is(err, errBoom) || !errors.Is(err, unproved) {
			t.Errorf("EndMade = %v, want the kill's and the end's failures", err)
		}
	})

	t.Run("a driver it can't bind fails it", func(t *testing.T) {
		d := saverDaemon{driver: func() (saverZmx, error) { return nil, errBoom }}

		if err := d.EndMade(t.Context()); !errors.Is(err, errBoom) {
			t.Errorf("EndMade = %v, want the bind's failure", err)
		}
	})
}

// assertProvedByStateDir asserts fake's one EndDaemon proved the daemon by
// the mark of the state directory its saver saves, so another state
// directory's saver is never signalled.
func assertProvedByStateDir(t *testing.T, fake *zmxFake) {
	t.Helper()
	if want := []string{saverhost.StateDirMark(daemonState)}; len(fake.dirs) != 1 || !slices.Equal(fake.dirs[0], want) {
		t.Errorf("EndDaemon proved by %q, want %q once", fake.dirs, want)
	}
}

func TestSaverDaemon_Unusable(t *testing.T) {
	for name, tc := range map[string]struct {
		fake zmxFake
		want bool
	}{
		"a daemon of zmx 0.8.1":           {zmxFake{versionErr: errNoAnswer, older: true}, true},
		"a daemon of another protocol":    {zmxFake{version: zmx.ProtocolVersion + 1}, true},
		"a daemon of Portal's protocol":   {zmxFake{version: zmx.ProtocolVersion}, false},
		"a daemon that answers neither":   {zmxFake{versionErr: errNoAnswer}, false},
		"a saver session gone":            {zmxFake{versionErr: backend.ErrNoSuchSession}, false},
		"a daemon that can't be read yet": {zmxFake{versionErr: errBoom}, false},
	} {
		t.Run(name, func(t *testing.T) {
			fake := tc.fake
			if got := daemonOver(&fake).unusable(t.Context()); got != tc.want {
				t.Errorf("unusable = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLatchFile(t *testing.T) {
	latch := state.Latch{BootID: "4DAA6ADD-0334-49EC-9FC6-8323D691249E", Version: "0.13.0"}

	t.Run("it writes, and removes, the latch in its directory", func(t *testing.T) {
		f := latchFile{dir: t.TempDir(), latch: latch}

		if err := f.WriteLatch(); err != nil || !state.LatchCurrent(f.dir, latch) {
			t.Fatalf("WriteLatch = %v, current %v; want it written", err, state.LatchCurrent(f.dir, latch))
		}
		if err := f.RemoveLatch(); err != nil {
			t.Fatalf("RemoveLatch = %v", err)
		}
		if _, err := state.ReadLatch(f.dir); err == nil {
			t.Error("ReadLatch found a latch after RemoveLatch")
		}
	})

	t.Run("without this boot's id it writes none, failing with ErrNoBootID", func(t *testing.T) {
		f := latchFile{dir: t.TempDir(), latch: latch, bootErr: errBoom}

		if err := f.WriteLatch(); !errors.Is(err, bootstrap.ErrNoBootID) || !errors.Is(err, errBoom) {
			t.Errorf("WriteLatch = %v, want ErrNoBootID wrapping the read's failure", err)
		}
		if _, err := state.ReadLatch(f.dir); err == nil {
			t.Error("a latch was written")
		}
	})
}

func TestRestoringFlag_LowerReportsWhetherItWasRaised(t *testing.T) {
	flag := restoringFlag(t.TempDir())
	if err := flag.Raise(); err != nil {
		t.Fatal(err)
	}

	for _, want := range []bool{true, false} {
		if raised, err := flag.Lower(); raised != want || err != nil {
			t.Errorf("Lower = %v, %v; want %v, nil", raised, err, want)
		}
	}
}
