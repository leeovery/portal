package bootstrap

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewFatal_carriesUserMessage(t *testing.T) {
	cause := errors.New("underlying boom")
	fatal := NewFatal("Portal failed to raise the restoring flag: underlying boom", cause)

	if fatal == nil {
		t.Fatal("NewFatal returned nil")
	}
	if fatal.UserMessage != "Portal failed to raise the restoring flag: underlying boom" {
		t.Errorf("UserMessage = %q, want %q", fatal.UserMessage, "Portal failed to raise the restoring flag: underlying boom")
	}
	if fatal.Cause != cause {
		t.Errorf("Cause = %v, want %v", fatal.Cause, cause)
	}
	if fatal.Step != 0 {
		t.Errorf("Step = %d, want 0 until a step claims it", fatal.Step)
	}
}

func TestFatalError_ErrorReturnsUserMessage(t *testing.T) {
	fatal := &FatalError{
		UserMessage: "Portal failed to lower the restoring flag: x",
		Cause:       errors.New("x"),
	}

	if got := fatal.Error(); got != "Portal failed to lower the restoring flag: x" {
		t.Errorf("Error() = %q, want %q", got, "Portal failed to lower the restoring flag: x")
	}
}

func TestFatalError_UnwrapReturnsCause(t *testing.T) {
	cause := errors.New("root cause")
	fatal := &FatalError{UserMessage: "wrapped", Cause: cause}

	if got := fatal.Unwrap(); got != cause {
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}
	if !errors.Is(fatal, cause) {
		t.Error("errors.Is(fatal, cause) = false, want true")
	}
}

func TestFatalError_UnwrapNilCause(t *testing.T) {
	fatal := &FatalError{UserMessage: "no cause"}

	if got := fatal.Unwrap(); got != nil {
		t.Errorf("Unwrap() = %v, want nil", got)
	}
}

func TestOtherStateDir(t *testing.T) {
	t.Run("a saver another state directory's is the fatal naming both, carrying it", func(t *testing.T) {
		err := fmt.Errorf("create the saver: %w", theirs)

		fatal := OtherStateDir(err)

		if fatal == nil || fatal.UserMessage != theirsMessage {
			t.Fatalf("OtherStateDir = %v, want %q", fatal, theirsMessage)
		}
		if !errors.Is(fatal, theirs) {
			t.Errorf("cause = %v, want it to carry the saver's error", fatal.Cause)
		}
	})

	for name, err := range map[string]error{"none": nil, "any other failure": errors.New("pid 4242 isn't confirmed as the save daemon")} {
		t.Run(name+" is no fatal", func(t *testing.T) {
			if fatal := OtherStateDir(err); fatal != nil {
				t.Errorf("OtherStateDir(%v) = %v, want nil", err, fatal)
			}
		})
	}
}

func TestFatalError_AsExtractsType(t *testing.T) {
	original := NewFatal("user-facing", errors.New("cause"))
	var wrapped error = original

	var got *FatalError
	if !errors.As(wrapped, &got) {
		t.Fatal("errors.As(wrapped, &FatalError) = false, want true")
	}
	if got != original {
		t.Errorf("got = %v, want %v", got, original)
	}
}

func TestCorruptSessionsJSONWarning_returnsExactSpecCopy(t *testing.T) {
	got := CorruptSessionsJSONWarning()
	want := []string{
		"Portal state file unusable — restoration skipped.",
		"Check `portal doctor` or ~/.config/portal/state/portal.log.",
	}
	if len(got.Lines) != len(want) {
		t.Fatalf("Lines len = %d, want %d; got %#v", len(got.Lines), len(want), got.Lines)
	}
	for i := range want {
		if got.Lines[i] != want[i] {
			t.Errorf("Lines[%d] = %q, want %q", i, got.Lines[i], want[i])
		}
	}
	assertRepointedToDoctor(t, got)
}

func TestSaverDownWarning_returnsExactSpecCopy(t *testing.T) {
	got := SaverDownWarning()
	want := []string{
		"Portal save daemon failed to start — sessions won't be captured.",
		"Run `portal doctor` for details.",
	}
	if len(got.Lines) != len(want) {
		t.Fatalf("Lines len = %d, want %d; got %#v", len(got.Lines), len(want), got.Lines)
	}
	for i := range want {
		if got.Lines[i] != want[i] {
			t.Errorf("Lines[%d] = %q, want %q", i, got.Lines[i], want[i])
		}
	}
	assertRepointedToDoctor(t, got)
}

func TestSessionsUnlistedWarning_returnsExactCopy(t *testing.T) {
	assertLines(t, SessionsUnlistedWarning(), []string{
		"Portal couldn't list zmx's sessions — restoration skipped; it will try again on the next command.",
		"Run `portal doctor` for details.",
	})
}

func TestWarnings_returnExactCopy(t *testing.T) {
	for name, tc := range map[string]struct {
		got  Warning
		want []string
	}{
		"the saver unread": {SaverUnreadWarning(), []string{
			"Portal couldn't read its save daemon — sessions may not be captured.",
			"Run `portal doctor` for details.",
		}},
		"the old saver not replaced": {SaverNotReplacedWarning(), []string{
			"Portal couldn't replace its save daemon — the old one is still saving.",
			"Run `portal doctor` for details.",
		}},
		"zmx's protocol unjudged": {UnjudgedWarning(), []string{
			"Portal couldn't tell which protocol zmx speaks — restoration skipped; it will try again on the next command.",
			"Run `portal doctor` for details.",
		}},
		"the sessions owed unmarked": {OwedUnmarkedWarning(), []string{
			"Portal couldn't record which sessions to restore — restoration skipped; it will try again on the next command.",
			"Check `portal doctor` or ~/.config/portal/state/portal.log.",
		}},
		"this boot's id unread": {BootIDUnreadWarning(), []string{
			"Portal couldn't read this boot's id — it will start in full on every command.",
			"Check `portal doctor` or ~/.config/portal/state/portal.log.",
		}},
		"the latch unwritten": {LatchUnwrittenWarning(), []string{
			"Portal couldn't record that it has started — it will start in full again on the next command.",
			"Check `portal doctor` or ~/.config/portal/state/portal.log.",
		}},
	} {
		t.Run(name, func(t *testing.T) { assertLines(t, tc.got, tc.want) })
	}
}

func TestUnrestoredWarning_countsTheSessions(t *testing.T) {
	assertLines(t, UnrestoredWarning(1), []string{
		"Portal couldn't restore 1 saved session — kept for the next restore.",
		"Check `portal doctor` or ~/.config/portal/state/portal.log.",
	})
	assertLines(t, UnrestoredWarning(3), []string{
		"Portal couldn't restore 3 saved sessions — kept for the next restore.",
		"Check `portal doctor` or ~/.config/portal/state/portal.log.",
	})
}

func assertLines(t *testing.T, w Warning, want []string) {
	t.Helper()
	if strings.Join(w.Lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("Lines = %q, want %q", w.Lines, want)
	}
	assertRepointedToDoctor(t, w)
}

func assertRepointedToDoctor(t *testing.T, w Warning) {
	t.Helper()
	body := strings.Join(w.Lines, "\n")
	if !strings.Contains(body, "portal doctor") {
		t.Errorf("warning does not mention `portal doctor`; got %q", body)
	}
	if strings.Contains(body, "portal state status") {
		t.Errorf("warning still references removed `portal state status`; got %q", body)
	}
}
