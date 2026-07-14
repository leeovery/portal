TASK: restore-host-terminal-windows-6-9 — N≥2 on unsupported/NULL: atomic no-op + re-asserted banner

ACCEPTANCE CRITERIA:
- N≥2 Enter on any resolved-unsupported terminal (DetectUnsupported()) — NULL remote/mosh OR non-NULL recognised-but-undriven (Apple Terminal) — opens nothing (no burstProgressPipe, no adapter resolve/call), does not self-attach (Selected()=="", no tea.Quit), stays in multi-select mode with selection intact.
- Non-NULL undriven → re-asserts "⚠ unsupported terminal — <name> · <bundleID> — nothing opened"; bare-NULL → "⚠ no host-local terminal — nothing opened" (copy branches on IsNull(), gate branches on resolution — matching CLI Task 2.7).
- In-flight N≥2 Enter is deferred and takes the no-op on resolving unsupported.
- Transient-error identity (Identity{}, NULL) treated identically (atomic no-op).
- N=1 Enter self-attaches regardless of terminal (unchanged — Task 5.7).
- Selection unchanged (no prune) after the no-op.

STATUS: Complete

SPEC CONTEXT: Spec §Terminal Identity & Detection (Unsupported-terminal behaviour / Detection lifecycle) — N≥2 on unsupported/NULL is an atomic no-op, banner (re)asserted naming the detected identity; in-flight-at-Enter is awaited then branches; transient error folds to Identity{} → unsupported. §Multi-Select Mode (notice-band precedence) — the multi-select banner owns the section-header row in mode, so the unsupported warning re-asserts as a transient flash at the N≥2 Enter block. This is the picker analogue of the CLI's atomic N≥2 gate (Task 2.7).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/burst_progress.go:405-439 (decideBurst) — the single decision chokepoint both entry points route through. Unsupported branch (416-437): pre-flight FIRST (§8-3), then emitUnsupportedNoop + setFlash(unsupportedFlashText(...)); constructs no pipe, resolves no adapter, no m.selected, returns (m, nil). Supported → dispatchBurst.
  - internal/tui/burst_progress.go:372-385 (beginBurst) — in-flight → sets pendingBurstEnter (defers) and dispatches detection if never kicked off; resolved → decideBurst.
  - internal/tui/model.go:2481-2483 (terminalDetectedMsg arm) — deferred Enter re-runs decideBurst against the now-cached resolution with the set RE-DERIVED live (m.orderedMarkedSessions()).
  - internal/tui/model.go:3560-3574 (handleMultiSelectEnter) — N=1 case (3565-3570) is m.selected + tea.Quit, structurally independent of detection; N≥2 → beginBurst.
  - internal/tui/burst_progress.go:450-452 (unsupportedFlashText) → internal/spawn/message.go:67-72 (UnsupportedNoopMessage) — the single shared renderer; CLI unsupportedSpawnMessage (cmd/spawn.go:225-227) prepends only "spawn: ". Copy cannot drift.
  - internal/tui/spawn_detect.go:116-118 (DetectUnsupported) — gates on cached resolution == ResolutionUnsupported (not IsNull()), so a non-NULL undriven terminal is correctly unsupported.
- Notes: Faithful to the plan. The unsupported flash co-renders with the `N selected` banner across two rows by design (notice_band.go:347-363: the flash arm takes the §11 slot regardless of multiSelectMode; the multi-select banner owns the section-header row) — matching the spec's "re-asserts at the N≥2 Enter block" while the banner steps aside. dispatchBurst's nil-adapter guard (burst_progress.go:483-487) routes an inconsistent supported+nil-adapter resolve to the SAME no-op emit+flash — belt-and-braces, no adapter can reach NewBurster. No scope creep; no orphaned code.

TESTS:
- Status: Adequate
- Coverage (internal/tui/burst_unsupported_noop_test.go + burst_preflight_before_unsupported_test.go):
  - TestUnsupportedFlashText — pins exact copy both branches + asserts no embedded ⚠ glyph (band prepends it).
  - TestBurstUnsupported_NonNullAtomicNoOp — Apple Terminal: full atomic no-op (assertAtomicNoOp: no pending, no pipe, zero adapter Calls, Selected()=="", still multi-select, count==2, both names kept) + named flash + not isQuitCmd.
  - TestBurstUnsupported_NullFlash — Identity{} (also pins the transient-error edge, per its comment): same no-op + honest no-host-local flash.
  - TestBurstUnsupported_DeferredThenUnsupported — in-flight Enter defers (no flash, no adapter call, nil cmd), then terminalDetectedMsg lands the same no-op + flash.
  - TestBurstUnsupported_SupportedStillDispatches — regression guard: ghostty still bursts, no flash.
  - Preflight-before-unsupported (§8-3) covered for both entry points.
  - Copy is source-tested (internal/spawn/message_test.go:90-103) and CLI-tested (cmd/spawn_test.go:652,700), so picker/CLI drift is structurally impossible.
- Notes: assertAtomicNoOp is a well-factored shared assertion — no over-testing, each test varies one meaningful axis. N=1 self-attach is covered generally by TestMultiSelectEnterN1 (multi_select_enter_test.go) and the N=1 path never consults detection, so it is structurally isolated from this branch (see non-blocking note). The deferred test resolves to a non-NULL identity rather than a bare NULL, but the NULL flash and the deferred-no-op mechanism are each covered independently.

CODE QUALITY:
- Project conventions: Followed — single-renderer copy sharing (DRY across CLI/picker), seam-based DI (resolve/sessionExists/ackChannel/spawnExe injected), no t.Parallel, white-box tests consistent with the tui surface, closed log emission via emitUnsupportedNoop → spawn.LogUnsupported.
- SOLID principles: Good — decideBurst is a single decision chokepoint both entry points share; unsupportedFlashText delegates rather than re-implementing copy.
- Complexity: Low — three-way branch (empty / unsupported / supported) with clear guards; no nesting beyond the pre-flight guard.
- Modern idioms: Yes.
- Readability: Good — intent-dense comments explain the resolution-vs-IsNull gate, the §8-3 ordering, and the two-entry-point convergence.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/burst_progress.go:436 (and sibling internal/tui/burst_partial_failure.go:58,77) — the unsupported no-op returns (m, nil) after setFlash, scheduling no flashTickCmd, so this "transient" flash does not auto-clear on a timer the way the async externally-killed gone flash does (model.go:2413 schedules flashTickCmd). It clears only on the next actionable keystroke. On the deferred (async terminalDetectedMsg) path no keystroke is guaranteed, so an idle user keeps the flash indefinitely. Decide whether burst-family outcome flashes should auto-clear for consistency with the gone-flash — this is a family-wide design choice (shared with the already-shipped partial-failure path), not a 6-9-specific defect.
- [quickfix] internal/tui/burst_unsupported_noop_test.go — the task's listed test "leaves N=1 self-attach unaffected and the selection intact" has no focused counterpart wiring an unsupported terminal + one mark; N=1 is only covered generally (TestMultiSelectEnterN1, no detection wired). The N=1 path is structurally independent of the unsupported branch, so this is optional belt-and-braces coverage. Could fold in a deferred-then-bare-NULL assertion at the same time (the existing deferred test resolves to a non-NULL identity).
