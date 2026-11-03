package cmd

import "errors"

// IsSilentExitError reports whether the top-level handler must suppress err's
// stderr emission: the command has said all it has to say, or has nowhere to
// say it, and the exit code alone answers.
func IsSilentExitError(err error) bool {
	return errors.Is(err, errCommitNowFailed) ||
		errors.Is(err, ErrDoctorUnhealthy) ||
		errors.Is(err, errNoCurrentSession)
}
