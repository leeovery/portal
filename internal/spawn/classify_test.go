package spawn

import (
	"slices"
	"testing"
)

// TestWindowResult_Confirmed pins the count-semantics predicate: a window is
// "opened" exactly when its Ack is AckConfirmed; every other ack (timeout, failed,
// the zero value) is not confirmed.
func TestWindowResult_Confirmed(t *testing.T) {
	tests := []struct {
		name     string
		ack      AckOutcome
		expected bool
	}{
		{name: "confirmed", ack: AckConfirmed, expected: true},
		{name: "timeout", ack: AckTimeout, expected: false},
		{name: "failed", ack: AckFailed, expected: false},
		{name: "zero value", ack: AckOutcome(""), expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := WindowResult{Ack: tt.ack}
			if got := r.Confirmed(); got != tt.expected {
				t.Errorf("Confirmed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestPartitionResults asserts the confirmed/failed partition preserves list order
// and unifies AckTimeout + AckFailed into "failed".
func TestPartitionResults(t *testing.T) {
	tests := []struct {
		name          string
		results       []WindowResult
		wantConfirmed []string
		wantFailed    []string
	}{
		{
			name:          "empty returns nil, nil",
			results:       nil,
			wantConfirmed: nil,
			wantFailed:    nil,
		},
		{
			name: "all confirmed preserves order",
			results: []WindowResult{
				{Session: "s1", Ack: AckConfirmed},
				{Session: "s2", Ack: AckConfirmed},
			},
			wantConfirmed: []string{"s1", "s2"},
			wantFailed:    nil,
		},
		{
			name: "timeout and failed both land in failed, order preserved",
			results: []WindowResult{
				{Session: "s1", Ack: AckConfirmed},
				{Session: "s2", Ack: AckTimeout},
				{Session: "s3", Ack: AckFailed},
				{Session: "s4", Ack: AckConfirmed},
			},
			wantConfirmed: []string{"s1", "s4"},
			wantFailed:    []string{"s2", "s3"},
		},
		{
			name: "all failed",
			results: []WindowResult{
				{Session: "s1", Ack: AckTimeout},
				{Session: "s2", Ack: AckFailed},
			},
			wantConfirmed: nil,
			wantFailed:    []string{"s1", "s2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotConfirmed, gotFailed := PartitionResults(tt.results)
			if !slices.Equal(gotConfirmed, tt.wantConfirmed) {
				t.Errorf("confirmed = %#v, want %#v", gotConfirmed, tt.wantConfirmed)
			}
			if !slices.Equal(gotFailed, tt.wantFailed) {
				t.Errorf("failed = %#v, want %#v", gotFailed, tt.wantFailed)
			}
		})
	}
}

// TestFirstPermission asserts FirstPermission returns the FIRST permission-required
// window (switching on the generic Outcome alone) and (zero, false) when none hit
// the wall.
func TestFirstPermission(t *testing.T) {
	tests := []struct {
		name        string
		results     []WindowResult
		wantOK      bool
		wantSession string
	}{
		{
			name:    "empty returns false",
			results: nil,
			wantOK:  false,
		},
		{
			name: "no permission window returns false",
			results: []WindowResult{
				{Session: "s1", Result: Success("ok")},
				{Session: "s2", Result: SpawnFailed("boom")},
			},
			wantOK: false,
		},
		{
			name: "returns the first permission window in list order",
			results: []WindowResult{
				{Session: "s1", Result: Success("ok")},
				{Session: "s2", Result: PermissionRequired("evt -1743", "grant access")},
				{Session: "s3", Result: PermissionRequired("evt -1712", "grant access too")},
			},
			wantOK:      true,
			wantSession: "s2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FirstPermission(tt.results)
			if ok != tt.wantOK {
				t.Fatalf("FirstPermission ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				if got != (WindowResult{}) {
					t.Errorf("on no permission, got = %#v, want the zero WindowResult", got)
				}
				return
			}
			if got.Session != tt.wantSession {
				t.Errorf("first permission session = %q, want %q", got.Session, tt.wantSession)
			}
		})
	}
}
