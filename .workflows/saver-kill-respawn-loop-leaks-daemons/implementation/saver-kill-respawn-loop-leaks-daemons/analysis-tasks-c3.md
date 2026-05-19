---
topic: saver-kill-respawn-loop-leaks-daemons
cycle: 3
total_proposed: 1
---
# Analysis Tasks: saver-kill-respawn-loop-leaks-daemons (Cycle 3)

## Task 1: Extract version-scenario and barrier-count test helpers in portal_saver_test.go
status: approved
severity: low
sources: duplication

**Problem**: Two verbatim boilerplate patterns have accumulated in `internal/tmux/portal_saver_test.go` as the version-matrix coverage grew:
1. A 3-line `scenario / mock / client` triplet repeated 24 times from ~:1336 onward (~72 LOC of pure duplication, zero variation).
2. A 4-line `barrierCalls := 0; installKillSaverFn(t, func(...) { barrierCalls++; return nil })` block repeated 12 times around :1659+ (~36 LOC), capturing only the call count.

Aggregate ~108 LOC of identical boilerplate obscures the per-test intent and inflates diffs whenever the seam shape changes.

**Solution**: Add two composable test-only helpers in the same file (or a sibling `_test.go` helper file in package `tmux_test`):
1. `newVersionScenarioClient(t *testing.T, sessionPresent bool) (*versionScenario, *MockCommander, *tmux.Client)` — collapses the triplet to one line per call site.
2. `recordBarrierCalls(t *testing.T) *int` — installs the kill-saver seam via `installKillSaverFn` and returns a pointer to the increment counter.

Both compose: a typical version-matrix test becomes two helper calls instead of seven lines of setup.

**Outcome**:
- `portal_saver_test.go` shrinks by ~100 LOC with no behaviour change.
- Every version-matrix test starts with a one-line scenario/client setup; every barrier-count assertion starts with a one-line counter install.
- `go test ./internal/tmux/...` passes identically (same assertions, same coverage).

**Do**:
1. Read `/Users/leeovery/Code/portal/internal/tmux/portal_saver_test.go` and identify the 24 triplet call sites (search for `versionScenario{sessionPresent`) and the 12 barrier-count sites (search for `barrierCalls := 0`).
2. Add `newVersionScenarioClient` near the existing `versionScenario` type definition. Body:
   ```go
   func newVersionScenarioClient(t *testing.T, sessionPresent bool) (*versionScenario, *MockCommander, *tmux.Client) {
       t.Helper()
       scenario := &versionScenario{sessionPresent: sessionPresent}
       mock := &MockCommander{RunFunc: scenario.run(t)}
       return scenario, mock, tmux.NewClient(mock)
   }
   ```
3. Add `recordBarrierCalls` near the existing `installKillSaverFn` usage:
   ```go
   func recordBarrierCalls(t *testing.T) *int {
       t.Helper()
       calls := 0
       installKillSaverFn(t, func(*tmux.Client, string) error {
           calls++
           return nil
       })
       return &calls
   }
   ```
4. Replace all 24 triplet sites with `scenario, mock, client := newVersionScenarioClient(t, true /* or false */)`. Preserve the existing `sessionPresent` boolean per site.
5. Replace all 12 barrier-count sites: change `barrierCalls := 0; installKillSaverFn(...)` to `barrierCalls := recordBarrierCalls(t)`. Update downstream assertions from `barrierCalls` to `*barrierCalls` (pointer deref).
6. Run `go test ./internal/tmux/...` and confirm pass count is unchanged.
7. Run `go vet ./...` to catch any stray unused-variable issues from the replacement.

**Acceptance Criteria**:
- `newVersionScenarioClient` and `recordBarrierCalls` are defined exactly once in the test file.
- Zero remaining occurrences of the literal `&versionScenario{sessionPresent:` followed by a `&MockCommander{RunFunc: scenario.run` line in the file.
- Zero remaining occurrences of `barrierCalls := 0` followed by an `installKillSaverFn` with a counter-increment closure.
- `go test ./internal/tmux/...` passes with the same number of test cases as before the refactor.
- Net LOC delta for `portal_saver_test.go` is negative (target: ~-90 LOC after helper definitions are added back).

**Tests**:
- No new tests. Existing version-matrix and barrier-count tests are the regression net — they must continue to pass byte-identically in their assertions.
- Spot-check one migrated site of each kind by reading the diff to confirm semantic equivalence.
