//go:build integration

// The measurement behind selfSupervisionHysteresisTicks: against real zmx and
// a daemon in _portal-saver, it measures the longest run of ticks a healthy
// daemon's self-check could read the saver session as lost, and holds N at no
// less than twice that, within [3, 9]. The probe is written here rather than
// borrowed from the daemon, so it measures the condition apart from the code
// it validates.

package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/saver"
)

const (
	hysteresisRuns   = 3
	hysteresisWindow = 10 * time.Second
	// hysteresisPause spaces out a disturbance's steps.
	hysteresisPause = 200 * time.Millisecond
)

func TestDaemonSelfSupervisionHysteresisAgainstZmx(t *testing.T) {
	binary := portalbintest.Build(t)
	scenarios := []struct {
		name    string
		disturb func(ctx context.Context, t *testing.T, d *portaltest.SaverDaemon)
	}{
		{"steady state", nil},
		{"clients attaching to the saver and detaching", attachToTheSaver},
		{"sessions made and killed beside it", churnSessions},
	}

	longest := 0
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			d := portaltest.StartSaverDaemon(t, binary)
			for run := range hysteresisRuns {
				lost := longestLostRun(t, d, sc.disturb)
				t.Logf("run %d: the saver session read as lost for at most %d ticks running", run+1, lost)
				longest = max(longest, lost)
			}
			if err := d.Signal(0); err != nil {
				t.Errorf("the daemon %d ended under the disturbance: %v\n--- portal.log ---\n%s",
					d.PID, err, portaltest.ReadPortalLogSafe(d.StateDir))
			}
		})
	}

	t.Logf("longest lost run %d ticks, N %d", longest, selfSupervisionHysteresisTicks)
	if selfSupervisionHysteresisTicks < 2*longest {
		t.Errorf("selfSupervisionHysteresisTicks = %d, want at least twice the longest lost run, %d",
			selfSupervisionHysteresisTicks, longest)
	}
	if selfSupervisionHysteresisTicks < 3 || selfSupervisionHysteresisTicks > 9 {
		t.Errorf("selfSupervisionHysteresisTicks = %d, want 3 <= N <= 9", selfSupervisionHysteresisTicks)
	}
}

// longestLostRun reads the saver session once a daemon tick for
// hysteresisWindow, while disturb runs, and returns the longest run of reads
// that found it lost. A read that fails for any reason counts as lost, more
// strictly than the daemon counts one.
func longestLostRun(t *testing.T, d *portaltest.SaverDaemon, disturb func(context.Context, *testing.T, *portaltest.SaverDaemon)) int {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
	}()
	if disturb != nil {
		wg.Go(func() { disturb(ctx, t, d) })
	}

	ticker := time.NewTicker(daemonTick)
	defer ticker.Stop()
	longest, run := 0, 0
	for deadline := time.Now().Add(hysteresisWindow); time.Now().Before(deadline); {
		<-ticker.C
		if s, err := d.Backend.Session(ctx, saver.SessionName); err == nil && s.PID == d.PID {
			run = 0
			continue
		}
		run++
		longest = max(longest, run)
	}
	return longest
}

// attachToTheSaver attaches a client to the saver session and detaches it,
// over and over: a client with no input detaches at once.
func attachToTheSaver(ctx context.Context, t *testing.T, d *portaltest.SaverDaemon) {
	attach, err := d.Backend.AttachCommand(ctx, saver.SessionName)
	if err != nil {
		t.Errorf("attach command for %s: %v", saver.SessionName, err)
		return
	}
	for pause(ctx) {
		client := exec.Command(attach.Argv[0], attach.Argv[1:]...)
		client.Env = attach.Env
		if out, err := client.CombinedOutput(); err != nil {
			t.Logf("attach to %s: %v\n%s", saver.SessionName, err, out)
		}
	}
}

// churnSessions makes a session beside the saver and kills it, over and over,
// each kill completing however the window ends.
func churnSessions(ctx context.Context, t *testing.T, d *portaltest.SaverDaemon) {
	dir := t.TempDir()
	for i := 0; pause(ctx); i++ {
		name := fmt.Sprintf("churn-%d", i)
		if _, err := d.Backend.Create(ctx, name, dir, []string{"/bin/sh", "-c", "exec sleep 600"}); err != nil {
			if ctx.Err() == nil {
				t.Logf("create %s: %v", name, err)
			}
			continue
		}
		if err := d.Backend.Kill(context.WithoutCancel(ctx), name); err != nil {
			t.Logf("kill %s: %v", name, err)
		}
	}
}

// pause waits hysteresisPause, reporting whether ctx is still running.
func pause(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(hysteresisPause):
		return true
	}
}
