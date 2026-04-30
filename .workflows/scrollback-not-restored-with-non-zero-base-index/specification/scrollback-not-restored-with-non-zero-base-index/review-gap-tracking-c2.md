---
status: complete
created: 2026-04-30
cycle: 2
phase: Gap Analysis
topic: scrollback-not-restored-with-non-zero-base-index
---

# Review Tracking: scrollback-not-restored-with-non-zero-base-index - Gap Analysis

## Findings

### 1. Eviction loop error handling unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope > Part 1 > Migration mechanics

**Details**:
Migration mechanics describe scan-then-evict-then-install but do not specify behaviour when `UnsetGlobalHookAt` fails partway through eviction. AC #3's "exactly 1 entry after bootstrap" invariant could be violated silently if a partial eviction is left in place.

**Proposed Addition**:
Add an "Error handling" bullet to Migration mechanics: best-effort eviction, log WARN per failure, continue with remaining indices, proceed to install regardless (which is itself idempotent), eviction failures never abort bootstrap, retry on next bootstrap.

**Resolution**: Approved
**Notes**: Applied as a new "Error handling" bullet in Migration mechanics.

---

### 2. AC #4 not bound to a verifying test

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria #4, Testing Requirements

**Details**:
AC #2 and AC #3 received explicit test bindings in cycle 1; AC #4 has no equivalent. None of the four Testing Requirements explicitly verifies absence of the misleading WARN.

**Proposed Addition**:
Bind AC #4 to TR 2 (reboot round-trip integration test): after bootstrap with non-zero base-index, `portal.log` must contain zero lines matching the predicted=/live= pattern. Note that the pre-deletion verification step confirms symbol removal; AC #4 confirms runtime behaviour removal.

**Resolution**: Approved
**Notes**: Applied — AC #4 now bound to TR 2 with a regex assertion.

---

### 3. Eviction substring match may be too permissive

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope > Part 1 > Migration mechanics

**Details**:
Eviction predicate (`contains 'portal state signal-hydrate' AND not 'portal state signal-hydrate --'`) could match user-authored hooks that legitimately reference the subcommand. Tighten to require the full Portal-authored hook shape (specifically the `command -v portal >/dev/null 2>&1 &&` prefix).

**Proposed Addition**:
Add an "Eviction predicate (precise)" bullet requiring the `command -v portal >/dev/null 2>&1 &&` prefix in addition to the existing substring checks.

**Resolution**: Approved
**Notes**: Applied — predicate now requires the full Portal-authored hook shape.

---

### 4. Location of migration logic in the codebase is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope > Part 1, Testing Requirements item 4

**Details**:
Spec does not say whether eviction lives in `internal/tmux/hooks_register.go`, `cmd/bootstrap/`, or `internal/bootstrapadapter`. This shapes test placement and the eviction signature.

**Proposed Addition**:
Add a "Code location" bullet: migration lives in `internal/tmux/hooks_register.go` as a free function called from `RegisterPortalHooks` before the hydration-trigger register loop. Tests live in `internal/tmux/hooks_register_test.go` (or sibling).

**Resolution**: Approved
**Notes**: Applied as a "Code location" bullet in Migration mechanics.

---

### 5. Independence and shipping order of Part 1 vs Part 2 not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope (overall structure)

**Details**:
Spec says "Both are required" but does not state whether parts may ship independently/in either order or must land atomically.

**Proposed Addition**:
(N/A — see resolution)

**Resolution**: Skipped
**Notes**: User explicitly directed during construction that PR/delivery sequencing concerns are out of scope for the spec. Skipping this finding preserves that direction; how the parts are sequenced is a planning concern, not a specification concern.

---

### 6. Identification of existing tests to delete is implicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope > Part 2

**Details**:
Part 2 says "Tests covering the deleted functions" but does not enumerate the test files/functions. Likely candidates (`internal/restore/session_test.go`, `internal/restore/restore_test.go`) are unnamed; integration tests in `cmd/bootstrap/` could also reference them.

**Proposed Addition**:
Add a "Test-side audit" paragraph after Pre-deletion verification: grep `_test.go` repo-wide for references to the four symbols, list likely candidates, and require explicit removal/refactoring (no dead mocks/fixtures).

**Resolution**: Approved
**Notes**: Applied as a new "Test-side audit" paragraph in Part 2.

---
