# Plan: Skip Bootstrap When Warm

## Phases

### Phase 1: Version-stamped latch + full-bootstrap set-point
status: approved
approved_at: 2026-07-02

**Goal**: A successful full bootstrap stamps `@portal-bootstrapped` with the running binary's `cmd.version`, and the latch can be read back with correct version-aware three-way semantics. `CleanStale` (former step 11) is removed from the orchestrator, dropping it from 11 → 10 steps.

**Why this order**: The latch is the foundation both later phases consume — the entry-path branch reads it, and the abridged path is only reachable once a full run sets it. The set-point is best-effort surgery inside `Orchestrator.Run`, and it shares that same `Run` surface with the `CleanStale` removal — building them together keeps all orchestrator changes in one cohesive, testable increment. Removing `CleanStale` here is a prerequisite for re-homing it on the daemon in Phase 3 (the single-home contract).

**Acceptance**:
- [ ] A latch read/verdict helper reduces `TryGetServerOption("@portal-bootstrapped")` results — {absent, version-match, version-mismatch, read-error/down-server} — to a single `latchSatisfied bool`, satisfied only when present **and** stored version equals the running version (plain string equality, parse-free `cmd.version`).
- [ ] The running version is injectable so the version-mismatch branch is unit-testable without rebuilding the binary.
- [ ] `Orchestrator.Run` sets `@portal-bootstrapped = <version>` via `SetServerOption` as its final action — after the last soft step and the fatal-error gate, before the orchestration-complete summary and return — identically on both invocation modes (synchronous and concurrent-goroutine), and before the terminal completion event on the concurrent path.
- [ ] A run with only soft warnings (`SaverDownWarning` / `CorruptSessionsJSONWarning` / partial restore) **sets** the latch; a run that aborts at a fatal step (EnsureServer / RegisterPortalHooks / SetRestoring / ClearRestoring) leaves it **unset**.
- [ ] A latch-write failure logs WARN under the bootstrap component and is swallowed — never fatal, never appended to the returned `warnings` slice or routed through the progress channel.
- [ ] The `CleanStale` step, its `StaleCleaner` seam, and its production adapter are removed from the orchestrator; `totalSteps` becomes `10`, the package doc enumerates ten steps, and the removed `emitStep(11, …)` call is gone.
- [ ] `internal/tui/loading_progress.go` — `stepLabelTable` and `totalBootstrapSteps` become `1..10` (drop key 11, no renumber); the drift-guard test asserts `1..10` and passes; the loading bar reaches 100% on a successful full bootstrap.
- [ ] Full test suite green; existing full-bootstrap behaviour (restore, sweeps, warnings) is unchanged apart from the dropped step.

#### Tasks
status: approved
approved_at: 2026-07-02

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| skip-bootstrap-when-warm-1-1 | Latch read/verdict helper with version-aware three-way semantics | absent → not satisfied, version-mismatch → not satisfied, read-error/down-server → not satisfied, empty stored value → not satisfied, present + exact version match → satisfied |
| skip-bootstrap-when-warm-1-2 | Set the latch as the final action of a successful Orchestrator.Run | fatal-step abort leaves latch unset, soft-warning run still latches, write failure logs WARN and is swallowed (not fatal, not in warnings, not on progress channel), latch written before terminal completion event on concurrent path |
| skip-bootstrap-when-warm-1-3 | Remove the CleanStale step from the orchestrator (11 → 10 steps) | none |
| skip-bootstrap-when-warm-1-4 | Retune loading_progress.go to ten bootstrap steps | bar reaches exactly 100% after the tenth step, drift-guard table covers exactly 1..10 with no gaps |

### Phase 2: Entry-path branch + abridged bootstrap path
status: approved
approved_at: 2026-07-02

**Goal**: `PersistentPreRunE` reads the latch once and, when satisfied, takes a single abridged path — liveness-only `EnsureSaver` plus the identical context-injection and warning plumbing the synchronous path already uses — instead of running the full orchestrator; when not satisfied it routes to the unchanged full bootstrap (concurrent + loading screen on the TUI path, synchronous otherwise).

**Why this order**: Consumes the latch read semantics and the set-point from Phase 1 — the abridged path is only meaningfully reachable once a prior full run has stamped the latch. This phase delivers the feature's primary user-visible outcome (warm commands skip the full orchestrator and its concurrency surface) and must come before the daemon work, which is a corollary that only matters once warm commands stop cleaning hooks.

**Acceptance**:
- [ ] A single `TryGetServerOption` read + version compare computes the `latchSatisfied` verdict exactly once, positioned upstream of `shouldRunConcurrentBootstrap`; the verdict is never re-read.
- [ ] Latch satisfied → a new **liveness-only** `EnsureSaver` helper in package `cmd` (`SaverPanePIDOrAbsent` → `BootstrapPortalSaver` if absent) runs and returns without reaching the orchestrator or the concurrent route; it **never** calls `EnsurePortalSaverVersion` (the version-gate).
- [ ] The abridged path injects `serverStartedKey = false` and `tmuxClientKey` into `cmd.Context()`, sets **no** `deferredBootstrapKey`, and funnels a `SaverDownWarning` into the existing `bootstrapWarnings` sink (CLI → stderr; TUI → notice band) — no new emission mechanism.
- [ ] `shouldRunConcurrentBootstrap` drops its `ServerRunning()` probe and reduces to the TUI-path test only (keyed off latch-not-satisfied); a warm-unlatched `portal open` now shows the loading screen + progress, and `openTUI`'s `serverStarted` force-true stays correct (comment reworded to "full bootstrap in progress").
- [ ] Latch satisfied + saver dead → the abridged liveness probe revives it (crash-recovery regression guard); a version-mismatch latch → full re-bootstrap that re-stamps with the new version.
- [ ] The full outcome matrix holds: `open` (no args) TUI satisfied → abridged instant picker; not satisfied → concurrent + loading; `attach` / `open <path>` / CLI satisfied → abridged sync; not satisfied → synchronous full bootstrap.
- [ ] Full test suite green, including warm-path parity coverage under `IsolateStateForTest`.

#### Tasks
status: approved
approved_at: 2026-07-02

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| skip-bootstrap-when-warm-2-1 | Liveness-only EnsureSaver helper in package cmd | saver alive → no-op (no revive, no warning), saver absent → revive via BootstrapPortalSaver, revive fails → SaverDownWarning into bootstrapWarnings sink and command proceeds (non-fatal), SaverPanePIDOrAbsent transient error → treated as absent → attempt revive, never calls EnsurePortalSaverVersion (no version-gate / kill-barrier) |
| skip-bootstrap-when-warm-2-2 | Re-key shouldRunConcurrentBootstrap to latch-not-satisfied + reword openTUI comment | warm-unlatched open (no args) now shows loading screen + progress, nil client → synchronous, non-TUI command (open <path> / attach / CLI) → never concurrent regardless of latch, ServerRunning() probe dropped, openTUI serverStarted force-true stays correct on extended route (comment-only reword to "full bootstrap in progress") |
| skip-bootstrap-when-warm-2-3 | Latch-read three-way branch + abridged-path wiring in PersistentPreRunE | latch read computed once (never re-read), abridged injects serverStartedKey=false + tmuxClientKey, abridged sets NO deferredBootstrapKey (serverStarted=false survives → instant picker), CLI abridged flushes warnings to stderr / TUI abridged drains to notice band, read-error/down-server folds into not-satisfied → full bootstrap, verdict threaded into re-keyed shouldRunConcurrentBootstrap not re-read |
| skip-bootstrap-when-warm-2-4 | Integration coverage — abridged self-heal + version-mismatch re-bootstrap (real tmux) | warm+satisfied skips restore (heavy steps not run), warm+satisfied + saver dead → abridged liveness revives it, version-mismatch latch → full re-bootstrap re-stamps current version, full outcome matrix rows hold, daemon-spawning tests under IsolateStateForTest |

### Phase 3: Daemon-owned hooks cleanup
status: approved
approved_at: 2026-07-02

**Goal**: The `_portal-saver` daemon becomes the sole automatic home for the removed hooks stale-cleanup, running the existing `runHookStaleCleanup` on a throttled (~10s) gate placed on the tick loop's idle branch, so stale `hooks.json` entries are still reaped over a weeks-long warm server lifetime that no longer full-bootstraps per command.

**Why this order**: Depends on `CleanStale` being removed from the orchestrator (Phase 1) so the single-home contract holds, and completes the weeks-long-server guarantee that the abridged path (Phase 2) would otherwise break by never re-cleaning hooks. It is genuinely distinct work — a new daemon responsibility with its own concurrency/placement reasoning — so it earns its own checkpoint after the entry-path behaviour is proven.

**Acceptance**:
- [ ] `daemonDeps` carries a `*hooks.Store` built **once** at daemon startup via the existing `loadHookStore()` (resolving the *same* `hooks.json` foreground commands mutate); the existing `daemonDeps.Client` is reused as the `AllPaneLister`; `lastCleanup` is initialised to daemon-start time so the first cleanup fires ~10s after start.
- [ ] Cleanup invokes the existing `runHookStaleCleanup` with `lister=Client`, `store=`the startup store, `swallowListError=true`, `onRemoved=nil` — reusing the mass-deletion hazard guard and the `EmitCleanStaleSummary` audit breadcrumb; no new audit event or vocabulary.
- [ ] The cleanup gate sits on the **idle branch** — at the `!dirty && !gap` point, after the `@portal-restoring` check, replacing the bare idle `return` — throttled by `time.Since(lastCleanup) >= interval` (~10s default).
- [ ] Cleanup fires on an idle tick once one interval has elapsed; it is **skipped** while `@portal-restoring` is set and on capture-pending (`dirty || gap`) ticks (scrollback always wins); a cleanup error logs WARN and never escalates or crashes the daemon.
- [ ] The daemon is the **only** remaining automatic hooks-`CleanStale` caller (bootstrap no longer runs it); `portal clean` remains the manual backstop.
- [ ] Full test suite green, including the throttled-cadence unit test (no cleanup before one interval; cleanup on the first eligible idle tick) under `IsolateStateForTest` for any daemon-spawning integration coverage.

#### Tasks
status: approved
approved_at: 2026-07-02

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| skip-bootstrap-when-warm-3-1 | Wire the hooks store and lastCleanup into daemonDeps | loadHookStore() error at startup surfaces (does not silently disable cleanup), lastCleanup init to daemon-start time not zero-value so first cleanup fires ~10s in, store resolves same hooks.json foreground commands mutate via inherited PORTAL_HOOKS_FILE/XDG_CONFIG_HOME env, Client reused as AllPaneLister (no new client/seam) |
| skip-bootstrap-when-warm-3-2 | Throttled hooks-cleanup gate calling runHookStaleCleanup | not-elapsed → no cleanup call and lastCleanup unchanged, exactly-elapsed boundary (>= interval) → fires and resets lastCleanup, cleanup error logged WARN and swallowed (never returned, never crashes daemon), args pinned lister=Client / store=startup store / swallowListError=true / onRemoved=nil, reuses mass-delete guard + EmitCleanStaleSummary breadcrumb (no new audit event) |
| skip-bootstrap-when-warm-3-3 | Place the cleanup gate on the tick idle branch | @portal-restoring set → whole tick skipped, no cleanup; capture-pending (dirty \|\| gap) → capture runs, cleanup skipped this tick (scrollback always wins); idle (!dirty && !gap) + throttle elapsed → cleanup runs then returns; daemon is the only remaining automatic hooks-CleanStale caller |
| skip-bootstrap-when-warm-3-4 | Real-tmux daemon integration coverage for throttled cleanup | no cleanup before one interval elapses, stale hooks.json entry reaped after the interval on an idle server, live-keyed entry retained, daemon-spawning test under IsolateStateForTest, no t.Parallel |

### Phase 4: Analysis (Cycle 1)

Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| skip-bootstrap-when-warm-4-1 | Consolidate duplicated scaffolding in the abridged bootstrap tests | three latch-verdict tests collapse to one table-driven test with cases for latch-absent / version-mismatch / latch-read-error each asserting runner.calls == 1, shared saverAbsentReviveFailsCommander() fixture with no inline RunFunc copy left in abridged_route_test.go or abridged_saver_test.go, pure consolidation changes no asserted behaviour, go test ./cmd passes |
| skip-bootstrap-when-warm-4-2 | Decouple daemon capture startup from best-effort hooks-cleanup store resolution | loadHookStore() failure no longer returns an error from daemon RunE (tick loop starts, cleanup disabled), exactly one WARN on the disabled-cleanup path using only closed vocabulary (error attr under daemon component, no new attr/event), maybeRunHookCleanup no-ops when store nil (no panic, no lastCleanup mutation surprise), capture/commit and self-supervision probe unaffected, existing state_daemon tests green |

### Phase 5: Analysis (Cycle 2)

Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| skip-bootstrap-when-warm-5-1 | Simplify runHookStaleCleanup to its post-step-11 usage — drop the dead swallowListError axis and fix the stale contract doc | swallowListError parameter removed and the dead ListAllPanes-error return err branch deleted (path now logs the Warn and returns nil unconditionally), both production callers (state_daemon.go maybeRunHookCleanup + clean.go cleanCmd.RunE) compile against the simplified signature and behave identically, Load-error and CleanStale-error branches still propagate non-nil, contract doc rewritten to name the daemon + portal-clean callers and drop every step-11 / cleanStaleAdapter / StaleCleaner reference, hooks_cleanstale_single_caller_guard_test.go untouched, unit tests retuned (ListAllPanes error now asserts nil return + list-panes Warn), go build ./... and go test ./cmd/... green |
| skip-bootstrap-when-warm-5-2 | Log the underlying error on abridged saver revive failure to restore diagnosability parity | failed BootstrapPortalSaver on the abridged path emits exactly one bootstrap-component WARN carrying the underlying error attr before adding SaverDownWarning, uses the package-level bootstrapLogger (no new logger parameter), SaverDownWarning sink + command-proceeds-anyway no-error-return posture unchanged, successful-presence early return emits no WARN, ensureSaverLiveness Failure-posture doc paragraph updated, go build ./... and go test ./cmd/... green |
