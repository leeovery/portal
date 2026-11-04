package cmd

import (
	"log/slog"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
)

// saverReads are what one tick's self-check can find of the saver session.
var saverReads = map[string]func(*backendtest.Fake){
	"held":    func(f *backendtest.Fake) { f.Add(backend.Session{Name: saver.SessionName, PID: os.Getpid()}) },
	"gone":    func(f *backendtest.Fake) { f.Remove(saver.SessionName) },
	"another": func(f *backendtest.Fake) { f.Add(backend.Session{Name: saver.SessionName, PID: 1}) },
	"silent": func(f *backendtest.Fake) {
		f.Add(backend.Session{Name: saver.SessionName, PID: os.Getpid()})
		f.StopAnswering(saver.SessionName)
	},
	"no pid": func(f *backendtest.Fake) { f.Add(backend.Session{Name: saver.SessionName}) },
}

func TestSupervise_SelfEjectsOnTheThirdTickInARunReadingItLost(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reads []string
		// ejectsOn is the read, counting from 1, the daemon ejects on; 0 for
		// none.
		ejectsOn int
	}{
		{"the session gone three ticks running", []string{"gone", "gone", "gone"}, 3},
		{"the session running another process counts as gone", []string{"another", "gone", "another"}, 3},
		{"a tick the session doesn't answer neither counts nor resets", []string{"gone", "gone", "silent", "gone"}, 4},
		{"a tick it names no process neither counts nor resets", []string{"gone", "no pid", "gone", "gone"}, 4},
		{"the session held again ends the run", []string{"gone", "gone", "held", "gone", "gone", "gone"}, 6},
		{"two ticks lost never eject", []string{"gone", "gone", "held", "gone", "gone"}, 0},
		{"held throughout never ejects", []string{"held", "held", "held", "held"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, fake := newTestDaemon(t, &fakeSaver{})
			var exits []int
			withOsExitFake(t, func(code int) { exits = append(exits, code) })
			sink := logtest.Install(t)

			ejectedOn := 0
			for i, read := range tc.reads {
				saverReads[read](fake)
				if deps.supervise(t.Context()) {
					ejectedOn = i + 1
					break
				}
			}

			if ejectedOn != tc.ejectsOn {
				t.Fatalf("ejected on read %d of %v, want %d", ejectedOn, tc.reads, tc.ejectsOn)
			}
			if tc.ejectsOn == 0 {
				if len(exits) != 0 {
					t.Errorf("exited %v, want no exit", exits)
				}
				return
			}
			if !slices.Equal(exits, []int{0}) {
				t.Errorf("exited %v, want one exit 0", exits)
			}
			rec := sink.Records().Matching("daemon", "self-eject").AtExactLevel(slog.LevelInfo).Only(t, "the self-eject line")
			if ticks, threshold := rec.IntAttr(t, "ticks"), rec.IntAttr(t, "threshold"); ticks != 3 || threshold != 3 {
				t.Errorf("self-eject ticks=%d threshold=%d, want 3 and 3", ticks, threshold)
			}
		})
	}
}

func TestSelfEject_LogsTheEjectThenTheProcessExitThenExits(t *testing.T) {
	sink := logtest.Install(t)
	var atExit logtest.Records
	var code = -1
	withOsExitFake(t, func(c int) {
		code = c
		atExit = sink.Records()
	})

	selfEject(selfSupervisionHysteresisTicks)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	eject := slices.IndexFunc(atExit, func(r logtest.Record) bool { return r.Matches("daemon", "self-eject") })
	exit := slices.IndexFunc(atExit, func(r logtest.Record) bool {
		return r.Matches("process", "exit") && r.AttrOrEmpty("code") == "0"
	})
	if eject < 0 || exit < 0 || exit < eject {
		t.Errorf("records at the exit = %+v, want self-eject, then process exit code=0, both before it", atExit)
	}
}

func TestDaemon_SelfEjectSkipsTheShutdownFlushAndLeavesItsPIDFile(t *testing.T) {
	s := &fakeSaver{}
	deps, fake := newTestDaemon(t, s)
	fake.Remove(saver.SessionName)
	ticks := make(chan time.Time)
	deps.Ticks = ticks
	var exits atomic.Int32
	withOsExitFake(t, func(int) { exits.Add(1) })
	sink := logtest.Install(t)

	done := make(chan error, 1)
	go func() { done <- defaultDaemonRun(t.Context(), deps) }()
	for n := range selfSupervisionHysteresisTicks {
		ticks <- second(n)
	}
	if err := <-done; err != nil {
		t.Errorf("defaultDaemonRun = %v, want nil", err)
	}

	if got := exits.Load(); got != 1 {
		t.Errorf("exited %d times, want once", got)
	}
	if polls, flushes := s.counts(); polls != 1 || flushes != 0 {
		t.Errorf("polls %d, flushes %d; want the first tick's poll alone, none on the eject tick, and no flush", polls, flushes)
	}
	if got := sink.Records().Matching("daemon", "shutdown"); len(got) != 0 {
		t.Errorf("logged %+v, want no shutdown line on the eject path", got)
	}
	if pid, err := state.ReadPIDFile(deps.Dir); err != nil || pid != os.Getpid() {
		t.Errorf("daemon.pid = %d, %v; want the ejected daemon's pid left in place", pid, err)
	}
}
