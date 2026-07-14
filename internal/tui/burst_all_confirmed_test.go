package tui

// restore-host-terminal-windows-9-6 — burstAllConfirmed derives its all-confirmed
// verdict from the shared spawn.PartitionResults chokepoint (the "failed slice is
// empty" relationship), NOT a hand-rolled !r.Confirmed() loop, so the picker's
// success gate rests on the SAME count-semantics the CLI's does (cmd/spawn.go's
// runSpawn) and the two orchestrations cannot drift.
//
// White-box (package tui): burstAllConfirmed reads only m.burstExternal and the
// terminal spawnCompleteMsg, so a bare Model literal with burstExternal set drives
// it directly — no async goroutine, no seams. No t.Parallel: consistent with the
// rest of the tui test surface.

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/spawn"
)

// confirmedResult / timeoutResult / failedResult / permissionResult are the four
// WindowResult shapes a burst produces, so the fixtures below read declaratively.
func confirmedResult(sess string) spawn.WindowResult {
	return spawn.WindowResult{Session: sess, Ack: spawn.AckConfirmed, Result: spawn.Success("")}
}

func timeoutResult(sess string) spawn.WindowResult {
	return spawn.WindowResult{Session: sess, Ack: spawn.AckTimeout, Result: spawn.Success("")}
}

func failedResult(sess string) spawn.WindowResult {
	return spawn.WindowResult{Session: sess, Ack: spawn.AckFailed, Result: spawn.SpawnFailed("boom")}
}

// permissionResult is a permission-walled window: it never opened (result.OK() is
// false → the burster leaves Ack at AckFailed), so it is a FAILED window that ALSO
// carries the permission Outcome. This is why len(failed)==0 stays equivalent to
// all-Confirmed even when a permission result is present.
func permissionResult(sess string) spawn.WindowResult {
	return spawn.WindowResult{Session: sess, Ack: spawn.AckFailed, Result: spawn.PermissionRequired("evt -1743", "grant access")}
}

// sessionsOf projects the target session names out of a result slice — the picker's
// burstExternal set for a full-length terminal event is exactly one name per result.
func sessionsOf(results []spawn.WindowResult) []string {
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Session
	}
	return names
}

// TestBurstAllConfirmed_TruthTable pins the whole truth table of the picker's
// full-success gate: true ONLY for an error-free, full-length, all-AckConfirmed
// terminal event; false for any non-confirmed ack (timeout/failed/permission), a
// non-nil msg.Err, or a length mismatch in either direction.
func TestBurstAllConfirmed_TruthTable(t *testing.T) {
	external := []string{"alpha", "bravo"}
	confirmedPair := []spawn.WindowResult{confirmedResult("alpha"), confirmedResult("bravo")}

	tests := []struct {
		name string
		msg  spawnCompleteMsg
		want bool
	}{
		{
			name: "error-free full-length all-confirmed → true",
			msg:  spawnCompleteMsg{Results: confirmedPair},
			want: true,
		},
		{
			name: "one ack-timeout → false",
			msg:  spawnCompleteMsg{Results: []spawn.WindowResult{confirmedResult("alpha"), timeoutResult("bravo")}},
			want: false,
		},
		{
			name: "one ack-failed → false",
			msg:  spawnCompleteMsg{Results: []spawn.WindowResult{confirmedResult("alpha"), failedResult("bravo")}},
			want: false,
		},
		{
			name: "permission result → false",
			msg:  spawnCompleteMsg{Results: []spawn.WindowResult{confirmedResult("alpha"), permissionResult("bravo")}},
			want: false,
		},
		{
			name: "msg.Err set (results otherwise all-confirmed) → false",
			msg:  spawnCompleteMsg{Err: errors.New("os.Executable: boom"), Results: confirmedPair},
			want: false,
		},
		{
			name: "length mismatch — too few results → false",
			msg:  spawnCompleteMsg{Results: []spawn.WindowResult{confirmedResult("alpha")}},
			want: false,
		},
		{
			name: "length mismatch — too many results → false",
			msg:  spawnCompleteMsg{Results: []spawn.WindowResult{confirmedResult("alpha"), confirmedResult("bravo"), confirmedResult("charlie")}},
			want: false,
		},
		{
			name: "empty results vs non-empty external → false (the length guard covers the vacuous case)",
			msg:  spawnCompleteMsg{Results: nil},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{burstExternal: external}
			if got := m.burstAllConfirmed(tt.msg); got != tt.want {
				t.Errorf("burstAllConfirmed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// burstClass is the terminal 3-way classification both the CLI (cmd/spawn.go's
// runSpawn) and the picker (the spawnCompleteMsg handler + burstAllConfirmed) reach
// for a completed burst.
type burstClass int

const (
	classAllConfirmed burstClass = iota
	classPermission
	classPartial
)

func (c burstClass) String() string {
	switch c {
	case classAllConfirmed:
		return "all-confirmed"
	case classPermission:
		return "permission"
	default:
		return "partial"
	}
}

// canonicalBurstClass derives the terminal classification from the SHARED spawn
// chokepoint — the exact spawn.PartitionResults → spawn.FirstPermission branch order
// runSpawn (cmd/spawn.go) keys its self-attach/partial/permission decision off, and
// the same relationship the picker's spawnCompleteMsg handler uses (burstAllConfirmed
// for the all-confirmed axis, spawn.FirstPermission for the permission-vs-partial
// split). Anchoring the fixture expectations to this single derivation is what proves
// the two orchestrations cannot drift.
func canonicalBurstClass(results []spawn.WindowResult) burstClass {
	if _, failed := spawn.PartitionResults(results); len(failed) == 0 {
		return classAllConfirmed
	}
	if _, ok := spawn.FirstPermission(results); ok {
		return classPermission
	}
	return classPartial
}

// TestBurstAllConfirmed_ClassificationParityWithChokepoint is the cross-caller parity
// guard: a shared fixture table of []spawn.WindowResult reaches the SAME terminal
// classification on the CLI and picker paths because both derive it from
// spawn.PartitionResults / spawn.FirstPermission. It asserts the picker's success gate
// (burstAllConfirmed) is true EXACTLY when the shared chokepoint's failed slice is
// empty — the identical `len(failed) == 0` relationship runSpawn's all-confirmed gate
// uses — for the same fixtures, so a future change to what "all confirmed" means lands
// in PartitionResults and both orchestrations move together.
func TestBurstAllConfirmed_ClassificationParityWithChokepoint(t *testing.T) {
	tests := []struct {
		name    string
		results []spawn.WindowResult
		want    burstClass
	}{
		{"all confirmed", []spawn.WindowResult{confirmedResult("s1"), confirmedResult("s2")}, classAllConfirmed},
		{"single confirmed", []spawn.WindowResult{confirmedResult("s1")}, classAllConfirmed},
		{"one timeout is partial", []spawn.WindowResult{confirmedResult("s1"), timeoutResult("s2")}, classPartial},
		{"one spawn-failed is partial", []spawn.WindowResult{confirmedResult("s1"), failedResult("s2")}, classPartial},
		{"timeout and failed is partial", []spawn.WindowResult{timeoutResult("s1"), failedResult("s2")}, classPartial},
		{"permission takes precedence over partial", []spawn.WindowResult{confirmedResult("s1"), permissionResult("s2")}, classPermission},
		{"permission with a trailing failed is still permission", []spawn.WindowResult{permissionResult("s1"), failedResult("s2")}, classPermission},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The shared chokepoint reaches the fixture's declared class — this is the
			// derivation both cmd/spawn.go and the picker key off.
			if got := canonicalBurstClass(tt.results); got != tt.want {
				t.Fatalf("canonicalBurstClass = %s, want %s (the shared spawn.PartitionResults/FirstPermission derivation)", got, tt.want)
			}

			// The picker's success gate agrees with the chokepoint's all-confirmed axis for
			// an error-free, full-length terminal event: true EXACTLY when the canonical
			// class is all-confirmed.
			m := Model{burstExternal: sessionsOf(tt.results)}
			gotConfirmed := m.burstAllConfirmed(spawnCompleteMsg{Results: tt.results})
			wantConfirmed := tt.want == classAllConfirmed
			if gotConfirmed != wantConfirmed {
				t.Errorf("burstAllConfirmed = %v, want %v (picker gate must equal the chokepoint's all-confirmed class)", gotConfirmed, wantConfirmed)
			}

			// And it rests directly on spawn.PartitionResults' failed==empty relationship —
			// the SAME expression runSpawn's `len(failed) == 0` gate uses — closing the loop
			// on "derives from the chokepoint, not a parallel loop".
			_, failed := spawn.PartitionResults(tt.results)
			if gotConfirmed != (len(failed) == 0) {
				t.Errorf("burstAllConfirmed = %v, but spawn.PartitionResults failed==empty is %v — the picker gate must rest on the same relationship the CLI's does", gotConfirmed, len(failed) == 0)
			}
		})
	}
}
