package cmd

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/runnertoken"
)

// The report rows a refused answer puts on the waiting session. Each names the
// act before its cause, so the one-line truncation only ever cuts the cause's
// end; the whole error chain goes to the log instead.
const (
	resumeReportFallback  = "can't carry out that answer"
	resumeReportLocate    = "can't locate hooks.json: "
	resumeReportRead      = "can't read hooks.json: "
	resumeReportMalformed = resumeReportRead + "malformed JSON"
	resumeReportLock      = "can't lock hooks.json: "
	resumeReportLockHeld  = resumeReportLock + "another process holds it"
	resumeReportWrite     = "can't write hooks.json: "
	resumeReportUnpause   = "can't unpause this session: "
	resumeReportDrop      = "can't clear pending input: "
	resumeReportBack      = "can't open the picker: "
	resumeReportDetach    = "can't detach: "
)

// resumeReasonSeparator joins the reasons of several runners that refused.
const resumeReasonSeparator = "; "

// resumeReasonUnanswered is the reason of a runner that never answered.
const resumeReasonUnanswered = "a runner did not answer"

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

// resumeClearRefusal is the panel's row for a resume-pending flag that could
// not be lowered: the operating system's words for why, or the state
// directory's or the session name's when the flag could not even be found.
func resumeClearRefusal(err error) string {
	return resumeReportUnpause + causeWords(err)
}

// resumeBackRefusal is the panel's row for an Escape that sent no terminal
// anywhere: the backend's detach failing with no runner registered, or else
// why no runner on the session took the request.
func resumeBackRefusal(err error) string {
	if _, ok := errors.AsType[detachError](err); ok {
		return resumeReportDetach + causeWords(err)
	}
	return resumeReportBack + runnersWords(err)
}

// runnersWords is why the session's runners took no request: each runner's
// reason once, in their order, or the one cause of a failure met before any
// runner was asked.
func runnersWords(err error) string {
	reasons := joinedErrors(err)
	if reasons == nil {
		return runnerWords(err)
	}
	var words []string
	for _, reason := range reasons {
		if w := runnerWords(reason); !slices.Contains(words, w) {
			words = append(words, w)
		}
	}
	return strings.Join(words, resumeReasonSeparator)
}

// runnerWords is one runner's reason: its refusal in its own words, Portal's
// for one that never answered, or else the cause of the failure to ask it.
func runnerWords(err error) string {
	if unanswered(err) {
		return resumeReasonUnanswered
	}
	for e := err; e != nil; e = errors.Unwrap(e) {
		if errors.Unwrap(e) == runnertoken.ErrRefused {
			return strings.TrimPrefix(e.Error(), runnertoken.ErrRefused.Error()+": ")
		}
	}
	return causeWords(err)
}

// unanswered reports whether err is a request cut by a deadline: the
// runner's own, its connection's or Escape's bound.
func unanswered(err error) bool {
	if _, bound := errors.AsType[backTimeout](err); bound {
		return true
	}
	return errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded)
}

// joinedErrors is the errors joined in err's chain, nil when it joins none.
func joinedErrors(err error) []error {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if joined, ok := e.(interface{ Unwrap() []error }); ok {
			return joined.Unwrap()
		}
	}
	return nil
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
