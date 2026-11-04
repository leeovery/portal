package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/devversion"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
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

func checkSaver(ctx context.Context, s doctorSaver) checkResult {
	const name = "saver"
	if s == nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: saverReadPhrase}
	}
	present, err := s.Present(ctx)
	switch {
	case err != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: saverReadPhrase + ": " + err.Error()}
	case !present:
		return checkResult{name: name, status: checkFail, detail: saver.SessionName + " not running — run portal open to start it"}
	}
	return checkResult{name: name, status: checkPass, detail: saver.SessionName + " up"}
}

// checkDaemon judges the daemon alive when daemon.pid names the process the
// saver session runs, and Prove, reading the process table, finds that
// process neither gone nor another program's. By the backend's word it is the
// session's live process, so a proof that can't be read leaves that judgement
// standing. A daemon recording another version, or none, still runs and
// saves; the next bootstrap replaces it, so it is reported, never failed.
func checkDaemon(ctx context.Context, s doctorSaver, dir string, dirErr error, version string) checkResult {
	const name = "daemon"
	if s == nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: saverReadPhrase}
	}
	if dirErr != nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: stateDirStandDownPhrase}
	}
	running, present, err := s.SaverPID(ctx)
	switch {
	case err != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: saverReadPhrase + ": " + err.Error()}
	case !present:
		return checkResult{name: name, status: checkFail, detail: "not running (no " + saver.SessionName + ")"}
	}
	pid, err := state.ReadPIDFile(dir)
	switch {
	case errors.Is(err, state.ErrPIDFileAbsent):
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("not running (no daemon.pid; %s runs pid %d)", saver.SessionName, running)}
	case err != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not read daemon.pid"}
	case pid != running:
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("daemon.pid names pid %d, but %s runs pid %d", pid, saver.SessionName, running)}
	}
	switch err := s.Prove(pid); {
	case errors.Is(err, saverhost.ErrGone):
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("not running (pid %d has exited)", pid)}
	case errors.Is(err, saverhost.ErrNotSaverDaemon):
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("pid %d, which %s runs, isn't Portal's save daemon", pid, saver.SessionName)}
	}
	return daemonVersionResult(name, pid, dir, version)
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
