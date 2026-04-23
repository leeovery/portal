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
| built-in-session-resurrection-1-9 | Implement the Phase 1 slice of `portal state cleanup` | no tmux server running is not an error, partial failure still attempts subsequent removals, running twice in a row is a clean no-op the second time |

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

### Phase 4: Resume-hook lifecycle migration
status: approved
approved_at: 2026-04-23

**Goal**: Move hook firing out of the old attach-time `ExecuteHooks` path into the hydrate helper's exec chain; add `session-renamed` key migration via a separate internal subcommand; update `CleanStale` to run unconditionally.

**Why this order**: Requires Phase 3's hydrate helper to exist — that is the new firing point. Cannot run earlier without breaking currently-shipping hook behaviour. Earlier phases intentionally leave the old executor wired up so the system stays usable during intermediate phases; this phase performs the cutover.

**Acceptance**:
- [ ] `internal/hooks/executor.go`, `cmd/hook_executor.go`, all `ExecuteHooks` call sites in `cmd/open.go` and `cmd/attach.go`, and any `@portal-active-<pane>` registration-time marker logic are deleted
- [ ] The hydrate helper reads `hooks.json` after its 100ms settle sleep, looks up by the `--hook-key` argument (not live pane position), and `exec`s `sh -c 'HOOK; exec $SHELL'` on match or `$SHELL` otherwise — on both the successful-dump and the missing-file success paths
- [ ] Hook firing does NOT happen on the 3-second timeout path; the timeout path also does NOT clear the skeleton marker (next attach re-signals)
- [ ] `portal state migrate-rename <old> <new>` is registered against `session-renamed` alongside `portal state notify` using the same content-based idempotency pattern; it rewrites every `<old>:*` key in `hooks.json` to `<new>:*` atomically via `AtomicWrite` and logs best-effort on failure
- [ ] `CleanStale` no longer has the `len(livePanes) == 0` early return; runs unconditionally as bootstrap step 7 and from `portal clean`
- [ ] Stale-detection criteria remain unchanged: structural-key mismatch against `list-panes -a` only; binary-missing and `projects.json`-absent are NOT staleness signals
- [ ] `portal hooks set`, `portal hooks list`, `portal hooks rm --on-resume` retain their existing user-facing surface; behavioural change is documented: hooks fire on skeleton-restored panes only, not on live detach/reattach within a server lifetime

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
