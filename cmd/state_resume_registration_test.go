package cmd

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resumemode"
)

const registrationHookKey = "tok123"

func lookupReturning(result hooks.OnResume, err error) func(string) (hooks.OnResume, error) {
	return func(string) (hooks.OnResume, error) { return result, err }
}

func assertLookupDebug(t *testing.T, sink *logtest.Sink, wantResult string) {
	t.Helper()
	rec := sink.Records().WithMessage("hook lookup").Only(t, "the lookup's DEBUG")
	if rec.Level != slog.LevelDebug {
		t.Errorf("hook lookup level = %v, want %v", rec.Level, slog.LevelDebug)
	}
	if got := rec.AttrOrEmpty("hook_key"); got != registrationHookKey {
		t.Errorf("hook lookup hook_key = %q, want %q", got, registrationHookKey)
	}
	if got := rec.AttrOrEmpty("result"); got != wantResult {
		t.Errorf("hook lookup result = %q, want %q", got, wantResult)
	}
}

func TestResumeRegistrationOrLog(t *testing.T) {
	// The production lookup answers a stored value carrying no command with the
	// zero result, so only a fake can hand this rule a registration found with
	// an empty command. The branch is kept so no caller can compose
	// sh -c "; exec $SHELL" if that ever stops being true.
	t.Run("it reads a registration carrying an empty command as no hook", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)

		got := resumeRegistrationOrLog(logger, lookupReturning(hooks.OnResume{Found: true}, nil), registrationHookKey)

		if got != (hooks.OnResume{}) {
			t.Errorf("registration = %+v, want the zero value", got)
		}
		assertLookupDebug(t, sink, "miss")
		if recs := sink.Records().AtOrAboveLevel(slog.LevelWarn); len(recs) != 0 {
			t.Errorf("records at or above WARN = %d, want none: %q", len(recs), sink.Body())
		}
	})

	t.Run("it records a read failure as a DEBUG and a WARN and answers no hook", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		lookupErr := errors.New("is a directory")

		got := resumeRegistrationOrLog(logger, lookupReturning(hooks.OnResume{}, lookupErr), registrationHookKey)

		if got != (hooks.OnResume{}) {
			t.Errorf("registration = %+v, want the zero value", got)
		}
		assertLookupDebug(t, sink, "error")
		debug := sink.Records().WithMessage("hook lookup").Only(t, "the lookup's DEBUG")
		if err := debug.ErrorAttr(t, "error"); !errors.Is(err, lookupErr) {
			t.Errorf("hook lookup error = %v, want %v", err, lookupErr)
		}

		warn := sink.Records().WithMessage("lookup on-resume hook failed").Only(t, "the failed lookup's WARN")
		if warn.Level != slog.LevelWarn {
			t.Errorf("failed lookup level = %v, want %v", warn.Level, slog.LevelWarn)
		}
		if got := warn.AttrOrEmpty("hook_key"); got != registrationHookKey {
			t.Errorf("WARN hook_key = %q, want %q", got, registrationHookKey)
		}
		if err := warn.ErrorAttr(t, "error"); !errors.Is(err, lookupErr) {
			t.Errorf("WARN error = %v, want %v", err, lookupErr)
		}
	})

	t.Run("it records a miss as a DEBUG alone", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)

		got := resumeRegistrationOrLog(logger, lookupReturning(hooks.OnResume{}, nil), registrationHookKey)

		if got != (hooks.OnResume{}) {
			t.Errorf("registration = %+v, want the zero value", got)
		}
		assertLookupDebug(t, sink, "miss")
		if recs := sink.Records().AtOrAboveLevel(slog.LevelWarn); len(recs) != 0 {
			t.Errorf("records at or above WARN = %d, want none: %q", len(recs), sink.Body())
		}
	})

	t.Run("it answers a hit with the whole registration and one DEBUG", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		want := hooks.OnResume{Command: "make deploy", Mode: resumemode.Lazy, Found: true}

		got := resumeRegistrationOrLog(logger, lookupReturning(want, nil), registrationHookKey)

		if got != want {
			t.Errorf("registration = %+v, want %+v", got, want)
		}
		assertLookupDebug(t, sink, "hit")
	})
}
