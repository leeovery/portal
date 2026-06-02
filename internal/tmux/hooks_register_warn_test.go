package tmux_test

// Task 4-5 (portal-observability-layer): close the ShowGlobalHooks
// failure-log asymmetry. Three siblings call c.ShowGlobalHooks() and wrap
// "show-hooks failed: %w" on failure; before this task only
// migrateSessionClosedHook surfaced a WARN, while RegisterHookIfAbsent and
// migrateHydrationHooks were silent. These tests pin the uniform WARN shape:
//
//	Warn("show-hooks failed", "error", <wrapped err>, "error_class", "unexpected")
//
// rendered under component=bootstrap, emitted BEFORE the existing return, and
// exactly once per failure (no errors.Join aggregate double-log).
//
// RegisterHookIfAbsent has no logger param — it uses the package-level
// bootstrapLogger = log.For("bootstrap"), so its WARN is captured via
// log.SetTestHandler. The two migrate helpers receive an injected *slog.Logger,
// so their WARNs are captured via the injected recorder (the production wiring
// passes log.For("bootstrap")).

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/log"
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

// TestRegisterHookIfAbsent_ShowHooksFailureEmitsWarn pins that the previously
// silent RegisterHookIfAbsent now emits exactly one WARN in the uniform shape
// before returning the wrapped error. The WARN routes through the package-level
// bootstrapLogger (log.For("bootstrap")), captured via log.SetTestHandler.
func TestRegisterHookIfAbsent_ShowHooksFailureEmitsWarn(t *testing.T) {
	sentinel := errors.New("tmux show-hooks failure")
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if args[0] == "show-hooks" {
				return "", sentinel
			}
			t.Fatalf("set-hook must not be called when show-hooks fails: %v", args)
			return "", nil
		},
	}
	client := tmux.NewClient(mock)

	rec := &recordingSlogHandler{}
	log.SetTestHandler(t, rec)

	err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify",
		`run-shell 'portal state notify'`)

	// Return/abort behaviour unchanged: still returns the wrapped error.
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}

	warns := showHooksWarnRecords(rec.records)
	if len(warns) != 1 {
		t.Fatalf("expected exactly 1 %q WARN, got %d: %v", showHooksWarnMessage, len(warns), rec.records)
	}
	assertShowHooksWarnShape(t, warns[0], sentinel)
}

// TestMigrateHydrationHooks_ShowHooksFailureEmitsWarn pins that the previously
// silent migrateHydrationHooks branch now emits the same WARN before returning
// (0, wrapped err). The migration is sealed inside RegisterPortalHooks; the
// injected logger is the WARN sink (production passes log.For("bootstrap")).
func TestMigrateHydrationHooks_ShowHooksFailureEmitsWarn(t *testing.T) {
	sentinel := errors.New("tmux show-hooks failure (hydration)")
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
				return "", sentinel
			}
			t.Fatalf("set-hook must not be called when show-hooks fails: %v", args)
			return "", nil
		},
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

	// The migrateHydrationHooks branch is the FIRST show-hooks call. Assert at
	// least one WARN carries the uniform shape with the sentinel reachable.
	warns := showHooksWarnRecords(rec.records)
	if len(warns) == 0 {
		t.Fatalf("expected at least one %q WARN, got none: %v", showHooksWarnMessage, rec.records)
	}
	assertShowHooksWarnShape(t, warns[0], sentinel)
}

// TestMigrateSessionClosedHook_ShowHooksFailureWarnIsNormalized pins that the
// session-closed migration's pre-existing WARN is normalized to the uniform
// shape (message "show-hooks failed", error_class=unexpected, error attr = the
// wrapped error) and still emitted before the return.
func TestMigrateSessionClosedHook_ShowHooksFailureWarnIsNormalized(t *testing.T) {
	// Fail only the session-closed migration's ShowGlobalHooks call (the third
	// show-hooks call inside RegisterPortalHooks: migrateHydrationHooks #1,
	// session-created RegisterHookIfAbsent #2, migrateSessionClosedHook #3) so
	// the asserted WARN is unambiguously the session-closed one.
	sentinel := errors.New("tmux show-hooks failure (session-closed)")
	var showCallCount int
	runFunc := func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
			showCallCount++
			if showCallCount == 3 {
				return "", sentinel
			}
			return "", nil
		}
		if len(args) >= 2 && args[0] == "set-hook" && (args[1] == "-ga" || args[1] == "-gu") {
			return "", nil
		}
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
	mock := &MockCommander{RunFunc: runFunc}
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

	// session-closed must NOT have been appended (migration skipped).
	for _, c := range mock.Calls {
		if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" && c[2] == "session-closed" {
			t.Errorf("session-closed must not be appended when ShowGlobalHooks fails: %v", c)
		}
	}
}

// TestShowHooksWarn_ErrorAttrCarriesCommandErrorChain pins that the error attr
// is the WRAPPED error value (not .Error()), so the underlying *CommandError
// (carrying tmux argv + stderr per Task 4-2) is reachable via errors.As on the
// captured attr value. Drives RegisterHookIfAbsent (the package-level sink) with
// a Commander that returns a *CommandError.
func TestShowHooksWarn_ErrorAttrCarriesCommandErrorChain(t *testing.T) {
	cmdErr := &tmux.CommandError{
		Stderr: "no server running on /tmp/tmux-1000/default",
		Err:    errors.New("exit status 1"),
		Args:   []string{"show-hooks", "-g"},
	}
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if args[0] == "show-hooks" {
				return "", cmdErr
			}
			t.Fatalf("set-hook must not be called when show-hooks fails: %v", args)
			return "", nil
		},
	}
	client := tmux.NewClient(mock)

	rec := &recordingSlogHandler{}
	log.SetTestHandler(t, rec)

	if err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify",
		`run-shell 'portal state notify'`); err == nil {
		t.Fatal("expected error, got nil")
	}

	warns := showHooksWarnRecords(rec.records)
	if len(warns) != 1 {
		t.Fatalf("expected exactly 1 %q WARN, got %d: %v", showHooksWarnMessage, len(warns), rec.records)
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
// invariant: when EVERY show-hooks call fails, each sibling failure is logged
// exactly once (N WARNs for N failing siblings), and RegisterPortalHooks adds no
// extra aggregate WARN for the errors.Join folding.
//
// Both the package-level bootstrapLogger (RegisterHookIfAbsent siblings) AND the
// injected logger (the two migrate helpers) must route to the same recorder, so
// the test installs the recorder via log.SetTestHandler AND passes a logger
// built over the same recorder into RegisterPortalHooks.
func TestRegisterPortalHooks_ShowHooksFailureLoggedExactlyOnce(t *testing.T) {
	sentinel := errors.New("tmux show-hooks fails everywhere")
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
				return "", sentinel
			}
			t.Fatalf("set-hook must not be called when show-hooks fails: %v", args)
			return "", nil
		},
	}
	client := tmux.NewClient(mock)

	rec := &recordingSlogHandler{}
	log.SetTestHandler(t, rec)
	injected := slog.New(rec).With("component", "bootstrap")

	err := tmux.RegisterPortalHooks(client, injected)
	if err == nil {
		t.Fatal("expected aggregate error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("aggregate error %v does not wrap sentinel %v", err, sentinel)
	}

	// One WARN per sibling that called ShowGlobalHooks:
	//   - migrateHydrationHooks (1)
	//   - migrateSessionClosedHook (1)
	//   - RegisterHookIfAbsent for the six non-session-closed save-trigger
	//     events + the two hydration-trigger events (8)
	// = 10 sibling show-hooks failures. No aggregate WARN from RegisterPortalHooks.
	wantSiblingFailures := 1 + 1 + len(nonSessionClosedSaveTriggerEvents) + len(tmux.HydrationTriggerEvents)
	warns := showHooksWarnRecords(rec.records)
	if len(warns) != wantSiblingFailures {
		t.Fatalf("expected exactly %d %q WARNs (one per sibling failure, no aggregate double-log), got %d: %v",
			wantSiblingFailures, showHooksWarnMessage, len(warns), rec.records)
	}
	for i, w := range warns {
		t.Run("warn-"+string(rune('0'+i)), func(t *testing.T) {
			assertShowHooksWarnShape(t, w, sentinel)
		})
	}
}
