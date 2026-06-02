package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/log"
)

// captureHandler is an in-memory slog.Handler that records every Handle call.
// It is the main-package equivalent of internal/log's recordingHandler, used to
// assert on the process: panic / process: exit terminal markers emitted across
// the run()/Close() boundary. WithAttrs returns h so the component attr (delivered
// via log.For -> root.With("component", ...)) is observed at Handle time but not
// re-bound; tests assert on Message + Level + the record's own attrs.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *captureHandler) WithGroup(string) slog.Handler { return h }

// messages returns the captured records whose Message equals msg.
func (h *captureHandler) messages(msg string) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.records {
		if r.Message == msg {
			out = append(out, r)
		}
	}
	return out
}

// attrValue resolves the named attr from a record, reporting whether it was set.
func attrValue(r slog.Record, key string) (slog.Value, bool) {
	var (
		v     slog.Value
		found bool
	)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			v = a.Value.Resolve()
			found = true
			return false
		}
		return true
	})
	return v, found
}

func TestRunPanicEmission(t *testing.T) {
	t.Run("it emits ERROR process: panic with reason on a recovered panic", func(t *testing.T) {
		rec := &captureHandler{}
		log.SetTestHandler(t, rec)
		withSeams(t, func() error { panic("kaboom") })

		code, panicked := run()

		if code != 2 {
			t.Errorf("code = %d, want 2", code)
		}
		if !panicked {
			t.Error("panicked = false, want true")
		}

		panics := rec.messages("panic")
		if len(panics) != 1 {
			t.Fatalf("expected exactly 1 process: panic record, got %d", len(panics))
		}
		r := panics[0]
		if r.Level != slog.LevelError {
			t.Errorf("process: panic level = %v, want ERROR", r.Level)
		}
		reason, ok := attrValue(r, "reason")
		if !ok {
			t.Fatalf("process: panic record missing reason attr")
		}
		if got := reason.String(); got != "kaboom" {
			t.Errorf("reason attr = %q, want %q", got, "kaboom")
		}
	})

	t.Run("it skips Close on the panic path so no process: exit fires", func(t *testing.T) {
		rec := &captureHandler{}
		log.SetTestHandler(t, rec)
		withSeams(t, func() error { panic("boom") })

		_, panicked := run()

		// run() does not call Close; Close is gated behind !panicked in main().
		// Model that gate: since panicked is true, main would skip Close, so no
		// process: exit marker may exist on this path.
		if !panicked {
			t.Fatal("panicked = false, want true (so main skips Close)")
		}
		mainEmitClose(2, panicked)

		if exits := rec.messages("exit"); len(exits) != 0 {
			t.Errorf("expected no process: exit on the panic path, got %d", len(exits))
		}
	})

	t.Run("it emits process: exit (not panic) on a clean run", func(t *testing.T) {
		rec := &captureHandler{}
		log.SetTestHandler(t, rec)
		withSeams(t, func() error { return nil })

		code, panicked := run()
		if code != 0 {
			t.Errorf("code = %d, want 0", code)
		}
		if panicked {
			t.Error("panicked = true, want false")
		}
		mainEmitClose(code, panicked)

		if panics := rec.messages("panic"); len(panics) != 0 {
			t.Errorf("expected no process: panic on a clean run, got %d", len(panics))
		}
		exits := rec.messages("exit")
		if len(exits) != 1 {
			t.Fatalf("expected exactly 1 process: exit on a clean run, got %d", len(exits))
		}
		codeVal, ok := attrValue(exits[0], "code")
		if !ok {
			t.Fatalf("process: exit record missing code attr")
		}
		if got := codeVal.Int64(); got != 0 {
			t.Errorf("exit code attr = %d, want 0", got)
		}
	})

	t.Run("it emits process: exit code=N on an error run", func(t *testing.T) {
		rec := &captureHandler{}
		log.SetTestHandler(t, rec)
		withSeams(t, func() error { return errors.New("boom") })

		code, panicked := run()
		if code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
		if panicked {
			t.Error("panicked = true, want false")
		}
		mainEmitClose(code, panicked)

		if panics := rec.messages("panic"); len(panics) != 0 {
			t.Errorf("expected no process: panic on an error run, got %d", len(panics))
		}
		exits := rec.messages("exit")
		if len(exits) != 1 {
			t.Fatalf("expected exactly 1 process: exit on an error run, got %d", len(exits))
		}
		codeVal, ok := attrValue(exits[0], "code")
		if !ok {
			t.Fatalf("process: exit record missing code attr")
		}
		if got := codeVal.Int64(); got != 1 {
			t.Errorf("exit code attr = %d, want 1", got)
		}
	})

	t.Run("the four-way classification stays mutually exclusive (panic XOR exit)", func(t *testing.T) {
		cases := []struct {
			name      string
			execute   func() error
			wantPanic bool
		}{
			{"clean", func() error { return nil }, false},
			{"error", func() error { return errors.New("boom") }, false},
			{"panic", func() error { panic("x") }, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := &captureHandler{}
				log.SetTestHandler(t, rec)
				withSeams(t, tc.execute)

				code, panicked := run()
				mainEmitClose(code, panicked)

				panics := len(rec.messages("panic"))
				exits := len(rec.messages("exit"))
				if panicked != tc.wantPanic {
					t.Fatalf("panicked = %v, want %v", panicked, tc.wantPanic)
				}
				// Exactly one terminal marker fires: panic XOR exit.
				if panics+exits != 1 {
					t.Fatalf("terminal markers panic=%d exit=%d, want exactly one total", panics, exits)
				}
				if tc.wantPanic && panics != 1 {
					t.Errorf("panic path: panic markers = %d, want 1 (exit must be skipped)", panics)
				}
				if !tc.wantPanic && exits != 1 {
					t.Errorf("non-panic path: exit markers = %d, want 1", exits)
				}
			})
		}
	})
}

// mainEmitClose models main()'s post-run !panicked gate: main calls
// log.Close(code) (which emits process: exit code=N) only on the non-panic path.
// It lets the panic/exit mutual-exclusivity be asserted at the main() level
// WITHOUT invoking the real main() (which calls os.Exit). Close routes through
// the already-swapped test handler.
func mainEmitClose(code int, panicked bool) {
	if !panicked {
		log.Close(code)
	}
}
