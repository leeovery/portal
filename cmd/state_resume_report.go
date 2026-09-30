package cmd

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/tmux"
)

// The report rows a refused answer puts on the waiting pane. Each names the act
// before its cause, so the one-line truncation only ever cuts the cause's end;
// the whole error chain goes to the log instead.
const (
	resumeReportFallback   = "can't carry out that answer"
	resumeReportLocate     = "can't locate hooks.json: "
	resumeReportRead       = "can't read hooks.json: "
	resumeReportMalformed  = resumeReportRead + "malformed JSON"
	resumeReportLock       = "can't lock hooks.json: "
	resumeReportLockHeld   = resumeReportLock + "another process holds it"
	resumeReportWrite      = "can't write hooks.json: "
	resumeReportUnpause    = "can't unpause this pane: "
	resumeReportTmuxAbsent = resumeReportUnpause + "tmux could not be run"
	resumeReportDrop       = "can't clear pending input: "
)

// hookStoreUnlocatedError marks a store whose path could not be resolved,
// keeping the resolution's own message as its text.
type hookStoreUnlocatedError struct{ err error }

func (e hookStoreUnlocatedError) Error() string { return e.err.Error() }
func (e hookStoreUnlocatedError) Unwrap() error { return e.err }

func isHookStoreUnlocated(err error) bool {
	_, ok := errors.AsType[hookStoreUnlocatedError](err)
	return ok
}

// resumeDiscardRefusal is the confirmation's row for a removal the store would
// not carry out.
func resumeDiscardRefusal(err error) string {
	switch {
	case isHookStoreUnlocated(err):
		return resumeReportLocate + causeWords(err)
	case errors.Is(err, hooks.ErrLockHeld):
		return resumeReportLockHeld
	case errors.Is(err, hooks.ErrLockFailed):
		return resumeReportLock + causeWords(err)
	case errors.Is(err, hooks.ErrMalformed):
		return resumeReportMalformed
	case errors.Is(err, hooks.ErrStoreRead):
		return resumeReportRead + causeWords(err)
	case fileutil.IsWriteFailure(err):
		return resumeReportWrite + causeWords(err)
	default:
		return resumeReportFallback
	}
}

// resumeClearRefusal is the panel's row for a pending-marker clear tmux did not
// carry out: tmux's own words when it answered, and none of the argv the
// client's error leads with.
func resumeClearRefusal(err error) string {
	cmdErr, ok := errors.AsType[*tmux.CommandError](err)
	if !ok {
		return resumeReportFallback
	}
	if stderr := strings.TrimSpace(cmdErr.Stderr); stderr != "" {
		return resumeReportUnpause + stderr
	}
	if _, notRun := errors.AsType[*exec.Error](cmdErr); notRun {
		return resumeReportTmuxAbsent
	}
	return resumeReportUnpause + causeWords(cmdErr)
}

func resumeDropRefusal(err error) string {
	return resumeReportDrop + causeWords(err)
}

// causeWords is the operating system's own message where the chain carries one,
// else the message at the bottom of the chain — never a path or a wrapping
// clause above it. A joined error is followed down its last branch, which is
// where a "%w: %w" puts the cause.
func causeWords(err error) string {
	if errno, ok := errors.AsType[syscall.Errno](err); ok {
		return errno.Error()
	}
	for {
		var next error
		switch e := err.(type) {
		case interface{ Unwrap() error }:
			next = e.Unwrap()
		case interface{ Unwrap() []error }:
			if errs := e.Unwrap(); len(errs) > 0 {
				next = errs[len(errs)-1]
			}
		}
		if next == nil {
			return err.Error()
		}
		err = next
	}
}
