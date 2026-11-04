package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/devversion"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// doctorSaver is what doctor reads of the saver, and all it may: whether the
// saver session is there, the process it runs, and a proof, read from the
// process table, that a pid is its daemon.
type doctorSaver interface {
	Present(ctx context.Context) (bool, error)
	SaverPID(ctx context.Context) (pid int, present bool, err error)
	Prove(pid int) error
}

// readOnlySaver is the saver doctor reads over the driver bound on its zmx
// dir and its state dir, through a host that signals nothing. It is nil when
// either can't be had, and the saver's checks are then not evaluable.
func readOnlySaver(deps *DoctorDeps) doctorSaver {
	dir, err := doctorStateDir(deps)
	if err != nil {
		return nil
	}
	driver, err := bindZmxAt(zmxProgram, deps.ZmxDir)
	if err != nil {
		return nil
	}
	host, err := newReadOnlySaverHost(driver, dir)
	if err != nil {
		return nil
	}
	return host
}

// saverReadPhrase words a saver doctor could not read.
const saverReadPhrase = "could not read " + saver.SessionName

// nothingToSave reports an install with nothing yet for a saver to save: no
// live session but the saver's own, and no entry in sessions.json. A listing
// or an index doctor can't read shows no such thing, so it reports false.
func nothingToSave(sessions []backend.Session, listErr error, dir string, dirErr error) bool {
	if listErr != nil || dirErr != nil {
		return false
	}
	for _, s := range sessions {
		if s.Name != saver.SessionName {
			return false
		}
	}
	idx, err := sessionindex.Read(dir)
	return err == nil && idx.Len() == 0
}

// missingSaver is a check of a saver that isn't running: info while there is
// nothing to save, saying so, and a failure, saying failed, otherwise.
func missingSaver(name string, nothingToSave bool, idle, failed string) checkResult {
	if nothingToSave {
		return checkResult{name: name, status: checkInfo, detail: idle}
	}
	return checkResult{name: name, status: checkFail, detail: failed}
}

func checkSaver(ctx context.Context, s doctorSaver, nothingToSave bool) checkResult {
	const name = "saver"
	if s == nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: saverReadPhrase}
	}
	present, err := s.Present(ctx)
	switch {
	case err != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: saverReadPhrase + ": " + err.Error()}
	case !present:
		return missingSaver(name, nothingToSave,
			saver.SessionName+" not running — nothing to save yet",
			saver.SessionName+" not running — run portal open to start it")
	}
	return checkResult{name: name, status: checkPass, detail: saver.SessionName + " up"}
}

// checkDaemon judges the daemon alive when daemon.pid names the process the
// saver session runs and Prove, reading the process table, finds that process
// Portal's save daemon. A daemon still starting is waited for up to settle,
// as the saver's own ensure waits, so one on its way up isn't failed. A
// daemon recording another version, or none, still runs and saves; the next
// bootstrap replaces it, so it is reported, never failed. With no saver, it
// reads as checkSaver does.
func checkDaemon(ctx context.Context, s doctorSaver, dir string, dirErr error, version string, settle time.Duration, nothingToSave bool) checkResult {
	const name = "daemon"
	if s == nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: saverReadPhrase}
	}
	if dirErr != nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: stateDirStandDownPhrase}
	}
	r := settledDaemon(ctx, s, dir, settle)
	switch {
	case r.saverErr != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: saverReadPhrase + ": " + r.saverErr.Error()}
	case !r.present:
		return missingSaver(name, nothingToSave,
			"not running (no "+saver.SessionName+"; nothing to save yet)",
			"not running (no "+saver.SessionName+")")
	case errors.Is(r.pidErr, state.ErrPIDFileAbsent):
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("not running (no daemon.pid; %s runs pid %d)", saver.SessionName, r.running)}
	case r.pidErr != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not read daemon.pid"}
	case r.pid != r.running:
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("daemon.pid names pid %d, but %s runs pid %d", r.pid, saver.SessionName, r.running)}
	}
	switch err := s.Prove(r.pid); {
	case errors.Is(err, saverhost.ErrGone):
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("not running (pid %d has exited)", r.pid)}
	case errors.Is(err, saverhost.ErrNotSaverDaemon):
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("pid %d, which %s runs, isn't Portal's save daemon", r.pid, saver.SessionName)}
	case err != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: fmt.Sprintf("could not read pid %d to prove it Portal's save daemon", r.pid)}
	}
	return daemonVersionResult(name, r.pid, dir, version)
}

// daemonSettlePoll spaces the reads of a daemon still starting, as the
// saver's own waits space theirs.
const daemonSettlePoll = 50 * time.Millisecond

// daemonReading is one read of the process the saver session runs and of
// daemon.pid, which is read only while there is a session.
type daemonReading struct {
	running  int
	present  bool
	saverErr error
	pid      int
	pidErr   error
}

func readDaemon(ctx context.Context, s doctorSaver, dir string) daemonReading {
	var r daemonReading
	r.running, r.present, r.saverErr = s.SaverPID(ctx)
	if r.saverErr == nil && r.present {
		r.pid, r.pidErr = state.ReadPIDFile(dir)
	}
	return r
}

// starting reports a saver session whose daemon has yet to record itself:
// daemon.pid absent, or naming a process the session doesn't run.
func (r daemonReading) starting() bool {
	if r.saverErr != nil || !r.present {
		return false
	}
	return errors.Is(r.pidErr, state.ErrPIDFileAbsent) || (r.pidErr == nil && r.pid != r.running)
}

// settledDaemon reads the daemon, then, only while it is starting, again
// until it isn't or settle has passed, and returns the last reading.
func settledDaemon(ctx context.Context, s doctorSaver, dir string, settle time.Duration) daemonReading {
	r := readDaemon(ctx, s, dir)
	if !r.starting() {
		return r
	}
	deadline := time.NewTimer(settle)
	defer deadline.Stop()
	tick := time.NewTicker(daemonSettlePoll)
	defer tick.Stop()
	for r.starting() {
		select {
		case <-ctx.Done():
			return r
		case <-deadline.C:
			return r
		case <-tick.C:
			r = readDaemon(ctx, s, dir)
		}
	}
	return r
}

// daemonVersionResult passes a daemon recording this Portal's version, a
// build it can be told by: an unidentified build's names none.
func daemonVersionResult(name string, pid int, dir, version string) checkResult {
	recorded, err := state.ReadVersionFile(dir)
	if err != nil || recorded == "" {
		recorded = "unknown"
	}
	running := fmt.Sprintf("running (pid %d, version %s)", pid, recorded)
	switch {
	case version == devversion.Unidentified:
		return checkResult{name: name, status: checkInfo, detail: running + "; this Portal's build is unidentified"}
	case recorded != version:
		return checkResult{name: name, status: checkInfo, detail: running + "; this Portal is " + version}
	}
	return checkResult{name: name, status: checkPass, detail: running}
}

// checkPendingResumes counts the live sessions waiting on their resume
// panel. It states, never judges.
func checkPendingResumes(dir string, dirErr error, sessions []backend.Session, listErr error) checkResult {
	const name = "pending resumes"
	switch {
	case dirErr != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: stateDirStandDownPhrase}
	case listErr != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: listStandDownPhrase}
	}
	waiting, err := waitingSessions(dir, sessions)
	if err != nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not read the resume-pending flags"}
	}
	return checkResult{name: name, status: checkInfo, detail: pluralCount(waiting, "session waiting to resume", "sessions waiting to resume")}
}

// waitingSessions counts the sessions listed in sessions whose resume panel
// waits, as the state directory dir's flags say.
func waitingSessions(dir string, sessions []backend.Session) (int, error) {
	pending, err := pendingResumeFlags{dir: dir}.ListPendingResumeSessions()
	if err != nil {
		return 0, err
	}
	waiting := 0
	for _, s := range sessions {
		if _, ok := pending[s.Name]; ok {
			waiting++
		}
	}
	return waiting, nil
}
