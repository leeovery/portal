package log

import (
	"log/slog"
	"testing"
)

func TestSetTestHandler_RoutesRecordsToTestHandler(t *testing.T) {
	rec := &recordingHandler{}
	SetTestHandler(t, rec)

	For("daemon").Info("routed")

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record routed to the test handler, got %d", len(rec.records))
	}
	if got := rec.records[0].Message; got != "routed" {
		t.Errorf("routed record message = %q, want %q", got, "routed")
	}
}

func TestSetTestHandler_RestoresPriorHandlerViaCleanup(t *testing.T) {
	before := currentHandler()

	rec := &recordingHandler{}
	// The inner subtest's t.Cleanup runs when t.Run returns, so the parent can
	// then assert the handler was restored (testing.T cannot be hand-built for
	// Cleanup).
	t.Run("swap", func(t *testing.T) {
		SetTestHandler(t, rec)
		if got := currentHandler(); got != slog.Handler(rec) {
			t.Fatalf("inner handler not swapped to the test handler")
		}
	})

	if got := currentHandler(); got != before {
		t.Errorf("prior handler not restored after subtest cleanup: got %v, want %v", got, before)
	}
}

func TestSetTestHandler_RestoresNestedSwapsInLIFOOrder(t *testing.T) {
	original := currentHandler()

	outer := &recordingHandler{}
	inner := &recordingHandler{}

	t.Run("outer", func(t *testing.T) {
		SetTestHandler(t, outer)
		if got := currentHandler(); got != slog.Handler(outer) {
			t.Fatalf("outer swap not applied")
		}

		t.Run("inner", func(t *testing.T) {
			SetTestHandler(t, inner)
			if got := currentHandler(); got != slog.Handler(inner) {
				t.Fatalf("inner swap not applied")
			}
		})

		// Inner subtest returned: its cleanup must have restored the OUTER
		// handler (LIFO), not the original.
		if got := currentHandler(); got != slog.Handler(outer) {
			t.Fatalf("after inner cleanup expected outer handler restored (LIFO), got %v", got)
		}
	})

	// Outer subtest returned: its cleanup must have restored the original.
	if got := currentHandler(); got != original {
		t.Errorf("after outer cleanup expected original handler restored, got %v want %v", got, original)
	}
}

func TestSetTestHandler_RestoresCleanlyWhenNeverLogged(t *testing.T) {
	before := currentHandler()

	rec := &recordingHandler{}
	t.Run("swap-no-log", func(t *testing.T) {
		SetTestHandler(t, rec)
		// Intentionally never log: cleanup must still restore without panicking.
	})

	if got := currentHandler(); got != before {
		t.Errorf("prior handler not restored when test never logged: got %v, want %v", got, before)
	}
}
