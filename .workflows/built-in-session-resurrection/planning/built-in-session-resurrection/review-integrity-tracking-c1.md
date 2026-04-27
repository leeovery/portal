---
status: in-progress
created: 2026-04-27
cycle: 1
phase: Plan Integrity Review
topic: built-in-session-resurrection
---

# Review Tracking: built-in-session-resurrection - Integrity

## Findings

### 1. Phase 6 task 6-9 changes orchestrator signature without updating Phase 5 call site

**Severity**: Critical
**Plan Reference**: `phase-6-tasks.md` task 6-9 (`built-in-session-resurrection-6-9`); `phase-5-tasks.md` task 5-3 (`built-in-session-resurrection-5-3`)
**Category**: Dependencies and Ordering / Task Self-Containment
**Change Type**: add-to-task

**Details**:
Task 6-9 says `Change Run's signature to (bool, []Warning, error) so callers get warnings separately from fatal errors.` But the only caller of `Orchestrator.Run` is `PersistentPreRunE`, defined in task 5-3 with the body `bootstrapStarted, bootstrapErr = orchestrator.Run(cmd.Context())` — a two-value receive. When task 6-9 ships, that line stops compiling. Task 6-9 mentions wiring the warnings sink into `PersistentPreRunE` ("After `orchestrator.Run` returns warnings: `for _, w := range warnings { bootstrapWarnings.Add(w) }`") but never explicitly tells the implementer to update the receive-tuple in task 5-3's code, nor to update `BootstrapDeps.Orchestrator`'s `Runner` interface (whose `Run(ctx) (bool, error)` shape was pinned in task 5-3).

Without an explicit "Do" step naming the affected call sites, an implementer following 6-9 in isolation will land a compile-broken change. This is the same class of cross-task wiring requirement that 6-10's "carries `Warnings []Warning`" handling already calls out for `BootstrapCompleteMsg`.

**Current** (in `phase-6-tasks.md`, the "Do" section of task 6-9, around the orchestrator-signature bullet):

```
- Extend `cmd/bootstrap/bootstrap.go` `Orchestrator`:
  - Add a `Warnings []Warning` field accumulated during `Run`.
  - In step 4 (EnsureSaver), on persistent failure: `o.Warnings = append(o.Warnings, SaverDownWarning())`. Also log WARN to `portal.log` with `ComponentBootstrap`.
  - In step 5 (Restore), the restore path returns a typed error for corrupt `sessions.json`; detect that case via `errors.Is(err, restore.ErrCorruptIndex)` (Phase 3 task 3-1 defines this error; if it doesn't, this task adds it as part of the Restore reader). Append `CorruptSessionsJSONWarning()` to the orchestrator's slice.
  - Change `Run`'s signature to `(bool, []Warning, error)` so callers get warnings separately from fatal errors.
```

**Proposed**:

```
- Extend `cmd/bootstrap/bootstrap.go` `Orchestrator`:
  - Add a `Warnings []Warning` field accumulated during `Run`.
  - In step 4 (EnsureSaver), on persistent failure: `o.Warnings = append(o.Warnings, SaverDownWarning())`. Also log WARN to `portal.log` with `ComponentBootstrap`.
  - In step 5 (Restore), the restore path returns a typed error for corrupt `sessions.json`; detect that case via `errors.Is(err, restore.ErrCorruptIndex)` (Phase 3 task 3-1 defines this error; if it doesn't, this task adds it as part of the Restore reader). Append `CorruptSessionsJSONWarning()` to the orchestrator's slice.
  - Change `Run`'s signature to `(serverStarted bool, warnings []Warning, err error)` so callers get warnings separately from fatal errors.
- Update the `Runner` interface introduced in task 5-3 to match: `type Runner interface { Run(ctx context.Context) (bool, []Warning, error) }`. Update `bootstrap.NewShim` (also from 5-3) so the shim returns `(started, nil, err)` — legacy bootstrappers produce no warnings.
- Update `cmd/root.go` `PersistentPreRunE` (task 5-3) to receive the third return value and feed it into the warnings sink:
  ```go
  bootstrapOnce.Do(func() {
      bootstrapStarted, bootstrapWarningsSlice, bootstrapErr = orchestrator.Run(cmd.Context())
      for _, w := range bootstrapWarningsSlice {
          bootstrapWarnings.Add(w)
      }
  })
  ```
  Add a package-level `var bootstrapWarningsSlice []bootstrap.Warning` alongside the existing `bootstrapStarted` / `bootstrapErr` memoisation state. Reset it in the `resetBootstrapOnce(t)` test helper.
- Verify every other test fixture in `cmd/root_test.go` / `cmd/bootstrap/bootstrap_test.go` that constructs an orchestrator literal or stub now satisfies the three-return shape.
```

**Resolution**: Pending
**Notes**:

---

### 2. Task 3-3's hydrate command quoting is unsafe for raw session names containing single quotes

**Severity**: Important
**Plan Reference**: `phase-3-tasks.md` task 3-3 (`built-in-session-resurrection-3-3`)
**Category**: Acceptance Criteria Quality / Edge Cases
**Change Type**: update-task

**Details**:
Task 3-3 builds the per-pane hydrate command as `sh -c 'portal state hydrate --fifo <fifoAbs> --file <scrollbackAbs> --hook-key <rawHookKey>; exec $SHELL'`. The Solution and Do sections call out a "minimal single-quote-safe shell escaper" with the rationale: "simplest correct approach is to forbid single quotes in state paths — paneKey sanitizer rules them out anyway — and rely on `sh -c`'s parsing of the single-quoted body."

The argument holds for `<fifoAbs>` and `<scrollbackAbs>` (paneKey-sanitized — no `'`) but it does NOT hold for `<rawHookKey>`. The hook key is `<raw-session>:<saved-window>.<saved-pane>` per the spec's "Save Format & Schema → Helper hook lookup under index drift" note, and Phase 4 task 4-1's edge case explicitly covers session names containing `:`. tmux session names can also contain `'`. A user with a session named `it's-mine` produces a hook key `it's-mine:0.0` which, embedded in the outer single-quoted `sh -c '...'` body, breaks the quoting and either fails to launch the helper or, worse, executes attacker-controlled fragments as shell.

The acceptance criteria do not include any "session name containing single quote round-trips safely" check. Tests do not list this case.

**Current** (in `phase-3-tasks.md`, task 3-3, the `buildHydrateCommand` description in the "Do" section):

```
- `func (r *SessionRestorer) buildHydrateCommand(fifoPath, scrollbackAbs, hookKey string) string` — returns `fmt.Sprintf("sh -c 'portal state hydrate --fifo %s --file %s --hook-key %s; exec $SHELL'", shellEscape(fifoPath), shellEscape(scrollbackAbs), shellEscape(hookKey))`. Uses a minimal single-quote-safe shell escaper (wrap paths in double quotes if they contain shell metacharacters; simplest correct approach is to forbid single quotes in state paths — paneKey sanitizer rules them out anyway — and rely on `sh -c`'s parsing of the single-quoted body).
```

**Proposed**:

```
- `func (r *SessionRestorer) buildHydrateCommand(fifoPath, scrollbackAbs, hookKey string) string` — returns the `sh -c '...'` invocation with every interpolated value POSIX-shell-safe. Implementation: use the "close-quote / escaped-quote / re-open-quote" pattern (`'` → `'\''`) on every argument before interpolation. The fifoPath and scrollbackAbs are paneKey-sanitized so they never contain `'`, but the hookKey carries the **raw** session name per spec "Save Format & Schema → Helper hook lookup under index drift" — session names can contain `'` (tmux permits it). A naive single-quote concatenation breaks the outer `sh -c '...'` body and either fails the helper launch or, worse, executes shell fragments from the session name. Concrete shape:
  ```go
  func quoteForSingleQuoted(s string) string {
      return strings.ReplaceAll(s, "'", `'\''`)
  }
  func (r *SessionRestorer) buildHydrateCommand(fifoPath, scrollbackAbs, hookKey string) string {
      return fmt.Sprintf("sh -c 'portal state hydrate --fifo %s --file %s --hook-key %s; exec $SHELL'",
          quoteForSingleQuoted(fifoPath),
          quoteForSingleQuoted(scrollbackAbs),
          quoteForSingleQuoted(hookKey))
  }
  ```
  The `quoteForSingleQuoted` helper is shared with task 4-3's `migrate-rename` body if that task ever needs it (it does not in v1; document the helper here).
```

Additionally extend the **Acceptance Criteria** with a new bullet:

```
- [ ] Hydrate command construction is safe for raw hook keys containing single quotes — every interpolated value is escaped via the `'\''` close-quote / escape / re-open-quote pattern.
```

And extend the **Tests** list with:

```
- `"it builds an exec-safe hydrate command when the raw session name contains a single quote"`
```

And extend **Edge Cases** with:

```
- Raw session name containing `'` (e.g., `it's-mine`): the hook key argument is `it's-mine:0.0`; naive interpolation into `sh -c '... --hook-key it's-mine:0.0 ...'` breaks quoting. The single-quote-escape pattern (`'` → `'\''`) keeps the command parseable. fifoPath and scrollbackAbs are paneKey-sanitized so the same hazard does not exist for them, but the helper is shared for consistency.
```

**Resolution**: Pending
**Notes**:

---

### 3. Task 3-3 Solution section contains an unresolved internal contradiction about FIFO timing

**Severity**: Minor
**Plan Reference**: `phase-3-tasks.md` task 3-3 (`built-in-session-resurrection-3-3`)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
The Solution paragraph reads as a stream-of-consciousness rewrite: it begins by describing the FIFO-after-creation flow, hits a "Correction: re-read the spec" mid-paragraph, then negotiates with itself about Option A (predict live indices) before settling on it. The reader has to parse the negotiation and figure out which version is the live plan. The `[needs-info]` block immediately after explicitly opens up Options B and C for user override.

The body text below the contradiction (the Do steps and Acceptance Criteria) all use Option A, so the implementation guidance is self-consistent. Only the Solution narrative is muddled. An implementer skimming the Solution may conclude the task is undecided when it is in fact pinned (subject to the `[needs-info]`). A clean rewrite removes that confusion without changing scope.

**Current** (in `phase-3-tasks.md`, task 3-3, the "Solution" section):

```
**Solution**: Add `func RestoreSession(client *tmux.Client, stateDir string, sess state.Session) error` in `internal/restore/session.go`. The function: (1) pre-creates every pane's FIFO via `state.CreateFIFO` so helpers can block immediately; (2) calls `tmux new-session -d -s <name> -c <root_cwd> '<hydrate cmd for window0.pane0>'` with the first pane's saved scrollback-file and hook-key values; (3) applies `set-environment -t <name> <KEY> <VAL>` for each key/value in `sess.Environment`; (4) for each remaining window, runs `tmux new-window -t <name>: -n <name> -c <cwd> '<hydrate cmd>'` and then `tmux split-window -t <name>:<window-index> -c <cwd> '<hydrate cmd>'` (N-1) times; (5) returns to the caller, leaving layout/zoom/active/marker work for task 3-4 and 3-5. Per-pane hydrate command construction uses `BuildHydrateCommand(stateDir, sess.Name, win, pane)` — a small pure helper that composes the saved scrollback-file path from `pane.ScrollbackFile`, the live FIFO path from `state.FIFOPath(dir, livePaneKey)` (using the *live* paneKey — see task 3-5), and the saved hook-key `fmt.Sprintf("%s:%d.%d", sess.Name, win.Index, pane.Index)`. Because the live paneKey is only knowable after pane creation, the FIFO path for each pane is pre-created using the *saved* paneKey and remapped in task 3-5; for this task, the spec requires FIFOs at the saved paneKey so the helper can block on them regardless of index drift.

Correction: re-read the spec — live paneKey is used for markers and FIFO paths (see "Canonical paneKey → Indices used in paneKey are always *live* indices"); the saved scrollback file path is passed via `--file` so the helper reads saved-indexed content. So FIFO creation must happen *after* each pane is created using the live paneKey, not before using the saved paneKey. Revise the flow: create session/window/pane → re-query live index → compute live paneKey → create FIFO at live paneKey path → re-send the hydrate command via `tmux send-keys` is NOT an option (the helper is already running as the pane's initial process and is mid-`open(O_RDONLY)` on a non-existent FIFO). The resolution the spec mandates is that FIFOs are created *just before pane creation* — which implies the orchestrator must compute the live paneKey *in advance*. Achievable because base-index / pane-base-index are server options readable via `tmux show-options -gv base-index` and `show-options -gv pane-base-index` before pane creation; the first-window will have window index `= base-index + 0`, first-pane `= pane-base-index + 0`, and subsequent windows/panes increment by 1. Resolve this by reading `base-index` / `pane-base-index` once at the start of `RestoreSession`, predicting live indices from saved structural order, computing live paneKeys, creating FIFOs at those live paths, and building hydrate commands with the live FIFO path and saved `--file` / `--hook-key` values.
```

**Proposed**:

```
**Solution**: Add `func RestoreSession(client *tmux.Client, stateDir string, sess state.Session) error` in `internal/restore/session.go`. Implementation outline (subject to the `[needs-info]` resolution below — Option A, prediction-before-creation, is the working assumption):

1. Read `base-index` and `pane-base-index` server options once via `show-options -gv`, defaulting to 0 if unset. Predict the live paneKey for each saved (window, pane) position: `liveWin = base-index + savedWindowOffset`, `livePane = pane-base-index + savedPaneOffset`.
2. For every saved pane, create the FIFO at `state.FIFOPath(stateDir, livePaneKey)` *before* creating the pane (so the helper can block on `open(O_RDONLY)` immediately when it starts).
3. `tmux new-session -d -s <name> -c <root_cwd> '<hydrate cmd for window0.pane0>'` — the hydrate command embeds the **live** FIFO path, the **saved** scrollback file path (`pane.ScrollbackFile` from `sessions.json`), and the **saved** hook key (`fmt.Sprintf("%s:%d.%d", sess.Name, win.Index, pane.Index)`) — never live re-derivation for `--file` or `--hook-key`.
4. Apply `set-environment -t <name> <KEY> <VAL>` for each key/value in `sess.Environment` *after* `new-session` but *before* any `new-window` / `split-window` (load-bearing — subsequent panes inherit the saved env).
5. For each remaining window, run `tmux new-window -t <name>: -n <name> -c <cwd> '<hydrate cmd>'` then `tmux split-window` for the additional panes.
6. Return to the caller. Layout / zoom / active selection (task 3-4) and `@portal-skeleton-<paneKey>` markers via re-queried live paneKey (task 3-5) run as sequenced follow-ups orchestrated by task 3-6.

Task 3-5 is the defensive re-alignment point: it re-queries `list-panes -t <session>` after creation and verifies the prediction matched, logging a warning if not. Under Option A, the prediction is correct in every realistic scenario; the re-query is belt-and-braces only.

**[needs-info]**: Task 3-3's "predict live indices via `base-index` / `pane-base-index` server options" is a planning invention — the spec describes a re-query approach (line 324) but does not mandate prediction-before-creation. Prediction works in the common case but adds complexity (option reads, prediction-vs-live divergence handling in task 3-5). Alternative approaches the spec is compatible with:
- **Option B**: pass the saved-paneKey FIFO path to the helper at construction; after pane creation, re-query live paneKey and either (b1) symlink live → saved or (b2) have `signal-hydrate` use the saved paneKey from the index (NOT the live paneKey).
- **Option C**: decouple FIFO naming from paneKey — use a UUID stored in the index.

Planning has chosen Option A (prediction). User should confirm or override before implementation. If Option A stands, task 3-5's drift-detection becomes a defensive log-only branch; if any other Option is chosen, task 3-5 simplifies further.
```

(The `[needs-info]` block from the existing task body is preserved verbatim — the rewrite consolidates the Solution narrative without changing the planning decision.)

**Resolution**: Pending
**Notes**:

---

### 4. Task 5-3's `BootstrapDeps.Bootstrapper` legacy shim path is under-specified

**Severity**: Minor
**Plan Reference**: `phase-5-tasks.md` task 5-3 (`built-in-session-resurrection-5-3`)
**Category**: Task Template Compliance / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Task 5-3 introduces `bootstrap.NewShim(ServerBootstrapper) Runner` to keep the legacy `BootstrapDeps.Bootstrapper` field working through the Phase 5 cutover. The shim is implied to satisfy the `Runner` interface by calling only `EnsureServer` and returning `(started, err)`. The task body never includes `bootstrap.NewShim` in its Do steps — it appears only inside an inline code block within the `buildBootstrapDeps` rewrite. The shim has no acceptance criterion, no tests, and no behavioural specification beyond the inline comment.

An implementer who reads only the Acceptance Criteria + Do steps will not know that the shim exists, what it should return for the second `bool` (skeleton-restored?), or whether it should error on a deps with neither `Orchestrator` nor `Bootstrapper` set.

**Current** (in `phase-5-tasks.md`, task 5-3, the `buildBootstrapDeps` "Do" bullet):

```
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
```

**Proposed**:

```
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
- Add `bootstrap.NewShim(s ServerBootstrapper) Runner` in `cmd/bootstrap/shim.go`. Behaviour:
  1. Returns a `Runner` whose `Run(ctx)` calls `s.EnsureServer()` and returns `(started, err)` from that call.
  2. Performs no hook registration, no `_portal-saver` setup, no restore, no CleanStale — legacy tests that injected a bare `ServerBootstrapper` predate those steps.
  3. If `s == nil`, `NewShim` returns a no-op Runner whose `Run(ctx)` returns `(false, nil)`. This guards against tests that set `BootstrapDeps{Client: ...}` without a `Bootstrapper`.
  4. Mark the shim as `// Deprecated: scheduled for removal in Phase 6 after every cmd-package test migrates to the full Orchestrator seam.`
- Tests in `cmd/bootstrap/shim_test.go`:
  - `"NewShim returns a Runner that delegates Run to ServerBootstrapper.EnsureServer"`.
  - `"NewShim wraps the EnsureServer error and propagates the started flag"`.
  - `"NewShim with a nil ServerBootstrapper returns a no-op Runner"`.
  - `"NewShim Runner does not register hooks, set markers, or call Restore"` — verified via a fake `ServerBootstrapper` whose other methods (none in the interface, but the fake should panic if any other method is invoked) don't fire.
- The shim is a transitional artefact with a finite lifetime. Phase 6 must delete it; add `TODO(phase-6): delete after legacy bootstrappers removed` markers in both `shim.go` and `BootstrapDeps.Bootstrapper`'s field comment.
```

Add to **Acceptance Criteria**:

```
- [ ] `bootstrap.NewShim(ServerBootstrapper) Runner` exists in `cmd/bootstrap/shim.go` and is documented as deprecated.
- [ ] Shim returns a no-op Runner when given a nil ServerBootstrapper.
- [ ] Shim's Run only calls `EnsureServer`; does not invoke hook registration, `_portal-saver` setup, restore, or CleanStale.
```

Add to **Tests**:

```
- `"NewShim returns a Runner that delegates Run to ServerBootstrapper.EnsureServer"`
- `"NewShim with a nil ServerBootstrapper returns a no-op Runner"`
- `"NewShim Runner does not perform hook registration or restore"`
```

**Resolution**: Pending
**Notes**:

---

### 5. Task 6-7's symlink defensive check has unresolved indecision in edge-case discussion

**Severity**: Minor
**Plan Reference**: `phase-6-tasks.md` task 6-7 (`built-in-session-resurrection-6-7`)
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Task 6-7's Edge Cases section discusses the symlink defensive check for `--purge`, then drifts into a self-negotiation: "Relax: the symlink check can be skipped if the given path is the system-canonical `~/.config/portal/state` ... Implementation choice: err on the safe side — any mismatch triggers refusal." The acceptance criterion just says "refuses to remove paths whose `EvalSymlinks` resolution differs from the given path."

The Edge Cases discussion mentions a problematic case: macOS users on the legacy `~/Library/Application Support/portal/` path. After the one-shot migration in `cmd/config.go`, `ResolveDir()` returns the new XDG path, but if the migration has not yet run for some reason, `EvalSymlinks` may return a different canonical form. The task's response is "err on the safe side — any mismatch triggers refusal", but the impact is that legitimate `--purge` invocations on certain configurations fail with "refusing to purge".

The Do steps implement strict equality check; the AC matches; but no test covers the false-positive scenario or how a user recovers. An implementer who wires this strictly will leave users on macOS confused.

**Current** (in `phase-6-tasks.md`, task 6-7, the relevant Edge Cases bullet):

```
- State dir's canonical resolution differs from the given path (e.g., `~/.config/portal` is a symlink to `~/Library/Application Support/portal`): `EvalSymlinks("~/.config/portal/state")` returns the canonical path; if `resolved != dir`, refuse. This catches the macOS migration artifact (`cmd/config.go` performs a one-time migration; after migration the resolved path matches the given path if the user has the new layout, or differs if they don't). For users still on the old path: `ResolveDir()` already returns the new `~/.config/portal/state/` so the defensive check's false-positive is rare.
  - Relax: the symlink check can be skipped if the given path is the system-canonical `~/.config/portal/state` (i.e., the check is only active when the resolved path is substantially different from the input). Implementation choice: err on the safe side — any mismatch triggers refusal. Users with custom setups can override via env var.
```

**Proposed**:

```
- State dir's canonical resolution differs from the given path (e.g., `~/.config/portal` is a symlink to `~/Library/Application Support/portal`): `EvalSymlinks("~/.config/portal/state")` returns the canonical path. The check uses `os.Lstat` on each path component — only the final component being a symlink triggers refusal. Intermediate symlinks (the user's `~/.config` is a symlink, or `~/Library` resolves through one) are tolerated because the final `state/` directory is what `os.RemoveAll` operates on. Concrete implementation: `if info.Mode()&os.ModeSymlink != 0 { refuse }` — only the leaf symlink is rejected, not the resolution chain. This avoids the false-positive on macOS legacy installs whose intermediate paths route through other symlinked directories.
- Users with `state/` itself as a symlink (deliberately redirected to e.g. an external drive): receive the refusal with a clear log line: `refusing to purge symlinked state dir <path>: remove it manually if intentional`. This is a deliberate friction, not a bug — purging through an opaque symlink could nuke unrelated data.
```

Update the relevant **Do** step:

```
- Implement `purgeStateDir(dir string, logger *state.Logger) error`:
  1. `info, err := os.Lstat(dir)` — if `ENOENT`, return nil (idempotent).
  2. If `info.Mode()&os.ModeSymlink != 0`, return an error: `"refusing to purge symlinked state dir: <path>"` with a logger.Warn line. The caller can `readlink` the path and `rm -rf` the target manually if intentional.
  3. (Removed: the prior `EvalSymlinks` strict-equality check is dropped — intermediate symlinks in the path resolution are tolerated; only the leaf being a symlink triggers refusal.)
  4. `if err := os.RemoveAll(dir); err != nil { logger.Error(state.ComponentDaemon, "purge failed: %v", err); return err }`.
  5. Log Info: `"purged state directory %s"`.
```

Update **Acceptance Criteria** — replace the broad symlink-check criterion:

```
- [ ] `purgeStateDir` refuses to remove paths where `state/` itself is a symlink (defensive — only the leaf check, not the resolution chain).
- [ ] Intermediate symlinks in the resolved path (e.g., `~/.config` is a symlink) are tolerated; `--purge` succeeds.
```

(Drop the old criterion: `purgeStateDir refuses to remove paths whose EvalSymlinks resolution differs from the given path (prevents symlinked-in-chain attacks).`)

Update **Tests** — add:

```
- `"--purge succeeds when an intermediate path component is a symlink"` (e.g., `~/.config` symlinked to `~/cfg`).
- `"--purge refuses when state/ itself is a symlink"` (existing test; tighten the assertion message).
```

**Resolution**: Pending
**Notes**:

---

### 6. Task 4-3's `migrate-rename` body ships before task 4-4 unblocks its registration — Phase 4 acceptance cannot be met

**Severity**: Important
**Plan Reference**: `planning.md` Phase 4 acceptance; `phase-4-tasks.md` task 4-3 (`built-in-session-resurrection-4-3`), task 4-4 (`built-in-session-resurrection-4-4`)
**Category**: Phase Structure / Dependencies and Ordering
**Change Type**: update-phase

**Details**:
Phase 4 acceptance criterion #4 reads: "`portal state migrate-rename <old> <new>` is registered against `session-renamed` alongside `portal state notify` using the same content-based idempotency pattern; it rewrites every `<old>:*` key in `hooks.json` to `<new>:*` atomically via `AtomicWrite` and logs best-effort on failure." This bundles task 4-3's body work AND task 4-4's registration into one acceptance bullet. Task 4-4 is BLOCKED on a `[needs-info]` planning decision (Route A vs Route B for sourcing the prior session name argument).

As the plan stands, Phase 4 ships task 4-3 as approved/ready but task 4-4 cannot land. An implementer completing every approved Phase 4 task will produce a `migrate-rename` binary that cannot be invoked because nothing registers the hook. Phase 4 cannot be marked "complete" because acceptance criterion #4 is unsatisfiable.

The traceability cycle 1 already marked task 4-4 BLOCKED with `[needs-info]`, but Phase 4 acceptance never inherited that status. This is an integrity issue: the phase-level bookkeeping doesn't reflect the task-level reality.

**Current** (in `planning.md`, the Phase 4 Acceptance section):

```
**Acceptance**:
- [ ] `internal/hooks/executor.go`, `cmd/hook_executor.go`, all `ExecuteHooks` call sites in `cmd/open.go` and `cmd/attach.go`, and any `@portal-active-<pane>` registration-time marker logic are deleted
- [ ] The hydrate helper reads `hooks.json` after its 100ms settle sleep, looks up by the `--hook-key` argument (not live pane position), and `exec`s `sh -c 'HOOK; exec $SHELL'` on match or `$SHELL` otherwise — on both the successful-dump and the missing-file success paths
- [ ] Hook firing does NOT happen on the 3-second timeout path; the timeout path also does NOT clear the skeleton marker (next attach re-signals)
- [ ] `portal state migrate-rename <old> <new>` is registered against `session-renamed` alongside `portal state notify` using the same content-based idempotency pattern; it rewrites every `<old>:*` key in `hooks.json` to `<new>:*` atomically via `AtomicWrite` and logs best-effort on failure
- [ ] `CleanStale` no longer has the `len(livePanes) == 0` early return; runs unconditionally as bootstrap step 7 and from `portal clean`
- [ ] Stale-detection criteria remain unchanged: structural-key mismatch against `list-panes -a` only; binary-missing and `projects.json`-absent are NOT staleness signals
- [ ] `portal hooks set`, `portal hooks list`, `portal hooks rm --on-resume` retain their existing user-facing surface; behavioural change is documented: hooks fire on skeleton-restored panes only, not on live detach/reattach within a server lifetime
```

**Proposed**:

```
**Acceptance**:
- [ ] `internal/hooks/executor.go`, `cmd/hook_executor.go`, all `ExecuteHooks` call sites in `cmd/open.go` and `cmd/attach.go`, and any `@portal-active-<pane>` registration-time marker logic are deleted
- [ ] The hydrate helper reads `hooks.json` after its 100ms settle sleep, looks up by the `--hook-key` argument (not live pane position), and `exec`s `sh -c 'HOOK; exec $SHELL'` on match or `$SHELL` otherwise — on both the successful-dump and the missing-file success paths
- [ ] Hook firing does NOT happen on the 3-second timeout path; the timeout path also does NOT clear the skeleton marker (next attach re-signals)
- [ ] `portal state migrate-rename <old> <new>` body is implemented (task 4-3): rewrites every `<old>:*` key in `hooks.json` to `<new>:*` atomically via `AtomicWrite` and logs best-effort on failure
- [ ] **Conditional on `[needs-info]` resolution in task 4-4**: `portal state migrate-rename` is registered against `session-renamed` alongside `portal state notify` using the same content-based idempotency pattern. Until the prior-name argument-source decision (Route A vs Route B) lands, this bullet is BLOCKED and the migration body is dead code — hooks for a renamed session get orphaned and are pruned by `CleanStale` (the spec's documented "best-effort / re-register" failure mode applies).
- [ ] `CleanStale` no longer has the `len(livePanes) == 0` early return; runs unconditionally as bootstrap step 7 and from `portal clean`
- [ ] Stale-detection criteria remain unchanged: structural-key mismatch against `list-panes -a` only; binary-missing and `projects.json`-absent are NOT staleness signals
- [ ] `portal hooks set`, `portal hooks list`, `portal hooks rm --on-resume` retain their existing user-facing surface; behavioural change is documented: hooks fire on skeleton-restored panes only, not on live detach/reattach within a server lifetime
```

**Resolution**: Pending
**Notes**: This finding mirrors the task 4-4 BLOCKED marker into the phase-level acceptance so the phase's completion state is honest. If the user pins Route A or Route B in the next planning cycle, both this acceptance bullet and task 4-4 unblock together.

---

### 7. Task 5-2's `EnsureSaver` failure handling promises a `SaverDownError` sentinel that is not defined

**Severity**: Minor
**Plan Reference**: `phase-5-tasks.md` task 5-2 (`built-in-session-resurrection-5-2`); `phase-6-tasks.md` task 6-9 (`built-in-session-resurrection-6-9`)
**Category**: Dependencies and Ordering / Task Self-Containment
**Change Type**: update-task

**Details**:
Task 5-2's "Do" section step 4 says: "Emit a `*bootstrap.SaverDownError` sentinel (for Phase 6 to surface as a one-line stderr warning) but do NOT short-circuit." Task 5-2's Acceptance Criteria mention "`EnsureSaver` failure surfaces as a `*SaverDownError` sentinel". Task 5-2's Tests include `"it continues past EnsureSaver failures with a SaverDownError sentinel"`.

Phase 6's task 6-9 introduces a `Warning` type and a `SaverDownWarning()` constructor — a different sentinel. There is no `SaverDownError` defined anywhere in the plan. Task 5-2 references a phantom type.

Task 6-8 also references `SaverDownError` in its acceptance "EnsureSaver failure does NOT return a FatalError — uses the soft `SaverDownError` (task 6-9)" but task 6-9 actually defines `SaverDownWarning`, not `SaverDownError`.

The intent is clear (a soft sentinel for the saver-down condition) but the type name and its definition are inconsistent across phases. An implementer landing task 5-2 cannot define `SaverDownError` because no task tells them what its shape is; landing task 6-9 produces `SaverDownWarning` which doesn't match task 5-2's references.

**Current** (in `phase-5-tasks.md`, task 5-2, the "Do" step 4):

```
4. `if err := o.Saver.EnsureSaver(); err != nil` → log best-effort, continue. Per spec "Failure Modes → `_portal-saver` creation fails at bootstrap": "retries a small number of times. On persistent failure: log, emit stderr warning, continue bootstrap without the save daemon." Emit a `*bootstrap.SaverDownError` sentinel (for Phase 6 to surface as a one-line stderr warning) but do NOT short-circuit.
```

And the corresponding **Acceptance Criteria** bullet:

```
- [ ] `EnsureSaver` failure surfaces as a `*SaverDownError` sentinel AND allows `Run` to continue with the remaining steps.
```

And the test:

```
- `"it continues past EnsureSaver failures with a SaverDownError sentinel"`
```

**Proposed**:

In task 5-2 "Do" step 4:

```
4. `if err := o.Saver.EnsureSaver(); err != nil` → log best-effort, continue. Per spec "Failure Modes → `_portal-saver` creation fails at bootstrap": "retries a small number of times. On persistent failure: log, emit stderr warning, continue bootstrap without the save daemon." Capture the underlying error in a Phase-6-bound buffer (via the `Warnings []bootstrap.Warning` accumulator that task 6-9 introduces); for Phase 5's slice, `EnsureSaver` failure is logged via the orchestrator's logger and execution continues. The `Warning` type lands in Phase 6 task 6-9; this task's only obligation is "continue, log, do not short-circuit". No `SaverDownError` sentinel type — soft warnings flow through the `[]Warning` accumulator pattern.
```

Update the **Acceptance Criteria** bullet:

```
- [ ] `EnsureSaver` failure does NOT short-circuit `Run` — subsequent steps still execute. The failure is logged via the orchestrator's logger; Phase 6 task 6-9 wires the user-visible warning through the `Warnings []bootstrap.Warning` accumulator.
```

Update the **Tests** entry:

```
- `"it continues past EnsureSaver failures and logs a warning without short-circuiting"`
```

In task 6-8 (`phase-6-tasks.md`), update the corresponding acceptance criterion:

```
- [ ] `EnsureSaver` failure does NOT return `FatalError` — produces a soft `bootstrap.Warning` via the accumulator pattern (task 6-9).
```

And the corresponding test:

```
- `"EnsureSaver failure does NOT return a FatalError (soft path — produces a Warning instead)"`
```

**Resolution**: Pending
**Notes**: This is purely a naming consistency fix; the design intent (soft warning, no fatal) is unchanged.
