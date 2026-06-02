package log

import (
	"log/slog"
	"testing"
	"time"
)

// TestTook_KeyAndDurationKind pins the reserved cycle-summary attr contract:
// the returned attr is keyed "took" and carries a slog.KindDuration value.
func TestTook_KeyAndDurationKind(t *testing.T) {
	attr := Took(time.Now())

	if attr.Key != "took" {
		t.Fatalf("Took attr key = %q, want %q", attr.Key, "took")
	}
	if got := attr.Value.Kind(); got != slog.KindDuration {
		t.Fatalf("Took attr value kind = %v, want %v", got, slog.KindDuration)
	}
}

// TestTook_MeasuresElapsedSinceStart confirms Took carries a non-negative
// duration derived from the supplied start time (i.e. it is time.Since(start),
// not a zero/sentinel value).
func TestTook_MeasuresElapsedSinceStart(t *testing.T) {
	start := time.Now().Add(-5 * time.Millisecond)

	attr := Took(start)

	if d := attr.Value.Duration(); d < 5*time.Millisecond {
		t.Fatalf("Took duration = %v, want >= 5ms (measured from a start 5ms in the past)", d)
	}
}
