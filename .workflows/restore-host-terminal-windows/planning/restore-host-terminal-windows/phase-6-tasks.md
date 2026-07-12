---
phase: 6
phase_name: Picker Burst Integration
total: 11
---

## restore-host-terminal-windows-6-1 | approved

### Task 6.1: Async terminal-detection lifecycle + caching

**Problem**: The picker must know the host-terminal identity to (a) surface the proactive unsupported banner (6-2) and (b) gate the N≥2 Enter burst (6-3/6-9). Detection is a `ps`/process-tree/`defaults read`/`list-clients` walk costing tens of ms, so it must run **asynchronously off the ~50ms first-paint appearance gate**, run **exactly once per picker session** on Sessions-page entry, and be **cached** — `rebuildSessionList` (hit on `s`-toggle, refresh, filter, projects-edit return) must never re-walk. The gate must distinguish an **in-flight** identity (dispatched, not yet resolved) from a **resolved NULL** (unsupported), and a transient detection error must fold to the same cached-unsupported state (the Phase-1 `spawn` WARN was already emitted inside `Detector.Detect`). Today `internal/tui` has no detection seam and no cached-identity state.

**Solution**: Inject a `TerminalDetector` seam (`Detect() spawn.Identity`) into the model via `tui.Deps.Detector` + a `WithTerminalDetector` option (wired in `cmd/open.go` `buildTUIModel` to `spawn.NewDetector(client)`; nil in the capture harness). Add cached-detection state to `Model` (`detectIdentity spawn.Identity`, `detectResolved bool`, `detectDispatched bool`), a `maybeDispatchDetectionCmd() tea.Cmd` that runs `detector.Detect()` in an async `tea.Cmd` goroutine and returns a `terminalDetectedMsg`, and an Update arm that caches the result. Dispatch the command once when the model settles on the Sessions page — from `transitionFromLoading` (cold loading→Sessions route, in the `LoadingMinElapsedMsg`/`BootstrapCompleteMsg` arms) and from the warm direct-Sessions landing (the `SessionsMsg` arm that runs `evaluateDefaultPage`) — guarded by `detectDispatched` so it fires exactly once and never on a rebuild.

**Outcome**: On Sessions-page entry the model dispatches exactly one async detection walk (never gating first paint); its `terminalDetectedMsg` caches `detectIdentity` and flips `detectResolved` true. Before that message the model is **in-flight** (`detectDispatched && !detectResolved`), distinct from a **resolved NULL** (`detectResolved && detectIdentity.IsNull()`). `rebuildSessionList`, the `s`-regroup, filter transitions, a `SessionsMsg` refresh, and the projects-edit→Sessions return never re-dispatch (the `detectDispatched` guard holds). A transient detection error surfaces as `Identity{}` from `Detect()` (Phase-1 already logged the WARN), which the model caches identically to a clean NULL. A direct warm Sessions entry (no loading page) dispatches exactly once too.

**Do**:
- In `internal/tui/build.go` add `Deps.Detector TerminalDetector` (a new 1-method seam `type TerminalDetector interface { Detect() spawn.Identity }`, declared in a new `internal/tui/spawn_detect.go`), wired in `Build` via a nil-tolerant `WithTerminalDetector(deps.Detector)` option (omit when nil, mirroring the other optional seams). In `cmd/open.go` `buildTUIModel`, set `Detector: cfg.detector` where `tuiConfig.detector = spawn.NewDetector(client)` (built once alongside the other cfg seams; nil-safe). **Also inject the config-aware `Resolve func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` seam here (`Deps.Resolve` + `WithResolve`, stored as the `m.resolve` field), defaulting in `cmd/open.go` to the SAME `spawn.NewResolver(terminals.json).Resolve` the CLI builds in Task 4.6 — load `terminals.json` once at TUI construction (alongside `Detector`), degrading to an empty native-only config on a `configFilePath` error. This is the single injection site for the resolve seam; Task 6.3's burst reuses `m.resolve` (it does not re-inject it), so the proactive banner (6-2) and the N≥2 gate (6-3/6-9) share one config-aware resolver.**
- In `internal/tui/model.go` add `Model` fields: `detector TerminalDetector`, `resolve func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` (the config-aware resolve seam injected via the `build.go` bullet above — the single field the `terminalDetectedMsg` arm consumes and that Task 6.3's burst dispatch and Task 6.9's gate reuse; never re-injected downstream), `detectIdentity spawn.Identity`, `detectResolution spawn.Resolution`, `detectResolved bool`, `detectDispatched bool`. Add test accessors mirroring the existing convention: `func (m Model) DetectDispatched() bool`, `func (m Model) DetectResolved() bool`, `func (m Model) DetectedIdentity() spawn.Identity`, `func (m Model) DetectedResolution() spawn.Resolution`, and `func (m Model) DetectUnsupported() bool { return m.detectResolved && m.detectResolution == spawn.ResolutionUnsupported }` — the single "this terminal cannot spawn host windows" predicate, **true for a NULL remote/mosh identity AND a non-NULL recognised-but-undriven identity (e.g. Apple Terminal → `com.apple.Terminal`)**. This is the resolution-based unsupported test the proactive banner (6-2) and the N≥2 gate (6-3/6-9) share; `IsNull()` alone is NOT the unsupported test (a non-NULL undriven identity is unsupported by *resolution*, not by NULL-ness — same as CLI task 2-7).
- Add `func (m *Model) maybeDispatchDetectionCmd() tea.Cmd`: return `nil` if `m.detector == nil` (harness/unwired), if `m.detectDispatched` (already fired), or if `m.activePage != PageSessions` (only on Sessions entry). Otherwise set `m.detectDispatched = true` and return an async `tea.Cmd` closure `func() tea.Msg { return terminalDetectedMsg{identity: m.detector.Detect()} }`. This runs on Bubble Tea's command goroutine, so the walk never blocks `Update` and never participates in the appearance-gate first-paint wait (spirit of `appearance_gate.go` — detection is async, resolves later, paints its banner when it lands).
- Define `type terminalDetectedMsg struct { identity spawn.Identity }` and add an Update arm that caches the identity **and its resolution** (resolution is a pure function of the identity + the once-loaded `terminals.json`, so resolving here caches it for the banner and the Enter gate — no re-walk, no re-load): `case terminalDetectedMsg: m.detectIdentity = msg.identity; _, m.detectResolution = m.resolve(msg.identity); m.detectResolved = true; return m, nil` — where `m.resolve` is the config-aware `Resolve` seam injected in this task's `build.go` bullet (wired in `cmd/open.go`). Caching `detectResolution` (not just `IsNull()`) is load-bearing: a recognised-but-undriven terminal is non-NULL yet resolves `unsupported`, so the banner (6-2) and the N≥2 no-op (6-9) key on `DetectUnsupported()`, not on `IsNull()`. (6-9 later extends this arm to resolve a deferred N≥2 Enter against the cached resolution.)
- Wire the two dispatch entry points (guarded, so exactly once):
  - Cold loading→Sessions: in the `LoadingMinElapsedMsg` and `BootstrapCompleteMsg` arms, after `m.transitionFromLoading()`, batch `m.maybeDispatchDetectionCmd()` into the returned `tea.Batch(...)` (alongside `surfaceBufferedWarnings()` / `refetchSessionsAfterRestore()`). The guard makes the second arm a no-op.
  - Warm direct Sessions entry: in the `SessionsMsg` arm, after `m.evaluateDefaultPage()` lands on `PageSessions` (the non-loading branch, `m.sessionsLoaded = true`), batch `m.maybeDispatchDetectionCmd()` into the returned cmd. (On the cold route this arm early-returns while `PageLoading`, so it does not fire there — the transition arms own the cold dispatch.)
- Confirm `rebuildSessionList`, `handleSwitchViewKey`, `applySessions`, and the projects-edit→Sessions transition never call `maybeDispatchDetectionCmd` and never reset `detectDispatched`/`detectResolved` — add a regression test per path.

**Acceptance Criteria**:
- [ ] Reaching PageSessions dispatches exactly one detection command; `DetectDispatched()` becomes true and a subsequent `terminalDetectedMsg` sets `DetectResolved()` true with the cached `DetectedIdentity()`.
- [ ] The `terminalDetectedMsg` arm caches the adapter **resolution** (`DetectedResolution()`) alongside the identity, computed once via the injected config-aware `m.resolve`; `DetectUnsupported()` is true for a resolved-`ResolutionUnsupported` identity (a NULL remote/mosh identity **or** a non-NULL recognised-but-undriven identity like `com.apple.Terminal`) and false for a `native`/`config` resolution; a rebuild never re-resolves.
- [ ] Before the `terminalDetectedMsg`, `DetectDispatched() && !DetectResolved()` (in-flight); after it with a NULL identity, `DetectResolved() && DetectedIdentity().IsNull()` (resolved NULL) — the two states are distinguishable.
- [ ] A rebuild (`s`-toggle, `SessionsMsg` refresh, filter apply/clear, projects-edit→Sessions return) does **not** dispatch a second detection command (`detectDispatched` stays latched; the fake detector's `Detect` call count stays 1).
- [ ] A transient detection error (fake detector returns `spawn.Identity{}`) caches as unsupported (`IsNull()` true); the model emits no additional WARN (the Phase-1 `Detect` chokepoint already did).
- [ ] A direct warm Sessions entry (no loading page — model built on PageSessions, first `SessionsMsg` lands) dispatches detection exactly once.
- [ ] The detection command is never part of the first-paint appearance gate — the appearance gate resolves independently of `detectResolved`.

**Tests** (Bubble Tea model unit tests in `internal/tui`, no `t.Parallel()`; inject a fake `TerminalDetector` recording `Detect` calls):
- `"it dispatches terminal detection exactly once on Sessions-page entry"`
- `"it caches the resolved identity from terminalDetectedMsg"`
- `"it distinguishes in-flight (dispatched, unresolved) from resolved-NULL"`
- `"it never re-dispatches detection on an s-regroup, filter, or SessionsMsg refresh"`
- `"it dispatches once on a direct warm Sessions entry with no loading page"`
- `"it caches a transient-error identity as unsupported without emitting a second WARN"`

**Edge Cases**:
- Rebuild (`s`-toggle/refresh/filter/projects-edit return) never re-dispatches detection (the `detectDispatched` latch).
- Transient detection error caches as unsupported (Phase-1 WARN already emitted inside `Detect`).
- In-flight state (`detectDispatched && !detectResolved`) distinct from resolved-NULL (`detectResolved && IsNull()`).
- Direct warm Sessions entry (no loading page) also dispatches exactly once.

**Context**:
> Spec *Terminal Identity & Detection → Detection lifecycle*: "**Detect once, cached.** The host-terminal identity is invariant for the picker's lifetime, so detection runs **once per picker session** on Sessions-page entry and is cached — reused by the on-entry banner and the N≥2 Enter gate. `rebuildSessionList` (hit on `s`-toggle, refresh, filter, projects-edit return) must **not** re-walk; it reads the cached identity. … **Off the first-paint path.** Detection runs asynchronously (the walk … costs tens of ms); the banner appears when it resolves, so it never stalls the ~50ms appearance-gate first paint. **Error vs clean NULL.** A clean NULL … and a *transient detection error* … both resolve to the unsupported/no-op path; a transient error additionally emits a `spawn`-component WARN breadcrumb. **In-flight at Enter.** … The gate distinguishes **in-flight** (not yet set) from a **resolved NULL**."
> The transient-error WARN is emitted inside `spawn.Detector.Detect()` (Phase-1 Task 1.5 — it folds the transient error to `Identity{}` and logs the WARN before returning), so the TUI simply caches the NULL identity and adds no logging of its own.
> `spawn.NewDetector(client *tmux.Client) *spawn.Detector` and `(*Detector).Detect() Identity` are the Phase-1 entry points; `spawn.Identity` carries `BundleID`/`Name` and `IsNull()`. The model holds the `Detect() spawn.Identity` seam behind a 1-method interface (Portal DI style), so tests inject a fake and the harness leaves it nil.
> The async dispatch mirrors how `internal/tui` already issues off-first-paint async commands (`tea.RequestBackgroundColor` in `Init`): a `tea.Cmd` closure whose goroutine performs the read and returns a `tea.Msg` consumed in `Update`. Detection is a read-only walk (no tmux mutation), and it is dispatched only after the cold-path hydration completes (Sessions-page entry gates on `BootstrapCompleteMsg`), so it never contends with a hydrating server (spec *Concurrency & Post-Reboot Safety*).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Terminal Identity & Detection → Detection lifecycle (Detect once / Off the first-paint path / Error vs clean NULL / In-flight at Enter)*; *Testing Strategy & DI Seams → Detection behind small seams*.

---

## restore-host-terminal-windows-6-2 | approved

### Task 6.2: Proactive unsupported/NULL banner + notice-band slot precedence

**Problem**: When detection (6-1) resolves the host terminal to NULL/unsupported (remote/mosh, or a recognised-but-undriven terminal like Apple Terminal), the user must learn this **proactively over the normal list** — before marking anything — via a banner naming the detected identity for copy-paste. The delivered Paper frame (`design/sessions-unsupported-terminal.png`) fixes the render: an amber `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal` at the section-header row with a right-aligned blue `see docs`, over the untouched normal Sessions list/footer. The banner must NOT show while detection is in-flight, must NOT show on a supported terminal, must step aside while in multi-select mode (the multi-select banner owns the mode indication), and must be glyph-backed under NO_COLOR.

**Solution**: Add `renderUnsupportedHeader(name, bundleID, width, mode, colourless) string` in `internal/tui/section_header.go` — an amber `⚠` + "unsupported terminal" (amber) + "— {name} · {bundleID}" (dim `text.detail`) left cluster, right-anchored blue `see docs`, composed through the shared `renderSectionHeaderRow`/`assembleRightAnchoredRow` geometry (same as the multi-select banner from Task 5.3). Insert it as a claimant in the Sessions-page section-header resolver (`applySectionHeader`, `internal/tui/model.go`) below the multi-select banner and above the standard section header, gated on `m.DetectUnsupported() && !m.multiSelectMode` — the resolution-based test (`detectResolved && detectResolution == ResolutionUnsupported`, true for a NULL remote/mosh identity **and** a non-NULL recognised-but-undriven identity like Apple Terminal), **not** `m.detectIdentity.IsNull()` (which would hide the banner for the design's own non-NULL `com.apple.Terminal` frame and fire only for NULL — see the Do section). Extend the `activeNoticeBand` gating (`internal/tui/notice_band.go`) so the By-Tag "No tags yet" signpost is also suppressed while the unsupported banner owns the section-header row (the banner outranks the signpost per the precedence).

**Outcome**: On a resolved-unsupported terminal outside multi-select mode, the section-header row reads `⚠ unsupported terminal — <name> · <bundleID>` (amber `⚠` + amber label + dim identity) with a right-aligned blue `see docs`, over the normal list — matching the delivered frame; while detection is in-flight the standard `Sessions ··· N` header shows (no banner); on a supported terminal the standard header shows; entering multi-select mode replaces it with the violet `N selected` banner (the unsupported banner steps aside, re-asserting only at the N≥2 Enter block per 6-9); the By-Tag signpost never co-renders with the unsupported banner; and under NO_COLOR the `⚠`, the label text, and `see docs` survive on the native fg/bg with all hues dropped.

**Do**:
- In `internal/tui/section_header.go` add `func renderUnsupportedHeader(name, bundleID string, width int, mode theme.Mode, colourless bool) string`:
  - Left cluster: **for a named (non-NULL) identity** — the `⚠` glyph (reuse `flashWarningGlyph` = `⚠`) + a space + `"unsupported terminal"` in `theme.MV.AccentOrange` (amber — the existing warning accent, no new token), then `" — " + name + " · " + bundleID` in `theme.MV.TextDetail` (the dim identity string, copy-paste key). Match the frame's `—` em-dash and `·` middot separators exactly. **For a NULL identity** (empty `name`/`bundleID`, i.e. remote/mosh — detect via `bundleID == ""`) render the honest `⚠ no host-local terminal` line instead (amber `⚠` + label, no identity, no `see docs`), matching CLI task 2-7's `IsNull()` copy branch.
  - Right hint: `"see docs"` in `theme.MV.AccentBlue` (the existing blue link token used elsewhere for links/hints).
  - Compose through the **shared** right-anchored section-header assembler (`renderSectionHeaderRow`/`assembleRightAnchoredRow`) so alignment, the canvas-painted flex spacer, and §2.7 narrow-degrade match the standard header exactly. One row (replaces the section header; no added row → pagination budget unchanged).
  - Under NO_COLOR every hue + the canvas drop; the `⚠` glyph, the label, the identity, and `see docs` survive (glyph-backed, never colour-only — §2.5).
- In `internal/tui/model.go` `applySectionHeader`, insert the unsupported claimant in precedence order (extending Task 5.3's chain):
  1. `FilterState() == list.Filtering` → filter input owns the row (unchanged).
  2. `m.multiSelectMode` → the violet `N selected` banner (Task 5.3) — the multi-select banner steps in front of the unsupported banner while in mode.
  3. **new:** `m.DetectUnsupported()` (resolved AND `detectResolution == ResolutionUnsupported` — covers a NULL remote/mosh identity **and** a non-NULL recognised-but-undriven identity like Apple Terminal, per 6-1) → `renderUnsupportedHeader(m.detectIdentity.Name, m.detectIdentity.BundleID, m.contentWidth(), m.canvasMode, m.colourless)` (first-line replacement, same mechanism as the FilterApplied query header), where `renderUnsupportedHeader` renders the honest `⚠ no host-local terminal` line when `m.detectIdentity.IsNull()` (empty Name/BundleID, remote/mosh) and the `⚠ unsupported terminal — <name> · <bundleID>` line for a named identity — the same copy-branch as CLI task 2-7. Gated on `detectResolved` (via `DetectUnsupported()`) so an **in-flight** identity shows the standard header, not the banner. Do **not** gate on `m.detectIdentity.IsNull()`: that hides the banner for the design's own Apple Terminal example (a non-NULL identity) and fires only for NULL — where the "unsupported terminal — · " copy is wrong.
  4. `FilterState() == list.FilterApplied` → the existing query header.
  5. else → the existing `renderSectionHeader`.
- In `internal/tui/notice_band.go` `activeNoticeBand`, gate the `byTagSignpost` arm off when the unsupported banner owns the section-header row: `if m.byTagSignpost && !m.multiSelectMode && !m.unsupportedBannerActive() { return bandInfo, byTagSignpostText, true }` — add a small `func (m Model) unsupportedBannerActive() bool { return m.DetectUnsupported() && !m.multiSelectMode }` helper (also consumed by `applySectionHeader` step 3 so the two reads can't drift). `DetectUnsupported()` (6-1) is the resolution-based test — true for a NULL remote/mosh identity and a non-NULL undriven identity alike — **not** `IsNull()`. The transient-flash arm stays first (unchanged).
- Do **not** route the unsupported banner through the `▌`-barred notice band — the delivered frame renders it as a bare section-header analogue with **no** `▌` left bar (like the multi-select and filter banners), so it lives at the section-header row (`applySectionHeader`), not `renderNoticeBand`.

**Acceptance Criteria**:
- [ ] On a resolved-unsupported terminal outside multi-select mode — whether a NULL remote/mosh identity or a non-NULL recognised-but-undriven identity (e.g. `com.apple.Terminal`) — the section-header row renders the unsupported banner (the named `⚠ unsupported terminal — <name> · <bundleID>` + blue `see docs` for a non-NULL identity; the honest `⚠ no host-local terminal` line for a NULL identity), and the `Sessions ··· N` header is not shown.
- [ ] While detection is in-flight (`detectDispatched && !detectResolved`), the standard `Sessions ··· N` header shows (no banner).
- [ ] On a resolved **supported** identity (resolver returns `native`/`config`), no banner shows (standard header). (Being non-NULL is **not** sufficient — a non-NULL identity the resolver cannot drive is unsupported.)
- [ ] Entering multi-select mode replaces the unsupported banner with the violet `N selected` banner (the unsupported banner steps aside).
- [ ] The By-Tag "No tags yet" signpost does not render while the unsupported banner owns the section-header row.
- [ ] Under NO_COLOR the `⚠`, the label, the identity string, and `see docs` render on the native fg/bg with no hue and no canvas fill.
- [ ] The banner matches the delivered frame `design/sessions-unsupported-terminal.png` (identity naming, `see docs` right-anchor, layout).

**Tests** (unit tests in `internal/tui`; assert the composed `applySectionHeader`/`viewSessionList` substring):
- `"it renders the amber unsupported banner naming name and bundle id with a blue see docs"`
- `"it renders the unsupported banner for a non-NULL undriven identity (Apple Terminal com.apple.Terminal)"`
- `"it shows the standard header while detection is in-flight"`
- `"it shows no banner on a supported terminal"`
- `"it lets the multi-select banner own the row and steps the unsupported banner aside in mode"`
- `"it suppresses the no-tags signpost while the unsupported banner is active"`
- `"it keeps the unsupported banner glyph-backed under NO_COLOR"`

**Edge Cases**:
- Detection in-flight → no banner (standard header).
- Supported terminal → no banner.
- In multi-select mode the multi-select banner owns the slot (unsupported steps aside; re-asserts at the N≥2 Enter block — 6-9).
- NO_COLOR glyph-backed, never colour-only (`⚠`/label/identity/`see docs` survive).

**Context**:
> Spec *Terminal Identity & Detection → Unsupported-terminal behaviour (banner + Enter)*: "**Detection runs on Sessions-page entry**, so the unsupported/unconfigured banner (naming the detected identity) surfaces **proactively** over the normal list — you know the terminal is unsupported before marking anything. **Multi-select stays available** on an unsupported terminal."
> Spec *Terminal Identity & Detection → User-facing display: both*: the banner shows both the friendly `.app` name and the exact bundle id (copy-paste key). "(Design copy example, from the delivered banner frame: `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal` with a `see docs` link.)"
> Spec *Multi-Select Mode → Mode affordance (notice-band precedence)*: "filter line (filter focused) → in-burst `Opening n/N…` (burst pending) → transient error/guidance flash → multi-select banner (in mode) → unsupported-terminal banner → no-tags signpost. On an unsupported terminal, entering multi-select shows the multi-select banner (the unsupported banner steps aside) and the unsupported warning re-asserts at the N≥2 Enter block."
> **Placement decision (design-anchored, ambiguity flagged):** the delivered frame renders the unsupported banner at the **section-header row** (amber `⚠` left + blue `see docs` right, no `▌` left bar), a filter-line analogue — not through the `▌`-barred notice band the spec's abstract precedence language describes. This mirrors the Task 5.3 decision (the golden frame governs placement; the multi-select `N selected` banner is likewise at the section-header row). So the higher precedence claimants (filter line, `Opening n/N…`, transient flash, multi-select banner, unsupported banner) are section-header-row claimants arbitrated by `applySectionHeader`; the no-tags signpost is the sole `▌` notice-band claimant (lowest), suppressed whenever a higher claimant is active. The tokens are the existing amber (`AccentOrange`) / blue (`AccentBlue`) / dim (`TextDetail`) — **no new tokens** (spec *Design References → Tokens*).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Terminal Identity & Detection → Unsupported-terminal behaviour / User-facing display: both*; *Multi-Select Mode → Mode affordance (notice-band precedence)*; *Design References → Sessions — Unsupported terminal (banner) / Tokens*.

---

## restore-host-terminal-windows-6-3 | approved

### Task 6.3: N≥2 burst dispatch + async spawn tea.Cmd + streaming message protocol

**Problem**: Task 5.7 left the N≥2 Enter branch of `handleMultiSelectEnter` a documented no-op stub. This task wires it to the in-process spawn service: on N≥2 Enter (supported terminal), the picker must run the pre-flight → sequential spawn → per-window token-ack burst as an **async, non-blocking `tea.Cmd`** (goroutine + progress channel + re-issued receiver — the same pattern as the cold-path concurrent bootstrap), streaming per-window progress and a terminal result back as `tea.Msg`s. It must open the marked set in **list order** (selection is a set), exclude the **trigger** session from the N−1 external spawn set (net-N), never open a cursor-but-unmarked row, and defer the branch decision when detection is still in-flight. The completion behaviours (self-attach, selection mutation, abort, cancellation, observability) are separate tasks (6-4/6-6/6-7/6-8/6-9/6-10); this task establishes the dispatch machinery, the streaming protocol, and the burst-pending entry.

**Solution**: Add an async burst pipe to `internal/tui` mirroring `cmd/bootstrap_progress.go`: a `burstProgressPipe` (buffered channel + goroutine + `receiver() tea.Cmd`) and the message shapes (`spawnProgressMsg{done,total}`, `spawnCompleteMsg{...}`, `spawnAbortMsg{gone}`). Inject the spawn seams the picker reuses from `internal/spawn` (the same pieces the CLI's `runSpawn` composes) via `tui.Deps`: `Resolve func(spawn.Identity)(spawn.Adapter,spawn.Resolution)` (`spawn.ResolveAdapter`), `SessionExists func(string)bool` (`*tmux.Client.HasSession`, for pre-flight), a burst runner over `spawn.Burster` (adapter + `spawn.AckChannelFull` + `spawn.ExecutableResolver` + `Getenv` + timing), and the ack channel's `Clean`. Rewire `handleMultiSelectEnter`'s N≥2 branch to compute the list-ordered marked set, pick the trigger + external split, and dispatch the burst `tea.Cmd`; enter burst-pending state. Add a `Progress func(done,total int)` callback + a `context.Context` to `spawn.Burster.Run` (additive Phase-6 integration seam, analogous to `internal/restore`'s per-session `Progress func(n,m int)` and the ctx-guarded bootstrap goroutine) so the goroutine can stream progress and 6-8 can cancel.

**Outcome**: N≥2 Enter on a supported terminal snapshots the marked sessions in list order, splits them into one trigger (self-attach target) + the N−1 external set, and dispatches an async burst goroutine that runs `spawn.PreflightMissing` then `spawn.Burster.Run(ctx, external, progressCb)` — streaming a `spawnProgressMsg{done,total}` per window (advancing the receiver) and a terminal `spawnCompleteMsg` (or `spawnAbortMsg` on a pre-flight gone). The model enters burst-pending on dispatch. The external set excludes the trigger (net-N), opens in list order, never includes a cursor-but-unmarked row, and an Enter fired while detection is in-flight defers until `terminalDetectedMsg` then branches (supported → burst; NULL → 6-9 no-op). N=0/N=1 (Task 5.7) are untouched. Driven in unit tests via `spawntest.FakeAdapter`/`FakeAckChannel` + a fake clock, or by injecting the terminal message directly.

**Do**:
- Add `internal/tui/burst_progress.go` (mirroring `cmd/bootstrap_progress.go`):
  - `type burstProgress struct { Done bool; DoneCount, Total int; Batch string; Results []spawn.WindowResult; Identity spawn.Identity; Resolution spawn.Resolution; Gone []string; Err error }` — non-terminal events carry `DoneCount`/`Total`; the single terminal event sets `Done` and carries the batch/results (or `Gone` for a pre-flight abort, or `Err`).
  - `type burstProgressPipe struct { ch chan burstProgress; cancel context.CancelFunc }` with `newBurstProgressPipe()`, a `start(...)` that launches the goroutine, `send(ctx, ev)` (ctx-guarded select, exactly like `bootstrap_progress.go`'s `send`), and `receiver() tea.Cmd` (single blocking receive → `spawnProgressMsg` for non-terminal / `spawnCompleteMsg` or `spawnAbortMsg` for terminal / a closed-channel sentinel; re-issued only on non-terminal events).
  - The goroutine body: (1) `gone := preflight(external+trigger)`; if non-empty, send the terminal `burstProgress{Done:true, Gone:gone}` and close — **no spawn** (6-7 owns the UI). (2) else `batch, results, err := burster.Run(ctx, external, func(done,total int){ pipe.send(ctx, burstProgress{DoneCount:done, Total:total}) })`; then `cleanBatch(batch)` (marker self-clean, all paths — 6-4 tests the success case, 6-6/6-8 rely on it); send the terminal `burstProgress{Done:true, Batch:batch, Results:results, Identity:id, Resolution:res, Err:err}` and close.
- Define the messages in `internal/tui/model.go` (or `burst_progress.go`): `type spawnProgressMsg struct { Done, Total int }`, `type spawnCompleteMsg struct { Batch string; Results []spawn.WindowResult; Identity spawn.Identity; Resolution spawn.Resolution; Err error }`, `type spawnAbortMsg struct { Gone []string }`, `type burstChannelClosedMsg struct{}`.
- Inject the remaining spawn seams into the model via `tui.Deps` + `With*` options (nil-tolerant; nil in the harness): `SessionExists func(string)bool` (default `client.HasSession`), `AckChannel spawn.AckChannelFull` (default `spawn.NewServerOptionAckChannel(client,client)`), `SpawnExe spawn.ExecutableResolver` (default `os.Executable`), `SpawnGetenv func(string)string` (default `os.Getenv`). Wire all in `cmd/open.go` `buildTUIModel`/`tuiConfig`. **The `Resolve` seam is already injected in Task 6.1 as the single injection site (the config-aware `spawn.NewResolver(terminals.json).Resolve`, the SAME resolver the CLI builds in Task 4.6 — NOT the zero-config `spawn.ResolveAdapter`, which never reads config); this task REUSES `m.resolve` for the burst's adapter resolution, so the in-picker burst honours `terminals.json` identically to `portal spawn`.** Packaging (individual fields vs a `SpawnDeps`-style struct) is an implementation choice — mirror `cmd`'s `SpawnDeps` shape for consistency.
- Extend `spawn.Burster.Run` to `Run(ctx context.Context, external []string, progress func(done, total int)) (batch string, results []spawn.WindowResult, err error)` — an **additive** integration seam: call `progress(i+1, len(external))` after each window's ack classification (nil-tolerant, like restore's `Progress`), and check `ctx.Err()` between windows and inside the ack poll to abandon remaining spawns on cancellation (6-8 drives the cancel). Document the callback + ctx like restore's `Progress func(n, m int)`. **Because this changes the signature of an approved Phase-3 seam, update its existing call site in the same change: in `cmd/spawn.go` `runSpawn`, change `burster.Run(external)` to `burster.Run(context.Background(), external, nil)` — the nil progress + `context.Background()` preserve the exact Phase-2/3 CLI behaviour (no progress streaming, no cancellation).** Any Phase-3 `internal/spawn/burst_test.go` call sites that invoke `Run(external)` are updated the same way.
- Rewire `handleMultiSelectEnter` N≥2 branch (`internal/tui/model.go`), replacing the Task 5.7 stub:
  - Build the **list-ordered** marked set: walk the current `sessionList` items top-to-bottom, collecting each `SessionItem` whose `Session.Name ∈ m.selectedSessions`, de-duplicated by name (a multi-tag session appears once, at its first list position). This yields the open order (spec: open in list order, selection is a set — not pick order).
  - Split net-N: `trigger := ordered[last]` (the trigger self-attach target; which one is spec-unspecified implementation-convenience — last-in-list is fine), `external := ordered[:last]` (the N−1). A cursor-but-unmarked row is never in `ordered`, so it is never opened.
  - Detection gate: if `!m.detectResolved` → **defer**: stash a `pendingBurstEnter bool` (+ the captured `ordered`/`trigger`/`external` snapshot) and return `m, nil`; the `terminalDetectedMsg` arm (6-1) resolves it. If `m.detectResolved`, branch on the cached **resolution**, not `IsNull()`: if `m.DetectUnsupported()` (`detectResolution == ResolutionUnsupported`, covering NULL remote/mosh **and** a non-NULL undriven identity like Apple Terminal / an unknown terminal with no native or config adapter) → the 6-9 atomic no-op; else (a supported `native`/`config` resolution) → dispatch the burst below (the adapter is then guaranteed non-nil). (6-3 owns the supported dispatch + the defer plumbing; 6-9 owns the unsupported branch.) **Do not** branch on `IsNull()` — a non-NULL undriven identity is non-NULL yet unsupported, so an `IsNull()` gate would fall into the "supported" arm and dispatch the burst with a nil adapter → `Burster.Run` calls `OpenWindow` on nil.
  - Dispatch (supported): resolve the adapter `adapter, resolution := m.resolve(m.detectIdentity)`; construct the `burstProgressPipe` with a fresh `context.WithCancel`, store `m.burstPipe`, `m.burstCancel`, set `m.burstPending = true`, `m.burstTrigger = trigger`, `m.burstExternal = external`, `m.burstTotal = len(ordered)` (= N, including the self-attach target), `m.burstDone = 0`; `pipe.start(...)` (goroutine with adapter/ack/exe/getenv/preflight/clean bound), and return `m, pipe.receiver()`.
  - A minimal terminal handler for this task: `case spawnCompleteMsg:` record `m.burstResults = msg.Results`, `m.burstPending = false` (the self-attach + selection mutation are 6-4/6-6); `case spawnAbortMsg:` clear pending (the abort UI is 6-7). This keeps 6-3 independently testable (dispatch + protocol) while 6-4/6-6/6-7 layer the real completion behaviour.
- Model fields to add: `burstPending bool`, `burstPipe *burstProgressPipe`, `burstCancel context.CancelFunc`, `burstTrigger string`, `burstExternal []string`, `burstTotal int`, `burstDone int`, `burstBatch string`, `burstResults []spawn.WindowResult`, `burstIdentity spawn.Identity`, `burstResolution spawn.Resolution`, `pendingBurstEnter bool`. Add test accessors `BurstPending() bool`, `BurstExternal() []string`, `BurstTrigger() string`.

**Acceptance Criteria**:
- [ ] N≥2 Enter on a resolved-supported terminal enters burst-pending (`BurstPending()==true`) and dispatches an async burst that calls the fake adapter's `OpenWindow` once per **external** session in **list order** (top-to-bottom), never for the trigger.
- [ ] The external set is exactly the marked set minus the trigger (net-N): a batch of N marked sessions produces N−1 `OpenWindow` calls; `BurstTotal()==N`.
- [ ] A multi-tag By-Tag session marked once appears once in the open order (de-duplicated by name at its first list position).
- [ ] A cursor-but-unmarked row is never opened.
- [ ] The goroutine streams a `spawnProgressMsg{Done,Total}` per window and one terminal `spawnCompleteMsg` (batch + results) — the receiver is re-issued on progress and stops on the terminal message.
- [ ] N≥2 Enter while detection is in-flight defers: no burst dispatches until `terminalDetectedMsg` lands, then it branches (supported → burst; the NULL branch is 6-9).
- [ ] N=0 and N=1 Enter (Task 5.7) remain unchanged (no burst).
- [ ] The picker's default `Resolve` seam is the config-aware `spawn.NewResolver(terminals.json).Resolve` (built once in `cmd/open.go`, degrading to empty config on a `configFilePath` error), so an identity matching a valid `terminals.json` entry resolves to the config adapter + `ResolutionConfig` in the picker burst — identical to `portal spawn` (a regression test injects a config-matched `Resolve` and asserts the config adapter is used).

**Tests** (Bubble Tea model unit tests in `internal/tui`, driven via `spawntest.FakeAdapter` + `spawntest.FakeAckChannel` + a fake clock and/or direct message injection):
- `"it dispatches an async burst on N>=2 Enter for a supported terminal"`
- `"it opens the external set in list order excluding the trigger (net-N)"`
- `"it opens a multi-tag marked session once in the open order"`
- `"it never opens a cursor-but-unmarked row"`
- `"it streams a progress message per window and one terminal complete message"`
- `"it defers the N>=2 Enter decision while detection is in-flight then branches on resolve"`
- `"it leaves N=0 and N=1 Enter unchanged"`
- `"it resolves the burst adapter through the config-aware terminals.json resolver, matching the CLI"`

**Edge Cases**:
- Enter while detection in-flight → defer decision until resolved then branch.
- Open in list order (selection is a set, not pick order).
- Trigger session excluded from the N−1 external spawn set (net-N).
- Cursor-but-unmarked row never opened.
- N=0/N=1 still handled by Phase 5 (untouched).

**Context**:
> Spec *Burst & Partial-Failure Contract → In-picker execution model*: "**Async, non-blocking.** The picker runs the burst as an async `tea.Cmd` (goroutine) that streams progress + per-window ack results back to the model as `tea.Msg`s — the same pattern as the cold-path concurrent bootstrap. It never blocks the `Update` loop, so the TUI stays responsive and the cancellation points stay live."
> Spec *Spawn Architecture → The N vs N−1 split / Order is load-bearing*: "the picker **always self-attaches to exactly one** of the N; only the **N−1 others** are externally spawned. … 1. Detect … 2. Spawn the N−1 windows … 3. **Only after all N−1 confirm**, exec self into the Nth session."
> Spec *Trigger-Context Matrix → Open order: list order (selection is a set)*: "Open in **list order** (top-to-bottom as shown), not pick order. … **Which marked session the trigger window becomes: unspecified (implementation-convenience).**" *Enter opens the marked set only*: "a highlighted-but-unmarked row is **not** opened."
> Spec *Terminal Identity & Detection → Detection lifecycle (In-flight at Enter)*: "an in-flight identity is **awaited** … the burst proceeds once it resolves (supported → spawn; NULL/error → unsupported no-op). It is never treated as unsupported merely for being unresolved."
> The streaming machinery is a faithful clone of `cmd/bootstrap_progress.go` (buffered channel + goroutine + `send(ctx,...)` ctx-guarded select + a single-blocking-receive `receiver()` re-issued per non-terminal event). `spawn.Burster.Run` (Phase-3 Task 3.5/3.6/3.7) already does sequential spawn → per-window ack → continue-through-failure with a permission early-stop; this task adds only the `Progress` callback + `ctx` (mirroring `internal/restore`'s `Progress func(n,m int)` and the bootstrap goroutine's ctx-guarded sends). The picker reuses `spawn.PreflightMissing` (Task 3.4), the **config-aware** `spawn.NewResolver(cfg).Resolve` (native + `terminals.json` tier, Task 4.6 — NOT the zero-config `spawn.ResolveAdapter` wrapper, which never reads config), the `spawn.Burster`, and `spawn.AckChannelFull.Clean` (Task 3.2) — the same pieces `cmd/spawn.go`'s `runSpawn` composes, so a `terminals.json` recipe resolves identically in the picker and the CLI — but the **trigger self-attach is the existing `m.selected`+`tea.Quit`→`processTUIResult` connector path (6-4), NOT the CLI's direct-exec self-attach**. `HasSession` folds a probe error to false → gone → conservative abort (Task 3.4).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → In-picker execution model / Sequential spawn*; *Spawn Architecture → The N vs N−1 split / Order is load-bearing*; *Trigger-Context Matrix & Open Order → Open order / Enter opens the marked set only*; *Terminal Identity & Detection → Detection lifecycle (In-flight at Enter)*.

---

## restore-host-terminal-windows-6-4 | approved

### Task 6.4: Full-success self-attach (net N) + marker self-clean

**Problem**: When every external window in the burst confirms its token ack, the trigger window must **self-attach silently** to its one marked session — giving net N windows for N marked sessions, never N+1 — and the picker must self-clean its `@portal-spawn-<batch>-*` markers before the exec handoff. The self-attach must reuse the picker's **existing** `m.selected`+`tea.Quit`→`processTUIResult` connector path (`AttachConnector` outside tmux / `SwitchConnector` inside), not the CLI's direct-exec self-attach. There must be no "N/N ✓" nag. Task 6.3 left the `spawnCompleteMsg` handler recording results only.

**Solution**: Implement the full-success arm of the `spawnCompleteMsg` handler (`internal/tui/model.go`): when `msg.Err == nil` and every `WindowResult.Ack == spawn.AckConfirmed`, clean the batch markers (the burst goroutine already calls `cleanBatch(batch)` on all terminal paths — Task 6.3 — so this arm relies on it having run before the terminal message), set `m.selected = m.burstTrigger`, clear burst-pending state, and return `tea.Quit` — driving the existing connector via `processTUIResult`. Emit no success flash.

**Outcome**: A burst where all N−1 external windows confirm sets `m.selected` to the trigger session and returns `tea.Quit`; the outer `openTUI`/`processTUIResult` then connects the picker's own window to the trigger via `AttachConnector` (outside tmux) or `SwitchConnector` (inside) — net N windows, never N+1. The `@portal-spawn-<batch>-*` markers are cleaned before the exec handoff (in the burst goroutine, before the terminal message). No "N/N ✓" banner or success flash renders. An includes-self selection (the trigger is one of the marked sessions) and a session already attached elsewhere (e.g. the iPhone) both confirm and self-attach correctly (the token ack confirms *our* new window regardless of other clients).

**Do**:
- In `internal/tui/model.go`, extend the `case spawnCompleteMsg:` arm from Task 6.3:
  - Compute `allConfirmed := msg.Err == nil && len(msg.Results) == len(m.burstExternal)` AND every `r.Ack == spawn.AckConfirmed`. (An empty external set cannot reach here — N=1 is Task 5.7.)
  - On `allConfirmed`: set `m.selected = m.burstTrigger`; clear burst-pending state (`m.burstPending = false`, nil the pipe/cancel, zero the counters); return `m, tea.Quit`. This is byte-identical in effect to `handleSessionListEnter`'s commit (set `m.selected`; `tea.Quit`), so `processTUIResult` opens the trigger in the picker's own window via the already-resolved `connector` (`cmd/open.go` builds it once via `buildSessionConnector`). **No adapter** is used for the self-attach (single attach needs none).
  - Do **not** set a flash on success — the self-attach is silent (no "N/N ✓" nag).
  - The non-all-confirmed branch is Task 6.6 (partial/permission) and 6-7 (abort); leave a clear split so this task tests only the full-success path.
- Confirm the marker self-clean runs before the exec handoff: the Task 6.3 goroutine calls `m.cleanBatch(batch)` (→ `spawn.AckChannelFull.Clean`) after `Burster.Run` returns and before sending the terminal `spawnCompleteMsg`, so by the time this handler issues `tea.Quit` the markers are already unset. Assert the ordering (Clean recorded before the terminal message is produced) in the test. (The spec's "self-clean before self-exec" is satisfied because Clean happens in the goroutine strictly before the terminal message that triggers the quit.)
- Ensure `m.selected` is the trigger even for an **includes-self** selection: the trigger is one marked session (the origin session ends up attached in the reused window); the external set spawns the rest. No special-casing.

**Acceptance Criteria**:
- [ ] A `spawnCompleteMsg` with every `WindowResult.Ack == AckConfirmed` sets `Selected() == burstTrigger` and returns `tea.Quit` (drives the existing connector via `processTUIResult`).
- [ ] The batch markers are cleaned (`AckChannel.Clean(batch)` recorded) **before** the terminal `spawnCompleteMsg` is produced — i.e. before the self-attach exec handoff.
- [ ] No success flash / "N/N ✓" banner renders on full success (silent self-attach).
- [ ] An includes-self selection self-attaches to the trigger (one marked session) with the rest spawned externally — the origin session ends up attached.
- [ ] A session confirmed via ack while already attached elsewhere (fake ack writes the token) still self-attaches correctly (no dup guard; the ack confirms our new window).
- [ ] The self-attach uses the existing `AttachConnector`/`SwitchConnector` path (via `Selected()`+`tea.Quit`), not a spawn-adapter call.

**Tests** (Bubble Tea model unit tests in `internal/tui`, via `spawntest` fakes + a fake clock, or by injecting a fully-confirmed `spawnCompleteMsg`):
- `"it self-attaches to the trigger and quits when every external window confirms"`
- `"it cleans the batch markers before the self-attach exec handoff"`
- `"it renders no success flash on full success (net N, no N/N nag)"`
- `"it self-attaches on an includes-self selection with the rest spawned"`
- `"it confirms a window already attached elsewhere via the token ack"`

**Edge Cases**:
- Includes-self selection (trigger becomes one marked session).
- Session already attached elsewhere (iPhone) confirmed via ack.
- No "N/N ✓" nag (silent).
- Trigger-window reuse via existing `AttachConnector`/`SwitchConnector` (the `Selected()`+`tea.Quit`→`processTUIResult` path).

**Context**:
> Spec *Burst & Partial-Failure Contract → Spawn, then self-attach LAST*: "**All confirm** → the trigger window self-attaches silently (no '14/14 ✓' nag)."
> Spec *Spawn Architecture → The N vs N−1 split*: "**Outside tmux** → exec `tmux attach` (existing `AttachConnector`) … **Inside tmux** → `switch-client` (existing `SwitchConnector`). So the picker **always self-attaches to exactly one** of the N."
> Spec *Trigger-Context Matrix → Behaviour across trigger contexts*: "**Selected session already attached elsewhere** … allowed — no dup guard; the token ack confirms *our* new window regardless of other clients. **Includes-self** … the trigger window becomes one attached session, the rest spawn; the marked origin session ends up attached either way."
> Spec *Burst & Partial-Failure Contract → Cleanup*: "The picker self-cleans its batch markers before self-exec." The clean runs in the burst goroutine (Task 6.3) after `Burster.Run` and before the terminal message, so it is strictly before the `tea.Quit` this handler issues.
> `processTUIResult` (`cmd/open.go`) reads `model.Selected()` and calls `connector.Connect(selected)`; the connector is `buildSessionConnector(client)` (SwitchConnector inside tmux, AttachConnector outside). `spawn.AckConfirmed` is the Phase-3 `AckOutcome` value; `spawn.WindowResult` carries `Session`/`Token`/`Result`/`Ack`.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → Spawn, then self-attach LAST / Cleanup*; *Spawn Architecture → The N vs N−1 split*; *Trigger-Context Matrix & Open Order → Behaviour across trigger contexts*.

---

## restore-host-terminal-windows-6-5 | approved

### Task 6.5: Input-lock while pending + `Opening n/N…` feedback band

**Problem**: During the multi-second in-flight burst (up to ~`spawnAckTimeout` per window) the picker must be **inert to row actions** so no concurrent user input can race the completion handler's selection mutation (which the "retry re-opens only what's missing" guarantee depends on): a second `Enter`, `m`, navigation, `Space`, `/`, and `s` are all ignored; only `Ctrl-C`/`Esc` (cancellation — 6-8) stay live. Meanwhile the picker must show a pending affordance — `Opening n/N…` — that advances as each window's ack lands. Today `updateSessionList` has no burst-pending gate and no `Opening n/N…` band.

**Solution**: Add an input-lock guard at the top of `updateSessionList` (`internal/tui/model.go`): while `m.burstPending`, swallow all key input except `Ctrl-C` and `Esc` (routed to 6-8's cancellation). Fold each streamed `spawnProgressMsg{Done,Total}` into `m.burstDone`/`m.burstTotal` and render an `Opening n/N…` band in the section-header slot with precedence just below the filter line and above the transient flash (per the notice-band precedence), reusing the section-header-row geometry.

**Outcome**: While a burst is pending, a second `Enter` does not re-dispatch, and `m`/navigation/`Space`/`/`/`s` are no-ops; `Ctrl-C` and `Esc` remain live (6-8 cancels). Each `spawnProgressMsg` advances the counter, so the section-header row reads `Opening 1/3…`, `Opening 2/3…`, … as windows confirm. The `Opening n/N…` band sits just below the filter line in precedence (above the transient flash, the multi-select banner, and the unsupported banner). Under NO_COLOR the band text survives on the native fg/bg.

**Do**:
- In `internal/tui/model.go` `updateSessionList`, add a burst-pending input-lock as the first key-handling step (after the Ctrl-C early check is *preserved*): keep `if keyIsCtrlC(msg) { return m, tea.Quit }`... but 6-8 changes Ctrl-C mid-burst to *cancel* not quit; coordinate so Ctrl-C reaches 6-8's cancel path while pending. Concretely: `if keyMsg, ok := msg.(tea.KeyPressMsg); ok && m.burstPending { if keyIsCtrlC(keyMsg) || keyIsCode(keyMsg, tea.KeyEscape) { return m.cancelBurst() } // 6-8; return m, nil // swallow every other key while pending }`. Place this ahead of the flash-clear, the `SettingFilter` guard, and the rune switch so **no** row action (`m`, `k`, `x`, `r`, `s`, `Space`, `Enter`, `/`, navigation) fires while pending. (6-8 defines `cancelBurst`; until then a stub returning `m, nil` keeps this task compilable/testable for the swallow behaviour, with the cancel wired in 6-8.)
- Fold progress into the counter: in the `case spawnProgressMsg:` arm, advance **only** the done count — `m.burstDone = msg.Done; return m, m.burstPipe.receiver()` (re-issue the receiver to pull the next event — the standard single-blocking-receive loop from `bootstrap_progress.go`). Do **not** overwrite `m.burstTotal` from `msg.Total`: the denominator is fixed at dispatch to `len(ordered)` = **N** (all marked sessions incl. the trigger self-attach target, set in 6-3), matching the `opened N/N` log summary (6-10) and the `Opening 2/3…` capture fixture (6-11). `msg.Total` from `Burster.Run` is `len(external)` = N−1 (the per-window progress total) and is intentionally ignored for the denominator; `burstDone` advances 0…N−1 as external windows confirm, so the band's final frame before the silent self-attach reads `Opening N−1/N…` (never `N/N` — consistent with "no '14/14 ✓' nag").
- Render the `Opening n/N…` band. Add `func renderOpeningBand(done, total, width int, mode theme.Mode, colourless bool) string` in `internal/tui/section_header.go`: a left cluster `fmt.Sprintf("Opening %d/%d…", done, total)` in `theme.MV.AccentViolet` (the mode accent — no new token), composed through `renderSectionHeaderRow` (no right hint, or a dim `esc cancel` right hint consistent with the multi-select banner). Insert it into `applySectionHeader` at the top of the section-header precedence chain, just below the filter line and above the transient flash / multi-select banner: after the `Filtering` check, `if m.burstPending { return <first-line-replace with renderOpeningBand(m.burstDone, m.burstTotal, ...)> }`. One row (replaces the section header; pagination budget unchanged).
- Do **not** show the multi-select banner or the unsupported banner while burst-pending — the `Opening n/N…` band outranks them (the `burstPending` check precedes those arms in `applySectionHeader`).
- Add test accessors `BurstDone() int`, `BurstTotal() int` (if not already added in 6-3).

**Acceptance Criteria**:
- [ ] While `burstPending`, a second `Enter` does not re-dispatch a burst (no new pipe, no additional adapter calls).
- [ ] While `burstPending`, `m`, navigation keys, `Space`, `/`, and `s` are all no-ops (swallowed).
- [ ] While `burstPending`, `Ctrl-C` and `Esc` are live (route to `cancelBurst` — 6-8).
- [ ] Each `spawnProgressMsg` advances `BurstDone()` only; `BurstTotal()` stays at the dispatch-time N (= marked-set size, incl. the trigger). The section-header row renders `Opening <burstDone>/<N>…`, so a 3-session batch renders `Opening 1/3…` then `Opening 2/3…` (never `2/2`), and never reaches `3/3` (the trigger self-attaches silently).
- [ ] The `Opening n/N…` band renders with precedence just below the filter line — above the transient flash, the multi-select banner, and the unsupported banner (it is the section-header claimant while `burstPending`).
- [ ] Under NO_COLOR the `Opening n/N…` text renders on the native fg/bg with hue dropped.

**Tests** (Bubble Tea model unit tests in `internal/tui`):
- `"it ignores a second Enter while a burst is pending"`
- `"it ignores m, navigation, Space, slash, and s while a burst is pending"`
- `"it keeps Ctrl-C and Esc live while a burst is pending"`
- `"it advances the Opening n/N counter on each progress message"`
- `"it holds the Opening denominator at N (marked-set size) across progress messages"`
- `"it renders the Opening band just below the filter line in precedence"`
- `"it renders the Opening band glyph/text under NO_COLOR"`

**Edge Cases**:
- Second Enter mid-burst ignored (no double-dispatch).
- `m`/nav/`Space`/`/`/`s`/filter all ignored while pending.
- Ctrl-C/Esc stay live.
- Counter advances with each per-window progress msg.
- `Opening` band precedence just below the filter line.

**Context**:
> Spec *Burst & Partial-Failure Contract → In-picker execution model*: "**In-burst feedback.** While spawning and awaiting acks, the picker shows a pending affordance in the notice-band single-slot arbiter (e.g. `Opening n/N…`). **Input-locked while pending.** During the in-flight burst the picker is **inert to row actions** — `m` (mark), navigation, `Space` preview, `/`, `s`, and a second `Enter` are all **ignored**; only **cancel** (`Ctrl-C`/`Esc`) is live. This prevents any race between concurrent user input and the completion handler's selection mutation."
> Spec *Multi-Select Mode → Mode affordance (notice-band precedence)*: "filter line (filter focused) → in-burst `Opening n/N…` (burst pending) → transient error/guidance flash → multi-select banner (in mode) → unsupported-terminal banner → no-tags signpost."
> *Design residual:* "the delivered Paper set has no 'spawning / awaiting acks' frame — capturing one (or accepting a minimal counter) is a design-phase deliverable for the visual gate." So the `Opening n/N…` copy/style is a minimal counter here and its frame is captured in Task 6.11. Per the Task 5.3 / 6.2 placement precedent (golden frames govern; the sibling banners are section-header-row analogues), the `Opening n/N…` band renders at the section-header row too — flagged as a design residual settled at the 6-11 visual gate.
> The progress-fold + receiver re-issue mirrors the `BootstrapProgressMsg` arm in `internal/tui/model.go` (fold into an accumulator, re-issue `m.progressReceiver`); here the accumulator is the `burstDone`/`burstTotal` pair and the receiver is `m.burstPipe.receiver()`. `spawnAckTimeout` (~8s per window, Phase-3 Task 3.5) bounds how long the pending state can last per window.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → In-picker execution model (In-burst feedback / Input-locked while pending)*; *Multi-Select Mode → Mode affordance (notice-band precedence)*.

---

## restore-host-terminal-windows-6-6 | approved

### Task 6.6: Partial-failure leave-what-opened + selection mutation

**Problem**: Once past pre-flight, a rare per-window spawn hiccup (a transient `osascript`/terminal failure → adapter `spawn-failed`, or a token that never arrives → ack timeout) can still occur; and the native adapter's defensive path can return `permission-required`. On any such non-all-confirmed result Portal must **not** tear down the windows that already opened (it does not own those host windows), **skip** the trigger's self-attach (stay in the picker), **unmark** the sessions whose windows opened, and **keep** the failed/un-acked ones marked — so a second `Enter` retries exactly the missing set — while staying in multi-select mode. `permission-required` additionally surfaces the driver's permission **guidance once for the batch** (target terminal + Automation-settings hint), not the generic one-line spawn error, since the burst was stopped (every later window would hit the same per-`(source,target)` wall). Task 6.4 handled only the full-success arm.

**Solution**: Implement the non-all-confirmed arm of the `spawnCompleteMsg` handler (`internal/tui/model.go`): compute the confirmed vs failed split from `msg.Results`, mutate the selection set (`applyBurstSelectionMutation`: delete confirmed session names, keep failed/un-acked marked), skip the self-attach (no `m.selected`/`tea.Quit`), exit burst-pending, stay in multi-select mode, refresh the delegate (so `●` clears from opened rows), and set a transient flash — the driver's `Result.Guidance` (once) if any window is `permission-required`, otherwise a one-line `⚠ … failed to open — others left open` naming the failed window(s). The markers are already self-cleaned by the burst goroutine (Task 6.3, all terminal paths). No opened window is ever torn down (there is deliberately no teardown code).

**Outcome**: A burst where one window times out (or returns `spawn-failed`) among many leaves the opened windows in place, skips the trigger self-attach (the picker stays open in multi-select mode), unmarks the sessions whose windows opened, and keeps the failed/un-acked ones marked — so a second `Enter` retries exactly the still-marked missing set; a one-line flash names the failed window(s). A `permission-required` result surfaces the driver-composed permission guidance once (naming the target terminal + the Automation-settings hint), keeps the affected session marked, and — because the `spawn.Burster` already stopped the burst on permission (Phase-3 Task 3.7) — the later windows were never attempted; the grant persists so a retry proceeds.

**Do**:
- In `internal/tui/model.go`, complete the `case spawnCompleteMsg:` arm (the branch not taken by Task 6.4's `allConfirmed`):
  - **First handle `msg.Err != nil`** — a pre-spawn abort returned by `Burster.Run` (an `os.Executable` resolution failure or an ack-id generation failure, Task 3.5) that occurred *before any window opened*, so `msg.Results` is empty: `m.setFlash("⚠ could not start opening windows")` (the opaque `msg.Err` string rides only to the DEBUG log — Task 6.10 — never the user-facing flash), leave the selection **unchanged** (nothing opened → nothing to unmark), clear burst-pending (`m.burstPending = false`, nil the pipe/cancel, zero counters), stay in multi-select mode, and **return without** running the confirmed/failed computation below (there are no results to split). This is the picker analogue of the CLI's `return err` on the same `Burster.Run` error (Task 3.5), surfaced as a flash instead of an exit. The confirmed/failed logic below runs only when `msg.Err == nil`.
  - Compute `confirmed := {r.Session : r.Ack == spawn.AckConfirmed}` and `failed := {r.Session : r.Ack != spawn.AckConfirmed}` from `msg.Results`. (Un-attempted external sessions after a permission stop are neither confirmed nor in `Results` — they stay marked because they are not in `confirmed`.)
  - Selection mutation `func (m *Model) applyBurstSelectionMutation(confirmed map[string]struct{})`: delete every `confirmed` name from `m.selectedSessions`; leave every other marked session (failed, un-acked, un-attempted) marked. The **trigger** self-attach did not happen, so the trigger stays marked unless it was independently confirmed as an external window (it is not — it was never in the external set). Refresh the delegate (`applyCanvasMode`) so the `●` clears from the unmarked (opened) rows and remains on the still-marked set.
  - Skip the self-attach: do **not** set `m.selected`, do **not** return `tea.Quit`. Clear burst-pending state (`m.burstPending = false`, nil the pipe/cancel, zero counters). Stay in multi-select mode (`m.multiSelectMode` untouched).
  - Do **not** attempt to close/undo any opened window — there is no teardown seam (assert by construction: no "close window" call exists anywhere in the burst path).
  - Set the transient flash:
    - If any `r.Result.Outcome == spawn.OutcomePermissionRequired`: `m.setFlash(perm.Result.Guidance)` — the driver-composed guidance **verbatim, once** for the batch (target terminal + Automation-settings hint; opaque — general code never parses the AppleEvent codes). Keep the affected session marked (it is in `failed`, not `confirmed`).
    - Else: `m.setFlash(fmt.Sprintf("⚠ %s failed to open — others left open", quoteJoin(failedNames)))` naming every failed window (mirrors the design/CLI copy; opaque `Result.Detail` goes only to the DEBUG log — 6-10 — never the user message).
  - The flash renders through the existing `flashText` notice band (or the section-header flash placement per the abort frame — see the placement note in 6-7); the exact placement is settled at the 6-11 visual gate. The flash outranks the multi-select banner in the precedence.
- Confirm the marker self-clean already ran (Task 6.3 goroutine, all terminal paths) — no clean call needed in this arm.
- Add a test accessor to inspect the post-mutation selection (`IsSessionSelected`, `SelectedSessionCount` from Task 5.1 suffice).

**Acceptance Criteria**:
- [ ] With one external window timing out among many, the opened windows are left in place (no teardown call), the self-attach is skipped (`Selected()==""`, no `tea.Quit`), and the picker stays in multi-select mode.
- [ ] The sessions whose windows confirmed are unmarked; the failed/un-acked (and any un-attempted) ones stay marked — so a second `Enter` retries exactly the still-marked missing set.
- [ ] An adapter `spawn-failed` and an ack `timeout` both classify as failed (unmarked = confirmed only); both are named in the one-line flash.
- [ ] A `permission-required` result surfaces the driver's `Result.Guidance` verbatim once (naming the target terminal + Automation-settings hint), not the generic failed-window flash; the affected session stays marked.
- [ ] Because the `spawn.Burster` stops the burst on `permission-required` (Phase-3), the windows after the permission wall were never attempted (their sessions stay marked).
- [ ] No opened window is ever torn down (no teardown seam is called from any burst path).
- [ ] A `Burster.Run` pre-spawn error (`msg.Err != nil`, empty `Results`) surfaces a generic `⚠ could not start opening windows` flash, leaves the selection unchanged, clears burst-pending, and stays in multi-select mode — no degenerate empty-named "failed to open" message.

**Tests** (Bubble Tea model unit tests in `internal/tui`, via `spawntest.FakeAdapter`/`FakeAckChannel` scripted timeouts/failures/permission, or injected `spawnCompleteMsg`):
- `"it leaves opened windows and skips self-attach on a partial failure"`
- `"it surfaces a generic flash and leaves selection unchanged on a Burster.Run pre-spawn error"`
- `"it unmarks the confirmed sessions and keeps the failed set marked for retry"`
- `"it classifies an ack timeout and an adapter spawn-failed identically as failed"`
- `"it surfaces the permission guidance once and keeps the affected session marked"`
- `"it keeps un-attempted post-permission windows marked (burst stopped)"`
- `"it stays in multi-select mode after a partial failure"`

**Edge Cases**:
- `permission-required` stop (burst-stopped, guidance flash once for the batch naming the target terminal + Automation-settings hint, affected session stays marked).
- Retry re-opens only the still-marked missing set.
- Opened windows never torn down.
- Stays in multi-select mode.

**Context**:
> Spec *Burst & Partial-Failure Contract → Spawn, then self-attach LAST*: "**Any fails** … Portal does **not** try to close or undo the windows that already opened … It **leaves them in place** … **skips the trigger window's self-attach** so you stay in the picker, and shows a clean one-line error naming the window that failed to come up. Portal **unmarks the sessions whose windows opened and keeps the failed/un-acked ones marked**, so a second `Enter` retries exactly the missing set."
> Spec *In-picker execution model → Input-locked while pending*: "the 'retry re-opens only what's missing' guarantee rests on a well-defined selection state at completion." (Input-lock — Task 6.5 — guarantees no concurrent mutation.)
> Spec *Permissions & Error Quarantine → Defensive net (within a burst)*: "a `permission-required` result is accounted like a failed window (skip self-attach, leave opened windows in place, keep the affected session marked) **and stops the burst** … It surfaces the permission **guidance once for the batch** (naming the target terminal + the Automation-settings deep-link), not the generic one-line spawn error. The grant persists, so a retry after granting proceeds."
> The `spawn.Burster` already implements continue-through-failure (Task 3.6) and the permission early-stop (Task 3.7); `WindowResult.Ack` is `confirmed`/`timeout`/`failed`, and `WindowResult.Result.Outcome`/`.Guidance` carry the permission category + opaque guidance. This task only wires the picker's **selection mutation** + flash on top of those results — the CLI (Task 3.6/3.7) had no persistent selection to mutate; the picker does. The delegate refresh (`applyCanvasMode` propagates `Selected` — Task 5.2) clears the `●` from unmarked rows.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → Spawn, then self-attach LAST — gated on ALL N−1 confirming*; *Permissions & Error Quarantine → Defensive net (within a burst)*; *Multi-Select Mode → Mode affordance (notice-band precedence)*.

---

## restore-host-terminal-windows-6-7 | approved

### Task 6.7: Pre-flight abort UI — gone flash + prune keeping survivors

**Problem**: If pre-flight (`has-session` over every marked session on Enter) finds any marked session gone — the dominant failure, a session killed between picker-load and Enter — the burst aborts atomically: **nothing spawns, no window opens, no self-attach**. The picker must show a clean one-line error naming the gone session(s), **prune** the gone session(s) from the selection while keeping the surviving marks intact (so a second `Enter` proceeds with the survivors, not a re-abort loop), and stay in multi-select mode. The delivered frame (`design/sessions-multi-select-preflight-abort.png`) fixes the render: red `⚠ '<session>' is gone — nothing opened` at the section-header row with a right-aligned dim `esc dismiss`, the gone session's row flagged with a red `⚠` marker + red `session gone` badge, the surviving marks' violet `●` intact, and the multi-select footer unchanged. Task 6.3 left the `spawnAbortMsg` handler clearing pending only.

**Solution**: Implement the `spawnAbortMsg` handler (`internal/tui/model.go`): render the red abort banner at the section-header row (a `renderPreflightAbortHeader` variant), flag the gone rows (a transient `goneFlagged map[string]struct{}` consulted by `SessionDelegate` to draw a red `⚠` + `session gone` in place of the `●`/attached badge), prune the gone session(s) from `m.selectedSessions` keeping survivors (reuse the same prune rule as the sticky-selection preview round-trip — Task 5.6), clear burst-pending, and stay in multi-select mode. Zero windows opened → no leave-what-opened flash.

**Outcome**: An N≥2 Enter where a marked session is gone opens nothing, self-attaches nothing, and shows `⚠ '<session>' is gone — nothing opened` (red) at the section-header row with a dim `esc dismiss`; the gone row is flagged with a red `⚠` marker + red `session gone` badge while the surviving marked rows keep their violet `●`; the gone session(s) are pruned from the selection and the survivors stay marked so a second `Enter` proceeds with them; the picker stays in multi-select mode with the multi-select footer; multiple gone sessions are all named. Matches the delivered abort frame.

**Do**:
- In `internal/tui/model.go`, implement `case spawnAbortMsg:` (from Task 6.3): the `msg.Gone` slice (list order, from `spawn.PreflightMissing`) drives:
  - The abort banner: set a transient abort state (reuse the `flashText`/`flashKind` mechanism with a red/warning kind, or a dedicated `abortBannerText` field) rendered at the section-header row via `func renderPreflightAbortHeader(message string, width int, mode theme.Mode, colourless bool) string` in `section_header.go` — a red `⚠` (`theme.MV.StateRed` — the existing error token) + `fmt.Sprintf("%s %s gone — nothing opened", quoteJoin(msg.Gone), goneVerb(len(msg.Gone)))`, right-anchored dim `esc dismiss` (`theme.MV.TextDetail`), through `renderSectionHeaderRow`. Add the tiny count-aware verb helper `func goneVerb(n int) string { if n == 1 { return "is" }; return "are" }` (co-located with `quoteJoin` in the shared `internal/spawn` helper so the picker and CLI stay in lockstep), so a single gone session renders `⚠ 'fab-flowx-explore' is gone — nothing opened` — **byte-matching the delivered design copy `⚠ '<session>' is gone — nothing opened` and Task 6.7's own Outcome/Acceptance Criterion** — while several gone sessions render `⚠ 's2', 's4' are gone — nothing opened` (grammatical plural, preserving the plan's plural-safety). It sits above the multi-select banner in the section-header precedence (the transient flash/abort claimant, per the notice-band precedence).
  - Flag the gone rows: set `m.goneFlagged = set(msg.Gone)` (a transient set, cleared on dismiss/refresh). Teach `SessionDelegate` (a `GoneFlagged map[string]struct{}` field propagated in `applyCanvasMode`, like `Selected` in Task 5.2) to render, for a `SessionItem` whose name is flagged: a red `⚠` in the left-bar column (in place of the `●`/`▌`) and a red `session gone` badge in place of the attached badge. Match the frame's `fab-flowx-explore` red row.
  - Prune the selection: delete every `msg.Gone` name from `m.selectedSessions`, keeping every survivor marked — the **same prune-what's-gone rule** as the sticky-selection preview round-trip (reuse or mirror `pruneSelectionToLiveSessions` from Task 5.6, here pruning the explicit gone set rather than re-deriving from a refreshed list). Refresh the delegate so the survivors keep `●` and the pruned/gone session shows the red flag (not a `●`).
  - Clear burst-pending (`m.burstPending = false`, nil pipe/cancel, zero counters). Stay in multi-select mode.
  - Zero windows opened → no leave-what-opened flash (nothing to undo; the abort banner is the only notice).
- Dismissal: `esc` (or any actionable key) while the abort banner shows clears the abort banner + the `goneFlagged` set and stays in multi-select mode (per the frame's `esc dismiss`). Route `esc` so it dismisses the abort banner **without** exiting multi-select mode when the abort banner is active (a small precedence: abort-dismiss before the Task 5.1 mode-exit `Esc` branch). Document this Esc nuance; a subsequent `esc` with no abort banner exits mode as normal (Task 5.1).
- Do not dispatch any adapter call, connector, or self-attach on this path (the goroutine already aborted before spawning — Task 6.3).

**Acceptance Criteria**:
- [ ] A pre-flight gone session aborts atomically: zero adapter calls, zero connector calls, no `tea.Quit`, no `m.selected`.
- [ ] The section-header row renders `⚠ '<session>' is gone — nothing opened` (red) with a right-aligned dim `esc dismiss`.
- [ ] The gone session's row is flagged with a red `⚠` marker + red `session gone` badge; surviving marked rows keep their violet `●`.
- [ ] The gone session(s) are pruned from `m.selectedSessions`; every survivor stays marked (a second `Enter` proceeds with the survivors, not a re-abort).
- [ ] Multiple gone sessions are all named in the one-line message.
- [ ] The picker stays in multi-select mode with the multi-select footer; `esc` dismisses the abort banner and the gone flags without exiting mode.
- [ ] Zero windows opened → no leave-what-opened flash renders.

**Tests** (Bubble Tea model unit tests in `internal/tui`, via a `SessionExists` fake returning false for the gone name(s), or an injected `spawnAbortMsg`):
- `"it aborts atomically with no adapter, connector, or self-attach on a pre-flight gone session"`
- `"it renders the red gone banner naming the gone session with esc dismiss"`
- `"it flags the gone row with a red warning marker and session-gone badge"`
- `"it prunes the gone session and keeps the survivors marked"`
- `"it names every gone session when several are missing"`
- `"it dismisses the abort banner on esc without exiting multi-select mode"`

**Edge Cases**:
- Multiple gone sessions named.
- Zero windows opened → nothing to undo (no leave-what-opened flash).
- Survivor marks intact.
- Same prune rule as the sticky-selection preview round-trip (Task 5.6).

**Context**:
> Spec *Burst & Partial-Failure Contract → Stance: pre-flight + all-or-nothing*: "**Abort atomically** — nothing spawns, no window opens, no self-attach. Show a clean one-line error in the picker naming the gone session(s) (design copy: `⚠ '<session>' is gone — nothing opened`). **Prune the gone session(s) from the selection** … and keep the surviving marks intact, so a second `Enter` proceeds with the survivors rather than re-aborting in a loop. You stay in multi-select mode. (This is the same prune-what's-gone rule as the sticky-selection preview round-trip.) Zero windows opened → nothing to undo, no flash."
> Spec *Design References → Sessions — Multi-Select (pre-flight abort)*: "Red `⚠ '<session>' is gone — nothing opened`; the gone session flagged with a red `⚠` + `session gone`, other selections intact, the multi-select mode + footer unchanged (nothing opened). Reflects the all-or-nothing contract."
> **Placement note (design-anchored):** the delivered frame renders the abort message at the **section-header row** (red `⚠` left + dim `esc dismiss` right, no `▌` left bar) — a section-header claimant, consistent with the Task 5.3 / 6.2 decision that the golden frames govern placement (the "strict single-row collapse of flash-vs-banner" that Task 5.3 explicitly deferred to Phase 6). The abort message is the "transient error/guidance flash" claimant in the notice-band precedence, realized at the section-header row per the frame. `spawn.PreflightMissing` (Task 3.4) returns the gone sessions in list order; `pruneSelectionToLiveSessions` (Task 5.6) is the same prune rule. The delegate gone-flag propagation mirrors the `Selected` propagation from Task 5.2 (`applyCanvasMode` is the single delegate-construction chokepoint). The red token is the existing error accent — no new token.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → Stance: pre-flight + all-or-nothing*; *Trigger-Context Matrix & Open Order → Behaviour across trigger contexts (Selected session vanished)*; *Design References → Sessions — Multi-Select (pre-flight abort)*; *Multi-Select Mode → Sticky selection (the shared prune rule)*.

---

## restore-host-terminal-windows-6-8 | approved

### Task 6.8: Cancellation — Ctrl-C/Esc mid-burst

**Problem**: Because self-exec is the *last* step, cancellation is clean: `Ctrl-C`/`Esc` mid-burst must **return to the picker in multi-select mode** (not quit Portal), abort the **remaining** spawns, **leave any already-opened windows in place** (nothing is torn down), self-clean the batch markers, and mutate the selection by the same rule as a partial failure (the sessions whose windows opened are unmarked, the rest stay marked, so a retry re-opens only what's missing). `Ctrl-C` must stay live even while the picker is input-locked (Task 6.5). After the self-exec point there is nothing to cancel (already attached). Task 6.5 routed `Ctrl-C`/`Esc` while pending to a `cancelBurst` stub; this task implements it.

**Solution**: Implement `func (m Model) cancelBurst() (tea.Model, tea.Cmd)` (`internal/tui/model.go`): invoke `m.burstCancel()` (the `context.CancelFunc` from Task 6.3) so the burst goroutine abandons the remaining sequential spawns (via the `ctx.Err()` checks added to `spawn.Burster.Run` in Task 6.3), then transition to a "cancelled, awaiting the goroutine's terminal event" state — the goroutine still cleans the batch markers and sends its terminal `spawnCompleteMsg` carrying whatever confirmed so far, which the completion handler processes with the leave-what-opened selection mutation (Task 6.6). Cancellation returns to multi-select mode; Portal does not quit.

**Outcome**: `Ctrl-C`/`Esc` mid-burst cancels the goroutine's remaining spawns, leaves already-opened windows in place, self-cleans the batch markers (in the goroutine's terminal step), returns to multi-select mode (Portal stays open — not `tea.Quit`), and applies the partial-failure selection mutation (opened → unmarked, rest → stay marked). Cancelling before the first spawn opens nothing and leaves all marked; cancelling after some opened unmarks those and keeps the rest marked. `Ctrl-C` is live even while input-locked. After the self-exec (full success → quit) there is nothing to cancel.

**Do**:
- In `internal/tui/model.go` implement `func (m Model) cancelBurst() (tea.Model, tea.Cmd)`:
  - If `m.burstCancel != nil` call it (cancels the goroutine's `context.Context`, so `Burster.Run` stops before the next window and abandons the current ack poll — the `ctx.Err()` checks from Task 6.3). Do **not** `tea.Quit` — cancellation returns to the picker in multi-select mode.
  - Keep `m.burstPending` true until the goroutine's terminal `spawnCompleteMsg` lands (the goroutine still runs `cleanBatch(batch)` and sends the terminal event with the windows confirmed so far), OR transition to a lightweight "cancelling" state that swallows further input until the terminal event. The completion handler (Task 6.6) then applies the leave-what-opened selection mutation over the partial results — opened/confirmed unmarked, the rest (un-attempted, cancelled) stay marked. Return `m, m.burstPipe.receiver()` so the terminal event is still drained (no goroutine leak — the ctx-guarded `send` in Task 6.3 lets the goroutine return even if the receiver stopped).
  - Reuse the Task 6.6 selection mutation for the post-cancel result (do not duplicate) — the terminal `spawnCompleteMsg` after a cancel is handled identically to a partial failure (some confirmed, some not), so cancellation and partial-failure converge on one selection-mutation path.
- Wire the trigger (Task 6.5 placed the guard): while `m.burstPending`, `keyIsCtrlC(msg)` and `keyIsCode(msg, tea.KeyEscape)` both call `m.cancelBurst()`. Ensure `Ctrl-C` reaches this path even though the top-level `updateSessionList` normally maps `Ctrl-C`→`tea.Quit`: the burst-pending guard (Task 6.5) is placed **before** that mapping, so mid-burst `Ctrl-C` cancels rather than quits. (Outside a burst, `Ctrl-C` still quits — the guard only intercepts while pending.)
- Guarantee no window teardown on cancel (leave-what-opened): the goroutine simply stops issuing new `OpenWindow` calls; it never closes an opened window (no teardown seam exists — assert by construction).
- After self-exec (full success path, Task 6.4 already returned `tea.Quit`) `burstPending` is false and the model is exiting — there is nothing to cancel; a stray `Ctrl-C` then follows the normal quit path.

**Acceptance Criteria**:
- [ ] `Ctrl-C`/`Esc` mid-burst calls `burstCancel` (cancels the goroutine's context) and does **not** `tea.Quit` — the picker returns to multi-select mode.
- [ ] Cancelling before the first spawn opens nothing and leaves every marked session marked.
- [ ] Cancelling after some windows opened leaves those windows in place (no teardown), unmarks the opened/confirmed sessions, and keeps the rest marked (retry re-opens only the missing set).
- [ ] The batch markers are self-cleaned on the cancel path (the goroutine's `cleanBatch` runs on its terminal step).
- [ ] `Ctrl-C` is live even while the picker is input-locked (the burst-pending guard intercepts it before the normal quit mapping).
- [ ] After a full-success self-exec there is nothing to cancel (`burstPending` is false; the model is quitting).

**Tests** (Bubble Tea model unit tests in `internal/tui`, via `spawntest` fakes + fake clock, cancelling between windows):
- `"it cancels the burst and returns to multi-select mode (not quit) on Ctrl-C mid-burst"`
- `"it cancels the burst and returns to multi-select mode on Esc mid-burst"`
- `"it opens nothing and keeps all marked when cancelled before the first spawn"`
- `"it leaves opened windows and unmarks only the opened sessions when cancelled after some opened"`
- `"it self-cleans the batch markers on the cancel path"`
- `"it keeps Ctrl-C live while input-locked and cancels rather than quits"`

**Edge Cases**:
- Cancel before first spawn (nothing opened, all stay marked).
- Cancel after some opened (opened unmarked, rest stay marked).
- Ctrl-C live even while input-locked.
- After self-exec there is nothing to cancel.

**Context**:
> Spec *Burst & Partial-Failure Contract → In-picker execution model (Cancellation post-state)*: "`Ctrl-C`/`Esc` mid-burst **returns to the picker in multi-select mode** (it does not quit Portal), aborts the remaining spawns, leaves any already-opened windows in place, and self-cleans the batch markers. Selection follows the same rule as a partial failure: the sessions whose windows opened are unmarked, the rest stay marked, so a retry re-opens only what's missing."
> Spec *Burst & Partial-Failure Contract → Cancellation*: "Self-exec being the *last* step keeps cancellation clean: `Ctrl-C`/`Esc` before it aborts the remaining spawns and leaves any already-opened windows in place (nothing is torn down); after it there is nothing to cancel (already attached)."
> The cancellation rides the `context.Context` added to `spawn.Burster.Run` in Task 6.3 (the `ctx.Err()` checks between windows + in the ack poll), mirroring the ctx-guarded goroutine + `send(ctx,...)` select in `cmd/bootstrap_progress.go` (which lets the orchestrator goroutine abandon a send when the program's context is cancelled). The post-cancel selection mutation is the **same** `applyBurstSelectionMutation` path as Task 6.6 (partial failure and cancellation converge), so this task adds no second mutation rule.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → In-picker execution model (Cancellation post-state) / Cancellation*.

---

## restore-host-terminal-windows-6-9 | approved

### Task 6.9: N≥2 on unsupported/NULL — atomic no-op + re-asserted banner

**Problem**: On an unsupported/NULL terminal (remote/mosh, or a recognised-but-undriven terminal, or a transient-error identity), an N≥2 Enter cannot spawn the N−1 external windows (no adapter) — so it must be an **atomic no-op**: nothing opens, no adapter is touched, no self-attach, and the unsupported banner is **(re)asserted** naming the detected identity (the proactive banner from 6-2 stepped aside on entering multi-select mode, so it must re-appear at the N≥2 Enter block). The N=1-works vs N≥2-blocked asymmetry is intentional (only external-window spawning needs the adapter). When detection is still in-flight at Enter, the decision is deferred (Task 6.3) then resolves to this no-op if NULL. The picker stays in multi-select mode with the selection intact.

**Solution**: Implement the unsupported branch of `handleMultiSelectEnter`'s N≥2 path (and the deferred-Enter resolution in the `terminalDetectedMsg` arm from Task 6.3): when the resolved identity `IsNull()` (or the resolver returns `spawn.ResolutionUnsupported`), do nothing but re-assert the unsupported warning (a transient flash naming the detected identity, since the multi-select banner owns the section-header row in mode) and stay in multi-select mode with the selection intact. No `burstProgressPipe` is constructed, no adapter is resolved/called, no `m.selected`/`tea.Quit`.

**Outcome**: N≥2 Enter on a resolved-unsupported/NULL terminal opens nothing, touches no adapter, does not self-attach, and re-asserts the unsupported warning naming the detected identity (`⚠ unsupported terminal — <name> · <bundleID>`), staying in multi-select mode with the selection intact. An Enter fired while detection is in-flight is deferred and, on resolving to NULL/unsupported (including a transient-error identity), takes this no-op. N=1 self-attach is unaffected (needs no adapter — Task 5.7). A transient-error identity is treated identically to a clean NULL.

**Do**:
- In `internal/tui/model.go` `handleMultiSelectEnter` N≥2 branch (built in Task 6.3), the detection gate resolves three ways:
  - resolved **supported** (`!m.DetectUnsupported()`, i.e. `detectResolution` is `native` or `config`) → dispatch the burst (Task 6.3).
  - resolved **unsupported** (`m.DetectUnsupported()` — `detectResolution == ResolutionUnsupported`; this covers BOTH a NULL remote/mosh identity **and** a non-NULL recognised-but-undriven identity like Apple Terminal, and any unknown passthrough terminal with no native or config adapter) → this task's atomic no-op:
    - Construct no pipe, resolve no adapter, call no adapter method, do not set `m.selected`, do not `tea.Quit`.
    - Re-assert the unsupported warning: `m.setFlash(unsupportedFlashText(m.detectIdentity))` where the **copy branches on `IsNull()`** (mirroring CLI Task 2.7): a named identity (non-NULL — e.g. Apple Terminal) yields `⚠ unsupported terminal — <name> · <bundleID> — nothing opened`; a NULL-with-no-identity (`Name`/`BundleID` empty, remote/mosh) folds to `⚠ no host-local terminal — nothing opened`. Both flash branches carry the `— nothing opened` outcome because this flash is the response to the N≥2 Enter no-op (distinct from 6-2's *proactive banner*, a persistent state indicator that omits the suffix on both branches). (A transient flash because the multi-select banner owns the section-header row in mode.)
    - Stay in multi-select mode; leave `m.selectedSessions` intact (no prune — nothing was gone, only unsupported).
  - in-flight (`!m.detectResolved`) → **defer** (Task 6.3's `pendingBurstEnter`): when `terminalDetectedMsg` lands, the arm (Task 6.1/6.3) re-evaluates the deferred Enter and routes to the supported dispatch or this no-op depending on the resolved identity.
- In the `terminalDetectedMsg` arm (Task 6.1), if `m.pendingBurstEnter` is set: clear it and re-run the N≥2 branch decision against the now-cached **resolution** (`!DetectUnsupported()` → dispatch; `DetectUnsupported()` → this no-op). A transient-error identity resolves to `Identity{}` (NULL → unsupported), and a non-NULL undriven identity resolves to `ResolutionUnsupported`, so both take the no-op.
- Do not touch N=1 (Task 5.7's single self-attach proceeds regardless of terminal — no adapter needed).

**Acceptance Criteria**:
- [ ] N≥2 Enter on any resolved-**unsupported** terminal (`DetectUnsupported()`) — a NULL remote/mosh identity **or** a non-NULL recognised-but-undriven identity (e.g. Apple Terminal `com.apple.Terminal`) — opens nothing (no `burstProgressPipe`, no adapter resolve/call), does not self-attach (`Selected()==""`, no `tea.Quit`), and stays in multi-select mode with the selection intact.
- [ ] A non-NULL undriven identity re-asserts `⚠ unsupported terminal — <name> · <bundleID> — nothing opened`; a bare-NULL identity re-asserts `⚠ no host-local terminal — nothing opened` (both flash branches carry the `— nothing opened` outcome; copy branches on `IsNull()`, gate branches on resolution — matching CLI Task 2.7).
- [ ] An N≥2 Enter fired while detection is in-flight is deferred and, on resolving to unsupported (NULL or non-NULL-undriven), takes the atomic no-op.
- [ ] A transient-error identity (`Identity{}`, NULL) is treated identically to any other unsupported resolution (atomic no-op).
- [ ] N=1 Enter self-attaches regardless of terminal (unchanged from Task 5.7 — no adapter needed).
- [ ] The selection is unchanged (no prune) after the unsupported no-op.

**Tests** (Bubble Tea model unit tests in `internal/tui`):
- `"it is an atomic no-op on N>=2 Enter for a resolved-unsupported terminal (no adapter, no self-attach)"`
- `"it treats a non-NULL recognised-but-undriven identity (Apple Terminal) as unsupported (atomic no-op, banner re-asserted)"`
- `"it re-asserts the unsupported banner naming the detected identity on the N>=2 block"`
- `"it prints the honest no-host-local line for a bare-NULL identity"`
- `"it defers an in-flight N>=2 Enter then no-ops when it resolves NULL"`
- `"it treats a transient-error identity as unsupported"`
- `"it leaves N=1 self-attach unaffected and the selection intact"`

**Edge Cases**:
- Detection in-flight at Enter → awaited then resolves NULL → no-op.
- Transient-error identity treated as unsupported.
- N=1 self-attach unaffected (no adapter needed).
- Stays in multi-select mode with selection intact.

**Context**:
> Spec *Terminal Identity & Detection → Unsupported-terminal behaviour (banner + Enter)*: "**`Enter` with N=1** proceeds regardless of detection … opens no host window, needs no adapter. **`Enter` with N≥2** on an unsupported/NULL terminal is an **atomic no-op** — nothing opens … — and the unsupported banner is (re)asserted naming the detected identity. Same 'honest no-op' as remote/mosh. (The N=1-works vs N≥2-blocked asymmetry is intentional: only external-window spawning needs the adapter.)"
> Spec *Multi-Select Mode → Mode affordance (notice-band precedence)*: "On an unsupported terminal, entering multi-select shows the multi-select banner (the unsupported banner steps aside) and the unsupported warning re-asserts at the N≥2 Enter block."
> Spec *Terminal Identity & Detection → Detection lifecycle (In-flight at Enter / Error vs clean NULL)*: an in-flight identity is awaited then branches; a transient error folds to the unsupported/no-op path. This is the picker analogue of the CLI's atomic N≥2 unsupported gate (Task 2.7 — check precedes any adapter call). The re-asserted warning names the identity as friendly name + bundle id (the 6-2 identity string), delivered as a transient flash because the section-header row is owned by the multi-select banner while in mode.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Terminal Identity & Detection → Unsupported-terminal behaviour (banner + Enter) / Detection lifecycle*; *Multi-Select Mode → Mode affordance (notice-band precedence)*; *Spawn Architecture → Reporting & exit codes (unsupported/NULL N≥2)*.

---

## restore-host-terminal-windows-6-10 | approved

### Task 6.10: Spawn batch-summary observability from the chokepoint

**Problem**: The picker's burst must emit the `spawn`-component batch summary from its chokepoint, matching the CLI's emission (Phase 2/3) and the bootstrap/restore/daemon instrumentation shape: **one INFO cycle-summary** (`spawn: opened N/N`) + **DEBUG per-window**, using only the closed `spawn` attr keys. The count semantics are precise: `total` = N (all sessions in the batch, **including** the trigger's self-attach target); `opened` = each acked spawn plus the trigger's self-attach **when it occurs** (full success ⇒ `opened N/N` with the trigger counted; partial/permission failure ⇒ the trigger self-attach is skipped and **not** counted). An unsupported no-op logs `resolution=unsupported`. Nothing yet emits this from the TUI.

**Solution**: Bind the `spawn` component logger in `internal/tui` (injected as a `*slog.Logger` seam for `logtest.Sink` capture, defaulting to `log.For("spawn")`) and emit the batch summary from the burst completion chokepoint (the `spawnCompleteMsg`/`spawnAbortMsg`/unsupported-no-op handlers): one INFO `spawn: opened <opened>/<total>` with `batch`/`terminal`/`bundle_id`/`resolution`/`opened`/`total`, one DEBUG per external window with `session`/`ack` (+ opaque `detail` from `Result.Detail`). Count `opened` = confirmed external windows + the trigger self-attach (only on full success); `total` = N.

**Outcome**: A full-success burst emits one INFO `spawn: opened N/N` (`batch`, `terminal`, `bundle_id`, `resolution=native|config`, `opened=N`, `total=N` — the trigger self-attach counted) plus a DEBUG per external window (`session`, `ack=confirmed`); a partial/permission failure emits `opened <k>/N` where `k` counts only confirmed external windows (the skipped trigger self-attach is **not** counted); an unsupported N≥2 no-op emits `resolution=unsupported` (no per-window records). Only the closed `spawn` attr keys appear. Unit-tested via an injected `logtest.Sink`.

**Do**:
- In `internal/tui`, bind the `spawn` component logger. Add a `SpawnLogger *slog.Logger` seam to the model (injected via `tui.Deps.SpawnLogger` + `WithSpawnLogger`; default `log.For("spawn")` when nil — use `log.OrDiscard` semantics so a nil never panics). Tests inject a `logtest.Sink`-backed logger (via `logtest.NewCaptureLogger` or the shared `Sink` wrapper) to capture records. (`internal/tui` may import `internal/log`; it is not a leaf like `prefs`.)
- Emit from the completion chokepoint — a single `func (m Model) emitBurstSummary(batch string, id spawn.Identity, resolution spawn.Resolution, results []spawn.WindowResult, triggerAttached bool)` called by the `spawnCompleteMsg` full-success (Task 6.4) and partial/permission (Task 6.6) handlers, and a variant for the unsupported no-op (Task 6.9):
  - `opened := count(results, Ack==AckConfirmed)`; on full success add 1 for the trigger self-attach (`triggerAttached == true`); `total := len(m.burstExternal) + 1` (= N — the external set + the one trigger). Emit INFO `spawn: opened <opened>/<total>` with attrs `batch`, `terminal`=`id.Name`, `bundle_id`=`id.BundleID`, `resolution`=string(resolution), `opened`, `total`.
  - Per-window DEBUG: for each `WindowResult`, `spawn` DEBUG with `session`=`r.Session`, `ack`=string(r.Ack), and the opaque `detail`=`r.Result.Detail` (the driver's OS-specific string rides up as `detail`, never parsed — honours the driver-quarantine rule). The `permission-required` window's opaque `detail` also rides here; general code logs it, never interprets it.
  - Unsupported no-op (Task 6.9): emit one INFO/`spawn` outcome line with `resolution=unsupported`, `terminal`=`id.Name`, `bundle_id`=`id.BundleID`, no per-window records (nothing was attempted). Pre-flight abort (Task 6.7): emit one `spawn` outcome line naming the gone session(s) (no per-window records).
- Emit **only** the closed `spawn` attr keys: `batch`, `terminal`, `bundle_id`, `resolution`, `session`, `ack`, `opened`, `total`, `detail`. Do not invent keys at the call site (the baseline `pid`/`version`/`process_role` are injected by the handler).
- Wire `SpawnLogger` in `cmd/open.go` `buildTUIModel` to `log.For("spawn")` (production).

**Acceptance Criteria**:
- [ ] Full success emits one INFO `spawn: opened N/N` with `batch`/`terminal`/`bundle_id`/`resolution`/`opened`/`total`, where `total==N` (external + trigger) and `opened==N` (the trigger self-attach counted).
- [ ] A partial/permission failure emits `spawn: opened <k>/N` where `k` counts only confirmed external windows — the skipped trigger self-attach is **not** counted.
- [ ] Per external window, one DEBUG `spawn` record carries `session` + `ack` (+ opaque `detail`); the opaque `Result.Detail` never appears in the user-facing flash.
- [ ] An unsupported N≥2 no-op emits `resolution=unsupported` with `terminal`/`bundle_id` and no per-window records.
- [ ] No emitted `spawn` record carries any attr key outside the closed set `batch`/`terminal`/`bundle_id`/`resolution`/`session`/`ack`/`opened`/`total`/`detail`.
- [ ] `total` includes the trigger self-attach target (= N) on every path.

**Tests** (unit tests in `internal/tui` with an injected `logtest.Sink`):
- `"it emits opened N/N with the trigger self-attach counted on full success"`
- `"it emits opened k/N with the trigger not counted on a partial failure"`
- `"it emits a DEBUG per external window with session and ack (and opaque detail)"`
- `"it emits resolution=unsupported with no per-window records on an unsupported no-op"`
- `"it emits only the closed spawn attr keys"`
- `"it sets total to N (including the trigger self-attach target) on every path"`

**Edge Cases**:
- Full success → `opened N/N` (trigger self-attach counted).
- Partial/permission failure → trigger self-attach skipped and not counted.
- Unsupported no-op → `resolution=unsupported`.
- `total=N` includes the trigger self-attach target.

**Context**:
> Spec *Observability & State Footprint → Observability (`spawn` log component)*: "Closed event catalog, emitted from the spawn chokepoint: detection outcome … adapter resolution … per-window spawn + ack outcome … `permission-required` … batch summary. Emission shape matches bootstrap/restore/daemon instrumentation: **one INFO cycle-summary** (e.g. `spawn: opened 11/14`) + **DEBUG per-window**. The driver's OS-specific detail rides up as an opaque `detail` attr so the closed vocabulary stays intact."
> Spec *Observability → Attr keys (closed set) / Count semantics*: "`batch` … `terminal` … `bundle_id` … `resolution` (`config | native | unsupported`), `session`, `ack` (`confirmed | timeout | failed`), `opened` / `total` … and the opaque `detail`. **Count semantics:** `total` = **N** (all sessions in the batch, including the trigger's self-attach target); `opened` = sessions **surfaced** — each acked spawn plus the trigger's self-attach when it occurs (full success = `opened N/N`; on the failure path the trigger self-attach is skipped and not counted)."
> The CLI already emits this summary from `cmd/spawn.go` (Phase 2 Task 2.6 + enriched with `ack`/`batch` in Phase 3 Task 3.5); this task is the picker's parallel emission from the TUI chokepoint. The `spawn` component + its closed attr keys were introduced in Phase 1 Task 1.5 — no new component/attr invention. `log.For("spawn")` is the binding; `logtest.Sink` is the capture seam.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Observability & State Footprint → Observability (`spawn` log component) / Attr keys (closed set) / Count semantics*.

---

## restore-host-terminal-windows-6-11 | approved

### Task 6.11: Visual gates — capture + wire the remaining frames

**Problem**: Phase 6's UI must be verified against the delivered Paper references at the visual gate: `sessions-unsupported-terminal` (6-2) and `sessions-multi-select-preflight-abort` (6-7) each have a committed reference frame, and the in-burst `Opening n/N…` band (6-5) is a **design residual** (absent from the delivered Paper set) that must be captured fresh. The `capturetool`/`capture` harness has no fixtures that open the model in a resolved-unsupported state, a pre-flight-abort state, or a burst-pending state, and the two delivered frames have not been moved into the reference tree.

**Solution**: Add three `capture` fixtures — `sessions-unsupported-terminal`, `sessions-multi-select-preflight-abort`, and `sessions-burst-opening` — to `internal/capture/fixtures.go` (registered in `FixtureByName` + `FixtureNames`), each seeding the deterministic state through the shared `tui.Build` constructor via new capture-only seed seams (mirroring `WithInitialFlash`/`WithInitialMultiSelect` from Task 5.8). Add the `vhs` tapes + committed reference PNGs under `testdata/vhs/`, capture the NO_COLOR variants, and move the two delivered design frames into `testdata/vhs/reference/` per the visual-gate process. Dark appearance only (light deferred).

**Outcome**: `go run ./cmd/capturetool --fixture sessions-unsupported-terminal` renders the amber `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal` + blue `see docs` over the normal list (matching `design/sessions-unsupported-terminal.png`); `--fixture sessions-multi-select-preflight-abort` renders the red `⚠ '<session>' is gone — nothing opened` + `esc dismiss`, the gone row flagged red, surviving `●` marks intact, and the multi-select footer (matching `design/sessions-multi-select-preflight-abort.png`); `--fixture sessions-burst-opening` renders the `Opening 2/3…` band (the new residual frame). NO_COLOR variants render glyph-backed. `capture.FixtureNames()` lists all three; the capturetool import guard + fixture-list tests stay green. Dark appearance only.

**Do**:
- Add capture-only seed seams to `internal/tui/build.go` + `Build` (mirroring Task 5.8's `WithInitialMultiSelect`, applied before `armAppearanceDetection`; nil/zero = production default):
  - `Deps.InitialDetection *spawn.Identity` + `WithInitialDetection(id)` — seeds `detectResolved = true`, `detectIdentity = *id`, **and `detectResolution` by resolving `*id`** (e.g. via the zero-config `spawn.ResolveAdapter(*id)`, which is sufficient for the capture fixtures — a non-NULL undriven identity like Apple Terminal resolves `ResolutionUnsupported` regardless of config). Seeding `detectResolution` (not just `IsNull()`) is required so `DetectUnsupported()` is true and the proactive banner renders for the **non-NULL** Apple Terminal frame. Capture-only.
  - `Deps.InitialGoneFlagged []string` + `WithInitialGoneFlagged(names)` — seeds the pre-flight-abort state: `goneFlagged` set + the abort banner text, over an in-multi-select model with the survivors marked. Capture-only.
  - `Deps.InitialBurstOpening [2]int` (done,total) + `WithInitialBurstOpening(done, total)` — seeds `burstPending = true` + `burstDone`/`burstTotal` for the `Opening n/N…` frame. Capture-only.
- In `internal/capture/fixtures.go` add three fixtures reusing the `sessionsFlatFixture()` 12-session set (same order as Task 5.8):
  - `sessionsUnsupportedTerminalFixture()` — `initialDetection` = a **non-NULL, resolves-unsupported** identity `Name:"Apple Terminal"`, `BundleID:"com.apple.Terminal"` (the frame's identity — note it is NOT NULL; it is unsupported by *resolution*, so `WithInitialDetection` seeds `detectResolution = ResolutionUnsupported` and `DetectUnsupported()` is true, driving the banner); normal mode (no multi-select); `initialMode: prefs.ModeFlat`. Maps into `Deps.InitialDetection`.
  - `sessionsMultiSelectPreflightAbortFixture()` — `initialMultiSelect` = `{"agentic-workflows-codify","fab-flowx-explore","designlab-web-r8suyU"}` with the cursor on `fab-flowx-explore` (the frame), `initialGoneFlagged` = `{"fab-flowx-explore"}` (the gone/flagged session), survivors `agentic-workflows-codify`/`designlab-web-r8suyU` still marked. Maps into `Deps.InitialMultiSelect` + `Deps.InitialGoneFlagged`.
  - `sessionsBurstOpeningFixture()` — `initialMultiSelect` = the same three, `initialBurstOpening` = `{2,3}` (`Opening 2/3…`). Maps into `Deps.InitialBurstOpening`.
  - Register all three in `FixtureByName` (new `case`s) and add them to the sorted `FixtureNames()` slice.
- Add `testdata/vhs/` tapes + committed reference PNGs (dark appearance, mirroring `sessions-flat.tape`): `sessions-unsupported-terminal.tape`/`.png`, `sessions-multi-select-preflight-abort.tape`/`.png`, `sessions-burst-opening.tape`/`.png`, plus the NO_COLOR variant tapes (`-nocolor.tape`, inline `NO_COLOR=1`) + PNGs. **Verify a fresh write** (file hash changed) and retry before trusting each capture — the vhs harness silently fails to write PNGs (the project capture-flake note).
- Move the two delivered design frames into the reference tree (the reference-first convention): copy `design/sessions-unsupported-terminal.png` → `testdata/vhs/reference/sessions-unsupported-terminal-mv.png` and `design/sessions-multi-select-preflight-abort.png` → `testdata/vhs/reference/sessions-multi-select-preflight-abort-mv.png`, so executor + reviewer Read them and self-check the captures. The `Opening n/N…` frame has **no** delivered reference (design residual) — the fresh capture is the new reference, approved at the visual gate.
- Update the `cmd/capturetool`/`internal/capture` fixture-count/list tests (`FixtureNames`, `import_guard_test.go`, `capture_test.go`) for the three new registrations.

**Acceptance Criteria**:
- [ ] `capture.FixtureByName` returns each of `sessions-unsupported-terminal`, `sessions-multi-select-preflight-abort`, `sessions-burst-opening`, and `capture.FixtureNames()` includes all three (sorted).
- [ ] `--fixture sessions-unsupported-terminal` renders the amber `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal` + blue `see docs` over the normal list — matching `testdata/vhs/reference/sessions-unsupported-terminal-mv.png`.
- [ ] `--fixture sessions-multi-select-preflight-abort` renders the red `⚠ '<session>' is gone — nothing opened` + `esc dismiss`, the gone row flagged red (`⚠` + `session gone`), surviving `●` marks intact, multi-select footer — matching `testdata/vhs/reference/sessions-multi-select-preflight-abort-mv.png`.
- [ ] `--fixture sessions-burst-opening` renders the `Opening 2/3…` band (the new residual frame, captured fresh — no delivered reference).
- [ ] The NO_COLOR variant of each renders glyph-backed (`⚠`/`●`/text/`see docs`/`esc`) on the native bg without crashing (no hue, no canvas).
- [ ] Dark appearance only — no light-mode fixtures/tapes added (deferred per spec).
- [ ] The capturetool import guard + fixture-list tests pass with the three new fixtures.

**Tests**:
- `"it registers the three Phase-6 fixtures in FixtureByName and FixtureNames"`
- `"it builds the unsupported-terminal fixture with a resolved non-NULL unsupported identity (Apple Terminal)"`
- `"it builds the pre-flight-abort fixture with a gone-flagged session and survivors marked"`
- `"it builds the burst-opening fixture in burst-pending state with the Opening counter"`
- `"it renders each NO_COLOR variant glyph-backed without crashing"`
- Visual gate (manual, at the gate): compare each capture against its reference (`sessions-unsupported-terminal-mv.png`, `sessions-multi-select-preflight-abort-mv.png`); for the `Opening n/N…` residual, present the fresh capture for the user's visual approval. Provide the live-view commands `go run ./cmd/capturetool --fixture <name> --appearance dark`.

**Edge Cases**:
- The `Opening n/N…` frame is a new design residual (absent from the delivered Paper set) — captured fresh, no reference to compare against.
- Dark-mode only (light deferred).
- NO_COLOR glyph-backed variants.
- Move references to `testdata/vhs/reference` when wiring.

**Context**:
> Spec *Design References*: three frames were delivered and approved (`sessions-multi-select-active.png`, `sessions-multi-select-preflight-abort.png`, `sessions-unsupported-terminal.png`); "Committed PNG exports under the feature's `design/` directory are the implementation reference." (The active frame was wired in Task 5.8; this task wires the remaining two + the residual.)
> Spec *Burst & Partial-Failure Contract → In-picker execution model*: "*Design residual:* the delivered Paper set has no 'spawning / awaiting acks' frame — capturing one (or accepting a minimal counter) is a design-phase deliverable for the visual gate." So the `Opening n/N…` frame is captured fresh here.
> Spec *Design References → Tokens / Visual-gate process*: "Dark-mode; light-mode variants deferred unless requested." "re-capture fresh frames via the `capturetool` / `vhs` harness once the feature is built — moving them to `testdata/vhs/reference/` when wiring the visual gate."
> The harness never opens a real tmux server or reads real config — the fixtures inject the session set in-memory and drive the exact production model through `tui.Build`; the seed seams mirror `WithInitialFlash`/`WithInitialMultiSelect` (Task 5.8, the established pattern for capturing an otherwise-transient state). Project convention (capture flake): the vhs harness silently fails to write PNGs — verify a fresh write (file hash changed) and retry before pixel-checking.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Design References (Sessions — Unsupported terminal / Sessions — Multi-Select (pre-flight abort) / Tokens / Visual-gate process)*; *Burst & Partial-Failure Contract → In-picker execution model (Design residual)*; *Testing Strategy & DI Seams → Irreducible manual/integration residue*.
