package cmd

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/tui"
)

const takenSession = "api-x7Kd9a"

// takeAt is where the tests' takes say a session went, from the environment
// they pin.
const takeAt = "SSH from 192.0.2.20"

// aTerminal is a zmx attach on a terminal, as the process table shows one.
var aTerminal = []backend.ClientProcess{{PID: 4242, Env: []string{"ZMX_DIR=/tmp/zx-open-test"}}}

// aLeader is the tracked environment zmx answers for a client leading a
// session: a variable it tracks that the client's environment lacks, as a
// raw zmx attach on zmx's default list answers.
var aLeader = backend.ClientEnv{{Name: "SSH_CONNECTION", Unset: true}}

// terminalOn is a Fake holding name with a terminal attached: the process
// table shows its zmx attach, and the session counts it among its clients.
// A detach leaves the process listed a moment, as zmx attach takes a moment
// to exit, while the session counts it no more.
func terminalOn(name string) *backendtest.Fake {
	f := backendtest.New(backend.Session{Name: name, Clients: 1})
	f.SetAttachClients(name, aTerminal)
	return f
}

// reconnecting is a Fake whose terminal connects again after each of its
// first detaches, as one still on its way to the session when a take
// detached its clients does.
type reconnecting struct {
	*backendtest.Fake
	// again counts the detaches the terminal comes back after; forever, it
	// always comes back.
	again   int
	forever bool
}

func (b *reconnecting) Detach(ctx context.Context, name string) error {
	if err := b.Fake.Detach(ctx, name); err != nil {
		return err
	}
	if b.forever || b.again > 0 {
		b.again--
		b.Add(backend.Session{Name: name, Clients: 1})
	}
	return nil
}

// takingConnector is a connector whose take reads b and asks runners, waiting
// up to wait for the session to be alone.
func takingConnector(b backend.Backend, runners *runnersFake, wait time.Duration) attachConnector {
	return attachConnector{cmd: contextCommand(), backend: b, runners: runners, takeWait: wait}
}

// pinWhere pins the environment a take reads where the session went from.
func pinWhere(t *testing.T) {
	t.Helper()
	t.Setenv("SSH_CONNECTION", "192.0.2.20 52144 192.0.2.5 22")
}

func detaches(f *backendtest.Fake) int { return len(f.CallsTo(backendtest.OpDetach)) }

func TestTake(t *testing.T) {
	t.Run("a runner on the session lets it go, and nothing is detached", func(t *testing.T) {
		pinWhere(t)
		fake := terminalOn(takenSession)
		runners := &runnersFake{onTake: func(session string) (int, error) {
			fake.Add(backend.Session{Name: session})
			return 1, nil
		}}

		takingConnector(fake, runners, takeWait).take(context.Background(), takenSession)

		if want := []takeCall{{takenSession, takeAt}}; !slices.Equal(runners.takes, want) {
			t.Errorf("takes = %v, want %v", runners.takes, want)
		}
		if n := detaches(fake); n != 0 || len(runners.records) != 0 {
			t.Errorf("detached %d times, recorded %v; want nothing more done", n, runners.records)
		}
	})

	for _, tc := range []struct {
		name   string
		onTake func(string) (int, error)
	}{
		{"a runner refusing leaves the session to the backend's detach, where it went recorded first", func(string) (int, error) {
			return 0, errors.New("no runner on api-x7Kd9a let it go")
		}},
		{"a raw zmx client, no runner to ask, is detached by the backend, where it went recorded first", nil},
		{"a runner that lets go while its client lingers is detached by the backend too", func(string) (int, error) { return 1, nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pinWhere(t)
			fake := terminalOn(takenSession)
			detachedAtRecord := -1
			runners := &runnersFake{onTake: tc.onTake, onRecord: func() { detachedAtRecord = detaches(fake) }}

			takingConnector(fake, runners, takeWait).take(context.Background(), takenSession)

			if want := []takeCall{{takenSession, takeAt}}; !slices.Equal(runners.records, want) {
				t.Errorf("recorded %v, want %v", runners.records, want)
			}
			if n := detaches(fake); n != 1 || detachedAtRecord != 0 {
				t.Errorf("detached %d times, %d by the record; want once, after it", n, detachedAtRecord)
			}
		})
	}

	for _, tc := range []struct {
		name string
		fake func() *backendtest.Fake
	}{
		{"a session with nothing attached", func() *backendtest.Fake { return backendtest.New(backend.Session{Name: takenSession}) }},
		{"a session only readers are connected to", func() *backendtest.Fake {
			return backendtest.New(backend.Session{Name: takenSession, Clients: 2})
		}},
		{"a session a zmx attach zmx switched elsewhere still names", func() *backendtest.Fake {
			f := backendtest.New(backend.Session{Name: takenSession})
			f.SetAttachClients(takenSession, aTerminal)
			return f
		}},
	} {
		t.Run(tc.name+" is neither recorded nor detached", func(t *testing.T) {
			fake := tc.fake()
			runners := &runnersFake{}

			takingConnector(fake, runners, takeWait).take(context.Background(), takenSession)

			if n := detaches(fake); n != 0 || len(runners.records) != 0 {
				t.Errorf("detached %d times, recorded %v; want nothing done", n, runners.records)
			}
		})
	}

	t.Run("a client zmx switched onto the session holds the one it leads, and not the one its process still names", func(t *testing.T) {
		pinWhere(t)
		const left = "blog-c3d4"
		fake := backendtest.New(backend.Session{Name: left}, backend.Session{Name: takenSession, Clients: 1})
		fake.SetAttachClients(left, aTerminal)
		fake.SetLeaderEnv(takenSession, aLeader)
		runners := &runnersFake{}
		c := takingConnector(fake, runners, takeWait)

		c.take(context.Background(), left)
		if n := detaches(fake); n != 0 || len(runners.records) != 0 {
			t.Errorf("taking %s, the session it left: detached %d times, recorded %v; want nothing done", left, n, runners.records)
		}

		c.take(context.Background(), takenSession)
		if want := []takeCall{{takenSession, takeAt}}; !slices.Equal(runners.records, want) || detaches(fake) != 1 {
			t.Errorf("taking %s, which it leads: recorded %v, detached %d times; want %v and one detach", takenSession, runners.records, detaches(fake), want)
		}
	})

	t.Run("a leader it can't read leaves the judgement to the process table", func(t *testing.T) {
		fake := terminalOn(takenSession)
		fake.Fail(backendtest.OpLeaderEnv, backend.ErrTimeout)

		takingConnector(fake, &runnersFake{}, takeWait).take(context.Background(), takenSession)

		if n := detaches(fake); n != 1 {
			t.Errorf("detached %d times, want once for the terminal the table shows", n)
		}
	})

	t.Run("a leader it can't read, and no terminal the table shows, detaches nothing, saying so", func(t *testing.T) {
		sink := logtest.Install(t)
		fake := backendtest.New(backend.Session{Name: takenSession, Clients: 1})
		fake.Fail(backendtest.OpLeaderEnv, backend.ErrTimeout)

		takingConnector(fake, &runnersFake{}, takeWait).take(context.Background(), takenSession)

		if n := detaches(fake); n != 0 {
			t.Errorf("detached %d times, want nothing done on what it can't tell", n)
		}
		rec := sink.Records().Matching("attach", "take failed").AtExactLevel(slog.LevelWarn).Only(t, "the read's warning")
		if detail := rec.AttrString(t, "detail"); detail != "read" {
			t.Errorf("detail = %q, want read", detail)
		}
	})

	t.Run("Portal's own session is never taken", func(t *testing.T) {
		fake := terminalOn("_portal-saver")
		runners := &runnersFake{}

		takingConnector(fake, runners, takeWait).take(context.Background(), "_portal-saver")

		if len(runners.takes) != 0 || len(runners.locked) != 0 || len(fake.Calls()) != 0 {
			t.Errorf("takes %v, locked %v, backend calls %v; want the session untouched", runners.takes, runners.locked, fake.Calls())
		}
	})

	t.Run("a terminal that connects after the detach is detached again before the wait ends", func(t *testing.T) {
		b := &reconnecting{Fake: terminalOn(takenSession), again: 1}

		takingConnector(b, &runnersFake{}, takeWait).take(context.Background(), takenSession)

		if n := detaches(b.Fake); n != 2 {
			t.Errorf("detached %d times, want again for the terminal that came back", n)
		}
		if s, _ := b.Session(context.Background(), takenSession); s.Attached() {
			t.Error("the take returned with a terminal still on the session")
		}
	})

	t.Run("past its bound it gives up, saying so", func(t *testing.T) {
		sink := logtest.Install(t)
		b := &reconnecting{Fake: terminalOn(takenSession), forever: true}
		const wait = 30 * time.Millisecond

		start := time.Now()
		takingConnector(b, &runnersFake{}, wait).take(context.Background(), takenSession)

		if took := time.Since(start); took < wait || took > time.Second {
			t.Errorf("the take took %s, want it giving up once %s had passed", took, wait)
		}
		rec := sink.Records().Matching("attach", "take failed").AtExactLevel(slog.LevelWarn).Only(t, "the wait's warning")
		if detail := rec.AttrString(t, "detail"); detail != "wait" {
			t.Errorf("detail = %q, want wait", detail)
		}
		if err := rec.ErrorAttr(t, "error"); !errors.Is(err, errNotAlone) {
			t.Errorf("error = %v, want a terminal still holding the session", err)
		}
	})

	for _, tc := range []struct {
		name, detail string
		fail         func(*backendtest.Fake, *runnersFake)
	}{
		{"terminals it can't read", "read", func(f *backendtest.Fake, _ *runnersFake) {
			f.Fail(backendtest.OpAttachClients, backend.ErrUnreadableClient)
		}},
		{"a session it can't read", "read", func(f *backendtest.Fake, _ *runnersFake) { f.Fail(backendtest.OpSession, backend.ErrTimeout) }},
		{"a record it can't write", "record", func(_ *backendtest.Fake, r *runnersFake) { r.recordErr = errors.New("disk full") }},
		{"a detach that fails", "detach", func(f *backendtest.Fake, _ *runnersFake) { f.Fail(backendtest.OpDetach, backend.ErrTimeout) }},
		{"a take lock it can't have", "lock", func(_ *backendtest.Fake, r *runnersFake) { r.lockErr = errors.New("lock held past the bound") }},
	} {
		t.Run(tc.name+" is logged", func(t *testing.T) {
			sink := logtest.Install(t)
			fake := terminalOn(takenSession)
			runners := &runnersFake{}
			tc.fail(fake, runners)

			takingConnector(fake, runners, 30*time.Millisecond).take(context.Background(), takenSession)

			got := sink.Records().Matching("attach", "take failed").AtExactLevel(slog.LevelWarn)
			if len(got) == 0 || got[0].AttrString(t, "detail") != tc.detail {
				t.Errorf("take warnings = %v, want the first saying %s", got, tc.detail)
			}
		})
	}

	t.Run("terminals it can't read detach nothing", func(t *testing.T) {
		fake := terminalOn(takenSession)
		fake.Fail(backendtest.OpAttachClients, backend.ErrUnreadableClient)
		runners := &runnersFake{}

		takingConnector(fake, runners, takeWait).take(context.Background(), takenSession)

		if n := detaches(fake); n != 0 || len(runners.records) != 0 {
			t.Errorf("detached %d times, recorded %v; want nothing done on what it can't see", n, runners.records)
		}
	})

	t.Run("a take that fails still attaches", func(t *testing.T) {
		fake, rec := stageAttach(t, takenSession)
		fake.Fail(backendtest.OpAttachClients, backend.ErrUnreadableClient)
		rec.runners.lockErr = errors.New("lock held past the bound")

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		rec.onlyRun(t)
	})
}

func TestTake_BeforeEveryAttach(t *testing.T) {
	t.Run("the session is taken, and alone, before the attach begins", func(t *testing.T) {
		fake := terminalOn(takenSession)
		b := &reconnecting{Fake: fake, again: 1}
		withBootstrapDeps(t, BootstrapDeps{})
		withBackendDeps(t, BackendDeps{Backend: b})
		rec := &attachRecorder{}
		withOpenDeps(t, OpenDeps{Execer: rec, Runner: rec.Run, Runners: &rec.runners})
		rec.answer(runResult{ended: func() {
			if s, _ := fake.Session(context.Background(), takenSession); len(rec.runners.takes) != 1 || detaches(fake) != 2 || s.Attached() {
				t.Errorf("at the attach: takes %v, %d detaches, attached %v; want the take done", rec.runners.takes, detaches(fake), s.Attached())
			}
		}})

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		rec.onlyRun(t)
	})

	t.Run("each attach of the loop takes its session", func(t *testing.T) {
		_, rec := stageAttach(t, takenSession, "blog-c3d4")
		rec.answer(runResult{out: runner.Outcome{Switch: "blog-c3d4"}})

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if got := rec.runners.takes; len(got) != 2 || got[0].session != takenSession || got[1].session != "blog-c3d4" {
			t.Errorf("takes = %v, want each attached session taken as it attached", got)
		}
	})
}

// Two takes at once must leave the later one's attach alone on the session,
// so each holds the take lock from its first ask until its attach leads.
func TestTake_Lock(t *testing.T) {
	t.Run("the session's own is taken before the take's first ask", func(t *testing.T) {
		_, rec := stageAttach(t, takenSession)
		var asked []string
		rec.runners.onTake = func(string) (int, error) { asked = slices.Clone(rec.runners.locked); return 0, nil }

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []string{takenSession}; !slices.Equal(asked, want) {
			t.Errorf("the take asked with locks %v held, want %v's held first", asked, want)
		}
	})

	t.Run("it is held through the attach's start, and released once the attach leads", func(t *testing.T) {
		_, rec := stageAttach(t, takenSession)
		var leading atomic.Bool
		rec.runners.confirm = func(context.Context) error {
			if leading.Load() {
				return nil
			}
			return errors.New("another client leads")
		}
		rec.answer(runResult{ended: func() {
			time.Sleep(3 * takePoll)
			if n := rec.runners.unlocks.Load(); n != 0 {
				t.Errorf("released %d times before the attach led, want held", n)
			}
			leading.Store(true)
			if !harnesstest.PollUntil(t, time.Second, takePoll, func() bool { return rec.runners.unlocks.Load() == 1 }) {
				t.Error("the lock was held on once the attach led")
			}
		}})

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
	})

	t.Run("it is released as the attach ends, led or not", func(t *testing.T) {
		_, rec := stageAttach(t, takenSession)
		rec.runners.confirm = func(context.Context) error { return errors.New("another client leads") }

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if n := rec.runners.unlocks.Load(); len(rec.runners.locked) != 1 || n != 1 {
			t.Errorf("held %v, released %d; want once each", rec.runners.locked, n)
		}
	})

	t.Run("it is released before an exec takes the attach's place", func(t *testing.T) {
		_, rec := stageAttach(t, takenSession)
		rec.answer(runResult{err: runner.ErrNoTerminal})

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if len(rec.execs) != 1 || rec.releasedAtExec != 1 {
			t.Errorf("execs %d, with %d locks released as it ran; want the lock let go first, since an exec never returns to release it", len(rec.execs), rec.releasedAtExec)
		}
	})

	t.Run("its wait outlasts another take's longest hold", func(t *testing.T) {
		hold := runnerreg.AskBound() + 2*saverhost.ReadCeiling + saverhost.DetachCeiling + 2*takeWait
		if got := takeLockWait(); got <= hold {
			t.Errorf("takeLockWait() = %s, want past the %s a take holds its lock for at most: its asks, its first reads, its detach, and its two waits", got, hold)
		}
	})

	t.Run("an attach that never leads releases it past the bound, saying so", func(t *testing.T) {
		sink := logtest.Install(t)
		release := func() {}
		released := make(chan struct{})
		c := takingConnector(backendtest.New(), &runnersFake{}, 30*time.Millisecond)

		go func() {
			defer close(released)
			c.awaitLead(context.Background(), takenSession, func(context.Context) error { return errors.New("another client leads") }, release)
		}()

		select {
		case <-released:
		case <-time.After(time.Second):
			t.Fatal("the lock was held past the bound")
		}
		rec := sink.Records().Matching("attach", "take failed").AtExactLevel(slog.LevelWarn).Only(t, "the lead's warning")
		if detail := rec.AttrString(t, "detail"); detail != "lead" {
			t.Errorf("detail = %q, want lead", detail)
		}
	})
}

func TestTake_Landing(t *testing.T) {
	t.Run("a take from elsewhere opens the picker on the taken screen, saying where", func(t *testing.T) {
		_, rec := stageAttach(t, takenSession)
		rec.answer(runResult{out: runner.Outcome{Taken: true, By: takeAt}})
		picker := stagePicker(t)

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []tui.Taken{{Session: takenSession, By: takeAt}}; !slices.Equal(picker.taken, want) {
			t.Errorf("picker landings = %+v, want %+v", picker.taken, want)
		}
	})

	t.Run("a drop with the session live is a take, where it went read from the record since the attach began", func(t *testing.T) {
		sink := logtest.Install(t)
		_, rec := stageAttach(t, takenSession)
		var taking, attaching time.Time
		rec.runners.onTake = func(string) (int, error) { taking = time.Now(); return 0, nil }
		rec.runners.takenBy = takeAt
		rec.answer(runResult{out: runner.Outcome{Dropped: true}, ended: func() { attaching = time.Now() }})
		picker := stagePicker(t)

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []tui.Taken{{Session: takenSession, By: takeAt}}; !slices.Equal(picker.taken, want) {
			t.Errorf("picker landings = %+v, want %+v", picker.taken, want)
		}
		if since := rec.runners.takenSince; len(since) != 1 || !since[0].After(taking) || since[0].After(attaching) {
			t.Errorf("read the record since %v, want after this attach's own take at %v and by the attach at %v", since, taking, attaching)
		}
		assertDropJudged(t, sink, "taken", dropRecorded)
	})

	for _, tc := range []struct {
		name string
		// after runs as the attach ends.
		after func(*backendtest.Fake)
	}{
		{"another terminal holds the session", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: takenSession, Clients: 1})
			f.SetAttachClients(takenSession, aTerminal)
		}},
		{"a client zmx switched onto the session from another leads it", func(f *backendtest.Fake) {
			f.SetLeaderEnv(takenSession, aLeader)
		}},
	} {
		t.Run("a drop with no record lands on the screen, saying no more, while "+tc.name, func(t *testing.T) {
			sink := logtest.Install(t)
			fake, rec := stageAttach(t, takenSession)
			rec.answer(runResult{out: runner.Outcome{Dropped: true}, ended: func() { tc.after(fake) }})
			picker := stagePicker(t)

			if err := openSession(contextCommand(), takenSession); err != nil {
				t.Fatalf("openSession: %v", err)
			}
			if want := []tui.Taken{{Session: takenSession}}; !slices.Equal(picker.taken, want) {
				t.Errorf("picker landings = %+v, want %+v", picker.taken, want)
			}
			assertDropJudged(t, sink, "taken", dropHeld)
		})
	}

	for _, tc := range []struct {
		name string
		why  dropWhy
		// after runs as the attach ends.
		after func(*backendtest.Fake)
	}{
		{"with no record and no other terminal on the session, as after a detach typed in it", dropUnheld, func(*backendtest.Fake) {}},
		{"with the session gone, its end", dropEnded, func(f *backendtest.Fake) { f.Remove(takenSession) }},
		{"the backend can't judge", dropUnjudged, func(f *backendtest.Fake) { f.Fail(backendtest.OpExists, backend.ErrTimeout) }},
		{"whose terminals can't be read", dropUnjudged, func(f *backendtest.Fake) {
			f.Fail(backendtest.OpAttachClients, backend.ErrUnreadableClient)
		}},
	} {
		t.Run("a drop "+tc.name+" exits 0 to the shell", func(t *testing.T) {
			sink := logtest.Install(t)
			fake, rec := stageAttach(t, takenSession)
			rec.answer(runResult{out: runner.Outcome{Dropped: true}, ended: func() { tc.after(fake) }})
			picker := stagePicker(t)

			if err := openSession(contextCommand(), takenSession); err != nil {
				t.Errorf("openSession = %v, want nil", err)
			}
			if len(picker.taken) != 0 {
				t.Errorf("picker landings %+v, want no picker", picker.taken)
			}
			assertDropJudged(t, sink, "shell", tc.why)
		})
	}
}

// assertDropJudged holds the log to one judgement of the drop of
// takenSession, with result and why.
func assertDropJudged(t *testing.T, sink *logtest.Sink, result string, why dropWhy) {
	t.Helper()
	rec := sink.Records().Matching("attach", "drop judged").AtExactLevel(slog.LevelInfo).Only(t, "the drop's judgement")
	if got := rec.AttrString(t, "session"); got != takenSession {
		t.Errorf("session = %q, want %q", got, takenSession)
	}
	if got := rec.AttrString(t, "result"); got != result {
		t.Errorf("result = %q, want %q", got, result)
	}
	if got := rec.AttrString(t, "detail"); got != string(why) {
		t.Errorf("detail = %q, want %q", got, why)
	}
}
