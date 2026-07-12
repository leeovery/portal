---
status: complete
created: 2026-07-12
cycle: 6
phase: Plan Integrity Review
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Integrity

## Result: CLEAN

Cycle 6 is a fresh, full pass over the whole plan (planning.md + all six
phase task files, 45 tasks) against every review-integrity criterion. No
genuine, material issue was found. The plan meets structural quality and
implementation-readiness standards.

## Findings

None.

## What was verified this cycle

**Cycle-5 export fix (QuoteJoin / GoneVerb) — HOLDS.**
- Single exported declaration in `internal/spawn/message.go`, authored once in
  Task 3-4, with explicit "do not re-declare" notes on 3-6/6-6/6-7.
- All four consumer call sites are cross-package and qualified `spawn.`:
  - 3-4 (`cmd/spawn.go`, pkg `cmd`): `spawn.QuoteJoin(gone)`, `spawn.GoneVerb(len(gone))`.
  - 3-6 (`cmd/spawn.go`, pkg `cmd`): `spawn.QuoteJoin(failed)`.
  - 6-6 (`internal/tui/model.go`, pkg `tui`): `spawn.QuoteJoin(failedNames)`.
  - 6-7 (`internal/tui/section_header.go`, pkg `tui`): `spawn.QuoteJoin(msg.Gone)`,
    `spawn.GoneVerb(len(msg.Gone))`.
- No duplicate declaration. Singular/plural copy is consistent: `GoneVerb(1)="is"`
  renders the delivered design copy `⚠ '<session>' is gone — nothing opened`;
  `GoneVerb(n>1)="are"` renders the grammatical plural.

**Cross-phase seams (all consistent):**
- `spawn.Burster.Run` signature evolution: 3-5 `(external)` → 6-3 adds
  `(ctx, external, progress)` as an additive seam, with 6-3 explicitly updating
  the Phase-2/3 CLI call site (`cmd/spawn.go` → `Run(context.Background(), external, nil)`)
  and the `internal/spawn/burst_test.go` call sites in the same change.
- `AckChannelFull` (Collect + Clean) referenced identically by `SpawnDeps.Ack`
  (3-5) and the picker `AckChannel`/`cleanBatch` (6-3); `Burster.Ack` holds the
  narrower `AckCollector`; `AckChannelFull` satisfies it.
- Config-resolver parity: 4-6 promotes `ResolveAdapter` → `Resolver.Resolve`
  (keeping the zero-config wrapper); both the CLI (4-6) and the picker (6-1 single
  injection site, reused by 6-3) default to the SAME config-aware
  `spawn.NewResolver(terminals.json).Resolve`, so `terminals.json` recipes resolve
  identically in CLI and picker.
- `DetectUnsupported()` (resolution-based, true for NULL remote/mosh AND non-NULL
  recognised-but-undriven identities) defined once in 6-1 and consumed by 6-2/6-3/6-9;
  `IsNull()` is correctly NOT used as the unsupported gate (avoids a nil-adapter
  dispatch on a non-NULL undriven identity).
- `Opening n/N…` counter semantics: `burstTotal` fixed at N (6-3/6-5), band advances
  0…N−1 (never N/N), while the log summary `opened N/N` counts the trigger self-attach
  (6-10); the two counters are intentionally distinct and mutually consistent, and
  `msg.Total` (=N−1) from `Burster.Run`'s progress callback is explicitly ignored for
  the denominator.
- Shared types (`Identity`, `Result`/`Outcome`/`Detail`/`Guidance`, `Resolution`,
  `WindowResult`/`AckOutcome`, `PreflightMissing`, `AttachCommand`) are produced and
  consumed with matching fields/signatures across phases.
- Completion-handler split is coherent: 6-4 (full success), 6-6 (partial/permission +
  `Burster.Run` pre-spawn error), 6-7 (pre-flight abort), 6-8 (cancellation converging
  on the 6-6 mutation) — the `len(msg.Results) == len(m.burstExternal)` guard correctly
  routes the permission-early-stop (shorter results) to the partial path.
- Esc precedence across states (burst-pending cancel → abort-banner dismiss →
  mode-exit) is coherent across 5-1/6-5/6-7/6-8.
- No import cycle introduced: `spawn` imports tmux/session/resolver/log; `tui` imports
  `spawn`; none of tmux/session/resolver/`spawn` import `tui`.

**Template compliance / self-containment:** every task carries Problem, Solution,
Outcome, Do, Acceptance Criteria, Tests, Edge Cases, Context, and Spec Reference;
acceptance criteria are concrete and pass/fail; tests include edge cases; scope
boundaries and Phase-to-Phase reconciliation notes make each task independently
executable.

## Notes

Plan is at its convergence tail; cycles 1–5 resolved all substantive cross-phase
defects (config-resolver parity, Opening counter, Burster.Run signature + call sites,
AckChannelFull, resolution-based DetectUnsupported, and the QuoteJoin/GoneVerb helper
copy/ownership/export). Cycle 6 confirms those fixes hold and surfaces no new material
gap. Recommend concluding the integrity review.
