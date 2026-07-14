TASK: restore-host-terminal-windows-10-1 (tick-7314d9) — Cache the burst Adapter at detection time so dispatchBurst cannot re-resolve to a nil adapter and panic (BUG, medium severity)

ACCEPTANCE CRITERIA:
1. m.resolve invoked exactly once per detection (in the terminalDetectedMsg arm); dispatchBurst reads m.detectAdapter/m.detectResolution and no longer calls m.resolve.
2. Config-script terminal whose script is removed between detection and Enter: the N≥2 burst does NOT panic — uses the cached adapter (fails cleanly through partial-failure) or, if nil, routes to the unsupported no-op; never constructs spawn.NewBurster(nil, …).
3. The redundant second os.Stat on the config-script path is eliminated (one resolve per detection).
4. Supported native/argv/config-script bursts behave identically for the un-mutated case; the empty-ordered no-op and the DetectUnsupported gate are unchanged.

STATUS: Complete

SPEC CONTEXT: This is a Phase 10 analysis-cycle BUG fix, not a §-numbered spec requirement. The underlying feature (§6 N≥2 picker burst) resolves the host terminal once at page-entry detection and caches the Resolution; dispatchBurst previously re-resolved to recover an Adapter. Because the config-script recipe adapter (spawn.newScriptRecipeAdapter) live-os.Stat's its script on every resolve, a script deleted/de-executabled between detection and Enter makes the second resolve return (nil, ResolutionUnsupported) while the cached (supported) resolution still bypasses the DetectUnsupported gate — so spawn.NewBurster(nil,…) was constructed and the un-recovered burst goroutine (bare go func(), no recover) nil-panicked the whole picker. The fix caches the Adapter alongside the Resolution from the single detection-time resolve and reads it at dispatch.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:476 — new `detectAdapter spawn.Adapter` field beside detectResolution, documented (468-475) as cached in lockstep from the same single resolve.
  - internal/tui/model.go:2471 — terminalDetectedMsg arm now `m.detectAdapter, m.detectResolution = m.resolve(msg.identity)`, retaining both halves of the single resolve (comment 2459-2469).
  - internal/tui/burst_progress.go:482 — dispatchBurst reads `adapter, resolution := m.detectAdapter, m.detectResolution` (no re-resolve).
  - internal/tui/burst_progress.go:483-487 — nil-adapter guard: on nil cached adapter, emit the unsupported outcome + set the unsupported flash and return — never constructs the burster. Mirrors decideBurst's unsupported branch.
  - internal/tui/burst_progress.go:454-474 — dispatchBurst doc comment rewritten: describes reading the detection-time cache, the removed second resolve, the TOCTOU rationale, and the belt-and-braces guard.
  - internal/tui/spawn_detect.go:59-70 — WithInitialDetection (capture-harness seed) also caches the adapter in lockstep via spawn.ResolveAdapter, so the harness path keeps detectAdapter/detectResolution consistent too.
- Notes: All five prescribed edits present and mutually consistent. The single-resolve invariant is verifiable structurally: the ONLY production m.resolve call site is now model.go:2471 (confirmed by grep — appearance_gate.go's g.resolve is an unrelated field; WithInitialDetection uses the package-level spawn.ResolveAdapter, not m.resolve). dispatchBurst no longer references m.resolve. The empty-ordered guard (decideBurst, burst_progress.go:413) and the DetectUnsupported gate (spawn_detect.go:116-118) are untouched. Item 6 (no other reader depends on the second resolve) verified: no other production caller of m.resolve exists.

TESTS:
- Status: Adequate
- Location: internal/tui/burst_cached_adapter_test.go
- Coverage:
  - TestBurstDispatch_UsesCachedAdapter_AlreadyResolved — direct N≥2 Enter (detection already resolved). A counting TOCTOU seam returns a non-nil adapter + ResolutionNative on the FIRST resolve and (nil, ResolutionUnsupported) on every later call; asserts *calls==1 after dispatch (dispatchBurst did not re-resolve), the burst is pending, and drives the goroutine to completion asserting the CACHED adapter opened exactly one external window (Calls==1). This is the core single-resolve + no-panic proof: a re-resolve would have returned nil and nil-panicked the un-recovered goroutine (which would crash the test binary), so a passing test genuinely proves the fix.
  - TestBurstDispatch_UsesCachedAdapter_DeferredEntry — the deferred-detection entry (terminalDetectedMsg → decideBurst → dispatchBurst): asserts the Enter defers while detection is in flight (no resolve, no dispatch, nil cmd), then on resolution *calls==1 across the whole path and the cached adapter opens the one window. Covers the second burst entry point per the tick's Tests section.
  - TestBurstDispatch_NilCachedAdapter_RoutesToUnsupportedNoOp — belt-and-braces: a resolve returning (nil, ResolutionNative) leaves DetectUnsupported false so decideBurst falls through to dispatchBurst; asserts the guard routes to the unsupported no-op — not burst-pending, no pipe, no tea.Quit, no self-attach, still multi-select, selection intact (count 2), and the unsupported flash set (== unsupportedFlashText(identity)). Directly covers AC2's nil branch and AC4's "stays in multi-select / selection intact".
- Notes: Well-targeted, no over-testing. Both entry points and both branches (cached-non-nil, cached-nil) are covered; the counting-seam pointer is the exact mechanism to prove the single-resolve invariant. Driving a real goroutine + real Burster + FakeAdapter (rather than mocking dispatch) makes the no-panic assertion honest (a nil-interface panic in the goroutine would abort the process). Regression for the un-mutated native/argv/config-script case is carried by the existing burst suites (wireBurstSeams returns the same adapter for any identity, so they remain green under the cache read) — appropriately not duplicated here. Minor: the TOCTOU seam labels the first resolve ResolutionNative rather than the real-world ResolutionConfig; this is a faithful behavioural proxy (any supported resolution exercises the same fall-through), acknowledged in the test comment — no change needed.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel (per CLAUDE.md and the file's own note); white-box package tui tests consistent with the surrounding surface; seams injected directly on the model; spawntest.FakeAdapter/FakeAckChannel used as the canonical spawn doubles. Pointer-vs-value receiver discipline intact (dispatchBurst is a value receiver returning the mutated Model, matching the Bubble Tea Update convention).
- SOLID: Good. The fix tightens the single-resolve responsibility (resolution owned solely by the detection arm) and removes a hidden second dependency-on-time in dispatchBurst.
- Complexity: Low. One field, one assignment change, one 4-line guard.
- Modern idioms: Yes.
- Readability: Good. Doc comments at all three cache sites (field, terminalDetectedMsg arm, dispatchBurst) explain the lockstep invariant and the TOCTOU rationale clearly.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/burst_progress.go:483-487 and :434-436 — the unsupported no-op body (`m.emitUnsupportedNoop(m.detectIdentity); m.setFlash(unsupportedFlashText(m.detectIdentity)); return m, nil`) is now duplicated verbatim in decideBurst's unsupported branch and dispatchBurst's nil-guard. Extract a small `func (m Model) unsupportedNoOp() (Model, tea.Cmd)` helper and call it from both, so the two no-op sites cannot drift. Low risk, mechanical.
