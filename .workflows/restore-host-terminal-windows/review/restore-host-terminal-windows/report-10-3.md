TASK: restore-host-terminal-windows-10-3 — Centralize the net-N split behind a shared spawn.SplitNetN helper (chore/refactor, Phase 10 analysis cycle)

ACCEPTANCE CRITERIA:
1. spawn.SplitNetN is the single computation of the net-N (external/trigger) split; neither runSpawn nor dispatchBurst hand-rolls the slice expressions.
2. For any ordered with len >= 1, external is the leading N-1 and trigger is the last element — byte-identical to the two prior inline computations.
3. The empty-set / N=1 guards on both callers are preserved; no caller passes a zero-length slice into SplitNetN.
4. CLI and picker net-N behaviour (never N+1) is unchanged; the sync/async control-flow ordering is untouched.

STATUS: Complete

SPEC CONTEXT: internal/spawn deliberately centralizes every leaf decision the two spawn callers (CLI runSpawn, picker dispatchBurst) share so "the two paths cannot drift" (PreflightMissing, PartitionResults, FirstPermission, the Log*/message renderers). The spec elevates "net N windows, never N+1" to a hard anti-requirement; the net-N split was the sole shared invariant still duplicated inline across the two callers. This chore unifies that one seam behind a pure helper, leaving the genuine sync-vs-async control-flow divergence as-is.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/split.go:15-17 — new pure SplitNetN(ordered) returning ordered[:len-1], ordered[len-1], with a thorough precondition/precondition-guarantor doc comment.
  - cmd/spawn.go:125 — external, trigger := spawn.SplitNetN(sessions) replaces the former inline sessions[:n-1] / sessions[n-1]; surrounding pre-flight, N=1 (len(external)==0) branch, and count-semantics logic unchanged.
  - internal/tui/burst_progress.go:480 — external, trigger := spawn.SplitNetN(ordered) replaces the former inline ordered[len-1] / ordered[:len-1]; decideBurst empty-ordered guard (burst_progress.go:413) unchanged.
- Notes: Repo-wide grep confirms SplitNetN is the only net-N split site — no inline [:n-1]/[n-1] or [:len-1]/[len-1] slice expressions remain in either caller. The helper sits in package spawn alongside its sibling shared primitives, exactly as intended. Byte-identity holds: SplitNetN's returns are the same subslice/element expressions the two callers used before (same backing-array aliasing, no copy), so behaviour is unchanged. Guards preserved on both sides: CLI's len(args)==0 usage gate (spawn.go:98) guarantees n>=1 before runSpawn; picker's decideBurst len(ordered)==0 early-return (burst_progress.go:413) guarantees len(ordered)>=1 before dispatchBurst. No zero-length slice can reach SplitNetN. Assignment order (external, trigger) is independent of the prior TUI order (trigger, external) — no control-flow impact; sync/async ordering untouched.

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/split_test.go — TestSplitNetN: table-driven over 2-element, 3-element, and single-element slices, asserting external/trigger. Single-element case pins empty external + that element as trigger (the boundary the doc calls out).
  - internal/tui/burst_dispatch_test.go:488 — TestBurstDispatch_SplitDerivesFromSplitNetN: the cross-caller drift guard the task's Tests section asks for — presses Enter on a marked supported model and asserts the picker's recorded BurstExternal/BurstTrigger are byte-identical to spawn.SplitNetN(fixture), then drains the async pipe to avoid a lingering goroutine.
  - Existing CLI net-N regression suites (cmd/spawn_test.go, cmd/spawn_seams_test.go) and picker-burst suites remain in place and exercise the unchanged behaviour through the refactored callers.
- Notes: Not over-tested — three focused unit rows plus one behavioural cross-caller guard, no redundant variations. Not under-tested — the single-element boundary and the cross-caller equivalence (the whole point of the refactor) are both covered. Tests verify behaviour (returned values / recorded split) rather than implementation detail. They would fail if either caller re-hand-rolled a divergent split.

CODE QUALITY:
- Project conventions: Followed — pure leaf helper in the shared package (golang-design-patterns), documented precondition with the guarantor named, table-driven test with t.Run subtests (golang-testing), no logging in the leaf. slices.Equal used for slice comparison.
- SOLID principles: Good — single responsibility, single source of truth for the invariant.
- Complexity: Low — one-line pure function.
- Modern idioms: Yes — multi-return, slices.Equal.
- Readability: Good — doc comments on both the helper and the two call sites explain the "cannot drift" rationale and the precondition guarantors.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The single-element test row declares wantExternal: nil while SplitNetN returns an empty non-nil slice; slices.Equal is length-based so this passes and correctly documents "empty external" — no action needed, so not recorded as a finding.)
