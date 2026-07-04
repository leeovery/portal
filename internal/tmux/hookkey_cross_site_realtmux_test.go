package tmux_test

// Real-tmux cross-site consistency guard: the registration read (ResolveHookKey)
// and the stale-cleanup live-key enumeration (ListAllPaneHookKeys) MUST produce
// byte-identical hook keys for the same live session on the same server.
//
// This is the fix's central invariant made testable across two live sites at
// once. Tasks 2-1..2-4 each verify one site in isolation: ResolveHookKey and
// ListAllPaneHookKeys are proven separately against a real tmux server. But
// nothing yet asserts the two live-tmux sites AGREE for the same session. If
// registration stored a key that stale-cleanup's enumeration did not recognise
// (or vice versa), cleanup would delete the very hook registration just stored —
// reintroducing the orphan bug at scale (spec § "Risks → Missed key-producing
// site"). Because both sites read the SAME tmux.HookKeyFormat but through
// different tmux verbs (registration via display-message, cleanup via
// list-panes -a -F), only a real server exercising both verbs against one
// session population can prove the two derivations coincide end to end.
//
// The assertions are byte-identity (exact string equality via slices.Contains),
// NOT structural equivalence: the registration key produced by ResolveHookKey
// must appear literally in the ListAllPaneHookKeys slice. Membership (not
// whole-slice equality) is used so the harness's anchor/bootstrap panes do not
// break the test — only that each registration key IS present in the cleanup
// enumeration matters.
//
// This task covers the LIVE half of the cross-site consistency requirement
// (registration read == cleanup enumeration). The restore-baker leg (HookKey
// from saved state) is Phase 3. Like the other real-tmux guards in this package
// the file carries NO build tag and is gated only by SkipIfNoTmux(t) so
// tmux-less environments skip cleanly rather than fail.
//
// The harness runs -f /dev/null, so base-index and pane-base-index default to
// 0 — hence the ":0.0" / ":w.p" suffixes below.
//
// Spec: .workflows/session-rename-orphans-resume-hook/specification §
// "Hook-Key Derivation (central invariant)" and § "Testing Requirements →
// Cross-site consistency", § "Risks → Missed key-producing site".

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestCrossSiteConsistency_StampedSession proves the registration read and the
// cleanup enumeration agree on a stamped single-pane session's hook key. It
// stamps @portal-id=tok123, resolves the registration key via ResolveHookKey
// (targeting the session name resolves against its single active pane → w0.p0),
// then enumerates every live hook key via ListAllPaneHookKeys and asserts the
// registration key is both the expected "tok123:0.0" AND a byte-identical member
// of the cleanup enumeration.
func TestCrossSiteConsistency_StampedSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-xsite-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const sessionName = "xsite-stamped"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	if err := client.SetSessionOption(sessionName, portalIDLiteral, "tok123"); err != nil {
		t.Fatalf("SetSessionOption(%q, %q, %q): %v", sessionName, portalIDLiteral, "tok123", err)
	}

	// Registration read: display-message against the session's active pane.
	reg, err := client.ResolveHookKey(sessionName)
	if err != nil {
		t.Fatalf("ResolveHookKey(%q): %v", sessionName, err)
	}
	if reg != "tok123:0.0" {
		t.Fatalf("registration key = %q, want %q (conditional must take the @portal-id branch)", reg, "tok123:0.0")
	}

	// Cleanup enumeration: list-panes -a -F across the whole server.
	live, err := client.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}

	// Byte-identity: the exact registration key must appear in the cleanup
	// enumeration. Disagreement here means cleanup would orphan the just-stored
	// registration.
	if !slices.Contains(live, reg) {
		t.Errorf("registration key %q not found byte-identically in cleanup enumeration %v (the two live sites disagree)", reg, live)
	}
}

// TestCrossSiteConsistency_MultiPaneStampedSession proves the two sites agree on
// each pane's distinct hook key across a multi-window/multi-pane stamped session.
// It splits the initial pane and adds a second window (yielding w0.p0, w0.p1,
// w1.p0), obtains each pane's #{pane_id}, resolves each pane's registration key
// via ResolveHookKey(paneID), then asserts every per-pane registration key is a
// byte-identical member of the single ListAllPaneHookKeys() slice — all sharing
// the tok123 prefix with distinct w.p suffixes.
func TestCrossSiteConsistency_MultiPaneStampedSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-xsite-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// Seed the shared 3-pane stamped fixture, then resolve each pane's
	// registration key by its concrete #{pane_id} target so each per-pane
	// registration read is unambiguous (no active-pane resolution ambiguity).
	paneIDs := seedThreePaneStampedSession(t, ts, client, "xsite-multi", "tok123")
	if len(paneIDs) != 3 {
		t.Fatalf("expected 3 panes (w0.p0, w0.p1, w1.p0), got %d: %v", len(paneIDs), paneIDs)
	}

	// Cleanup enumeration: a single list-panes -a -F read for all per-pane
	// membership assertions.
	live, err := client.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}

	var regKeys []string
	for _, paneID := range paneIDs {
		reg, err := client.ResolveHookKey(paneID)
		if err != nil {
			t.Fatalf("ResolveHookKey(%q): %v", paneID, err)
		}
		regKeys = append(regKeys, reg)

		// Every per-pane registration key must share the id prefix (rename-immune)
		// — a name-based key here would prove one site diverged.
		if !strings.HasPrefix(reg, "tok123:") {
			t.Errorf("per-pane registration key %q for pane %q does not share the tok123 prefix", reg, paneID)
		}
		// Byte-identity: this pane's registration key must appear verbatim in the
		// cleanup enumeration.
		if !slices.Contains(live, reg) {
			t.Errorf("per-pane registration key %q not found byte-identically in cleanup enumeration %v (the two live sites disagree)", reg, live)
		}
	}

	// The three per-pane registration keys must be distinct (distinct w.p
	// suffixes under the one shared id) — a collision would mean two panes
	// resolved to the same key, masking a suffix bug.
	if distinct := uniqueCount(regKeys); distinct != 3 {
		t.Errorf("expected 3 distinct per-pane registration keys, got %d from %v", distinct, regKeys)
	}
}

// TestCrossSiteConsistency_UnstampedSession proves the two sites agree on the
// name-based fallback key for an un-stamped session (the no-migration
// coincidence). It creates a session WITHOUT stamping @portal-id; the
// registration read must resolve to "<sessionName>:0.0" and that exact string
// must be a byte-identical member of the cleanup enumeration.
func TestCrossSiteConsistency_UnstampedSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-xsite-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const sessionName = "xsite-unstamped"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	// Deliberately do NOT stamp @portal-id — both sites must fall back to the
	// #{session_name} branch and still coincide.
	want := sessionName + ":0.0"

	reg, err := client.ResolveHookKey(sessionName)
	if err != nil {
		t.Fatalf("ResolveHookKey(%q): %v", sessionName, err)
	}
	if reg != want {
		t.Fatalf("un-stamped registration key = %q, want %q (unset @portal-id must take the #{session_name} branch)", reg, want)
	}

	live, err := client.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}
	if !slices.Contains(live, reg) {
		t.Errorf("un-stamped registration key %q not found byte-identically in cleanup enumeration %v (the two live sites disagree)", reg, live)
	}
}

// sessionPaneIDs (the shared 3-pane fixture's pane-id reader) lives in
// hookkey_realtmux_shared_test.go.

// uniqueCount returns the number of distinct strings in s.
func uniqueCount(s []string) int {
	seen := make(map[string]struct{}, len(s))
	for _, v := range s {
		seen[v] = struct{}{}
	}
	return len(seen)
}
