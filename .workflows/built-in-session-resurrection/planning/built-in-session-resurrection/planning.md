# Plan: Built-In Session Resurrection

## Phases

### Phase 1: Portal state CLI scaffolding & tmux hook registration
status: approved
approved_at: 2026-04-23

**Goal**: Establish the `portal state` command namespace, the tmux ≥ 3.0 version guard, and the idempotent global-hook registration / removal plumbing — the foundation every subsequent phase builds on.

**Why this order**: Subsequent phases register hooks, add subcommands, and depend on version-guarded bootstrap. Without this scaffold nothing else can be wired into the tmux event pipeline, and the hook-registration idempotency contract must be pinned down before any callers rely on it.

**Acceptance**:
- [ ] `portal state` namespace exists with user-facing subcommands registered (stubs are acceptable at this phase) and internal subcommands hidden from `--help`
- [ ] `PersistentPreRunE` rejects tmux < 3.0 with the specified user-facing error before any `set-hook` call
- [ ] `set-hook -ga` registration runs idempotently: a given (event, command-substring) pair is appended only when `show-hooks -g` shows no matching Portal entry
- [ ] Save-trigger events (`session-created`, `session-closed`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out`) and hydration-trigger events (`client-attached`, `client-session-changed`) are registered with the `command -v portal` defensive guard
- [ ] Hook removal via `set-hook -gu '<EVENT>[N]'` runs in reverse index order and leaves non-Portal entries untouched
- [ ] All new tmux invocations go through the existing `Commander` interface and are covered by table-driven unit tests with canned `show-hooks` outputs
- [ ] `portal state cleanup` command exists and removes Portal's hook entries (daemon teardown and `--purge` land in Phase 6)

#### Tasks
status: approved
approved_at: 2026-04-23

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| built-in-session-resurrection-1-1 | Scaffold `portal state` command namespace with stub subcommands | hidden subcommands excluded from `--help`, bare `portal state` prints help, exemption applies to nested subcommands |
| built-in-session-resurrection-1-2 | Add tmux version detection and >= 3.0 guard | suffixed versions (`3.0a`, `3.3-rc`), `tmux-next`/OpenBSD build strings, malformed output, missing binary, leading/trailing whitespace |
| built-in-session-resurrection-1-3 | Wire the version guard into `PersistentPreRunE` (memoized, exempt-aware) | repeated `PersistentPreRunE` invocations run check once, exempt commands bypass entirely, failure short-circuits before any `set-hook` call |
| built-in-session-resurrection-1-4 | Add `show-hooks` / `set-hook -ga` / `set-hook -gu` wrappers on `tmux.Client` | empty `show-hooks -g` output, commands containing single quotes, Commander errors bubble verbatim |
| built-in-session-resurrection-1-5 | Parse `show-hooks -g` output into an indexed per-event map | sparse indices from prior removals, hyphenated event names, leading whitespace, unrelated lines, non-indexed entries |
| built-in-session-resurrection-1-6 | Implement content-based idempotent registration (`RegisterHookIfAbsent`) | unrelated user/plugin entries on same event preserved, Portal entry already present is a no-op, `show-hooks` failure propagates without partial append |
| built-in-session-resurrection-1-7 | Register the full Phase 1 hook table at bootstrap | partial prior registration, per-event failure does not silently skip later events, double-bootstrap produces no duplicates |
| built-in-session-resurrection-1-8 | Implement Portal hook removal in reverse index order | sparse arrays with Portal and non-Portal entries interleaved, already-absent entries are a no-op, two Portal entries on same event (`notify` + `migrate-rename` on `session-renamed`) both removed |
| built-in-session-resurrection-1-9 | Implement the Phase 1 slice of `portal state cleanup` | no tmux server running is not an error, partial failure still attempts subsequent removals, running twice in a row is a clean no-op the second time, `--purge` parses as a boolean flag without error (body deferred to Phase 6) |

### Phase 2: Save daemon, triggers, and on-disk state format
status: approved
approved_at: 2026-04-23

**Goal**: Bring up `_portal-saver` hosting `portal state daemon`, wire the dirty-flag + 1-second ticker capture loop, and land the full `sessions.json` + `scrollback/` directory layout with atomic writes, content-hash dedup, GC, and version-marker-driven daemon restart.

**Why this order**: Save must precede restore — you cannot restore what was never saved. This phase produces the on-disk artifacts Phase 3 will read. Hook registration from Phase 1 already routes `portal state notify` correctly; this phase adds the receiving end.

**Acceptance**:
- [ ] Bootstrap creates `_portal-saver` idempotently with `set-option -t _portal-saver destroy-unattached off`, backed by PID-file + `signal(0)` liveness check (not `#{pane_current_command}`)
- [ ] `portal state notify` touches `save.requested` and exits with no tmux calls, no state reads, and no conditional logic
- [ ] Daemon 1-second ticker captures when dirty OR ≥30s since last save; honours `@portal-restoring` (skip entire tick) and `@portal-skeleton-<paneKey>` (skip pane) via a single `show-options -sv` enumeration per cycle
- [ ] `sessions.json` schema v1 is written with every documented field (version, saved_at, sessions[].name, environment, windows[].{index,name,layout,zoomed,active}, panes[].{index,cwd,active,current_command,scrollback_file})
- [ ] Per-session `show-environment` values round-trip through capture → save → restore
- [ ] Per-pane scrollback is captured via `capture-pane -e -p -S -`, xxhash-deduped, and only rewritten on hash change; daemon seeds the hash map from disk on startup to avoid full-rewrite on every daemon start
- [ ] Post-commit GC removes any `scrollback/*.bin` not referenced by the freshly-written index; runs synchronously after `sessions.json` rename
- [ ] Sessions whose names begin with `_` are excluded from capture
- [ ] Daemon writes `daemon.version` and `daemon.pid` on startup; clears `save.requested` defensively on startup; rotates `portal.log` on startup if ≥1 MB
- [ ] Version mismatch (including empty / `"dev"` version) triggers `kill-session -t _portal-saver` + recreate on the next bootstrap
- [ ] SIGHUP and SIGTERM handlers flush a final atomic write via `AtomicWrite`, unless `@portal-restoring` is set

#### Tasks
status: approved
approved_at: 2026-04-23

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| built-in-session-resurrection-2-1 | Add state-directory path helpers and paneKey sanitizer | `/`, null bytes, leading `.` in session names, collision hash-suffix, directories created with `0700`, XDG overrides, per-file env vars |
| built-in-session-resurrection-2-2 | Implement `portal state notify` subcommand | state directory missing (create with `0700`), existing file bumped mtime, permission error surfaces via exit code only, stray args are harmless |
| built-in-session-resurrection-2-3 | Define `sessions.json` v1 schema types and encoder/decoder | unknown fields ignored on decode, missing optional fields round-trip, empty `environment` map, empty `panes` slice serializes as `[]` not `null` |
| built-in-session-resurrection-2-4 | Add daemon.pid + signal(0) liveness check and version-marker read/write helpers | PID file missing, unparseable, whitespace, PID reused by a different process (accepted), version file missing, empty version, `"dev"` version |
| built-in-session-resurrection-2-5 | Implement idempotent `_portal-saver` bootstrap with defensive `destroy-unattached off -t` | `has-session` true but PID liveness false → `kill-session` then recreate, `-t` scoping (never `-g`), concurrent bootstraps race, transient new-session failure then retry, `destroy-unattached off` is idempotent |
| built-in-session-resurrection-2-6 | Wire version-marker-driven restart into bootstrap | version file absent while `_portal-saver` live (accepted unnecessary restart), empty-string version, `"dev"` always restarts, release semver equality, `kill-session` tolerant of already-dead session |
| built-in-session-resurrection-2-7 | Scaffold `portal state daemon` entrypoint with startup side-effects and signal wiring | state directory missing, existing `portal.log.old` replaced on rotation, log file absent (no rotation), repeated startup overwrites pid/version defensively, SIGHUP and SIGTERM both cancel context |
| built-in-session-resurrection-2-8 | Implement structural capture: enumerate sessions, panes, and per-session environment | `_*` sessions filtered, zero live sessions → empty `sessions` array (not `null`), environment `-r` removed-form entries ignored, multi-byte UTF-8 session names, empty environment map round-trips |
| built-in-session-resurrection-2-9 | Implement per-pane scrollback capture with xxhash content dedup and startup seed | first-ever capture (empty map), identical scrollback across ticks produces zero writes, empty scrollback, unreadable existing `.bin` during seed logs and continues, hash collision accepted per xxhash guarantees |
| built-in-session-resurrection-2-10 | Atomic commit of `sessions.json` plus post-commit orphan GC | zero-change cycle skips both write and GC, GC tolerates `ENOENT`, GC preserves files for still-skeleton-marked panes, rename failure leaves prior state intact, per-file GC failure logs and continues |
| built-in-session-resurrection-2-11 | Marker-aware capture via single `show-options -sv` per cycle | `show-options -sv` empty output, unrelated `@` options present, marker values other than `1` still treated as present, skeleton-marked pane's `.bin` and `sessions.json` entry preserved, marker cleared mid-cycle → next cycle captures |
| built-in-session-resurrection-2-12 | Ticker trigger logic, defensive startup clear, and shutdown final flush | dirty flag set during restore suppressed by `@portal-restoring`, 30s max-gap fires with zero dirty signals, `@portal-restoring` set at shutdown skips final flush, in-flight capture started before flag set commits normally, `save.requested` race between clear and next notify picked up on following tick |

### Phase 3: Skeleton restore and lazy scrollback hydration
status: approved
approved_at: 2026-04-23

**Goal**: On bootstrap, read `sessions.json` and skeleton-restore every missing saved session (windows, panes, layout, zoom, session environment, CWDs) wired to the hydrate-helper FIFO mechanism; on client attach, `signal-hydrate` unblocks the helpers which dump scrollback with ANSI fidelity and degrade locally on every failure mode the spec enumerates.

**Why this order**: Requires Phase 2's on-disk format as input and Phase 1's `client-attached` / `client-session-changed` registration as the attach-time trigger. Must land before Phase 5 can delete `WaitForSessions`.

**Acceptance**:
- [ ] `Restore()` skips live sessions by name, logs and skips empty-pane sessions, and skeleton-restores each remaining saved session; a missing or unparseable `sessions.json` is a non-fatal no-op warning
- [ ] Windows and panes are created in saved structural order; `select-layout` applies the saved string and falls back to `select-layout tiled` with a log on failure; active pane and zoom are re-applied in the documented order (layout → select-pane → conditional `resize-pane -Z`)
- [ ] Saved session environment is applied via `set-environment -t <session>` after `new-session` but before any `new-window` / `split-window`, so subsequent panes inherit the saved env
- [ ] Each skeleton-created pane runs `sh -c 'portal state hydrate --fifo F --file S --hook-key K; exec $SHELL'` and has `@portal-skeleton-<paneKey>` set via `set-option -s` using the **live** paneKey
- [ ] Live paneKey drift (base-index / pane-base-index changes) is handled: helpers receive the saved scrollback path and saved hook key via flags; live paneKey is used for markers and FIFO paths; the first post-hydration capture writes under the live paneKey and GC removes the stale file
- [ ] FIFOs are created via `os.Remove` (ignore `ENOENT`) + `syscall.Mkfifo(path, 0600)` before each pane is created; a state-dir sweep removes stale `hydrate-*.fifo` files not matching an active pane
- [ ] `portal state hydrate --fifo F --file S --hook-key K` implements the 3-second blocking FIFO read, reset preamble (`\033[?25h\033[?1049l\033[0m`), content dump, reset postamble + CRLF, 100ms settle sleep, marker-unset via `set-option -su`, and the timeout / missing-file degradation paths exactly as specified. Hook firing arrives in Phase 4 — for this phase the helper `exec $SHELL` on all success paths.
- [ ] `portal state signal-hydrate <session>` enumerates panes via `list-panes -t`, writes one byte per skeleton-marked pane with `O_WRONLY | O_NONBLOCK` and the 10/20/40/…/≤500ms retry ladder on `ENXIO`/`EAGAIN`, is idempotent across `client-attached` + `client-session-changed`, and never touches the skeleton marker
- [ ] `@portal-restoring` is set before `_portal-saver` creation at bootstrap and cleared only after skeleton-restore completes
- [ ] Integration test on an isolated `tmux -L` socket verifies a multi-session, multi-window save round-trips structure + ANSI scrollback

#### Tasks
status: approved
approved_at: 2026-04-23

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| built-in-session-resurrection-3-1 | Add `sessions.json` reader for bootstrap consumption | missing file is non-fatal no-op, unparseable JSON logs warning and returns skip-all sentinel, empty `sessions` array, unknown fields tolerated, future `version` value logged as warning and skipped |
| built-in-session-resurrection-3-2 | Add per-pane FIFO create helper (`os.Remove` + `syscall.Mkfifo 0600`) | `os.Remove` ignores `ENOENT`, pre-existing stale FIFO replaced cleanly, state directory missing is created with `0700`, permission / EEXIST race surfaces as error, non-FIFO file at path is removed then recreated |
| built-in-session-resurrection-3-3 | [BLOCKED — needs planning decision on live-index source] Skeleton-create one saved session: `new-session` + `set-environment` + windows/panes with hydrate command in saved structural order | empty `environment` map skips set-environment cleanly, `set-environment` runs after `new-session` but before any `new-window` / `split-window`, session name with multibyte UTF-8, sanitized-collision session name uses hash-suffixed paneKey, saved `--file` / `--hook-key` flag values are taken from `sessions.json` (not live re-derivation), live-index source pinned to one of Option A (predict via base-index/pane-base-index) / B (re-query + symlink) / C (UUID FIFO names) |
| built-in-session-resurrection-3-4 | Apply per-window layout, active pane, and zoom with `tiled` fallback | `select-layout` error logs warning and falls back to `select-layout tiled`, `zoomed=false` skips `resize-pane -Z`, saved active index maps to re-queried live pane id, corrupt layout string does not abort remaining windows, single-pane window still applies active correctly |
| built-in-session-resurrection-3-5 | Re-query live paneKey after pane creation and set `@portal-skeleton-<paneKey>` via `set-option -s` | `base-index` / `pane-base-index` changed between save and restore, marker uses `-s` (server scope) not `-g`, live pane count equals saved pane count (sanity), live paneKey differs from saved paneKey (both coexist until first post-hydration capture), markers set per-pane before any signal path can fire |
| built-in-session-resurrection-3-6 | Implement top-level `Restore()` orchestrator with per-session error isolation | missing `sessions.json` returns cleanly, unparseable index emits single-line stderr warning and logs (no partial restore), live session name collision skips (never clobbers), `panes` empty array logs warning and skips that session, per-session error logged and next session continues, `_`-prefixed session names in index are skipped defensively |
| built-in-session-resurrection-3-7 | Wrap `Restore()` with `@portal-restoring` set-before / clear-after using `set-option -s` | set-option failure bubbles as fatal (per Observability), marker cleared even on Restore internal error (deferred), clear uses `set-option -su` (not empty-string assignment), marker is server-scope (volatile across server restart), idempotent if already set from a prior crashed bootstrap |
| built-in-session-resurrection-3-8 | Implement `portal state hydrate` signal-arrived success path | FIFO open uses `O_RDONLY`, signal-arrives reads single byte then closes and `os.Remove`s FIFO, marker unset uses `-su` (empty-string would NOT remove), `exec $SHELL` replaces helper process, large scrollback file streamed without buffering entire contents, hook firing is NOT performed (deferred to Phase 4) |
| built-in-session-resurrection-3-9 | Implement `portal state hydrate` 3-second timeout path | timeout measured via goroutine + `time.After` + `select` (FIFO reads don't time out natively), no content dump and no 100ms sleep on this path, marker stays set so next attach re-signals, log entry identifies `--hook-key` for diagnosis, FIFO unlinked on timeout too (prevent orphan) |
| built-in-session-resurrection-3-10 | Implement `portal state hydrate` scrollback-file-missing path | ENOENT vs permission error both degrade but log distinctly, marker cleared via `set-option -su` so save loop resumes, no 100ms sleep (nothing was dumped), file becomes readable later does not retry (permanent for this attempt), partial-read I/O error mid-dump treated as this path |
| built-in-session-resurrection-3-11 | Implement `portal state signal-hydrate <session>` with `O_WRONLY \| O_NONBLOCK` retry ladder | idempotent across `client-attached` + `client-session-changed` (second write hits closed/unlinked FIFO harmlessly), never touches skeleton marker (helper owns unset), panes without marker are no-op, retries exhaust → log warning and move on (marker stays set for next attach), non-skeleton session (zero markers) is a zero-write no-op, session argument refers to non-existent session logs and returns 0 |
| built-in-session-resurrection-3-12 | Bootstrap state-dir sweep of orphan `hydrate-*.fifo` files | state directory missing tolerated, non-FIFO files matching glob pattern left untouched (only FIFOs removed), FIFO corresponding to a live pane preserved, paneKey sanitization round-trips for comparison, sweep failure per-file logs and continues |
| built-in-session-resurrection-3-13 | Integration test on isolated `tmux -L` socket: multi-session, multi-window save→restore round-trip of structure + ANSI scrollback | isolated `tmux -L <unique>` socket never contaminates user sessions, socket killed in `t.Cleanup` on success and failure, base-index / pane-base-index variations covered, scrollback bytes compared with ANSI SGR preserved, `@portal-skeleton` markers cleared after signal-hydrate + helper dump, restore is re-runnable (skips live) on re-invocation |

### Phase 4: Resume-hook lifecycle migration
status: approved
approved_at: 2026-04-23

**Goal**: Move hook firing out of the old attach-time `ExecuteHooks` path into the hydrate helper's exec chain; add `session-renamed` key migration via a separate internal subcommand; update `CleanStale` to run unconditionally.

**Why this order**: Requires Phase 3's hydrate helper to exist — that is the new firing point. Cannot run earlier without breaking currently-shipping hook behaviour. Earlier phases intentionally leave the old executor wired up so the system stays usable during intermediate phases; this phase performs the cutover.

**Acceptance**:
- [ ] `internal/hooks/executor.go`, `cmd/hook_executor.go`, all `ExecuteHooks` call sites in `cmd/open.go` and `cmd/attach.go`, and any `@portal-active-<pane>` registration-time marker logic are deleted
- [ ] The hydrate helper reads `hooks.json` after its 100ms settle sleep, looks up by the `--hook-key` argument (not live pane position), and `exec`s `sh -c 'HOOK; exec $SHELL'` on match or `$SHELL` otherwise — on both the successful-dump and the missing-file success paths
- [ ] Hook firing does NOT happen on the 3-second timeout path; the timeout path also does NOT clear the skeleton marker (next attach re-signals)
- [ ] `portal state migrate-rename <old> <new>` body is implemented (task 4-3): rewrites every `<old>:*` key in `hooks.json` to `<new>:*` atomically via `AtomicWrite` and logs best-effort on failure
- [ ] **Conditional on `[needs-info]` resolution in task 4-4**: `portal state migrate-rename` is registered against `session-renamed` alongside `portal state notify` using the same content-based idempotency pattern. Until the prior-name argument-source decision (Route A vs Route B) lands, this bullet is BLOCKED and the migration body is dead code — hooks for a renamed session get orphaned and are pruned by `CleanStale` (the spec's documented "best-effort / re-register" failure mode applies).
- [ ] `CleanStale` no longer has the `len(livePanes) == 0` early return; runs unconditionally as bootstrap step 7 and from `portal clean`
- [ ] Stale-detection criteria remain unchanged: structural-key mismatch against `list-panes -a` only; binary-missing and `projects.json`-absent are NOT staleness signals
- [ ] `portal hooks set`, `portal hooks list`, `portal hooks rm --on-resume` retain their existing user-facing surface; behavioural change is documented: hooks fire on skeleton-restored panes only, not on live detach/reattach within a server lifetime

#### Tasks
status: approved
approved_at: 2026-04-23

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| built-in-session-resurrection-4-1 | Add `hooks.LookupByKey` helper for on-resume lookup by saved structural key | missing `hooks.json` returns empty no-hook, malformed JSON returns empty no-hook (matches `Store.Load`), key absent returns empty, key present without `on-resume` event returns empty, raw session name containing `:` round-trips verbatim, IO error surfaces distinctly from "no hook" |
| built-in-session-resurrection-4-2 | Wire hook firing into hydrate helper's signal-arrived and file-missing success paths | signal-arrived path with registered hook execs `sh -c 'HOOK; exec $SHELL'`, signal-arrived path without hook execs `$SHELL`, file-missing path with hook execs hook-chain, file-missing path without hook execs `$SHELL`, 3s-timeout path never reads hooks and never fires, hook lookup IO error degrades to bare `$SHELL` with log warning, lookup uses `--hook-key` flag verbatim (not live pane position), hook command containing single quotes is exec-safe |
| built-in-session-resurrection-4-3 | Implement `portal state migrate-rename <old> <new>` body — rewrite `<old>:*` keys in `hooks.json` | zero matching keys is clean no-op (no write), multiple `<old>:window.pane` keys all rewritten, `<old>` is prefix of another session name (`old` vs `old-2`) only exact-session-segment matches, `<new>:X` collision already present → new-name entries overwrite with log warning, `AtomicWrite` failure logged best-effort and exits non-zero, malformed `hooks.json` treated as empty (no-op), missing positional args rejected with usage error, empty `<new>` rejected, session names containing `:` preserved in raw form |
| built-in-session-resurrection-4-4 | [BLOCKED — needs planning decision on prior-name argv source] Register `portal state migrate-rename` on `session-renamed` alongside `notify` via content-based idempotency | coexists with existing `portal state notify` entry on same event (both substrings matched per-command, no cross-contamination), re-running bootstrap appends no duplicate `migrate-rename` entry, old/new session names sourced from a route that satisfies the spec's atomic-migration contract (planning to pin Route A: tmux format variable, or Route B: daemon-side rename-delta side-band), `command -v portal` guard wraps the invocation, `show-hooks` parsing distinguishes the two Portal substrings, first-ever bootstrap on server without either entry appends both |
| built-in-session-resurrection-4-5 | Delete `ExecuteHooks` / `internal/hooks/executor.go` / `cmd/hook_executor.go` and all attach-time firing call sites | `internal/hooks/executor.go` + `executor_test.go` removed, `cmd/hook_executor.go` removed, `HookExecutor` field removed from `AttachDeps`, `hookExecutor(name)` call in `cmd/attach.go` removed, `ExecuteHooks` reference in `cmd/open.go` + its test fixture removed, now-unused `TmuxOperator` / `HookRepository` / `PaneLister` / `KeySender` interfaces pruned, `MarkerName` helper retained for task 4-6, package still compiles with tests passing |
| built-in-session-resurrection-4-6 | Strip `@portal-active-<pane>` registration-time marker logic from `portal hooks set` / `portal hooks rm` | `hooks set` no longer calls `SetServerOption(hooks.MarkerName, "1")`, `hooks rm` no longer calls `DeleteServerOption(hooks.MarkerName)`, `HooksDeps.OptionSetter` and `OptionDeleter` fields deleted, `hooks.MarkerName` helper deleted (no remaining callers), user-facing CLI shape unchanged (`set --on-resume`, `list`, `rm --on-resume`), existing `hooks_test.go` tests updated to not expect marker tmux calls, `hooks set` still writes to `hooks.json` correctly with no tmux side-effect |
| built-in-session-resurrection-4-7 | Remove `len(livePanes) == 0` early return in `cmd/clean.go` so `CleanStale` runs unconditionally | zero live panes + existing hooks → all entries pruned, `ListAllPanes` error path still safety-nets (returns without pruning), stale-detection criteria unchanged (structural-key mismatch only; binary-missing and `projects.json`-absent are NOT signals), `portal clean` output still lists removed hook keys, invocation from bootstrap step 7 (wired in Phase 5) also prunes under same semantics, hooks.json with zero entries is still a clean no-op |

### Phase 5: Bootstrap integration and `WaitForSessions` removal
status: approved
approved_at: 2026-04-23

**Goal**: Stitch the full `PersistentPreRunE` sequence together in the specified order, delete `WaitForSessions` / `bootstrapWait` and their call sites, and land the TUI loading-page 1.2-second minimum-display treatment.

**Why this order**: Previous phases land pieces individually; this phase guarantees they compose correctly in the documented order and removes the now-obsolete polling code that existed only because Portal did not own restoration. Must follow Phase 3 because `Restore()` is the functional replacement for `WaitForSessions`.

**Acceptance**:
- [ ] `PersistentPreRunE` executes steps 1–8 in the documented order for every non-exempt command; exempt commands (`version`, `init`, `help`, `alias`, `clean`, and every `portal state …` subcommand including the internal ones) skip bootstrap
- [ ] `@portal-restoring` is set in step 3 before `_portal-saver` is created in step 4; cleared in step 6 after skeleton-restore; `CleanStale` runs in step 7
- [ ] Integration test proves that structural events fired during skeleton-restore produce no mid-build captures (daemon's first post-restore tick captures the complete final state)
- [ ] `internal/tmux/wait.go`, `cmd/bootstrap_wait.go`, and all call sites of `WaitForSessions` / `bootstrapWait` are deleted; `EnsureServer()` and the `serverStarted` context flag remain
- [ ] TUI loading page shows for a minimum of 1.2 seconds (padded if bootstrap was faster, natural if slower); CLI path runs silently with no loading output
- [ ] Integration test on an isolated `tmux -L` socket reboots a saved multi-session configuration and verifies structure, layout, zoom, CWDs, per-session environment, resume-hook firing, and scrollback content with ANSI all round-trip
- [ ] Steady-state reattach cost (all saved sessions already live) is a single JSON read + `list-sessions` + diff, with no structural rewrites
- [ ] `portal attach NAME` and `portal open` continue to resolve names that only exist in `sessions.json` at bootstrap time (skeleton is created before the command's own attach logic runs)

#### Tasks
status: approved
approved_at: 2026-04-23

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| built-in-session-resurrection-5-1 | Align `skipTmuxCheck` exempt list with spec (drop `hooks`, add `state` and its subcommands) | nested `portal state <sub>` walks parent chain correctly, all four internal subcommands (`daemon`, `notify`, `signal-hydrate`, `hydrate`) exempt to prevent recursive bootstrap, `portal hooks set` no longer exempt (goes through full bootstrap), unknown subcommand still reaches cobra default help without bootstrap side-effects |
| built-in-session-resurrection-5-2 | Introduce `bootstrap.Run` orchestrator executing steps 1–8 in spec order | step ordering asserted via ordered-call recorder, `@portal-restoring` set-option failure short-circuits before `_portal-saver` creation, `serverStarted` flag still computed and threaded through context, idempotent across repeated invocations within a single Portal process, orchestrator injectable for tests |
| built-in-session-resurrection-5-3 | Wire `PersistentPreRunE` to call the orchestrator and remove inline bootstrap logic | exempt commands short-circuit before any orchestrator call, `bootstrapDeps` injection preserved for existing tests, repeated `PersistentPreRunE` invocations run orchestrator once per process (memoized), orchestrator error propagates as `PersistentPreRunE` error, context keys (`serverStartedKey`, `tmuxClientKey`) still populated |
| built-in-session-resurrection-5-4 | Delete `internal/tmux/wait.go` + `wait_test.go` and `cmd/bootstrap_wait.go` + `bootstrap_wait_test.go` | `DefaultMinWait` / `DefaultMaxWait` / `DefaultPollInterval` / `WaitConfig` / `DefaultWaitConfig` / `WaitForSessions` removed, `BootstrapDeps.Waiter` field removed, no dangling imports, package still builds with `go build ./...` |
| built-in-session-resurrection-5-5 | Remove `bootstrapWait(cmd)` call sites in `cmd/attach.go`, `cmd/kill.go`, `cmd/list.go`, `cmd/open.go` | `cmd/open.go` path-argument flow reaches `qr.Resolve` without pre-attach wait, `portal attach NAME` where NAME exists only in `sessions.json` resolves because skeleton ran in bootstrap, `portal list` no longer prints `"Starting tmux server..."`, existing fixtures in `cmd/open_test.go` that asserted on that stderr signal updated |
| built-in-session-resurrection-5-6 | Strip TUI's session-polling loading state machine and replace with 1.2s minimum-display pad | `MinWaitElapsedMsg` / `MaxWaitElapsedMsg` / `pollSessionsCmd` / `sessionsReceived` / `minWaitDone` all removed, single `LoadingMinElapsedMsg` at 1.2s, `sessionsLoaded` bookkeeping still correct for `evaluateDefaultPage`, `viewLoading` copy updated from "Starting tmux server..." to restoration-appropriate text, no 6s hard cap |
| built-in-session-resurrection-5-7 | Dismiss TUI loading page on orchestrator completion (not on `ListSessions` returning rows) | bootstrap <1.2s pads to exactly 1.2s, bootstrap >1.2s dismisses naturally on completion, bootstrap error tears down loading page cleanly (Phase 6 handles stderr emission; wiring preserved), empty saved state still dismisses at 1.2s minimum, `Init` no longer schedules `tmux.DefaultMinWait`/`DefaultMaxWait` ticks |
| built-in-session-resurrection-5-8 | Integration test (isolated `tmux -L` socket): `@portal-restoring` suppresses captures during skeleton-restore window | at least one structural hook event verified to fire during the window (non-vacuous), `save.requested` present during the window does not cause a tick, in-flight capture started pre-flag is permitted to commit its pre-restore snapshot, first post-clear tick captures complete final state, socket killed in `t.Cleanup` on both pass and fail |
| built-in-session-resurrection-5-9 | Integration test (isolated `tmux -L` socket): end-to-end reboot round-trip verifies structure, layout, zoom, CWDs, environment, hook firing, ANSI scrollback | isolated `-L <unique>` socket never contaminates user sessions, `base-index` / `pane-base-index` variation covered, resume-hook command captures an assertable side-effect, `@portal-skeleton-<paneKey>` markers cleared after attach, `client-attached` and `client-session-changed` both exercised, helper `exec $SHELL` replaces helper so hook fires exactly once |
| built-in-session-resurrection-5-10 | Integration test: `portal attach NAME` and `portal open NAME` resolve names present only in `sessions.json` | steady-state reattach (saved session already live) does zero structural rewrites, `has-session` post-bootstrap returns true for every name in `sessions.json`, `switch-client` (inside-tmux) and `exec attach-session -A` (bare-shell) paths both verified, name in neither live nor saved still fails with existing not-found error |

### Phase 6: Observability, user commands, and documentation
status: approved
approved_at: 2026-04-23

**Goal**: Deliver the `portal state status` diagnostic, finish `portal state cleanup` (daemon kill and `--purge`), land `portal.log` with rotation + concurrent-writer discipline, emit stderr one-liners for fatal and soft bootstrap errors, and ship the required README sections.

**Why this order**: Observability consumes data produced by Phases 2–5 (`saved_at`, daemon PID, log entries, corrupt-sessions paths, fatal bootstrap conditions). Ships last so every warning/error surface the spec calls out has a real source to report on. README updates document the now-shipping behaviour.

**Acceptance**:
- [ ] `portal state status` prints the documented fields (daemon liveness with PID + version, last save, sessions/panes captured counts, state size, recent warnings) and exits 0 when healthy, non-zero when daemon not running / last save >5 min / recent errors in log
- [ ] "Recent warnings" scans `portal.log` only (never `portal.log.old`) over a 1-hour window; missing log file is treated as healthy / zero warnings
- [ ] `portal state cleanup` performs all three actions (kill `_portal-saver` with SIGHUP-final-flush, remove Portal hook entries, optional `--purge` of the state directory) idempotently; continues through partial failures and reflects them in exit code; logs every failure
- [ ] `portal.log` uses the `timestamp | level | component | message` single-line format; `PORTAL_LOG_LEVEL=debug` enables verbose tracing; components covered: `daemon`, `restore`, `hydrate`, `notify`, `hooks`, `bootstrap`
- [ ] Only the daemon rotates logs (on startup check and on the write that crosses ≥1 MB → `portal.log.old`); every other writer uses `O_APPEND` without size checks or rotation
- [ ] Fatal bootstrap errors (`tmux -V` fail, `EnsureServer()` fail, mass hook-registration failure, `@portal-restoring` set-option failure) emit a single stderr line and exit non-zero; TUI tears down the loading page cleanly first
- [ ] Soft bootstrap warnings (corrupt `sessions.json`, `_portal-saver` failed to start after retries) emit the specified single-line stderr warning and continue; TUI buffers these in memory during the loading window and emits them after the loading page dismisses
- [ ] README ships Privacy Considerations, Uninstall (both supported paths), hooks-fire-on-reboot-only clarification, tmux ≥ 3.0 requirement, and `~/.config/portal/state/` storage-location notes as specified

#### Tasks
status: approved
approved_at: 2026-04-23

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| built-in-session-resurrection-6-1 | Introduce structured `portal.log` writer with `timestamp \| level \| component \| message` format and `PORTAL_LOG_LEVEL` filtering | state directory missing (created `0700`), invalid `PORTAL_LOG_LEVEL` value defaults to `WARN`, multibyte/UTF-8 messages preserved, message containing embedded `\|` handled deterministically, RFC 3339 UTC timestamp regardless of local zone, empty component string rejected at compile-site |
| built-in-session-resurrection-6-2 | Non-daemon writers append via `O_APPEND` with no size check; retrofit every component's existing log call to the new logger | file does not exist on first write (created `0600`), permission error surfaces but does not crash caller, single-line entries below `PIPE_BUF` rely on POSIX atomic append, no rotation attempted even if size > 1 MB, parent directory missing tolerated by lazy create, every prior `log.Println` / ad-hoc writer call site replaced |
| built-in-session-resurrection-6-3 | Daemon-only mid-write rotation when the next write crosses >=1 MB | write that exactly equals 1 MB rotates, existing `portal.log.old` replaced (not appended), rotation-rename failure logs to stderr and continues with current file, write spanning the 1 MB boundary completes in original file then rotates for next write, daemon restart between size check and rename tolerated, non-daemon callers never reach this code path |
| built-in-session-resurrection-6-4 | Implement `portal state status` data collection (daemon liveness, last-save, counts, state size, recent warnings) | `sessions.json` missing (counts = 0, last-save = "never"), `daemon.pid` missing or stale (liveness = not running), `signal(0)` permission denied surfaces as not-running, state directory missing (size = 0 B), `portal.log` missing (recent warnings = 0, healthy), `portal.log.old` never scanned, malformed log entries tolerated and skipped, 1-hour window rolling from `time.Now()` |
| built-in-session-resurrection-6-5 | Render `portal state status` output and compute exit code | daemon not running -> non-zero, last save > 5 min ago -> non-zero, recent ERROR-level entries -> non-zero, all healthy -> zero, `--json` not in v1, `state size` rendered with binary units, "Last save: never" when no `sessions.json`, "(last: none)" when zero recent warnings |
| built-in-session-resurrection-6-6 | Add daemon-kill action to `portal state cleanup` (SIGHUP-final-flush via `kill-session -t _portal-saver`) | `_portal-saver` absent is not an error, `kill-session` failure for non-"session-absent" reason logged and contributes to non-zero exit, runs before hook removal so daemon's SIGHUP flush captures pre-cleanup state, `@portal-restoring` set during cleanup still skips final flush per Phase 2 contract, second invocation is a clean no-op |
| built-in-session-resurrection-6-7 | Add `--purge` flag to `portal state cleanup` for state-directory removal | `--purge` absent -> state dir untouched, state dir missing -> idempotent no-op, per-file removal failure logged and exit non-zero but other files still attempted, refuses to remove paths outside resolved state dir (defensive symlink check), `--purge` runs after daemon kill and hook removal, FIFOs and `.bin` files all swept |
| built-in-session-resurrection-6-8 | Emit single-line stderr + non-zero exit for fatal bootstrap errors | `tmux -V` failure message matches Phase 1 user-facing copy verbatim, `EnsureServer` failure surfaces underlying error context, hook-registration failure is "mass" only when every event failed, `@portal-restoring` set-option failure short-circuits before `_portal-saver` creation, every fatal also logs to `portal.log` at `ERROR`, no banners or color, exit code distinguishable from Cobra usage errors |
| built-in-session-resurrection-6-9 | Emit single-line stderr warnings for soft bootstrap failures (CLI path direct write) | corrupt-`sessions.json` warning text matches spec verbatim, daemon-failed-after-retries warning text matches spec verbatim, both also logged to `portal.log` at `WARN`, multiple soft warnings each get their own line, CLI path writes immediately, TUI path defers via task 6-10 |
| built-in-session-resurrection-6-10 | TUI buffers bootstrap warnings during loading window and flushes after page dismissal; tears down loading page cleanly on fatal | zero warnings -> no flush noise, multiple warnings emitted in original order after dismissal, fatal error during loading dismisses page cleanly before stderr write, warnings buffered before TUI ever starts flushed once TUI exits, buffer accessible from non-TUI callers via shared sink, log file always written regardless of stderr buffering |
| built-in-session-resurrection-6-11 | Ship README updates: Privacy Considerations, Uninstall (both paths), hooks-fire-on-reboot-only, tmux >= 3.0 requirement, storage location | Privacy section names `0600` mode and the `history-limit 0` / `clear-history` workarounds, Uninstall covers both "remove binary only" and `portal state cleanup` paths, hooks section calls out behavioural change (no live detach/reattach firing), tmux >= 3.0 listed under Installation requirements, storage-location note adds `~/.config/portal/state/` alongside existing config files, no exhaustive tmux API reference, no internal architecture diagrams |
| built-in-session-resurrection-6-12 | Delete legacy `bootstrap.NewShim` and `BootstrapDeps.Bootstrapper` field | every legacy-shape test migrated, `ServerBootstrapper` deleted iff no remaining callers, `go build` clean after `grep` returns zero `NewShim` matches |

### Phase 7: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| built-in-session-resurrection-7-1 | Remove obsolete README hooks paragraph that contradicts the new firing model | line 177 paragraph deletion only, surrounding "When hooks fire:" paragraph at 179-184 still flows, no remaining `@portal-active-<pane>` / `ExecuteHooks` / `send-keys` references in hooks section, manual-only verification (no automated test) |
| built-in-session-resurrection-7-2 | Implement functional migrate-rename hook key migration (or remove the inert scaffolding) | path (a) requires daemon-side last-seen-names tracking and real (old, new) hook args; path (b) drops `migrateRenameEvents`/`migrateRenameCommand`/`migrateRenameSubstring` and records v2 deferral in spec; `cmd/state_migrate_rename.go` retained either way; `UnregisterPortalHooks` deduped union collapses to two if path (b) |
| built-in-session-resurrection-7-3 | Consolidate paneKey-from-FIFO helper into internal/state | absolute path input, basename-only input, idempotency on input lacking prefix/suffix, both `cmd/state_hydrate.go` and `internal/state/fifo_sweep.go` call sites swap to new helper, no remaining unexported `paneKeyFromFIFO*` helpers |
| built-in-session-resurrection-7-4 | Remove redundant Logger nil guards across new files | guards whose entire body is a single Logger call removed; mixed-statement guards left intact; `state.NopLogger()` paths still pass; six files: `internal/restore/restore.go`, `restore_marker.go`, `session.go`, `cmd/state_hydrate.go`, `state_signal_hydrate.go`, `cmd/bootstrap/bootstrap.go` |
| built-in-session-resurrection-7-5 | Delete dead-code parallel restoring-marker API in internal/restore | `internal/restore/restore_marker.go` and `restore_marker_test.go` removed; integration tests rewritten to use `state.RestoringMarkerName` inline; no remaining callers of `RestoreWithMarker`/`SetRestoring`/`ClearRestoring`; `@portal-restoring` literal limited to `state/markers.go` and docs |
| built-in-session-resurrection-7-6 | Collapse DeleteServerOption and UnsetServerOption to a single method | `DeleteServerOption` removed; four callers migrated to `UnsetServerOption`; mock/fake `Commander` implementations updated; `TestDeleteServerOption` cases merged into `TestUnsetServerOption`; symmetric `Set/Unset` naming |
| built-in-session-resurrection-7-7 | Promote tmuxSocket integration-test harness to shared package | new `internal/tmuxtest/socket.go` (non-`_test.go`) is sole definition; `internal/restore/integration_test.go` and `cmd/bootstrap/phase5_integration_test.go` both import it; `_ = out` discard in `waitForSession` restored; both integration suites pass |
| built-in-session-resurrection-7-8 | Eliminate real-step adapter copies in phase5 integration test | choose Option A (extract to `internal/bootstrapadapter`) or Option B (move test into `cmd` package); `realRestoringMarker`/`realHookRegistrar` deleted; no hardcoded `"@portal-restoring"` literal in phase5 test; phase5 integration test still passes against real tmux |
| built-in-session-resurrection-7-9 | Re-query live tmux indices post-creation instead of predicting | two-phase restructure (collect saved-position metadata, then walk live `list-panes`); FIFO path and skeleton-marker key derived from live indices; `--file <scrollback>` passed correctly post-creation; `PredictLiveIndices` dropped or repurposed; regression test for `base-index`/`pane-base-index` drift between save and restore |
| built-in-session-resurrection-7-10 | Self-enforce the Restorer interface contract for corrupt-vs-soft errors | Option A: signature change to `Restore() (corrupt bool, err error)` with `(true, err)` only for `state.ErrCorruptIndex`; Option B: doc-comment + contract test asserting orchestrator rejects non-corrupt non-nil errors; orchestrator step 5 cannot escalate soft per-session failure to a `PersistentPreRunE` abort |
| built-in-session-resurrection-7-11 | Delete SaverDownError and LastSaverErr triple-encoding | `LastSaverErr` field and `SaverDownError` type removed; step 4 retains warning append + `Logger.Warn` only; `bootstrap_test.go:260-261` updated to assert via warning slice; no remaining grep hits on either symbol |
| built-in-session-resurrection-7-12 | Table-drive RegisterPortalHooks event categories | `type hookCategory struct { events []string; substring, command string }`; single outer loop over table replaces three identical blocks; error accumulation behaviour preserved; if Task 2 picks path (b) the table has two entries, otherwise three |
| built-in-session-resurrection-7-13 | Replace daemon's hand-rolled index load with state.ReadIndex | three return shapes mapped: `(skip=true, err=nil)` -> `prevIdx=nil` no-log; `(skip=true, err non-nil)` -> `prevIdx=nil` + warn; `(skip=false, _)` -> `prevIdx=&idx`; daemon now distinguishes corrupt vs missing in logs; existing daemon tests pass |
| built-in-session-resurrection-7-14 | Hoist surrounding-quote stripping into a shared leaf package | new `internal/tmuxout/strip.go` exporting `StripMatchedOuterQuotes`; `internal/state/markers.go:71-74` and `internal/tmux/hooks_parse.go` both call shared helper; both quote styles covered (`"..."` and `'...'`); unit tests cover asymmetric quotes and empty input |
| built-in-session-resurrection-7-15 | Hoist xdgConfigBase into a shared leaf package | new `internal/xdg/xdg.go` exporting `ConfigBase() (string, error)`; `cmd/config.go` and `internal/state/paths.go` both delegate; `homeDir` parameter dropped from cmd-side caller; only the new helper grep-matches `XDG_CONFIG_HOME`; env-set / env-unset / env-empty tests |
| built-in-session-resurrection-7-16 | Move noopRunner out of production cmd/root.go | `noopRunner` cut from `cmd/root.go:113-120`; lives in a `_test.go` file (e.g. `cmd/bootstrap_orchestrator_test.go` or new `cmd/root_test_helpers.go`); production code references `bootstrap.Runner` only via concrete `*bootstrap.Orchestrator`; `bootstrapDeps`-injection tests still pass |
| built-in-session-resurrection-7-17 | Reconcile log rotation threshold with spec wording (1 MB vs 1 MiB) | choose A (constant -> `1_000_000`) or B (spec wording -> "1 MiB"); apology comment removed; rotation tests in `internal/state/logger_test.go` pass at the chosen threshold |
| built-in-session-resurrection-7-18 | Relocate AllPaneLister out of misnamed internal/hooks/tmux.go | choose A (`cmd/clean.go`) or B (`internal/hooks/lookup.go`); `internal/hooks/tmux.go` deleted; interface declared in exactly one file; `cmd/clean_test.go` continues to pass |
| built-in-session-resurrection-7-19 | Add canonical SetSkeletonMarker / UnsetSkeletonMarker helpers in internal/state | small `ServerOptionWriter` interface in `internal/state/markers.go`; three direct sites (`internal/restore/session.go:313-318`, `cmd/state_hydrate.go:189-193`, `cmd/state_hydrate.go:322-325`) swapped to helpers; no remaining `SkeletonMarkerPrefix +` outside `markers.go`; set/unset/key-escape unit tests |
| built-in-session-resurrection-7-20 | If migrate-rename hook is removed (Task 2 path B), collapse the dedupe-category list | strictly conditional on Task 2 path (b); `migrateRenameEvents` removed from deduped union; `portalCommandSubstrings` shrinks symmetrically; `UnregisterPortalHooks` references only two event categories; if Task 2 picks path (a) this task is obsolete |

### Phase 8: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| built-in-session-resurrection-8-1 | Update CLAUDE.md to remove pre-resurrection mechanisms | L42 drop `WaitForSessions` + add resurrection-era methods; L49 hooks row reduces to JSON `Store` only; L63 bootstrap section rewritten for `bootstrap.Orchestrator` + TUI min-pad; L65-67 paragraph replaced with hydrate-time firing; new `internal/state` / `internal/restore` / `_portal-saver` mentions; grep for `ExecuteHooks\|WaitForSessions\|bootstrapWait\|@portal-active` in CLAUDE.md returns zero |
| built-in-session-resurrection-8-2 | Promote skipIfNoTmux to internal/tmuxtest | new `tmuxtest.SkipIfNoTmux`; both verbatim copies (`internal/restore/integration_test.go:35-40`, `cmd/bootstrap/phase5_integration_test.go:39-44`) deleted including mirror comment; both files import `internal/tmuxtest`; `grep -rn "func skipIfNoTmux"` returns zero; tmux-absent CI still skips cleanly |
| built-in-session-resurrection-8-3 | Update specification to describe respawn-pane -k arming | spec L632 example switches to default-shell form + adds respawn-pane sentence; L730-740 rephrased to describe respawn-pane atomicity (preserves "shell does not produce output before helper" guarantee); L750 atomicity claim retained; L1022 example replaced with default-shell form + respawn sub-step; phrase "as its initial process" no longer in skeleton-restore context |
| built-in-session-resurrection-8-4 | Centralise AtomicWrite + chmod 0600 doublet | `fileutil.AtomicWrite0600` (or private state helper) wraps `AtomicWrite` + post-write chmod; `internal/state/commit.go:47-50` and `scrollback.go:107-110` collapse to single helper call; `daemon_state.go` `WritePIDFile` / `WriteVersionFile` left unchanged with cross-reference comment; permissive-umask test asserts mode 0600 |
| built-in-session-resurrection-8-5 | Extract unsetSkeletonMarkerOrLog helper for hydrate paths (incl. FIFO->marker convenience) | new `state.UnsetSkeletonMarkerForFIFO` composes `PaneKeyFromFIFOPath` + `UnsetSkeletonMarker`; private `unsetSkeletonMarkerOrLog(cfg)` in `cmd/state_hydrate.go` emits canonical WARN; `cmd/state_hydrate.go:178-180` and `:302-305` collapse to single call each; unused `livePaneKey` recomputes pruned; unit test asserts FIFO-derived paneKey passed correctly |
| built-in-session-resurrection-8-6 | Simplify ApplySkeletonMarkers signature (drop error + predicted-base params) | signature drops `error` return + `predictedBase` / `predictedPaneBase` params; drift comparison + `warnOnPaneKeyDrift` loop relocated to `restoreOne`; lone caller `internal/restore/restore.go:144` `if err :=` branch removed; test callers updated; drift-WARN test now targets `restoreOne` path |
| built-in-session-resurrection-8-7 | Collapse isolated-socket tmux argument prefix into a helper | new private `socketArgs(socketPath, args...)` helper; three sites in `internal/tmuxtest/socket.go` (`Socket.cmd` lines 70-73, `socketCommander.Run` lines 113-119, `socketCommander.RunRaw` lines 124-130) delegate; `grep -n '"-S"' internal/tmuxtest/socket.go` returns one hit; cross-package integration tests still pass |
| built-in-session-resurrection-8-8 | Extract bootstrap.Orchestrator fatal-message helper | new `(o *Orchestrator) fatalf(verb, err)` builds `"Portal failed to <verb>: <err>"`; four sites at `cmd/bootstrap/bootstrap.go` lines 151, 156, 161, 193 collapse to single line each; `grep -n '"Portal failed to '` returns one hit; byte-identical messages preserved |
| built-in-session-resurrection-8-9 | Move bootstrap noop step types into cmd/bootstrap as canonical sources | new `cmd/bootstrap/noop.go` exports `NoOpServer`, `NoOpHooks`, `NoOpRestoringMarker`, `NoOpSaver`, `NoOpRestorer`, `NoOpStaleCleaner`; production `noopStaleCleaner` in `cmd/bootstrap_production.go:102-109` deleted; phase5 test's four sibling noop types deleted; audit retires any other private noops in bootstrap tests |
| built-in-session-resurrection-8-10 | Extract daemon-state read template | new private `readDaemonFile(path, absentSentinel)` collapses ENOENT-classification + read-error wrapping; `ReadPIDFile` (`internal/state/daemon_state.go:36-50`) and `ReadVersionFile` (`:101-110`) reduce to trim/parse-only after helper call; absent-file and parse-error behaviour unchanged |
