package tmux_test

// Show-hooks failure-log shape for the unified per-event convergence path.
// Each per-event read goes through c.ShowGlobalHooksForEvent(event); on a read
// failure convergeEvent wraps "show-hooks failed: %w" and emits the uniform
// WARN shape:
//
//	Warn("show-hooks failed", "error", <wrapped err>, "error_class", "unexpected")
//
// rendered under component=bootstrap, emitted BEFORE the per-event return, and
// exactly once per failing event (the errors.Join fold adds no aggregate
// double-log). The legacy migration helpers (migrateHydrationHooks /
// migrateSessionClosedHook / RegisterHookIfAbsent) that previously owned this
// WARN have been deleted; their coverage now lives entirely on the
// RegisterPortalHooks convergence path exercised below.

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

const showHooksWarnMessage = "show-hooks failed"

// showHooksWarnRecords filters captured records for the show-hooks WARN.
func showHooksWarnRecords(recs []slog.Record) []slog.Record {
	var out []slog.Record
	for _, r := range recs {
		if r.Level == slog.LevelWarn && r.Message == showHooksWarnMessage {
			out = append(out, r)
		}
	}
	return out
}

// assertShowHooksWarnShape verifies the uniform WARN shape on a single record:
// component=bootstrap, error_class=unexpected, and an "error" attr carrying the
// wrapped error value (asserted via the supplied errors.Is/As checks against
// the captured attr value).
func assertShowHooksWarnShape(t *testing.T, rec slog.Record, wantErr error) {
	t.Helper()
	var gotComponent, gotErrorClass string
	var gotErr error
	var sawError, sawErrorClass bool
	rec.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "component":
			gotComponent = a.Value.String()
		case "error_class":
			gotErrorClass = a.Value.String()
			sawErrorClass = true
		case "error":
			sawError = true
			if e, ok := a.Value.Any().(error); ok {
				gotErr = e
			}
		}
		return true
	})
	if gotComponent != "bootstrap" {
		t.Errorf("WARN component = %q, want %q", gotComponent, "bootstrap")
	}
	if !sawErrorClass {
		t.Fatalf("WARN missing error_class attr: %v", rec)
	}
	if gotErrorClass != "unexpected" {
		t.Errorf("WARN error_class = %q, want %q", gotErrorClass, "unexpected")
	}
	if !sawError {
		t.Fatalf("WARN missing error attr: %v", rec)
	}
	if gotErr == nil {
		t.Fatalf("WARN error attr is not an error value (was passed .Error()?): %v", rec)
	}
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("WARN error attr %v does not wrap expected error %v", gotErr, wantErr)
	}
}

// TestRegisterPortalHooks_HydrationReadFailureEmitsCanonicalWarn pins that the
// previously silent migrateHydrationHooks branch (deleted; its coverage now
// lives on the convergence path) emits the same WARN before returning the
// wrapped err. The hydration convergence runs inside RegisterPortalHooks; the
// injected logger is the WARN sink (production passes log.For("bootstrap")).
func TestRegisterPortalHooks_HydrationReadFailureEmitsCanonicalWarn(t *testing.T) {
	sentinel := errors.New("tmux show-hooks failure (hydration)")
	mock := &MockCommander{
		RunFunc: perEventDispatchWithFaults(t, "", nil, readErrForAllManagedEvents(sentinel), nil),
	}
	client := tmux.NewClient(mock)

	rec := &recordingSlogHandler{}
	err := tmux.RegisterPortalHooks(client, slog.New(rec).With("component", "bootstrap"))

	// The migration's show-hooks failure must still surface as a returned
	// (aggregate) error wrapping the sentinel.
	if err == nil {
		t.Fatal("expected error from RegisterPortalHooks, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}

	// No set-hook may be dispatched when every per-event read fails.
	assertNoSetHookCalls(t, mock.Calls)

	// The hydration event's convergence is the FIRST show-hooks call. Assert at
	// least one WARN carries the uniform shape with the sentinel reachable.
	warns := showHooksWarnRecords(rec.records)
	if len(warns) == 0 {
		t.Fatalf("expected at least one %q WARN, got none: %v", showHooksWarnMessage, rec.records)
	}
	assertShowHooksWarnShape(t, warns[0], sentinel)
}

// TestRegisterPortalHooks_SessionClosedReadFailureEmitsCanonicalWarn pins that the
// session-closed convergence emits the uniform WARN (message "show-hooks
// failed", error_class=unexpected, error attr = the wrapped error) when its
// per-event ShowGlobalHooksForEvent read fails, and skips appending
// session-closed. The convergence engine now reads each event independently,
// so the failure is scoped to the single failing event's read.
func TestRegisterPortalHooks_SessionClosedReadFailureEmitsCanonicalWarn(t *testing.T) {
	// Fail only the per-event read for session-closed.
	sentinel := errors.New("tmux show-hooks failure (session-closed)")
	mock := &MockCommander{RunFunc: perEventDispatchWithFaults(t, "", nil,
		map[string]error{"session-closed": sentinel}, nil)}
	client := tmux.NewClient(mock)

	rec := &recordingSlogHandler{}
	err := tmux.RegisterPortalHooks(client, slog.New(rec).With("component", "bootstrap"))

	if err == nil {
		t.Fatal("expected aggregate error wrapping the session-closed sentinel, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}

	warns := showHooksWarnRecords(rec.records)
	if len(warns) != 1 {
		t.Fatalf("expected exactly 1 %q WARN (session-closed), got %d: %v", showHooksWarnMessage, len(warns), rec.records)
	}
	assertShowHooksWarnShape(t, warns[0], sentinel)

	// session-closed must NOT have been appended (its convergence was skipped).
	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "session-closed" {
			t.Errorf("session-closed must not be appended when its read fails: %v", c)
		}
	}
}

// TestShowHooksWarn_ErrorAttrCarriesCommandErrorChain pins that the error attr
// is the WRAPPED error value (not .Error()), so the underlying *CommandError
// (carrying tmux argv + stderr per Task 4-2) is reachable via errors.As on the
// captured attr value. Drives the unified RegisterPortalHooks convergence path
// with a per-event reader that returns a *CommandError.
func TestShowHooksWarn_ErrorAttrCarriesCommandErrorChain(t *testing.T) {
	cmdErr := &tmux.CommandError{
		Stderr: "no server running on /tmp/tmux-1000/default",
		Err:    errors.New("exit status 1"),
		Args:   []string{"show-hooks", "-g", "session-created"},
	}
	mock := &MockCommander{
		RunFunc: perEventDispatchWithFaults(t, "", nil,
			map[string]error{"session-created": cmdErr}, nil),
	}
	client := tmux.NewClient(mock)

	rec := &recordingSlogHandler{}
	if err := tmux.RegisterPortalHooks(client, slog.New(rec).With("component", "bootstrap")); err == nil {
		t.Fatal("expected error, got nil")
	}

	warns := showHooksWarnRecords(rec.records)
	if len(warns) != 1 {
		t.Fatalf("expected exactly 1 %q WARN (only session-created's read fails), got %d: %v", showHooksWarnMessage, len(warns), rec.records)
	}

	var gotErr error
	warns[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "error" {
			if e, ok := a.Value.Any().(error); ok {
				gotErr = e
			}
		}
		return true
	})
	if gotErr == nil {
		t.Fatal("WARN error attr is not an error value (was passed .Error()?)")
	}
	var asCmdErr *tmux.CommandError
	if !errors.As(gotErr, &asCmdErr) {
		t.Fatalf("WARN error attr %v does not unwrap to *tmux.CommandError", gotErr)
	}
	if asCmdErr.Stderr != cmdErr.Stderr {
		t.Errorf("recovered CommandError.Stderr = %q, want %q", asCmdErr.Stderr, cmdErr.Stderr)
	}
}

// TestRegisterPortalHooks_ShowHooksFailureLoggedExactlyOnce pins the no-double-log
// invariant: when EVERY per-event read fails, each event's failure is logged
// exactly once (one WARN per managed event), and RegisterPortalHooks adds no
// extra aggregate WARN for the errors.Join folding.
//
// The convergence engine reads each event via ShowGlobalHooksForEvent and emits
// the WARN through the injected logger; the recorder is installed via the
// injected logger built over the same handler.
func TestRegisterPortalHooks_ShowHooksFailureLoggedExactlyOnce(t *testing.T) {
	sentinel := errors.New("tmux show-hooks fails everywhere")
	mock := &MockCommander{
		RunFunc: perEventDispatchWithFaults(t, "", nil, readErrForAllManagedEvents(sentinel), nil),
	}
	client := tmux.NewClient(mock)

	rec := &recordingSlogHandler{}
	injected := slog.New(rec).With("component", "bootstrap")

	err := tmux.RegisterPortalHooks(client, injected)
	if err == nil {
		t.Fatal("expected aggregate error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("aggregate error %v does not wrap sentinel %v", err, sentinel)
	}

	// No set-hook may be dispatched when every per-event read fails.
	assertNoSetHookCalls(t, mock.Calls)

	// One WARN per managed event whose per-event read failed: every event in
	// managedEvents fails once. No aggregate WARN from RegisterPortalHooks.
	wantSiblingFailures := expectedManagedEventCount
	warns := showHooksWarnRecords(rec.records)
	if len(warns) != wantSiblingFailures {
		t.Fatalf("expected exactly %d %q WARNs (one per managed event, no aggregate double-log), got %d: %v",
			wantSiblingFailures, showHooksWarnMessage, len(warns), rec.records)
	}
	for i, w := range warns {
		t.Run("warn-"+string(rune('0'+i)), func(t *testing.T) {
			assertShowHooksWarnShape(t, w, sentinel)
		})
	}
}
