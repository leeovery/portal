---
status: complete
created: 2026-05-08
cycle: 1
phase: Gap Analysis
topic: daemon-merge-reintroduces-dead-sessions
---

# Review Tracking: daemon-merge-reintroduces-dead-sessions - Gap Analysis

## Findings

### 1. Daemon tick interval contradiction (1s vs ≤30s)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Impact (line 29), Reproduction Steps (line 228), Code Context / Affected Code Path (line 190)

**Details**:
The spec contradicts itself on the daemon tick cadence. Code Context states the tick "fires every 1s in the `_portal-saver` daemon", while Impact and Reproduction Steps say "≤30s". The 1s figure is the daemon's `TickerPeriod`; the 30s figure is `MaxGap` (forced-save fallback). These are different quantities.

**Resolution**: Approved
**Notes**: Reconciled. Impact now states "~1s under normal load" with a note that ≤30s refers to `MaxGap`. Reproduction Steps similarly disambiguated.

---

### 2. Bootstrap step insertion description is internally contradictory

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Location

**Details**:
Headline said "between current step 5 (Restore) and step 7 (SweepOrphanFIFOs) — making it the new step 6", which conflicts with the existing step 6 ("Clear `@portal-restoring`"). Corrected note clarified but headline remained misleading.

**Resolution**: Approved
**Notes**: Headline rewritten: "between current step 6 (Clear `@portal-restoring`) and step 7 (SweepOrphanFIFOs) — becoming the new step 7".

---

### 3. Seam interface for marker cleanup is unnamed and unsigned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Adapter Wiring

**Details**:
Three responsibilities listed (marker enumeration, live pane enumeration, marker unset) but no interface name or method signatures specified.

**Resolution**: Approved
**Notes**: Adapter Wiring section rewritten with recommended seam name (`StaleMarkerCleaner` or similar), three independently-mockable methods, and explicit guidance that whether to compose them as one or three interfaces is an implementation choice consistent with bootstrap conventions.

---

### 4. "or equivalent live read" leaves marker enumeration source ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Adapter Wiring

**Details**:
"Or equivalent live read" left it unclear whether to use `state.ListSkeletonMarkers` or author a new function.

**Resolution**: Approved
**Notes**: Pinned to `state.ListSkeletonMarkers` as the canonical source. Documented its return type (`map[string]struct{}` keyed by paneKey, prefix-stripped).

---

### 5. Live pane enumeration method on `*tmux.Client` unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Adapter Wiring

**Details**:
`*tmux.Client` exposes multiple pane-listing methods; the spec didn't specify which one or what its output format is.

**Resolution**: Approved
**Notes**: Pinned to `(*tmux.Client).ListAllPanes()` returning `[]string` of `session:window.pane` form, with explicit guidance that each entry must be converted to canonical paneKey via `state.SanitizePaneKey(session, window, pane)` before set-difference.

---

### 6. PaneKey extraction from marker option name not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Behavior

**Details**:
Whether `ListSkeletonMarkers` returned paneKeys or option names was unclear, and the parsing/normalisation contract was not stated.

**Resolution**: Approved
**Notes**: Documented that `ListSkeletonMarkers` already returns paneKeys (prefix stripped), so no marker-side parsing is needed. Live-pane side requires `SanitizePaneKey` conversion. Marker unset uses full option name `@portal-skeleton-<paneKey>` via `UnsetServerOption`.

---

### 7. `mergeSkippedPanes` signature change is implied but undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component A / Filtering Levels, Code Context / Contributing Factors

**Details**:
Two reasonable approaches existed: thread `keep` as a new parameter, or build the structural map locally from `idx.Sessions`. The spec didn't pick one.

**Resolution**: Approved
**Notes**: Added a "Data Flow / Signature Approach" subsection picking option (b): build the structural map locally inside `mergeSkippedPanes` from `idx.Sessions`. No external signature change to callers; helper may be added internally.

---

### 8. Concurrency between bootstrap cleanup and daemon tick is unaddressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Behavior

**Details**:
EnsureSaver starts the daemon before the cleanup runs, so concurrent ticks are possible. The spec implicitly relied on Fix Component A making this safe but didn't state it.

**Resolution**: Approved
**Notes**: Added "Concurrency with the Daemon" subsection: explicitly states no serialisation needed; Fix Component A neutralises the marker's authority over the merge.

---

### 9. SweepOrphanFIFOs ↔ marker cleanup ordering interaction not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Location

**Details**:
The new cleanup unsets stale markers immediately before SweepOrphanFIFOs runs, which means previously-protected orphan FIFOs are now eligible for sweep. Worth confirming intent.

**Resolution**: Approved
**Notes**: Added "Synergy with `SweepOrphanFIFOs`" subsection: explicitly states the compound cleanup is intentional — both halves of a stale-marker / orphan-FIFO pair are reclaimed in one bootstrap.

---

### 10. Behaviour when restore phase A leaves markers without live panes

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Behavior, Preserved Behavior

**Details**:
If restore phase A partially succeeds, the cleanup unsets markers for failed-pane cases. Spec didn't address whether this is desirable or interferes with retry paths.

**Resolution**: Approved
**Notes**: Added "Behaviour Against Partial Restore Failures" subsection: no special-casing required; "stale" is observably defined as "no live pane for this paneKey" and that definition is correct regardless of how the staleness arose.

---

### 11. Acceptance criterion 4 ignores soft-warning failure mode

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria, Soft-Warning Posture

**Details**:
Criterion 4 was an absolute snapshot assertion that would falsify on a partial cleanup that emitted a warning.

**Resolution**: Approved
**Notes**: Criterion 4 qualified to apply only "after a successful bootstrap (cleanup step did not surface a soft warning)" with explicit guidance that warnings indicate the next successful bootstrap completes the cleanup.

---

### 12. Test file locations for new bootstrap step unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements / cleanup integration, Files Touched

**Details**:
"Bootstrap tests for the new step" was vague; specific files not named.

**Resolution**: Approved
**Notes**: Pinned to `cmd/bootstrap/bootstrap_test.go` (orchestrator sequence + soft-warning) and `internal/bootstrapadapter/adapters_test.go` (production adapter wiring).

---

### 13. "Approximately N lines" estimates lack scope-bound implication

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Scope and Risk / In Scope

**Details**:
Line-count estimates risk being read as scope budgets.

**Resolution**: Approved
**Notes**: Reworded to "Estimated ~N lines" with explicit "The figure is illustrative, not a scope budget" qualifier.

---
