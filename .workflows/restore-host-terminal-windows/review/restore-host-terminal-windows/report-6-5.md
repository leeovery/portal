TASK: restore-host-terminal-windows-6-5 — Input-lock while pending + `Opening n/N…` feedback band

ACCEPTANCE CRITERIA:
- While burstPending, a second Enter does not re-dispatch a burst (no new pipe, no additional adapter calls).
- While burstPending, m, navigation keys, Space, /, and s are all no-ops (swallowed).
- While burstPending, Ctrl-C and Esc are live (route to cancelBurst — 6-8).
- Each spawnProgressMsg advances BurstDone() only; BurstTotal() stays at the dispatch-time N; band renders Opening burstDone/N… (Opening 1/3… then Opening 2/3…, never 2/2, never 3/3).
- The Opening n/N… band renders with precedence just below the filter line — above the transient flash, multi-select banner, and unsupported banner.
- Under NO_COLOR the Opening n/N… text renders on the native fg/bg with hue dropped.

STATUS: Complete

SPEC CONTEXT:
Spec *Burst & Partial-Failure Contract → In-picker execution model*: "In-burst feedback" (a pending affordance in the notice-band single-slot arbiter, e.g. `Opening n/N…`) and "Input-locked while pending" (the picker is inert to row actions — m, navigation, Space, /, s, and a second Enter are all ignored; only cancel Ctrl-C/Esc is live — preventing any race between concurrent user input and the completion handler's selection mutation). Spec *Multi-Select Mode → Mode affordance (notice-band precedence)*: filter line → in-burst `Opening n/N…` → transient flash → multi-select banner → unsupported banner → no-tags signpost. Denominator is N (marked-set incl. the silent self-attach trigger); the band never reaches N/N ("no 14/14 nag").

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:3295-3300` — input-lock guard: first step of the `tea.KeyPressMsg` case in `updateSessionList`; while `m.burstPending`, Ctrl-C/Esc route to `cancelBurst()`, every other key returns `(m, nil)`. Placed after the modal check and BEFORE the flash-clear, abort-banner dismissal, the top-level Ctrl-C→tea.Quit, the `SettingFilter` guard, and the rune switch — so no row handler fires while pending.
  - `internal/tui/model.go:2485-2496` — `spawnProgressMsg` arm folds only `m.burstDone = msg.Done`, deliberately leaving `m.burstTotal` untouched (comment documents that `msg.Total` = external N−1 must not shrink BurstTotal), re-issues `m.burstPipe.receiver()` (nil-pipe guard returns `(m, nil)`).
  - `internal/tui/model.go:4716-4724` — `applySectionHeader`: `burstPending` claimant is placed immediately after the `Filtering` check and BEFORE the abort banner, multi-select banner, unsupported banner, and standard header — highest section-header claimant below the live filter input. Replaces line 0 via `replaceHeaderLine` (pagination budget unchanged).
  - `internal/tui/section_header.go:147-151` — `renderOpeningBand(done, total, width, mode, colourless)`: `fmt.Sprintf("Opening %d/%d…", done, total)` (U+2026) in `theme.MV.AccentViolet`, composed through the shared `renderRightAnchoredSectionRow` core with no right hint. Reuses the same geometry as the multi-select / unsupported / abort banners — no new token, one row.
  - `internal/tui/burst_progress.go:300-308` — `BurstDone()`/`BurstTotal()` accessors (BurstTotal added in 6-3).
- Notes: Routing verified — a `tea.KeyPressMsg` on PageSessions never matches a top-level `Update` message-type case (lines 2196-2540 handle only specific msg types); it falls through to `switch m.activePage` (default → `updateSessionList`), so the burst guard is genuinely the first key handler reached. Both cancel keys use the correct v2 predicates (`keyIsCtrlC` = 'c'+ModCtrl; `keyIsCode(_, tea.KeyEscape)` requires Mod==0). No drift from the plan; the chosen "no right hint" variant is explicitly allowed by the task Do.

TESTS:
- Status: Adequate
- Location: `internal/tui/burst_input_lock_test.go`
- Coverage:
  - `TestBurstInputLock_IgnoresSecondEnter` — second Enter swallowed: nil cmd, `adapter.Calls == 0`, pipe unchanged, still pending. (Robust: a dispatch would synchronously create a new pipe and return a non-nil receiver cmd, both asserted — not reliant on the async goroutine having run.)
  - `TestBurstInputLock_IgnoresRowActions` — table over m / down / up / Space / slash / s; asserts inert page, grouping mode, marked count, cursor index, filter state, and still-pending. (The `k`/`x`/`r` keys named in the task are covered by the same catch-all `return m, nil` — representative subset is sufficient.)
  - `TestBurstInputLock_CtrlCAndEscStayLive` — Ctrl-C does not quit and Esc does not exit multi-select; both stay pending (routed to cancelBurst).
  - `TestBurstInputLock_AdvancesOpeningCounter` — each `spawnProgressMsg{Done,Total:2}` advances BurstDone and the rendered band (Opening 1/3…, Opening 2/3…) while N=3 holds.
  - `TestBurstInputLock_HoldsDenominatorAtN` — BurstTotal held at 3 across progress; band never uses `/2`, never reaches `3/3`.
  - `TestBurstInputLock_OpeningBandPrecedence` — outranks multi-select banner, unsupported banner + standard header; steps aside only for the live filter input.
  - `TestOpeningBand_RendersVioletCounter` — pins the render contract (violet run, exactly 1 row, full content width, canvas-painted spacer) in Dark + Light.
  - `TestOpeningBand_ColourlessDropsHueAndCanvas` — NO_COLOR: text survives, no canvas SGR, no violet fg SGR.
- Notes: Maps 1:1 to the plan's 7 named tests plus one extra render-contract test — no over-testing, no redundant assertions. Tests would fail if the feature broke (a missing guard would move the cursor / dispatch a burst; a `burstTotal` overwrite would surface as `/2`). White-box package `tui`, no `t.Parallel()` — matches project convention. The `burstPendingModel` helper force-sets `burstPending` without a real pipe (documented); valid because the guard logic is pipe-independent.

CODE QUALITY:
- Project conventions: Followed. Token-only render (AccentViolet via `headerStyle`, no raw hex — passes the colour-literal guard); shared right-anchor core reused (no geometry drift); no `t.Parallel()`; nil-tolerant seam discipline preserved.
- SOLID principles: Good. `renderOpeningBand` is a single-responsibility pure render fn; the guard is a focused early-return; the progress fold is minimal.
- Complexity: Low. Guard is a 4-line branch; progress arm is 3 statements; band is a 2-line composition.
- Modern idioms: Yes. `fmt.Sprintf` + lipgloss composition; v2 key predicates.
- Readability: Good. Each site carries a precise §-referenced comment explaining ordering and the deliberate `burstTotal` non-overwrite.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/notice_band.go:361 (`activeNoticeBand` flash arm) — The spec's single-slot precedence places `Opening n/N…` ABOVE the transient flash, but the two live on separate physical rows (Opening = section-header claimant; flash = notice band), so they do not actually arbitrate. In the normal N≥2 Enter path this is inert (the dispatching Enter clears any flash via the flash-clear before dispatch). It is only reachable in the narrow detection-defer window: a deferred Enter (detection in-flight) leaves `burstPending` false, so the picker is not yet input-locked; a flash set by a keystroke in that ~tens-of-ms window then persists when `terminalDetectedMsg → dispatchBurst` flips `burstPending`, causing the flash to co-render beneath the Opening band. Consider gating the flash arm on `!m.burstPending` (or clearing the flash at `dispatchBurst`) so the Opening band's precedence over the transient flash holds on that path too. Decide whether it is worth addressing given the near-zero probability and that it straddles the 6-3/6-5 boundary — flagged for awareness, not a defect in 6-5's own scope.
