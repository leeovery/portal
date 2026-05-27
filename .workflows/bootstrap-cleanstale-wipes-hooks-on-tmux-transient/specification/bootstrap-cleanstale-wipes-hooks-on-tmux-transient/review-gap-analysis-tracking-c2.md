---
status: in-progress
created: 2026-05-27
cycle: 2
phase: Gap Analysis
topic: bootstrap-cleanstale-wipes-hooks-on-tmux-transient
---

# Review Tracking: bootstrap-cleanstale-wipes-hooks-on-tmux-transient - Gap Analysis

## Findings

### 1. Stale "Promoted Parser Coverage" test-requirements section contradicts Change 2

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Requirements → "Promoted Parser Coverage" (lines 244-246); Coverage Matrix row "Promoted parser" (line 273)

**Details**:
Change 2 (lines 150-154) explicitly locks the disposition: `parseLivePaneSet` is **not** promoted — it remains the marker-side parser, and `parsePaneOutput` is reused inside the repurposed `ListAllPanes`. No parser is moved by this work unit.

However, the Test Requirements section retains a conditional clause:

> *"If `parseLivePaneSet` is moved out of `cmd/bootstrap/stale_marker_cleanup.go`, the existing coverage in `cmd/bootstrap/stale_marker_cleanup_test.go` must move or be duplicated to live next to the new location."*

…and the Coverage Matrix lists a "Promoted parser" row referencing the move/duplicate of test coverage. Both are leftovers from a pre-cycle-1-correction shape of the fix that no longer applies.

An implementer reading this would either (a) be momentarily confused about whether a parser move is happening, or (b) treat the conditional as live and look for a parser-promotion task that does not exist in the planning surface. Minor planning hazard but easy to remove.

**Current**:
> *Test Requirements line 244-246:*
> ### Promoted Parser Coverage
>
> If `parseLivePaneSet` is moved out of `cmd/bootstrap/stale_marker_cleanup.go`, the existing coverage in `cmd/bootstrap/stale_marker_cleanup_test.go` must move or be duplicated to live next to the new location.
>
> *Coverage Matrix row (line 273):*
> | Promoted parser | Moved/duplicated from `stale_marker_cleanup_test.go` |

**Proposed Addition**:
*(Pending discussion — remove both the "Promoted Parser Coverage" subsection and the matrix row, since Change 2 locks that no promotion occurs.)*

**Resolution**: Pending
**Notes**:

---

### 2. `portal clean` early-exit conflicts with "every invocation emits two log lines" contract

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 3 → step 1 (line 166); Change 4 (lines 188-204); Acceptance Criteria #4 (line 286)

**Details**:
Change 3 step 1 notes that `portal clean` *"already calls `hookStore.Load()` (line 65) and exits early when empty (line 71-73)."* This early-exit happens **before** `ListAllPanes` is invoked.

Change 4 specifies that *"every invocation emits **exactly two log lines**: one entry-point line (after enumeration so both counts are known) and one terminal line."* Acceptance Criteria #4 reinforces this as a binding contract: *"every invocation of `cleanStaleAdapter.CleanStale` and the `portal clean` hook tail emits exactly two log lines on the success-of-enumeration paths."*

These two statements are inconsistent for the `portal clean` callsite in the `persisted == 0` case:

- If `portal clean` retains its early-exit, then on the `persisted == 0` branch **zero log lines** are emitted (enumeration never runs, so the entry-point Debug never fires; no terminal line fires because there was no work). This contradicts Acceptance Criteria #4.
- If `portal clean` is restructured to defer the early-exit until after enumeration, that is a behavioural change the spec does not call out and which inverts the helpful "no tmux server → no error" ergonomics that Change 1's Disposition rationale (line 146) and Out of Scope (lines 294) already discuss.

The bootstrap adapter has no such collision — it currently calls `ListAllPanes` first, and the spec adds a `Load()` after that, so the entry-point Debug naturally fires after both counts are known. The collision is `portal clean`-specific.

Resolution options the spec should pick between:
- **(i)** Keep the `portal clean` early-exit; carve an explicit exception into Acceptance Criteria #4 and Change 4 ("when `persisted == 0` at `portal clean`, no log lines are emitted").
- **(ii)** Remove the `portal clean` early-exit; both counts always logged. (More uniform but loses the no-tmux-server ergonomics.)
- **(iii)** Emit a single Debug ("nothing to do; persisted == 0") at the early-exit before returning, so every invocation still gets at least one breadcrumb. Closest to the spirit of "every invocation logs."

**Current**:
> *Change 3 step 1 (line 166):*
> *"`portal clean` already calls `hookStore.Load()` (line 65) and exits early when empty (line 71-73), so this read is already paid for at that callsite. The bootstrap adapter must add the `Load()` call."*
>
> *Acceptance Criteria #4 (line 286):*
> *"every invocation of `cleanStaleAdapter.CleanStale` and the `portal clean` hook tail emits exactly two log lines on the success-of-enumeration paths…"*

**Proposed Addition**:
*(Pending — pick (i), (ii), or (iii) above and amend Change 4 + Acceptance Criteria #4 to match. If (i), name the exception explicitly so an implementer doesn't add the early-exit Debug "to be safe.")*

**Resolution**: Pending
**Notes**:

---

### 3. `hookStore.Load()` error handling at new bootstrap-adapter callsite is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 3 → step 1 (line 166); Soft-warning surfacing contract (line 182)

**Details**:
Change 3 step 1 instructs the bootstrap adapter to *"Load the persisted hooks via `hookStore.Load()`… The bootstrap adapter must add the `Load()` call."* This is a new caller of `Load()` that did not previously exist.

The spec is silent on what the adapter does when `Load()` returns a non-nil error (e.g., disk read failure, JSON parse error on a corrupt `hooks.json`). Three reasonable interpretations:

- Surface as soft warning and skip (same as a `ListAllPanesWithFormat` error).
- Treat as `len(persisted) == 0` and let the hazard guard decide.
- Treat as `len(persisted) == unknown` and skip the destructive call (extra-conservative).

Without specification an implementer must guess. The choice is consequential: a corrupt `hooks.json` interpreted as "empty persisted" would let a normal-path stale removal proceed and overwrite the corrupt file with `{}` — silently destroying recoverable hook state, the exact failure class this fix is closing.

The Soft-warning surfacing contract paragraph (line 182) covers the `ListAllPanesWithFormat` error path but does not mention `Load()` errors.

**Current**:
> *Change 3 step 1 (line 166):*
> *"Load the persisted hooks via `hookStore.Load()` (already a public method on `*hooks.Store`); this returns the current `hooksFile` map. Use `len(persisted) > 0` as the guard's right-hand condition. No new API on `internal/hooks/store.go` is required. `portal clean` already calls `hookStore.Load()` (line 65) and exits early when empty (line 71-73), so this read is already paid for at that callsite. The bootstrap adapter must add the `Load()` call."*

**Proposed Addition**:
*(Pending — name the `Load()` error path explicitly. Recommended default: treat `Load()` non-nil error the same as `ListAllPanesWithFormat` non-nil error — adapter returns it, orchestrator surfaces as soft warning, no destructive call. This matches the "treat empty as unknown" principle and avoids the corrupt-file-overwrite hazard.)*

**Resolution**: Pending
**Notes**:

---

### 4. `ListAllPanes` helper docstring rewrite is unmentioned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 1 (lines 125-148); Root Cause → "Why This Happens" (line 90-91)

**Details**:
Change 3 explicitly calls out a docstring rewrite for `cleanStaleAdapter.CleanStale` (lines 184-185) because the existing prose describes a contract that the fix invalidates. The same condition holds for the `ListAllPanes` helper itself:

The Root Cause section (line 90-91) cites the helper's existing docstring as describing the swallow as *"a convenience for the 'no tmux server' case."* Post-Change-1 the swallow is gone; the helper propagates errors uniformly. The existing docstring describing the no-server-convenience contract is therefore actively misleading after the fix lands, in the same way the `cleanStaleAdapter` docstring is.

Change 1's "Disposition rationale" paragraph reasons about the contract change but does not direct the implementer to rewrite the docstring on `ListAllPanes` itself. A pedantic implementer might land the contract change while leaving the helper docstring intact, then surface as a code-review nit later.

Minor — but Change 3's docstring-rewrite call-out sets a precedent that the spec is willing to be explicit about, and inconsistency between the two changes (Change 1 silent, Change 3 explicit) is itself a small clarity gap.

**Current**:
> *Change 1 (lines 125-148) — no docstring-rewrite directive.*
>
> *Compare Change 3 line 184:*
> *"the existing docstring on `cleanStaleAdapter.CleanStale` at `cmd/bootstrap_production.go:71-75` reads: … Post-fix this is actively misleading … The docstring must be rewritten alongside the code change…"*

**Proposed Addition**:
*(Pending — add a "Docstring rewrite" bullet under Change 1 directing the implementer to rewrite `ListAllPanes`'s docstring to describe the new error-propagating contract and remove the no-server-convenience framing. Mirrors the Change 3 directive.)*

**Resolution**: Pending
**Notes**:

---
