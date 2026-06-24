TASK: spectrum-tui-design-5-6 — Fatal cold-boot error contract: in-TUI error state on the loading page (state.red marker + one-line message) + fatal-error-as-tea.Quit with non-zero exit (§10.5 / §10.2).

ACCEPTANCE CRITERIA:
- A fatal cold-boot step failure shows the in-TUI loading-page error state: failed step row carries a state.red marker + one-line message.
- The model stays on the loading page (no transition to a half-restored picker).
- q/Esc quits with a non-zero exit; openTUI returns the fatal error so main.classify yields code 1 (single-line stderr, no double-print).
- Only the four fatal steps (EnsureServer / RegisterPortalHooks / SetRestoring / ClearRestoring) abort this way; best-effort step failures warn-and-continue.
- The synchronous warm/CLI fatal-error exit path is byte-for-byte unchanged.
- VISUAL: error frame is MOCKED at implementation (not a §15.1 frame); capture compared against the mock.

STATUS: Complete

SPEC CONTEXT:
§10.5 mandates a fatal cold-boot step failure becomes an in-TUI error state on the loading page — the failed step gets a state.red marker + one-line message, q/Esc quits non-zero, never dropping into a half-restored picker; the error frame is mocked at implementation (no §15.1 Paper reference). §10.2 names "fatal-error-as-tea.Quit (today a PersistentPreRunE error return)" as a real cost: on the concurrent cold/TUI path the orchestrator runs in a goroutine while Bubble Tea is live, so a fatal can no longer return through PersistentPreRunE. §2.9 pins state.red; §2.2 requires state never carried by hue alone (the ✗ glyph reinforces the red).

IMPLEMENTATION:
- Status: Implemented (full, end-to-end, idiomatic)
- Location:
  - internal/tui/model.go:200-204 — BootstrapFatalMsg{FailedStep, Message, Err}.
  - internal/tui/model.go:308-311 — model fatal state fields (fatalActive/fatalStep/fatalMessage/fatalErr).
  - internal/tui/model.go:2059-2072 — BootstrapFatalMsg arm: sets fatal state, does NOT re-issue the receiver (terminal), never flips PageSessions.
  - internal/tui/model.go:2210-2212 — PageLoading key arm: q OR Esc → tea.Quit only when fatalActive.
  - internal/tui/model.go:557-565 — FatalError() accessor.
  - internal/tui/model.go:3795-3800 + viewLoading — renders FailedView on fatalActive.
  - internal/tui/model.go:2007-2009 / 2041-2043 — LoadingMinElapsedMsg + BootstrapCompleteMsg both guarded on fatalActive (no transition).
  - internal/tui/loading_progress.go:228-258 — FailedView projects the error-frame input (bar frozen at fatal-time fraction; failed label → LabelFailed; later labels stay pending). LabelForStepIndex (261-268) maps the 1-based step index to the friendly label.
  - internal/tui/loading_view.go:73,403-417,462-473 — ✗ glyph (loadingGlyphFailed), state.red painting for glyph+label+message, height-budgeted error footer (message + quit hint, shed in priority order under §2.7).
  - cmd/bootstrap_progress.go:184-198 — goroutine computes failedStep = lastStep+1 (carried on the EVENT, value copy, per the 5-2 carry-forward race contract); 230-275 — receiver maps the terminal fatal event → tui.BootstrapFatalMsg (NOT BootstrapCompleteMsg); fatalMsgFromEvent extracts FatalError.UserMessage and rides Err through.
  - cmd/open.go:420-428 — processTUIResult returns model.FatalError() BEFORE any connect.
  - cmd/root.go:170-183 — only cold+TUI defers; 185-220 + Execute (271-287) + main.classify (84-89) keep the synchronous fatal path unchanged.
- Notes:
  - failedStep = lastStep+1 is correct against the orchestrator: each StepEvent is emitted at the step-complete site, and a fatal returns o.fatalf WITHOUT emitting, so the next un-emitted index is exactly the failing step. Confirmed for all four fatal steps (1→1, 2→2, 3→3, 8→8). LabelForStepIndex maps 1→Started tmux server, 2/3→Registered hooks, 8→Running resume commands — all valid (none is the dual-mapped restore step).
  - Best-effort steps (4,5,6-restore,7,9,10,11) all warn-and-continue (o.Logger.Warn, no o.fatalf return) — confirmed in cmd/bootstrap/bootstrap.go. They never produce a fatal terminal event, so the error state is unreachable from them.
  - Exit-code path is fully load-bearing and clean: no bare os.Exit outside main; the fatal flows openTUI→Execute(writes UserMessage once)→main.classify→code 1. Returns the SAME *bootstrap.FatalError instance for byte-for-byte parity.
  - internal/tui correctly stays decoupled from cmd/bootstrap — the fatal rides as a plain error interface (Err), extracted via errors.As cmd-side.

TESTS:
- Status: Adequate (well-balanced; covers every acceptance criterion + edges, no redundancy)
- Coverage:
  - internal/tui/loading_fatal_test.go — error-state render (✗ + message + failed label + quit hint), no-overflow at 80x24, stays-on-PageLoading (incl. late complete/progress not rescuing), q→Quit, Esc→Quit, FatalError() carries the error, non-fatal run → nil FatalError.
  - internal/tui/loading_fatal_internal_test.go — failed row is red ✗ (asserts the actual state.red SGR seq via tokenFgSeq), step states around the failure (pre=✓, post=·), never-overflows-height across 6 dimensions incl. degrade.
  - cmd/bootstrap_progress_fatal_test.go — fatal@step3 → BootstrapFatalMsg{FailedStep:3, UserMessage, Err}, NOT BootstrapCompleteMsg; fatal@step1 → FailedStep:1; non-fatal still completes (no fatal leak).
  - cmd/open_fatal_test.go — processTUIResult returns the SAME *bootstrap.FatalError instance + skips connect; no-fatal path unchanged (clean exit nil, selection still connects).
  - internal/capture/capture_test.go:177-231 — drives the loading-error fixture, asserts PageLoading park + ✗ + message + quit hint (the mock-and-compare harness behind the vhs tape).
- Notes:
  - Maps 1:1 to the task's Tests list; each acceptance criterion has a direct assertion. The "byte-for-byte unchanged warm/CLI path" criterion is covered structurally (deferral scoped to cold+TUI; sync path untouched) and behaviourally via TestProcessTUIResult_NoFatalUnchanged + the existing Execute/classify tests; q-vs-Esc both asserted; fatal-at-mid-sequence (step 3, with steps 1-2 done) is the central render case.
  - Not over-tested: each test pins a distinct behaviour; the two height-overflow assertions sit at different layers (model View vs renderLoadingScreen direct) and are justified by the §2.7 footer-budget regression they guard.

CODE QUALITY:
- Project conventions: Followed. cmd tests carry the "NO t.Parallel()" note; internal/tui stays decoupled from cmd/bootstrap (fatal as error interface); leaf canvas-paint delegates via loadingStyle/loadingFg; single-source step→label table reused (LabelForStepIndex sibling of LabelForStep); errors.As round-trips the *bootstrap.FatalError per golang-error-handling.
- SOLID principles: Good. FailedView is a pure projection beside View; fatal state is a cohesive model concern; the pipe's fatal mapping is a single function (fatalMsgFromEvent).
- Complexity: Low. The fatal arm is a flat field-set + terminal return; FailedView is a single labelled loop; failedStep is a one-line lastStep+1.
- Modern idioms: Yes — errors.As, value-copy event carry to dodge the goroutine happens-before race, defensive guards on every transition gate.
- Readability: Good. Comments are precise and cite the spec sections + the 5-2 carry-forward race contract.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] testdata/vhs/loading-error.png — the hero wordmark renders as "PORTALI" (the violet caret block sits flush against the trailing L with no visible gap), so at a glance it reads as a 7-letter word rather than PORTAL + caret. This is pre-existing 5-5 wordmark geometry, not introduced by 5-6, and it is most legible exactly on this error frame's static capture. Worth deciding whether the caret needs a clearer gap from the L; out of scope for this task.
- [quickfix] cmd/bootstrap_progress.go:265 — fatalMsgFromEvent dereferences ev.Fatal.Error() unconditionally before the errors.As; it is only reached when ev.Fatal != nil (receiver guards at :237), so it is safe today, but a defensive nil-guard (or a comment pinning the caller's non-nil contract at the function boundary) would harden it against a future caller that does not pre-check.
