//go:build integration

package cmd_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

var shutdownSessions = []killPathSession{
	{name: "alpha", hookKeys: []string{hookstest.ReapableSeedA, hookstest.ReapableSeedB}},
	{name: "bravo", hookKeys: []string{hookstest.ReapableSeedC}},
	{name: "charlie", hookKeys: []string{hookstest.ReapableSeedD}},
}

// daemonTickPeriod mirrors the production daemon's ticker, which the in-flight
// trials schedule kill-server against.
const daemonTickPeriod = time.Second

// shutdownSettleBudget bounds how long the state directory may keep changing
// once the daemon and the server are gone, while session-closed hooks'
// commit-now subprocesses finish.
const shutdownSettleBudget = 10 * time.Second

// shutdownQuietWindow is how long the state directory must hold still before a
// trial reads it.
const shutdownQuietWindow = 500 * time.Millisecond

const shutdownSettlePollInterval = 100 * time.Millisecond

// cycleCalibrationBudget bounds the requested save the in-flight trials time
// themselves against: one tick period plus a loaded host's slack.
const cycleCalibrationBudget = 10 * time.Second

// scheduleLead is the least time left between requesting a save and the tick
// that runs it, so the request is on disk before that tick reads it.
const scheduleLead = 300 * time.Millisecond

func TestShutdown_DaemonSIGTERMedBeforeServerPreservesFullState(t *testing.T) {
	binary, binDir := stageShutdownBinary(t)

	for _, delay := range daemonLeadDelays() {
		t.Run(fmt.Sprintf("server SIGTERMed %s after the daemon", delay), func(t *testing.T) {
			rt := newShutdownRuntime(t, binary, binDir)

			rt.signal(t, rt.daemonPID, syscall.SIGTERM)
			time.Sleep(delay)
			rt.signal(t, rt.serverPID, syscall.SIGTERM)

			rt.awaitShutdown(t)
			rt.assertFullStatePreserved(t)
		})
	}
}

func TestShutdown_KillServerLeavesSavedStateUnchanged(t *testing.T) {
	binary, binDir := stageShutdownBinary(t)
	rt := newShutdownRuntime(t, binary, binDir)

	rt.killServer(t)

	rt.awaitShutdown(t)
	rt.assertSavedStateUnchanged(t)
}

func TestShutdown_KillServerDuringSaveLeavesSavedStateUnchanged(t *testing.T) {
	binary, binDir := stageShutdownBinary(t)

	const trials = 24
	var inFlight int
	for i := range trials {
		t.Run(fmt.Sprintf("trial %d", i), func(t *testing.T) {
			rt := newShutdownRuntime(t, binary, binDir)
			cycleStart, took := rt.calibrateSaveCycle(t)
			offset := saveOffset(i, trials, took)

			requested := rt.requestSaveAt(t, cycleStart)
			sleepUntil(requested.next.Add(offset))
			rt.killServer(t)

			rt.awaitShutdown(t)
			rt.assertSavedStateUnchanged(t)

			landing := rt.classifyLanding(requested.at)
			t.Logf("kill-server %s into a %s save: %s", offset, took, landing)
			if landing == landedInFlight {
				inFlight++
			}
		})
	}
	if !t.Failed() && inFlight == 0 {
		t.Fatalf("no trial's kill-server landed while the daemon's save was in flight")
	}
}

// daemonLeadDelays spans the 10–30ms lead at which the server's exit lands
// during the daemon's shutdown flush.
func daemonLeadDelays() []time.Duration {
	var delays []time.Duration
	for ms := 10; ms <= 30; ms += 2 {
		delays = append(delays, time.Duration(ms)*time.Millisecond)
	}
	return delays
}

// saveOffset spreads the trials from just before the save starts to just past
// its end, so the exit lands at different points across them.
func saveOffset(trial, trials int, took time.Duration) time.Duration {
	span := took + took/2
	return -took/4 + span*time.Duration(trial)/time.Duration(trials-1)
}

func stageShutdownBinary(t *testing.T) (binary, binDir string) {
	t.Helper()
	requireUnwrappedTmux(t)

	binDir = portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}
	return binary, binDir
}

// requireUnwrappedTmux refuses a tmux on PATH that is a script: a wrapper
// shifts the timing these trials depend on.
func requireUnwrappedTmux(t *testing.T) {
	t.Helper()
	path, err := exec.LookPath("tmux")
	if err != nil {
		t.Skipf("tmux not on PATH; skipping: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve tmux at %s: %v", path, err)
	}
	f, err := os.Open(resolved)
	if err != nil {
		t.Fatalf("open tmux at %s: %v", resolved, err)
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 2)
	if _, err := f.Read(head); err != nil {
		t.Fatalf("read tmux at %s: %v", resolved, err)
	}
	if string(head) == "#!" {
		t.Fatalf("tmux on PATH (%s) is a script wrapping tmux; these trials need tmux itself", resolved)
	}
}

// shutdownRuntime is a live Portal runtime whose daemon has saved every user
// pane, with the saved state it is expected to keep through a shutdown.
type shutdownRuntime struct {
	symptomFixture
	daemonPID     int
	serverPID     int
	savedFiles    map[string][]string
	sessionsJSON  []byte
	scrollback    map[string][]byte
	sessionsNamed []string
}

func newShutdownRuntime(t *testing.T, binary, binDir string) shutdownRuntime {
	t.Helper()

	// Unset before the tmux server starts, so the daemon it hosts and every
	// hook subprocess it runs log at the production default level.
	t.Setenv("PORTAL_LOG_LEVEL", "")
	if err := os.Unsetenv("PORTAL_LOG_LEVEL"); err != nil {
		t.Fatalf("unset PORTAL_LOG_LEVEL: %v", err)
	}

	fixture, _ := newKillPathFixture(t, binary, binDir, shutdownSessions)
	saved := waitForDumpedScrollback(t, fixture, shutdownSessions)

	daemonPID, present, err := tmux.SaverPanePIDOrAbsent(fixture.sock.Client(), tmux.PortalSaverName)
	if err != nil || !present {
		t.Fatalf("read daemon pid: present=%v err=%v\n%s", present, err, fixture.diagnostic())
	}
	assertDefaultLogLevel(t, fixture.stateDir, daemonPID)

	rt := shutdownRuntime{
		symptomFixture: fixture,
		daemonPID:      daemonPID,
		serverPID:      liveServerPID(t, fixture.sock),
		savedFiles:     saved,
		sessionsNamed:  sessionNamesOf(shutdownSessions),
	}
	rt.sessionsJSON, rt.scrollback = rt.readSavedState(t)
	return rt
}

// assertDefaultLogLevel reads the daemon's own resolution line rather than the
// environment, so a level reaching it some other way still fails the trial.
func assertDefaultLogLevel(t *testing.T, stateDir string, pid int) {
	t.Helper()
	want := fmt.Sprintf("pid=%d", pid)
	for _, line := range portalLogLines(stateDir) {
		parsed, ok := log.ParseLogLine(line)
		if !ok || parsed.Component != "process" || parsed.Message != "log-level resolved" {
			continue
		}
		fields := strings.Fields(line)
		if !slices.Contains(fields, want) {
			continue
		}
		if !slices.Contains(fields, "resolved=info") || !slices.Contains(fields, "source=default") {
			t.Fatalf("daemon %d did not run at the production default log level: %s", pid, line)
		}
		return
	}
	t.Fatalf("no log-level resolved line for daemon %d\n--- portal.log ---\n%s",
		pid, portaltest.ReadPortalLogSafe(stateDir))
}

func (rt shutdownRuntime) readSavedState(t *testing.T) ([]byte, map[string][]byte) {
	t.Helper()
	index, err := os.ReadFile(state.SessionsJSON(rt.stateDir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	files := map[string][]byte{}
	for _, rel := range slices.Concat(slices.Collect(maps.Values(rt.savedFiles))...) {
		data, err := os.ReadFile(filepath.Join(rt.stateDir, rel))
		if err != nil {
			t.Fatalf("read saved scrollback %s: %v", rel, err)
		}
		files[rel] = data
	}
	return index, files
}

// pid must be this fixture's tmux server or a process it hosts.
func (rt shutdownRuntime) signal(t *testing.T, pid int, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(pid, sig); err != nil {
		t.Fatalf("signal %v to %d: %v\n%s", sig, pid, err, rt.diagnostic())
	}
}

func (rt shutdownRuntime) killServer(t *testing.T) {
	t.Helper()
	if out, err := rt.sock.TryRun("kill-server"); err != nil {
		t.Fatalf("tmux kill-server: %v\n%s", err, out)
	}
}

// awaitShutdown returns once the daemon and the server are gone and the state
// directory has stopped changing.
func (rt shutdownRuntime) awaitShutdown(t *testing.T) {
	t.Helper()
	for _, p := range []struct {
		what string
		pid  int
	}{{"daemon", rt.daemonPID}, {"tmux server", rt.serverPID}} {
		gone := func() bool { return errors.Is(syscall.Kill(p.pid, 0), syscall.ESRCH) }
		if !harnesstest.PollUntil(t, daemonExitBudget, daemonTickPollInterval, gone) {
			t.Fatalf("%s %d still running %s after the shutdown", p.what, p.pid, daemonExitBudget)
		}
	}
	awaitStateDirSettled(t, rt.stateDir)
}

func awaitStateDirSettled(t *testing.T, stateDir string) {
	t.Helper()
	deadline := time.Now().Add(shutdownSettleBudget)
	prev := snapshotForSettle(t, stateDir)
	stillSince := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(shutdownSettlePollInterval)
		cur := snapshotForSettle(t, stateDir)
		if len(portaltest.DiffFingerprints(prev, cur)) != 0 {
			prev, stillSince = cur, time.Now()
			continue
		}
		if time.Since(stillSince) >= shutdownQuietWindow {
			return
		}
	}
	t.Fatalf("state directory still changing %s after the shutdown\n%s",
		shutdownSettleBudget, dumpStateDir(stateDir))
}

func snapshotForSettle(t *testing.T, stateDir string) map[string]portaltest.Fingerprint {
	t.Helper()
	snap, err := portaltest.SnapshotStateDir(stateDir)
	if err != nil {
		t.Fatalf("snapshot state dir: %v", err)
	}
	return snap
}

// assertFullStatePreserved holds sessions.json to naming every saved session
// and every scrollback file it names to being present and non-empty.
func (rt shutdownRuntime) assertFullStatePreserved(t *testing.T) {
	t.Helper()
	idx, skip, err := state.ReadIndex(rt.stateDir)
	if err != nil || skip {
		t.Fatalf("read sessions.json after the shutdown: skip=%v err=%v\n%s",
			skip, err, rt.shutdownDiagnostic())
	}
	named := slices.Sorted(maps.Keys(sessionNames(idx)))
	if want := slices.Sorted(slices.Values(rt.sessionsNamed)); !slices.Equal(named, want) {
		t.Errorf("sessions.json after the shutdown names %v, want %v\n%s",
			named, want, rt.shutdownDiagnostic())
	}
	referenced := slices.Concat(slices.Collect(maps.Values(scrollbackFilesBySession(idx)))...)
	for _, rel := range slices.Concat(referenced, slices.Collect(maps.Keys(rt.scrollback))) {
		info, err := os.Stat(filepath.Join(rt.stateDir, rel))
		switch {
		case err != nil:
			t.Errorf("scrollback %s after the shutdown: %v", rel, err)
		case info.Size() == 0:
			t.Errorf("scrollback %s is empty after the shutdown", rel)
		}
	}
}

func (rt shutdownRuntime) assertSavedStateUnchanged(t *testing.T) {
	t.Helper()
	index, err := os.ReadFile(state.SessionsJSON(rt.stateDir))
	if err != nil {
		t.Fatalf("read sessions.json after the shutdown: %v\n%s", err, rt.shutdownDiagnostic())
	}
	if !bytes.Equal(index, rt.sessionsJSON) {
		t.Errorf("sessions.json changed across the shutdown\n--- before ---\n%s\n--- after ---\n%s\n%s",
			rt.sessionsJSON, index, rt.shutdownDiagnostic())
	}
	for rel, want := range rt.scrollback {
		got, err := os.ReadFile(filepath.Join(rt.stateDir, rel))
		switch {
		case err != nil:
			t.Errorf("scrollback %s after the shutdown: %v", rel, err)
		case !bytes.Equal(got, want):
			t.Errorf("scrollback %s changed across the shutdown: %d bytes before, %d after",
				rel, len(want), len(got))
		}
	}
}

func (rt shutdownRuntime) shutdownDiagnostic() string {
	return fmt.Sprintf("--- state directory (%s) ---\n%s\n--- portal.log ---\n%s",
		rt.stateDir, dumpStateDir(rt.stateDir), portaltest.ReadPortalLogSafe(rt.stateDir))
}

// calibrateSaveCycle requests one save and reads its start and duration back
// from the cycle summary the daemon logs at the default level.
func (rt shutdownRuntime) calibrateSaveCycle(t *testing.T) (start time.Time, took time.Duration) {
	t.Helper()
	before := len(rt.cycleSummaries(t))
	if err := state.TouchSaveRequested(rt.stateDir); err != nil {
		t.Fatalf("request a save: %v", err)
	}
	var summaries []cycleSummary
	logged := func() bool {
		summaries = rt.cycleSummaries(t)
		return len(summaries) > before
	}
	if !harnesstest.PollUntil(t, cycleCalibrationBudget, daemonTickPollInterval, logged) {
		t.Fatalf("requested save logged no cycle summary within %s\n%s",
			cycleCalibrationBudget, rt.shutdownDiagnostic())
	}
	last := summaries[len(summaries)-1]
	return last.end.Add(-last.took), last.took
}

type cycleSummary struct {
	end  time.Time
	took time.Duration
}

func (rt shutdownRuntime) cycleSummaries(t *testing.T) []cycleSummary {
	t.Helper()
	var summaries []cycleSummary
	for _, line := range portalLogLines(rt.stateDir) {
		parsed, ok := log.ParseLogLine(line)
		if !ok || parsed.Component != "capture" || parsed.Message != "tick complete" {
			continue
		}
		took, ok := tookAttr(line)
		if !ok {
			t.Fatalf("cycle summary carries no took: %s", line)
		}
		summaries = append(summaries, cycleSummary{end: parsed.Time, took: took})
	}
	return summaries
}

func tookAttr(line string) (time.Duration, bool) {
	for field := range strings.FieldsSeq(line) {
		if v, ok := strings.CutPrefix(field, "took="); ok {
			d, err := time.ParseDuration(v)
			return d, err == nil
		}
	}
	return 0, false
}

type saveRequest struct {
	at   time.Time
	next time.Time
}

// requestSaveAt requests a save ahead of the next tick on the calibrated
// cycle's period that leaves room for the request to land, and reports when
// that tick's save starts.
func (rt shutdownRuntime) requestSaveAt(t *testing.T, cycleStart time.Time) saveRequest {
	t.Helper()
	next := cycleStart
	for time.Until(next) < scheduleLead {
		next = next.Add(daemonTickPeriod)
	}
	at := time.Now()
	if err := state.TouchSaveRequested(rt.stateDir); err != nil {
		t.Fatalf("request a save: %v", err)
	}
	return saveRequest{at: at, next: next}
}

func sleepUntil(when time.Time) {
	if d := time.Until(when); d > 0 {
		time.Sleep(d)
	}
}

type saveLanding string

const (
	landedUnshown  saveLanding = "not shown to have reached the save"
	landedInFlight saveLanding = "landed while the save was in flight"
	landedAfter    saveLanding = "landed after the save completed"
)

// classifyLanding reads where the exit fell against the requested save. A
// save that started consumed save.requested and either logged a failure or
// was cancelled; one that finished logged its cycle summary.
func (rt shutdownRuntime) classifyLanding(requestedAt time.Time) saveLanding {
	var completed, failed bool
	for _, line := range portalLogLines(rt.stateDir) {
		parsed, ok := log.ParseLogLine(line)
		if !ok || parsed.Time.Before(requestedAt) {
			continue
		}
		switch {
		case parsed.Component == "capture" && parsed.Message == "tick complete":
			completed = true
		case parsed.Level == "WARN" && strings.HasPrefix(parsed.Message, "tick "):
			failed = true
		}
	}
	_, statErr := os.Stat(state.SaveRequested(rt.stateDir))
	consumed := errors.Is(statErr, fs.ErrNotExist)
	switch {
	case completed:
		return landedAfter
	case failed || consumed:
		return landedInFlight
	default:
		return landedUnshown
	}
}

func portalLogLines(stateDir string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(portaltest.ReadPortalLogSafe(stateDir)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}
