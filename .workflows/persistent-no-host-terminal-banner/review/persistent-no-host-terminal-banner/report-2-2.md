TASK: persistent-no-host-terminal-banner-2-2 — Proactive Multi-Select Entry Block + TUI-Local Blocked-Entry Flash Helper (commit 5656e3e0)

ACCEPTANCE CRITERIA:
- Named unsupported (appleTerminalIdentity): m leaves MultiSelectActive()==false, SelectedSessionCount()==0, flashText=="multi-select isn't available on this terminal".
- NULL/remote (spawn.Identity{}): mode closed, flashText=="multi-select isn't available over a remote connection".
- Block flash clears on next actionable key.
- Named two-row co-render: unsupportedBannerActive()==true; bannerFirstLine carries ⚠ + "unsupported terminal"; renderActiveNoticeBand carries ⚠ + the block-flash string; band does NOT repeat "unsupported terminal"/identity/"see docs".
- Double-m keeps mode closed and re-sets the block flash (clear-then-reflash).
- In-flight (detectDispatched && !detectResolved): m still enters (A1 window not blocked).
- WithInitialMultiSelect + resolved-unsupported: multiSelectMode==true (construction seam ungated).
- Supported (ghosttyIdentity): m enters, no flash.
- multiSelectBlockedFlashText returns the two plain per-shape strings, neither containing flashWarningGlyph.
- go test ./internal/tui/... passes (unit lane).

STATUS: complete

SPEC CONTEXT: Sub-fix 2 (spec §3) — the pre-existing bug is that handleMultiSelectToggle's entry branch read no detection state, so on a resolved-unsupported terminal `m` opened a walkable dead-end mode whose N≥2 Enter could never fire a burst (only the downstream decideBurst reactive gate existed). The root-cause fix (Fork B → B1) is a proactive entry block that fails immediately with a visible honest per-shape flash rather than a silent swallow. §5 governs the copy: intent-only strings, NO ⚠ glyph (the notice band prepends it), NO "— nothing opened" suffix, and — because the flash co-renders two-row with the persistent banner on a named terminal — non-repetition of identity/"see docs"/"unsupported terminal". §4/§8 require an inline source note recording the latent guard-coupling with keymap_dispatch_guard_test's detection-unwired seed. The reactive backstop (decideBurst) is retained untouched for the async in-flight window.

IMPLEMENTATION:
- Status: Implemented (faithful to task, no drift)
- Location:
  - internal/tui/model.go:3497-3517 — multiSelectBlockedRemoteFlash / multiSelectBlockedNamedFlash constants + multiSelectBlockedFlashText(id) free function mirroring unsupportedFlashText's identity-shape branch (IsNull → remote copy, else named copy). Carries no ⚠, no "— nothing opened". Documented as TUI-local (not the CLI-shared spawn.UnsupportedNoopMessage).
  - internal/tui/model.go:3533-3557 — the proactive block at the TOP of the `if !m.multiSelectMode {` entry branch, before `m.multiSelectMode = true`: `if m.DetectUnsupported() { (&m).setFlash(multiSelectBlockedFlashText(m.detectIdentity)); return m, flashTickCmd(m.flashGen) }`. setFlash is the pointer method (called via (&m)); flashTickCmd captures the post-bump flashGen — identical lifecycle to the session-gone bail (model.go:2413-2414). The authoritative clear remains the next-actionable-key path (model.go:3328).
  - The inline guard-coupling source note (model.go:3545-3552) accurately records the NewModelWithSessions detection-unwired dependency for keymap_dispatch_guard_test's `m` probe.
- Verified untouched (as required): WithInitialMultiSelect (model.go:1006, sets multiSelectMode directly at construction — correctly NOT gated); decideBurst / unsupportedFlashText / emitUnsupportedNoop (burst_progress.go); internal/spawn/message.go. Commit stat confirms only model.go + the new test file + two reactive-backstop test files were touched (plus tick bookkeeping).
- Two-row co-render mechanism confirmed: on a blocked named `m`, multiSelectMode stays false so unsupportedBannerActive() (model.go:4736 — DetectUnsupported && !multiSelectMode && !IsNull) stays true → banner owns the section-header row; the §6-6 precedence seam in activeNoticeBand (notice_band.go:361) returns the flash for the band slot REGARDLESS of mode, so the flash co-renders on row 2. Both rows carry ⚠ (banner via renderUnsupportedHeader, band via the bandWarning role from setFlash's flashKind=flashWarning).
- Notes: The reactive-backstop test adaptations (burst_observability_test.go, burst_preflight_before_unsupported_test.go) correctly move markTwo BEFORE resolveDetection so entry happens in the in-flight window where the new block is inert (DetectUnsupported()==false), then resolve-unsupported + Enter still drives the retained decideBurst reactive no-op. This is the intended rework from the blocking dependency tick-68e162 and is logically sound.

TESTS:
- Status: Adequate (comprehensive, 1:1 mapping to criteria, not over-tested)
- Location: internal/tui/multi_select_entry_block_test.go (new, package tui, no t.Parallel)
- Coverage:
  - TestMultiSelectBlockedFlashText — pure-function copy for both shapes + negative assertions (no ⚠ glyph, no "nothing opened").
  - TestMultiSelectEntryBlock_NamedUnsupported / _NullRemote — core block: mode closed, zero marked, correct per-shape flash string.
  - _FlashClearsOnNextActionableKey — clear via KeyDown.
  - _NamedTwoRowCoRender — both rows carry ⚠; band carries the block string; band does NOT repeat "unsupported terminal"/"Apple Terminal"/"com.apple.Terminal"/"see docs" (non-repetition constraint).
  - _RepeatedMReBlocks — clear-then-reflash on second press.
  - _InFlightStillEnters — dispatched-but-unresolved window still enters (A1).
  - _WithInitialMultiSelectNotGated — construction seam opens the mode under resolved-unsupported detection (WithInitialDetection resolves com.apple.Terminal → ResolutionUnsupported via spawn.ResolveAdapter, so the precondition genuinely holds).
  - _SupportedEntersNoFlash — ghostty enters, no flash.
- Notes: Each test targets a distinct acceptance criterion — no redundant/bloated checks. Tests reuse established package helpers (unsupportedResolvedModel, pressSession, pressM, dispatchWarmDetection). Tests would fail if the feature broke (they assert both the closed-mode and flash-string outcomes, and the negative non-repetition set). No new-coverage gaps against the criteria. Judged by reading; not executed.

CODE QUALITY:
- Project conventions: Followed. Free-function copy helper + package-level constants mirror the existing unsupportedFlashText shape; the (&m).setFlash pointer-method call on the value receiver matches the surrounding mark-on-entry / session-gone-bail idiom; heavy explanatory comments match this file's density.
- SOLID principles: Good. Single-purpose helper; the gate is a minimal, well-isolated pre-condition at the branch top.
- Complexity: Low. One added guarded early-return; no new branches beyond the IsNull selector.
- Modern idioms: Yes. Idiomatic Go const block + identity-shape switch on IsNull().
- Readability: Good. The gate and the guard-coupling note are self-documenting.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/multi_select_entry_block_test.go:230 — TestMultiSelectEntryBlock_SupportedEntersNoFlash constructs its model via unsupportedResolvedModel(t, ghosttyIdentity()); the helper name reads "unsupported" while the case is deliberately the supported path. It is functionally correct (the helper just wraps warmResolvedModel), but calling warmResolvedModel(t, &fakeDetector{identity: ghosttyIdentity()}, nativeResolve()) directly — or adding a neutrally-named resolvedModel wrapper — would remove the misleading-name read at this one call site. Cosmetic only.
