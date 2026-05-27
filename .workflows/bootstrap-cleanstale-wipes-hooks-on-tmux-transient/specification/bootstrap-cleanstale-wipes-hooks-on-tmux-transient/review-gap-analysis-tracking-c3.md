---
status: in-progress
created: 2026-05-27
cycle: 3
phase: Gap Analysis
topic: bootstrap-cleanstale-wipes-hooks-on-tmux-transient
---

# Review Tracking: bootstrap-cleanstale-wipes-hooks-on-tmux-transient - Gap Analysis

## Findings

### 1. `Load()` error path — bootstrap adapter Warn breadcrumb unspecified, asymmetric with `portal clean`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 3 → "`Load()` error handling" paragraph (line 170); Acceptance Criteria #4 (line 287); Change 4 → terminal-line enumeration (lines 202-206)

**Details**:
The spec covers the `Load()` error path explicitly for `portal clean` ("emit a `Warn` breadcrumb before the existing error return") but leaves the bootstrap-adapter side silent. The adapter prose says only: *"return the error directly (no `hookStore.CleanStale` call), letting the orchestrator surface it as a soft warning."*

Acceptance Criteria #4 enumerates the log shapes for three paths — normal completion, hazard-guard skip, and the enumeration-error path ("propagated error from `ListAllPanesWithFormat`") — but does not cover the `Load()`-error path symmetrically. Change 4's terminal-line list at lines 202-206 only enumerates a `Warn` "on propagated error from `ListAllPanesWithFormat`" — not `Load()`.

An implementer is left to guess one of three behaviours for the bootstrap adapter on `Load()` failure:

1. Silent return of the error (matches the literal "return the error directly" prose, but inverts the "every invocation logs at least one breadcrumb" property and creates a second silent-destructive-context fingerprint analogous to the original bug).
2. Emit a `Warn` breadcrumb mirroring `portal clean` (symmetric, matches the "single auditable destructive-callsite log stream" intent stated at line 184), and then return the error.
3. Emit the entry-point `Debug` with `live=<N> persisted=<unknown>` before returning the error (likely wrong — entry-point Debug is documented as firing only after both `ListAllPanes + Load` succeed).

Given the spec's stated intent of a "single auditable destructive-callsite log stream" and the explicit `portal clean` Warn breadcrumb on `Load()` error, option (2) is the most likely intended behaviour, but the spec does not say so. This is material because Acceptance Criteria #6 requires the test coverage matrix to assert log breadcrumb shapes; absent an explicit rule, the new `cmd/bootstrap_production_test.go` would either skip the `Load()`-error path entirely or codify whichever option the implementer chose.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
