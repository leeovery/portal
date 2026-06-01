# Plan: Portal Observability Layer

## Phases

### Phase 1: Logging foundation and call-site migration
status: approved
approved_at: 2026-06-01

**Goal**: Establish the `internal/log` package (slog-based, swappable-handler indirection, `For`/`Init`/`Close`/`SetTestHandler`) with the locked level contract, `process_role` resolution, and the `main` exit shape; migrate every existing call site off `internal/state.Logger` in one big-bang sweep.

**Why this order**: Nothing else in the spec can be instrumented until the single logging owner exists and the legacy logger is gone. The existing codebase is the foundation; this phase integrates the new package with established conventions and proves both new behaviour and that no existing logging breaks.

**Acceptance**:
- [ ] `internal/log` exposes `Init`, `For`, `Close`, `SetTestHandler` with the package-init swappable-handler indirection; `For` is safe before `Init` and returns a non-nil logger
- [ ] Baseline attrs (`component`, `pid`, `version`, `process_role`) are injected per-record by the configured handler, present on package-init children created before `Init`
- [ ] `PORTAL_LOG_LEVEL` resolves DEBUG/INFO/WARN/ERROR with default INFO; an invalid value falls back to INFO with one startup WARN
- [ ] `process_role` is resolved from `os.Args` prefix-matching to one of the six closed values, defaulting to `bootstrap`
- [ ] `main` routes all termination through the single-`os.Exit` shape (clean/error/panic) and `internal/state.Logger`, `Component*` constants, the pipe-delimited format, and `NopLogger` are deleted
- [ ] All former `state.Logger` call sites and the `*state.Logger` test-mock surfaces (`bootstrapDeps` and friends) compile against `*slog.Logger`; `go test ./...` is green

#### Tasks
status: approved
approved_at: 2026-06-01

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| portal-observability-layer-1-1 | Create `internal/log` package skeleton with swappable-handler indirection and `For()` | For before Init returns valid non-nil logger, concurrent For/handler-swap, empty component string |
| portal-observability-layer-1-2 | Resolve log level from `PORTAL_LOG_LEVEL` with INFO default and invalid-value fallback | unset→default info, mixed-case, surrounding whitespace, invalid→fallback info, legacy "warning" no longer accepted |
| portal-observability-layer-1-3 | Inject per-record baseline attrs (pid/version/process_role) and render component-prefix text in the foundation handler | package-init child created before Init still carries baselines, multi-word values quoted, `time.Duration` default String(), `slog.Group` flattened to dotted keys, component not duplicated in attr list |
| portal-observability-layer-1-4 | Implement `Init`/`Close` public API with stateDir/version/processRole wiring and startTime capture | second Init re-points handler without panic, pre-Init cached loggers route to configured handler after Init, Close owns no control flow, Close before Init safe |
| portal-observability-layer-1-5 | Add `SetTestHandler` test-only seam restoring prior handler via `t.Cleanup` | nested swaps restore in correct order, restore on a test that never logged |
| portal-observability-layer-1-6 | Resolve `process_role` from `os.Args` longest-prefix match against the closed table | bare portal→tui, unknown subcommand→bootstrap, flags interleaved/ignored, `state hydrate` vs `state daemon`, `x`/`attach` aliases→tui |
| portal-observability-layer-1-7 | Adopt the `main` exit shape: single `os.Exit`, panic recovery, `Close` on non-panic path | Execute error→code 1, recovered panic→code 2 with Close skipped, UsageError→code 2, FatalError silent-exit + `IsSilentExitError` preserved |
| portal-observability-layer-1-8 | Migrate intermediate logging seams off `*state.Logger` to `*slog.Logger` | nil-receiver no-op contract removed (callers hold a real logger), component becomes an attr not a method arg, `export_test` `VersionWriterLoggerSeam` type change, `bootstrap.Logger`/`NoopLogger`/`BarrierLogger`/`MigrationLogger`/`SetBarrierLogger`/`SetVersionWriterLogger`/`SaverVersionSeams.WriterLogger`/`restore.Orchestrator.Logger` |
| portal-observability-layer-1-9 | Big-bang rewrite of all production `state.Logger` call sites to `log.For` + slog attrs | `fmt.Sprintf`-in-message converted to attrs, `Component*` constants resolved to literal taxonomy names, attr keys mapped to closed vocabulary, logger-open sites (`state_common`/`state_daemon` rotate=true/`open.go` preview) and `*state.Logger` Deps fields replaced with `*slog.Logger` |
| portal-observability-layer-1-10 | Delete the legacy `internal/state` logger and migrate its tests off `OpenLogger`/`NopLogger` | tests asserting on the old pipe format, `NopLogger` sentinel usages, `restoretest.OpenTestLogger`/`portaltest` log-read helpers, tests depending on the LevelWarn default |

### Phase 2: Rotation, retention, and defensive invariants
status: approved
approved_at: 2026-06-01

**Goal**: Implement the date-aware rotating handler (calendar-daily boundary, `O_CREAT|O_EXCL` first-of-day open, inode-identity reopen, pid-scoped symlink swing, size-cap safety valve, rotated-file immutability), the single-winner retention sweep with per-deletion breadcrumbs, the per-process lifecycle markers (`process: start`/`exit`/`exec`/`panic`), the level-filter bypass for lifecycle markers, and the `log-level resolved` propagation line with its `portaltest` assertion helper.

**Why this order**: This is the core fix for the 2026-05-28 evidence-loss incident and the defensive-detectability guarantee the whole feature exists to deliver. It builds directly on the Phase 1 handler and lifecycle hooks, and every later instrumentation catalog relies on rotation/retention being correct so its trails survive.

**Acceptance**:
- [ ] Each `Handle` reuses the open fd only when both the date matches today and the fd's inode matches the `portal.log` symlink target; otherwise it reopens (date-change runs the new-day path + retention sweep; same-day inode mismatch reopens without the sweeps)
- [ ] First write of a day opens `portal.log.YYYY-MM-DD` via `O_CREAT|O_EXCL` with append-fallback on `EEXIST`, swings the symlink via pid-scoped tmp + atomic rename, and the first-run migration guard deletes a legacy regular-file `portal.log`/`portal.log.old`
- [ ] Past-day files are `chmod 0400` (strict date-parse, skipping the symlink tmp and `swept` sentinel); same-day size-cap overflow rotates to `.N` without sealing the prior segment; the writer is unbuffered
- [ ] The retention sweep runs once per host per day behind the `portal.log.swept.<today>` `O_EXCL` gate, emits one INFO `log-rotate: deleted` per file before deletion, prunes stale sentinels, falls back to 30 days on invalid `PORTAL_LOG_RETENTION_DAYS` with a WARN, and `portal clean --logs` bypasses the gate with `cutoff=today`
- [ ] `process: start`, `log-level resolved`, `process: exit`, `process: panic`, and `process: exec` markers fire exactly once per the four-way terminal classification and bypass the level filter even at WARN/ERROR
- [ ] `portaltest.AssertLogLevelResolved` is available and asserts the resolved level with `source=env` for a given pid; disk-full/`chmod`/symlink failures are best-effort and never crash portal

#### Tasks
status: approved
approved_at: 2026-06-01

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| portal-observability-layer-2-1 | Parse `PORTAL_LOG_ROTATE_SIZE` and `PORTAL_LOG_RETENTION_DAYS` once at handler init | missing/empty→default, K/M/G suffixes + bare bytes, invalid size string→500M fallback, retention non-integer/negative/>365→30 fallback, lowercase/uppercase suffix |
| portal-observability-layer-2-2 | Date-aware fd reuse with inode-identity reopen and first-of-day `O_CREAT\|O_EXCL` open | EEXIST create-race→append fallback, same-day inode mismatch (file unlinked mid-day), ENOENT on symlink target→recreate, date-change vs same-day reopen distinction, first Handle ever (no file yet) |
| portal-observability-layer-2-3 | Pid-scoped atomic symlink swing with crash-leftover reclamation | stale same-pid tmp from prior crash→remove+recreate, concurrent cross-process swing (identical target, last-writer-wins), swing failure leaves prior symlink in place, tmp never collides across pids |
| portal-observability-layer-2-4 | First-run migration guard deleting legacy regular-file `portal.log` / `portal.log.old` | portal.log is regular file→removed, portal.log already a symlink→guard no-ops, portal.log.old absent→tolerated, portal.log absent entirely→no-op, guard never fires on second run |
| portal-observability-layer-2-5 | Past-day `chmod 0400` immutability sweep on the day-roll path | strict date-parse skips symlink-tmp + swept sentinel + non-log siblings, already-0400 skipped, multi-day downtime catches all past days at once, chmod failure→WARN-and-continue, same-day segments NOT sealed |
| portal-observability-layer-2-6 | Same-day size-cap overflow rotation to `portal.log.<today>.N` | max-N discovery (none→1, gaps), EEXIST on N→retry N+1, prior segment NOT chmod'd (peer may hold open fd), peer keeps appending to prior segment (acceptable split), cap never fires in steady state |
| portal-observability-layer-2-7 | Best-effort write path with stderr fallback and unbuffered-writer guarantee | open failure→stderr fallback+continue, write failure mid-record→drop+continue, disk-full/EACCES never propagates to caller, writer is unbuffered (marker in kernel before Info returns), failed symlink swing→writes continue to open fd |
| portal-observability-layer-2-8 | Single-winner retention sweep with per-deletion breadcrumbs and sentinel prune | EEXIST gate loss→return immediately (no emit/no run), invalid `PORTAL_LOG_RETENTION_DAYS`→WARN+default 30, INFO breadcrumb BEFORE delete, os.Remove failure→WARN+continue, strict date-parse excludes sentinel/tmp, stale non-today sentinels pruned, partial-sweep crash self-heals next day |
| portal-observability-layer-2-9 | `portal clean --logs` gate-bypassing sweep with `cutoff=today` | no flag→logs preserved (sweep not triggered), `--logs` bypasses swept.<today> gate, cutoff=today leaves only current file, removes stale swept.* sentinels, reuses the same sweep function (no duplication) |
| portal-observability-layer-2-10 | Lifecycle-marker level-filter bypass in the handler | process lifecycle msgs emitted at WARN/ERROR level, non-process INFO still filtered at WARN, only the closed lifecycle msg set bypasses (not arbitrary `process:` lines), identification by component+msg |
| portal-observability-layer-2-11 | `process: start` and `log-level resolved` emission in `Init` (with invalid-level WARN) | start emitted exactly once as final pre-return action, log-level resolved immediately after start, source=env/default/fallback rendering, fallback also emits the bootstrap WARN, both lines bypass the level filter (visible at warn/error), baseline attrs auto-injected not passed |
| portal-observability-layer-2-12 | `process: exit` emission in `Close` | code attr reflects exitCode, took from startTime non-negative, Close still never calls os.Exit, Close before Init safe (no panic, took bounded), exactly one exit line per Close call |
| portal-observability-layer-2-13 | `process: panic` emission in the `main` recover block | panic emits ERROR `process: panic` with reason, Close still skipped on panic path (no double terminal marker), non-panic path unchanged, four-way classification stays mutually exclusive |
| portal-observability-layer-2-14 | `process: exec` marker at the `AttachConnector` bare-shell handoff | exec marker emitted before syscall.Exec (in kernel pre-image-replace), target=tmux + args=joined argv, SwitchConnector path unaffected (gets normal exit via Close), args logged verbatim |
| portal-observability-layer-2-15 | `portaltest.AssertLogLevelResolved` integration-test assertion helper | matches the correct line by pid attr when multiple processes wrote, fails on absent line (env did not propagate), fails when source≠env, parses the symlink-followed current log, tolerates baseline-attr ordering |

### Phase 3: State-mutation audit trail for user config files
status: approved
approved_at: 2026-06-01

**Goal**: Instrument the `hooks`, `aliases`, and `projects` store mutation methods (`Set`/`Remove`/`Save`/`CleanStale`) at the store seam — INFO on success, WARN on `AtomicWrite` failure — with the closed `op` vocabulary, the per-file key attr, `value`/`via` optionals, no-op DEBUG handling, batch summaries, and the one sanctioned `migrateConfigFile` emission site.

**Why this order**: This is an independent vertical slice over the config stores (the seam the spec names "PR 2"), depending only on the Phase 1 logging foundation. It directly addresses the `hooks.json`-wipe diagnosability gap from the motivating incident and is independently greppable via `grep "hooks:"`.

**Acceptance**:
- [ ] Every in-scope store mutation emits one INFO on success / one WARN on failure under its owning component, with `op` from the closed value space and the correct key attr (`hook_key`/`alias`/`project`)
- [ ] A `set` whose key+value already match emits DEBUG `op=set-noop` (not INFO); `value` is present for `set`/`modify` and absent for `rm`/`clean-stale`
- [ ] WARN paths carry the correct `error_class`: AtomicWrite-phase values for a whole-mutation failure, `unexpected` for a per-entry batch failure
- [ ] Batch operations (`CleanStale`) emit per-entry DEBUG, per-entry WARN on mid-loop failure, and one INFO summary with `entries` and `entries_failed`
- [ ] `migrateConfigFile` emits one INFO per migrated file under the file's owning component with `op=migrate via=migrate`, and `AtomicWrite` remains audit-unaware (no logging inside it, none scattered at callers)

#### Tasks
status: approved
approved_at: 2026-06-01

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| portal-observability-layer-3-1 | Expose `AtomicWrite` write-phase sentinels for `error_class` mapping (no logging in `fileutil`) | AtomicWrite stays log-free/audit-unaware, fsync phase has no current AtomicWrite step (close→write-failed-fsync — `[needs-info]`), unknown/wrapped error→safe default phase, sessions.json caller unaffected, errors.Is unwraps through %w |
| portal-observability-layer-3-2 | Instrument `hooks.Store` `Set`/`Remove` (set/modify/rm + set-noop DEBUG, value/via/error_class) | set-noop (key+event exist, value matches)→DEBUG and skips Save, set-vs-modify from pre-write Load, rm of absent key, value omitted for rm, WARN carries write-failed-* error_class, via=cli (hooks set/rm) vs internal (migrate-rename Save) |
| portal-observability-layer-3-3 | Instrument `hooks.Store` `CleanStale` batch (per-entry DEBUG, INFO summary, per-entry WARN) | zero removals→no Save + summary handling, whole-batch Save failure→write-failed-* not per-entry unexpected, single batched Save means per-entry WARN reachability (`[needs-info]`), entries_failed omitted when zero |
| portal-observability-layer-3-4 | Instrument `project.Store` `Upsert`/`Rename`/`Remove`/`CleanStale` (op vocabulary, project/value/via/error_class) | Upsert found→modify / not-found→set / unchanged→set-noop, Rename no-op when path absent (no Save/no INFO), Remove of absent path still Saves, CleanStale batch summary entries/entries_failed, via=cli (TUI) vs internal (session prepare Upsert) |
| portal-observability-layer-3-5 | Instrument `alias.Store` mutation+persist seam (alias/op/value/via, set-noop) | in-memory Set/Delete separate from persist (emission-point `[needs-info]`), Save uses os.WriteFile not AtomicWrite (error_class phase `[needs-info]`), set/modify/set-noop for alias, Delete of absent alias errors before Save, via=cli, value=path |
| portal-observability-layer-3-6 | Emit `migrateConfigFile` INFO per migrated file (op=migrate, via=migrate, owning component) | component threaded from the three configFilePath sites, no-op when oldPath absent / newPath exists (no INFO), migrate WARN error_class for os.Rename failure (`[needs-info]`), PR-1-window migration unlogged (accepted), whole-file-move key attr value (`[needs-info]`), AtomicWrite/other callers stay audit-unaware |

### Phase 4: Diagnostic context preservation at boundaries
status: approved
approved_at: 2026-06-01

**Goal**: Sweep the four external-boundary classes (`exec.Cmd`, `internal/tmux` commander, `os` syscalls, `io`/FIFO reads) so every wrapped error embeds stderr/errno/phase context, and close the four enumerated existing-code defects (`defaultIdentifyPS`, `escalateKillToSIGKILL`, `ShowGlobalHooks` asymmetry, uncommented defensive branches).

**Why this order**: This is an error-wrapping concern distinct from emitting log calls — it guarantees the failure context the later instrumentation will log actually survives to the log site. It depends on the Phase 1 level discipline (expected/unexpected classification) but is otherwise self-contained and verifiable per boundary.

**Acceptance**:
- [ ] Every `exec.Cmd` boundary captures stderr and embeds exit status/signal + trimmed stderr in the wrapped error; no `_, _ = cmd.Run()` or `Output()` without `Stderr` assignment remains in scope
- [ ] `RealCommander.Run`/`RunRaw` embed exit code + tmux argv + trimmed stderr on non-zero exit, and detect/wrap `ErrNoSuchSession`/`ErrEmptyPaneList` sentinels from stderr text
- [ ] `os`-package errors preserve the underlying `*os.PathError` via `%w`; EOF/timeout on FIFO/io reads classify as `expected` while mid-stream errors wrap with path context
- [ ] The `"error"` attr at log sites passes the wrapped error directly (not `.Error()`) so the handler renders the full chain
- [ ] All four enumerated gap-closure sites are fixed per their prescribed remedy (stderr embed, escalation DEBUG breadcrumb, missing WARN branch, code comment)

### Phase 5: Cycle summaries and saver/daemon lifecycle catalogs
status: approved
approved_at: 2026-06-01

**Goal**: Instrument every cataloged cycle (daemon tick, bootstrap orchestration + per-step, restore phase A/B, the three clean sweeps) with one INFO summary carrying the closed unit/sub-category counts + `took` and per-item DEBUG/WARN, and emit the closed saver/daemon lifecycle event taxonomy (placeholder created, destroy-unattached off, respawn-daemon, daemon ready, kill-barrier started/escalated, placeholder died; daemon lock acquired, self-eject, shutdown).

**Why this order**: These catalogs instrument the long-running machinery (daemon, bootstrap, restore, clean, saver) that the motivating incident centred on, consuming the Phase 1 foundation and the Phase 4 boundary-preserved errors. They deliver the operator-level "reconstruct a window from summaries" capability and the saver/daemon forensic trail.

**Acceptance**:
- [ ] Each cataloged cycle emits exactly one INFO summary with the verb-phrase + closed unit/outcome keys (`sessions`/`panes`/`entries`/`steps`/`reaped`/`killed`/`skipped`/`unset`) + sub-category counts (`natural_churn`/`anomalous`/`entries_failed`) + `took`
- [ ] Per-item events inside cycles are DEBUG (steady-state) and WARN (anomaly), with anomalous counts reflected in the summary's `anomalous` attr
- [ ] Every saver lifecycle site emits its cataloged INFO line with the required attrs; the redundant `daemon: spawn` is dropped and its `tmux_pane` rides on `daemon: lock acquired`
- [ ] Daemon `self-eject` emits `daemon: self-eject ticks=N threshold=N` then `log.Close(0)` then `os.Exit(0)` (no `daemon: shutdown` on that path); normal `shutdown` carries `reason` and `flush_completed`
- [ ] Hysteresis-internal probe failures are DEBUG per tick with one INFO on the trip; reason value spaces match the closed catalogs

### Phase 6: Hydrate-helper forensic trail
status: approved
approved_at: 2026-06-01

**Goal**: Instrument the hydrate helper's exec chain (`execShellOrHookAndExit` path in `cmd/state_hydrate.go`) with the hook-lookup DEBUG breadcrumb, the four exit-path INFO lines (fifo missing, signal timeout, scrollback missing, scrollback replayed), and the terminal `hydrate: exec` INFO line structurally parallel to `process: exec`.

**Why this order**: The hydrate helper is the explicit "undiagnosable per-pane recovery" surface, instrumented last because it is a small, self-contained forensic slice that depends on the foundation (Phase 1), the exec-marker pattern (Phase 2), and the boundary-preserved lookup errors (Phase 4). It completes the `grep "hydrate:"` reconstruction guarantee up to the exec moment.

**Acceptance**:
- [ ] The helper emits a DEBUG `hook lookup` breadcrumb with `hook_key` and `result` ∈ {hit, miss, error} (plus `error` attr on error) before the exec
- [ ] Each of the four exit paths emits its cataloged INFO line (`fifo missing`, `signal timeout` with `took=3s`, `scrollback missing`, `scrollback replayed` with `bytes`/`took`) followed by the exec INFO
- [ ] The terminal `hydrate: exec` INFO carries `target`, `args`, and `hook_present`, structurally parallel with `process: exec`; `target` is distinct from the reserved `path` attr
- [ ] Exact line ranges in `cmd/state_hydrate.go` are pinned against the live file (the spec's line numbers are hints), and `grep "hydrate:" portal.log` reconstructs every invocation up to the exec moment
