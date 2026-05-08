---
status: in-progress
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
The spec contradicts itself on the daemon tick cadence. Line 190 (Code Context) states: "`tick` in `cmd/state_daemon.go:77` — fires every 1s in the `_portal-saver` daemon." But line 29 (Impact) and line 228 (Reproduction Steps) both characterise the propagation/repro window as "≤30s" / "Wait one daemon tick (≤30s)". An implementer reading the repro steps would expect a 30s tick; an implementer reading Code Context would expect a 1s tick. This affects test design (synthetic repro timing), acceptance criterion verification, and any mention of "self-heals on the next daemon tick" — which is either ~1s or up to 30s. Should be reconciled to a single cadence value or, if both are correct (e.g., `≤30s` is a worst-case bound while 1s is the steady cadence), explicitly stated.

**Proposed Addition**:
[Pending — confirm true tick cadence; pick one value or state both with their meanings explicitly]

**Resolution**: Pending
**Notes**:

---

### 2. Bootstrap step insertion description is internally contradictory

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Location (lines 67-69), Testing Requirements / cleanup integration (line 121)

**Details**:
Line 67 states the new step is inserted "between current step 5 (Restore) and step 7 (SweepOrphanFIFOs) — making it the new step 6". This is incorrect because the existing nine-step orchestrator already has a step 6 ("Clear `@portal-restoring`") between Restore and SweepOrphanFIFOs (per CLAUDE.md and per the spec's own clarifying note on line 69). The line-67 phrasing implies the new step replaces or precedes the existing step 6. Line 69 partially corrects this by saying "the existing 'Clear `@portal-restoring`' step remains immediately after Restore as it does today; the new cleanup runs after that and before SweepOrphanFIFOs", and line 121 says "after step 6 'Clear `@portal-restoring`', before existing step 7 SweepOrphanFIFOs". An implementer reading only line 67's headline will set up the wrong sequence; reading the corrected note resolves it but leaves the headline misleading. Recommend: rewrite line 67 to match line 69 / line 121 precisely (e.g. "inserted between current step 6 (Clear `@portal-restoring`) and step 7 (SweepOrphanFIFOs) — becoming the new step 7, with subsequent steps renumbered").

**Current**:
> **Location:** New step in the bootstrap orchestrator (`cmd/bootstrap/`), inserted **between current step 5 (Restore) and step 7 (SweepOrphanFIFOs)** — making it the new step 6, with subsequent steps renumbered (the existing "Clear `@portal-restoring`" step remains immediately after Restore as it does today; the new cleanup runs after that and before SweepOrphanFIFOs).

**Proposed Addition**:
[Pending — rewrite location prose so headline matches the corrected description]

**Resolution**: Pending
**Notes**:

---

### 3. Seam interface for marker cleanup is unnamed and unsigned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Adapter Wiring (lines 83-89)

**Details**:
"A new seam interface exposed by the bootstrap Orchestrator" is mentioned but the interface name and method signatures are not specified. Three responsibilities are listed (marker enumeration, live pane enumeration, marker unset) — should they be one interface with three methods, three small interfaces (per the project's "1-3 methods" DI convention), or composed in some other way? Without this, an implementer must invent the design unilaterally. Given the project's DI pattern of small interfaces (1-3 methods) and the existing `bootstrap` package conventions, a recommended shape would help: e.g., a single `MarkerCleaner` interface with methods `ListSkeletonMarkers`, `ListLivePaneKeys`, `UnsetSkeletonMarker`, OR three composed interfaces. Without guidance, naming and shape are an implementer design decision.

**Proposed Addition**:
[Pending — specify interface name(s) and method signatures, aligned with existing bootstrap seam conventions]

**Resolution**: Pending
**Notes**:

---

### 4. "or equivalent live read" leaves marker enumeration source ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Adapter Wiring (line 86)

**Details**:
"Marker enumeration (`state.ListSkeletonMarkers` or equivalent live read)" — "or equivalent" leaves it open whether the existing function is the source of truth or a new function should be authored. The daemon currently uses `state.ListSkeletonMarkers` (line 194). Cleanup should use the same function (single source of truth). Recommend: state explicitly that the cleanup step uses `state.ListSkeletonMarkers`, OR if a different function is needed, name it.

**Proposed Addition**:
[Pending — pin to `state.ListSkeletonMarkers` or specify the new function name]

**Resolution**: Pending
**Notes**:

---

### 5. Live pane enumeration method on `*tmux.Client` unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Adapter Wiring (line 87)

**Details**:
"Live pane enumeration (via `*tmux.Client`)" doesn't specify the method. The Client exposes `ListAllPanes`, `ListAllPanesWithFormat`, `ListPanes`, `ListPanesInSession`, etc. The paneKey format returned must match the format used in `@portal-skeleton-<paneKey>` for the set-difference to be meaningful (the marker uses `SanitizePaneKey` output per `internal/state` conventions). Without specification, the implementer must research which method yields paneKeys directly comparable to marker suffixes — this is a load-bearing detail because a format mismatch would silently break cleanup.

**Proposed Addition**:
[Pending — name the specific Client method or describe the paneKey format the cleanup expects from each side]

**Resolution**: Pending
**Notes**:

---

### 6. PaneKey extraction from marker option name not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Behavior (lines 73-77)

**Details**:
Markers are server options named `@portal-skeleton-<paneKey>`. To compute the set difference (markers whose paneKey is not present in live panes), the cleanup must extract the `<paneKey>` substring from each marker option name and normalise it to a form comparable with paneKeys derived from `ListPanes`-style output. The spec doesn't describe this parsing step, the canonical paneKey format used here, or whether `state.ListSkeletonMarkers` already returns `paneKey` strings vs full option names. If `ListSkeletonMarkers` returns option names, the implementer must strip `@portal-skeleton-` prefix; if it returns paneKeys directly, no parsing is needed. The contract should be stated explicitly in the spec or pointed to in code.

**Proposed Addition**:
[Pending — clarify whether `ListSkeletonMarkers` returns paneKeys or option names, and where the parsing/normalisation occurs]

**Resolution**: Pending
**Notes**:

---

### 7. `mergeSkippedPanes` signature change is implied but undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component A / Filtering Levels (lines 38-50), Code Context / Contributing Factors (line 238)

**Details**:
Fix Component A implies `mergeSkippedPanes` (and possibly `mergePane` / `findOrAppendSession`) needs access to the live structural truth — currently it has only `prev` and `skipSet`. Two reasonable approaches exist: (a) thread `keep` (or a derived structural map) as a new parameter from `CaptureStructure`, or (b) build the structural map locally from `idx.Sessions` (the freshly-built index, already in scope at the call site). Option (a) requires changing the function's external signature and updating callers (including tests); option (b) is internal and self-contained. The choice affects test mock surface, helper visibility, and the merge function's contract. The spec doesn't pick one. Recommend: explicit choice (likely option (b) since `idx` already contains the live truth at the call site, and line 100 of capture.go shows the call point).

**Proposed Addition**:
[Pending — pick the signature/data-flow approach, explicitly]

**Resolution**: Pending
**Notes**:

---

### 8. Concurrency between bootstrap cleanup and daemon tick is unaddressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Behavior (lines 73-77), Bootstrap step ordering (Step 4 EnsureSaver runs before new cleanup step)

**Details**:
EnsureSaver (existing step 4) starts the `_portal-saver` session that hosts the daemon. The new marker-cleanup step runs after Restore + Clear `@portal-restoring`, by which point the daemon may already be ticking. A concurrent tick could:
- Read a marker that the cleanup is about to unset (no harm — daemon would simply skip a stale entry).
- Race on the merge filter — but Fix Component A makes the merge resilient regardless.
The spec implicitly relies on Fix Component A making cleanup ordering safe, but this synergy is not stated. An implementer needs to know whether cleanup should serialise with the daemon (e.g., via a lock or by running before EnsureSaver). Most likely: no serialisation needed because Fix Component A neutralises the marker's authority. Worth stating to avoid an implementer adding unnecessary synchronisation.

**Proposed Addition**:
[Pending — note that cleanup may run concurrently with daemon ticks and explain why this is safe given Fix Component A]

**Resolution**: Pending
**Notes**:

---

### 9. SweepOrphanFIFOs ↔ marker cleanup ordering interaction not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Location (lines 65-69), Testing Requirements / cleanup integration (line 121)

**Details**:
Per CLAUDE.md, SweepOrphanFIFOs (existing step 7) cleans "orphan `hydrate-*.fifo` files whose paneKey is no longer represented by a live `@portal-skeleton-*` marker". The new cleanup unsets stale markers immediately before SweepOrphanFIFOs runs. Result: a stale marker is unset by the new step, then SweepOrphanFIFOs sees the FIFO as orphan and removes it. This is plausibly the intended effect but creates a subtle behaviour change: previously, a stale marker would protect an orphan FIFO indefinitely; now both are cleaned in the same bootstrap. The spec doesn't address whether SweepOrphanFIFOs's "no live marker" criterion still semantically holds or whether the cleanup ordering means more FIFOs are now swept per bootstrap. Worth confirming this is intentional.

**Proposed Addition**:
[Pending — note the synergy with SweepOrphanFIFOs and confirm intent]

**Resolution**: Pending
**Notes**:

---

### 10. Behaviour when restore phase A leaves markers without live panes

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B / Behavior (lines 73-77), Preserved Behavior (lines 56-58)

**Details**:
The cleanup step runs after Restore. If restore phase A partially succeeds (creates some skeletons but fails before others, or panes get killed externally between phase A and cleanup), the cleanup will unset markers for those failed-pane cases. This may be desirable (clears the stale marker promptly) or undesirable (interferes with a retry path). The spec's "Preserved Behavior" section discusses phase A only relative to the merge filter, not the new cleanup step. Worth explicitly stating cleanup's behaviour against just-failed phase-A skeletons: fine to clean (they're genuinely stale), or special-case (defer until next bootstrap)?

**Proposed Addition**:
[Pending — state the cleanup step's intended behaviour against partial-phase-A leftovers]

**Resolution**: Pending
**Notes**:

---

### 11. Acceptance criterion 4 ignores soft-warning failure mode

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria (line 135), Soft-Warning Posture (lines 79-80)

**Details**:
Criterion 4 — "After bootstrap, no `@portal-skeleton-*` marker exists for a paneKey that has no corresponding live pane" — is an absolute snapshot assertion. But Soft-Warning Posture (line 80) states cleanup is best-effort and may surface as a soft warning on tmux failure, in which case markers may legitimately remain. The criterion does not qualify for failure modes. As written, the criterion would falsify on a partial cleanup that emitted a warning. Recommend: qualify with "absent cleanup failure" / "when cleanup succeeds" or weaken the criterion.

**Current**:
> 4. After bootstrap, no `@portal-skeleton-*` marker exists for a paneKey that has no corresponding live pane.

**Proposed Addition**:
[Pending — qualify criterion 4 to align with soft-warning posture]

**Resolution**: Pending
**Notes**:

---

### 12. Test file locations for new bootstrap step unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements / cleanup integration (lines 116-122), Files Touched (lines 158-164)

**Details**:
"Bootstrap tests for the new step (sequence + soft-warning behaviour)" — the test file is not named. The bootstrap package likely has an existing orchestrator/sequence test file (e.g., `cmd/bootstrap/orchestrator_test.go`) and an adapter test file in `internal/bootstrapadapter/`. An implementer would need to find both and decide which to extend or whether to add new files. Minor but worth pinning to keep the work unit's file inventory complete and reviewable.

**Proposed Addition**:
[Pending — list the specific test files to be added or extended]

**Resolution**: Pending
**Notes**:

---

### 13. "Approximately N lines" estimates lack scope-bound implication

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Scope and Risk / In Scope (lines 153-156)

**Details**:
"Approximately 15 lines" / "Approximately 50 lines" line counts are descriptive but neither serve as acceptance criteria nor scope guards. If the implementation comes in at 100 lines for Component A, is that out of scope? Probably not — the count is illustrative. Worth either removing (to avoid implying a budget) or framing as "estimate, not a budget". Minor.

**Current**:
> - **Fix Component A** — Live-set filtering in `mergeSkippedPanes` (`internal/state/capture.go`). Approximately 15 lines (session/window/pane filtering).
> - **Fix Component B** — New stale-marker cleanup bootstrap step. Approximately 50 lines including adapter wiring, plus orchestrator sequence and test updates.

**Proposed Addition**:
[Pending — clarify the line counts are estimates, not a scope budget; or remove]

**Resolution**: Pending
**Notes**:

---
