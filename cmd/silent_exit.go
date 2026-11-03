package cmd

import (
	"errors"
	"strconv"
)

// IsSilentExitError reports whether the top-level handler must suppress err's
// stderr emission: the command has said all it has to say, or has nowhere to
// say it, and the exit code alone answers.
func IsSilentExitError(err error) bool {
	_, isExitCode := errors.AsType[*ExitCodeError](err)
	return isExitCode ||
		errors.Is(err, errCommitNowFailed) ||
		errors.Is(err, ErrDoctorUnhealthy) ||
		errors.Is(err, errNoCurrentSession)
}

// ExitCodeError ends Portal with Code, and says nothing: the status an attach
// that no key of Portal's ended hands the shell.
type ExitCodeError struct {
	Code int
}

func (e *ExitCodeError) Error() string {
	return "exit status " + strconv.Itoa(e.Code)
}
