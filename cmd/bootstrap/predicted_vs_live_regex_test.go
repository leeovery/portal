package bootstrap_test

import (
	"regexp"
	"testing"
)

// predictedVsLiveWarnRegex matches the misleading
// `predicted=<paneKey> live=<paneKey>` WARN that the deleted PredictLiveIndices
// diagnostic used to emit when the live tmux server's base-index /
// pane-base-index differed from the always-zero predicted indices.
//
// Spec § "Acceptance Criteria" item 4 requires the diagnostic to be GONE, not
// silenced — `portal.log` must contain zero lines matching this regex after a
// bootstrap-and-restore round-trip under any tmux config (notably non-zero
// base-index / pane-base-index, which is where the WARN previously surfaced).
//
// The pattern requires BOTH a `predicted=...__N.N` AND a `live=...__N.N`
// segment on the same line so that the preserved armPanes:202 pane-count
// mismatch warning ("live pane count <n> != saved count <m>") is not
// matched. This is the exact regex the integration assertion uses; the
// unit test below proves it (a) catches the offending shape and
// (b) ignores the preserved armPanes:202 warning.
//
// This var is shared between the integration assertion in
// reboot_roundtrip_test.go and the unit test below so both compile
// byte-identical patterns.
var predictedVsLiveWarnRegex = regexp.MustCompile(`predicted=.*__\d+\.\d+ live=.*__\d+\.\d+`)

// TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning
// verifies the regex used by the integration assertion is meaningful and
// false-positive-safe: it must match a representative offending log line
// produced by the deleted PredictLiveIndices diagnostic, and it must NOT
// match the preserved `armPanes:202` pane-count mismatch warning (spec
// § "Acceptance Criteria" item 5 explicitly preserves that diagnostic).
//
// This is a plain-tag unit test (no `//go:build integration` guard) so it
// runs on every CI invocation and does not depend on tmux being installed.
func TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantMatch bool
	}{
		{
			name:      "offending predicted-vs-live shape",
			line:      `WARN | restore | session "alpha": pane 0 predicted=alpha__0.0 live=alpha__1.1`,
			wantMatch: true,
		},
		{
			name:      "preserved armPanes:202 pane-count mismatch warning",
			line:      `WARN | restore | session "alpha": live pane count 2 != saved count 3`,
			wantMatch: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := predictedVsLiveWarnRegex.MatchString(tc.line)
			if got != tc.wantMatch {
				t.Fatalf("regex.MatchString(%q) = %v; want %v", tc.line, got, tc.wantMatch)
			}
		})
	}
}
