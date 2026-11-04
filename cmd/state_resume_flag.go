package cmd

import "github.com/leeovery/portal/internal/state"

// The waiting session's resume-pending flag, in the state directory as each
// process of the chain resolves it. Raised, it keeps the saver from capturing
// the panel over the session's transcript.

// raiseResumePending raises session's flag and returns its path, which the
// parked chain's shell tests and removes once no Portal binary is left to.
func raiseResumePending(session string) (string, error) {
	dir, err := state.Dir()
	if err != nil {
		return "", err
	}
	path, err := state.ResumePending.Path(dir, session)
	if err != nil {
		return "", err
	}
	if err := state.ResumePending.Set(dir, session); err != nil {
		return "", err
	}
	return path, nil
}

// clearResumePending lowers session's flag. Lowering a flag already lowered
// succeeds: the session is as the answer wants it.
func clearResumePending(session string) error {
	dir, err := state.Dir()
	if err != nil {
		return err
	}
	_, err = state.ResumePending.Clear(dir, session)
	return err
}

// resumePendingRaised reports whether session's flag is raised. A flag that
// can't be looked for is an error, which the chain counts as raised.
func resumePendingRaised(session string) (bool, error) {
	dir, err := state.Dir()
	if err != nil {
		return false, err
	}
	return state.ResumePending.Exists(dir, session)
}
