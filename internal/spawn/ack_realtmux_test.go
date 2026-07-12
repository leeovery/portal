//go:build integration

package spawn_test

// Real-tmux round-trip guard for the @portal-spawn ack channel.
//
// The unit tests drive Collect/Clean/Write with crafted show-options strings and
// in-memory fakes; none exercises the round trip through a real tmux server, so
// a set-option / show-options / set-option -su quoting or format drift could
// pass every unit test while silently breaking the ack contract against a live
// server. This test closes that seam on a per-test isolated -S socket (never the
// developer's default server): it writes a real @portal-spawn-b1-t1 marker and a
// co-resident @portal-skeleton-foo marker, then proves the two enumerators are
// blind to each other, that Clean removes only the spawn marker, that a second
// Clean is a nil no-op (idempotency), and that the skeleton marker is untouched
// throughout.
//
// No daemon, no built binary — consistent with the unit lane's real-tmux client
// tests. Carries //go:build integration per CLAUDE.md's lane rule (real tmux).

import (
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// hasToken reports whether token is present in a Collect result.
func hasToken(set map[string]struct{}, token string) bool {
	_, ok := set[token]
	return ok
}

// sortedSet returns the keys of a set for diagnostic messages.
func sortedSet(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestServerOptionAckChannel_RealTmuxRoundTripAndIdempotentClean(t *testing.T) {
	// "integration: it round-trips and idempotently cleans a real tmux marker alongside a skeleton marker"
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "spawnack-")
	client := ts.Client()

	// A session anchors the server so server options are queryable.
	ts.Run(t, "new-session", "-d", "-s", "anchor")

	// *tmux.Client satisfies both unexported spawn seams implicitly.
	ch := spawn.NewServerOptionAckChannel(client, client)

	// Write a real spawn marker and a co-resident skeleton marker.
	if err := ch.Write("b1", "t1"); err != nil {
		t.Fatalf("Write(b1, t1): %v", err)
	}
	if err := state.SetSkeletonMarker(client, "foo"); err != nil {
		t.Fatalf("SetSkeletonMarker(foo): %v", err)
	}

	// Collect sees only the spawn token; the skeleton marker is invisible to it.
	got, err := ch.Collect("b1")
	if err != nil {
		t.Fatalf("Collect(b1): %v", err)
	}
	if len(got) != 1 || !hasToken(got, "t1") {
		t.Fatalf("Collect(b1) = %v, want exactly {t1}", sortedSet(got))
	}

	// ListSkeletonMarkers sees only the skeleton paneKey; the spawn marker is
	// invisible to it — the complementary isolation direction, over real tmux.
	skel, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}
	if len(skel) != 1 || !hasToken(skel, "foo") {
		t.Fatalf("ListSkeletonMarkers = %v, want exactly {foo}", sortedSet(skel))
	}

	// Clean removes the spawn marker.
	if err := ch.Clean("b1"); err != nil {
		t.Fatalf("Clean(b1): %v", err)
	}
	afterClean, err := ch.Collect("b1")
	if err != nil {
		t.Fatalf("Collect(b1) after Clean: %v", err)
	}
	if len(afterClean) != 0 {
		t.Errorf("Collect(b1) after Clean = %v, want empty", sortedSet(afterClean))
	}

	// A second Clean on the now-empty batch is a nil-return no-op (idempotency —
	// tmux set-option -su on an absent option exits 0, and enumeration finds no
	// markers to unset).
	if err := ch.Clean("b1"); err != nil {
		t.Errorf("second Clean(b1) = %v, want nil (idempotent no-op)", err)
	}

	// The skeleton marker was untouched throughout.
	skelAfter, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers after Clean: %v", err)
	}
	if len(skelAfter) != 1 || !hasToken(skelAfter, "foo") {
		t.Errorf("ListSkeletonMarkers after Clean = %v, want {foo} untouched", sortedSet(skelAfter))
	}
}
