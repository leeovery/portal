package cmd

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/tui"
)

const takenSession = "api-x7Kd9a"

// takeAt is where the tests' takes say a session went, from the environment
// they pin.
const takeAt = "SSH from 192.0.2.20"

// lingering is a Fake that counts a client on every session for its next
// reads past a detach, as zmx counts a client until its socket closes; held
// counts them for every read.
type lingering struct {
	*backendtest.Fake
	reads int
	held  bool
}

func (b *lingering) Detach(ctx context.Context, name string) error {
	if err := b.Fake.Detach(ctx, name); err != nil {
		return err
	}
	b.reads = 3
	return nil
}

func (b *lingering) Session(ctx context.Context, name string) (backend.Session, error) {
	s, err := b.Fake.Session(ctx, name)
	if err == nil && (b.held || b.reads > 0) {
		s.Clients++
		b.reads = max(0, b.reads-1)
	}
	return s, err
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
		fake := backendtest.New(backend.Session{Name: takenSession, Clients: 1})
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			pinWhere(t)
			fake := backendtest.New(backend.Session{Name: takenSession, Clients: 1})
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

	t.Run("a session with nothing attached is neither recorded nor detached", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: takenSession})
		runners := &runnersFake{}

		takingConnector(fake, runners, takeWait).take(context.Background(), takenSession)

		if n := detaches(fake); n != 0 || len(runners.records) != 0 {
			t.Errorf("detached %d times, recorded %v; want nothing done", n, runners.records)
		}
	})

	t.Run("Portal's own session is never taken", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "_portal-saver", Clients: 1})
		runners := &runnersFake{}

		takingConnector(fake, runners, takeWait).take(context.Background(), "_portal-saver")

		if len(runners.takes) != 0 || len(fake.Calls()) != 0 {
			t.Errorf("takes %v, backend calls %v; want the session untouched", runners.takes, fake.Calls())
		}
	})

	t.Run("it waits for the session to have no other client", func(t *testing.T) {
		b := &lingering{Fake: backendtest.New(backend.Session{Name: takenSession, Clients: 1})}

		takingConnector(b, &runnersFake{}, takeWait).take(context.Background(), takenSession)

		if b.reads != 0 {
			t.Errorf("the take returned with the session counting a client for %d more reads", b.reads)
		}
	})

	t.Run("past its bound it gives up, saying so", func(t *testing.T) {
		sink := logtest.Install(t)
		b := &lingering{Fake: backendtest.New(backend.Session{Name: takenSession, Clients: 1}), held: true}
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
			t.Errorf("error = %v, want clients still attached", err)
		}
	})

	for _, tc := range []struct {
		name, detail string
		fail         func(*backendtest.Fake, *runnersFake)
	}{
		{"a session it can't read", "read", func(f *backendtest.Fake, _ *runnersFake) { f.Fail(backendtest.OpSession, backend.ErrTimeout) }},
		{"a record it can't write", "record", func(_ *backendtest.Fake, r *runnersFake) { r.recordErr = errors.New("disk full") }},
		{"a detach that fails", "detach", func(f *backendtest.Fake, _ *runnersFake) { f.Fail(backendtest.OpDetach, backend.ErrTimeout) }},
	} {
		t.Run(tc.name+" is logged", func(t *testing.T) {
			sink := logtest.Install(t)
			fake := backendtest.New(backend.Session{Name: takenSession, Clients: 1})
			runners := &runnersFake{}
			tc.fail(fake, runners)

			takingConnector(fake, runners, 30*time.Millisecond).take(context.Background(), takenSession)

			got := sink.Records().Matching("attach", "take failed").AtExactLevel(slog.LevelWarn)
			if len(got) == 0 || got[0].AttrString(t, "detail") != tc.detail {
				t.Errorf("take warnings = %v, want the first saying %s", got, tc.detail)
			}
		})
	}

	t.Run("a take that fails still attaches", func(t *testing.T) {
		fake, rec := stageAttach(t, takenSession)
		fake.Fail(backendtest.OpSession, backend.ErrTimeout)

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		rec.onlyRun(t)
	})
}

func TestTake_BeforeEveryAttach(t *testing.T) {
	t.Run("the session is taken, and alone, before the attach begins", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: takenSession, Clients: 1})
		b := &lingering{Fake: fake}
		withBootstrapDeps(t, BootstrapDeps{})
		withBackendDeps(t, BackendDeps{Backend: b})
		rec := &attachRecorder{}
		withOpenDeps(t, OpenDeps{Execer: rec, Runner: rec.Run, Runners: &rec.runners})
		rec.answer(runResult{ended: func() {
			if len(rec.runners.takes) != 1 || detaches(fake) != 1 || b.reads != 0 {
				t.Errorf("at the attach: takes %v, %d detaches, %d reads counting a client; want the take done", rec.runners.takes, detaches(fake), b.reads)
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
	})

	t.Run("a drop with no record says only that another device has it", func(t *testing.T) {
		_, rec := stageAttach(t, takenSession)
		rec.answer(runResult{out: runner.Outcome{Dropped: true}})
		picker := stagePicker(t)

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []tui.Taken{{Session: takenSession}}; !slices.Equal(picker.taken, want) {
			t.Errorf("picker landings = %+v, want %+v", picker.taken, want)
		}
	})

	t.Run("a drop with the session gone is its end, exiting 0 to the shell", func(t *testing.T) {
		fake, rec := stageAttach(t, takenSession)
		rec.answer(runResult{out: runner.Outcome{Dropped: true}, ended: func() { fake.Remove(takenSession) }})
		picker := stagePicker(t)

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Errorf("openSession = %v, want nil", err)
		}
		if len(picker.taken) != 0 || len(rec.runners.takenSince) != 0 {
			t.Errorf("picker landings %+v, records read %d; want no picker", picker.taken, len(rec.runners.takenSince))
		}
	})

	t.Run("a drop the backend can't judge exits to the shell", func(t *testing.T) {
		fake, rec := stageAttach(t, takenSession)
		rec.answer(runResult{out: runner.Outcome{Dropped: true}, ended: func() { fake.Fail(backendtest.OpExists, backend.ErrTimeout) }})
		picker := stagePicker(t)

		if err := openSession(contextCommand(), takenSession); err != nil {
			t.Errorf("openSession = %v, want nil", err)
		}
		if len(picker.taken) != 0 {
			t.Errorf("picker landings %+v, want no picker", picker.taken)
		}
	})
}
