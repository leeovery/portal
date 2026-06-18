---
phase: 5
phase_name: Cold-path startup flip (concurrent bootstrap + honest loading screen)
total: 8
---

## spectrum-tui-design-5-1 | approved

### Task spectrum-tui-design-5-1: Cold-vs-warm gating — scope the concurrent flip to the cold + TUI path only

**Problem**: Today the loading page is shown whenever `serverStarted` is true (set by step 1 `EnsureServer` when it actually had to start the tmux server), but the whole 11-step bootstrap still runs *synchronously* in `PersistentPreRunE` before the TUI launches (§10.2 "Today"). Phase 5 turns the cold path concurrent, which carries genuine startup-path risk (the slow-open / zombie-session prior incident, §10). That risk must be quarantined: only the **cold + TUI** path may take the new concurrent route; the warm path and the CLI/direct-path must keep today's exact synchronous behaviour with **zero new risk** (§10.1). This task establishes the gate that decides which path a given launch takes — the foundation every other Phase 5 task builds on.

**Solution**: Introduce a single, cheap, well-defined cold-vs-warm + TUI-vs-CLI decision that selects between two delivery routes: (a) today's synchronous bootstrap (warm, OR cold-but-CLI/direct-path) and (b) the new concurrent route (cold + TUI). The decider is the existing `serverStarted` signal from `EnsureServer` (true only when Portal had to start the server — the cold boot, §10.1) combined with the existing `isTUIPath(cmd, args)` (true only for `portal open` with zero args — §10.2). A cheap `tmux has-server` check (`tmux.Client.ServerRunning`, which runs `tmux info`) is the cold/warm decider where one is needed before `EnsureServer` runs. This task only wires the **decision** and the routing seam; tasks 5-2…5-7 fill in the concurrent route behind it. No behaviour changes on any path yet — the concurrent route initially routes to today's synchronous code so the gate can be landed and tested in isolation.

**Outcome**: A launch is classified as cold+TUI exactly when `serverStarted == true` (server was just started by `EnsureServer`) **and** `isTUIPath(cmd, args) == true`; every other launch (warm, or cold-but-CLI/direct-path) is classified as "synchronous-untouched". The loading page is gated on `serverStarted` (warm → `serverStarted=false` → no loading page → straight to picker, as today). The gate is verifiable independently of the concurrent machinery.

**Do**:
- In `cmd/root.go` `PersistentPreRunE` and/or `cmd/open.go` `openTUI`, introduce the cold+TUI classification: cold is `serverStarted` (the value already returned by `runBootstrap` and threaded via `serverStartedKey` in `cmd/bootstrap_context.go`); TUI is `isTUIPath(cmd, args)` (already defined in `cmd/root.go`, lines 205-207).
- Where a cold/warm decision is needed *before* `EnsureServer` would run (i.e. to decide whether to take the concurrent route at all), use `tmux.Client.ServerRunning()` (`internal/tmux/tmux.go:121`, runs `tmux info`) as the cheap `tmux has-server` probe. Note that `EnsureServer`'s own return already distinguishes started-vs-already-running, so the probe is only needed if the route decision must precede the orchestrator call; document which one you use and why in a comment.
- Add a single routing seam (e.g. an unexported helper `shouldRunConcurrentBootstrap(cmd, args) bool` in `cmd/`) that returns true only for cold+TUI. Keep `serverWasStarted(cmd)` and the existing `serverStartedKey` context delivery intact for now — task 5-2 replaces the delivery mechanism for the concurrent route only.
- Leave the concurrent branch as a stub that calls into today's synchronous path (so this task is pure routing, no concurrency yet). Add the production wiring point that tasks 5-2…5-7 will extend.
- Confirm `WithServerStarted` (`internal/tui/model.go:556`) still gates `PageLoading` on `serverStarted`; the warm path (`serverStarted=false`) must never land on `PageLoading`.

**Acceptance Criteria**:
- [ ] Cold + TUI (`serverStarted=true` AND `isTUIPath`) is classified for the concurrent route; every other combination routes to today's synchronous path
- [ ] Warm (`serverStarted=false`) shows no loading page and reaches the picker via today's path — byte-for-byte behaviour parity with pre-Phase-5
- [ ] CLI/direct-path (`!isTUIPath`) keeps the synchronous bootstrap even when cold (`serverStarted=true`)
- [ ] The `tmux has-server` decider (where used) is `ServerRunning`/`tmux info`, a single cheap call; no extra tmux round-trips added to the warm path
- [ ] This is non-visual plumbing — explicitly `vhs`-exempt; verification is behavioural only

**Tests**:
- `"it classifies cold + TUI (serverStarted=true, isTUIPath=true) for the concurrent route"`
- `"it classifies warm (serverStarted=false) for the synchronous path and shows no loading page"`
- `"it classifies cold + CLI/direct-path (serverStarted=true, isTUIPath=false) for the synchronous path"`
- `"it leaves the warm path's tmux call count and PersistentPreRunE behaviour byte-for-byte unchanged"`
- `"it gates PageLoading on serverStarted (warm never lands on PageLoading)"`

**Edge Cases**:
- Warm path must add zero new tmux round-trips and zero new risk (§10.1) — assert call counts via the injected `bootstrapDeps`/mock client where possible.
- `isTUIPath` is true only for `open` with zero positional args (`open <path>` resolves via `openPath` and is NOT TUI) — the gate must mirror this exactly (cmd/root.go:198-207).
- A cold boot via direct-path (`portal open ~/some/dir`) starts the server but is `!isTUIPath` → synchronous route; verify it does not accidentally take the concurrent path.

**Context**:
> §10.1: "The loading page is gated on `serverStarted` (set only when `EnsureServer` actually had to start the tmux server)." Cold boot → loading page shown; warm → `serverStarted=false` → bootstrap steps no-op → straight to the picker, no loading page — "The common case — instant and **untouched**." §10.1: "The flip is scoped to the COLD path only. A cheap `tmux has-server` check decides; warm keeps today's fast synchronous path, carrying **zero new risk**." §10.2: scoped via the existing `isTUIPath`; CLI/direct-path keeps the synchronous bootstrap.
>
> Existing code: `cmd/root.go` `PersistentPreRunE` runs `runBootstrap` synchronously then threads `started` into context via `serverStartedKey`; `serverWasStarted(cmd)` reads it back in `cmd/open.go` `openTUI` → `tuiConfig.serverStarted` → `WithServerStarted` → `m.activePage = PageLoading`. `isTUIPath` is at `cmd/root.go:205`. `tmux.Client.ServerRunning()` at `internal/tmux/tmux.go:121`. `EnsureServer` at `internal/tmux/tmux.go:300` returns (started bool, err).

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §10.1, §10.2, §14.7

## spectrum-tui-design-5-2 | approved

### Task spectrum-tui-design-5-2: Progress channel + goroutine orchestrator wrapper (cold/TUI path)

**Problem**: On the cold + TUI path the 11-step bootstrap currently runs synchronously in `PersistentPreRunE` *before* Bubble Tea launches, so the loading page renders only after restore is already 100% done — a cosmetic 1.2s pad over a frozen terminal during a slow boot (§10.2 "Today"). To make the loading screen honest and eliminate the frozen-terminal failure, Bubble Tea must launch **immediately** on the loading page and the orchestrator must run **concurrently in a goroutine**, streaming live per-step progress to the TUI. Today's delivery of `serverStarted` (a `context` value) and warnings (a package-level sink) is built for the synchronous model and cannot carry live progress.

**Solution**: For the cold/TUI path only, run `Orchestrator.Run(ctx)` in a goroutine and stream progress to the TUI over a **progress channel** that carries `serverStarted` plus one event per real bootstrap step (and, via task 5-3, per restored session). The TUI ingests channel events as `tea.Msg`s (a `tea.Cmd` that blocks on a channel receive and re-issues itself, the standard Bubble Tea external-channel pattern), updating the loading screen, and transitions to Sessions when a terminal "complete" event arrives. The channel replaces today's `context` + package-memo delivery **on the cold/TUI path only** — the synchronous warm/CLI path keeps `serverStartedKey` context delivery and the `sync.Once` memo untouched (§10.2). The orchestrator gains a progress-emitter seam (a callback or channel-send the steps invoke) wired only on the concurrent route; on the synchronous route the emitter is nil and the orchestrator behaves exactly as today.

**Outcome**: On cold + TUI, `tea.NewProgram` runs from frame one on the loading page while `Orchestrator.Run` executes in a goroutine; a `tea.Msg` is delivered per real step in step order; the model receives `serverStarted` over the channel; a terminal "bootstrap complete" event (or fatal, task 5-6) drives the transition to Sessions; the channel is drained and closed on completion with no goroutine leak. The warm/CLI path is byte-for-byte unchanged.

**Do**:
- Define a progress channel type and event shape in `cmd/` (e.g. `type bootstrapProgress struct { ... }` carrying step index/name, the friendly-label group (task 5-4), the restore N/M counter (task 5-3), and a terminal `done`/`fatal` marker; plus the `serverStarted` flag delivered once). Keep the channel buffered enough that a fast orchestrator never blocks on a slow render, but bounded.
- Add a progress-emitter seam to the orchestrator path. The `Orchestrator.Run(ctx)` signature already accepts a `ctx` that is currently unused (`cmd/bootstrap/bootstrap.go:260` `_ = ctx`) — wire the emitter either through the context, an added field on `Orchestrator`, or a wrapper that observes step boundaries; pick the lowest-risk option that keeps the synchronous path's emitter nil. Document the choice. Each of the 11 steps emits its progress event at the same site it logs `"step complete"`.
- In `cmd/open.go` `openTUI` (cold/TUI branch), spawn the orchestrator in a goroutine, create the channel, and pass a receiver-side `tea.Cmd` into the model that blocks on the channel and emits a `tea.Msg` per event. Add a new model message type for streamed progress (e.g. `BootstrapProgressMsg`) and an Update arm; the existing `BootstrapCompleteMsg` becomes the terminal event (now sent by the channel, not synthesised in `Init`).
- Replace, on the cold/TUI path only, today's `serverStartedKey` context delivery (`cmd/bootstrap_context.go`) and the synthetic `bootstrapCompleteCmd` emitted from `Init` (`internal/tui/model.go:1318`). The synchronous path keeps both unchanged.
- Ensure the orchestrator goroutine closes the channel on return (success or fatal) and the receiver `tea.Cmd` stops re-issuing on a closed channel — no goroutine leak, no blocked receive after Quit.
- Preserve the `TUI is inert during loading` property (§10.2): the model must not enumerate sessions or accept page navigation until the terminal complete event — this is what contains the race surface.

**Acceptance Criteria**:
- [ ] On cold + TUI, Bubble Tea launches on the loading page before the orchestrator finishes; the orchestrator runs in a goroutine
- [ ] One `tea.Msg` is delivered per real bootstrap step in step order; `serverStarted` is carried over the channel
- [ ] A terminal "complete" event drives the transition to Sessions; Sessions enumeration is still gated on that event (TUI inert during loading)
- [ ] The channel is drained and closed on completion; no goroutine leak and no blocked receive after the program quits
- [ ] The warm/CLI synchronous path keeps `serverStartedKey` context delivery and the `sync.Once` memo unchanged (byte-for-byte parity)
- [ ] Non-visual plumbing — explicitly `vhs`-exempt; verification is behavioural

**Tests**:
- `"it launches the TUI on the loading page and runs the orchestrator in a goroutine on cold+TUI"`
- `"it streams one progress msg per real step in step order"`
- `"it carries serverStarted over the progress channel on the cold/TUI path"`
- `"it transitions to Sessions on the terminal complete event and not before"`
- `"it closes the channel and leaves no goroutine running after Quit"`
- `"it leaves the warm/CLI synchronous serverStarted+memo delivery byte-for-byte unchanged"`
- `"it keeps the TUI inert during loading (no session enumeration / no page nav before complete)"`

**Edge Cases**:
- Channel drain/close on complete without goroutine leak (receiver must detect a closed channel and stop re-issuing the receive `tea.Cmd`).
- A very fast cold boot (M=0 restore, task 5-4) still emits its per-step events then the terminal event — the channel handles a burst that arrives before the first render.
- The package-memo `sync.Once` warm path must be unaffected: the concurrent route bypasses the memo, but the memo's package state must not be left in a half-set state that perturbs a subsequent warm invocation in the same process (tests run multiple `Execute()` in one process via `bootstrapDeps`).
- Msg-per-real-step ordering must be exact even though Bubble Tea batches commands — the receiver pattern (single blocking receive re-issued) preserves order; assert it.

**Context**:
> §10.2: for the cold + TUI path only, "launch Bubble Tea **immediately** on the loading page, run the orchestrator in a **goroutine**, stream a `tea.Msg` per real step (and per restored session), transition to Sessions on complete, **quit-with-error** on the one fatal step." "A progress channel carries `serverStarted` + per-step progress to the TUI on the cold/TUI path, replacing today's `context` + package-memo delivery." "The loading page already gates Sessions enumeration on `BootstrapCompleteMsg`, and the TUI is **inert during loading** (animation only) — this **contains the race surface**."
>
> Existing code: `cmd/bootstrap/bootstrap.go` `Orchestrator.Run(ctx)` — `ctx` is currently `_ = ctx // reserved`; each step emits `"step complete"` at a fixed site. `cmd/open.go` `openTUI` calls `tea.NewProgram(m, tea.WithAltScreen())` after a fully-synchronous bootstrap. `internal/tui/model.go:1305-1323` `Init` synthesises `BootstrapCompleteMsg` from the first tick (this synthesis is what the channel replaces on the cold/TUI path). `BootstrapCompleteMsg` Update arm at model.go:1373; `LoadingMinElapsedMsg` at model.go:1366. `serverWasStarted`/`serverStartedKey` in `cmd/bootstrap_context.go` — the context delivery this task replaces on the concurrent route.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §10.2

## spectrum-tui-design-5-3 | approved

### Task spectrum-tui-design-5-3: Restore per-session progress callback (the N/M source)

**Problem**: The honest loading screen needs a real per-item progress source for the `Restoring sessions (N/M)` label (§10.4) — the **only** label that carries an N/M counter, because the restore per-session loop is the one real per-item progress source in the whole bootstrap. Today `internal/restore/restore.go`'s per-session loop has no progress hook, so the loading screen has nothing live to count against. The callback must be additive and nil-tolerant so the synchronous warm/CLI path (which passes no callback) is completely unaffected.

**Solution**: Inject an optional per-session progress callback into the restore per-session loop. `M = len(idx.Sessions)` (the total to consider) and `N` advances on each loop iteration; the callback is invoked once per session with `(N, M)` so the cold/TUI progress channel (task 5-2) can render `Restoring sessions (N/M)`. The callback is a nil-tolerant field on `restore.Orchestrator` (or threaded through its existing seam): when nil (the synchronous path), the loop behaves exactly as today. `M=0` (first run / nothing saved) fires zero callbacks — the loop body never runs — which task 5-4 maps to "tick ✓ immediately, suppress (N/M)".

**Outcome**: On the cold/TUI path the restore loop fires the callback per restored session with a monotonically advancing `N` against a fixed `M = len(idx.Sessions)`; live-skips (already-live sessions, underscore-prefixed, invalid topology) still advance `N` against `M` so the counter reaches `M/M` even when some sessions are skipped; `M=0` fires zero callbacks; the synchronous path (nil callback) is byte-for-byte unchanged; per-session restore behaviour is otherwise identical (purely additive instrumentation).

**Do**:
- Add an optional progress callback field to `restore.Orchestrator` (e.g. `Progress func(n, m int)`), defaulting nil. Keep `Restore() (bool, error)` signature unchanged so the `cmd/bootstrap.Restorer` contract and the `bootstrapadapter.RestoreAdapter` wiring are untouched.
- In `Restore()` (`internal/restore/restore.go:45`), compute `m := len(idx.Sessions)` (already available; the early-return `len(idx.Sessions) == 0` path is `M=0`). In the `for i, sess := range idx.Sessions` loop (currently `for _, sess`), invoke `o.Progress(i+1, m)` once per iteration when `Progress != nil`. Decide and document whether the callback fires before or after `restoreOne` — fire it so that `N` advances even on a skip (live-skip / underscore / invalid topology all still tick N against M, so the counter completes); the spec wants the counter to reach M even when sessions are live-skipped.
- Wire the callback only in the cold/TUI production path (the goroutine route from task 5-2): the `restore.Orchestrator` built in `buildProductionOrchestrator` (`cmd/bootstrap_production.go:158`) gets `Progress` set to a closure that forwards onto the progress channel. The synchronous-route production orchestrator leaves `Progress` nil.
- Do NOT change `restoreOne`, `SessionRestorer`, geometry/scrollback replay, or any per-session restore logic — this task is additive instrumentation only.

**Acceptance Criteria**:
- [ ] The callback fires once per session in the loop with `N` advancing 1..M against a fixed `M = len(idx.Sessions)`
- [ ] Live-skipped sessions (already live, underscore-prefixed, invalid topology) still advance `N` so the counter reaches `M/M`
- [ ] `M=0` (early return) fires zero callbacks
- [ ] A nil callback (synchronous warm/CLI path) leaves `Restore` behaviour byte-for-byte unchanged — same return values, same logs, same per-session sequence
- [ ] `Restore() (bool, error)` signature and the `Restorer` contract are unchanged (additive field only)
- [ ] Non-visual plumbing — explicitly `vhs`-exempt; verification is behavioural

**Tests**:
- `"it fires the callback once per session with N advancing against fixed M"`
- `"it advances N on live-skipped sessions so the counter reaches M/M"`
- `"it fires zero callbacks when M=0 (nothing saved)"`
- `"it leaves Restore behaviour byte-for-byte unchanged when the callback is nil"`
- `"it does not change per-session restore outcomes (additive instrumentation only)"`

**Edge Cases**:
- `M=0` (first run / empty `sessions.json` / corrupt-skip): the `len(idx.Sessions) == 0` early return and the corrupt/skip paths fire zero callbacks — the counter never appears (task 5-4 suppresses `(N/M)` and ticks ✓).
- Live-skips, underscore-prefixed names, and invalid-topology sessions must still advance N (they are part of M); the callback fires regardless of whether `restoreOne` returned true.
- Nil-tolerant: the synchronous warm/CLI path passes no callback — the loop must guard `if o.Progress != nil`.
- A per-session restore failure (logged + swallowed inside `restoreOne`) must still tick N — a failure isolates locally and does not stall the counter.

**Context**:
> §10.2: "A progress callback is injected at the restore per-session loop." §10.4: `Restoring sessions (N/M)` maps to "6 Restore — skeleton phase (the per-session loop; `N/M` is its real counter)"; "Only `Restoring sessions` carries an `N/M` counter (the restore loop is the one real per-item progress source); other labels tick once." "Empty restore (M=0 …): the `Restoring sessions` label **suppresses the `(N/M)` counter** … and ticks `✓` immediately."
>
> Existing code: `internal/restore/restore.go:45` `Restore()`; `len(idx.Sessions)` is M (line 51 early-returns when 0); the loop `for _, sess := range idx.Sessions` is at line 72; `restoreOne` (line 139) returns a bool used only for the cycle-summary tally. The `restore.Orchestrator` is wired in `cmd/bootstrap_production.go:158` and adapted via `bootstrapadapter.RestoreAdapter{Inner: restoreInner}`.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §10.2, §10.4

## spectrum-tui-design-5-4 | approved

### Task spectrum-tui-design-5-4: Step mapping — 11 real bootstrap steps → 5 friendly labels

**Problem**: The honest loading screen must show progress the user can read, but the bootstrap has 11 internal steps with cryptic names (`EnsureServer`, `RegisterPortalHooks`, `SweepOrphanDaemons`, …). §10.4 mandates collapsing the 11 real steps into 5 friendly labels, advancing the progress bar on **every** real step while the active label is the friendly group the current step falls in. This mapping is the contract between the streamed per-step progress (task 5-2) and the rendered tick-list (task 5-5); it must be a single source of truth so the bar advance, the active-label selection, and the tick-off behaviour stay consistent — including the M=0 empty-restore degenerate case.

**Solution**: Define the canonical 11→5 step-mapping as a pure, testable function/table in the TUI (or a shared package consumed by the loading-screen render). It maps each streamed real-step event to (a) one of the 5 friendly labels and (b) a bar-advance increment, and tracks which labels are done / active / pending. Only `Restoring sessions` carries the `N/M` counter (sourced from task 5-3). When a label's steps complete with zero items (M=0 restore, or `Running resume commands` with no on-resume hooks), the label ticks ✓ — "done, not stalled" — and the bar still advances through every real step.

**Outcome**: Each of the 11 streamed step events advances the bar and resolves to its friendly label per the §10.4 table; the active label is the group of the current step; completed labels show `✓`, the current label `◐`, future labels `·`; only `Restoring sessions` renders `(N/M)`; M=0 suppresses `(N/M)` and ticks `Restoring sessions` ✓ immediately; `Running resume commands` ticks ✓ with no per-item work; the bar reaches 100% only after the last real step. The mapping is a pure function, independently unit-testable.

**Do**:
- Encode the §10.4 table as a single source of truth (a slice/map keyed by the closed `step*` StepName constants from `cmd/bootstrap/bootstrap.go:67-79`, or by step index 1..11). The 5 labels and their member steps:
  - `Started tmux server` → step 1 `EnsureServer`
  - `Registered hooks` → steps 2 `RegisterPortalHooks` · 3 `SetRestoring` · 4 `SweepOrphanDaemons` · 5 `EnsureSaver`
  - `Restoring sessions (N/M)` → step 6 Restore skeleton phase (the per-session loop; N/M from task 5-3)
  - `Replaying scrollback` → step 6 Restore geometry + scrollback replay · 7 `EagerSignalHydrate`
  - `Running resume commands` → hydrate helpers firing on-resume commands · 8 `ClearRestoring` · 9 `CleanStaleMarkers` · 10 `SweepOrphanFIFOs` · 11 `CleanStale`
- Implement a pure function `mapStep(event) (label, barIncrement, labelStates)` (exact shape per task 5-5's render needs) that, given the stream of step events, produces the current bar fraction and the per-label ✓/◐/· states. The bar advances on every real step (so 11 increments, not 5).
- Handle the N/M counter: `Restoring sessions` shows `(N/M)` driven by the restore callback events (task 5-3); when `M=0` (no restore callbacks, the restore step completes immediately) suppress `(N/M)` and mark the label done.
- Handle `Running resume commands` with zero per-item work: it ticks ✓ once its constituent steps complete — there is no per-item counter there; "a label whose steps completed with zero items is done, not stalled" (§10.4).
- Note in a comment that §10.4 permits implementation to adjust which fast cleanup step (8–11) sits under which label, but the bar must advance through every real step.
- Keep this mapping decoupled from the channel transport (task 5-2) and from the render (task 5-5) so it is unit-testable in isolation.

**Acceptance Criteria**:
- [ ] All 11 real steps map to exactly one of the 5 friendly labels per the §10.4 table
- [ ] The bar advances on every real step (11 increments), reaching 100% only after step 11
- [ ] The active label is the friendly group of the current step; completed labels are ✓, current ◐, future ·
- [ ] Only `Restoring sessions` carries an `(N/M)` counter
- [ ] M=0 suppresses `(N/M)` and ticks `Restoring sessions` ✓ immediately without stalling
- [ ] `Running resume commands` ticks ✓ with no per-item work; cleanup steps 8–11 fold under it
- [ ] Non-visual plumbing/contract — explicitly `vhs`-exempt (its render is task 5-5); verification is behavioural

**Tests**:
- `"it maps each of the 11 real steps to its §10.4 friendly label"`
- `"it advances the bar on every real step and reaches 100% only after step 11"`
- `"it marks completed labels ✓, the current label ◐, and future labels ·"`
- `"it renders (N/M) only on Restoring sessions and ticks it from the restore callback"`
- `"it suppresses (N/M) and ticks Restoring sessions ✓ immediately when M=0"`
- `"it ticks Running resume commands ✓ with no per-item work and folds cleanup steps 8–11 under it"`

**Edge Cases**:
- M=0 empty restore: the `Restoring sessions` label must tick ✓ immediately and render no `(N/M)`; the bar still advances through step 6 and onward — verify it is "done, not stalled".
- `Running resume commands` with zero registered on-resume hooks: ticks ✓ with no counter, no stall.
- A label spanning multiple steps (`Registered hooks`, `Running resume commands`) stays `◐` until its last constituent step completes; the bar advances within the label across its steps.
- Out-of-order or duplicate step events should not double-advance the bar (defensive — the channel is in-order per task 5-2, but the mapping should be idempotent per step index).

**Context**:
> §10.4 step-mapping table (verbatim): `Started tmux server` [1 EnsureServer]; `Registered hooks` [2 RegisterPortalHooks · 3 set @portal-restoring · 4 SweepOrphanDaemons · 5 EnsureSaver]; `Restoring sessions (N/M)` [6 Restore skeleton — the per-session loop, N/M is its real counter]; `Replaying scrollback` [6 Restore geometry+scrollback replay · 7 EagerSignalHydrate]; `Running resume commands` [hydrate helpers firing on-resume commands · 8 clear @portal-restoring · 9–11 cleanup]. "The bar advances on **every real bootstrap step**; the **active label** is the friendly group the current step falls in." "Empty restore (M=0 …): the `Restoring sessions` label suppresses the `(N/M)` counter and ticks ✓ immediately; `Running resume commands` likewise ticks ✓ with no per-item work. The bar still advances through every real step — a label whose steps completed with zero items is 'done,' not stalled."
>
> Existing code: the closed StepName set `stepEnsureServer` … `stepCleanStale` at `cmd/bootstrap/bootstrap.go:67-79`; `totalSteps = 11` at line 59. The restore N/M source is task 5-3's callback.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §10.4

## spectrum-tui-design-5-5 | approved

### Task spectrum-tui-design-5-5: Honest loading-screen render (VISUAL) — `PORTAL ▌` + thick violet bar + ticking step-list

**Problem**: Today's `viewLoading` (`internal/tui/model.go:2232`) is a single centred `"Restoring sessions…"` string placed with `lipgloss.Place` — a cosmetic placeholder, not the honest determinate screen the redesign requires. The loading screen is the **one visual surface** of Phase 5 (§10.3) and must match the `Loading 6 — Combined (thick bar)` Paper frame (§15.1): centred `PORTAL ▌` wordmark over a thick block progress bar and a real ticking step-list that reflects live bootstrap progress (from tasks 5-2/5-4), all painted on the correct owned canvas from frame one (§2.6/§10.2).

**Solution**: Restyle `viewLoading` to render, centred on the owned canvas: (1) the `PORTAL ▌` wordmark — `PORTAL` in `text.primary` (the Phase 2 wordmark treatment) + the block caret `▌` in `accent.violet`; (2) a **thick** block progress bar (filled `accent.violet`, track `bg.track`) driven by the task-5-4 bar fraction; (3) a **real** step-list (one row per friendly label, not an in-place text swap) where each label shows `✓` done (glyph `state.green`, label `text.muted-bright`), `◐` active (glyph `accent.cyan`, label `text.primary`), or `·` pending (glyph `text.faint`, label `text.dim`), with `Restoring sessions (N/M)` showing the live counter. Consume the step-mapped progress stream (task 5-4) for live state. The first real paint gates on the Phase 1 task 1-7 detect-or-timeout gate so the correct canvas paints from frame one (no flip). Honour the `LoadingMinDuration` / `LoadingMinElapsedMsg` 1.2s min-display pad in concert with the now-honest progress. Warm path shows no loading screen at all (gated by task 5-1).

**Outcome**: On cold + TUI, the loading screen renders `PORTAL ▌` over a thick violet bar on `bg.track` with a ticking step-list, advancing live as bootstrap progresses, on the correct owned canvas from frame one; the `vhs` capture matches `Loading 6 — Combined (thick bar)` for layout/structure/colour-role; the warm path is untouched (no loading screen — behaviour parity); `NO_COLOR` renders colourless on the terminal's native bg; narrow/short terminals degrade rather than overflow.

**Do**:
- Replace the body of `viewLoading` (`internal/tui/model.go:2232-2243`) with the MV composition (`lipgloss.JoinVertical` of wordmark / bar / step-list, centred via `lipgloss.Place` on the owned canvas — last-layer full-terminal fill per Phase 1 task 1-6). Reuse the Phase 2 `PORTAL ▌` wordmark token treatment (do not re-derive it) and Phase 1 tokens (`text.primary`, `accent.violet`, `bg.track`, `state.green`, `accent.cyan`, `text.faint`, `text.muted-bright`, `text.dim`).
- Render the thick block progress bar from the task-5-4 bar fraction (filled `accent.violet`, empty track `bg.track`); the bar weight is "thick" (decided, §10.3).
- Render the step-list as a real list of rows (one per friendly label), each with its glyph + label coloured by ✓/◐/· state from task 5-4; show `(N/M)` only on `Restoring sessions` (suppress when M=0 per task 5-4).
- Store the live progress state on the model (fed by `BootstrapProgressMsg` from task 5-2) and read it in `viewLoading`; the model is otherwise inert during loading (animation/progress only).
- Gate the first real paint on the Phase 1 task 1-7 detect-or-timeout first-paint gate so the canvas is correct from frame one — no paint-then-flip (§2.6/§10.2). The tens-of-ms detection is invisible against the multi-hundred-ms bootstrap.
- Reconcile `LoadingMinDuration` with honest progress: the page stays until BOTH the terminal complete event AND `LoadingMinElapsedMsg` (1.2s) — keep the existing dual-gate in `transitionFromLoading` / the `LoadingMinElapsedMsg` + `BootstrapCompleteMsg` arms (model.go:1366-1385). With honest progress the bar/ticks reflect real work; the 1.2s pad still applies when the boot is faster than 1.2s.
- Narrow/short degrade: drop the wordmark to compact / let height drive layout so the screen never overflows (§2.7); `NO_COLOR` suppresses the canvas and renders colourless on native bg (Phase 1 task 1-8 carve-out, §2.5).
- VERIFICATION (mandatory per the spec's top verification mandate, §15.4): add/extend a `vhs` tape (harness from Phase 1 task 1-1, `testdata/vhs/`) that drives the TUI to the loading state from seeded fixture state and writes `loading.png`; compare it against the committed `Loading 6 — Combined (thick bar)` Paper reference export (§15.1/§15.5) for layout/structure/colour-role (agent/user-judged, not pixel-diff). Confirm behaviour parity: the warm path shows no loading screen.

**Acceptance Criteria**:
- [ ] `viewLoading` renders centred `PORTAL ▌` (`text.primary` + `accent.violet` caret) over a thick block bar (filled `accent.violet`, track `bg.track`) and a real ticking step-list
- [ ] Step-list rows tick ✓ (glyph `state.green` / label `text.muted-bright`), ◐ (glyph `accent.cyan` / label `text.primary`), · (glyph `text.faint` / label `text.dim`) per task-5-4 live state; only `Restoring sessions` shows `(N/M)`
- [ ] The first real paint gates on the Phase 1 task 1-7 detect-or-timeout gate — correct canvas from frame one, no flip
- [ ] It is a real list (rows), not an in-place text swap
- [ ] `LoadingMinDuration` (1.2s) min-display pad still applies, dual-gated with the terminal complete event
- [ ] Narrow/short terminals degrade (compact wordmark, height-driven) and never overflow; `NO_COLOR` renders colourless on native bg
- [ ] Warm path shows no loading screen (behaviour parity)
- [ ] VISUAL: a `vhs` tape drives the TUI to the loading state and writes a PNG; it is compared against `Loading 6 — Combined (thick bar)` (§15.1) for layout/structure/colour-role; behaviour parity (warm path untouched) is confirmed

**Tests**:
- `"it renders PORTAL ▌ over a thick violet bar on bg.track with a step-list"`
- `"it ticks step rows ✓/◐/· with the spec'd glyph and label tokens from live progress"`
- `"it renders (N/M) only on Restoring sessions and suppresses it when M=0"`
- `"it paints the correct canvas from frame one (gated on detect-or-timeout, no flip)"`
- `"it keeps the 1.2s min-display pad dual-gated with the terminal complete event"`
- `"it degrades on a narrow/short terminal without overflow"`
- `"it renders colourless on native bg under NO_COLOR"`
- `"it shows no loading screen on the warm path"`
- `"vhs: the loading capture matches Loading 6 — Combined (thick bar) for layout/structure/colour-role"` (manual/agent-judged compare per §15.5)

**Edge Cases**:
- Detect-or-timeout: never paint-then-flip — the first frame must already be the resolved (or dark-fallback) canvas (Phase 1 task 1-7).
- Faster-than-1.2s boot: the bar may complete before `LoadingMinElapsedMsg`; the page stays for the pad showing the completed bar / all-✓ list, then dismisses.
- M=0 restore: `Restoring sessions` shows ✓ with no `(N/M)`, the bar still advances (task 5-4).
- Narrow/short terminal: compact wordmark, the step-list must not overflow the viewport; height drives layout (§2.7).
- `NO_COLOR`: no canvas paint, colourless glyphs leaning on ✓/◐/· + bold/dim (state never colour-only, §2.2).

**Context**:
> §10.3: "Centred `PORTAL ▌` (wordmark `text.primary` + caret `accent.violet`) over a **thick block progress bar** (filled `accent.violet`, track `bg.track`) and a **tick-list that ticks off** as each boot step completes — a **real list**, not an in-place text swap: `✓` done — glyph `state.green`, label `text.muted-bright`; `◐` active — glyph `accent.cyan`, label `text.primary`; `·` pending — glyph `text.faint`, label `text.dim`. Bar weight is **thick** (decided). Warm path shows no loading screen." §10.2 canvas-flip avoidance: "the first real paint gates on light/dark detection-or-timeout (§2.6), so the loading page paints the correct canvas from frame one." §2.6: "The cold-path loading page gates the same way — it paints the correct canvas from its first frame; the tens-of-ms detection is invisible against the multi-hundred-ms bootstrap." §15.1 frame: `Loading 6 — Combined (thick bar)`.
>
> Existing code: `internal/tui/model.go:2232` `viewLoading` (the restyle target — currently `lipgloss.Place(w, h, Center, Center, "Restoring sessions…")`). `LoadingMinDuration = 1200ms` at model.go:126; `LoadingMinElapsedMsg`/`BootstrapCompleteMsg` arms at model.go:1366-1385; `transitionFromLoading` at model.go:1269. Phase 1 task 1-6 (owned-canvas outer fill), task 1-7 (detect-or-timeout first-paint gate), task 1-8 (NO_COLOR). Phase 2 task 2-2 (PORTAL ▌ wordmark treatment).

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §10.3, §10.4, §2.6, §10.2, §15.1

## spectrum-tui-design-5-6 | approved

### Task spectrum-tui-design-5-6: Fatal cold-boot error contract — in-TUI error state + fatal-error-as-`tea.Quit` (non-zero exit)

**Problem**: Today a fatal bootstrap step (EnsureServer / RegisterPortalHooks / SetRestoring / ClearRestoring) returns a `*bootstrap.FatalError` from `PersistentPreRunE`, which `Execute` surfaces and `main.go` maps to exit code 1 — all *before* the TUI launches. On the new concurrent cold/TUI path the orchestrator runs in a goroutine *while* Bubble Tea is live on the loading page, so a fatal step can no longer return through `PersistentPreRunE`. §10.5 mandates that a fatal cold-boot step failure becomes an **in-TUI error state on the loading page** (the failed step gets a `state.red` marker + a one-line message) and that `q`/`Esc` quits with a **non-zero exit** — rather than silently dropping into a half-restored picker.

**Solution**: On the cold/TUI path, the orchestrator goroutine sends a terminal **fatal** event over the progress channel (task 5-2) carrying the failed step and its user-facing message (the `*bootstrap.FatalError.UserMessage`). The model renders the loading-page error state: the failed step's row gets a `state.red` marker and a one-line message; the screen stays on the loading page (no transition to Sessions). `q`/`Esc` triggers `tea.Quit`, and `openTUI` returns the fatal error so `main.go` maps it to a non-zero exit (preserving today's `*bootstrap.FatalError` → code 1 classification). Only the four fatal steps abort this way; every best-effort step warns-and-continues (task 5-7) and never produces this error state. The error frame is **mocked at implementation** (§10.5) — there is no §15.1 Paper frame for it.

**Outcome**: A fatal cold-boot step failure shows the in-TUI loading-page error state (failed step `state.red` + one-line message); `q`/`Esc` quits with a non-zero exit; no half-restored picker is ever shown; best-effort step failures never trigger this path (they warn-and-continue); the synchronous warm/CLI path keeps today's `PersistentPreRunE` → `Execute` → `main.classify` fatal-error exit behaviour unchanged.

**Do**:
- Identify the fatal step set against `cmd/bootstrap/bootstrap.go` (CONFIRM against the code): EnsureServer (step 1), RegisterPortalHooks (step 2), SetRestoring (step 3), ClearRestoring (step 8) — these return `o.fatalf(...)` and short-circuit `Run`; all other steps Warn-and-continue. The orchestrator already constructs a `*bootstrap.FatalError` via `o.fatalf` → `NewFatal` (errors.go).
- On the cold/TUI path, when the orchestrator goroutine's `Run` returns a non-nil error (always a `*bootstrap.FatalError`), send a terminal **fatal** progress event over the channel carrying the failed step's friendly label (task 5-4) and the `FatalError.UserMessage`. Do not transition to Sessions.
- Add a model fatal-error state: a field holding the failed step + message; the `viewLoading` render (task 5-5) shows that step's row with a `state.red` marker and the one-line message beneath. The model stops gating on `BootstrapCompleteMsg` (it will never arrive) and instead awaits a quit key.
- Bind `q`/`Esc` on the loading-page error state to `tea.Quit`. After the program returns, `openTUI` must return the fatal error (not nil) so `Execute`/`main.classify` map it to a non-zero exit. Preserve the existing `*bootstrap.FatalError` classification: `main.classify` already maps it to code 1 with no double-print (`Execute` writes `UserMessage` once). Decide whether `openTUI` returns the original `*bootstrap.FatalError` (so the existing single-line stderr + code-1 contract holds) — prefer this for parity.
- Ensure no transition into a half-restored picker: the model must remain on the loading page in the error state, never flip `activePage` to `PageSessions`.
- The synchronous warm/CLI path is untouched: fatal errors there still flow through `PersistentPreRunE` → `Execute` → `main.classify` exactly as today.
- VERIFICATION: the error frame is **mocked at implementation** (§10.5) — produce a mock of the loading-page error frame, drive the TUI to the fatal state via a `vhs` tape (or a fault-injecting fixture), and compare the capture against the implementation mock (NOT a §15.1 Paper frame). State the mock-and-compare explicitly in the verification step.

**Acceptance Criteria**:
- [ ] A fatal cold-boot step failure shows the in-TUI loading-page error state: the failed step's row carries a `state.red` marker + a one-line message
- [ ] The model stays on the loading page (no transition to a half-restored picker)
- [ ] `q`/`Esc` quits with a non-zero exit; `openTUI` returns the fatal error so `main.classify` yields code 1 (single-line stderr, no double-print)
- [ ] Only the four fatal steps (EnsureServer / RegisterPortalHooks / SetRestoring / ClearRestoring) abort this way; best-effort step failures warn-and-continue
- [ ] The synchronous warm/CLI fatal-error exit path is byte-for-byte unchanged
- [ ] VISUAL: the error frame is MOCKED at implementation (not a §15.1 frame); the capture is compared against the mock

**Tests**:
- `"it renders the loading-page error state (state.red marker + one-line message) on a fatal step failure"`
- `"it quits with a non-zero exit on q/Esc in the error state"`
- `"it never transitions to the picker on a fatal step failure"`
- `"it does not enter the error state on a best-effort step failure (warn-and-continue)"`
- `"it returns the *bootstrap.FatalError from openTUI so main.classify yields code 1 without double-printing"`
- `"it leaves the synchronous warm/CLI fatal-error exit path byte-for-byte unchanged"`
- `"mock: the error-frame capture matches the implementation mock"` (mock-and-compare per §10.5)

**Edge Cases**:
- Only the fatal steps abort; a best-effort failure (SweepOrphanDaemons, EnsureSaver, Restore-corrupt, EagerSignalHydrate, CleanStaleMarkers, SweepOrphanFIFOs, CleanStale) must warn-and-continue and never reach this error state — confirm against the Warn-and-swallow sites in `Orchestrator.Run`.
- Non-zero exit must be preserved exactly: the existing `*bootstrap.FatalError` → code 1 mapping in `main.classify` (no stderr double-print) must hold via the returned error.
- A fatal step that aborts mid-sequence (e.g. ClearRestoring at step 8) leaves earlier best-effort steps already done — the error state still shows the failed step and quits non-zero; do not attempt cleanup-into-picker.
- `q` vs `Esc`: both must quit non-zero in the error state (loading-page error state has no other binds active).

**Context**:
> §10.5: "Fatal cold-boot step failure → an in-TUI error state on the loading page: the failed step gets a `state.red` marker + a one-line message; `q`/`Esc` quits with a non-zero exit — rather than dropping into a half-restored picker. The loading-page error frame is mocked at implementation." §10.2 real-costs: "fatal-error-as-`tea.Quit` (today a `PersistentPreRunE` error return)."
>
> Existing code: fatal steps in `cmd/bootstrap/bootstrap.go` return `o.fatalf(...)` and short-circuit `Run` — EnsureServer (line 281), RegisterPortalHooks (line 289), SetRestoring (line 297), ClearRestoring (line 370); all other steps Warn-and-continue. `*bootstrap.FatalError` / `NewFatal` in `cmd/bootstrap/errors.go`. `Execute` (`cmd/root.go:220`) writes `fatal.UserMessage` once to `fatalErrorStderr` then returns the error; `main.classify` (`main.go:83`) maps `*bootstrap.FatalError` → code 1 with no stderr write (Execute already wrote it).

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §10.5, §10.2

## spectrum-tui-design-5-7 | approved

### Task spectrum-tui-design-5-7: Soft warnings ride the progress channel → post-load notice after the picker appears

**Problem**: Soft bootstrap warnings (SaverDownWarning, CorruptSessionsJSONWarning) are today accumulated in the package-level `bootstrapWarnings` sink (`cmd/bootstrap_warnings.go`), drained by `stageBootstrapWarningsOnModel` before `tea.NewProgram`, folded into the synthetic `BootstrapCompleteMsg` in `Init`, and flushed to stderr via an alt-screen toggle on loading-page dismissal (`flushBufferedWarningsCmd`). On the new concurrent cold/TUI path the orchestrator runs in a goroutine and produces warnings *while* the TUI is live, so the pre-launch package-sink staging no longer captures them. §10.5 mandates that soft warnings ride the **progress channel** and surface as a **post-load notice** (after the picker appears), reusing the Phase 4 notice-band primitive — not the stderr alt-screen toggle.

**Solution**: On the cold/TUI path, warnings accumulated by the orchestrator goroutine are carried on the progress channel (task 5-2) — either per-warning events or bundled into the terminal complete event. The model holds them through the loading window and, after transitioning to Sessions (the picker appears), surfaces them as a **post-load notice** using the Phase 4 notice-band primitive (the `▌` left-bar band, under the title separator above the section header, routed through the single-slot arbiter). Zero warnings → no notice. The warm/CLI warning delivery path is unchanged: it keeps the package sink + `EmitTo`-to-stderr (CLI) and the existing staging behaviour where applicable.

**Outcome**: On cold + TUI, soft warnings surface as a post-load notice band after the picker appears (never over the loading page or via an alt-screen toggle during loading); zero warnings produces no notice; the warm/CLI warning delivery path (package sink → stderr for CLI) is byte-for-byte unchanged; the post-load notice reuses the Phase 4 notice-band primitive and obeys the single-slot rule.

**Do**:
- On the cold/TUI path, carry the orchestrator's accumulated `[]bootstrap.Warning` over the progress channel (task 5-2) — bundle them onto the terminal complete event (cleanest) or stream per-warning events; document the choice. This replaces the package-memo staging (`stageBootstrapWarningsOnModel` → `pendingBootstrapWarnings` → synthetic `BootstrapCompleteMsg`) ON THE COLD/TUI PATH ONLY.
- In the model, hold the warnings through the loading window and, on transition to Sessions, surface them as a post-load notice via the Phase 4 notice-band primitive (the `▌` left-bar band routed through the single-slot arbiter; task 4-1). Decide which role variant fits a warning — these are warnings, so the orange/warning treatment is the natural fit; confirm against the notice-band role variants. Replace today's stderr alt-screen flush (`flushBufferedWarningsCmd`) for the cold/TUI path with the in-TUI notice band.
- Zero warnings → no notice band (the single-slot arbiter shows nothing; do not toggle alt-screen).
- Keep the warm/CLI path delivery unchanged: `PersistentPreRunE` still feeds the package sink and `EmitTo`s to stderr for `!isTUIPath` (cmd/root.go:168-173); the warm-TUI path (no loading page) keeps whatever delivery it has today. Only the cold/TUI concurrent route changes.
- Ensure the notice surfaces AFTER the picker appears (post-transition), not over the loading page or during the alt-screen — the notice band lives on the Sessions page chrome.

**Acceptance Criteria**:
- [ ] On cold + TUI, soft warnings surface as a post-load notice band after the picker appears (not over the loading page, not via alt-screen toggle)
- [ ] The post-load notice reuses the Phase 4 notice-band primitive and obeys the single-slot rule
- [ ] Zero warnings produces no notice
- [ ] The warm/CLI warning delivery path (package sink → stderr for CLI) is byte-for-byte unchanged
- [ ] The cold/TUI path no longer relies on the pre-launch package-sink staging / synthetic-BootstrapCompleteMsg fold for warnings
- [ ] Non-visual plumbing (the band render is Phase 4) — explicitly `vhs`-exempt here; verification is behavioural (warnings surface post-load, zero → none, warm path unchanged)

**Tests**:
- `"it surfaces soft warnings as a post-load notice band after the picker appears on cold+TUI"`
- `"it produces no notice when there are zero warnings"`
- `"it routes the post-load notice through the Phase 4 single-slot arbiter / notice-band primitive"`
- `"it does not flush warnings to stderr via an alt-screen toggle on the cold/TUI path"`
- `"it leaves the warm/CLI warning delivery (package sink → stderr) byte-for-byte unchanged"`

**Edge Cases**:
- Zero warnings: no notice, no alt-screen toggle (today's `flushBufferedWarningsCmd` already returns nil for empty — preserve the no-spurious-toggle property in the new path).
- A warning produced by a best-effort step that fails (e.g. SaverDownWarning, CorruptSessionsJSONWarning) must ride the channel and surface post-load — not abort the boot (task 5-6 covers only fatal steps).
- The single-slot rule: if a transient flash or persistent band would also want the slot, the post-load warning notice must coexist per the Phase 4 arbiter's hand-off rules — it is a notice surfaced once after load, not a permanent band; document expected lifetime (auto-clear like a flash, or persists until keypress) and confirm against §11 single-slot semantics.
- Multiple warnings: all surface (the notice band must accommodate or sequence them per Phase 4); confirm ordering matches orchestrator-observation order.

**Context**:
> §10.5: "Soft warnings ride the progress channel and surface as a post-load notice (after the picker appears)." §11 shared convention: notices use a `▌` left-bar accent band under the title separator, above the section header; the single-slot rule holds at most one band. The Phase 4 notice-band primitive (task 4-1) is the reuse target.
>
> Existing code: `cmd/bootstrap_warnings.go` — `BootstrapWarningsSink` (package sink), `Add`/`Drain`/`EmitTo`, `stageBootstrapWarningsOnModel`. `cmd/root.go:168-173` feeds the sink and `EmitTo`s to stderr for `!isTUIPath`. `internal/tui/bootstrap_warnings.go` — `flushBufferedWarningsCmd` (the alt-screen-toggle stderr flush this task replaces on the cold/TUI path); `internal/tui/model.go:1318` synthesises `BootstrapCompleteMsg{Warnings: pending}`; `pendingBootstrapWarnings`/`bufferedWarnings` fields at model.go:208-209. Phase 4 task 4-1 notice-band primitive + single-slot arbiter.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §10.5, §11

## spectrum-tui-design-5-8 | approved

### Task spectrum-tui-design-5-8: Restore/daemon race review + startup-ordering integration-test updates (prior-incident surface)

**Problem**: The concurrent cold-boot flip runs the orchestrator (which performs Restore, EnsureSaver/daemon bootstrap, EagerSignalHydrate, FIFO/marker cleanup) in a goroutine *while* the Bubble Tea event loop is live — exactly the interaction surface behind the prior slow-open / empty-previews / zombie-session incident (§10.2 "prior-incident history"). The spec flags this as the genuine-risk part of Phase 5 (~1–1.5 days with variance). Before this ships, the restore/daemon interaction against the now-live event loop must be deliberately reviewed for race safety, and the existing startup-ordering integration tests must be updated for the new concurrent boot — with warm-path synchronous ordering parity explicitly asserted.

**Solution**: This is the explicit risk-review task. Review the restore/daemon interaction against the live event loop: trace the ordering invariants that the synchronous path guaranteed (e.g. `@portal-restoring` set before saver/restore, cleared before the daemon's first uncontested tick; daemon self-supervision; the `daemon.lock` singleton) and confirm they still hold when the orchestrator runs concurrently with a live TUI that is inert during loading. Update the startup-ordering integration tests to drive the concurrent boot and assert the new ordering, while adding/keeping a warm-path parity assertion (synchronous ordering unchanged). All daemon-spawning tests use `portaltest.IsolateStateForTest(t)` with the env applied to every spawned subprocess; no test uses `t.Parallel()` (the cmd package injects mocks via package-level mutable state). Record the race-review findings (in-code comments or the task's review notes) so the analysis is durable.

**Outcome**: The restore/daemon interaction against the live event loop is reviewed and documented race-safe (or any found race is fixed); the startup-ordering integration tests pass for the concurrent cold boot AND assert warm-path synchronous-ordering parity; every daemon-spawning test is isolated via `IsolateStateForTest` with the env applied to subprocesses and uses no `t.Parallel()`; the known load-flaky `internal/tmux` kill-barrier timing test is re-run in isolation before any timing failure is treated as a regression.

**Do**:
- Review the ordering invariants the synchronous bootstrap guaranteed and confirm they survive the concurrent route. Key invariants to trace against `cmd/bootstrap/bootstrap.go` and the daemon docs: (1) `@portal-restoring` is Set (step 3) before SweepOrphanDaemons/EnsureSaver/Restore and Cleared (step 8) before cleanup — the daemon's `captureAndCommit` suppression window; (2) SweepOrphanDaemons runs before EnsureSaver so the new saver-pane daemon's first tick is uncontested; (3) the `daemon.lock` flock singleton + Component C pre-check; (4) the daemon self-supervision hysteresis. Confirm none of these depend on the TUI being absent — the TUI is inert during loading (no tmux mutation until the picker appears), which is the property that contains the race surface (§10.2). Document the finding.
- Confirm the live event loop never mutates tmux/state while the orchestrator runs: the model is inert during loading (task 5-2) — no session enumeration, no page nav, no attach until the terminal complete event. Assert this in a test.
- Update the startup-ordering integration tests (the cmd integration suite, e.g. `cmd/root_integration_test.go` and any daemon/restore integration tests) to drive the concurrent cold boot and assert: orchestrator step ordering preserved, daemon spawned exactly once (singleton), no zombie/leaked daemon, no slow-open regression.
- Add an explicit warm-path parity assertion: a warm boot (`serverStarted=false`) takes the synchronous path with unchanged ordering and no loading page (ties back to task 5-1).
- Discipline (MANDATORY): every test that runs `portal state daemon` directly OR via `portal open`/bootstrap calls `portaltest.IsolateStateForTest(t)` and applies the returned env to every spawned subprocess (`cmd.Env = env`); prefer `portaltest.SpawnIsolatedDaemon` + `portaltest.RegisterSubprocessCleanup` for the SIGKILL+Wait+reap pattern. No `t.Parallel()` anywhere in the cmd package tests.
- Known flake: the `internal/tmux` kill-barrier timing test is load-flaky under full `go test ./...`; re-run it in isolation before treating a timing failure as a regression (reference: the project flaky-killbarrier note).

**Acceptance Criteria**:
- [ ] The restore/daemon interaction against the live event loop is reviewed and the analysis recorded (race-safe, or any found race fixed)
- [ ] The ordering invariants (`@portal-restoring` window, sweep-before-saver, `daemon.lock` singleton, self-supervision) are confirmed to hold under the concurrent route
- [ ] The live event loop performs no tmux/state mutation while the orchestrator runs (TUI inert during loading), asserted by a test
- [ ] Startup-ordering integration tests pass for the concurrent cold boot (step ordering preserved, daemon singleton, no zombie/leaked daemon, no slow-open regression)
- [ ] A warm-path synchronous-ordering parity assertion exists and passes
- [ ] Every daemon-spawning test uses `IsolateStateForTest(t)` with env applied to every subprocess; no `t.Parallel()`
- [ ] Non-visual risk-review/test task — explicitly `vhs`-exempt; verification is behavioural and the kill-barrier flake is re-run in isolation before any timing failure is called a regression

**Tests**:
- `"it preserves the orchestrator step ordering under the concurrent cold boot"`
- `"it spawns the daemon exactly once (singleton) with no zombie/leaked daemon under the concurrent boot"`
- `"it performs no tmux/state mutation from the live event loop while the orchestrator runs"`
- `"it keeps the @portal-restoring set-before-restore / clear-before-cleanup window intact concurrently"`
- `"it asserts warm-path synchronous ordering parity (no loading page, unchanged ordering)"`
- `"it isolates every daemon-spawning test via IsolateStateForTest with env applied to subprocesses"`

**Edge Cases**:
- Prior-incident surface (slow-open / empty-previews / zombie-session): explicitly assert no slow-open regression and no leaked test daemon corrupting the developer's `~/.config/portal/state/` — the `IsolateStateForTest` fingerprint-diff backstop is defence-in-depth, not a substitute for the env override.
- A daemon spawned in a test must be reaped (SIGKILL+Wait) so it does not hold open the state dir's file descriptors past `t.Cleanup` (load-bearing on macOS) — use `SpawnIsolatedDaemon`/`RegisterSubprocessCleanup`.
- The `internal/tmux` kill-barrier timing test is load-flaky under full `go test ./...` — re-run in isolation before treating a timing failure as a regression (do not "fix" a flaky timing assertion as if it were a concurrency regression).
- No `t.Parallel()` in the cmd package (package-level mutable mock state: `bootstrapDeps`, `openDeps`, etc., cleaned via `t.Cleanup`).
- A concurrent boot where the orchestrator finishes before the first render (very fast cold boot, M=0) and one where it finishes after a slow restore — both must reach a clean Sessions page with the daemon singleton intact.

**Context**:
> §10.2 real-costs/risks: "careful restore/daemon race review against the live event loop (prior-incident history); integration-test updates around startup ordering." §10 phase note: "~1–1.5 days incl. tests + race review — treat the estimate as having genuine variance given the load-bearing startup path and its prior-incident history (the slow-open / zombie-session episode)." §10.2: "the TUI is inert during loading (animation only) — this contains the race surface."
>
> Existing code / discipline: `cmd/bootstrap/bootstrap.go` ordering (the `@portal-restoring` Set step 3 / Clear step 8 window; SweepOrphanDaemons step 4 before EnsureSaver step 5). `internal/state.AcquireDaemonLock` / `daemon.lock` flock singleton + Component C pre-check; daemon self-supervision hysteresis in `cmd/state_daemon.go`. `portaltest.IsolateStateForTest(t) (env, stateDir)` (`internal/portaltest/isolated_env.go`) — MANDATORY for daemon-spawning tests; `SpawnIsolatedDaemon`/`RegisterSubprocessCleanup` for the reap pattern. cmd tests MUST NOT use `t.Parallel()` (package-level mock state). The `internal/tmux` kill-barrier timing test is known load-flaky (reference: `reference_flaky_killbarrier_test.md`).

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §10.2, §10
