TASK: spectrum-tui-design-1-7 — Light/dark detection (OSC 11) + appearance override + detect-or-timeout first-paint gate (dark fallback)

ACCEPTANCE CRITERIA:
- (VISUAL) Captured Sessions screens match `Sessions — Modern Vivid v2` (dark) and `Sessions — Modern Vivid (Light)` (light); correct canvas on frame one, no paint-then-flip.
- Auto mode: OSC 11 (tea.RequestBackgroundColor/BackgroundColorMsg) drives canvas mode; first real paint gates on detect-resolved-OR-short-timeout.
- appearance light/dark pins the mode and skips detection AND the wait; only auto runs detection.
- No-answer/timeout → dark fallback; mis-detection is cosmetic-not-broken (floor holds against whichever canvas is painted).
- COLORFGBG is a weak secondary hint only and never overrides an OSC 11 answer.
- Behaviour parity: detection adds startup messages but does not alter Sessions navigation/selection/filter/key behaviour once resolved.

STATUS: Complete

SPEC CONTEXT (§2.6 + §10.2):
Portal owns a mode-matched canvas, so it must decide which canvas (light/dark) to paint. Mechanism is OSC 11 (Bubble Tea v2 RequestBackgroundColor → BackgroundColorMsg); COLORFGBG is a weak secondary hint only ("use it, if at all, only as a tie-break/early hint" — not consulting it is explicitly permitted). The reply is async, so the first real paint gates on "detection resolved OR a short timeout (tens of ms)" — never paint-then-flip. No-answer → dark fallback (termenv default, MV is dark-first). The `appearance` pref (auto/light/dark) pins the mode and skips both detection and the wait. NO_COLOR skips detection entirely (no canvas to select). The §10 cold-path loading page gates the same way. The chosen timeout value is left as an implementation detail in the tens-of-ms range; the chosen value + rationale must be recorded.

IMPLEMENTATION:
- Status: Implemented (clean, well-factored)
- Location:
  - internal/tui/appearance_gate.go:1-167 — the reusable `appearanceGate` (the single-resolution detect-or-timeout mechanism): mode/pending/pinned/colourless fields, newAppearanceGate / newColourlessGate constructors, arm(), timeoutCmd(), resolve()/resolveDark()/resolveFromDark(), and the 50ms `appearanceDetectTimeout` with a recorded rationale block (lines 11-24).
  - internal/tui/model.go:1056-1113 — gate construction in New (NO_COLOR > pin > auto precedence at 1069-1079), armAppearanceDetection (1091), modeResolved (1101), syncResolvedMode (1110).
  - internal/tui/model.go:1859-1930 — Init: issues the OSC 11 query (requestBg, nil under NO_COLOR) and the detect-or-timeout tick (gate.timeoutCmd, nil when resolved/pinned), batched on every path so the two race.
  - internal/tui/model.go:1947-1979 — Update: BackgroundColorMsg captures originalBg (nil-guarded against the no-answer panic) AND resolves the auto gate via resolveFromDark(msg.IsDark()); appearanceTimeoutMsg resolves the dark fallback via resolveDark. Both are no-ops once resolved (no flip).
  - internal/tui/model.go:3245-3302 — View: the first-paint gate — `if !m.modeResolved()` returns the neutral `blankFrame()` (full-terminal plain spaces, NO canvas SGR), else paints the real canvas.
  - internal/tui/build.go:97-171 — production wiring: WithAppearance injected from prefs, then armAppearanceDetection() opens the window for the live picker (no-op for pin/colourless).
  - cmd/open.go:499-507,547 — reads the persisted appearance tolerantly and injects it; the program launches at open.go:596.
- Notes:
  - The gate is genuinely reusable as the spec/plan asked ("structure the gate for reuse" — the §10 Phase 5 loading page shares it); the negatively-named `pending` flag is unusual but the rationale is documented in-source (model-literal test models read resolved by default).
  - The OSC 11 query in Init still fires when appearance is PINNED. This is correct: that query is dual-purpose (restore-on-exit background capture is orthogonal to detection and must always fire), and detection itself is correctly skipped because resolveFromDark is a no-op on a resolved gate, while the WAIT (timeout tick) is skipped because timeoutCmd returns nil for a pinned gate. The plan's "skip the OSC 11 query" refers to skipping detection-driven gating; the observable contract (paint frame one, no wait, no flip) holds and is test-pinned. Not a defect — worth being explicit about because the wording could read as a miss.
  - The temporary 1-6 mode source (WithCanvasMode) is correctly repurposed as a test/capture-only direct override; production is now driven solely by appearance + detection. The View() outer wrap point is unchanged (parity preserved).

TESTS:
- Status: Adequate
- Coverage (internal/tui/appearance_detection_test.go + appearance_option_test.go):
  - TestAutoDetectsDark / TestAutoDetectsLight — blank-frame-before, correct canvas painted after the OSC 11 reply (asserts the actual canvas SGR is present in View, not just a flag).
  - TestNoPaintThenFlip — the load-bearing one: blank before resolution; correct canvas after; a late timeout AND a late conflicting light reply both fail to flip the resolved Dark mode. This directly verifies the single-resolution invariant.
  - TestTimeoutFallsBackToDark — timeout-before-reply resolves dark and paints.
  - TestPinLightSkipsDetection / TestPinDarkSkipsDetection — resolved at construction, paints from frame one, and assertNoTimeoutTick confirms Init arms NO timeout tick.
  - TestAutoArmsTimeoutTick — auto Init DOES arm the tick (the no-answer fallback path exists).
  - TestColorFGBGNeverOverridesOSC11 — OSC 11 dark wins with COLORFGBG advertising light.
  - TestMisdetectionLegibleNotBroken — a wrong-but-painted canvas still renders (not blank/crashed).
  - TestBuildArmsAutoGate — pins the PRODUCTION wiring (Build): auto gates the first paint and resolves via OSC 11; light/dark pins paint from frame one. This is the critical "production path, not just the model" test.
  - TestWithAppearance (option_test) — option sets the field, defaults to auto.
  - The blank-frame assertion checks for absence of the "Sessions" title; the painted assertion checks for both "Sessions" AND the mode's exact canvas SGR — strong, behaviour-anchored assertions.
- Notes:
  - Not over-tested; each test targets a distinct edge from the spec.
  - TestColorFGBGNeverOverridesOSC11 is weak by construction: the implementation never reads COLORFGBG, so the env var is inert and the test passes vacuously (it would pass identically with COLORFGBG unread). It still correctly asserts the observable contract (OSC 11 wins), and not-reading COLORFGBG is a spec-sanctioned choice ("if at all"). The test documents the intended invariant; flagged as non-blocking only.
  - The no-answer (nil Color) → dark path is exercised indirectly (the nil-safety of msg.IsDark() and the originalBg nil-guard), but there is no test that feeds a `BackgroundColorMsg{Color: nil}` directly through Update to pin that the nil-reply collapses to dark without panicking. The timeout path covers the practical no-answer outcome, but a direct nil-Color Update test would lock the nil-guard at model.go:1964.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); deterministic message-driven tests; small interfaces; option-based DI; no bare os.Exit; logging untouched (not a logged surface). Idiomatic Go.
- SOLID principles: Good. The gate is a single-responsibility value type owning exactly the resolution race; the model delegates to it (modeResolved/syncResolvedMode) rather than duplicating flags.
- Complexity: Low. resolve() is the one single-resolution core; resolveDark/resolveFromDark/timeoutCmd are thin wrappers. The New() precedence (colourless > non-auto-or-unpinned > rebuild) is the one branch needing care and it is heavily commented with the WithCanvasMode-guard rationale.
- Modern idioms: Yes — value receiver for the pure read (resolved), pointer receivers for mutation; tea.Tick races BackgroundColorMsg via tea.Batch.
- Readability: Good — arguably over-commented, but the comments are load-bearing for a concurrency/timing seam and pull their weight.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/appearance_detection_test.go:180-203 — TestAutoArmsTimeoutTick and assertNoTimeoutTick drain Init's batch via initCmds, which calls each cmd closure; the auto-mode timeout cmd (tea.Tick at appearance_gate.go:131) blocks on its 50ms timer, so each auto drain sleeps ~50ms. Detect the appearanceTimeoutMsg producer without executing it (e.g. reflect on the cmd, or lower the constant via a test seam) to drop the ~100ms of real sleep from the suite.
- [quickfix] internal/tui/appearance_detection_test.go (new test) — add a direct Update(tea.BackgroundColorMsg{Color: nil}) case asserting it resolves to Dark and does not panic, to lock the no-answer nil-guard at model.go:1964 and the nil→dark collapse of msg.IsDark() independently of the timeout path.
- [do-now] cmd/open.go:502-503 — the comment "The model only stores it for now; honouring it (skip detection + first-paint wait) is a later task." is stale: task 1-7 now honours it via Build → armAppearanceDetection. Update the comment to reflect that the value is honoured (pin → skip detection + wait).
- [idea] internal/tui/appearance_detection_test.go:151-157 — TestColorFGBGNeverOverridesOSC11 passes vacuously because COLORFGBG is never read. If a future tie-break/early-hint use of COLORFGBG is ever added (spec leaves it optional), this test would not catch a regression where COLORFGBG wrongly overrides a no-answer. Decide whether to (a) leave as-is documenting the never-read invariant, or (b) strengthen it only if/when COLORFGBG is wired as a hint. No action needed while COLORFGBG stays unread.
