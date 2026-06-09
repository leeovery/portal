package log

import (
	"testing"
	"time"
)

// TestParseLogLine_WellFormed parses a canonical writer line into its four
// fields. The line shape mirrors textHandler.Handle exactly.
func TestParseLogLine_WellFormed(t *testing.T) {
	line := "2026-06-09T10:15:30.123456789Z WARN daemon: tick complete took=12ms pid=4242 version=1.2.3 process_role=daemon"

	parsed, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("ParseLogLine ok = false, want true for %q", line)
	}

	wantTime, err := time.Parse(time.RFC3339Nano, "2026-06-09T10:15:30.123456789Z")
	if err != nil {
		t.Fatalf("fixture timestamp unparseable: %v", err)
	}
	if !parsed.Time.Equal(wantTime) {
		t.Errorf("Time = %v, want %v", parsed.Time, wantTime)
	}
	if parsed.Level != "WARN" {
		t.Errorf("Level = %q, want %q", parsed.Level, "WARN")
	}
	if parsed.Component != "daemon" {
		t.Errorf("Component = %q, want %q", parsed.Component, "daemon")
	}
	if parsed.Message != "tick complete" {
		t.Errorf("Message = %q, want %q", parsed.Message, "tick complete")
	}
}

// TestParseLogLine_StripsAttrsAndBaselines confirms the message boundary drops
// both contextual attrs and the pid/version/process_role baselines in one pass.
func TestParseLogLine_StripsAttrsAndBaselines(t *testing.T) {
	line := "2026-06-09T10:15:30Z INFO bootstrap: orchestration complete warnings=2 took=1.2s pid=9 version=1.0.0 process_role=cli"

	parsed, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("ParseLogLine ok = false, want true")
	}
	if parsed.Message != "orchestration complete" {
		t.Errorf("Message = %q, want %q", parsed.Message, "orchestration complete")
	}
}

// TestParseLogLine_NoAttrsPreservedWhole confirms a message with no trailing
// attrs (only the always-present baselines) is preserved whole, not truncated.
func TestParseLogLine_NoAttrsPreservedWhole(t *testing.T) {
	line := "2026-06-09T10:15:30Z WARN restore: skeleton reconstruction failed pid=9 version=1.0.0 process_role=cli"

	parsed, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("ParseLogLine ok = false, want true")
	}
	if parsed.Message != "skeleton reconstruction failed" {
		t.Errorf("Message = %q, want %q", parsed.Message, "skeleton reconstruction failed")
	}
}

// TestParseLogLine_PreservesLaterColons confirms only the first ':' delimits the
// component; colons inside the message are retained.
func TestParseLogLine_PreservesLaterColons(t *testing.T) {
	line := "2026-06-09T10:15:30Z WARN daemon: flush failed: disk full pid=9 version=1.0.0 process_role=daemon"

	parsed, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("ParseLogLine ok = false, want true")
	}
	if parsed.Component != "daemon" {
		t.Errorf("Component = %q, want %q", parsed.Component, "daemon")
	}
	if parsed.Message != "flush failed: disk full" {
		t.Errorf("Message = %q, want %q", parsed.Message, "flush failed: disk full")
	}
}

// TestParseLogLine_QuotedMultiWordAttrValue confirms a quoted attr value
// containing spaces does not shift the message boundary earlier than the first
// real attr key. version="3.6 beta" produces a token 'beta"' that has no
// key= shape, so the boundary stays on the first genuine attr key.
func TestParseLogLine_QuotedMultiWordAttrValue(t *testing.T) {
	line := `2026-06-09T10:15:30Z INFO process: start pid=9 version="3.6 beta" process_role=cli`

	parsed, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("ParseLogLine ok = false, want true")
	}
	if parsed.Message != "start" {
		t.Errorf("Message = %q, want %q", parsed.Message, "start")
	}
}

// TestParseLogLine_EmptyComponent confirms an empty-component line (the writer
// emits a space before the colon) yields Component == "" with ok == true.
func TestParseLogLine_EmptyComponent(t *testing.T) {
	line := "2026-06-09T10:15:30Z WARN : tick complete pid=9 version=1.0.0 process_role=cli"

	parsed, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("ParseLogLine ok = false, want true")
	}
	if parsed.Component != "" {
		t.Errorf("Component = %q, want %q", parsed.Component, "")
	}
	if parsed.Message != "tick complete" {
		t.Errorf("Message = %q, want %q", parsed.Message, "tick complete")
	}
}

// TestParseLogLine_EmptyMessage confirms a line whose component colon-space is
// immediately followed by the first attr token yields Message == "" with ok.
func TestParseLogLine_EmptyMessage(t *testing.T) {
	line := "2026-06-09T10:15:30Z WARN daemon: pid=9 version=1.0.0 process_role=daemon"

	parsed, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("ParseLogLine ok = false, want true")
	}
	if parsed.Component != "daemon" {
		t.Errorf("Component = %q, want %q", parsed.Component, "daemon")
	}
	if parsed.Message != "" {
		t.Errorf("Message = %q, want %q", parsed.Message, "")
	}
}

// TestParseLogLine_UnparseableTimestamp confirms a non-RFC3339Nano first token
// yields ok == false.
func TestParseLogLine_UnparseableTimestamp(t *testing.T) {
	line := "not-a-timestamp WARN daemon: tick complete pid=9 version=1.0.0 process_role=daemon"

	if _, ok := ParseLogLine(line); ok {
		t.Fatalf("ParseLogLine ok = true, want false for unparseable timestamp")
	}
}

// TestParseLogLine_NoColon confirms a line with no ':' (no component delimiter)
// yields ok == false.
func TestParseLogLine_NoColon(t *testing.T) {
	line := "2026-06-09T10:15:30Z WARN daemon tick complete pid=9"

	if _, ok := ParseLogLine(line); ok {
		t.Fatalf("ParseLogLine ok = true, want false for line with no colon")
	}
}

// TestParseLogLine_FewerThanTwoTokens confirms a single-token line yields
// ok == false (no level token).
func TestParseLogLine_FewerThanTwoTokens(t *testing.T) {
	line := "2026-06-09T10:15:30Z"

	if _, ok := ParseLogLine(line); ok {
		t.Fatalf("ParseLogLine ok = true, want false for single-token line")
	}
}

// TestParseLogLine_EmptyLine confirms the empty string yields ok == false.
func TestParseLogLine_EmptyLine(t *testing.T) {
	if _, ok := ParseLogLine(""); ok {
		t.Fatalf("ParseLogLine ok = true, want false for empty line")
	}
}

// TestParseLogLine_WholeAndFractionalSecondTimestamps confirms both a
// whole-second timestamp and a fractional-second (RFC3339Nano) timestamp parse.
func TestParseLogLine_WholeAndFractionalSecondTimestamps(t *testing.T) {
	cases := []struct {
		name string
		ts   string
	}{
		{name: "whole second", ts: "2026-06-09T10:15:30Z"},
		{name: "fractional second", ts: "2026-06-09T10:15:30.987654321Z"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := tc.ts + " WARN daemon: tick complete pid=9 version=1.0.0 process_role=daemon"

			parsed, ok := ParseLogLine(line)
			if !ok {
				t.Fatalf("ParseLogLine ok = false, want true for %q", tc.ts)
			}
			want, err := time.Parse(time.RFC3339Nano, tc.ts)
			if err != nil {
				t.Fatalf("fixture timestamp unparseable: %v", err)
			}
			if !parsed.Time.Equal(want) {
				t.Errorf("Time = %v, want %v", parsed.Time, want)
			}
		})
	}
}
