package bootstrap

import (
	"errors"
	"testing"
)

func TestNewFatal_carriesUserMessage(t *testing.T) {
	cause := errors.New("underlying boom")
	fatal := NewFatal("Portal failed to start tmux server: underlying boom", cause)

	if fatal == nil {
		t.Fatal("NewFatal returned nil")
	}
	if fatal.UserMessage != "Portal failed to start tmux server: underlying boom" {
		t.Errorf("UserMessage = %q, want %q", fatal.UserMessage, "Portal failed to start tmux server: underlying boom")
	}
	if fatal.Cause != cause {
		t.Errorf("Cause = %v, want %v", fatal.Cause, cause)
	}
}

func TestFatalError_ErrorReturnsUserMessage(t *testing.T) {
	fatal := &FatalError{
		UserMessage: "Portal failed to register tmux hooks: x",
		Cause:       errors.New("x"),
	}

	if got := fatal.Error(); got != "Portal failed to register tmux hooks: x" {
		t.Errorf("Error() = %q, want %q", got, "Portal failed to register tmux hooks: x")
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
		"Portal state file is corrupt — restoration skipped.",
		"Check `portal state status` or ~/.config/portal/state/portal.log.",
	}
	if len(got.Lines) != len(want) {
		t.Fatalf("Lines len = %d, want %d; got %#v", len(got.Lines), len(want), got.Lines)
	}
	for i := range want {
		if got.Lines[i] != want[i] {
			t.Errorf("Lines[%d] = %q, want %q", i, got.Lines[i], want[i])
		}
	}
}

func TestSaverDownWarning_returnsExactSpecCopy(t *testing.T) {
	got := SaverDownWarning()
	want := []string{
		"Portal save daemon failed to start — sessions won't be captured.",
		"Run `portal state status` for details.",
	}
	if len(got.Lines) != len(want) {
		t.Fatalf("Lines len = %d, want %d; got %#v", len(got.Lines), len(want), got.Lines)
	}
	for i := range want {
		if got.Lines[i] != want[i] {
			t.Errorf("Lines[%d] = %q, want %q", i, got.Lines[i], want[i])
		}
	}
}
