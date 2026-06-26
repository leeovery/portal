---
topic: cold-boot-restore-lands-on-projects
cycle: 1
total_proposed: 1
---
# Analysis Tasks: Cold-boot Restore Lands on Projects (Cycle 1)

## Task 1: Extract a shared cold-route driver that delivers ProjectsLoadedMsg before the loading transition
status: approved
severity: medium
sources: duplication

**Problem**: Four AC landing-page tests in `internal/tui/coldboot_session_refetch_test.go`
(`TestColdBoot_NPositive_LandsOnSessions` ~lines 264-287, `TestColdBoot_ZeroSessions_LandsOnProjects`
~lines 369-390, `TestColdBoot_InitialFilter_RoutesToSessions` ~lines 435-455,
`TestColdBoot_InterimPage_IsValidSessions` ~lines 674-692) each re-inline the identical ~13-line
cold-route drive sequence: `WindowSizeMsg` → stale `SessionsMsg` + "expected PageLoading after stale
SessionsMsg" invariant → `ProjectsLoadedMsg{Projects: []project.Project{{Path: "/p/one", Name: "one"}}}`
+ "expected PageLoading after ProjectsLoadedMsg" invariant → `LoadingMinElapsedMsg` →
`BootstrapCompleteMsg` → `drainBatchToModel`. The bodies are byte-identical apart from the
per-test post-drain assertions. The existing `driveColdBootToSessions` helper (lines 70-99) was
deliberately NOT reused because it omits the mandatory `ProjectsLoadedMsg` delivery the spec's
Testing Requirements demand ("A regression test MUST deliver ProjectsLoadedMsg before the
loading-page transition"). The result is four independent transcriptions of the same mandatory
ordering that can silently diverge — e.g. one test dropping the pre-transition invariant check,
or the stub project path drifting — weakening the regression guard. The same
`{Path: "/p/one", Name: "one"}` project literal is also hand-written across six tests.

**Solution**: Extract one shared driver in `coldboot_session_refetch_test.go` that encapsulates
the `ProjectsLoadedMsg`-before-transition cold-route sequence and returns the post-drain `Model`,
so the mandatory ordering lives in a single source of truth. Fold the repeated project-record
literal into the driver (and/or a small `oneProjectLoaded()` helper) so the four-test cluster
stops hand-writing it. Because `TestColdBoot_InterimPage_IsValidSessions` needs the pre-drain
interim model, expose the interim model / `completeCmd` from the driver (return them, or split the
driver into a "drive to transition" half and a "drain refetch" half) so that test can assert the
interim page between the two halves.

**Outcome**: Each of the four AC tests reduces to a setup, one driver call, and its distinguishing
assertions. The spec-mandated `ProjectsLoadedMsg`-before-transition ordering and its two
pre-transition `PageLoading` invariants are pinned in exactly one place and cannot drift across
task boundaries. The repeated project literal is gone from the four-test cluster. The full
cold-boot test suite still passes with identical behavioural coverage.

**Do**:
1. In `internal/tui/coldboot_session_refetch_test.go`, add a new helper alongside the existing
   `driveColdBootToSessions` — e.g. `driveColdBootWithProjects(t *testing.T, m Model, stale []tmux.Session) Model`
   (mark it `t.Helper()`). It must perform, in order: `WindowSizeMsg{Width: 80, Height: 24}`; the
   stale `SessionsMsg{Sessions: stale}` followed by the "expected PageLoading after stale SessionsMsg"
   invariant; `ProjectsLoadedMsg{Projects: []project.Project{{Path: "/p/one", Name: "one"}}}` followed
   by the "expected PageLoading after ProjectsLoadedMsg" invariant; `LoadingMinElapsedMsg{}`;
   `BootstrapCompleteMsg{}`; then `drainBatchToModel` on the resulting model + cmd, returning the
   drained `Model`.
2. To support `TestColdBoot_InterimPage_IsValidSessions` (which asserts the interim page between
   the `BootstrapCompleteMsg` transition and the refetch drain), either (a) split the driver into a
   "drive to transition" half that returns the interim model + `completeCmd` and a separate
   "drain refetch" half, or (b) have the combined driver also return the interim model/`completeCmd`.
   Pick whichever keeps the four call sites cleanest; preserve the existing interim-page assertion
   exactly.
3. Replace the inlined sequences in `TestColdBoot_NPositive_LandsOnSessions`,
   `TestColdBoot_ZeroSessions_LandsOnProjects`, `TestColdBoot_InitialFilter_RoutesToSessions`, and
   `TestColdBoot_InterimPage_IsValidSessions` with a setup + one driver call + their existing
   distinguishing post-drain assertions. Do not change any test's assertions or its observable
   intent.
4. Fold the `{Path: "/p/one", Name: "one"}` project literal into the driver (it now owns the
   `ProjectsLoadedMsg` delivery for the four-test cluster). Optionally add a tiny file-level
   `oneProjectLoaded()` helper (or shared var) and reuse it for the remaining standalone deliveries
   (`TestColdBoot_LateProjectsLoadedMsg_StillLandsOnSessions`, `TestWarmRoute_ZeroSessions_LandsOnProjects`,
   `TestCommandPending_LandsOnProjects_NoInterimFlash`, `TestColdBoot_RefetchError`) only if it reads
   cleanly — this is a nice-to-have, not required.
5. Leave the existing `driveColdBootToSessions` helper untouched — other tests that legitimately do
   not deliver `ProjectsLoadedMsg` still use it.

**Acceptance Criteria**:
- A single shared driver in `coldboot_session_refetch_test.go` performs the cold-route sequence
  including the `ProjectsLoadedMsg` delivery and both pre-transition `PageLoading` invariants; the
  four named AC tests call it instead of re-inlining the sequence.
- The mandatory "deliver `ProjectsLoadedMsg` before the loading-page transition" ordering and its
  two pre-transition invariants exist in exactly one place in the file.
- Every previously-passing assertion in the four refactored tests is preserved unchanged, including
  the interim-page assertion in `TestColdBoot_InterimPage_IsValidSessions`.
- The `{Path: "/p/one", Name: "one"}` literal no longer appears inline in the four-test cluster.
- No production code is modified; the existing `driveColdBootToSessions` helper is unchanged.

**Tests**:
- Run `go test ./internal/tui/...` (the full cold-boot suite) and confirm all tests pass with the
  refactor in place — coverage and behaviour must be identical to before.
- Confirm `go build ./...` succeeds and `golangci-lint run ./internal/tui/...` is clean (no unused
  helper, no shadowing).
