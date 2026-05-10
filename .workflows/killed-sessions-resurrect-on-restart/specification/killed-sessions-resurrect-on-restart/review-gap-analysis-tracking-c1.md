---
status: in-progress
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
**Affects**: Fix 1 → Adapter Wiring; Fix 1 → Pane Enumeration and FIFO Resolution; Test Plan → Unit (`cmd/bootstrap` new step)

**Details**:
The spec says the new step's seam interface is "wired through `internal/bootstrapadapter` in the same shape as the existing post-restore steps (concrete `*tmux.Client`, `state` package functions)" and the unit test mocks "the FIFO writer" — but never names the seam interface, its method signatures, or which existing seam (CleanStaleMarkers, SweepOrphanFIFOs) it should mirror most closely. A planner cannot derive: (a) the interface name and methods, (b) whether it consumes `state.ListSkeletonMarkers` directly or via a seam, (c) whether `writeFIFOSignal` is exposed via a method or imported directly, (d) what stateDir source the step receives. The unit-test bullet "mock the FIFO writer" implies a seam exists for the writer specifically, but the spec does not define one.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 2. "Reasonable time post-bootstrap" is unmeasurable in AC1 and integration test

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria → AC1; Test Plan → Integration → Multi-session cold-start

**Details**:
AC1 reads "all `@portal-skeleton-<paneKey>` markers are unset within reasonable time post-bootstrap". The integration test bullet repeats the phrase verbatim. "Reasonable time" is not bounded. The helper has a 100 ms settle sleep and the spec mentions "usually within milliseconds" — but the test needs a deterministic upper bound (e.g. poll-with-timeout of N seconds) and the AC needs a concrete pass condition. Without a number a planner has to invent one, and a flake-prone integration test is the likely outcome.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 3. Step name vs. method name not stated for the orchestrator

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Bootstrap Step Numbering Update; Test Plan → Integration → Bootstrap orchestrator ordering

**Details**:
The spec calls the new step `EagerSignalHydrate` in the numbered list, but does not state whether this is the orchestrator method name, the step's logged label, the seam method name, or all three. The "Sequence test asserts ordering by injecting a recording orchestrator deps fake" bullet relies on a specific identifier being recorded. A planner needs to know the canonical name for cross-file consistency (orchestrator step list, log line, test assertions, `CLAUDE.md` update).

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 4. Where stateDir comes from for the eager-signal step is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Pane Enumeration and FIFO Resolution; Fix 1 → Adapter Wiring

**Details**:
`state.FIFOPath(stateDir, paneKey)` requires stateDir, but the spec does not say where the new step receives it from — passed through orchestrator construction, looked up via `state.Paths()` at call time, or shared with EnsureSaver/Restore via the orchestrator struct. The other restore-window steps (Restore, EnsureSaver) presumably already receive it; the spec should either say "same source as Restore" or specify the wiring explicitly.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 5. `writeFIFOSignal` reuse mechanics — copy, export, or refactor — not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Write Primitive

**Details**:
"The step uses the existing `writeFIFOSignal` helper and `signalHydrateRetryDelays` retry schedule from `cmd/state_signal_hydrate.go`". Both identifiers are package-private (lowercase) inside `cmd`. The new step lives in `cmd/bootstrap`, a different package. A planner has three implementation choices: (a) export both symbols, (b) move them to a shared package (`internal/state`?), (c) duplicate them in `cmd/bootstrap`. Each has different test-surface implications. The spec should pick one or call this out as a deferred implementation decision.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 6. Write-failure logging context — paneKey, sessionName, or both — undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Failure Posture; Fix 1 → Logging (implicit); AC6

**Details**:
"Per-FIFO write failures are soft warnings. The step logs a `WARN | hydrate | …` entry mirroring `runSignalHydrate`'s posture". `runSignalHydrate`'s log shape is referenced but not reproduced. A planner needs to know the exact log fields (paneKey, FIFO path, error string, session context) to write the assertion in unit tests and for AC6's "log volume drops to zero in steady state" check. Mirroring `runSignalHydrate` is a directional cue, not a contract.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 7. AC6 "drops to zero" lacks a measurement window or method

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria → AC6

**Details**:
AC6 says the two WARN log lines drop to zero "in the steady state" on cold-start. There is no test in the Test Plan that asserts this — the integration tests assert markers unset and hooks fire, but not log absence. A planner needs to know whether log-absence is part of the verification harness or an observational AC checked manually via the Manual Verification Protocol step 2. If the latter, AC6 should explicitly delegate to the manual protocol; if the former, a test bullet is missing.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 8. Argument quoting responsibility for bare-form hydrate command not pinned down

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 3 → Wrapper Drop in `buildHydrateCommand`; Fix 3 → Argument Quoting

**Details**:
"Argument quoting/escaping responsibilities shift from the wrapper-shell to the call-site formatter — this is the same shape the existing tmux command-construction helpers in `internal/tmux` already produce for non-shell pane commands." This describes the destination but not the change. A planner needs to know: (a) does `buildHydrateCommand` now return a single shell-quoted string, or a `[]string` argv that `RespawnPane` joins, (b) does `RespawnPane`'s signature change, (c) which existing `internal/tmux` helper is the reference. The spec then says "the call-site already constructs a properly-quoted string" and "the change is in `buildHydrateCommand`'s output, not in the `RespawnPane` interface" — apparently contradicting the earlier "shifts to the call-site formatter" sentence. Whether quoting moves or stays needs a single answer.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 9. "Fire hooks if registered" path on timeout — `--hook-key` plumbing not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 2 → Specific Changes (item 2); Test Plan → Unit → Hook-firing on timeout end-to-end

**Details**:
Routing the timeout path to `execShellOrHookAndExit` requires the hook key to be in scope at the timeout site. The unit test bullet "registered on-resume hook, force `OpenFIFO` to return `ErrHydrateTimeout`, assert exec target is `sh -c '<HOOK>; exec $SHELL'`" assumes the hook lookup happens inside `execShellOrHookAndExit` — but the spec never confirms whether `handleHydrateTimeout` calls into the same lookup the file-missing path uses, or whether the hook key is already a parameter the helper carries. A planner needs explicit confirmation that no new `--hook-key` plumbing is required (e.g. "`runHydrate` already holds the hook key in scope; both recovery handlers can call `execShellOrHookAndExit(hookKey)` symmetrically").

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 10. Eager-step interaction with helpers that have already self-signaled is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Behaviour; Fix 1 → Failure Posture

**Details**:
Between Restore (step 5) populating markers and EagerSignalHydrate iterating them, the user-attached session's `client-attached` hook may have already fired (the bootstrap is typically driven by `portal open <session>`, which attaches before/during/after bootstrap depending on path). The spec asserts "second-fire on already-hydrated panes is a no-op (marker already unset, `signal-hydrate` skips)" but the inverse — eager step writes to a FIFO whose helper already exec'd — is not addressed. Outcomes a planner needs to know: (a) is writing to an unlinked FIFO an expected ENOENT (logged as warning under current shape), (b) does the eager step pre-check the marker is still set before writing, (c) is the race actually impossible because bare-CLI attach happens via `syscall.Exec` only after bootstrap returns. If (c), state it; otherwise the failure-posture section under-counts an expected source of warnings.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 11. `CLAUDE.md` update is mentioned but not scoped as a deliverable

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 → Bootstrap Step Numbering Update; Risks & Rollout → Non-User-Visible Changes

**Details**:
"The `CLAUDE.md` 'Server bootstrap' section will be updated to reflect the new ordering as part of the fix." Two points need pinning: (a) is this part of the same PR/work unit or a follow-up, (b) does the update only renumber, or also re-describe the post-restore boundary (the existing CLAUDE.md says 'Return is the post-step boundary, not a numbered step' — the new step sits inside the restore window so this framing still holds, but a planner should be told the existing wording is preserved). Without explicit scoping, the doc update can fall through the cracks.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 12. Snapshot/equality test for `buildHydrateCommand` — concrete expected output missing

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Plan → Unit → `internal/restore/session.go` (wrapper drop)

**Details**:
"Update the existing snapshot/equality test in `session_test.go` to the new shape." The spec shows the conceptual before/after (`sh -c '...'` → bare form) but does not pin the exact resulting string format — particularly how `--fifo`, `--file`, `--hook-key` are quoted, whether single or double quotes, whether the helper path is absolute or `portal` (PATH-resolved). The existing test must be updated to a specific expected string; a planner needs that string or an explicit "match the format already produced for non-shell pane commands in <file:line>".

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 13. AC2 "for every restored pane that has a hook registered" — hook registration prerequisite untested

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria → AC2; Test Plan → Integration → End-to-end hook firing on cold-start

**Details**:
AC2 requires the hook to fire "regardless of which session the user attached to". The integration test registers a hook for a non-attached saved session and asserts it fired. But the AC also implicitly requires the hook fires on the **attached** session under the new flow (the eager step now signals it before the per-session hook does). A planner should know whether the test must cover both or whether the existing happy-path coverage subsumes the attached-session case. The "Regression Coverage to Preserve" section asserts existing happy-path tests stay green, which arguably covers this — making it implicit, not explicit.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 14. AC-to-fix traceability matrix absent

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria; Fix 1 / Fix 2 / Fix 3

**Details**:
The eight ACs and three fixes are listed in separate sections with no explicit mapping. A planner breaking this into tasks must reverse-engineer which AC each fix's tasks satisfy (e.g., AC1 ← Fix 1; AC2 ← Fix 1 + Fix 2; AC3 ← regression guard against companion fix; AC4 ← Fix 1; AC5 ← Fix 3; AC6 ← Fix 1 + Fix 2; AC7/AC8 ← invariants). A small mapping table would make task-decomposition mechanical and prevent an AC from being orphaned.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 15. "Empirical Reconfirmation Before Implementation Starts" — owner and gating semantics unclear

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Risks & Rollout → Empirical Reconfirmation Before Implementation Starts

**Details**:
"The investigation flagged that Symptom A's user-visible behaviour … should be empirically re-checked against current `main` before implementation begins." This is described as a "one-time check, not an ongoing acceptance criterion" but does not say (a) who runs it (planning agent, implementation agent, user), (b) what happens if reconfirmation fails (does the spec scope change? does Fix 1 still ship?), (c) where the result is recorded. As written, a planning agent might skip it or treat it as an implementation prerequisite without any actionable wiring.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 16. `state.UnsetSkeletonMarkerForFIFO` vs `unsetSkeletonMarkerOrLog` — which is the canonical primitive

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 2 → Specific Changes (item 1); Test Plan → Unit → `cmd/state_hydrate.go`

**Details**:
The Specific Changes line names two symbols separated by a slash: `state.UnsetSkeletonMarkerForFIFO` / `unsetSkeletonMarkerOrLog`. They are at different layers (state package primitive vs cmd-layer wrapper that adds logging). The unit test bullet says "Use the existing `unsetSkeletonMarkerOrLog` mock pattern". A planner should know: does the timeout handler call the primitive directly, or the cmd-layer wrapper, and is the slash an "or" (either-is-fine) or an "and" (calls one which calls the other)? This affects the mock surface in tests.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---

### 17. "Done" definition for the work unit not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria; Test Plan; Risks & Rollout

**Details**:
The spec lists ACs (behavioural, logging, spec conformance), a test plan, and a manual verification protocol. It does not state which of these constitute "done" gates vs. "post-merge observation". For example, AC6 (log volume to zero) is observable but no automated test enforces it; the Manual Verification Protocol is described as confirming pre/post-fix behaviour but not gated. A planner needs an explicit statement: "the work unit is complete when all unit + integration tests in the Test Plan pass AND the manual verification protocol has been executed once on a real machine" — or similar.

**Proposed Addition**:
*(blank — to be discussed)*

**Resolution**: Pending
**Notes**:

---
