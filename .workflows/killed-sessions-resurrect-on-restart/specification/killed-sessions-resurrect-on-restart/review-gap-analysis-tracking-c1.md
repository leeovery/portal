---
status: complete
created: 2026-05-10
cycle: 1
phase: Gap Analysis
topic: killed-sessions-resurrect-on-restart
---

# Review Tracking: killed-sessions-resurrect-on-restart - Gap Analysis

## Findings

### 1. New step's seam interface shape is under-specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Adapter Wiring

**Resolution**: Approved
**Notes**: Added `EagerHydrateSignaler` interface shape with method signatures, named the production adapter source, and stated `stateDir` ownership in the orchestrator.

---

### 2. "Reasonable time post-bootstrap" is unmeasurable in AC1 and integration test

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: AC1, Test Plan integration entry

**Resolution**: Approved
**Notes**: Bound set to 2 seconds in AC1 and the multi-session integration test. Justification: helper writes scrollback + unsets marker after 100 ms settle, so 2s gives ~10× slack.

---

### 3. Step name vs. method name not stated for the orchestrator

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Bootstrap Step Numbering Update

**Resolution**: Approved
**Notes**: `EagerSignalHydrate` is canonical across orchestrator method, log label, seam suffix, and test assertions.

---

### 4. Where stateDir comes from for the eager-signal step is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Pane Enumeration and FIFO Resolution

**Resolution**: Approved
**Notes**: stateDir plumbed through orchestrator construction; same source as Restore and EnsureSaver; resolved once via `state.Paths().StateDir`.

---

### 5. `writeFIFOSignal` reuse mechanics — copy, export, or refactor — not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Write Primitive

**Resolution**: Approved
**Notes**: Pick option (b) — move `writeFIFOSignal` and `signalHydrateRetryDelays` to `internal/state`; both `cmd/state_signal_hydrate.go` and the new `cmd/bootstrap` step call into the shared package.

---

### 6. Write-failure logging context — paneKey, sessionName, or both — undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Failure Posture

**Resolution**: Approved
**Notes**: Log shape pinned: `WARN | hydrate | eager-signal: write fifo <fifoPath>: <error>` — paneKey derivable from FIFO basename.

---

### 7. AC6 "drops to zero" lacks a measurement window or method

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: AC6

**Resolution**: Approved
**Notes**: AC6 explicitly delegates verification to Manual Verification Protocol step 2 — observational, not a gated automated test.

---

### 8. Argument quoting responsibility for bare-form hydrate command not pinned down

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 3 → Argument Quoting

**Resolution**: Approved
**Notes**: `buildHydrateCommand` returns single shell-safe string (no argv split). `RespawnPane` interface unchanged. Helper invocation is PATH-resolved `portal`; value-args quoted via existing internal/tmux helper.

---

### 9. "Fire hooks if registered" path on timeout — `--hook-key` plumbing not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 2 → Specific Changes (item 2)

**Resolution**: Approved
**Notes**: `runHydrate` already holds the hook key in scope as `cfg.HookKey`; both timeout and file-missing recovery paths call `execShellOrHookAndExit(cfg.HookKey)` symmetrically. No new plumbing.

---

### 10. Eager-step interaction with helpers that have already self-signaled is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Behaviour

**Resolution**: Approved
**Notes**: New "Race-Free Ordering vs. Client-Attached" subsection added — bootstrap completes via PersistentPreRunE before any tmux attach; eager step always runs before client-attached event for bootstrap-time skeleton.

---

### 11. `CLAUDE.md` update is mentioned but not scoped as a deliverable

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Bootstrap Step Numbering Update

**Resolution**: Approved
**Notes**: Stated as part of same PR; only renumbers and inserts EagerSignalHydrate description; existing "Return is post-step boundary" framing preserved.

---

### 12. Snapshot/equality test for `buildHydrateCommand` — concrete expected output missing

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Plan → Unit (wrapper drop)

**Resolution**: Approved
**Notes**: Argument quoting subsection now states the bare form template; unit test snapshot asserts the exact resulting string format produced by the helper on representative inputs.

---

### 13. AC2 "for every restored pane that has a hook registered" — hook registration prerequisite untested

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: AC2

**Resolution**: Approved
**Notes**: AC2 now explicitly states attached-session case is covered by existing happy-path tests preserved under Regression Coverage; the new test specifically covers the previously-broken non-attached case.

---

### 14. AC-to-fix traceability matrix absent

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria

**Resolution**: Approved
**Notes**: Added AC ↔ Fix Traceability table mapping each AC to the fix(es) that satisfy it.

---

### 15. "Empirical Reconfirmation Before Implementation Starts" — owner and gating semantics unclear

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Risks & Rollout → Empirical Reconfirmation

**Resolution**: Approved
**Notes**: Owner: planning agent. Result recorded in plan pre-flight notes / PR description. Branch behaviour stated: if Symptom A still reproduces, plan adds explicit regression test; otherwise AC3 stays a regression guard. Either way Fix 1/2/3 ship.

---

### 16. `state.UnsetSkeletonMarkerForFIFO` vs `unsetSkeletonMarkerOrLog` — which is the canonical primitive

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 2 → Specific Changes (item 1)

**Resolution**: Approved
**Notes**: Canonical: handler calls `unsetSkeletonMarkerOrLog` (cmd-layer wrapper that internally invokes `state.UnsetSkeletonMarkerForFIFO` and logs warnings). Tests override the underlying state-package seam.

---

### 17. "Done" definition for the work unit not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria

**Resolution**: Approved
**Notes**: Added "Definition of Done" subsection: tests pass, regression preserved, manual verification protocol executed once, CLAUDE.md updated, PR merged.
