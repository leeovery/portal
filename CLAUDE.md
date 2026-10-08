# CLAUDE.md

Guidance for agents working in this repository. The code is the source of truth for how things work; this file holds what the code won't tell you quickly: how to build and test without damaging the developer's machine, where things live, and the invariants that look removable but aren't. Most mechanisms carry doc comments and source guards that explain and enforce their own rules — read those at the named site before changing one.

## Build & Test

```bash
go build -o portal .                        # build binary
go test ./...                               # unit lane: fast, hermetic
go test -tags integration -p 1 ./...        # integration lane: real tmux + daemons + built binaries; -p 1 is REQUIRED
go test ./cmd -run TestOpenCommand_PathArgument_SkipsTUI   # single unit test
go test -tags integration ./cmd -run TestCommitNowSymptom   # single integration test
golangci-lint run                           # lint (config sets the integration tag, so tagged files are covered)
```

- **Lane rule.** Any test that *builds* a `portal` binary, spawns a `portal state daemon`, or execs a built `portal` binary carries `//go:build integration`. The unit lane may still hold fast real-tmux *client* tests on disposable `tmuxtest` servers (`-S <tmpdir>/ptl-*/s`) — no daemons, no portal binary. Build `portal` in a test only through `internal/portalbintest`, which builds with `-tags integration` and so compiles in the daemon-pgrep sandbox; never hand-roll `go build`.
- **`-p 1` is load-bearing.** There is no CI; daemon-timing suites flake when packages run in parallel on one machine.
- **No `t.Parallel()`.** `cmd` injects mocks through package-level mutable state. Stage `*Deps` structs with the `withXDeps(t, …)` helpers and function-var seams with `withFuncSeam(t, &seam, fn)` (both in `cmd/testhelpers_test.go`, both self-restoring). `cmd/seam_guard_test.go` fails any test that assigns a seam directly.
- `golangci-lint` cannot see files gated `//go:build !integration`. It is not wired into CI. There is no code generation.

### Visual capture harness

`cmd/capturetool` is a separate offline program (not a `portal` subcommand) that renders a named deterministic TUI fixture through `tui.Build` with every tmux seam faked (`internal/capture`, kept out of the production binary by an import guard). It is the only way to see a visual change before release — a scratch build of Portal disturbs the running daemon and touches real state.

```bash
go run ./cmd/capturetool --fixture <name> [--theme <slug|path>]   # live view; capture.FixtureNames() lists fixtures
vhs testdata/vhs/<tape>                                          # re-capture a still
```

- `testdata/vhs/*.tape` and `*.png` are scaffolding: written while work is in progress, deleted after sign-off. `testdata/vhs/reference/*.png` are committed design exports and are kept — nothing in Go points at them, so a sweep must not treat them as orphans. Retention table: `testdata/vhs/README.md`.
- Fixture definitions in `internal/capture` are permanent: the swap-and-diff completeness guard enumerates them, so deleting one silently shrinks coverage. Standalone surfaces that aren't picker fixtures (contrast swatch, resume panel screens) are enumerated through `capture.StandaloneNames()`; a new one joins every guard at that one edit.

## Test safety — ABSOLUTE INVARIANT

**A test must never mutate or affect the real system:** not the filesystem outside its temp dirs, not the default tmux server, not any process it did not spawn. Tests run on the developer's machine, where their live tmux server, real sessions and `portal state daemon` are always running. All three boundaries are mandatory:

1. **Filesystem/state.** Any test that runs `portal state daemon` — directly or via `portal open`/bootstrap — calls `portaltest.IsolateStateForTest(t)` and sets the returned env on every subprocess (`cmd.Env = env`). It scrubs `HOME`/`XDG_CONFIG_HOME`, poisons `TMUX`, enables the pgrep sandbox, and registers a fingerprint-diff cleanup. The fingerprint is defence in depth, not a substitute: it walks the state dir under the scrubbed `HOME`, so it catches a process that took the scrub and still resolved the default path, not one that escaped the scrub.
2. **tmux.** Only per-test sockets from `tmuxtest`, never the default socket — including indirectly. A test that Executes a real Cobra command body inherits its production wiring, and `tmux.DefaultClient()` honours the ambient `TMUX`, so inject every tmux-touching `*Deps` seam. `cmd`'s `TestMain` poisons `TMUX` and the `PORTAL_*` paths package-wide, so a missed injection dials a dead socket and fails loudly. A subprocess that needs a server appends its own `TMUX=<test socket>,0,0` after the isolated env (last wins). A test asserting inside/outside-tmux behaviour sets `TMUX` itself with `t.Setenv`. *(Incident: an uninjected command body killed the developer's live `_portal-saver` and unregistered their global hooks on every `go test ./cmd`.)*
3. **Processes.** The daemon is identified machine-wide by argv (`pgrep -fx '^portal state daemon( |$)'`), so file isolation does not make a test daemon distinguishable from the real one. `state.PgrepPortalDaemons` is filtered by a default-deny sandbox (`internal/state/pgrep_sandbox.go`, integration-tagged, absent from production) that surfaces only test-registered daemons, and reaches test-spawned binaries through `state.SandboxRegistryEnv` (`PORTAL_TEST_SANDBOX_REGISTRY`), a file of test-owned state dirs. Never run the production orphan sweep, `PgrepPortalDaemons`, or any `pgrep`/`pkill`/broad signal in a test without `IsolateStateForTest` active. *(Incident: a global-pgrep sweep test repeatedly SIGKILLed the developer's live daemon.)*

For daemon-spawning tests use `portaltest.SpawnIsolatedDaemon` (pins `PORTAL_STATE_DIR` and `TMUX`, registers ownership) with `portaltest.RegisterSubprocessCleanup` (SIGKILL + Wait + reap — on macOS an unreaped subprocess holds state-dir fds past cleanup). Fixtures whose tmux server can host writers at teardown also call `portaltest.RegisterStateDirTeardownGuard(t, stateDir)`, after `IsolateStateForTest` and before `tmuxtest.New`. Integration tests that set `PORTAL_LOG_LEVEL` assert it propagated with `portaltest.AssertLogLevelResolved`.

### Cleaning up after a test run (human-invoked; never from a test)

Test runs leak `tmuxtest` servers (each with its own daemon), and an agent faking CPU load can orphan `yes`/busy loops to PID 1. Leaked load manufactures timing flakes and gets them blamed on the code under test, so sweep after a session that ran the integration lane, and before trusting any timing result while the load average is far above core count.

The developer's live tmux server must never be touched:

1. Read `$TMUX` (`<socket>,<server-pid>,<session-id>`). That socket and PID are protected.
2. Select targets by an explicit test socket in argv (`-L ptl-…` or `-S <tmpdir>/ptl-*/s`). The real server carries no socket flag. Never match on `tmux`, `_portal-bootstrap`, `_portal-saver` or session names — those exist on both.
3. Exclude the protected PID explicitly and assert it is absent from the kill list.
4. Re-verify each PID's argv immediately before signalling it (PIDs get reused).
5. SIGTERM first, SIGKILL stragglers. Afterwards confirm the developer's server is alive with its session count unchanged and their daemon still running.

For load generators, match the exact command (`ps -eo ppid,command | awk '$1==1 && $2=="yes"'`), never `pkill -f yes`. Better not to create them; if load is needed, bound it (`timeout 60 yes`).

## Architecture

Cobra CLI + Bubble Tea v2 TUI + Lipgloss v2; all tmux access goes through `internal/tmux`. `main.go` → `cmd/root.go` → subcommands. `PersistentPreRunE` runs the server bootstrap and injects a shared `tmux.Client` and `serverStarted` into the context, except for the bootstrap-exempt `skipTmuxCheck` set (`version`, `init`, `help`, `alias`, `hook`, `doctor`, `theme`, `uninstall`, `state`, `__complete`).

### Commands

- **`open`** (`cmd/open.go`; the `x` shell function from `portal init`) — the main entry. A bare positional runs the resolution chain (exact session → glob → path → alias → zoxide) and creates or attaches. `-s/-p/-a/-z` pin one domain and hard-fail on a miss; `-f` opens the picker pre-filtered; no args opens the picker. A positional starting with `/` and containing no further `/` is the **search form** (`resolver.IsSearchSigil`, `cmd/open_search.go`): never resolved, composes with nothing. Two or more targets, or a glob that may expand to two, run the multi-target burst (`cmd/open_burst*.go`; see Multi-window spawn).
- **`doctor [--fix]`** (`cmd/doctor.go`) — read-only health catalog with a scriptable exit code (0 iff no check fails; info and not-evaluable lines never fail it). `--fix` applies reversible repairs (prune stale hooks and projects, sweep logs) then re-diagnoses. It must start no server and heal nothing on the read-only path.
- **`uninstall`** — kills `_portal-saver` (SIGHUP, so the daemon flushes) and unregisters the global hooks. Touches no files.
- **`hook set/rm/list`** (`cmd/hooks.go`; `hooks` is a permanent silent alias) — see Resume hooks. `hook list` is a machine interface: tab-separated key, event, command, location, mode. Never add columns before the existing ones, or header/footer lines.
- `list`, `kill`, `alias`, `theme export`, `version`, `init`, and the hidden `state` subcommands (`daemon`, `commit-now`, `hydrate`, `notify`, `signal-hydrate`, `resume-draw`, `resume-wait`, `resume-recover`).

Connecting to a session: outside tmux, `AttachConnector` `syscall.Exec`s `tmux attach-session` (Portal's process is replaced); inside tmux, `SwitchConnector` runs `switch-client`.

### Packages

| Package | Role |
|---|---|
| `tmux` | tmux CLI client (`Commander`: trimmed `Run`, verbatim `RunRaw`) — sessions, panes, options, global hooks, server and `_portal-saver` lifecycle. Owns target composition (see Invariants) |
| `state` | Resurrection: capture, the commit cycle, scrollback dump/replay, FIFOs, markers, the daemon singleton (`daemon.lock`, `daemon.pid`, `IdentifyDaemon`, `PgrepPortalDaemons`) |
| `restore` | Two-phase restore: skeleton (`respawn-pane -k` into the hydrate helper), then geometry and scrollback FIFOs |
| `session` | Session creation (git root → project → `{project}-{nanoid}` name → tmux), `@portal-dir` stamping, dir resolution for unstamped sessions |
| `resolver` | The resolution chain and the search-form vocabulary. Pure and log-free; `cmd/open.go` logs its decisions |
| `spawn` | Multi-window spawn: terminal detection, adapters, argv composition, ack channel, burster |
| `tui` | Bubble Tea model, Modern Vivid rendering, grouping, theme slide-over, resume panel renderers |
| `theme` | The closed 19-token colour vocabulary and the `.theme` loader; built-ins are embedded `.theme` files parsed by the same loader as drop-ins |
| `project` | `projects.json` store, directory-anchored tags, `CanonicalDirKey` / `Index` |
| `prefs` | `prefs.json` leaf store (grouping mode, theme keys, `resume_mode`) |
| `hooks` | `hooks.json` store with its `hooks.json.lock` sidecar and the `StaleKeys` staleness rule |
| `hooksweep` | The hook-staleness cycle and its stand-down reasons, shared by the daemon sweep, `doctor --fix` and doctor's read-only count |
| `log` | Sole owner of logging (see Logging) |
| `bootstrapadapter` | Production adapters for `cmd/bootstrap`'s Orchestrator seams |
| `xdg` | Config path resolution and per-file identity (`ConfigFileID`, `ConfigDirID`) |
| `alias`, `fuzzy`, `fileutil`, `storelog`, `warning` | Small shared pieces; `fileutil.AtomicWrite` is the temp-file-and-rename writer every store uses |
| `nanoid`, `shellquote`, `resumekeys`, `resumemode`, `tmuxout`, `tmuxerr` | Stdlib-only leaf vocabularies shared by packages that must not import each other; a guard pins each one's dependency set |
| `capture`, `portaltest`, `tmuxtest`, `portalbintest`, `restoretest`, `statetest`, `hookstest`, `logtest`, `harnesstest`, `sourceguardtest`, `commandertest`, `transienttest`, `spawntest`, `themetest` | Test-only — production code must not import them |

### Test helpers

Use the shared helper before writing a local one:

- **`logtest.Sink`** — captured log records. Query with `sink.Records()` and chain `Matching(component, msg)`, `WithMessage`, `AtExactLevel`, `AtOrAboveLevel`, ending in `.Only(t, desc)`. Don't add combined query methods. `AssertRecord` and `AssertWriteFailure` cover audit-trail lines.
- **`harnesstest`** — `TestingT`/`Recorder` to drive a fatal helper's own failure path; `PollUntil`; `AwaitProgress` for progress-based waits (use it for daemon lifecycle rather than wall-clock deadlines).
- **`sourceguardtest`** — source-guard primitives (`RepoSources`, `PackageGoFiles`, `ParseSources`, `ForEachFuncCall`, `PackageDeps`/`AssertDepsWithin` with `Lanes`). A guard must fail when it scans nothing.
- **`commandertest.Scripted`** — the tmux `Commander` fake; owns the `Run`/`RunRaw` trim-versus-verbatim contract.
- **`hookstest`** — the only route to seed keys and staged `hooks.json` files (`StageStore`, `HooksPath`, sidecar-lock fixtures).
- **`themetest`** — built-in accessors and `.theme` fixture authoring. **`transienttest`** — `list-panes -a` failure modes for destructive-path suites. **`spawntest`** — adapter and ack fakes.

### Config paths

Files (`projects.json`, `aliases`, `hooks.json`, `prefs.json`, `terminals.json`) resolve per-file env var → `$XDG_CONFIG_HOME/portal/` → `~/.config/portal/` through `xdg.ConfigFilePath`, with a one-shot move from the old macOS `~/Library/Application Support/portal/` that never overwrites. Directories (`state/` via `PORTAL_STATE_DIR`, `themes/` via `PORTAL_THEMES_DIR`) resolve through `xdg.ConfigDirPath` with no migration; nothing creates or stats the themes dir.

`cmd/config.go`'s `loadPrefsStore` owns the one-shot `appearance` → theme translation. `loadPrefsStoreNoMigrate` is the non-migrating read used by `doctor`, the hydrate helper and `state resume-draw`. Keep it inert: anything added to it runs on doctor's read-only path and in every restored pane's startup.

### Logging

All logging goes through `internal/log`. Bind once per package (`var logger = log.For("<component>")`) and log with slog attrs.

- **Closed taxonomy.** Component names, attr keys and levels are spec-governed. Never invent a component or attr key at a call site. Production default level is INFO.
- `main.go` calls `log.Init` first and owns the single `os.Exit`. Bare `os.Exit` elsewhere is prohibited; the daemon self-eject is the one sanctioned exception and pairs itself with `log.Close`. The `process:` lifecycle markers bypass the level filter.
- No discard-backed `*slog.Logger` outside `internal/log` — use `log.Discard`/`log.OrDiscard`. A guard enforces this in test files too. `internal/log` must not import `internal/state`; `internal/prefs` must not import `internal/log`.
- Store mutations log from the store method (the chokepoint), not the caller. A cycle emits one INFO summary, with per-item detail at DEBUG.
- The `theme` component records where a theme is *used*, never where one is diagnosed: `doctor`, `theme export` and `capturetool` load through `theme.NewSilentLoader()`.
- Env: `PORTAL_LOG_LEVEL`, `PORTAL_LOG_ROTATE_SIZE`, `PORTAL_LOG_RETENTION_DAYS`. `portal.log` is a symlink to the current day's file.

### Server bootstrap

`cmd/bootstrap`'s Orchestrator runs ten steps. The order is load-bearing:

1. **EnsureServer** — starts tmux with a detached `_portal-bootstrap` session, because a server with no sessions exits. That session is a permanent anchor, never killed by production code and hidden from the picker and capture. It is not an orphan.
2. **RegisterPortalHooks** — converges each event in `managedEvents` to exactly one Portal hook, reading with per-event `show-hooks -g <event>`; the no-arg global read is blind to `pane-*` and geometry `window-*` events on tmux 3.6b.
3. **Set `@portal-restoring`** — before saver and restore, so the daemon and hydrate helpers know a restore is in progress.
4. **SweepOrphanDaemons** — before EnsureSaver, so the new daemon's first tick is uncontested.
5. **EnsureSaver** — the `_portal-saver` session hosting `portal state daemon`, with a kill-barrier on version upgrade.
6. **Restore.**
7. **EagerSignalHydrate** — signal every restored pane's FIFO while `@portal-restoring` still suppresses commits.
8. **Clear `@portal-restoring`.**
9. **CleanStaleMarkers.**
10. **SweepOrphanFIFOs.**

Steps 1, 2, 3 and 8 are fatal; the rest log WARN and surface warnings. Hook and project stale-cleanup run on the daemon's throttled idle tick, with `doctor --fix` as the manual backstop.

**Cold-path flip.** When a full bootstrap is owed and the launch is a picker (`shouldRunConcurrentBootstrap`, `isTUIPath`), the bootstrap runs concurrently with the TUI and streams progress messages (`cmd/bootstrap_progress.go`; labels in `internal/tui/loading_progress.go`). A fatal becomes an in-TUI error frame and a non-zero exit; warnings become a post-load notice band. The search form holds the loading page until bootstrap completes; a command-pending pick is held (`stagedMint`) and minted on completion. Warm and CLI paths run synchronously, unchanged.

**Daemon self-supervision.** Each tick the daemon checks it is still `_portal-saver`'s pane process. After `selfSupervisionHysteresisTicks` (3) consecutive failures it logs, calls `log.Close(0)`, then `os.Exit(0)`, skipping the shutdown flush so a divergent daemon commits nothing. It leaves `daemon.pid` stale on purpose: the next acquire's pre-check handles a dead PID, and cleanup there would race that pre-check.

## Invariants

These encode incidents and data-loss traps. Most have a source guard.

### tmux targets

- tmux prefix-matches `-t`: a bare `-t foo` silently resolves to a live `foo-2` once `foo` is gone (on the kill path, killing the wrong session). Every per-session `-t` uses an exact-match helper, chosen by measuring what that command parses. Today all of them take `CoordTargetExact` (`=name:`): a bare `=name` is read as a window or pane name, and a colon-free `=my.app` is split on the `.`. Panes use `PaneTargetExact` or `PaneIDTarget`.
- Targets are `tmux.Target`, not strings, and every `-t` parameter takes one. `internal/tmux/target_composition_guard_test.go` catches what the type can't: a literal `-t` followed by a concatenation, untyped constants, explicit conversions.
- `ValidateSessionName` refuses a name containing `:` or starting with `$` or `-` (`a.b` stays legal). `RenameSession` and the picker's `r` modal run it; the refusal wording in `internal/tui/sessions_flash.go` is mirrored in README's `hook` section, so change both together. `session.SanitiseProjectName` keeps generated names valid.
- Read a pane option through `ReadPaneOption` (an existence probe, then the format read). A lone `display-message -F` answers a gone pane with exit 0 and empty output, identical to an unset option.

### Saving and the commit cycle

The rule: a session the user kills never comes back. Detach, `tmux kill-server` and reboot leave everything restorable, with no Portal shutdown step.

- Every commit goes through `state.RunCommitCycle` (`internal/state/commit_cycle.go`), which holds `commit.lock` (not `daemon.lock`) from the marker read through housekeeping; its previous index is `sessions.json` as read under that lock. Code outside `internal/state` must not call `state.Commit` (guarded; it is exported only for test fixtures).
- A cycle commits only if the committer's **own** tmux server (`tmux.ServerPIDFromEnv`) answers a confirmation (`ConfirmAnswering`) sent after the last capture read. An exiting tmux refuses new connections and never stops exiting, so an answer proves every earlier read predates the exit. Otherwise the cycle stands down with `ErrTmuxStoppedAnswering`: no commit, no housekeeping, logged as `<stage> backed off: tmux stopped answering`.
- The committing path lists sessions with `ListSessionNamesProbe`, which returns a failed `list-sessions` as an error. The shared `ListSessions`/`ListSessionNames` deliberately read a failure as "no server, no sessions" for the picker, resolver, completion and restore. Change neither: if the shared listing returned errors, one transient failure at bootstrap would make restore bring back nothing, and the next tick would commit that empty index over the saved state.
- The scrollback dump never writes an empty capture over a non-empty saved transcript unless the own server answers after it.
- Waiting (resume-pending) panes are never capture-paned. Their transcripts are re-filed to token-named paths by hard link that never overwrites, so a cycle that ends uncommitted never leaves `sessions.json` naming a missing file. Pending and skeleton panes merge their previous record by durable token, not by address. Ask `CaptureCycle.SkipsScrollback` which panes to skip rather than reading the sets directly.
- Every dropped session logs `session dropped session=<name>` at INFO; a rename is not a drop.

### Resume hooks

- **A hook key is the pane's `@portal-pane-id` token** (`state.PortalPaneIDOption`): no session name and no coordinates, so no rearrangement or rename changes it. Stamping is lazy — `hook set` mints only when the pane reads back empty, and split/new-window panes inherit nothing. The stamp lands before the `hooks.json` write, with no rollback or unstamp if the write fails. Restore re-stamps saved tokens and mints one for each pane saved without (`state.RecordRestoredPaneTokens`).
- **The firing path never reads the live pane token.** Restore bakes the key from saved state (`Pane.PortalPaneID`) into the hydrate helper's argv; an empty saved token bakes no key.
- **A key the staleness rule cannot judge is retained forever — deleting one is data loss.** `hooks.StaleKeys` reaps a key only when it is absent from the live token set *and* token-shaped (`nanoid.IsTokenShaped`) or empty. Pre-token `<session>:<window>.<pane>` keys survive every run; only `hook rm --pane-key` or a hand edit removes them. They may sit beside a token-keyed entry for the same pane. Don't add expiry, migration or a tidy pass.
- **A registration that lands in the enumeration gap is never reaped.** `CleanStale` snapshots `hooks.json` before the unlocked live enumeration, and `narrowToSnapshot` may only drop candidates from the delete set, never add them.
- `hooks.json` values may be a string or an object; both shapes are permanently valid, with no migration. The codec re-emits untouched entries byte-for-byte, so attributes it doesn't model survive a sibling's rewrite.
- The `hooks.json.lock` sidecar is created by the first mutation and is absent on most installs, so the unlocked read (`load-unlocked` DEBUG) is the common path, not contention. A read never fails for want of the lock.
- `hook rm` exits 0 only if the removal itself removed something. The panel's discard (`hooks.Store.Discard`) removes only when the stored command still matches what the confirmation showed, compared under the lock.
- Hooks fire only in the hydrate helper (`cmd/state_hydrate.go`), on reboot recovery. The mode is `resumemode.Resolve` of registration → `prefs.json` `resume_mode` → shipped default **lazy**. Eager panes exec `sh -c 'trap : TERM; <HOOK>; exec $SHELL'`; lazy panes park in the resume-panel chain (`cmd/state_resume_{draw,wait,recover,chain,altscreen,report}.go`), which runs under the existing `hydrate` role and log component.
- The lazy-pane ordering in `state_hydrate.go` is load-bearing: token write, alternate-screen pin, `@portal-resume-pending` marker — all before the mid-restore marker clears, since a gap lets the saver write the panel over the transcript. Any failure there downgrades the pane to eager with one WARN.
- Portal's resume panes *catch* SIGTERM so a shutdown saves a waiting pane still waiting. Never use an ignored disposition — it survives `exec` into the user's hook and shell. SIGHUP stays uncaught so a kill still ends the pane at once.

### TUI

- **Owned canvas.** Every cell is painted from the active theme's `canvas`. The outer full-terminal fill (the last layer in `model.go`'s `View`) stays outside the list's height budget. The terminal default background is set with OSC 11 and restored on exit (`internal/tui/restore.go`).
- **Canvas-echo guard — don't drop it.** The exit set-back is skipped when the captured original equals the canvas, compared against `startupCanvasHex` (the canvas the light/dark gate selected at startup). Never re-derive it from the active theme: after a mid-session theme change that would leave a colour the user never chose stuck in their terminal. The swap-and-diff guard can't see this path.
- **Light/dark.** A constant theme paints from frame one. An adaptive pair arms a single-resolution gate: OSC 11 query against a ~50ms timeout, dark on no answer, never paint-then-flip. `NO_COLOR` paints nothing and keeps every state glyph- or letter-backed, never colour-only.
- **Grouping pagination.** Group headings are real, non-selectable `HeaderItem` rows (`FilterValue()==""`), one delegate line each, injected by `injectGroupHeaders`. Drawing extra lines inside a row breaks `bubbles/list` pagination and scrolls the title off-screen. Don't route the picker through `lipgloss/tree`. `rebuildSessionList` is the single re-render chokepoint. Grouped modes read each session's dir from `@portal-dir`, falling back to the active pane's git root cached in memory only — never stamped back to tmux.
- No raw hex in `internal/tui` (`colour_literal_guard_test.go`, no exemptions); every colour is a theme token. The 19 token names are a public contract for drop-in theme files.
- The keymap descriptor (`keymap.go`) drives the footer, help and theme-panel footer; dispatch is separate, and `keymap_dispatch_guard_test*.go` catches drift. Navigation is arrows only: `pinArrowOnlyNav` strips vim and page aliases from all three lists (in the theme panel the defaults also collide with commit keys).
- The theme slide-over is an opaque layer over the composed page, which is not re-laid-out, so a swap is an O(1) restyle with no reflow. Notice bands go through the single-slot arbiter in `notice_band.go`.
- `prefs.json`'s legacy `appearance` key must stay declared as a preserved raw string: every writer re-encodes the whole struct, so dropping the field erases the user's value on the next write.

### Multi-window spawn

- One service (`internal/spawn`) reached by the `open` burst and the picker's multi-select (`m`). Both run detect → pre-flight → spawn N−1 → self-connect the trigger window. **Net N windows, never N+1.**
- Inside tmux, detection picks the most-active client across all clients first, then walks only that one. A remote or unresolvable winner, or a transient error, means unsupported — never spawn on uncertainty.
- Spawned windows run `<os.Executable()> open --session|--path … --ack <batch>:<token>` with `TMUX`/`TMUX_PANE` stripped; `os.Executable()` keeps the binary version-identical so the warm-command latch holds. Confirmation is the `@portal-spawn-<batch>-<token>` server option (namespaced away from `@portal-skeleton-`), not osascript success.
- Pre-flight is all-or-nothing. Past it, failures are leave-what-opened, and permission-required stops the burst. Classification, messages and logging are single-sourced for both callers.
- Ghostty wraps the command as `bash -lc '<cmd>; exec "$SHELL" -il'` so a window lands at a shell; `terminals.json` recipes get the plain command. Spawn persists nothing beyond the transient `@portal-spawn-*` options.

## Release

goreleaser (`.goreleaser.yaml`); version injected via `-X github.com/leeovery/portal/cmd.version`. Tags trigger GitHub Actions → homebrew tap. `CHANGELOG.md` is generated by the release process — never edit it.
