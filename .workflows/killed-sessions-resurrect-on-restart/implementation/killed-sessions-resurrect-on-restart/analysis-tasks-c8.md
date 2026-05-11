# Analysis Tasks: killed-sessions-resurrect-on-restart (Cycle 8)

- topic: killed-sessions-resurrect-on-restart
- cycle: 8
- total_proposed: 0

## Status

STATUS: clean — no actionable tasks proposed. All three analysis agents converged; the single low-severity duplication finding is discarded as out-of-scope premature abstraction.

## Discarded Findings

- **Extract `handleFileMissingAndExec` helper from `runHydrate`** — duplication, low severity, `/Users/leeovery/Code/portal/cmd/state_hydrate.go:103-117, 140-149, 159-167`.
  - The two byte-identical file-missing sites (lines 140-149 and 159-167) pre-date this work unit; introduced by built-in-session-resurrection T4-2, not touched by killed-sessions-resurrect-on-restart.
  - T2-3 added a third near-identical block (timeout branch, lines 103-117), but the duplication agent's own recommendation explicitly leaves the timeout site open-coded — its inserted `time.Sleep(hydrateSettleSleep)` would force a boolean settle parameter on any shared helper (anti-pattern per `code-quality.md` "Boolean parameters").
  - The proposed extraction therefore touches only the two pre-existing sites and does not touch T2-3's site.
  - Work unit's hard rule on duplication scope: pre-existing duplications outside the work unit's edited surface are out of scope. Cycle 7 already discarded a similar "extract more" candidate (`bootstrapEagerHydrateScenario`) for the same reason — premature abstraction beyond what the task requires.
  - Severity is low; the finding itself records a counter-argument that inline recovery branches keep `runHydrate`'s linear happy-path read crisp.
