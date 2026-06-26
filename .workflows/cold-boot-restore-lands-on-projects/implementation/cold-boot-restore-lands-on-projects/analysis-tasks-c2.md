---
topic: cold-boot-restore-lands-on-projects
cycle: 2
total_proposed: 1
---
# Analysis Tasks: Cold-boot Restore Lands on Projects (Cycle 2)

## Task 1: Consolidate the restored-sessions fixture and visible-names assertion in coldboot_session_refetch_test.go
status: pending
severity: low
sources: duplication

**Problem**: In `internal/tui/coldboot_session_refetch_test.go`, two paired literals introduced by this implementation repeat across the cold-route test cluster. (1) The two-element restored-sessions fixture `[]tmux.Session{{Name: "restored-alpha", Windows: 1}, {Name: "restored-bravo", Windows: 2}}` is declared verbatim in 5 tests (lines ~210-213, ~315-316, ~446-448, ~669-671, ~742-744). (2) The post-landing verification block — `got := visibleSessionNames(final); want := []string{"restored-alpha", "restored-bravo"}; if len(got) != len(want) { t.Fatalf(...) }; for i := range want { if got[i] != want[i] { t.Errorf(...) } }` — is a 9-10 line block repeated verbatim in 4 tests (lines ~228-237, ~336-345, ~709-718, ~797-806). The `want` slice in each block restates the same two names already encoded in the shared `restored` fixture, so the two literals must be kept in sync by hand; an edit to one expected name silently drifts the assertion from the fixture. This file already establishes the consolidation convention via `oneProjectLoaded()` (cycle 1), whose doc comment states the goal explicitly: "keeps the literal from drifting across the cluster of tests that deliver it." The restored fixture + its paired assertion are the un-consolidated parallel of that exact case.

**Solution**: Mirror the existing `oneProjectLoaded()` helper. Add a sibling test-file helper that returns the shared restored fixture, and a small assertion helper that wraps the len-check + index loop. Route the five fixture sites and four assertion blocks through the new helpers so the alpha/bravo names derive from a single source.

**Outcome**: The two-session restored fixture and its expected-names list have exactly one definition each. The five `restored := []tmux.Session{...}` declarations and the four verbatim verification blocks collapse to single helper calls, eliminating the hand-sync hazard between fixture and assertion. Tests continue to build and pass with behaviour unchanged.

**Do**:
1. Add a test-file helper, e.g. `twoRestoredSessions() []tmux.Session`, returning `[]tmux.Session{{Name: "restored-alpha", Windows: 1}, {Name: "restored-bravo", Windows: 2}}`, with a doc comment mirroring the `oneProjectLoaded()` rationale (single source of the literal so it cannot drift across the cluster).
2. Replace each of the 5 verbatim `restored := []tmux.Session{...}` declarations (TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions, _NPositive_LandsOnSessions, _InitialFilter_RoutesToSessions, _InterimPage_IsValidSessions, _LateProjectsLoadedMsg_StillLandsOnSessions) with a call to the new fixture helper.
3. Add a small assertion helper, e.g. `assertVisibleSessionNames(t *testing.T, m <model type>, want []string)`, wrapping the `got := visibleSessionNames(...)` len-check (`t.Fatalf`) + index loop (`t.Errorf`), preserving the existing failure-message intent.
4. Replace each of the 4 verbatim `got/want` verification blocks (lines ~228-237, ~336-345, ~709-718, ~797-806) with a single call to the assertion helper, passing the expected names. Source the expected names from a single place so they track the fixture (e.g. derive `want` from the fixture's names, or keep one shared `want` slice alongside the fixture helper).
5. Leave the `New(...)` construction blocks as-is — they vary meaningfully per test (WithProjectStore / WithInitialFilter / WithCommand / WithProgressReceiver) and consolidating them would be premature abstraction, not duplication removal.
6. Do not add `t.Parallel()` (CLAUDE.md mandates no `t.Parallel()` for the cmd/tui mutable-state test surfaces).

**Acceptance Criteria**:
- The literal `{Name: "restored-alpha", Windows: 1}, {Name: "restored-bravo", Windows: 2}` fixture appears in exactly one place (the new helper); the 5 prior inline declarations are gone.
- The 9-10 line `got/want` visible-names verification block appears in exactly one place (the new assertion helper); the 4 prior inline blocks are gone, each replaced by a single helper call.
- The expected `restored-alpha`/`restored-bravo` names derive from a single source shared with the fixture, so editing one name cannot silently drift fixture from assertion.
- Per-test `New(...)` construction blocks are unchanged.
- Behaviour of every affected test is unchanged (same assertions, same failure semantics); no production code is modified.

**Tests**:
- Run `go test ./internal/tui/...` and confirm all cold-route tests in `coldboot_session_refetch_test.go` still build and pass after the consolidation.
- Manually verify (or via a deliberate temporary edit) that changing a name in the new fixture helper propagates to the assertions through the single shared source, confirming the hand-sync hazard is removed.
