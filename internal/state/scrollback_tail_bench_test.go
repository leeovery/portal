package state_test

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// Performance budget anchor: spec § History Depth > Read Pipeline >
// Performance budget pins tail-N read p99 < 5 ms on a 4 MB .bin file
// (warmed-cache, representative of a busy-session worst case). This file
// holds both the regression-guard test and the standalone benchmark that
// guard that assertion. Cold-cache cost is explicitly out of scope.

// perfBudget is the warmed-cache wall-clock ceiling for a single
// TailScrollback(N=1000) call against the 4 MB fixture below. Anchored
// to spec § History Depth > Read Pipeline > Performance budget; if a
// future change pushes the second-call timing above this, the
// synchronous-in-Update read decision must be revisited.
const perfBudget = 5 * time.Millisecond

// perfFixtureLines / perfFixtureLineWidth target ~4 MB total
// (50_000 lines × ~80 bytes). The exact total varies because line widths
// are jittered around the mean; a ±10% swing is acceptable.
const (
	perfFixtureLines     = 50_000
	perfFixtureLineWidth = 80
)

// buildPerfFixture writes a deterministic ~4 MB scrollback fixture to a
// fresh path inside dir and returns the path. Seeded with rand.NewPCG(42, 42)
// so size and content are reproducible across runs and machines.
//
// The fixture mimics real tmux capture-pane -e output: mostly printable
// ASCII at varied line widths around perfFixtureLineWidth, with synthetic
// ANSI SGR escapes (e.g. \x1b[31mred\x1b[0m) sprinkled in roughly every
// 3-5 lines. ANSI presence is structural — the tail helper treats them as
// opaque bytes, but a fixture without escapes would not faithfully
// represent the worst-case content the helper must scan.
func buildPerfFixture(tb testing.TB, dir string) string {
	tb.Helper()
	rng := rand.New(rand.NewPCG(42, 42))
	colours := []string{
		"\x1b[31m", // red
		"\x1b[32m", // green
		"\x1b[33m", // yellow
		"\x1b[34m", // blue
		"\x1b[36m", // cyan
	}
	const reset = "\x1b[0m"

	// Pre-size the buffer to the expected total to avoid grow-copy
	// thrash during fixture build.
	var buf bytes.Buffer
	buf.Grow(perfFixtureLines * (perfFixtureLineWidth + 8))

	// nextAnsiAt is the line index at which the next ANSI escape pair will
	// be injected. Drawing the gap from [3, 6) gives an average sprinkle
	// interval of ~4 lines, matching the "every 3-5 lines" target.
	nextAnsiAt := 3 + rng.IntN(3)

	for i := range perfFixtureLines {
		// Jitter line width by ±20% of the mean so the reverse-scan must
		// cope with non-uniform record sizes.
		jitter := rng.IntN(perfFixtureLineWidth/5*2+1) - (perfFixtureLineWidth / 5)
		width := max(perfFixtureLineWidth+jitter, 16)

		prefix := fmt.Sprintf("[%05d] ", i)
		// payload fills the remaining width budget (less the trailing \n).
		payloadLen := max(width-len(prefix)-1, 0)
		// Generate alphanumeric-ish content: cheap, printable, varied.
		payload := make([]byte, payloadLen)
		for j := range payload {
			payload[j] = byte('A' + rng.IntN(26))
		}

		buf.WriteString(prefix)
		if i == nextAnsiAt {
			colour := colours[rng.IntN(len(colours))]
			buf.WriteString(colour)
			buf.Write(payload)
			buf.WriteString(reset)
			nextAnsiAt = i + 3 + rng.IntN(3)
		} else {
			buf.Write(payload)
		}
		buf.WriteByte('\n')
	}

	path := filepath.Join(dir, "perf-fixture.bin")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		tb.Fatalf("write perf fixture: %v", err)
	}
	return path
}

// BenchmarkTailScrollback measures TailScrollback(N=1000) against a
// representative 4 MB / ~50k-line scrollback fixture with mixed line widths
// and ANSI escapes. Fixture generation cost is excluded from the measured
// region because b.Loop() resets the timer on its first call, after the
// fixture is written to disk — see spec § History Depth > Read Pipeline >
// Performance budget. b.Loop() also keeps the loop body live, so no manual
// dead-code-elimination guard on the result is needed.
func BenchmarkTailScrollback(b *testing.B) {
	path := buildPerfFixture(b, b.TempDir())

	for b.Loop() {
		if _, err := state.TailScrollback(path, 1000); err != nil {
			b.Fatalf("TailScrollback: %v", err)
		}
	}
}

// TestTailScrollback_PerformanceBudget enforces the warmed-cache
// regression guard for the tail-N read: spec § History Depth > Read
// Pipeline > Performance budget pins p99 < 5 ms on a 4 MB .bin. The
// budget is the audit threshold, not a soft aspiration — if a future
// change pushes this past 5 ms after warmup, the synchronous-read
// decision in the preview Update path must be revisited.
//
// Cold-cache cost is explicitly out of scope; the first call below is a
// warmup whose time is discarded. The second call's wall-clock is the
// asserted measurement.
//
// The PORTAL_SKIP_PERF env var is an opt-out for slow CI runners — set
// it to any non-empty value to skip this test.
func TestTailScrollback_PerformanceBudget(t *testing.T) {
	if os.Getenv("PORTAL_SKIP_PERF") != "" {
		t.Skip("PORTAL_SKIP_PERF set; skipping warmed-cache perf budget assertion")
	}

	path := buildPerfFixture(t, t.TempDir())

	// Warmup: populate the page cache so the measured call reflects the
	// warmed-cache budget the spec actually pins.
	if _, err := state.TailScrollback(path, 1000); err != nil {
		t.Fatalf("warmup TailScrollback: %v", err)
	}

	start := time.Now()
	got, err := state.TailScrollback(path, 1000)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("measured TailScrollback: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected non-empty tail bytes, got 0")
	}
	if elapsed >= perfBudget {
		t.Fatalf("warmed tail-N read = %v, budget = %v (spec: tail-N p99 < 5ms on 4 MB .bin)", elapsed, perfBudget)
	}
}
