TASK: spectrum-tui-design-5-7 — Soft warnings ride the progress channel to a post-load notice band after the picker appears (cold/TUI path); reworks bootstrapWarnings package-sink delivery on the cold/TUI route only.

ACCEPTANCE CRITERIA (from tick-93c4fc):
- On cold + TUI, soft warnings surface as a post-load notice band after the picker appears (not over the loading page, not via alt-screen toggle)
- The post-load notice reuses the Phase 4 notice-band primitive and obeys the single-slot rule
- Zero warnings produces no notice
- The warm/CLI warning delivery path (package sink -> stderr for CLI) is byte-for-byte unchanged
- The cold/TUI path no longer relies on the pre-launch package-sink staging / synthetic-BootstrapCompleteMsg fold for warnings
- Non-visual plumbing — vhs-exempt; verification is behavioural
- Refinement (note 1): post-load notice pinned to orange/warning role + TRANSIENT lifetime (auto-clears on next actionable keypress via flashGen/isActionableKey, plus a timeout tick)
- Carry-forward (5-2): read warnings off the terminal channel EVENT (value copies), not the pipe struct fields, to avoid a happens-before race

STATUS: Complete

SPEC CONTEXT:
§10.5: soft warnings ride the progress channel and surface as a post-load notice after the picker appears (vs the old stderr alt-screen toggle). §11: left-bar accent notices; accent.orange = transient/warning; single-slot rule (the slot holds at most one band; a transient flash takes the slot and auto-clears on next keypress or timeout). §11.2: the inline-flash band is the transient orange/warning primitive built in Phase 4. §10.2: the cold/TUI route runs the orchestrator in a goroutine and streams a progress channel, replacing today's context + package-memo delivery; warm/CLI keeps the synchronous path.

IMPLEMENTATION:
- Status: Implemented
- Send side (cmd): cmd/bootstrap_progress.go:182,192-198 — the orchestrator goroutine writes the accumulated []bootstrap.Warning onto the terminal `bootstrapProgress{Done:true, Warnings: warnings, ...}` event (a value copy on the event). Receiver cmd/bootstrap_progress.go:240-243 reads `ev.Warnings` (the event copy, NOT p.warnings) into `tui.BootstrapCompleteMsg{Warnings:...}` — honours the 5-2 carry-forward race-avoidance note. bootstrap.Warning / tui.BootstrapWarning are both aliases of warning.Warning, so the slice passes through with no copy.
- Cold/TUI route scoping (cmd/root.go:170-183): shouldRunConcurrentBootstrap returns early BEFORE the package-sink Add/EmitTo block, so the cold/TUI route never touches bootstrapWarnings — warnings flow solely over the channel. No double-delivery.
- Warm/CLI route (cmd/root.go:185-201): unchanged — bootstrapWarnings.Add for every warning, EmitTo(cmd.ErrOrStderr()) for !isTUIPath. Warm-TUI keeps stageBootstrapWarningsOnModel (cmd/open.go:582).
- Model buffering + surfacing: model.go:2044-2057 (BootstrapCompleteMsg arm) and model.go:2010-2017 (LoadingMinElapsedMsg arm) both buffer msg.Warnings only while on PageLoading, then on transition call tea.Batch(m.surfaceBufferedWarnings(), m.refetchSessionsAfterRestore()). Orphaned completes (after dismissal) drop their warnings (model.go:2048).
- surfaceBufferedWarnings (internal/tui/bootstrap_warnings.go:114-128): routes by progressReceiver — cold/TUI (receiver != nil) → setFlash(formatWarningsFlash(...)) + flashTickCmd; warm/staging (receiver == nil) → flushBufferedWarningsCmd (the unchanged stderr path).
- Transient lifecycle: setFlash (model.go:1736-1745) → flashKind=flashWarning → activeNoticeBand (notice_band.go:347-355) → bandWarning (orange). Auto-clears on next actionable keypress (model.go:2949-2953, clearFlash) and on the timeout tick (flashTickCmd). Matches the §11.2 flash hand-off exactly.
- Phase 4 primitive reuse: NO new render path — the warning rides the same flashText/flashKind → activeNoticeBand → viewSessionList single-insert arbiter the §11.2 flashes use. formatWarningsFlash (bootstrap_warnings.go:55-61) joins all warning lines with "\n"; the band splits on "\n" and repeats the left-bar per line (notice_band.go:240,269), so multi-warning messages render multi-row in observation order.
- Zero warnings: formatWarningsFlash yields "" → surfaceBufferedWarnings returns nil, no setFlash, no tick, no flush (bootstrap_warnings.go:122-125).

TESTS:
- Status: Adequate
- internal/tui/sessions_postload_warning_test.go covers: surfaces post-load on PageSessions via both transition arms (Complete-then-MinElapsed and MinElapsed-then-Complete); band role = bandWarning; renders in Sessions view chrome incl. the noticeBarGlyph; does NOT appear over the loading page; zero warnings → no band AND no stderr flush; no stderr flush even with warnings; transient auto-clear on actionable keypress; multiple-warning observation-order preservation; best-effort step warning does not abort boot (lands on PageSessions, not the fatal frame); warm/staging route STILL flushes to stderr and does NOT surface an in-TUI band; formatWarningsFlash flattening (nil/empty/no-lines → "", and "\n"-join order).
- cmd/bootstrap_progress_test.go:192 TestBootstrapProgressPipe_CarriesWarningsOnTerminalEvent — asserts the orchestrator warnings ride the channel event onto BootstrapCompleteMsg.Warnings (value copies, multi-warning, multi-line). Closes the cmd-boundary half the tui tests can't reach.
- Coverage maps 1:1 onto every acceptance criterion + every named edge case (zero warnings, best-effort-step-no-abort, single-slot transient lifetime documented + asserted, multiple warnings ordered). Not over-tested: each test asserts a distinct property; the two transition-arm tests are justified (both arms are real transition sites). Tests correctly drive BootstrapCompleteMsg directly rather than the channel (the channel-carry is covered separately at the cmd boundary) — a clean seam split, not a gap. Tests would fail if the feature broke (e.g. wrong role, surfacing over loading page, stderr flush on cold path, no auto-clear).

CODE QUALITY:
- Project conventions: Followed — no t.Parallel (cmd-discipline family, documented in the test header); package-level seam (SetFlushWarningsToStderrForTest) with restore; reuses existing flash primitives rather than duplicating; closed-token discipline preserved (bandWarning → accent.orange, no literal hex). Aliases keep call sites readable.
- SOLID principles: Good — surfaceBufferedWarnings is a single route-dispatch chokepoint both transition arms share; no logic duplicated across the two arms; warm vs cold split is a clean single branch on progressReceiver.
- Complexity: Low — the new control flow is a single nil-check branch plus a "" sentinel guard; the rest is existing primitives.
- Modern idioms: Yes — value-copy-on-event for the race-avoidance contract; tea.Batch composition; empty-string sentinel reused consistently.
- Readability: Good — the doc comment on surfaceBufferedWarnings precisely documents both routes, the role choice, and the transient lifetime decision (satisfies the task's "document the choice" requirement).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/bootstrap_progress.go:182,284 — `p.warnings = warnings` plus the `Warnings()` accessor have no production caller: the cold/TUI delivery reads `ev.Warnings` off the event (the correct race-safe path), so this struct field + accessor are dead for delivery. It is a parallel-accessor sibling of the equally-unused `ServerStarted()` (line 279) and `Err()` (line 288), so it matches a pre-existing (pre-5-7) scaffolding pattern rather than introducing a new smell. If the trio is genuinely test-only/future-API, a one-line comment marking them as such would prevent a future reader assuming `p.warnings` is the live delivery field (and risking a regression that reads it instead of the event). Low priority; safe to leave.
