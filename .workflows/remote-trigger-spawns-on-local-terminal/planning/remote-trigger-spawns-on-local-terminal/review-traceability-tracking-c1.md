---
status: complete
created: 2026-07-23
cycle: 1
phase: Traceability Review
topic: remote-trigger-spawns-on-local-terminal
---

# Review Tracking: remote-trigger-spawns-on-local-terminal - Traceability

## Result: CLEAN — no findings

The plan is a faithful, complete, bidirectional translation of the specification. Every spec
element is covered by a task with sufficient depth, and every task element traces back to the
specification with no invented scope, hallucinated behaviour, or over-reach. The codebase
references embedded in the tasks (test fixtures, PIDs, line numbers, function names) were spot-verified
against the actual source and are accurate.

## Findings

None.

## Analysis Record

### Direction 1: Specification → Plan (completeness)

Every specification element has plan coverage:

- **The Fix — select-winner-then-locality-check (4 steps)** → Task 1-1 (`remote-trigger-spawns-on-local-terminal-1-1`).
  Enumerate via `ListClients(session)` (Do step 1), select max-`client_activity` winner across all clients
  with first-listed tie-break (Do step 3, AC), walk only winner with three branches — local `.app` drive /
  clean NULL no-op / transient → NULL+`ErrDetectTransient` (Do step 4, AC), empty client list → clean NULL
  (Do step 2, AC, Edge Cases).
- **Behavioural outcomes by scenario (8-row table)** → Task 1-1 Context (full table reproduced) plus Task 1-2
  for the "Mixed: local most-active, remote idle → drive" row.
- **Implementation approach** (reuse existing enumeration, no extra tmux round-trip, keep `clientLister` seam
  and `detectInsideTmux(session, lister, walker, reader)` signature, rejected `display-message` alternative)
  → Task 1-1 Do + Context.
- **Owned Behaviour Change: Dropped Walk-Resilience Property** → Task 1-1 Do: full docstring rewrite
  (algorithm + Outcomes list, ~lines 49–72), removal of the two directly-inverted sentences
  ("NULL-filtering is the primary signal" ~:55; "client_activity is used ONLY to disambiguate among
  host-local clients — never as a cross-client primary signal" ~:70–72), explicit ownership of the dropped
  "one flaky ps cannot mask a resolvable local" guarantee, and removal of stale scan-all/`firstWalkErr`
  text. The lost resilience is locked in by the reframed `:196` regression test.
- **Edge Contracts to Pin** (empty client list → clean NULL nil-error; first-listed deterministic tie-break;
  `client_activity` epoch-seconds granularity as an acknowledged non-defect) → Task 1-1 Edge Cases.
- **Scope: Affected Surfaces** (CLI burst `cmd/open_burst_run.go`, TUI picker `internal/tui/spawn_detect.go`,
  `portal doctor` `checkHostTerminal` — all unchanged, inherit the corrected resolution) → Task 1-1 AC
  (consumers untouched) + Task 1-3 (manual per-surface verification, incl. doctor "unsupported (remote session)").
- **Automatically re-armed `m`-entry safeguard** (`DetectUnsupported()` / `ResolutionUnsupported`, no extra code)
  → Task 1-3 (confirms the proactive block now fires) + phase "Why this order".
- **Coherence with persistent-no-host-terminal-banner** (mixed → NULL → NULL/remote branch → no persistent
  banner, standard header, `spawn.UnsupportedNoopMessage` copy) → Task 1-3 Context.
- **Unaffected Paths** (outside-tmux `detect_outside.go` untouched; pure-remote unchanged; single-local
  unchanged) → Task 1-1 AC + retained-green invariants.
- **Out of Scope** (Blink spawn adapter; same-or-later-second residual edge) → correctly not built; the
  residual edge is explicitly noted as out-of-scope in Task 1-1 Edge Cases.
- **Testing Requirements** — invert `:133` and reframe `:196` (Task 1-1), net-new local-most-active +
  remote-idle-bystander drive (Task 1-2), all 8 pinned green invariants enumerated (Task 1-1 Tests, incl.
  the explicit "do not delete `:171` as a duplicate of `:196`" guard), and the manual real-multi-client
  end-to-end verification (Task 1-3).
- **Release approach** (regular release, no feature flag, no hotfix urgency) → Task 1-3 Context.

### Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every task element traces to the specification:

- **Task 1-1** — Problem/Solution/Do/AC/Tests/Edge/Context all derive from Root Cause, The Fix, Owned
  Behaviour Change, Edge Contracts to Pin, and Testing Requirements. The concrete test fixtures (PIDs 501/502/601/602,
  activity values, `localWalkSeams()`, `walkToBundle`, `transient`, `ErrDetectTransient`, docstring line
  ranges) are accurate codebase substrate for a bugfix, not invented requirements — verified against
  `internal/spawn/detect_inside.go` and `internal/spawn/detect_inside_test.go`.
- **Task 1-2** — the local-most-active + idle-remote-bystander drive test traces directly to Testing
  Requirements "New coverage to add" and the corresponding behavioural-outcomes row. No over-reach.
- **Task 1-3** — the manual E2E (both surfaces + doctor + control check) traces to Verification scope,
  The Bug (Precondition), Affected Surfaces, the re-armed safeguard, and Coherence. The control check
  ("legitimate local trigger still drives") traces to Unaffected Paths (single-local drives).

No plan content lacks a specification anchor. No technical approach, requirement, edge case, or acceptance
criterion was found that the specification does not justify.
