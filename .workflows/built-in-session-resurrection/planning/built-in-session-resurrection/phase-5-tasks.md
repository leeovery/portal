---
phase: 5
phase_name: Bootstrap integration and `WaitForSessions` removal
total: 10
---

## built-in-session-resurrection-5-1 | approved

### Task 5-1: Align `skipTmuxCheck` exempt list with spec (drop `hooks`, add `state` and its subcommands)

**Problem**: Today's `cmd/root.go:13-20` `skipTmuxCheck` map is `{version, init, help, alias, clean, hooks}`. The spec's exempt list (Bootstrap Flow → "exempt commands") is `{version, init, help, alias, clean, state}` — `hooks` is deliberately removed (Phase 4 migrated hook firing into the hydrate helper; `portal hooks set` / `list` / `rm` no longer need tmux-side marker management, so they can and should go through full bootstrap), and `state` is added because every `portal state …` subcommand is either user-facing diagnostics that inspect/tear down the machinery bootstrap sets up (`status`, `cleanup`) or internal, tmux-hook-invoked subcommands that would recursively re-bootstrap (`daemon`, `notify`, `signal-hydrate`, `hydrate`, `migrate-rename`). Any internal subcommand that re-entered bootstrap would fire hooks / start `_portal-saver` / attempt restore while already mid-hook-execution — an infinite recursion hazard. Existing walker loop in `PersistentPreRunE` (`for c := cmd; c != nil; c = c.Parent()`) already matches on parent-chain names, so adding a single `state` entry covers every child subcommand transparently.

**Solution**: Replace the `skipTmuxCheck` map in `cmd/root.go` with `{"version": true, "init": true, "help": true, "alias": true, "clean": true, "state": true}`. Delete the `"hooks": true` entry. No logic changes required — the walker loop already handles nested subcommands. Add a unit test that asserts every user-facing and internal `state` subcommand walks to the exempt `state` parent and short-circuits, and that `portal hooks set/list/rm` no longer short-circuit. No changes to any other file in this task.

**Outcome**: `portal state daemon` / `notify` / `signal-hydrate` / `hydrate` / `migrate-rename` / `status` / `cleanup` all skip bootstrap (no recursion, no double hook registration, no daemon inside a daemon). `portal hooks set "foo"` now reaches `EnsureServer` / hook registration / skeleton restore / CleanStale — the full Phase 5 sequence — matching the spec's "hooks commands go through full bootstrap" intent.

**Do**:
- Edit `cmd/root.go` lines 13-20. Final map (alphabetised):
  ```go
  var skipTmuxCheck = map[string]bool{
      "alias":   true,
      "clean":   true,
      "help":    true,
      "init":    true,
      "state":   true,
      "version": true,
  }
  ```
  Delete `"hooks": true`. Add `"state": true`.
- Keep the walker-loop shape in `PersistentPreRunE` unchanged — iterating `for c := cmd; c != nil; c = c.Parent()` and returning `nil` when `skipTmuxCheck[c.Name()]` is true. This already handles nested subcommands because every direct child of `stateCmd` (registered in Phase 1 task 1-1) has `Parent().Name() == "state"`.
- Tests in `cmd/root_test.go` (extending the existing bootstrap-exempt suite):
  - For each subcommand name `{"status", "cleanup", "daemon", "notify", "signal-hydrate", "hydrate", "migrate-rename"}`, construct a cobra command with `Use: name` and parent chain `portal → state → <name>`; assert `PersistentPreRunE` returns `nil` without invoking `bootstrapDeps.Bootstrapper` (use a fake bootstrapper whose `EnsureServer` panics if called).
  - For `portal hooks set`, `portal hooks list`, `portal hooks rm`: assert `PersistentPreRunE` DOES invoke the bootstrapper (no longer short-circuits). Use the existing injected `BootstrapDeps` pattern with a recording `ServerBootstrapper`.
  - For an unknown subcommand (e.g., a cobra command with `Use: "foobar"` directly under `rootCmd`): assert bootstrap still runs (cobra itself handles the "unknown command" path; we just confirm the exempt list doesn't accidentally match). This guards against accidentally swallowing unknown commands via wildcard matching.
  - Each test sets `bootstrapDeps = &BootstrapDeps{…}` at start and calls `t.Cleanup(func(){ bootstrapDeps = nil })` per the project's DI/testing pattern (cmd package uses package-level mutable mocks; `t.Parallel()` is forbidden per `CLAUDE.md`).

**Acceptance Criteria**:
- [ ] `skipTmuxCheck` in `cmd/root.go` contains exactly `{alias, clean, help, init, state, version}` — six entries.
- [ ] `"hooks"` is NOT in `skipTmuxCheck`.
- [ ] `"state"` IS in `skipTmuxCheck`.
- [ ] `portal state status` / `cleanup` / `daemon` / `notify` / `signal-hydrate` / `hydrate` / `migrate-rename` all short-circuit `PersistentPreRunE` before any `ServerBootstrapper.EnsureServer` call.
- [ ] `portal hooks set` / `list` / `rm` all invoke `ServerBootstrapper.EnsureServer` (reach full bootstrap).
- [ ] The walker-loop walks parent chain up from the invoked subcommand — verified by a test that builds a 3-deep chain `portal → state → daemon` and asserts exemption via the `state` middle node.
- [ ] Unknown top-level subcommand (neither in `skipTmuxCheck` nor registered) still reaches bootstrap — no false-positive exemption.

**Tests**:
- `"it exempts portal state status from bootstrap"`
- `"it exempts portal state cleanup from bootstrap"`
- `"it exempts portal state daemon from bootstrap"`
- `"it exempts portal state notify from bootstrap"`
- `"it exempts portal state signal-hydrate from bootstrap"`
- `"it exempts portal state hydrate from bootstrap"`
- `"it exempts portal state migrate-rename from bootstrap"`
- `"it runs bootstrap for portal hooks set"`
- `"it runs bootstrap for portal hooks list"`
- `"it runs bootstrap for portal hooks rm"`
- `"it still runs bootstrap for unknown subcommands"`
- `"it walks the parent chain to match the state exempt at any nesting depth"`

**Edge Cases**:
- Walker-loop walks to the `state` parent regardless of nesting depth — tested explicitly for a 3-deep chain and future-proofed for any new `portal state <sub>` commands.
- The four internal subcommands (`daemon`, `notify`, `signal-hydrate`, `hydrate`) are invoked from tmux `run-shell` hooks at save/attach time; if bootstrap fired on them, every structural event would recursively register hooks / start a daemon-inside-a-daemon / re-enter `Restore`. The exemption is correctness-critical, not cosmetic.
- `portal hooks set`'s move to full bootstrap is intentional per spec "CleanStale Behavior" — now that hooks fire at hydration time, registering a new hook should not stomp the machinery that makes hooks fire. Running bootstrap once extra on `hooks set` (one-time per process invocation; cost ~8ms per Scenario 2 in the spec) is invisible.
- An unknown top-level command (cobra's own unknown-command path) must not be accidentally exempt — verified by the last test.
- Reserved tmux-running-required subcommands (if any are added later without updating `skipTmuxCheck`) will correctly reach bootstrap by default. Default behaviour is "bootstrap runs" — exemption must be opt-in by map entry.

**Context**:
> Spec "Bootstrap Flow (Integrated) → PersistentPreRunE Sequence":
> "The **exempt commands** (skip bootstrap entirely) are: `version`, `init`, `help`, `alias`, `clean`, and all `portal state ...` subcommands — both user-facing (`portal state status`, `portal state cleanup`) and internal (`portal state daemon`, `portal state notify`, `portal state signal-hydrate`, `portal state hydrate`). The internal subcommands are invoked from hooks or as pane commands and would otherwise recursively re-bootstrap; the user-facing `state` commands inspect or tear down the very machinery that bootstrap sets up, so running bootstrap first would be circular."
>
> Spec "CleanStale Behavior → Where CleanStale Runs": "Bootstrap step 7 (every non-exempt `PersistentPreRunE` invocation, post-restore). Keeps `hooks.json` consistent with live state on every Portal command that goes through bootstrap." Implies `portal hooks …` goes through bootstrap.
>
> Spec "Resume Hook Firing → What Is Deleted from the Previous Design": "`@portal-active-<pane>` volatile marker set during `portal hooks set` as a one-shot-per-server-lifetime gate. Deleted. The registration path (`portal hooks set`) becomes a pure write to `hooks.json` with no tmux-side marker management." Phase 4 task 4-6 completed this; this task removes the now-obsolete `hooks` exemption.
>
> Existing `cmd/root.go:54-58` walker: `for c := cmd; c != nil; c = c.Parent() { if skipTmuxCheck[c.Name()] { return nil } }`. Already handles nested subcommand lookup — no logic changes needed.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Bootstrap Flow (Integrated) → PersistentPreRunE Sequence", "CleanStale Behavior → Where CleanStale Runs", "Resume Hook Firing → What Is Deleted from the Previous Design".

## built-in-session-resurrection-5-2 | approved

### Task 5-2: Introduce `bootstrap.Run` orchestrator executing steps 1–8 in spec order

**Problem**: Today's `PersistentPreRunE` in `cmd/root.go:53-73` performs three steps inline (version check, `EnsureServer`, context threading). The spec's Bootstrap Flow pins **eight** steps in a specific order with a load-bearing ordering constraint (step 3 `@portal-restoring` MUST precede step 4 `_portal-saver` creation; step 7 `CleanStale` MUST follow step 6 marker-clear). Stuffing all eight into `PersistentPreRunE` inline would bloat that function beyond readability and make ordering invariants invisible to the reader. Extracting an orchestrator type makes the step order explicit, enables table-driven ordered-call tests, and gives Phase 6 a single seam to inject observability hooks (stderr emission, log surfacing) without mutating cobra-level code.

**Solution**: Create `cmd/bootstrap/bootstrap.go` defining `type Orchestrator struct { ... }` with a single entry point `func (o *Orchestrator) Run(ctx context.Context) (serverStarted bool, err error)`. The orchestrator holds injected dependencies for each step: `EnsureServer`, `RegisterHooks`, `SetRestoringMarker`, `EnsureSaver`, `Restore`, `ClearRestoringMarker`, `CleanStale`. `Run` calls them in exact spec order. Steps 1–8 map to method calls on injected collaborators — each is a trivial interface with 1-2 methods. The orchestrator is constructed once from a `*tmux.Client` via a `NewOrchestrator(client *tmux.Client) *Orchestrator` factory that wires Phase 1–4 real implementations. Tests substitute a recording `stepRecorder` that logs the call sequence for order assertions.

**Outcome**: A single type with a single `Run` method executes the complete bootstrap sequence. Ordering is enforced by implementation (step 3 sets the marker BEFORE step 4 calls `EnsureSaver`), verified by tests that record call order and assert it. The orchestrator is the sole seam between cobra and the underlying tmux/state work — Phase 6 builds on top of it (observability wrappers) without touching cobra code.

**Do**:
- Create `cmd/bootstrap/bootstrap.go`:
  - Interfaces (each 1-2 methods, satisfying Portal's DI style):
    - `type ServerBootstrapper interface { EnsureServer() (bool, error) }` — re-uses existing contract.
    - `type HookRegistrar interface { RegisterPortalHooks() error }` — wraps `internal/tmux/hooks_register.go` (Phase 1 task 1-7 + Phase 4 task 4-4).
    - `type RestoringMarker interface { Set() error; Clear() error }` — `@portal-restoring` server-option. Backed by `*tmux.Client.SetServerOption("@portal-restoring", "1")` and `UnsetServerOption("@portal-restoring")`.
    - `type SaverBootstrapper interface { EnsureSaver() error }` — Phase 2 task 2-5 + 2-6 (idempotent `_portal-saver` bootstrap with version-marker-driven restart).
    - `type Restorer interface { Restore() error }` — Phase 3 task 3-6 `Restore()` orchestrator.
    - `type StaleCleaner interface { CleanStale() error }` — existing `hooks.CleanStale` (Phase 4 task 4-7 removes its empty-panes guard).
  - `type Orchestrator struct` holding one field per interface.
  - `func NewOrchestrator(client *tmux.Client) *Orchestrator` wiring production implementations. This factory is the production-path entry; tests construct `Orchestrator` literals directly.
  - `func (o *Orchestrator) Run(ctx context.Context) (bool, error)`:
    1. `serverStarted, err := o.Server.EnsureServer()`; on error → return `false, fmt.Errorf("bootstrap step 1 (EnsureServer): %w", err)`.
    2. `if err := o.Hooks.RegisterPortalHooks(); err != nil` → return `serverStarted, fmt.Errorf("bootstrap step 2 (RegisterPortalHooks): %w", err)`.
    3. `if err := o.Restoring.Set(); err != nil` → return `serverStarted, fmt.Errorf("bootstrap step 3 (Set @portal-restoring): %w", err)`. **This MUST precede step 4** — creating `_portal-saver` fires `session-created`, which the hooks registered in step 2 would otherwise route to `portal state notify`, dirtying the flag during restore.
    4. `if err := o.Saver.EnsureSaver(); err != nil` → log best-effort, continue. Per spec "Failure Modes → `_portal-saver` creation fails at bootstrap": "retries a small number of times. On persistent failure: log, emit stderr warning, continue bootstrap without the save daemon." Emit a `*bootstrap.SaverDownError` sentinel (for Phase 6 to surface as a one-line stderr warning) but do NOT short-circuit.
    5. `restoreErr := o.Restore.Restore()`; capture — do not return yet. Per spec "Restore-Side Architecture → Restoration Trigger": "a missing or unparseable `sessions.json` is a non-fatal no-op warning" — so `Restore()` should already swallow its own per-session errors; an error surfacing here is exceptional.
    6. `if err := o.Restoring.Clear(); err != nil` → **fatal**. Per spec "Fatal Bootstrap Errors": "`@portal-restoring` set-option fails: same as `set-hook` failure" — return `serverStarted, fmt.Errorf("bootstrap step 6 (Clear @portal-restoring): %w", err)`. The clear is wrapped in a `defer` off of step 3 for safety too (so a panic between 3 and 6 still clears the marker), but the explicit-call path is the primary semantics.
    7. `if err := o.Clean.CleanStale(); err != nil` → log, continue. CleanStale failure is soft (non-critical pruning step).
    8. Return `(serverStarted, restoreErr)` — if `Restore()` errored, surface it; otherwise `nil`.
  - Document the step ordering in a leading comment block that mirrors the spec's "Bootstrap Flow → PersistentPreRunE Sequence" section. Copy the exact 1–8 numbering so a future reader can diff implementation against spec.
- Create `cmd/bootstrap/bootstrap_test.go`:
  - Build a `stepRecorder` struct implementing all interfaces and appending its method name + args to a `[]string` log on each call.
  - `"it executes steps 1 through 8 in spec order"`: construct `Orchestrator` with the recorder; call `Run`; assert the recorded log equals `["EnsureServer", "RegisterPortalHooks", "Restoring.Set", "EnsureSaver", "Restore", "Restoring.Clear", "CleanStale"]`. (Seven entries — step 8 is "return", no call.)
  - `"it propagates EnsureServer errors and skips subsequent steps"`: recorder returns error from `EnsureServer`; assert only `"EnsureServer"` is in the log.
  - `"it propagates RegisterPortalHooks errors and skips subsequent steps"`: similarly.
  - `"it propagates Restoring.Set errors and skips EnsureSaver / Restore"`: recorder errors on `Restoring.Set`; log is `["EnsureServer", "RegisterPortalHooks", "Restoring.Set"]` — critically, `EnsureSaver` is NOT called, preventing the race.
  - `"it continues past EnsureSaver failures with a SaverDownError sentinel"`: recorder errors on `EnsureSaver`; log is the full 7-entry sequence; returned error is non-nil wrapping a `bootstrap.SaverDownError`.
  - `"it clears @portal-restoring even when Restore fails"`: recorder errors on `Restore`; log still contains `"Restoring.Clear"` after `"Restore"`.
  - `"it reports Restoring.Clear failure as a fatal error"`: recorder errors on `Restoring.Clear`; `Run` returns the wrapped clear error.
  - `"it is idempotent across repeated invocations"`: call `Run` twice on the same orchestrator; second invocation's log is a full 7-entry sequence (every step runs again — bootstrap.Run itself does NOT memoise; memoisation happens at the `PersistentPreRunE` layer in task 5-3).
  - `"it returns the serverStarted flag from EnsureServer"`: recorder returns `true`; `Run` returns `(true, nil)`.
- Do NOT put cobra imports in `cmd/bootstrap/`. The package is pure Go orchestration; cobra/context wiring lives in `cmd/root.go` (task 5-3). This keeps the orchestrator reusable from tests without spinning up cobra commands.

**Acceptance Criteria**:
- [ ] `cmd/bootstrap/bootstrap.go` defines `Orchestrator`, `NewOrchestrator`, and `(o *Orchestrator) Run(ctx) (bool, error)`.
- [ ] `Run` executes 7 step calls in the order `EnsureServer → RegisterPortalHooks → Restoring.Set → EnsureSaver → Restore → Restoring.Clear → CleanStale`.
- [ ] `Restoring.Set` is called BEFORE `EnsureSaver` (ordering asserted via recorder).
- [ ] `Restoring.Clear` runs AFTER `Restore` even when `Restore` returns an error.
- [ ] `EnsureServer` failure short-circuits — no subsequent steps run.
- [ ] `RegisterPortalHooks` failure short-circuits.
- [ ] `Restoring.Set` failure short-circuits BEFORE `EnsureSaver` — verified by asserting `EnsureSaver` was NOT called when `Set` errored.
- [ ] `EnsureSaver` failure surfaces as a `*SaverDownError` sentinel AND allows `Run` to continue with the remaining steps.
- [ ] `Restoring.Clear` failure returns a fatal error wrapping the underlying tmux error.
- [ ] `CleanStale` failure logs best-effort and does NOT fail `Run` (task 5-3's wiring still completes successfully).
- [ ] `Run` returns the `serverStarted` bool from `EnsureServer` verbatim.
- [ ] `Orchestrator` contains no cobra/context.Context dependencies beyond the `ctx context.Context` parameter (no import of `github.com/spf13/cobra`).
- [ ] Package-leading comment block reproduces the spec's 1–8 step labels so implementation-vs-spec diffing is visual.

**Tests**:
- `"it executes steps 1 through 8 in spec order"`
- `"it propagates EnsureServer errors and skips subsequent steps"`
- `"it propagates RegisterPortalHooks errors and skips subsequent steps"`
- `"it propagates Restoring.Set errors and skips EnsureSaver and Restore"`
- `"it continues past EnsureSaver failures with a SaverDownError sentinel"`
- `"it clears the restoring marker even when Restore returns an error"`
- `"it reports Restoring.Clear failure as a fatal error"`
- `"it is idempotent across repeated invocations (no internal memoisation)"`
- `"it returns the serverStarted flag from EnsureServer verbatim"`
- `"it does not call EnsureSaver when Restoring.Set fails (ordering invariant)"`

**Edge Cases**:
- Step 3 MUST precede step 4. A subtle implementation bug (e.g., `defer o.Restoring.Set()` instead of calling inline) would create the exact race the spec warns about — `_portal-saver` creation fires `session-created` BEFORE the marker is set, the first daemon tick runs mid-build. The `"it does not call EnsureSaver when Restoring.Set fails"` test is the regression guard.
- `Restoring.Clear` must run even on `Restore()` error. Implemented via a deferred call off step 3 as belt-and-braces in addition to the explicit call. If both succeed (the normal path), the deferred clear on an already-absent marker is a no-op per `set-option -su` semantics.
- `EnsureSaver` failure is soft per spec "Failure Modes" (saves are paused until next bootstrap, but user is not blocked). Implementation must distinguish `SaverDownError` from fatal errors.
- `Restoring.Clear` failure is fatal per spec "Fatal Bootstrap Errors" — cannot leave the marker set because every subsequent daemon tick would skip.
- `CleanStale` failure is soft — pruning hooks is bookkeeping, not critical-path. Log and continue.
- Orchestrator does NOT memoise itself. Memoisation is the `PersistentPreRunE`-level concern (task 5-3) so that each Portal process runs bootstrap at most once. Separating the layers keeps unit tests deterministic.
- `ctx context.Context` is accepted but currently unused inside `Run` (cancellation not plumbed). It is present to let Phase 6 plumb a timeout/cancel path without signature churn.

**Context**:
> Spec "Bootstrap Flow (Integrated) → PersistentPreRunE Sequence":
> "1. `EnsureServer()` — start tmux server if not running.
> 2. Register global hooks idempotently (`set-hook -ga` with content-based check).
> 3. Set `@portal-restoring 1` as a server-level option. Must happen *before* `_portal-saver` is created, because creating `_portal-saver` fires `session-created` which triggers the dirty-flag notify path; `@portal-restoring` ensures that notify is a no-op while bootstrap is still running.
> 4. `_portal-saver` session setup (idempotent).
> 5. `Restore()` — skeleton-only restoration.
> 6. Unset `@portal-restoring`. Save loop resumes normal operation on its next tick.
> 7. `CleanStale()` — prune stale entries from `hooks.json`.
> 8. Return to the calling command."
>
> Spec "Bootstrap Flow → Ordering Rationale":
> "The critical ordering — `@portal-restoring` is set in step 3 **before** `_portal-saver` is created in step 4 — exists because step 4 fires `session-created`, which the hook pipeline would otherwise use to dirty the flag. Without `@portal-restoring` set first, the daemon's first tick could attempt to capture while the restoration is still building structure."
>
> Spec "Observability & Diagnostics → Fatal Bootstrap Errors":
> "`@portal-restoring` set-option fails: same as `set-hook` failure."
>
> Spec "Observability & Diagnostics → Proactive Health Signals":
> "`_portal-saver` cannot be created after retry attempts: `Portal save daemon failed to start — sessions won't be captured. Run 'portal state status' for details.`"
>
> Phase 1 task 1-7 `RegisterPortalHooks`, Phase 2 task 2-5/2-6 `_portal-saver` bootstrap, Phase 3 task 3-6 `Restore()`, Phase 4 task 4-7 guard-removed `CleanStale` are all production implementations this orchestrator composes.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Bootstrap Flow (Integrated) → PersistentPreRunE Sequence", "Bootstrap Flow → Ordering Rationale", "Observability & Diagnostics → Fatal Bootstrap Errors".

## built-in-session-resurrection-5-3 | approved

### Task 5-3: Wire `PersistentPreRunE` to call the orchestrator and remove inline bootstrap logic

**Problem**: Task 5-2 builds the orchestrator but nothing calls it. `PersistentPreRunE` in `cmd/root.go:53-73` still invokes `CheckTmuxAvailable` + `EnsureServer` inline and threads only `serverStarted` + `*tmux.Client` into context — it knows nothing about hook registration / restore / CleanStale. This task swaps the inline logic for a single `bootstrap.Orchestrator.Run` call, adds per-process memoisation so nested cobra invocations (if any) don't re-bootstrap, and preserves the context-threading semantics other commands depend on (`tmuxClient(cmd)` panics if the client key is absent — see `cmd/bootstrap_context.go:34-42`).

**Solution**: Replace the body of `PersistentPreRunE` with: (1) the existing exempt-walker check (task 5-1), (2) lookup / construction of the shared `*tmux.Client`, (3) a once-per-process memoised call to `bootstrap.Orchestrator.Run`, (4) population of both `serverStartedKey` and `tmuxClientKey` in `cmd.Context()`. The memoisation uses a `sync.Once` at package scope that remembers the last `(serverStarted bool, err error)` outcome; the second call returns the memoised outcome without re-invoking the orchestrator. Version check (`tmux.CheckTmuxAvailable` from Phase 1 task 1-2, wired in task 1-3) stays at the top of `PersistentPreRunE` — it is memoised inside `CheckTmuxAvailable` already per task 1-3's acceptance.

**Outcome**: `portal open` / `portal list` / `portal attach` / every other non-exempt command runs the full 7-step Phase 1–4 bootstrap once per process invocation. Existing downstream `tmuxClient(cmd)` / `serverWasStarted(cmd)` context lookups keep working verbatim — no call-site churn. `bootstrapDeps` injection (`BootstrapDeps.Bootstrapper`, `BootstrapDeps.Client`) stays as the test seam; tests can additionally supply a fake `*bootstrap.Orchestrator` via a new `BootstrapDeps.Orchestrator` field.

**Do**:
- Edit `cmd/root.go`:
  - Remove the inline `tmux.CheckTmuxAvailable()` + `bootstrapper.EnsureServer()` block from `PersistentPreRunE`. Keep only: (a) the exempt-walker (task 5-1), (b) the memoised version check, (c) the orchestrator call, (d) context population.
  - Add package-level memoisation state:
    ```go
    var (
        bootstrapOnce   sync.Once
        bootstrapStarted bool
        bootstrapErr    error
    )
    ```
  - New `PersistentPreRunE` body:
    ```go
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        for c := cmd; c != nil; c = c.Parent() {
            if skipTmuxCheck[c.Name()] {
                return nil
            }
        }
        if err := tmux.CheckTmuxAvailable(); err != nil {
            return err
        }
        orchestrator, client := buildBootstrapDeps()
        bootstrapOnce.Do(func() {
            bootstrapStarted, bootstrapErr = orchestrator.Run(cmd.Context())
        })
        if bootstrapErr != nil {
            return bootstrapErr
        }
        ctx := context.WithValue(cmd.Context(), serverStartedKey, bootstrapStarted)
        if client != nil {
            ctx = context.WithValue(ctx, tmuxClientKey, client)
        }
        cmd.SetContext(ctx)
        return nil
    },
    ```
- Update `BootstrapDeps`:
  ```go
  type BootstrapDeps struct {
      Orchestrator bootstrap.Runner  // new; interface with Run(ctx) (bool, error)
      Bootstrapper ServerBootstrapper // retained for backwards-compat with existing tests; only used if Orchestrator is nil
      Client       *tmux.Client
      // Waiter field removed — task 5-4 deletes bootstrapWait entirely.
  }
  ```
  Define `type Runner interface { Run(ctx context.Context) (bool, error) }` in `cmd/bootstrap/bootstrap.go` (task 5-2 already has `*Orchestrator` satisfying this shape; this task formalises the interface for injection).
- Update `buildBootstrapDeps()` to return `(Runner, *tmux.Client)`:
  ```go
  func buildBootstrapDeps() (bootstrap.Runner, *tmux.Client) {
      if bootstrapDeps != nil {
          if bootstrapDeps.Orchestrator != nil {
              return bootstrapDeps.Orchestrator, bootstrapDeps.Client
          }
          // Legacy shim: synthesise an Orchestrator wrapping the injected Bootstrapper
          // so existing tests keep passing during the Phase 5 cutover.
          shim := bootstrap.NewShim(bootstrapDeps.Bootstrapper)
          return shim, bootstrapDeps.Client
      }
      client := tmux.NewClient(&tmux.RealCommander{})
      return bootstrap.NewOrchestrator(client), client
  }
  ```
  Add `bootstrap.NewShim(ServerBootstrapper) Runner` that satisfies `Runner` by calling only `EnsureServer` and returning `(started, err)` — used by legacy tests in the cmd package that still set `BootstrapDeps.Bootstrapper` without providing a full orchestrator. This shim keeps the existing `root_test.go` / `bootstrap_context_test.go` fixtures working through the Phase 5 cutover; Phase 6 can delete the shim once every cmd-package test has been migrated.
- Add a test helper `resetBootstrapOnce(t *testing.T)` in `cmd/root_test.go`:
  ```go
  func resetBootstrapOnce(t *testing.T) {
      t.Helper()
      bootstrapOnce = sync.Once{}
      bootstrapStarted = false
      bootstrapErr = nil
      t.Cleanup(func() {
          bootstrapOnce = sync.Once{}
          bootstrapStarted = false
          bootstrapErr = nil
      })
  }
  ```
  Every test that exercises `PersistentPreRunE` must call `resetBootstrapOnce(t)` at the top. This matches Portal's package-level mutable-state testing pattern (`CLAUDE.md` forbids `t.Parallel()` in the cmd package specifically because of this).
- Tests in `cmd/root_test.go`:
  - `"it calls the orchestrator exactly once across repeated PersistentPreRunE invocations"`: inject a counting orchestrator; invoke `PersistentPreRunE` three times with three different cobra commands; assert orchestrator call count == 1.
  - `"it populates serverStartedKey in context with the orchestrator's return value"`: orchestrator returns `(true, nil)`; after `PersistentPreRunE`, `serverWasStarted(cmd) == true`.
  - `"it populates tmuxClientKey when a client is injected"`: verify `tmuxClient(cmd)` returns the injected client.
  - `"it returns the orchestrator error and does NOT populate context on failure"`: orchestrator returns `(_, errFatal)`; `PersistentPreRunE` returns `errFatal`; downstream `tmuxClient(cmd)` panics (context not populated).
  - `"it does NOT invoke the orchestrator for exempt commands"`: using `portal state status` fixture, assert orchestrator call count == 0 (exempt walker short-circuits before orchestrator).
  - `"it returns the tmux version-check error before invoking the orchestrator"`: inject a `CheckTmuxAvailable` that errors; assert orchestrator call count == 0.

**Acceptance Criteria**:
- [ ] `cmd/root.go` `PersistentPreRunE` body no longer calls `bootstrapper.EnsureServer()` directly — it calls `orchestrator.Run(ctx)`.
- [ ] The orchestrator is invoked exactly once per Portal process via `sync.Once`-backed memoisation.
- [ ] Exempt commands (task 5-1) short-circuit BEFORE orchestrator call.
- [ ] Version check (`tmux.CheckTmuxAvailable`) runs BEFORE orchestrator call.
- [ ] `serverStartedKey` is populated with the orchestrator's `serverStarted` return value.
- [ ] `tmuxClientKey` is populated with the shared `*tmux.Client` (when client is non-nil per existing semantics).
- [ ] Orchestrator error propagates as the `PersistentPreRunE` return value.
- [ ] On orchestrator error, context is NOT populated — callers' downstream `tmuxClient(cmd)` panics fail fast instead of operating on an incoherent context.
- [ ] `BootstrapDeps.Waiter` field is removed (task 5-4 deletes the underlying type; this task removes the field so the struct compiles).
- [ ] `BootstrapDeps.Orchestrator` field is added as the primary test injection seam.
- [ ] `BootstrapDeps.Bootstrapper` field is retained with a legacy shim (`bootstrap.NewShim`) to keep existing cmd-package tests passing; marked for removal in Phase 6.
- [ ] `resetBootstrapOnce(t)` helper is provided and called by every test that exercises `PersistentPreRunE` (guards against cross-test memoisation leakage).
- [ ] `tmux.CheckTmuxAvailable` memoisation (Phase 1 task 1-3) continues to work — repeated `PersistentPreRunE` invocations run the tmux-version check at most once.

**Tests**:
- `"it calls the orchestrator exactly once across repeated PersistentPreRunE invocations"`
- `"it populates serverStartedKey in context with the orchestrator's return value"`
- `"it populates tmuxClientKey when a client is injected"`
- `"it returns the orchestrator error and does not populate context on failure"`
- `"it does not invoke the orchestrator for exempt commands"`
- `"it returns the tmux version-check error before invoking the orchestrator"`
- `"it preserves the existing serverWasStarted / tmuxClient context lookup contract"`
- `"it uses the Orchestrator injection when both Orchestrator and Bootstrapper are set on BootstrapDeps"`
- `"it falls back to the Bootstrapper shim when only Bootstrapper is set (legacy test compat)"`

**Edge Cases**:
- `sync.Once` memoisation means a test that injects orchestrator A, runs `PersistentPreRunE`, then changes `bootstrapDeps.Orchestrator = B` and runs again would see A's cached outcome. Test helper `resetBootstrapOnce(t)` MUST be called between swaps. Document in helper godoc.
- Memoised orchestrator error persists for the remainder of the process — every subsequent `PersistentPreRunE` returns the same error. This matches the spec's fatal-error semantics: a dead bootstrap stays dead for the process lifetime.
- Context is deliberately NOT populated on orchestrator failure. `tmuxClient(cmd)`'s panic ("no client in context") surfaces the programming error if any non-`PersistentPreRunE` code path tries to recover from a bootstrap failure — fail fast beats silent incorrect behaviour.
- `BootstrapDeps.Bootstrapper` + `bootstrap.NewShim` is a bridge, not a permanent API. Phase 6 deletes the shim once every cmd test is migrated to the full `Orchestrator` seam. Marked in the field's godoc comment.
- `bootstrapOnce` reset between tests is the cmd package's reason for `t.Parallel()` being forbidden — add a comment pointing at `CLAUDE.md`'s testing guidance.
- The orchestrator's `ctx context.Context` parameter is currently ignored inside `Run`. Phase 6 may wire cancellation; this task pipes `cmd.Context()` through unchanged.

**Context**:
> Spec "Bootstrap Flow → Ordering Rationale": the orchestrator from task 5-2 already encodes the step ordering. `PersistentPreRunE` is a thin wrapper that invokes it once per process.
>
> Spec "WaitForSessions / bootstrapWait Removal → Why":
> "`WaitForSessions` and `bootstrapWait` existed because Portal had no control over *when* resurrect/continuum would finish populating sessions after startup. ... Under the new design, Portal owns restoration directly. When `Restore()` returns, skeleton-restored sessions exist because Portal just created them synchronously. There is nothing to wait for."
>
> Spec "WaitForSessions / bootstrapWait Removal → What Stays":
> "`EnsureServer()` — keeps its job of starting the tmux server if not running. `serverStarted` flag in `cmd.Context()` — still used by other call sites (e.g., decision whether to show bootstrap messages)."
>
> Existing `cmd/root.go:53-73` `PersistentPreRunE` shape and existing `cmd/bootstrap_context.go:12-42` context key contract — preserved end-to-end through this refactor.
>
> Phase 1 task 1-3 memoised `CheckTmuxAvailable` — its first-call check is the authoritative version guard. This task does not duplicate that memoisation; it relies on it.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Bootstrap Flow (Integrated) → PersistentPreRunE Sequence", "WaitForSessions / bootstrapWait Removal", "Observability & Diagnostics → Fatal Bootstrap Errors".

## built-in-session-resurrection-5-4 | approved

### Task 5-4: Delete `internal/tmux/wait.go` + `wait_test.go` and `cmd/bootstrap_wait.go` + `bootstrap_wait_test.go`

**Problem**: Phase 5's new bootstrap sequence (tasks 5-2, 5-3) replaces the old "start tmux, then poll for sessions to show up" dance with Portal-owned synchronous `Restore()`. Nothing needs to wait: when `Restore()` returns, skeleton sessions exist because Portal just created them. The poll-for-sessions code (`internal/tmux/wait.go` defining `WaitConfig` / `WaitForSessions` / `DefaultMinWait` / `DefaultMaxWait` / `DefaultPollInterval` / `DefaultWaitConfig`; `cmd/bootstrap_wait.go` defining `bootstrapWait(cmd)` / `buildWaiter(cmd)`) is dead code once task 5-5 removes its call sites. Leaving it in the tree invites future regressions. This task deletes it wholesale and updates `BootstrapDeps` to remove the now-orphan `Waiter` field.

**Solution**: Delete `internal/tmux/wait.go`, `internal/tmux/wait_test.go`, `cmd/bootstrap_wait.go`, and `cmd/bootstrap_wait_test.go`. Remove `BootstrapDeps.Waiter` from `cmd/root.go` (task 5-3 already drops the field in its struct update; this task is the corresponding package-wide file removal). Verify `go build ./...` succeeds — any stray import of `tmux.WaitForSessions` or `tmux.DefaultMinWait` / `DefaultMaxWait` / `DefaultPollInterval` / `WaitConfig` / `DefaultWaitConfig` surfaces as a compiler error at build time. This task's sole job is deletion + compile-clean verification.

**Outcome**: Zero references to `WaitForSessions`, `WaitConfig`, `DefaultWaitConfig`, `DefaultMinWait`, `DefaultMaxWait`, `DefaultPollInterval`, `bootstrapWait`, or `buildWaiter` remain in the repo. `go build ./...` passes. Test suite still passes (task 5-5 removed the call sites; task 5-6 removed the TUI's usage of `tmux.DefaultPollInterval`).

**Do**:
- Delete files:
  - `internal/tmux/wait.go`
  - `internal/tmux/wait_test.go` (if present; `Grep` the repo to confirm)
  - `cmd/bootstrap_wait.go`
  - `cmd/bootstrap_wait_test.go`
- Update `cmd/root.go` `BootstrapDeps` struct: remove the `Waiter func()` field. (Task 5-3 covers this in the same commit or as a prerequisite ordering; this task is the deletion-half that removes the field's referent.)
- Run `go build ./...` and fix any unresolved imports. Expected resolution path: task 5-5 removed `bootstrapWait(cmd)` call sites; task 5-6 removed `tmux.DefaultMinWait` / `DefaultMaxWait` / `DefaultPollInterval` references in the TUI. If any other call site remains, surface it — it indicates a missed deletion in the precursor tasks.
- Run `go test ./...` and confirm all tests pass. Any test referencing `MinWaitElapsedMsg` / `MaxWaitElapsedMsg` / `pollSessionsCmd` is task 5-6's concern (TUI model tests); any test referencing `BootstrapDeps{Waiter: ...}` is task 5-3's concern.
- `Grep` the entire repo for residual references after deletion:
  - `rg -n 'WaitForSessions|WaitConfig|DefaultMinWait|DefaultMaxWait|DefaultPollInterval|DefaultWaitConfig|bootstrapWait\b|buildWaiter'` — expected zero matches.
  - `rg -n 'BootstrapDeps\{[^}]*Waiter'` — expected zero matches.
  - Document results in the commit message for reviewer sanity.
- Commit as a single deletion commit with a message tying to tasks 5-2 / 5-3 / 5-5 / 5-6.

**Acceptance Criteria**:
- [ ] `internal/tmux/wait.go` no longer exists.
- [ ] `internal/tmux/wait_test.go` no longer exists (if it existed).
- [ ] `cmd/bootstrap_wait.go` no longer exists.
- [ ] `cmd/bootstrap_wait_test.go` no longer exists.
- [ ] `WaitForSessions`, `WaitConfig`, `DefaultWaitConfig`, `DefaultMinWait`, `DefaultMaxWait`, `DefaultPollInterval`, `bootstrapWait`, `buildWaiter` symbols are absent from the codebase — verified by a repo-wide `grep`.
- [ ] `BootstrapDeps.Waiter` field is absent.
- [ ] `go build ./...` succeeds with zero errors.
- [ ] `go test ./...` passes end-to-end.
- [ ] The commit is deletion-only — no new code is added by this task beyond the `BootstrapDeps` field removal already attributed to task 5-3.

**Tests**:
- `"go build ./... succeeds with zero errors"` (automated by CI's build step; this task adds no new tests)
- `"go test ./... passes end-to-end"` (existing test suite)
- `"repo-wide grep for WaitForSessions returns zero matches"` (manual verification in the commit message; no new Go test)
- `"repo-wide grep for bootstrapWait returns zero matches"` (manual verification)

**Edge Cases**:
- If `Grep` finds a stray reference after file deletion, task 5-5 or 5-6 missed a call site — fix the call site, don't reinstate the deleted code. The deletion is the forcing function for the cleanup.
- The `BootstrapDeps.Waiter` field removal is shared with task 5-3. Coordinate so whichever task lands first is responsible for compiling cleanly. Recommended ordering: 5-3 lands first (field removed + orchestrator wired), 5-4 lands next (file deletions no longer have compile-time holdouts).
- `cmd/bootstrap_context.go` (`serverWasStarted`, `tmuxClient` helpers and the `serverStartedKey` / `tmuxClientKey` constants) is NOT deleted — those context helpers are still used by `cmd/attach.go`, `cmd/kill.go`, `cmd/list.go`, `cmd/open.go`. Only `bootstrap_wait.go` goes.
- If `wait_test.go` did not exist in the first place (some packages ship a wait.go without a dedicated wait_test.go — depends on prior hygiene), simply skip its deletion; the `go build` + `grep` checks are the authoritative verification.

**Context**:
> Spec "WaitForSessions / bootstrapWait Removal → What Is Deleted":
> "`internal/tmux/wait.go` — where `WaitForSessions` lives. Deleted entirely. `bootstrapWait` function (in the `cmd` package). Deleted. All call sites of both functions. Deleted."
>
> Spec "WaitForSessions / bootstrapWait Removal → Replacement":
> "A single synchronous `Restore()` call in `PersistentPreRunE`, immediately after `EnsureServer()` and the hook-registration / `_portal-saver` setup steps. When it returns, live tmux state reflects every saved session that was not already live. No polling, no external dependency, deterministic timing."
>
> Spec "WaitForSessions / bootstrapWait Removal → What Stays":
> "`EnsureServer()` — keeps its job of starting the tmux server if not running. `serverStarted` flag in `cmd.Context()` — still used by other call sites. The loading page itself — retained for the TUI path, with the 1.2s minimum-display-time padding described in Bootstrap Flow."
>
> Existing `cmd/bootstrap_wait.go:14-36` is replaced; existing `internal/tmux/wait.go:1-72` is replaced. Both are fully superseded by the orchestrator from task 5-2.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "WaitForSessions / bootstrapWait Removal".

## built-in-session-resurrection-5-5 | approved

### Task 5-5: Remove `bootstrapWait(cmd)` call sites in `cmd/attach.go`, `cmd/kill.go`, `cmd/list.go`, `cmd/open.go`

**Problem**: Task 5-4 deletes `bootstrapWait`; until its four call sites are removed, the code fails to compile. The call sites are: `cmd/attach.go:30` (top of `attachCmd.RunE`), `cmd/kill.go:29` (top of `killCmd.RunE`), `cmd/list.go:52` (top of `listCmd.RunE`), and `cmd/open.go:93` (inside `openCmd.RunE` on the path-argument branch). Each call exists solely because those commands ran BEFORE Portal owned restoration — they had to block on `WaitForSessions` until resurrect/continuum rehydrated sessions. Under the new bootstrap, Phase 3's `Restore()` runs synchronously in `PersistentPreRunE` (step 5 of the orchestrator), so by the time the command's `RunE` begins, every saved session is already live. No wait is needed; no stderr "Starting tmux server..." output is needed.

**Solution**: Delete the `bootstrapWait(cmd)` call line in each of the four files. No replacement code — `PersistentPreRunE` (task 5-3) already ran bootstrap + restore. Update tests in `cmd/open_test.go` (and any sibling tests for attach/kill/list) that asserted on the `"Starting tmux server..."` stderr signal — that output is gone because `bootstrapWait` is gone. `portal list` no longer prints anything on stderr during cold-start. Names that exist only in `sessions.json` resolve via `HasSession` because skeleton restore created them.

**Outcome**: `portal attach NAME` / `portal kill NAME` / `portal list` / `portal open PATH` all reach their command bodies with live tmux state already reflecting skeleton-restored sessions. The first two rely on `HasSession(NAME)` returning true for names that lived only in `sessions.json` pre-bootstrap — because bootstrap just skeleton-restored them. `portal list` prints the combined live + restored set of sessions. `portal open PATH` resolves the path without a pre-`Resolve` blocking wait.

**Do**:
- Edit `cmd/attach.go` line 30: delete `bootstrapWait(cmd)` from `attachCmd.RunE`. The function body starts directly with `name := args[0]`.
- Edit `cmd/kill.go` line 29: delete `bootstrapWait(cmd)` from `killCmd.RunE`. The function body starts directly with `name := args[0]`.
- Edit `cmd/list.go` line 52: delete `bootstrapWait(cmd)` from `listCmd.RunE`. The function body starts directly with the `--short` / `--long` flag lookups.
- Edit `cmd/open.go` line 93: delete `bootstrapWait(cmd)` from the path-argument branch of `openCmd.RunE`. The branch now goes `destination != "" → buildQueryResolver → qr.Resolve → ...` without the intermediate wait. Do NOT delete anything from the TUI branch (`destination == ""` → `openTUIFunc(...)`); task 5-7 handles the TUI-side dismissal wiring.
- Update tests:
  - `cmd/open_test.go`: find fixtures that asserted on `"Starting tmux server..."` stderr output when `serverStarted == true`. Remove those assertions; add assertions that the stderr stream is empty on the CLI path (e.g., `if got := stderr.String(); got != "" { t.Errorf("expected empty stderr on CLI path, got %q", got) }`). Per spec "Loading-Page Minimum Display → CLI path has no loading page, no 'Restoring...' output."
  - `cmd/attach_test.go`, `cmd/kill_test.go`, `cmd/list_test.go`: sweep for similar assertions and remove/update.
  - Add a new integration fixture in `cmd/open_test.go`: a test where `sessions.json` contains session `work-abc123` but no tmux session exists at test start; call `portal attach work-abc123`; the attach succeeds because orchestrator's step 5 (Restore) skeleton-created the session before `RunE` ran. This fixture uses the `BootstrapDeps.Orchestrator` injection (task 5-3) — pass a fake orchestrator that records `Restore()` was called and populates `HasSession` accordingly.
- `Grep` the repo after edits: `rg -n 'bootstrapWait\b'` expected zero matches.

**Acceptance Criteria**:
- [ ] `cmd/attach.go`'s `RunE` no longer calls `bootstrapWait(cmd)`.
- [ ] `cmd/kill.go`'s `RunE` no longer calls `bootstrapWait(cmd)`.
- [ ] `cmd/list.go`'s `RunE` no longer calls `bootstrapWait(cmd)`.
- [ ] `cmd/open.go`'s `openCmd.RunE` path-argument branch no longer calls `bootstrapWait(cmd)`.
- [ ] `portal list` prints zero bytes to stderr on a cold start (no `"Starting tmux server..."` line).
- [ ] `portal attach NAME` where `NAME` exists only in `sessions.json` at start-of-process succeeds (skeleton-restore in step 5 of orchestrator made it live by the time `RunE` runs).
- [ ] `portal open PATH` where `PATH` resolves to a skeleton-restored session reaches `qr.Resolve` with the session already live.
- [ ] Every existing test fixture that asserted on `"Starting tmux server..."` stderr output is updated to assert empty stderr (or removed if the signal was the only thing the test verified).
- [ ] `go build ./...` succeeds.
- [ ] `go test ./...` passes.
- [ ] Repo-wide `grep` for `bootstrapWait` returns zero matches.

**Tests**:
- `"portal attach NAME resolves a name that lived only in sessions.json pre-bootstrap"`
- `"portal kill NAME resolves a name that lived only in sessions.json pre-bootstrap"`
- `"portal list prints zero bytes to stderr on cold start"`
- `"portal list prints combined live + restored sessions"`
- `"portal open PATH on a cold start reaches qr.Resolve without a pre-attach wait"`
- `"existing open_test fixtures that asserted on 'Starting tmux server...' have been updated to assert empty stderr"`

**Edge Cases**:
- Saved session name matches a LIVE session at start-of-process (rare but possible on a `portal open` that runs while tmux has live sessions): per spec "Restoration Trigger", "If a live tmux session already exists with that name → skip. User's current reality is authoritative." So the live session is not clobbered; `HasSession(NAME)` returns true for the live instance; commands operate on it.
- `sessions.json` missing / unparseable: `Restore()` no-ops (per Phase 3 task 3-6). `HasSession(NAME)` returns false for names in `sessions.json` that were never restored; `portal attach NAME` returns the existing `"No session found: %s"` error. This matches pre-Phase-5 behaviour for any name not in live tmux.
- `portal list` on a completely empty system (no live sessions, no `sessions.json`): empty stdout, empty stderr, exit 0. Unchanged from pre-Phase-5 behaviour aside from the absent `"Starting tmux server..."` line.
- `portal open` TUI branch (no path argument): deliberately NOT modified by this task. Task 5-7 handles loading-page dismissal wiring for the TUI path.
- `bootstrapWait(cmd)` removal is a pure deletion — no new code. Each call site was a single line; the fix is symmetric across the four files.

**Context**:
> Spec "WaitForSessions / bootstrapWait Removal → What Is Deleted":
> "All call sites of both functions. Deleted."
>
> Spec "Bootstrap Flow → Return-to-Caller Timing → CLI path":
> "(e.g., `portal attach NAME`, `portal hooks set ...`): bootstrap runs silently; command-specific logic runs next. For `portal attach NAME` where the target was in `sessions.json`, skeleton was restored before the attach logic runs, so `has-session -t NAME` returns true by the time the attach needs it."
>
> Spec "Loading-Page Minimum Display → CLI path has no loading page, no 'Restoring...' output":
> "Typical bootstrap is ~600ms; fast enough to not need a progress indicator. If long waits surface as user complaints, a stderr one-liner can be added later when `elapsed > 2s`. YAGNI for v1."
>
> Phase 5 acceptance: "`portal attach NAME` and `portal open` continue to resolve names that only exist in `sessions.json` at bootstrap time (skeleton is created before the command's own attach logic runs)." This task is the call-site half of that guarantee; task 5-10 is the integration-test half.
>
> Existing call sites: `cmd/attach.go:30`, `cmd/kill.go:29`, `cmd/list.go:52`, `cmd/open.go:93`.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "WaitForSessions / bootstrapWait Removal", "Bootstrap Flow → Return-to-Caller Timing → CLI path", "Loading-Page Minimum Display".

## built-in-session-resurrection-5-6 | approved

### Task 5-6: Strip TUI's session-polling loading state machine and replace with 1.2s minimum-display pad

**Problem**: `internal/tui/model.go`'s loading page was built around resurrect/continuum's unpredictable completion time. It polls `ListSessions` on a 500ms ticker until sessions appear or `DefaultMaxWait` (6s) elapses, and it emits two tick commands (`MinWaitElapsedMsg` at 1s, `MaxWaitElapsedMsg` at 6s) to gate the transition off `PageLoading`. Under the new bootstrap, `Restore()` runs synchronously BEFORE the TUI launches (orchestrator step 5, task 5-2/5-3) — by the time the TUI's `Init` runs, live tmux reflects the final post-restore state. There is nothing to poll for. The only remaining UI concern is the spec's **minimum 1.2-second display** so the loading page doesn't flash as a UI glitch. Task 5-4 deletes `tmux.DefaultMinWait` / `DefaultMaxWait` / `DefaultPollInterval`, so the TUI must be purged of those references.

**Solution**: Delete `MinWaitElapsedMsg`, `MaxWaitElapsedMsg`, `pollSessionsCmd`, `sessionsReceived` and `minWaitDone` fields, and all their call sites in `Update`. Introduce a single new message `LoadingMinElapsedMsg struct{}` scheduled in `Init` via `tea.Tick(1200*time.Millisecond, ...)`. The `Update` handler for `LoadingMinElapsedMsg` flips a new `minElapsed bool` field; when both `minElapsed` and the orchestrator-completion signal (wiring arrives in task 5-7) are true, the model transitions to `PageSessions` (or `PageProjects`, per `evaluateDefaultPage`). `sessionsLoaded` bookkeeping on the non-loading code path is unchanged — `evaluateDefaultPage` still needs it. Update `viewLoading`'s displayed text from `"Starting tmux server..."` to `"Restoring sessions…"` (spec's "Visual treatment" notes the copy change is cosmetic and local to the TUI). No 6s hard cap — the loading page dismisses when bootstrap completes (possibly after 1.2s, possibly much later for pathological restore).

**Outcome**: TUI loading state machine is a single variable (`minElapsed`) and a single tick message (`LoadingMinElapsedMsg`). No polling, no `DefaultMinWait`/`DefaultMaxWait`/`DefaultPollInterval` imports. Fast bootstrap (< 1.2s) pads to exactly 1.2s; slow bootstrap (> 1.2s) dismisses as soon as bootstrap completes. Orchestrator-completion wiring is the next task (5-7); this task lands the TUI-side plumbing with an explicit extension point for 5-7.

**Do**:
- In `internal/tui/model.go`:
  - Delete types: `MinWaitElapsedMsg`, `MaxWaitElapsedMsg` (lines 90-94).
  - Delete fields from `Model`: `minWaitDone` (line 144), `sessionsReceived` (line 145). Keep `sessionsLoaded` (line 151) — still used by `evaluateDefaultPage`.
  - Add new field: `minElapsed bool` next to `sessionsLoaded`. Add new field: `bootstrapComplete bool` (task 5-7 sets this from the orchestrator-completion message; this task just declares it and wires it into the loading-dismissal logic).
  - Delete method: `pollSessionsCmd` (lines 574-580).
  - Delete import of `"time"` if unused after these changes (check — `tea.Tick` uses `time.Duration`, so likely still needed).
  - Delete import of `"github.com/leeovery/portal/internal/tmux"` — it was only pulled in for `DefaultMinWait` / `DefaultMaxWait` / `DefaultPollInterval`. The `SessionLister` interface already has an `import` near the top and returns `[]tmux.Session`, so the import is actually still needed. Keep it; remove only the `DefaultMinWait` / `DefaultMaxWait` / `DefaultPollInterval` call sites.
  - In `Init` (lines 607-635): remove the `minWaitTick` / `maxWaitTick` scheduling for `PageLoading`. Replace with a single `loadingPadTick := tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg { return LoadingMinElapsedMsg{} })`. Add `LoadingMinElapsedMsg struct{}` type declaration near the other message types.
  - In `Update` (lines 637-736):
    - Delete the `case MinWaitElapsedMsg:` branch (lines 687-695).
    - Delete the `case MaxWaitElapsedMsg:` branch (lines 696-701).
    - Add `case LoadingMinElapsedMsg:` branch: `m.minElapsed = true; if m.bootstrapComplete && m.activePage == PageLoading { m.transitionFromLoading() }; return m, nil`. (Task 5-7 sets `m.bootstrapComplete` via a new `BootstrapCompleteMsg` it defines.)
    - Update the `case SessionsMsg:` branch (lines 654-686): delete the `if m.activePage == PageLoading { ... }` sub-block entirely (lines 672-682). `SessionsMsg` handling for the loading page is no longer responsible for triggering dismissal — the orchestrator-completion signal (task 5-7) is. Fall-through to the existing `m.sessionsLoaded = true; m.evaluateDefaultPage()` path. The `pollSessionsCmd` retry branch goes away with this block.
  - Update `transitionFromLoading` (lines 582-589): unchanged behaviourally. It sets `m.activePage = PageSessions` and marks `m.sessionsLoaded = true` then re-evaluates. Reached now from the `LoadingMinElapsedMsg` branch (when bootstrap already complete) or from the `BootstrapCompleteMsg` branch (task 5-7) (when min already elapsed).
  - Update `viewLoading` (lines 1267-1279): change `text := "Starting tmux server..."` to `text := "Restoring sessions…"`. Unicode ellipsis is fine per project conventions (no existing emoji-avoidance rule applied to the TUI string).
- Update `internal/tui/model_test.go`:
  - Delete tests referring to `MinWaitElapsedMsg`, `MaxWaitElapsedMsg`, `sessionsReceived`, `minWaitDone`, `pollSessionsCmd`.
  - Add tests:
    - `"it schedules a single LoadingMinElapsedMsg at 1.2s from Init when on PageLoading"`: inspect the `tea.Cmd` returned by `Init()` and assert it produces `LoadingMinElapsedMsg` after the expected delay (project's existing pattern uses `tea.Tick`'s inspectable shape — see existing `MinWaitElapsedMsg` tests as the template).
    - `"it sets minElapsed on LoadingMinElapsedMsg but stays on PageLoading when bootstrap not yet complete"`.
    - `"it stays on PageLoading when bootstrap completes before minElapsed"` (task 5-7 will cover this in full; stub the `bootstrapComplete` field directly here).
    - `"it transitions off PageLoading when both minElapsed and bootstrapComplete are true"`.
    - `"it does NOT poll ListSessions on a loading tick"` — verify no retry `tea.Cmd` is emitted from the `SessionsMsg` branch.
    - `"it renders 'Restoring sessions…' as loading text"`.
    - `"an orphaned LoadingMinElapsedMsg after transition is harmless"` — mirrors the existing orphaned-message tests (lines 7602-7638).

**Acceptance Criteria**:
- [ ] `MinWaitElapsedMsg` type is deleted from `internal/tui/model.go`.
- [ ] `MaxWaitElapsedMsg` type is deleted.
- [ ] `pollSessionsCmd` method is deleted.
- [ ] `Model.sessionsReceived` field is deleted.
- [ ] `Model.minWaitDone` field is deleted.
- [ ] `Model.sessionsLoaded` field is PRESERVED (still used by `evaluateDefaultPage`).
- [ ] New `LoadingMinElapsedMsg struct{}` type is declared.
- [ ] New `Model.minElapsed bool` field is declared.
- [ ] New `Model.bootstrapComplete bool` field is declared (task 5-7 will set it via a new message; this task just declares it).
- [ ] `Init` schedules exactly one `tea.Tick` (`LoadingMinElapsedMsg` at 1200ms) when `m.activePage == PageLoading` — no `minWaitTick`, no `maxWaitTick`, no `pollSessionsCmd`.
- [ ] `Update`'s `case SessionsMsg` branch no longer triggers any `PageLoading`-specific logic — `pollSessionsCmd` retry is gone, `sessionsReceived` tracking is gone.
- [ ] `Update` handles `LoadingMinElapsedMsg` by setting `m.minElapsed = true` and transitioning off `PageLoading` iff `m.bootstrapComplete` is also true.
- [ ] `viewLoading` displays `"Restoring sessions…"` (not `"Starting tmux server..."`).
- [ ] No import of `tmux.DefaultMinWait` / `DefaultMaxWait` / `DefaultPollInterval` remains (task 5-4 deleted these; model.go must not reference them).
- [ ] No 6-second hard cap exists in the model — the loading page dismisses when bootstrap completes, no matter how long that takes.
- [ ] `go build ./...` succeeds.
- [ ] All model tests pass.

**Tests**:
- `"it schedules a single LoadingMinElapsedMsg at 1.2s from Init when on PageLoading"`
- `"it sets minElapsed on LoadingMinElapsedMsg but stays on PageLoading when bootstrap not yet complete"`
- `"it transitions off PageLoading when both minElapsed and bootstrapComplete are true"`
- `"it does not poll ListSessions on a loading tick"`
- `"it renders 'Restoring sessions…' as loading text"`
- `"an orphaned LoadingMinElapsedMsg after transition is harmless"`
- `"Init does not schedule minWait/maxWait ticks when on PageLoading"` (regression guard against old behaviour re-appearing)
- `"evaluateDefaultPage still works with sessionsLoaded tracking when not on PageLoading"` (regression guard)

**Edge Cases**:
- Bootstrap finishes in 200ms: `bootstrapComplete` flips true at t=200ms; `LoadingMinElapsedMsg` arrives at t=1200ms; transition at t=1200ms. Page padded to 1.2s, matching spec.
- Bootstrap finishes in 3s: `LoadingMinElapsedMsg` arrives at t=1200ms (minElapsed=true, bootstrapComplete still false, stay on PageLoading); `BootstrapCompleteMsg` (task 5-7) arrives at t=3s; transition at t=3s. Natural length.
- Bootstrap finishes exactly at 1.2s: whichever message arrives first flips its flag; the second message's handler sees both flags true and transitions. Deterministic.
- User presses `Ctrl+C` while on `PageLoading`: existing `case PageLoading: if keyMsg.Type == tea.KeyCtrlC { return m, tea.Quit }` handler (line 724-728) is preserved verbatim.
- Orphaned `LoadingMinElapsedMsg` after an already-completed transition (e.g., bootstrap errored and the TUI was torn down and re-entered): the `if m.activePage == PageLoading` guard in the handler makes it a no-op. Same shape as the existing `MinWaitElapsedMsg` orphan-guard.
- `bootstrapComplete` defaults to `false` on model construction. If a test constructs a model without ever sending `BootstrapCompleteMsg`, the loading page stays forever — matching spec "loading page is only kept up while bootstrap is making progress" with the caveat that the TUI owner (task 5-7) is responsible for eventually sending the message.
- `sessionsLoaded` remains true-gated by an explicit `SessionsMsg` arrival (normal non-loading code path). On the loading path, `sessionsLoaded` transitions to true inside `transitionFromLoading` — preserved for `evaluateDefaultPage`'s use.

**Context**:
> Spec "Loading-Page Minimum Display (TUI Only)":
> "Skeleton restoration typically completes in ~600ms for a heavy 10-session configuration. A loading page that flashes in and out sub-second reads as a UI glitch rather than a deliberate moment. Portal enforces a **minimum display duration of 1.2 seconds** for the loading page:
> ```
> start := time.Now()
> // show loading page
> // bootstrap steps 1-7 run
> elapsed := time.Since(start)
> if elapsed < 1200*time.Millisecond {
>     time.Sleep(1200*time.Millisecond - elapsed)
> }
> // dismiss loading page
> ```
> - If bootstrap is faster than 1.2s → padded to exactly 1.2s.
> - If bootstrap is slower than 1.2s → loading page stays until bootstrap returns.
> 1.2s is intentional: long enough to register as an intentional UX beat, short enough to not become friction."
>
> Spec "Loading-Page Minimum Display → Visual treatment":
> "Reuse Portal's existing loading page as it is today. No new visual redesign is specified. The page's displayed message text may be updated in planning to reflect restoration (e.g., 'Restoring sessions…') instead of the previous 'waiting for sessions' copy; the decision is cosmetic and local to the TUI package."
>
> Spec "Observability & Diagnostics → Fatal Bootstrap Errors":
> "TUI path: loading page never 'hangs forever.' Any unrecoverable error tears down the Bubble Tea program cleanly, emits the error, exits. The loading page is only kept up while bootstrap is making progress."
>
> Existing TUI model: `internal/tui/model.go:90-94` defines the old messages; lines 144-145 the old fields; lines 574-580 the old poller; lines 607-635 `Init`'s scheduling; lines 687-701 `Update`'s handlers; lines 1267-1279 `viewLoading`.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Loading-Page Minimum Display (TUI Only)", "Bootstrap Flow → Return-to-Caller Timing → TUI path", "Observability & Diagnostics → Fatal Bootstrap Errors".

## built-in-session-resurrection-5-7 | approved

### Task 5-7: Dismiss TUI loading page on orchestrator completion (not on `ListSessions` returning rows)

**Problem**: Task 5-6 landed the TUI-side plumbing (`minElapsed`, `bootstrapComplete` fields, `LoadingMinElapsedMsg` tick, `viewLoading` copy change) but `bootstrapComplete` is never set — the loading page hangs forever unless the test manually flips the flag. The spec says the loading page dismisses when `minElapsed && bootstrapComplete`. `bootstrapComplete` must be set when the bootstrap orchestrator (task 5-2) returns. This task wires `cmd/open.go`'s `openTUI` path to send a new `tui.BootstrapCompleteMsg` into the running `tea.Program` as soon as `PersistentPreRunE` has finished (which, by cobra's invocation order, is before `openTUI` is reached — meaning the message must be sent from inside the TUI's `Init` or an equivalent entry-point command that runs immediately after `tea.NewProgram(...).Run()` starts). On bootstrap error (orchestrator returned non-nil error via `PersistentPreRunE`), the TUI never launches — cobra's error propagation already tears down cleanly before `openTUI` is called; the loading page is never shown, so no in-TUI error teardown is needed for that specific path. Phase 6 handles stderr emission of soft / fatal warnings; this task just wires the successful-completion message.

**Solution**: Because `PersistentPreRunE` has already run bootstrap synchronously BEFORE `openTUI` is invoked, bootstrap is effectively "complete" at the moment `openTUI` enters. The simplest and correct wiring is to have the TUI emit `BootstrapCompleteMsg` from its own `Init` command immediately alongside the other Init commands — `Init` schedules `fetchSessions`, `loadProjects`, `loadingPadTick`, and (new) `bootstrapCompleteCmd := func() tea.Msg { return BootstrapCompleteMsg{} }`. This message is delivered on the next event-loop tick, before any user input. `Update` handles `BootstrapCompleteMsg` by setting `m.bootstrapComplete = true` and transitioning off `PageLoading` iff `m.minElapsed` is also true. Because `PersistentPreRunE` completes synchronously before `tea.NewProgram.Run()` starts, the orchestrator cannot fail AFTER the TUI is running — this task's wiring is safe with no race window.

**Outcome**: TUI loading page appears, stays for exactly 1.2s on a fast bootstrap (padded), stays for bootstrap duration on a slow bootstrap (natural), and dismisses cleanly in both cases. The dismissal is decoupled from `ListSessions` rows — an empty saved state still dismisses at 1.2s minimum, an error during `ListSessions` still arrives through the existing `SessionsMsg.Err` path and no longer races against loading-state bookkeeping. Phase 6 will build on the same wiring to emit buffered stderr warnings AFTER the loading page dismisses.

**Do**:
- In `internal/tui/model.go`:
  - Declare new message type near the other message types (adjacent to `LoadingMinElapsedMsg` from task 5-6):
    ```go
    // BootstrapCompleteMsg signals that PersistentPreRunE's bootstrap orchestrator finished.
    // Sent once per TUI lifetime from Init.
    type BootstrapCompleteMsg struct{}
    ```
  - In `Init` (task 5-6's modified body): when `m.activePage == PageLoading`, add a `bootstrapCompleteCmd := func() tea.Msg { return BootstrapCompleteMsg{} }` to the batch alongside `fetchSessions`, `loadProjects`, and `loadingPadTick`. The message is delivered on the first event-loop iteration.
  - In `Update`: add a new branch:
    ```go
    case BootstrapCompleteMsg:
        m.bootstrapComplete = true
        if m.activePage == PageLoading && m.minElapsed {
            m.transitionFromLoading()
        }
        return m, nil
    ```
  - In the `LoadingMinElapsedMsg` branch (added by task 5-6): make the dismissal conditional on `m.bootstrapComplete`:
    ```go
    case LoadingMinElapsedMsg:
        m.minElapsed = true
        if m.activePage == PageLoading && m.bootstrapComplete {
            m.transitionFromLoading()
        }
        return m, nil
    ```
  - (Task 5-6's acceptance already mandated the `bootstrapComplete`-gated transition; this task implements the matching `BootstrapCompleteMsg` sender.)
- In `internal/tui/model_test.go`:
  - `"it sets bootstrapComplete on BootstrapCompleteMsg but stays on PageLoading when minElapsed is false"`: construct a model with `activePage = PageLoading`; send `BootstrapCompleteMsg{}`; assert `bootstrapComplete == true` and `activePage == PageLoading`.
  - `"it transitions off PageLoading when BootstrapCompleteMsg arrives after minElapsed"`: send `LoadingMinElapsedMsg{}` then `BootstrapCompleteMsg{}`; assert transition to `PageSessions` (or `PageProjects` per `evaluateDefaultPage`).
  - `"it transitions off PageLoading when LoadingMinElapsedMsg arrives after BootstrapCompleteMsg"`: reverse order — `BootstrapCompleteMsg` first, then `LoadingMinElapsedMsg`.
  - `"it pads to 1.2s when bootstrap completes in under 1.2s"`: verify order-of-messages by inspecting the Batch returned by `Init` — `BootstrapCompleteMsg` is in the batch along with `loadingPadTick`; the pad tick is scheduled at 1.2s; thus the transition is at max(bootstrap-duration, 1.2s).
  - `"it dismisses at bootstrap duration when bootstrap completes after 1.2s"`: simulate `LoadingMinElapsedMsg` arriving first, `BootstrapCompleteMsg` second; verify transition timestamp aligns with the `BootstrapCompleteMsg` receipt.
  - `"empty saved state still dismisses at 1.2s minimum"`: model has zero sessions (`SessionsMsg{Sessions: []}`) and bootstrap completed immediately; verify loading page shows for 1.2s, then dismisses.
  - `"an orphaned BootstrapCompleteMsg after transition is harmless"`: transition off `PageLoading` first; send `BootstrapCompleteMsg{}`; assert no page change.
  - `"Init does not emit BootstrapCompleteMsg when not on PageLoading"`: a model started with `serverStarted = false` begins on `PageSessions`; assert `Init`'s batch does NOT contain a `BootstrapCompleteMsg`-producing command.
- In `cmd/open.go`:
  - No functional change required — `openTUI` already runs AFTER `PersistentPreRunE`. But add a comment block above the `tea.NewProgram(m, tea.WithAltScreen())` line (line 391) that documents the bootstrap-before-TUI ordering:
    ```go
    // PersistentPreRunE ran the bootstrap orchestrator synchronously before this
    // function was reached. The TUI's Init emits a BootstrapCompleteMsg on its
    // first tick so the loading page (if shown due to serverStarted) dismisses
    // naturally. Loading-page dismissal is gated on:
    //   (a) BootstrapCompleteMsg (always delivered from Init)
    //   (b) LoadingMinElapsedMsg (delivered after 1.2s via tea.Tick)
    // The transition happens when both (a) and (b) have been received.
    ```

**Acceptance Criteria**:
- [ ] `internal/tui/model.go` declares `BootstrapCompleteMsg struct{}` as a public message type.
- [ ] `Init`, when `m.activePage == PageLoading`, batches a `BootstrapCompleteMsg`-producing command alongside `fetchSessions`, `loadProjects`, and `loadingPadTick`.
- [ ] `Init`, when `m.activePage != PageLoading`, does NOT batch a `BootstrapCompleteMsg`-producing command (no-op on the non-loading path — irrelevant but hygienic).
- [ ] `Update`'s `BootstrapCompleteMsg` branch sets `m.bootstrapComplete = true`.
- [ ] `Update`'s `BootstrapCompleteMsg` branch triggers `transitionFromLoading` iff `m.minElapsed && m.activePage == PageLoading`.
- [ ] `Update`'s `LoadingMinElapsedMsg` branch triggers `transitionFromLoading` iff `m.bootstrapComplete && m.activePage == PageLoading` (task 5-6 + this task must agree on the gating).
- [ ] Fast bootstrap (< 1.2s): loading page dismisses at exactly 1.2s (verified by test inspecting `tea.Tick` timing).
- [ ] Slow bootstrap (> 1.2s): loading page dismisses on `BootstrapCompleteMsg` receipt (verified by message-ordering test).
- [ ] Empty saved state still dismisses at 1.2s minimum (verified by test).
- [ ] Orphaned `BootstrapCompleteMsg` after a transition is a no-op (verified by test).
- [ ] `cmd/open.go`'s `openTUI` body contains a comment block documenting the bootstrap-before-TUI ordering and the dismissal-gate semantics.
- [ ] No changes to `cmd/open.go`'s actual executable code beyond the comment — `PersistentPreRunE`'s synchronous completion before `openTUI` is the existing contract this task relies on.

**Tests**:
- `"it sets bootstrapComplete on BootstrapCompleteMsg but stays on PageLoading when minElapsed is false"`
- `"it transitions off PageLoading when BootstrapCompleteMsg arrives after minElapsed"`
- `"it transitions off PageLoading when LoadingMinElapsedMsg arrives after BootstrapCompleteMsg"`
- `"it pads to 1.2s when bootstrap completes in under 1.2s"`
- `"it dismisses at bootstrap duration when bootstrap completes after 1.2s"`
- `"empty saved state still dismisses at 1.2s minimum"`
- `"an orphaned BootstrapCompleteMsg after transition is harmless"`
- `"Init does not emit BootstrapCompleteMsg when not on PageLoading"`

**Edge Cases**:
- Both messages arrive within the same event-loop tick: Bubble Tea processes them sequentially; whichever hits `Update` first flips its flag; the second message's handler sees both flags true and transitions. Deterministic.
- Bootstrap error: `PersistentPreRunE` returned non-nil; cobra tears down before `openTUI` is called; TUI never launches; no loading page is ever shown. No in-TUI error handling needed on this path. Phase 6 surfaces the error to stderr via cobra's existing error-return mechanism.
- TUI started on `PageSessions` directly (normal case when `serverStarted == false`): `Init` returns the non-PageLoading path, `BootstrapCompleteMsg` is never emitted, loading-page machinery never runs. Matches spec "The page is only kept up while bootstrap is making progress."
- Empty saved state (`sessions.json` absent or empty): `Restore()` no-ops, `SessionsMsg{Sessions: []}` arrives normally; loading page still pads to 1.2s because `BootstrapCompleteMsg` fires from Init regardless of session count. Spec-accurate: the pad is about UI-feel, not data presence.
- Loading page shown for a very long bootstrap (e.g., 5s restore of 50 sessions): no 6s hard cap. Page stays until `BootstrapCompleteMsg` + `LoadingMinElapsedMsg` both received. Matches spec "If bootstrap is slower than 1.2s → loading page stays until bootstrap returns."
- Phase 6 will extend this seam: the `BootstrapCompleteMsg` carrier will include a `[]bootstrap.Warning` slice of soft warnings buffered during bootstrap, and the handler will emit them to stderr AFTER the loading page dismisses. This task leaves the type as `struct{}` for v1 — Phase 6's extension is a structural change, not a semantic one.

**Context**:
> Spec "Loading-Page Minimum Display (TUI Only)" — quoted in task 5-6 context block.
>
> Spec "Bootstrap Flow → Return-to-Caller Timing → TUI path":
> "bootstrap runs. Loading page shows for minimum 1.2s (padded if restoration was faster; natural if slower). TUI appears with populated picker. User selects → attach flow runs."
>
> Spec "Observability & Diagnostics → TUI interaction":
> "While the Bubble Tea loading page is active, direct stderr writes would corrupt the rendered UI. The TUI path therefore **buffers** bootstrap warnings in memory during the loading window and emits them to stderr *after* the loading page is dismissed (before the TUI picker renders, or immediately before exit on fatal error). The CLI path writes to stderr directly as described. Both paths log the same content to `portal.log` regardless of stderr behaviour."
>
> Phase 6 (out of scope for this task) will extend `BootstrapCompleteMsg` with a buffered warnings slice; for now, the type is `struct{}`.
>
> Existing `cmd/open.go:349-406` `openTUI` — called after `PersistentPreRunE` completes, confirming the bootstrap-before-TUI ordering this task's wiring depends on.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Loading-Page Minimum Display (TUI Only)", "Bootstrap Flow → Return-to-Caller Timing → TUI path", "Observability & Diagnostics → TUI interaction".

## built-in-session-resurrection-5-8 | approved

### Task 5-8: Integration test (isolated `tmux -L` socket): `@portal-restoring` suppresses captures during skeleton-restore window

**Problem**: The whole spec architecture hinges on one subtle invariant: between orchestrator step 3 (set `@portal-restoring`) and step 6 (clear `@portal-restoring`), every structural event fired by the restoration itself (`session-created`, `window-linked`, `window-layout-changed`, `pane-focus-out` as panes appear) must be a no-op on the save side — `portal state notify` touches `save.requested`, but the daemon's tick sees `@portal-restoring=1` at entry and skips the entire cycle. If this invariant ever regressed — if the order flipped, or if the daemon's tick check were removed, or if `notify` were "helpfully" made marker-aware and later reverted — the daemon's first post-bootstrap tick would capture a mid-build state and overwrite the pre-reboot `sessions.json`. That corruption is silent and catastrophic. No unit test catches it — it is an ordering / coordination invariant across the save daemon, the hook pipeline, and the restore sequence. Integration is the only validation surface.

**Solution**: Add an integration test in `cmd/bootstrap/bootstrap_integration_test.go` that spins up an **isolated** `tmux -L <unique>` socket (pattern validated in Phase 3 task 3-13), seeds a `sessions.json` with a multi-pane saved session, invokes the orchestrator end-to-end, and verifies: (a) at least one structural hook event fired during the restore window (otherwise the test is vacuous — it must assert a NON-trivial suppression happened); (b) `save.requested` may have been touched during the window but did not cause a capture (`sessions.json.saved_at` is unchanged mid-window); (c) an in-flight capture started BEFORE the marker was set is allowed to commit its pre-restore snapshot (per spec "In-Flight Capture Atomicity"); (d) the first post-clear tick captures the complete final state (not a mid-build partial). The test kills the isolated socket in `t.Cleanup` on both pass and fail so no leftover tmux servers pollute the runner.

**Outcome**: A regression guard that fires any time the marker-ordering invariant breaks. This test is the definitive home for assertions that Phase 2's `@portal-restoring`-awareness in the daemon (task 2-11, 2-12) composes correctly with Phase 5's orchestrator-enforced ordering (task 5-2).

**Do**:
- Create `cmd/bootstrap/bootstrap_integration_test.go`. Gate with a `//go:build integration` build tag (or equivalent) so `go test ./...` short path skips it; integration runs via `go test -tags=integration ./...`.
- Test scaffolding:
  - `t.TempDir()` for state directory; set `PORTAL_STATE_DIR` env var (or whatever env var Phase 2 task 2-1 introduced for the state-dir override) so Portal writes into the test dir.
  - Build portal binary once per test run (`go build -o <tempdir>/portal .` — reuse pattern from `cmd/root_integration_test.go` if it already exists).
  - Generate a unique tmux socket name: `socket := fmt.Sprintf("portal-test-%d-%d", os.Getpid(), time.Now().UnixNano())`.
  - `t.Cleanup` runs `tmux -L <socket> kill-server` (ignore error — server may already be dead).
- Seed `sessions.json`:
  - Single saved session `work` with 2 windows, 2-3 panes per window, plausible layout strings from a real `list-panes` dump. Use fixture constants at the top of the test file.
  - Use Phase 2 task 2-3's encoder to write the fixture so schema consistency is guaranteed.
- Invoke orchestrator:
  - Construct a real `tmux.Client` backed by a `RealCommander` but with `TMUX_SOCKET_NAME=<socket>` set in the command environment so every `tmux` call targets the isolated socket.
  - Construct `bootstrap.NewOrchestrator(client)` (task 5-2).
  - Instrument the orchestrator's step 3–6 with a `hookEventRecorder` that subscribes to the tmux socket via `tmux pipe-pane` or (simpler) by tailing the `portal.log` file for the `notify` entries the daemon writes (Phase 6 task landing log format; for this task, write a lightweight debug hook that dumps timestamped event names to the test's state dir — see Phase 2 task 2-2's `notify` structure). If the logging infrastructure isn't available yet at Phase 5 landing time, the test can register a PROBE hook on the isolated socket that writes each fired event into a test-temp file: `set-hook -ga session-created 'run-shell "echo session-created >> <tempfile>"'`. Since Phase 1's registration is `-ga` (append), probe hooks coexist with Portal's; Portal's entries still fire.
  - Run the orchestrator's `Run(ctx)`. Capture the elapsed time of each step via a `stepTimer` wrapper.
- Assertions:
  - **(a) Non-vacuous suppression**: the probe-file contains at least one structural event name (`session-created`, `window-linked`, `window-layout-changed`) with a timestamp between step-3-marker-set and step-6-marker-clear. If zero events fired in the window, fail the test with `"vacuous: no structural events fired during @portal-restoring window — cannot verify suppression"`. This prevents a future spec change from accidentally turning the test into a pass-by-doing-nothing.
  - **(b) No mid-window capture**: read `sessions.json.saved_at` before invoking orchestrator (if the file predates the run). Compare to `saved_at` immediately after step 6 and BEFORE the daemon's first post-clear tick completes (sleep 1100ms to be sure the daemon's 1s ticker has NOT yet fired a capture). `saved_at` must equal the pre-run value — no capture happened during the window. Use `time.Parse(time.RFC3339, ...)` to compare.
  - **(c) Permitted in-flight capture commit**: this is a negative-space assertion. The test does NOT assert no capture happened during the window at all — an in-flight capture started PRE-marker may commit mid-window. The assertion is specifically that no capture STARTED after the marker was set produced a write. Approximated by: `sessions.json.saved_at` either equals pre-run OR is stamped before step 3's marker-set timestamp. If `saved_at` is somewhere between step-3 and step-6 timestamps, fail with "capture committed during restore window".
  - **(d) First post-clear tick captures complete final state**: after step 6, sleep 1500ms (≥1s ticker + slack). Read `sessions.json`; parse. Assert: (i) `saved_at` now > step-6-clear-timestamp; (ii) the parsed sessions include the restored `work` session with both windows and all panes; (iii) `environment` map is non-empty (if the seed had env); (iv) each pane has a `scrollback_file` reference. This confirms the capture ran AFTER the marker was cleared and captured the complete post-restore state.
- Use `testing.Short()` to skip the test on short-mode runs; this test is slow (~3s) due to real sleeps.
- If Phase 2/3 infrastructure is insufficient on the Phase 5 landing commit (e.g., daemon log shape not finalised), fallback probe file + tmux hook is the documented minimal dependency.

**Acceptance Criteria**:
- [ ] New integration test lives in `cmd/bootstrap/bootstrap_integration_test.go` with `//go:build integration` tag.
- [ ] Test uses an isolated `tmux -L <unique>` socket — never touches the user's default tmux.
- [ ] Socket is killed in `t.Cleanup` on both pass and fail.
- [ ] Test seeds a multi-session, multi-pane `sessions.json` via Phase 2 task 2-3's encoder.
- [ ] Test invokes `bootstrap.Orchestrator.Run(ctx)` end-to-end with a real `tmux.Client` / `RealCommander` bound to the isolated socket.
- [ ] Test verifies at least one structural hook event fires during the `@portal-restoring` window (non-vacuous).
- [ ] Test verifies `sessions.json.saved_at` is NOT advanced by a capture during the window (mid-build state never committed).
- [ ] Test tolerates an in-flight capture that started pre-marker committing during the window (per spec "In-Flight Capture Atomicity").
- [ ] Test waits ≥1.5s after step-6-marker-clear and verifies the first post-clear daemon tick captured the complete final state.
- [ ] Test passes `go test -tags=integration ./cmd/bootstrap/...` from a clean checkout.
- [ ] Test is skipped in `testing.Short()` mode (keeps the default `go test ./...` run fast).

**Tests**:
- `"@portal-restoring suppresses captures across structural events fired during skeleton-restore"`
- `"at least one structural event fires during the @portal-restoring window (non-vacuous)"`
- `"save.requested present during the window does not trigger a daemon tick"`
- `"in-flight capture started pre-marker is permitted to commit its pre-restore snapshot"`
- `"first post-clear daemon tick captures the complete final state with all sessions + panes"`
- `"isolated tmux -L socket is killed in t.Cleanup on both pass and fail"`

**Edge Cases**:
- Integration tests are inherently timing-sensitive. The 1100ms / 1500ms sleeps above are conservative against the 1s ticker; if the daemon's tick cadence changes (it shouldn't per spec), the sleeps need updating.
- If Phase 3 task 3-13 already ships an isolated-socket integration scaffold, reuse it — don't re-invent the `tmux -L` wiring.
- `TMUX_SOCKET_NAME` env var vs `-L` flag: both work; prefer `-L` for clarity because it's visible in every `tmux` invocation's argv. If portal's `Commander` always adds the socket flag via env, use env; if not, ensure `-L` propagates through `tmux.Client` calls.
- Probe hook leaks: even with reverse-order removal, the probe's `run-shell` hook coexists with Portal's registered hooks. The `kill-server` in `t.Cleanup` tears down all hooks with the server — probe is ephemeral.
- Timing flakiness on CI: a slow CI runner might miss the "no capture during window" assertion if the window is very short and the daemon's ticker aligns inconveniently. Mitigate by running the test with a larger fixture (multiple sessions, many windows) so skeleton-restore takes longer than the 1s tick interval. Fixture design is intentional here.
- The test depends on Phase 2 tasks 2-11 (marker-aware capture) and 2-12 (ticker trigger logic) being correct. A failure here could indicate a Phase 2 regression, not a Phase 5 regression — diagnostic output should make that distinction clear.

**Context**:
> Spec "Bootstrap Flow → Ordering Rationale":
> "The critical ordering — `@portal-restoring` is set in step 3 **before** `_portal-saver` is created in step 4 — exists because step 4 fires `session-created`, which the hook pipeline would otherwise use to dirty the flag."
> "With the ordering above:
> - `@portal-restoring` set → daemon's first tick no-ops.
> - Restoration runs → more structural events fire → every one is a no-op on the notify side because `@portal-restoring` is still set.
> - `@portal-restoring` cleared → next daemon tick captures the now-complete post-restoration state."
>
> Spec "Save-Side Architecture → In-Flight Capture Atomicity":
> "A capture cycle is a single synchronous Go function. It checks `@portal-restoring` at entry only and runs to completion without re-checking. If bootstrap flips `@portal-restoring` from `0` to `1` while a capture is mid-execution, the in-flight capture completes normally and may commit its write after the flag is set. This is safe because: (a) a capture that started before the flag was set was capturing pre-restore (steady-state) tmux, which is a valid snapshot. (b) Writes are atomic (per-file `AtomicWrite`) and commit via the `sessions.json` rename."
>
> Spec "Save-Side Architecture → Triggers & Serialization → Properties → Restoration guard":
> "The daemon's tick checks `@portal-restoring` at the top of the cycle. While set (during the skeleton-restore window), no capture runs, regardless of dirty-flag state."
>
> Phase 3 task 3-13 established the isolated-socket integration-test pattern; this task inherits that scaffolding.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Bootstrap Flow → Ordering Rationale", "Save-Side Architecture → In-Flight Capture Atomicity", "Save-Side Architecture → Triggers & Serialization → Properties".

## built-in-session-resurrection-5-9 | approved

### Task 5-9: Integration test (isolated `tmux -L` socket): end-to-end reboot round-trip verifies structure, layout, zoom, CWDs, environment, hook firing, ANSI scrollback

**Problem**: Phase 3 task 3-13 ships a save→restore round-trip test. Phase 5 needs a tighter, end-to-end flow that exercises the **full Phase 5 path**: save daemon captures a multi-session configuration, simulate a reboot (kill-server → fresh server on the same socket with the same state dir), invoke orchestrator `Run`, let `client-attached` fire via a real `attach-session`, and verify every spec-required property round-trips: session names, window structure, pane count per window, layout string, zoom flag, per-pane CWD, per-session environment, per-pane resume-hook firing, per-pane ANSI scrollback. Without this test, a regression in any single phase's implementation could break the round-trip in ways unit tests cannot catch.

**Solution**: Extend / mirror Phase 3 task 3-13's integration test pattern, but gate the reboot on Phase 5's orchestrator and exercise the hydration trigger end-to-end via a real `tmux attach-session`. Register a resume hook with a deterministic side-effect (e.g., write a sentinel file) so post-attach verification asserts the hook fired exactly once per skeleton-restored pane. Vary `base-index` / `pane-base-index` across save and restore (one test variant with `base-index=0`, another with `base-index=1`) to catch the index-drift code paths in tasks 3-3, 3-5, 4-1.

**Outcome**: A single integration test certifies the entire feature end-to-end. Any phase's regression that breaks round-trip surfaces here first. This is the test the user will run (or the CI will run) after every release-candidate build to confirm "Zellij in tmux" actually works.

**Do**:
- Create `cmd/bootstrap/reboot_roundtrip_test.go` with `//go:build integration` tag.
- Test variants (table-driven; each runs as a subtest):
  - Variant 1: `base-index=0`, `pane-base-index=0`; 2 sessions, 2 windows each, 2-3 panes per window.
  - Variant 2: `base-index=1`, `pane-base-index=1`; same topology. Exercises the index-drift mapping.
  - (Optional additional variant if the test budget allows: `base-index=0` at save, `base-index=1` at restore — verifies paneKey drift handling at save→restore boundary.)
- Per-variant scaffolding:
  - Isolated `tmux -L <socket>` socket (same pattern as task 5-8).
  - Temp state directory via env var.
  - Start tmux server on the isolated socket, set `base-index` and `pane-base-index` per variant.
  - Build the save-time configuration: create 2 sessions with the planned structure via `tmux new-session` / `new-window` / `split-window`. Write deterministic per-pane content (e.g., `printf "SESS=%s WIN=%d PANE=%d\n\033[31mRED\033[0m\n" ...` so ANSI is in the scrollback).
  - Register resume hooks via `PORTAL_HOOKS_FILE` env-var override: map each pane's structural key to a command like `echo "HOOK:<session>:<window>.<pane>" > <state-dir>/hook-fired-<key>.sentinel`. Use the existing `hooks.Store.Save` to write the file.
  - Set per-session environment via `tmux set-environment -t <session>` for a handful of pairs (`LANG`, `TEST_VAR`).
  - Zoom one pane in one window (`resize-pane -Z`).
  - Invoke `bootstrap.Orchestrator.Run(ctx)` — this fires a capture (via daemon tick — wait 1100ms).
  - Verify `sessions.json` exists and `saved_at` is recent.
  - Save phase complete.
- Simulate reboot:
  - `tmux -L <socket> kill-server` — tears down everything on the socket.
  - Start a fresh tmux server on the same socket with the same `base-index` / `pane-base-index`.
  - Invoke `bootstrap.Orchestrator.Run(ctx)` again. Step 5 (`Restore()`) skeleton-restores from the saved `sessions.json`.
  - Verify skeleton was created: `tmux -L <socket> list-sessions` includes both saved sessions plus `_portal-saver`.
  - Trigger `client-attached`: spawn `tmux -L <socket> attach-session -t <saved-session>` in a goroutine with a fresh PTY (use `creack/pty` or similar, or spawn as a subprocess with a dummy controlling terminal). Wait briefly (50-100ms) for the attach to propagate the `client-attached` hook → `portal state signal-hydrate` → FIFO signals → helpers dump scrollback.
  - Wait up to 3s for all `hook-fired-*.sentinel` files to appear (one per skeleton-restored pane with a registered hook).
- Verify post-restore state:
  - Structure: `tmux -L <socket> list-panes -a -F '#{session_name}:#{window_index}.#{pane_index}'` returns every saved pane (modulo index drift, which variants 2/3 intentionally exercise).
  - Layout: for each restored window, `#{window_layout}` equals the saved layout string verbatim.
  - Zoom: the pane that was zoomed at save time has `#{window_zoomed_flag} == 1` after restore.
  - CWDs: each pane's `#{pane_current_path}` equals the saved CWD. Use `tmux display-message -p -t <pane> '#{pane_current_path}'`.
  - Environment: `tmux -L <socket> show-environment -t <session>` includes every saved key/value pair.
  - Hook firing: every `hook-fired-<key>.sentinel` file exists AND contains exactly one line of the form `HOOK:<session>:<window>.<pane>` (fired once, not duplicated per `client-attached` + `client-session-changed`).
  - ANSI scrollback: use `capture-pane -e -p -t <pane>` after hydration settles (wait additional 300ms post-sentinel appearance) and assert the captured content includes the original ANSI SGR sequences (e.g., the `\033[31m` red / `\033[0m` reset we wrote pre-reboot). Byte-for-byte comparison of `capture-pane -e -p` output from save-time and restore-time, after normalising the PTY-induced differences (trailing whitespace, terminal-width-induced line wraps). If byte-for-byte is too strict on terminal width differences, compare the set of ANSI SGR escape sequences present.
  - Skeleton markers: `tmux -L <socket> show-options -sv` filtered for `@portal-skeleton-*` returns zero entries (all cleared by the helpers after dump + 100ms settle).
- Exercise both attach paths in subtests:
  - Subtest A: `client-attached` fires first (the first pane attach after bootstrap).
  - Subtest B: `client-session-changed` also fires when switching between two restored sessions via `tmux switch-client -t <other-session>`; verify the second session's helpers also unblock and dump.
- `t.Cleanup` kills the socket.

**Acceptance Criteria**:
- [ ] New integration test at `cmd/bootstrap/reboot_roundtrip_test.go` with `//go:build integration` tag.
- [ ] At least two table variants differing in `base-index` / `pane-base-index` — verifies the index-drift code path non-vacuously.
- [ ] Isolated `tmux -L <socket>` — never touches user sessions.
- [ ] Socket killed in `t.Cleanup` on pass and fail.
- [ ] Save phase: multi-session, multi-window, multi-pane fixture with per-session env, one zoomed pane, deterministic per-pane ANSI scrollback.
- [ ] Reboot simulated via `kill-server` + fresh server on the same socket.
- [ ] Orchestrator `Run` invoked for both the save-warming call and the post-reboot skeleton-restore call.
- [ ] `client-attached` is exercised by spawning `tmux attach-session` on a fresh PTY.
- [ ] `client-session-changed` is exercised by `tmux switch-client`.
- [ ] Structure, layout, zoom, CWDs, environment, resume hooks, and ANSI scrollback all verified post-restore.
- [ ] Resume hook fires exactly once per skeleton-restored pane (sentinel file contains exactly one line — not two from the double-attach + switch-client firing paths).
- [ ] `@portal-skeleton-*` markers are cleared post-attach (verified by `show-options -sv` filter).
- [ ] Byte-level (or SGR-sequence-level) comparison of scrollback content pre-reboot vs post-hydration.
- [ ] Test is skipped in `testing.Short()` mode.

**Tests**:
- `"end-to-end reboot round-trip preserves structure, layout, zoom, CWDs, environment"` (per variant)
- `"resume hooks fire exactly once per skeleton-restored pane"`
- `"ANSI scrollback round-trips with colour / attribute sequences preserved"`
- `"base-index and pane-base-index drift does not break hook lookup (hook-key uses saved position)"`
- `"client-attached triggers hydration for the attached session"`
- `"client-session-changed triggers hydration when switching to a second saved session"`
- `"@portal-skeleton markers cleared after helper dump + 100ms settle"`
- `"_portal-saver daemon is live and capturing post-reboot (first post-clear tick verified)"`
- `"save-time and restore-time SGR sequences match on a representative pane"`

**Edge Cases**:
- PTY spawning in Go tests: `creack/pty` is a common pattern; if unavailable, spawn `tmux attach-session` as a subprocess with stdin/stdout/stderr redirected (may require `setsid` / controlling-terminal dance). If PTY setup is too fragile for CI, fall back to invoking `portal state signal-hydrate <session>` directly (the hook's body would do the same thing) and document that the test doesn't exercise the `client-attached` tmux-hook path specifically — this is a pragmatic tradeoff but reduces coverage.
- Timing sensitivity: hydration helper has a 3s FIFO-read timeout; the test must signal well inside that window. Use a 1s wait between `attach-session` spawn and sentinel-file polling.
- Index drift variants 2 / 3: exercise paths in Phase 3 tasks 3-3 (saved `--file` and `--hook-key` flags passed verbatim), 3-5 (re-query live paneKey), and Phase 4 task 4-1 (hook lookup uses `--hook-key` verbatim). A bug in any of those surfaces as a missing sentinel or wrong-pane hook firing.
- Zoom + layout ordering: a regression that applies zoom before layout would produce visually different geometry. Verify `window_zoomed_flag` AFTER `window_layout` is saved, and assert both on the restored window.
- Hook firing exactly once: the spec design fires hooks at hydrate-helper time; `signal-hydrate` is idempotent across `client-attached` + `client-session-changed`, but the helper only receives one FIFO byte — the second signal hits an already-unlinked FIFO and is a no-op. Verify by spawning BOTH attach events and asserting sentinel file line count == 1.
- ANSI comparison: PTY line-wrapping at a different terminal width could shift bytes. Run the test with a fixed terminal geometry (`tmux -L <socket> -f /dev/null attach-session -d -t <s>`? Or set `tmux resize-window -t ...` to a known size at save and restore time). If still flaky, compare the SET of SGR escape sequences that appear rather than byte-exact content.
- `_portal-saver` daemon startup during post-reboot orchestrator Run: the daemon seeds its hash map from disk (Phase 2 task 2-9) so its first post-clear tick does NOT rewrite every scrollback file — content-hash dedup holds. Verify scrollback file mtimes are NOT all bumped after the first post-restore tick (indirect check: if they were, the hydration hadn't cleared skeleton markers in time, or the hash-seed is broken).

**Context**:
> Spec "Bootstrap Flow (Integrated) → Attach Flow (After Bootstrap)":
> "1. Portal's open/attach code resolves the target session.
> 2. `tmux switch-client -t <session>` if Portal is running inside tmux; else `exec tmux attach-session -A -t <session>`.
> 3. tmux fires `client-attached` (bare-shell attach) or `client-session-changed` (inside-tmux switch).
> 4. The registered hook runs `portal state signal-hydrate <session-name>`.
> 5. Subprocess work: `list-panes -t <session-name>` → enumerate panes; for each pane with `@portal-skeleton-<paneKey>` set: write a byte to the pane's FIFO.
> 6. Per-pane helpers unblock, dump scrollback, sleep 100ms, unset own markers, exec hook-or-shell.
> 7. Daemon's next tick (sub-second away) captures now-hydrated panes normally."
>
> Spec "Resume Hook Firing → Firing Point: Inside the Helper's Exec Chain": hooks fire from the helper's exec chain after dump + settle + marker-unset.
>
> Spec "Layout Restoration → Per-Window Restoration Order": window → split-window × (N-1) → select-layout → select-pane → resize-pane -Z if zoomed.
>
> Spec "Save Format & Schema → Helper hook lookup under index drift": "The helper is invoked with a `--hook-key '<raw-session>:<saved-window>.<saved-pane>'` flag populated from `sessions.json` at bootstrap. The helper uses that flag (not its own live position) to look up hooks in `hooks.json`. This preserves hooks across `base-index`/`pane-base-index` changes between save and restore."
>
> Phase 3 task 3-13 established the save→restore round-trip test shape; this task extends it into a full reboot-cycle.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Bootstrap Flow → Attach Flow (After Bootstrap)", "Resume Hook Firing → Firing Point: Inside the Helper's Exec Chain", "Layout Restoration", "Save Format & Schema → Helper hook lookup under index drift".

## built-in-session-resurrection-5-10 | approved

### Task 5-10: Integration test: `portal attach NAME` and `portal open NAME` resolve names present only in `sessions.json`

**Problem**: Spec Phase 5 acceptance criterion: "`portal attach NAME` and `portal open` continue to resolve names that only exist in `sessions.json` at bootstrap time (skeleton is created before the command's own attach logic runs)." Task 5-5 removed the `bootstrapWait(cmd)` call sites that previously masked this requirement (by waiting for sessions to appear). This task adds an integration test that specifically exercises the name-resolution path: pre-seed `sessions.json` with a session `foo`, start with zero live sessions, invoke `portal attach foo` and `portal open foo` (both from inside-tmux and bare-shell contexts), and verify the resolve succeeds because orchestrator step 5 skeleton-created the session before the command's `RunE` ran. Also verify steady-state reattach (saved session already live at start-of-process) is a single JSON read + `list-sessions` + diff with no structural rewrites.

**Solution**: Add `cmd/reattach_integration_test.go` (or extend the existing reboot-roundtrip test file from 5-9). Four test cases:

1. Bare-shell `portal attach NAME` where NAME is in `sessions.json` but not yet live at start-of-process.
2. Bare-shell `portal open NAME` (path-argument form) — resolver maps NAME → path → `openPath` → `HasSession` sees the just-skeletoned session.
3. Inside-tmux `portal attach NAME` (uses `switch-client` instead of `exec attach-session -A`).
4. Steady-state: same NAME already live at start-of-process; verify orchestrator's step 5 `Restore()` skips (per spec "If a live tmux session already exists with that name → skip"); no structural rewrites (verify via tmux `display-message -p -t <s> '#{pane_id}'` returning the SAME pane id as pre-run).
5. Negative case: NAME exists in neither live tmux nor `sessions.json` → existing `"No session found: %s"` error path unchanged.

**Outcome**: Regression guard that catches any future regression that re-introduces a pre-attach wait or accidentally short-circuits `Restore()` for cases that need it. This is the test that proves the spec's "name resolution post-bootstrap" acceptance criterion.

**Do**:
- Create `cmd/reattach_integration_test.go` with `//go:build integration` tag. OR extend `cmd/bootstrap/reboot_roundtrip_test.go` from task 5-9 — judgment call; keep them separate if the test file is getting large.
- Common scaffolding (extract as a helper if task 5-8 / 5-9 have not already):
  - Isolated `tmux -L <socket>` socket.
  - Temp state directory.
  - Portal binary built once.
  - Invoked as a subprocess (`exec.Cmd`) with `TMUX_SOCKET_NAME`, `HOME`, and `PORTAL_STATE_DIR` env vars threaded through so portal binds to the isolated socket and state dir.
- Test 1 — bare-shell `portal attach NAME`, name in sessions.json only:
  - Pre-seed `sessions.json` with session `foo` (minimal: 1 window, 1 pane, plausible layout).
  - Start tmux server on the isolated socket (`tmux -L <socket> new-session -d -s _bootstrap "sleep 999"` then `kill-session -t _bootstrap`? Simpler: let `EnsureServer` start it — portal does this automatically).
  - Verify pre-condition: `tmux -L <socket> list-sessions` does NOT include `foo`.
  - Run `portal attach foo` as subprocess. Because we are bare-shell (no `$TMUX`), portal's `AttachConnector.Connect` will `syscall.Exec` into `tmux attach-session`. For test purposes, swap in a SwitchConnector-equivalent via `attachDeps` injection OR detect exec and verify via a wrapper script. Simpler: set `PORTAL_TEST_NOEXEC=1` (a new test-only env var the attach code path honours by printing `"would exec: tmux attach-session -t foo"` instead of actually execing) — this requires a small production-code toggle, OR construct the test as "portal attach succeeded or failed based on the exit code + stderr contents."
  - Pragmatic alternative: invoke `portal attach foo --dry-run` (new flag the test can use) — if that doesn't exist, invoke via a thin test harness that sets `attachDeps` directly. This keeps the test in the cmd-package testing pattern rather than the subprocess pattern.
  - Either way, the assertion: `HasSession("foo") == true` AFTER `PersistentPreRunE` runs; command's `RunE` does not return `"No session found: foo"`.
- Test 2 — bare-shell `portal open NAME` (path-argument):
  - Pre-seed `sessions.json` with a session whose name matches an alias pointing to a path; OR use `portal open <path>` where `<path>` resolves to a pre-seeded session name.
  - Invoke; verify the resolver lands on the restored session without error.
- Test 3 — inside-tmux `portal attach NAME`:
  - Pre-seed `sessions.json` with `foo`. Create a live session `host-session` on the isolated socket.
  - Run `portal attach foo` with `TMUX=<socket>,<pid>,<pane>` env var set (simulating inside-tmux). Portal's `InsideTmux()` returns true → uses `SwitchConnector` → `tmux switch-client -t foo`.
  - Verify `foo` is now live AND the client is switched (via `tmux -L <socket> display-message -p '#{client_session}'`).
- Test 4 — steady-state (zero rewrites):
  - Pre-seed `sessions.json` with `foo`.
  - Create `foo` as a live session BEFORE running portal (simulating "server already has foo live").
  - Capture pre-run pane id: `tmux -L <socket> display-message -p -t foo:0.0 '#{pane_id}'`.
  - Run `portal attach foo`.
  - Capture post-run pane id for the same structural position.
  - Assert pane ids are equal → `Restore()` did NOT rewrite the pane. Also verify `sessions.json.saved_at` is not advanced during the restore window (matching task 5-8's property).
- Test 5 — negative case:
  - Empty `sessions.json`, empty live state. Run `portal attach nonexistent`.
  - Assert exit code non-zero; stderr contains `"No session found: nonexistent"` (existing error path from `cmd/attach.go:37`, unchanged).
- `t.Cleanup` kills the socket in every test case.

**Acceptance Criteria**:
- [ ] New integration test file at `cmd/reattach_integration_test.go` with `//go:build integration` tag (or an extension of task 5-9's file).
- [ ] Five explicit test cases covering: bare-shell attach, bare-shell open, inside-tmux attach, steady-state zero-rewrite, negative not-found.
- [ ] Isolated `tmux -L` socket per test.
- [ ] `t.Cleanup` kills the socket.
- [ ] Tests 1–3 verify that orchestrator's step 5 `Restore()` creates the skeleton so `HasSession(NAME)` returns true before the command's `RunE` reaches its `HasSession` guard.
- [ ] Test 4 verifies `Restore()` is a no-op for already-live sessions: pane id is unchanged post-run; `saved_at` is unchanged within the restore window.
- [ ] Test 5 verifies the existing `"No session found: %s"` error path works for names in neither live tmux nor `sessions.json`.
- [ ] `portal open NAME` path is tested via the path-argument flow (not just `portal attach`).
- [ ] `client-attached` and `client-session-changed` are both exercised (tests 1 and 3 respectively).
- [ ] Tests skipped in `testing.Short()` mode.

**Tests**:
- `"portal attach NAME resolves a name present only in sessions.json (bare shell)"`
- `"portal open PATH resolves a session name present only in sessions.json"`
- `"portal attach NAME resolves a name present only in sessions.json (inside tmux switch-client)"`
- `"steady-state reattach with saved session already live performs zero structural rewrites"`
- `"portal attach NAME returns the existing not-found error for names in neither live nor saved state"`
- `"has-session returns true for every name in sessions.json post-bootstrap"`
- `"saved_at is not advanced during a steady-state reattach window"`

**Edge Cases**:
- Bare-shell `portal attach` via `syscall.Exec`: testing an exec handoff inside a Go test requires either (a) intercepting `syscall.Exec` via an `execer` interface injection (pattern already present in `cmd/open.go:173-184` — the `realExecer`), or (b) spawning portal as a subprocess and verifying via exit codes. Pattern (a) is cleaner for `attach` too — extend `AttachConnector` with an injectable execer, default to `realExecer`, let tests supply a recording one.
- Inside-tmux detection: portal checks `os.Getenv("TMUX") != ""`. Set that env var in the test subprocess invocation; or inject `tmux.InsideTmux` via a package-level var.
- `portal open` resolver: `buildQueryResolver` reaches the alias store and zoxide. For the integration test, either seed aliases (`PORTAL_ALIAS_FILE` env var override) or use a path-argument that resolves via the direct-path branch. The latter is simpler.
- Steady-state test (no rewrites): panes created by portal skeleton-restore have fresh `pane_id`s different from the pre-existing live pane. If `Restore()` incorrectly re-created the pane, pane_id would differ. If it correctly skipped, pane_id is preserved. This is the assertion shape.
- Negative case exit code: `cmd/attach.go:37` returns `fmt.Errorf("No session found: %s", name)` which cobra surfaces as a non-zero exit. `SilenceErrors: true` on rootCmd (line 75) means cobra does NOT print the error — but the exit code is still non-zero. Test asserts the exit code and checks stderr via the injected test seam rather than relying on cobra's own printing.
- `saved_at` not advanced: reinforces task 5-8's assertion. Duplicated intentionally because this test case exercises a different trigger (normal command, not a test-only probe).

**Context**:
> Phase 5 acceptance (from the phase header):
> "`portal attach NAME` and `portal open` continue to resolve names that only exist in `sessions.json` at bootstrap time (skeleton is created before the command's own attach logic runs)."
>
> Spec "Bootstrap Flow → Return-to-Caller Timing → CLI path":
> "For `portal attach NAME` where the target was in `sessions.json`, skeleton was restored before the attach logic runs, so `has-session -t NAME` returns true by the time the attach needs it."
>
> Spec "Restore-Side Architecture → Restoration Trigger":
> "If a live tmux session already exists with that name → skip. User's current reality is authoritative; Portal never clobbers live sessions."
> "Steady-state cost (all saved sessions already live): ~20ms — one JSON read + one `list-sessions` call + diff → no-op. Invisible."
>
> Existing `cmd/attach.go:36-44` — `HasSession` guard + `"No session found: %s"` error path — unchanged by Phase 5.
> Existing `cmd/open.go:107-115` — `qr.Resolve` result switch handling — unchanged; the prerequisite `HasSession` / path resolution happens with live tmux already reflecting the skeleton-restored state.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Bootstrap Flow → Return-to-Caller Timing → CLI path", "Restore-Side Architecture → Restoration Trigger", phase-5 acceptance criteria (planning document).
