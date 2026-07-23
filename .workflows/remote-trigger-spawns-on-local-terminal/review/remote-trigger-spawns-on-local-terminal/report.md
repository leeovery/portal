# Implementation Review: remote-trigger-spawns-on-local-terminal

**Plan**: remote-trigger-spawns-on-local-terminal
**QA Verdict**: Approve

## Summary

The bugfix is implemented correctly, minimally, and exactly as specified. The single-root-cause fix inverts `detectInsideTmux` from filter-then-tiebreak to **select-winner-then-locality-check** — it now picks the most-active client across *all* enumerated clients (first-listed on an exact tie) and walks only that winner, so a remote trigger with a host-local bystander on the same session correctly resolves NULL (honest no-op) instead of driving windows onto the wrong machine. The deliberately-dropped walk-resilience property is *owned*: the docstring was fully rewritten (both inverted sentences removed) and the fail-safe-to-NULL trade is locked in by a reframed regression test. Analysis-cycle follow-ups landed cleanly — T2-1 collapsed the hand-rolled three-branch propagation to a direct `walkToBundle` passthrough (matching the sibling `detectOutsideTmux`) and refreshed the `ClientActivity` type doc; T3-1 collapsed the seven happy-path subtests into an idiomatic table-driven test with no coverage loss. All five task verifiers returned **zero blocking issues**, production change is confined to `internal/spawn/detect_inside.go`, and `go build` + `go test ./internal/spawn/...` are green. The only substantive carry-forward is that the **manual multi-client end-to-end verification (T1-3) has not yet been performed** — it was deliberately deferred to release testing to avoid disturbing the developer's live daemon, and the automated decision surface is fully pinned by T1-1/T1-2. That is a release-testing action item, not a code defect.

## QA Verification

### Specification Compliance

Implementation aligns with the specification in full:
- **Root cause & fix** (spec "The Fix: Gate Locality on the Triggering (Most-Active) Client"): `detectInsideTmux` selects `max(client_activity)` across all clients via the pure `selectTriggeringClient` helper (strictly-greater replacement → first-listed wins a tie), walks only the winner, and branches on its locality — drive / clean-NULL no-op / transient→NULL+`ErrDetectTransient`. Empty client list → clean NULL, nil error.
- **Owned behaviour change** (spec "Owned Behaviour Change: Dropped Walk-Resilience Property"): the entire docstring contract was rewritten. Both directly-inverted sentences ("NULL-filtering is the primary signal"; "client_activity is used ONLY to disambiguate among host-local clients — never as a cross-client primary signal") are gone; no stale scan-all / `firstWalkErr` text remains; the dropped-resilience fail-safe is explicitly stated.
- **Affected surfaces** (spec "Scope"): `detect.go` and all three `Detect()` consumers (`cmd/open_burst_run.go`, `internal/tui/spawn_detect.go`, `cmd/doctor.go`) are untouched and inherit the correction; the TUI proactive `m`-entry safeguard re-arms with no separate change. Coherence with `persistent-no-host-terminal-banner` holds (mixed case now flows into the NULL/remote branch).

### Plan Completion
- [x] Phase 1 (Gate Locality) — tasks 1-1, 1-2 completed; 1-3 (manual e2e) **deliberately deferred** to release testing (commit 62898477), coherent with the project's live-daemon isolation invariant.
- [x] Phase 2 (Analysis cycle 1) — task 2-1 completed (residue cleanup).
- [x] Phase 3 (Analysis cycle 2) — task 3-1 completed (table-driven test collapse).
- [x] All automatable acceptance criteria met; every spec-pinned edge contract has a pinning subtest.
- [x] No scope creep — production change confined to `internal/spawn/detect_inside.go`; test changes confined to `internal/spawn/detect_inside_test.go`. `internal/tmux/clients.go` correctly left untouched (out of scope). No unplanned files or features.

### Code Quality

No issues found. The fix is low-complexity (linear winner scan, single winner walk, three return shapes delegated to `walkToBundle`). `selectTriggeringClient` is a pure single-responsibility helper; the 1-method DI seams (`clientLister`, `ProcessWalker`, `BundleReader`) are preserved; error wrapping via `transient(...)` keeps the underlying cause reachable through `errors.Is`. The T2-1 collapse to a direct `return walkToBundle(...)` removes a divergence risk against both the callee's contract and the sibling `detectOutsideTmux`. Conventions (golang-testing, golang-error-handling, golang-design-patterns) are followed; no `t.Parallel()` (correct for this project).

### Test Quality

Tests adequately verify requirements, neither over- nor under-tested:
- The two codified-bug subtests were transformed in place (inverted `:133` → NULL for remote-most-active + local bystander; reframed `:196` → NULL + `ErrDetectTransient` on a flaky winner walk), landing in the same commit as the code so red-before-green holds for the combined change.
- T1-2 adds the one genuinely net-new scenario (local-most-active + remote-idle bystander → local drives), with the local listed *second* so a pass proves max-by-activity rather than first-listed luck.
- All retained invariants are present (pure-remote NULL, single-local drive, 2+ locals highest-activity in both orderings, exact-tie first-listed, list-clients failure transient, single-client walk-failure transient — retained not deleted, empty→clean NULL).
- T3-1's table collapse preserves every scenario 1:1 and actually *strengthens* the session-passthrough assertion (now run per-row instead of only in the all-remote case); the three error paths stay separate and byte-identical.

### Required Changes (if any)

None. Verdict is **Approve**.

## Recommendations

### Do now
1. `internal/spawn/detect_inside_test.go:22` — stale doc comment `// ghosttyProc/terminalProc are single-hop ancestries…` references identifiers that don't exist (the vars are `ghosttyCommand`/`ghosttyAppPath`/`terminalCommand`/`terminalAppPath`). Pre-existing, outside the T3-1 diff; trivial wording fix. (Report 3-1)
2. `internal/tmux/clients.go:11-12` — the mirror `tmux.ClientInfo` type doc still carries the now-falsified "Activity is the local-only tiebreak used to choose among 2+ host-local clients" phrase. Deliberately excluded from T2-1 (out of scope) and correctly untouched; apply the same cross-client-winner-selection rewrite here as a follow-up so the two mirror docs stay consistent. (Report 2-1)
3. Tracking records — the task is `cancelled` in tick (`tick-d11c21`) but listed as completed/deferred in the manifest; same "deferred" fact recorded two ways. A one-line "deferred to release testing" note on the source-of-truth record prevents a future reader misreading it as an inconsistency. (Report 1-3)

### Ideas
4. **Release-testing action item — manual remote+local e2e verification (T1-3), still outstanding.** The live multi-client confirmation (real remote SSH/mosh trigger + host-local client on the same session → **zero** host windows across the TUI burst, CLI burst, and `portal doctor` line; local-only control still drives; outcome recorded pass/fail) has NOT been performed — it was explicitly deferred to release testing (commit 62898477). The code behaviour is unit-proven, but no human has yet observed the real "N−1 windows do not open on the host" outcome on real hardware. Add this to the release-testing checklist for this work unit so the confirmation is not lost between merge and release. (Report 1-3)
