---
topic: restore-host-terminal-windows
cycle: 1
total_findings: 7
deduplicated_findings: 6
proposed_tasks: 6
---
# Analysis Report: restore-host-terminal-windows (Cycle 1)

## Summary
The three analysis agents (duplication, standards, architecture) reviewed a complete, all-tests-green feature and returned 7 findings — no high-severity items, three medium, four low. The dominant theme is that `internal/spawn` under-extracts cross-caller (CLI `cmd/spawn.go` vs picker `internal/tui`) logic the spec designates as parity-critical: the result-classification "count-semantics chokepoint" is re-authored per caller, the permission-required outcome diverges (a spec-catalogued log event is unreachable on the picker path), and gone/unsupported message TEXT is hand-copied across 5+2 sites despite the sub-primitives already being shared. The remainder are localized cleanup/correctness items: byte-identical adapter exec plumbing, a dead exported `spawn.AttachCommand`, a timing-dependent stale-selection snapshot, and a notice-band precedence divergence that needs a decision. Sanctioned parallels (the `logSpawnSummary`/`emitBurstSummary` emission pair, `QuoteJoin`/`GoneVerb`, the deliberately-separate `recipeRunner`/`osascriptRunner` seams) were reviewed and correctly left untouched.

## Deduplication / Grouping Notes
- Architecture finding 1 (count-semantics chokepoint re-authored per caller) and architecture finding 2 (CLI/picker permission-event divergence) were **merged into one task**: the agent's own recommendation folds finding 2's permission detection into finding 1's shared `FirstPermission` helper, so the classification hoist and the picker `emitPermission` mirror are one coherent, self-contained change. → Task 1.
- The low-severity items were retained rather than discarded: Task 3 (adapter exec dup) belongs to the same "spawn under-extracts shared cross-caller logic" cluster as Tasks 1–2; Task 4 (dead public API) and Task 5 (stale snapshot) are concrete, actionable single-file items (Task 5 is a latent correctness defect, not mere style); Task 6 (notice-band precedence) is a literal divergence from an explicitly-decided spec precedence and clusters with the permission-outcome theme.
- Tasks 1 and 2 both touch `cmd/spawn.go` and `internal/tui` but at different functions and neither consumes the other's output — each is independently executable; if both are approved they should be sequenced to reduce edit overlap.

## Discarded Findings
- None. All 7 findings trace to genuine, avoidable duplication or divergence (not the feature's sanctioned parallel emission pairs or deliberately-separate seams). Low-severity items were kept because they either cluster around the CLI/picker parity theme the spec makes load-bearing or are concrete single-site cleanup/correctness fixes rather than speculative noise.
