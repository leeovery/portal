---
phase: 6
phase_name: Observability, user commands, and documentation
total: 11
---

## built-in-session-resurrection-6-1 | approved

### Task 6-1: Introduce structured `portal.log` writer with `timestamp | level | component | message` format and `PORTAL_LOG_LEVEL` filtering

**Problem**: Phase 2 task 2-7 landed a minimal `internal/state/logger.go` with `OpenLogger(path, rotate) (*Logger, error)` and the four level methods (`Debug`, `Info`, `Warn`, `Error`) used by the daemon. The other components the spec enumerates — `restore`, `hydrate`, `notify`, `hooks`, `bootstrap` — also need to log, their call sites are scattered across `internal/state`, `cmd/state_hydrate.go`, `cmd/state_notify.go`, `internal/hooks`, and `cmd/bootstrap/`. Across those writers the format must be byte-for-byte identical (`timestamp | level | component | message`), the timestamp must be RFC 3339 UTC regardless of the writer's local time zone, and level filtering must obey `PORTAL_LOG_LEVEL` uniformly. Today's Phase 2 logger is a minimal slice — it satisfies the daemon only. This task audits and hardens the shared logger into a stable API every component can rely on, and specifies the level-filter precedence (`DEBUG < INFO < WARN < ERROR`) so an invalid `PORTAL_LOG_LEVEL` value does not silently swallow or over-emit entries.

**Solution**: Harden the `internal/state/logger.go` API with explicit semantics: (a) introduce a typed `Level` enum (`LevelDebug`, `LevelInfo`, `LevelWarn`, `LevelError`) plus a `parseLevel(env string) Level` helper that maps the four canonical strings (case-insensitive), returns `LevelWarn` on empty/unknown input, and treats `PORTAL_LOG_LEVEL=debug` as "emit all levels"; (b) ensure every `Debug/Info/Warn/Error(component, format, args...)` call writes exactly one line of the form `2026-04-23T10:30:00Z | WARN | daemon | message here\n` — timestamp via `time.Now().UTC().Format(time.RFC3339)` (never local time), level literal upper-cased, component verbatim, message formatted via `fmt.Sprintf(format, args...)`; (c) if the formatted message contains embedded `|` pipes, write them verbatim — no escaping (grep-friendly by convention; the field separator is `" | "` with spaces, so a raw `|` inside message text is unambiguous); (d) reject empty `component` at call site via a `const` catalogue (`ComponentDaemon = "daemon"`, `ComponentRestore = "restore"`, etc.) so a typo or empty string is a compile error, not a silent log oddity; (e) make `Logger` safe to share across goroutines via an internal `sync.Mutex` around writes — the daemon already uses it from two goroutines (tick + signal), and Phase 6 callers multiply the concurrency.

**Outcome**: Every subsystem in the project writes structured log lines with identical format, level filtering, and UTC timestamps. Invalid `PORTAL_LOG_LEVEL=foo` behaves as default (WARN+ERROR emitted). `PORTAL_LOG_LEVEL=debug` enables every level. A message like `Warn(ComponentRestore, "layout parse failed for session=%s window=%d | falling back to tiled", "work", 0)` writes exactly `2026-04-23T10:30:00Z | WARN | restore | layout parse failed for session=work window=0 | falling back to tiled\n` — the embedded `|` inside the message is preserved verbatim, deterministic. Compile-site enforcement via component constants keeps the component column tidy.

**Do**:
- Edit `internal/state/logger.go`:
  - Add typed `type Level int` with `const (LevelDebug Level = iota; LevelInfo; LevelWarn; LevelError)`.
  - Add `var levelNames = [...]string{"DEBUG", "INFO", "WARN", "ERROR"}`.
  - Add `func parseLevel(s string) Level`:
    - Trim + lowercase input.
    - Map `"debug"→LevelDebug`, `"info"→LevelInfo`, `"warn"→LevelWarn`, `"error"→LevelError`.
    - Anything else → `LevelWarn`.
  - Rework `Logger` to hold: `file *os.File`, `mu sync.Mutex`, `threshold Level` (resolved once at construction from `os.Getenv("PORTAL_LOG_LEVEL")`).
  - `OpenLogger(path string, rotate bool) (*Logger, error)` — preserves the Phase 2 rotation-on-open semantics (rename `portal.log` → `portal.log.old` when `rotate && Stat.Size() >= 1MB`, replacing any existing `.old`); opens with `O_APPEND|O_CREATE|O_WRONLY`, mode `0600`; resolves threshold via `parseLevel(os.Getenv("PORTAL_LOG_LEVEL"))`.
  - `(l *Logger) emit(level Level, component string, format string, args ...any)`:
    - Return immediately if `level < l.threshold`.
    - Build line: `fmt.Sprintf("%s | %s | %s | %s\n", time.Now().UTC().Format(time.RFC3339), levelNames[level], component, fmt.Sprintf(format, args...))`.
    - `l.mu.Lock(); defer l.mu.Unlock()`; `_, _ = l.file.Write([]byte(line))` — swallow write error (logger should never crash a caller). Optionally write to stderr on write failure as a last-resort diagnostic.
  - `Debug/Info/Warn/Error(component, format, args...)` — thin wrappers over `emit`.
  - `(l *Logger) Close() error` — closes the underlying file.
- Add component constants block at the top of `internal/state/logger.go`:
  ```go
  const (
      ComponentDaemon    = "daemon"
      ComponentRestore   = "restore"
      ComponentHydrate   = "hydrate"
      ComponentNotify    = "notify"
      ComponentHooks     = "hooks"
      ComponentBootstrap = "bootstrap"
  )
  ```
  All existing `logger.Warn("daemon", ...)` style calls (Phase 2/3/4) migrate to `logger.Warn(state.ComponentDaemon, ...)` as part of task 6-2. This task only introduces the constants.
- Unit tests in `internal/state/logger_test.go` (extending Phase 2's suite):
  - `"it writes entries in timestamp | level | component | message format"`: emit one line, parse it with `strings.SplitN(line, " | ", 4)`, assert four fields.
  - `"it uses UTC RFC3339 timestamps regardless of local time zone"`: set `time.Local = time.FixedZone("TEST", 4*3600)`, emit, assert timestamp ends with `"Z"` (UTC indicator) and parses via `time.Parse(time.RFC3339, ...)`.
  - `"it preserves embedded | characters inside messages verbatim"`: `Warn(ComponentRestore, "a | b | c")`; assert the written line's message field (splitting on `" | "` with limit 4) is `"a | b | c"`.
  - `"it filters DEBUG when PORTAL_LOG_LEVEL is unset"`: no env; emit Debug + Info + Warn; assert Debug absent, Info + Warn present... wait, default is `LevelWarn` so Info should also be filtered. Corrected: default threshold `LevelWarn`; Debug + Info filtered; Warn + Error emitted. Assert Debug and Info absent, Warn and Error present.
  - `"it emits DEBUG when PORTAL_LOG_LEVEL=debug"`: set env; assert all four levels appear.
  - `"it emits INFO when PORTAL_LOG_LEVEL=info"`: set env; assert Debug absent, Info/Warn/Error present.
  - `"it defaults to WARN when PORTAL_LOG_LEVEL is an invalid value"`: set env to `"gibberish"`; assert same behaviour as unset default.
  - `"it is case-insensitive for PORTAL_LOG_LEVEL values"`: `"DEBUG"`, `"Debug"`, `"debug"` all enable DEBUG.
  - `"it is safe to call from multiple goroutines concurrently"`: spawn 16 goroutines each emitting 100 lines; assert the final file contains 1600 well-formed lines, no partial writes (split on `\n`, count, and parse each).
  - `"it defines component constants matching the spec's six components"`: reflection test that `ComponentDaemon/Restore/Hydrate/Notify/Hooks/Bootstrap` exist as string constants.

**Acceptance Criteria**:
- [ ] `internal/state/logger.go` exports component constants `ComponentDaemon`, `ComponentRestore`, `ComponentHydrate`, `ComponentNotify`, `ComponentHooks`, `ComponentBootstrap` with the lowercase string values shown above.
- [ ] `Logger` has methods `Debug`, `Info`, `Warn`, `Error` with the same `(component string, format string, args ...any)` signature; every one writes in `timestamp | level | component | message` format with a trailing `\n`.
- [ ] Timestamps are RFC 3339 UTC (`time.RFC3339` + `UTC()`), ending in `Z`.
- [ ] Level filtering honours `PORTAL_LOG_LEVEL=debug|info|warn|error` case-insensitively; default is `WARN`; unknown values fall back to `WARN`.
- [ ] Logger is safe for concurrent use from multiple goroutines (internal `sync.Mutex`).
- [ ] Write failures never panic or propagate to callers.
- [ ] Existing Phase 2 daemon-only call sites continue to work (API is a superset; method signatures unchanged).
- [ ] Embedded `|` characters inside messages are written verbatim (no escaping).

**Tests**:
- `"it writes entries in timestamp | level | component | message format"`
- `"it uses UTC RFC3339 timestamps regardless of local time zone"`
- `"it preserves embedded | characters inside messages verbatim"`
- `"it filters DEBUG and INFO when PORTAL_LOG_LEVEL is unset (default WARN)"`
- `"it emits all four levels when PORTAL_LOG_LEVEL=debug"`
- `"it emits INFO, WARN, ERROR when PORTAL_LOG_LEVEL=info"`
- `"it defaults to WARN on an invalid PORTAL_LOG_LEVEL value"`
- `"it is case-insensitive for PORTAL_LOG_LEVEL values"`
- `"it is safe to call from multiple goroutines concurrently"`
- `"it exposes component constants for daemon/restore/hydrate/notify/hooks/bootstrap"`

**Edge Cases**:
- State directory missing when `OpenLogger` is called — callers are expected to `state.EnsureDir()` first (Phase 2 task 2-1). This logger does not auto-create the parent; that stays the caller's contract.
- Invalid `PORTAL_LOG_LEVEL` value (`"gibberish"`, empty, numeric) → default `LevelWarn`. Not an error. No stderr noise.
- Multibyte/UTF-8 messages preserved verbatim — `fmt.Sprintf` handles UTF-8; the file is opened as bytes.
- Message containing embedded `|` is written verbatim; grep-users know to split with a reasonable limit (`awk -F ' \\| ' '{...}'` with implicit limit 4 gives the four canonical fields).
- RFC 3339 UTC timestamp regardless of local zone — ensures grep / ordering across machines is stable.
- Empty `component` is prevented at compile site via the exported constants catalogue; free-form component strings are still possible (the signature is `string`) but discouraged. No runtime validation — Phase 2 task 2-7's API is preserved.
- `Close()` is idempotent-ish (calling it twice returns `os.ErrClosed` on the second call); callers should `defer logger.Close()` exactly once per opened logger.
- Rotation still happens only on `OpenLogger(path, true)` — this task does not change rotation policy; task 6-3 adds mid-write rotation for the daemon.

**Context**:
> Spec "Observability & Diagnostics → Log File":
> "**Format:** single line per entry.
> ```
> timestamp | level | component | message
> ```
> - `timestamp` — RFC 3339 UTC.
> - `level` — one of `DEBUG`, `INFO`, `WARN`, `ERROR`.
> - `component` — short identifier for the subsystem (`daemon`, `restore`, `hydrate`, `notify`, `hooks`, `bootstrap`).
> - `message` — free-form, human-readable."
>
> Spec "Observability & Diagnostics → Log File → Log level":
> "warnings and errors by default. `PORTAL_LOG_LEVEL=debug` env var enables verbose tracing for debugging sessions."
>
> Spec "Observability & Diagnostics → What gets logged":
> "Save failures (disk full, write errors, permission issues). Restoration warnings (missing scrollback file, layout fallback, corrupt `sessions.json`). Hydrate timeouts (3-second signal did not arrive). Helper crashes (in the rare cases where they can be observed). Bootstrap events at `DEBUG` level only."
>
> Phase 2 task 2-7 landed `OpenLogger(path, rotate)` with `Debug/Info/Warn/Error` methods already present. This task hardens the implementation — adds component constants, typed levels, explicit threshold parsing, mutex-guarded writes, UTC timestamp guarantee. The public surface is preserved; tests from Phase 2 remain green.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Observability & Diagnostics → Log File", "Observability & Diagnostics → What gets logged".

## built-in-session-resurrection-6-2 | approved

### Task 6-2: Non-daemon writers append via `O_APPEND` with no size check; retrofit every component's existing log call to the new logger

**Problem**: Per spec "Observability & Diagnostics → Log Rotation → Concurrent-writer discipline": "Only the daemon rotates. Every other Portal writer appends to `portal.log` with `O_APPEND` (POSIX guarantees atomic appends for writes smaller than `PIPE_BUF` — trivially satisfied by a one-line log entry). CLI / helper / subprocess writers do not check size and do not rotate." Today's `internal/state/logger.go` (Phase 2 + task 6-1) supports `OpenLogger(path, rotate)` — the daemon passes `true`, everyone else must pass `false`. But the **callers** are scattered: Phase 3's restore / hydrate / signal-hydrate / FIFO sweep code, Phase 4's hook firing in the hydrate helper + `migrate-rename`, Phase 5's `cmd/bootstrap/` orchestrator, and any ad-hoc `log.Println` / `fmt.Fprintln(os.Stderr, ...)` call sites left over. Without a retrofit, some subsystems still use stdlib `log`, some print to stderr, some don't log at all. A scattered approach defeats the single-log-file grep workflow `portal state status` (task 6-4) depends on.

**Solution**: Audit every log call site in `internal/state/*`, `internal/hooks/*`, `cmd/state_*.go`, `cmd/bootstrap/*`, and `internal/tmux/hooks_*.go`. For each: (a) replace `log.Println` / `log.Printf` / `fmt.Fprintf(os.Stderr, ...)` style calls with a `*state.Logger` obtained via `state.OpenLogger(state.PortalLog(dir), false)` — `rotate=false` because only the daemon rotates; (b) pick the appropriate `ComponentX` constant for the subsystem; (c) pick the appropriate level (`Debug` for bootstrap traces per spec, `Warn` for recoverable failures, `Error` for fatal conditions). Each subsystem that needs to log gets a helper `openLoggerNoRotate()` that calls `state.OpenLogger(state.PortalLog(dir), false)` — on open failure, the subsystem degrades to a no-op (logging must never crash callers). The helper `defer`s `Close()` at the end of each short-lived command (`notify`, `signal-hydrate`, `hydrate`); long-lived callers like `cmd/bootstrap/` reuse a single `*Logger` for the orchestrator's lifetime.

**Outcome**: A grep of the repo for `log.Println`, `log.Printf`, `log.Fatalf`, or raw `fmt.Fprint` into stderr returns zero matches inside resurrection code paths. Every warning / error / debug line routes through `*state.Logger` into `portal.log`. A user running `portal state status` sees "Recent warnings: N" (task 6-4) populated by entries from every subsystem — restore / hydrate / hooks / bootstrap — uniformly formatted. Non-daemon callers open the log with `O_APPEND|O_CREATE|O_WRONLY`, mode `0600`, never rotate, never check size. A CLI invocation that errors has its diagnostic logged even though no daemon was running at the time.

**Do**:
- Repository-wide audit with `Grep` for log call sites needing migration:
  - `log\.(Println|Printf|Fatalf|Print)` — stdlib log; none should remain in resurrection code (check `cmd/state_*.go`, `internal/state/*`, `internal/hooks/*`, `cmd/bootstrap/*`, `internal/tmux/hooks_*`).
  - `fmt\.Fprint(ln|f)?\(os\.Stderr` — excluding the *intentional* stderr one-liners for fatal bootstrap errors (tasks 6-8, 6-9, 6-10 own those; keep them in `cmd/bootstrap/` or at `PersistentPreRunE` layer).
  - For each found site, replace with the appropriate `logger.Warn/Error/Debug(ComponentX, format, args...)` call.
- Introduce a shared helper `cmd/state_common.go` (new file):
  ```go
  package cmd

  import (
      "io"
      "github.com/leeovery/portal/internal/state"
  )

  // openNoRotateLogger opens portal.log in non-rotating append mode.
  // On error, returns a no-op logger so callers never crash on missing log infra.
  func openNoRotateLogger() (*state.Logger, io.Closer) {
      dir, err := state.EnsureDir()
      if err != nil {
          return state.NopLogger(), noopCloser{}
      }
      l, err := state.OpenLogger(state.PortalLog(dir), false)
      if err != nil {
          return state.NopLogger(), noopCloser{}
      }
      return l, l
  }
  ```
  Add `state.NopLogger() *Logger` to `internal/state/logger.go` — returns a sentinel logger whose `emit` is a no-op. The `NopLogger` keeps callers from having to nil-check.
- Migrate call sites (one commit per subsystem is fine; the final state is the deliverable):
  - **`cmd/state_notify.go`** (Phase 2 task 2-2): replace any `log.Printf` or stderr print with `logger := openNoRotateLogger(); defer logger.Close(); logger.Warn(state.ComponentNotify, "...")`. `notify` is a hot-path binary — keep logging to genuine failures only (dir-create error, file-create error). No DEBUG lines in `notify`.
  - **`cmd/state_hydrate.go`** (Phase 3 tasks 3-8 / 3-9 / 3-10): use `ComponentHydrate`. Emit `Warn` on timeout (identifying `--hook-key`), `Warn` on file missing, `Warn` on hook read failure, `Debug` on successful dump (length, duration).
  - **`cmd/state_signal_hydrate.go`** (Phase 3 task 3-11): use `ComponentHydrate` (same component — this is part of the hydration pipeline). Emit `Warn` on FIFO retry exhaustion, `Debug` on enumeration stats.
  - **`internal/state/restore.go`** or equivalent Phase 3 file (task 3-6 `Restore()` orchestrator): use `ComponentRestore`. Emit `Warn` on corrupt `sessions.json`, `Warn` on `select-layout` fallback, `Warn` on per-session skip, `Debug` on per-session success.
  - **`cmd/state_migrate_rename.go`** (Phase 4 task 4-3): use `ComponentHooks`. Emit `Warn` on `hooks.json` write failure, `Debug` on zero-match no-op.
  - **`cmd/bootstrap/bootstrap.go`** (Phase 5 task 5-2 `Orchestrator`): use `ComponentBootstrap`. Emit `Debug` on step entry, `Warn` on `EnsureSaver` failure (soft), `Error` on fatal failures (those are also surfaced to stderr by task 6-8, but they must land in `portal.log` too per spec "Fatal Bootstrap Errors → every fatal also logs to `portal.log` at `ERROR`"). The orchestrator accepts a `*state.Logger` via its dependency struct.
  - **Daemon** (`cmd/state_daemon.go` from Phase 2 task 2-7): already logs — confirm it uses `ComponentDaemon` everywhere; migrate any remaining `fmt.Print` calls.
- Each migrated call site must close its logger via `defer logger.Close()` at the enclosing function scope. Long-lived components (daemon, orchestrator) close theirs on shutdown.
- Tests per migrated subsystem:
  - Inject a fake `*state.Logger` (or a real one pointed at `t.TempDir()`) and assert that a known failure path emits a line containing the expected component name.
  - Verify `O_APPEND`: pre-populate `portal.log` with a sentinel line; run the code path; assert the sentinel is still the first line of the file (no truncation).
  - Verify no rotation: pre-populate `portal.log` with 2MB of content; run a non-daemon code path; assert `portal.log.old` does NOT exist after the call and `portal.log` is still ≥ 2MB.
- Integration test in `internal/state/logger_test.go`:
  - `"non-daemon writer does not rotate even when log exceeds 1MB"`: create a 2MB `portal.log`; call `OpenLogger(path, false)`; emit a line; assert `portal.log.old` does not exist and `portal.log.Size() ≥ 2MB + one-line-length`.

**Acceptance Criteria**:
- [ ] `internal/state/logger.go` exports `NopLogger() *Logger` returning a sentinel whose emit is a no-op.
- [ ] `cmd/state_common.go` (or equivalent) provides `openNoRotateLogger()` that returns a real `*state.Logger` or the no-op sentinel on failure — callers never nil-check.
- [ ] Every subsystem from the migration list logs via `*state.Logger`; zero `log.Println/Printf/Fatalf` calls remain in resurrection code.
- [ ] Every non-daemon caller opens the log with `rotate=false`.
- [ ] Log file is created on first write with mode `0600` (O_APPEND|O_CREATE|O_WRONLY).
- [ ] Non-daemon caller does NOT rotate even when the log is ≥ 1MB (verified by integration test).
- [ ] Parent directory missing is tolerated by lazy-create via `state.EnsureDir()` in `openNoRotateLogger`.
- [ ] Permission error on log open does not crash the caller — logger degrades to no-op.
- [ ] Each migrated call site emits with the correct `ComponentX` constant from task 6-1.
- [ ] `portal state notify` / `signal-hydrate` / `hydrate` / `migrate-rename` all defer `logger.Close()` at their command root.

**Tests**:
- `"non-daemon writer does not rotate even when log exceeds 1MB"`
- `"notify logs a WARN to ComponentNotify on file-create failure"`
- `"hydrate logs a WARN to ComponentHydrate on FIFO timeout"`
- `"signal-hydrate logs a WARN to ComponentHydrate on FIFO retry exhaustion"`
- `"restore logs a WARN to ComponentRestore on corrupt sessions.json"`
- `"migrate-rename logs a WARN to ComponentHooks on hooks.json write failure"`
- `"bootstrap orchestrator logs a DEBUG to ComponentBootstrap on step entry"`
- `"bootstrap orchestrator logs a WARN to ComponentBootstrap on soft EnsureSaver failure"`
- `"bootstrap orchestrator logs an ERROR to ComponentBootstrap on fatal @portal-restoring failure"`
- `"logger degrades to no-op when portal.log cannot be opened (permission error)"`
- `"openNoRotateLogger returns a working logger when the state directory exists"`
- `"openNoRotateLogger returns NopLogger when EnsureDir fails"`
- `"append semantics preserved — pre-existing sentinel line survives subsequent writes"`
- `"zero residual log.Println/log.Printf/log.Fatalf calls in resurrection code (repo grep)"`

**Edge Cases**:
- Parent directory missing → `openNoRotateLogger` runs `state.EnsureDir()` first; creates `state/` with mode `0700` if absent. Caller does not need to worry.
- Log file permission error (e.g., dir is `0500`) → `OpenLogger` returns error; helper returns `NopLogger`. Caller's code path continues as though logging was a no-op. Spec: "permission error surfaces but does not crash caller."
- POSIX atomic append guarantee applies for writes < `PIPE_BUF` (4096 on Linux, higher on macOS). One-line log entries are trivially smaller. No lock contention across processes.
- Non-daemon caller running on a log that's already 10MB — writes a fresh line to the end; does not rotate; file grows until the next daemon start performs rotation on its next `OpenLogger(true)`. Spec: "If no daemon is running and log size grows past 1 MB, it continues to grow until the next daemon starts."
- Concurrent non-daemon writers — two CLI invocations logging simultaneously: each opens its own file descriptor with `O_APPEND`; POSIX guarantees atomic append per write system call. Each line appears intact; interleaving between lines is accepted and well-defined.
- Migration must preserve existing test expectations — any test that asserted on stderr output from a migrated call site needs to be updated to assert on the log file instead, or to assert stderr is empty.
- Fatal bootstrap errors (task 6-8) still emit stderr AND log — dual-destination. The log write must not block the stderr write; use two independent sinks.

**Context**:
> Spec "Observability & Diagnostics → Log Rotation → Concurrent-writer discipline":
> "Multiple Portal processes can log concurrently (daemon + CLI commands + hydrate helpers + signal-hydrate subprocesses). To avoid rotation races (two processes both observing ≥1 MB and both renaming, clobbering `portal.log.old`), **only the daemon rotates.** Every other Portal writer appends to `portal.log` with `O_APPEND` (POSIX guarantees atomic appends for writes smaller than `PIPE_BUF` — trivially satisfied by a one-line log entry). CLI / helper / subprocess writers do not check size and do not rotate. If no daemon is running and log size grows past 1 MB, it continues to grow until the next daemon starts, at which point the daemon's startup sequence performs a rotation check and rotates if needed."
>
> Spec "Observability & Diagnostics → What gets logged":
> "Save failures (disk full, write errors, permission issues). Restoration warnings (missing scrollback file, layout fallback, corrupt `sessions.json`). Hydrate timeouts (3-second signal did not arrive). Helper crashes (in the rare cases where they can be observed). Bootstrap events at `DEBUG` level only."
>
> Spec "Observability & Diagnostics → Fatal Bootstrap Errors":
> "every fatal also logs to `portal.log` at `ERROR`" — task 6-8 handles the stderr half; this task ensures the log-write half is wired through the shared logger.
>
> Task 6-1 introduced the component constants and level filtering; this task drives adoption across every subsystem. Phase 2 task 2-7 already wired the daemon — daemon-side migration is a minor sweep; non-daemon migration is the bulk of this task.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Observability & Diagnostics → Log File", "Observability & Diagnostics → Log Rotation → Concurrent-writer discipline", "Observability & Diagnostics → What gets logged".

## built-in-session-resurrection-6-3 | approved

### Task 6-3: Daemon-only mid-write rotation when the next write crosses >=1 MB

**Problem**: Phase 2 task 2-7 rotates at `OpenLogger(rotate=true)` call time — daemon startup checks size, rotates if ≥ 1MB, opens fresh file. That handles rotation across daemon restarts but not during a long-running daemon lifetime. A daemon that starts with `portal.log` at 500KB and logs for weeks will never rotate — the file grows unbounded until the next daemon restart (version bump, crash, `portal state cleanup`). The spec's 2 MB total budget (1 MB `portal.log` + 1 MB `portal.log.old`) is violated. This task adds mid-write rotation: before each write, if the pending write would push the file to ≥ 1MB, rename `portal.log` → `portal.log.old` first, open a fresh `portal.log`, then write the pending line there. Only the daemon does this — other writers (task 6-2) skip size checks per concurrent-writer discipline.

**Solution**: Introduce a `Logger.rotating bool` flag set by `OpenLogger(path, true)`. On each `emit`, *if* `rotating` is true: write the pending line first; then `Stat` the file; if `Stat.Size() >= 1MB`, close the current fd, rename → `.old` (replacing any existing `.old`), open a fresh fd with `O_APPEND|O_CREATE|O_WRONLY` mode `0600` for the NEXT write. The triggering write lands in the file that becomes `.old`, matching the spec's "On reaching 1 MB during a write, Portal renames `portal.log` → `portal.log.old` ... then starts a fresh `portal.log`" — `portal.log.old` may briefly contain content slightly over 1 MB by one log-line's worth; `portal.log` (the active file) starts fresh for subsequent writes. The write-then-stat-then-rotate sequence is protected by the logger's existing `sync.Mutex` — the daemon is the only rotating writer so there is no cross-process race (other writers skip the check entirely per task 6-2). On rename failure, log a single diagnostic to stderr (last-resort) and continue writing to the current file — rotation failure never blocks logging.

**Outcome**: Daemon running for weeks never lets `portal.log` exceed 1MB. When a write would cross the boundary, `portal.log` is renamed to `portal.log.old` (any existing `.old` is replaced, not appended), a fresh `portal.log` is opened, and the write lands there. Non-daemon writers (task 6-2) continue appending without size checks — if the daemon is down and non-daemon writers push the file past 1MB, the next daemon start rotates on `OpenLogger(true)` per the startup check from Phase 2 task 2-7.

**Do**:
- Edit `internal/state/logger.go`:
  - Add `rotating bool` field to `Logger` struct, set from the `rotate` argument to `OpenLogger`.
  - Add `path string` field so the logger knows its own file path (needed for rename + reopen).
  - Add constant `const LogRotateThreshold = 1 * 1024 * 1024` (exported so task 6-4 and tests can reference it).
  - Modify `emit`:
    ```go
    func (l *Logger) emit(level Level, component string, format string, args ...any) {
        if level < l.threshold {
            return
        }
        line := fmt.Sprintf("%s | %s | %s | %s\n",
            time.Now().UTC().Format(time.RFC3339),
            levelNames[level], component,
            fmt.Sprintf(format, args...))
        l.mu.Lock()
        defer l.mu.Unlock()
        _, _ = l.file.Write([]byte(line))
        if l.rotating {
            l.maybeRotate()
        }
    }
    ```
  - Add `func (l *Logger) maybeRotate()` — runs AFTER the write, so the triggering write lands in the file being renamed to `.old`:
    1. `st, err := l.file.Stat()`; on error, log to stderr and return (keep writing to current file).
    2. If `st.Size() < LogRotateThreshold`, return.
    3. `if err := l.file.Close(); err != nil { ... best-effort log ... }`.
    4. `oldPath := l.path + ".old"` (equivalent to `state.PortalLogOld(dir)`; logger uses its `path` field).
    5. `_ = os.Remove(oldPath)` — replace any existing `.old`, ignore `ENOENT`.
    6. `if err := os.Rename(l.path, oldPath); err != nil { fmt.Fprintf(os.Stderr, "portal: log rotation failed: %v\n", err); /* reopen same file, continue */ }`.
    7. `f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)`; on error, fall back to stderr and leave `l.file = nil` (emit will no-op on nil file).
    8. `l.file = f` — fresh file for the NEXT write.
- Unit tests in `internal/state/logger_test.go` (subprocess-free; use `t.TempDir()` and tight `os.Stat` assertions):
  - `"daemon rotates after a write that crosses the 1 MB boundary"`: pre-populate `portal.log` with `LogRotateThreshold - 200` bytes of content; open logger with `rotate=true`; emit a ~100-byte line (post-write size still < 1MB — no rotate); then emit another ~150-byte line that pushes the file ≥ 1MB. Assert `portal.log.old` exists containing the pre-existing content + both lines, and `portal.log` is fresh (empty or contains only subsequent writes).
  - `"daemon rotation triggers at exactly 1 MB (inclusive threshold on post-write size)"`: pre-populate `portal.log` so the next write lands the file at exactly `LogRotateThreshold` bytes; emit; assert rotation fired (`portal.log.old` exists, `portal.log` is fresh).
  - `"existing portal.log.old is replaced on rotation"`: create `portal.log.old` with sentinel content; trigger rotation; assert `portal.log.old` content matches the new pre-rotation `portal.log`, not the sentinel.
  - `"rotation-rename failure logs to stderr and continues with current file"`: mock `os.Rename` to fail (inject via a package-level var or by making `portal.log.old` undeletable with a directory-permission trick on Linux); emit; assert stderr contains a rotation-failed line; assert `portal.log` still receives the write.
  - `"the triggering write lands in portal.log.old (the file being rotated)"`: pre-populate with `LogRotateThreshold - 50` bytes; emit a 60-byte line (post-write size = `LogRotateThreshold + 10` ≥ 1MB); assert the 60-byte line is in `portal.log.old` (alongside the 50-byte historical content) and `portal.log` is fresh for the next write.
  - `"daemon restart between size check and rename tolerated"`: open logger with rotate=true, emit enough to trigger rotation, kill & relaunch logger, verify new logger sees a well-formed file state and continues writing. (Subprocess test.)
  - `"non-daemon caller never reaches mid-write rotation code path"`: open logger with `rotate=false`; pre-populate with 2MB; emit a line; assert `portal.log.old` does NOT exist, `portal.log` grew by the line's length (this is the regression guard for the concurrent-writer discipline).
- Update `cmd/state_daemon.go` to trust the Phase 2 setup — it already calls `OpenLogger(path, true)`. No code change in the daemon; all behaviour lands in the logger itself.

**Acceptance Criteria**:
- [ ] `Logger.emit` in `internal/state/logger.go` calls `maybeRotate` only when `rotating == true`.
- [ ] `maybeRotate` uses `LogRotateThreshold` (exported `= 1 * 1024 * 1024`) as the post-write inclusive boundary (post-write `size >= threshold` triggers rotation).
- [ ] Rotation replaces any existing `portal.log.old` via `os.Remove(oldPath)` before `os.Rename`.
- [ ] On rotation-rename failure, a single diagnostic is written to stderr and the logger continues writing to the current file (degraded, not crashed).
- [ ] The triggering write lands in `portal.log.old` (the file being rotated) — matches spec "On reaching 1 MB during a write, Portal renames `portal.log` → `portal.log.old` ... then starts a fresh `portal.log`". `portal.log.old` may briefly contain content slightly over 1 MB by one log-line's worth.
- [ ] Non-daemon callers (task 6-2) never reach `maybeRotate` — the `rotating` flag gates the whole code path.
- [ ] Existing Phase 2 startup-rotation (Phase 2 task 2-7) continues to work unchanged (rotation on `OpenLogger(true)` when size ≥ threshold).
- [ ] `Logger.path` field is populated at construction so `maybeRotate` knows the rename target.

**Tests**:
- `"daemon rotates after a write that crosses the 1 MB boundary"`
- `"daemon rotation triggers at exactly 1 MB (inclusive threshold on post-write size)"`
- `"existing portal.log.old is replaced on rotation"`
- `"rotation-rename failure logs to stderr and continues with current file"`
- `"the triggering write lands in portal.log.old (the file being rotated)"`
- `"daemon restart between size check and rename is tolerated"`
- `"non-daemon caller never reaches mid-write rotation code path"`
- `"startup rotation from Phase 2 task 2-7 still works"`

**Edge Cases**:
- Inclusive post-write threshold (`>= LogRotateThreshold`). Matches spec phrasing "On reaching 1 MB during a write".
- Existing `portal.log.old` replaced, not appended. `os.Remove` before `os.Rename`. If the remove fails for `ENOENT`, ignore. If it fails for other reasons (permission), the subsequent rename will error and the stderr diagnostic fires.
- Rotation-rename failure (e.g., dir is read-only, `.old` is locked on some platform): logger logs to stderr once, keeps writing to the current file. Rotation is attempted again on the NEXT write that crosses the threshold — every future write hits the same check, so transient conditions self-heal.
- Write that crosses the boundary: per spec the trigger is "during a write", so the triggering write lands in the file that becomes `.old`. `portal.log.old` may briefly exceed 1MB by one log-line's worth; `portal.log` (the active file) starts fresh for subsequent writes.
- Daemon restart between size check and rename: the next daemon's `OpenLogger(path, true)` re-checks at startup and rotates again if `portal.log` is still ≥ 1MB. Harmless; the check is idempotent.
- Non-daemon callers never reach this code path — the `rotating` bool short-circuits at the top of `maybeRotate`. This is the load-bearing invariant preventing the rotation-race the spec warns about.
- Logger reopened after rotation failure: if `os.OpenFile` fails in step 7 of `maybeRotate`, `l.file` becomes nil; subsequent `emit` calls check for nil and no-op. Caller never crashes. Diagnosable via the stderr one-liner.

**Context**:
> Spec "Observability & Diagnostics → Log Rotation":
> "Simple 2-file cap at **1 MB per file**.
> - On reaching 1 MB during a write, Portal renames `portal.log` → `portal.log.old` (replacing any existing old file), then starts a fresh `portal.log`.
> - Total disk usage bounded at ~2 MB.
> - Portal performs rotation itself in-process. No external `logrotate` dependency."
>
> Spec "Observability & Diagnostics → Log Rotation → Concurrent-writer discipline":
> "only the daemon rotates. Every other Portal writer appends to `portal.log` with `O_APPEND` ... CLI / helper / subprocess writers do not check size and do not rotate. If no daemon is running and log size grows past 1 MB, it continues to grow until the next daemon starts, at which point the daemon's startup sequence performs a rotation check and rotates if needed."
>
> Phase 2 task 2-7 landed startup-time rotation (on `OpenLogger(rotate=true)` when file ≥ 1MB). This task adds mid-write rotation for long-running daemons. Both paths coexist: startup handles the "daemon down → other writers grew the file" case; mid-write handles the "daemon up for weeks" case.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "Observability & Diagnostics → Log Rotation".

## built-in-session-resurrection-6-4 | approved

### Task 6-4: Implement `portal state status` data collection (daemon liveness, last-save, counts, state size, recent warnings)

**Problem**: The spec lists `portal state status` as the primary user-facing diagnostic. Task 1-1 scaffolded the command stub. Nothing populates it today. Users will run `portal state status` when they suspect Portal isn't saving / restoring, and the command must surface enough data to diagnose: is the daemon up? how recent is the last save? how much disk does state use? any recent warnings in the log? This task implements the **data-collection layer** — a pure function that gathers all six fields (daemon liveness with PID + version, last save, sessions-captured count, panes-captured count, state size, recent warnings) into a `StatusReport` struct. Task 6-5 then renders the struct into output and computes the exit code. Splitting collection from rendering lets the `--json` path (a future extension) reuse the collection layer unchanged.

**Solution**: Create `internal/state/status.go` defining `type StatusReport struct { ... }` and `func CollectStatus(dir string, now time.Time) (*StatusReport, error)`. `CollectStatus` reads `daemon.pid` + `daemon.version` (via Phase 2 task 2-4 `ReadPIDFile` / `ReadVersionFile`), runs `IsProcessAlive(pid)` to determine liveness, reads `sessions.json` (via Phase 2 task 2-3 decoder) for counts + `saved_at`, walks `state/` with `filepath.WalkDir` to compute total disk usage, and scans `portal.log` for WARN/ERROR entries within the last hour (parameterised as `now - 1*time.Hour` so tests can inject time deterministically). Missing files are handled as zero / "never" / "not running" per spec — never surfaced as errors. The function only returns an error for genuinely exceptional conditions (e.g., permission denied on the state directory itself).

**Outcome**: `CollectStatus("/home/user/.config/portal/state", time.Now())` returns a fully-populated `*StatusReport` with every field the spec's example output mentions. A healthy Portal reports `DaemonRunning: true`, `DaemonPID: 12345`, `DaemonVersion: "v0.4.2"`, `LastSaveAt: <recent time>`, `SessionsCount: 10`, `PanesCount: 34`, `StateSize: 19088896` bytes, `RecentWarnings: 0`. An unhealthy Portal (daemon down, stale save, errors in log) populates fields accordingly, and the caller (task 6-5) computes the exit code + renders the output.

**Do**:
- Create `internal/state/status.go`:
  ```go
  package state

  import (
      "bufio"
      "errors"
      "io/fs"
      "os"
      "path/filepath"
      "strings"
      "time"
  )

  type StatusReport struct {
      DaemonRunning  bool
      DaemonPID      int
      DaemonVersion  string
      LastSaveAt     time.Time // zero if no sessions.json
      HasLastSave    bool      // false if no sessions.json
      SessionsCount  int
      PanesCount     int
      StateSize      int64     // bytes
      RecentWarnings int
      LastWarning    string    // last raw log line matched, "" if none
  }

  func CollectStatus(dir string, now time.Time) (*StatusReport, error) {
      r := &StatusReport{}
      // Daemon liveness
      pid, perr := ReadPIDFile(dir)
      if perr == nil {
          r.DaemonPID = pid
          r.DaemonRunning = IsProcessAlive(pid)
      } // missing/unparseable PID file → DaemonRunning=false, DaemonPID=0
      if v, err := ReadVersionFile(dir); err == nil {
          r.DaemonVersion = v
      }
      // Last save + counts from sessions.json
      idx, err := ReadSessionsJSON(dir) // Phase 3 task 3-1 reader
      if err == nil && idx != nil {
          r.HasLastSave = true
          r.LastSaveAt = idx.SavedAt
          r.SessionsCount = len(idx.Sessions)
          for _, s := range idx.Sessions {
              for _, w := range s.Windows {
                  r.PanesCount += len(w.Panes)
              }
          }
      }
      // State size: walk dir, sum sizes
      size, _ := computeStateSize(dir)
      r.StateSize = size
      // Recent warnings
      logPath := PortalLog(dir)
      count, last, _ := scanRecentWarnings(logPath, now.Add(-1*time.Hour))
      r.RecentWarnings = count
      r.LastWarning = last
      return r, nil
  }

  func computeStateSize(dir string) (int64, error) {
      var total int64
      err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
          if err != nil {
              if errors.Is(err, fs.ErrNotExist) {
                  return fs.SkipDir
              }
              return nil // tolerate per-entry errors
          }
          if !d.IsDir() {
              info, ierr := d.Info()
              if ierr == nil {
                  total += info.Size()
              }
          }
          return nil
      })
      if errors.Is(err, fs.ErrNotExist) {
          return 0, nil
      }
      return total, err
  }

  func scanRecentWarnings(logPath string, cutoff time.Time) (int, string, error) {
      f, err := os.Open(logPath)
      if err != nil {
          if errors.Is(err, fs.ErrNotExist) {
              return 0, "", nil
          }
          return 0, "", err
      }
      defer f.Close()
      sc := bufio.NewScanner(f)
      sc.Buffer(make([]byte, 64*1024), 1024*1024)
      count := 0
      last := ""
      for sc.Scan() {
          line := sc.Text()
          fields := strings.SplitN(line, " | ", 4)
          if len(fields) < 4 {
              continue // malformed, skip
          }
          ts, terr := time.Parse(time.RFC3339, fields[0])
          if terr != nil {
              continue
          }
          if ts.Before(cutoff) {
              continue
          }
          level := fields[1]
          if level != "WARN" && level != "ERROR" {
              continue
          }
          count++
          last = line
      }
      return count, last, sc.Err()
  }
  ```
- Tests in `internal/state/status_test.go`:
  - `"CollectStatus returns DaemonRunning=false when daemon.pid is missing"` — no file, assert `DaemonRunning==false`, `DaemonPID==0`.
  - `"CollectStatus returns DaemonRunning=true for a live PID"` — write `os.Getpid()` to `daemon.pid`; assert `DaemonRunning==true`.
  - `"CollectStatus returns DaemonRunning=false for a dead PID"` — spawn `exec.Command("sleep", "30")`; record PID; kill + wait; write PID to `daemon.pid`; assert `DaemonRunning==false`.
  - `"CollectStatus returns HasLastSave=false when sessions.json is missing"` — no file, assert `HasLastSave==false`, `SessionsCount==0`, `PanesCount==0`.
  - `"CollectStatus returns populated fields when sessions.json has 2 sessions with 3+2 panes"` — fixture, assert `SessionsCount==2`, `PanesCount==5`.
  - `"CollectStatus returns LastSaveAt matching sessions.json saved_at"` — fixture with `saved_at: "2026-04-23T10:30:00Z"`, assert parsed time matches.
  - `"CollectStatus returns StateSize==0 when directory is missing"` — dir absent, assert `StateSize==0`, no error.
  - `"CollectStatus returns StateSize as sum of file sizes in state dir"` — write 3 files of sizes 100, 200, 300; assert `StateSize==600`.
  - `"CollectStatus includes scrollback/ subdirectory contents in StateSize"` — file in `scrollback/` included.
  - `"CollectStatus returns RecentWarnings==0 when portal.log is missing"` — no file, assert count=0, no error.
  - `"CollectStatus does NOT scan portal.log.old for recent warnings"` — put WARN entries in `.old`, none in `.log`; assert `RecentWarnings==0`.
  - `"CollectStatus counts WARN and ERROR entries within the 1-hour window"` — log with 3 WARN + 2 ERROR within window, 5 outside window; assert count=5.
  - `"CollectStatus ignores INFO and DEBUG entries"` — log with only INFO/DEBUG entries; assert count=0.
  - `"CollectStatus tolerates malformed log entries"` — log with 2 malformed + 2 valid WARN; assert count=2, no error.
  - `"CollectStatus uses the caller-supplied 'now' for the 1-hour window"` — pass a `now` explicitly, verify cutoff is `now - 1h`.
  - `"CollectStatus's LastWarning holds the last valid WARN/ERROR line in temporal order"`.
  - `"CollectStatus treats daemon.pid permission denied as not-running"` — chmod 0000; assert `DaemonRunning==false`, no error bubble up (graceful).

**Acceptance Criteria**:
- [ ] `internal/state/status.go` defines `StatusReport` with the fields listed above.
- [ ] `CollectStatus(dir, now)` returns a `*StatusReport` populated from disk.
- [ ] Missing `daemon.pid` → `DaemonRunning=false`, `DaemonPID=0`, no error.
- [ ] Dead PID → `DaemonRunning=false`.
- [ ] `signal(0)` permission denied → treated as not-running (no error bubbled).
- [ ] Missing `sessions.json` → `HasLastSave=false`, `SessionsCount=0`, `PanesCount=0`, no error.
- [ ] `SessionsCount` == len(idx.Sessions); `PanesCount` == sum of panes across all windows.
- [ ] `StateSize` is the byte sum of all regular files under the state directory (including `scrollback/`).
- [ ] Missing state directory → `StateSize=0`, no error.
- [ ] `RecentWarnings` counts log entries in the last 1 hour measured from the caller-supplied `now`.
- [ ] `portal.log.old` is NEVER scanned.
- [ ] Missing `portal.log` → `RecentWarnings=0`, no error.
- [ ] Malformed log entries tolerated and skipped without error.
- [ ] Only `WARN` and `ERROR` level entries are counted.
- [ ] `LastWarning` holds the last valid WARN/ERROR line encountered (by scan order).

**Tests**:
- `"CollectStatus returns DaemonRunning=false when daemon.pid is missing"`
- `"CollectStatus returns DaemonRunning=true for a live PID"`
- `"CollectStatus returns DaemonRunning=false for a dead PID"`
- `"CollectStatus treats daemon.pid permission denied as not-running"`
- `"CollectStatus returns HasLastSave=false when sessions.json is missing"`
- `"CollectStatus populates SessionsCount and PanesCount from sessions.json"`
- `"CollectStatus returns LastSaveAt matching sessions.json saved_at"`
- `"CollectStatus returns StateSize==0 when directory is missing"`
- `"CollectStatus returns StateSize as sum of file sizes in state dir"`
- `"CollectStatus includes scrollback/ subdirectory contents in StateSize"`
- `"CollectStatus returns RecentWarnings==0 when portal.log is missing"`
- `"CollectStatus does NOT scan portal.log.old"`
- `"CollectStatus counts WARN and ERROR entries within the 1-hour window"`
- `"CollectStatus ignores INFO and DEBUG entries"`
- `"CollectStatus tolerates malformed log entries"`
- `"CollectStatus uses the caller-supplied now for the 1-hour window"`
- `"CollectStatus LastWarning holds the last valid WARN/ERROR line"`

**Edge Cases**:
- `sessions.json` missing → counts = 0, `HasLastSave=false`, `LastSaveAt` is the zero value — renderer (task 6-5) shows "never".
- `daemon.pid` missing or unparseable → `DaemonRunning=false`, `DaemonPID=0`.
- `signal(0)` permission denied (rare — the PID belongs to another user) → `IsProcessAlive` returns false. Not an error path.
- State directory missing → `StateSize=0`. `WalkDir` returns `fs.ErrNotExist` on the root; function catches it and returns zero.
- `portal.log` missing → zero recent warnings, no error. Missing log is healthy, not a failure signal.
- `portal.log.old` never scanned — entries there are considered historical. Task explicitly asserts this via test fixture placing WARNs in `.old` and asserting count=0.
- Malformed log entries (< 4 pipe-separated fields, unparseable timestamp) are tolerated and skipped — scanner continues to the next line.
- 1-hour window is inclusive of `now - 1h` boundary: `ts.Before(cutoff)` filters strictly older entries; `ts >= cutoff` entries are kept.
- `now` is caller-supplied (not `time.Now()` inside `CollectStatus`) so tests can pin the cutoff deterministically.
- Large `portal.log` (near 1MB) scan time: a 1MB file is ~16k lines at 64 bytes/line; `bufio.Scanner` handles this in milliseconds. No performance optimisation needed.
- Nested directories under `scrollback/` handled by `filepath.WalkDir` recursion.
- Symbolic links in state dir: `filepath.WalkDir` follows by default. Symlinks inside `state/` are rare; if present, they are counted. Documented behaviour; no special handling.

**Context**:
> Spec "CLI Surface → `portal state status`":
> "**Output (example):**
> ```
> Portal state:
>   Save daemon: running (pid 12345, version v0.4.2)
>   Last save: 12 seconds ago
>   Sessions captured: 10
>   Panes captured: 34
>   State size: 18.2 MB on disk
>   Recent warnings: 0 (last: none)
> ```
> **Data sources:**
> - **Daemon liveness:** `has-session -t _portal-saver` plus process verification (pane command resolves to `portal state daemon`).
> - **Last save time:** `sessions.json.saved_at`.
> - **Session / pane counts:** parsed from `sessions.json`.
> - **State size:** total disk usage under `~/.config/portal/state/`.
> - **Recent warnings:** scan `portal.log` for entries within the last hour."
>
> Spec "CLI Surface → `portal state status` → Scan window semantics":
> "'Recent warnings' and the exit-code 'recent errors' check use the **same one-hour window**, measured from now back over entries in the *current* `portal.log` file only. `portal.log.old` is not scanned (its entries are considered older historical data, not 'recent'). If `portal.log` does not exist (first-ever run, no warnings logged yet), both displayed count and exit-code treatment are **zero/healthy** — missing log file is never itself a warning."
>
> Spec footnote on daemon-liveness: this task uses **PID-file + signal(0)** (per Phase 2 task 2-4) rather than `has-session -t _portal-saver`. The spec's "Lifecycle Summary" prefers PID+signal(0) over `has-session`/`pane_current_command` ("definitive" liveness). Task 6-5 optionally adds a `has-session` sanity check but the authoritative signal is PID+signal(0).

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "CLI Surface → `portal state status`", "Observability & Diagnostics → `portal state status`", "Save-Side Architecture → Execution Model → Lifecycle Summary → Liveness verification".

## built-in-session-resurrection-6-5 | approved

### Task 6-5: Render `portal state status` output and compute exit code

**Problem**: Task 6-4 provides the data (`StatusReport`). This task wires `cmd/state_status.go`'s `RunE` to call `CollectStatus`, render the documented output, and compute the exit code. Rendering is simple but has several display-formatting concerns: "12 seconds ago" relative time, "18.2 MB on disk" binary-unit formatting, "(last: none)" vs "(last: <log line snippet>)". Exit code has three non-zero conditions: daemon not running, last save > 5 minutes ago, any recent WARN/ERROR in the log. The spec excludes `--json` from v1.

**Solution**: In `cmd/state_status.go`, replace the stub `RunE` with:
1. Resolve state directory via `state.ResolveDir()` (no create — status should be read-only).
2. Call `state.CollectStatus(dir, time.Now())`.
3. Render the six-line output using `fmt.Fprintln` on `cmd.OutOrStdout()`.
4. Compute exit code: `0` if all healthy; non-zero if any of (a) `!DaemonRunning`, (b) `HasLastSave && now.Sub(LastSaveAt) > 5*time.Minute`, (c) `RecentWarnings > 0`. Return a sentinel error (`ErrStatusUnhealthy`) that Cobra surfaces as exit 1. Set `cmd.SilenceErrors = true` on the status command so the sentinel does not print — the output has already been rendered on stdout.

Render helpers:
- `formatDuration(d time.Duration) string`: "just now" (< 2s), "N seconds ago" (< 60s), "N minutes ago" (< 60m), "N hours ago" (< 24h), "N days ago" (≥ 24h). `saved_at` zero value renders as "never".
- `formatBytes(b int64) string`: binary units (KiB style but spec says "18.2 MB" so use KB/MB/GB/TB with binary-1024 math for consistency with most disk tools): `< 1024` → `"B"`, `< 1024²` → `"KB"`, etc. Format with 1 decimal for values ≥ 1 unit up. Per spec example "18.2 MB", use "MB" not "MiB" despite binary math — matches the spec's display string.

**Outcome**: `portal state status` on a healthy server prints:
```
Portal state:
  Save daemon: running (pid 12345, version v0.4.2)
  Last save: 12 seconds ago
  Sessions captured: 10
  Panes captured: 34
  State size: 18.2 MB on disk
  Recent warnings: 0 (last: none)
```
and exits 0. An unhealthy server prints:
```
Portal state:
  Save daemon: not running
  Last save: 7 minutes ago
  Sessions captured: 5
  Panes captured: 12
  State size: 2.1 MB on disk
  Recent warnings: 3 (last: 2026-04-23T10:28:15Z | WARN | hydrate | FIFO timeout for work:0.0)
```
and exits 1.

**Do**:
- Edit `cmd/state_status.go`:
  - Replace stub `RunE`:
    ```go
    RunE: func(cmd *cobra.Command, args []string) error {
        dir, err := state.ResolveDir()
        if err != nil {
            return fmt.Errorf("resolve state dir: %w", err)
        }
        now := time.Now()
        r, err := state.CollectStatus(dir, now)
        if err != nil {
            return fmt.Errorf("collect status: %w", err)
        }
        renderStatus(cmd.OutOrStdout(), r, now)
        if isUnhealthy(r, now) {
            return ErrStatusUnhealthy
        }
        return nil
    },
    ```
  - Set `stateStatusCmd.SilenceErrors = true` so `ErrStatusUnhealthy` propagates as exit 1 without Cobra re-printing. Also set `SilenceUsage = true` to suppress usage on error (rendering already completed).
  - Define `ErrStatusUnhealthy = errors.New("")` — zero-content message so Cobra never prints anything. Alternative: return an error whose `Error()` returns "" — verify Cobra still propagates non-zero exit.
  - Implement `renderStatus(w io.Writer, r *state.StatusReport, now time.Time)`:
    ```go
    fmt.Fprintln(w, "Portal state:")
    if r.DaemonRunning {
        fmt.Fprintf(w, "  Save daemon: running (pid %d, version %s)\n", r.DaemonPID, renderVersion(r.DaemonVersion))
    } else {
        fmt.Fprintln(w, "  Save daemon: not running")
    }
    if r.HasLastSave {
        fmt.Fprintf(w, "  Last save: %s\n", formatDuration(now.Sub(r.LastSaveAt)))
    } else {
        fmt.Fprintln(w, "  Last save: never")
    }
    fmt.Fprintf(w, "  Sessions captured: %d\n", r.SessionsCount)
    fmt.Fprintf(w, "  Panes captured: %d\n", r.PanesCount)
    fmt.Fprintf(w, "  State size: %s on disk\n", formatBytes(r.StateSize))
    if r.RecentWarnings == 0 {
        fmt.Fprintln(w, "  Recent warnings: 0 (last: none)")
    } else {
        fmt.Fprintf(w, "  Recent warnings: %d (last: %s)\n", r.RecentWarnings, r.LastWarning)
    }
    ```
  - `renderVersion(v string) string`: return `v` if non-empty, `"unknown"` if empty.
  - Implement `isUnhealthy(r *state.StatusReport, now time.Time) bool`:
    ```go
    if !r.DaemonRunning {
        return true
    }
    if r.HasLastSave && now.Sub(r.LastSaveAt) > 5*time.Minute {
        return true
    }
    if r.RecentWarnings > 0 {
        return true
    }
    return false
    ```
    Note: `!r.HasLastSave` (no sessions.json yet) does NOT trigger unhealthy — a fresh install with nothing saved yet is still healthy if the daemon is running. Spec is silent on this; the interpretation is "last save > 5 min" means "the save cadence is broken", which doesn't apply when there's simply nothing saved yet. Task's acceptance criterion makes this explicit.
  - Implement `formatDuration`:
    ```go
    func formatDuration(d time.Duration) string {
        switch {
        case d < 2*time.Second:
            return "just now"
        case d < time.Minute:
            return fmt.Sprintf("%d seconds ago", int(d.Seconds()))
        case d < time.Hour:
            return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
        case d < 24*time.Hour:
            return fmt.Sprintf("%d hours ago", int(d.Hours()))
        default:
            return fmt.Sprintf("%d days ago", int(d.Hours())/24)
        }
    }
    ```
  - Implement `formatBytes`:
    ```go
    func formatBytes(b int64) string {
        const unit = 1024
        if b < unit {
            return fmt.Sprintf("%d B", b)
        }
        div, exp := int64(unit), 0
        for n := b / unit; n >= unit; n /= unit {
            div *= unit
            exp++
        }
        return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
    }
    ```
- Tests in `cmd/state_status_test.go`:
  - `"it renders the healthy-state six-line layout"` — fixture with all healthy fields; assert stdout matches the documented example line-for-line.
  - `"it renders 'not running' when daemon is absent"`.
  - `"it renders 'Last save: never' when sessions.json is missing"`.
  - `"it renders '(last: none)' when RecentWarnings == 0"`.
  - `"it renders '(last: <line>)' when RecentWarnings > 0"`.
  - `"it exits 0 when healthy"` — mock `CollectStatus` to return a healthy report; assert `RunE` returns `nil`.
  - `"it exits non-zero when daemon not running"`.
  - `"it exits non-zero when last save > 5 minutes ago"`.
  - `"it exits non-zero when RecentWarnings > 0"`.
  - `"it exits 0 when HasLastSave is false (fresh install)"` — daemon running, no sessions.json, zero warnings → healthy.
  - `"it renders version 'unknown' when DaemonVersion is empty"`.
  - `"formatDuration: just now for <2s"`, `"... seconds ago for <1m"`, etc. (table-driven).
  - `"formatBytes: 500 B, 1.5 KB, 18.2 MB, 2.0 GB"` (table-driven; binary math).
  - `"SilenceErrors is true on stateStatusCmd"` — reflection test asserting the cobra command's field.
  - `"SilenceUsage is true on stateStatusCmd"`.

**Acceptance Criteria**:
- [ ] `stateStatusCmd.RunE` calls `state.CollectStatus` and `renderStatus`.
- [ ] Output format matches the spec example line-for-line when all fields are healthy.
- [ ] "Save daemon: running (pid N, version V)" for live daemon; "Save daemon: not running" otherwise.
- [ ] "Last save: X ago" when `HasLastSave`; "Last save: never" otherwise.
- [ ] "State size: X.Y MB on disk" (binary math, spec-matching unit labels `B`/`KB`/`MB`/`GB`/`TB`).
- [ ] "Recent warnings: 0 (last: none)" when zero; "Recent warnings: N (last: <line>)" when positive.
- [ ] Exit code 0 when healthy; non-zero when daemon not running OR last save > 5 min ago OR RecentWarnings > 0.
- [ ] Healthy with no sessions.json yet (fresh install) exits 0.
- [ ] `SilenceErrors = true` + `SilenceUsage = true` set on the command (so a non-zero exit from the sentinel doesn't print extra noise).
- [ ] `--json` flag is NOT present (explicitly out of v1 scope per spec).
- [ ] Version rendered as "unknown" when empty (covers empty/missing daemon.version).
- [ ] No emojis, no colors, no tables — plain text per Portal's existing CLI output conventions.

**Tests**:
- `"it renders the healthy-state six-line layout"`
- `"it renders 'not running' when daemon is absent"`
- `"it renders 'Last save: never' when sessions.json is missing"`
- `"it renders '(last: none)' when RecentWarnings == 0"`
- `"it renders '(last: <line>)' when RecentWarnings > 0"`
- `"it exits 0 when healthy"`
- `"it exits non-zero when daemon not running"`
- `"it exits non-zero when last save > 5 minutes ago"`
- `"it exits non-zero when RecentWarnings > 0"`
- `"it exits 0 when HasLastSave is false (fresh install)"`
- `"it renders version 'unknown' when DaemonVersion is empty"`
- `"formatDuration covers the expected bucketing (just now / seconds / minutes / hours / days)"`
- `"formatBytes covers B / KB / MB / GB bucketing"`
- `"SilenceErrors and SilenceUsage are set to suppress Cobra's default error/usage output"`
- `"--json flag is not registered in v1"`

**Edge Cases**:
- Daemon not running → non-zero exit regardless of other fields.
- Last save > 5 min ago → non-zero exit even if daemon is running (the daemon is up but stuck).
- RecentWarnings > 0 → non-zero exit (something went wrong in the last hour).
- HasLastSave = false + DaemonRunning = true + RecentWarnings = 0 → healthy. Fresh install hasn't captured anything yet; not a failure.
- State size 0 B on a fresh install renders as "0 B on disk".
- Empty daemon.version → "Save daemon: running (pid 12345, version unknown)".
- Very old last save (7 days ago) → "Last save: 7 days ago" + non-zero exit (vastly past 5-min threshold).
- 1 day exactly → "1 days ago" (don't bother with singular/plural — matches utilitarian CLI tone).
- Binary-math byte formatting: `18.2 MB` = `18.2 * 1024 * 1024 = 19,084,083 B`; spec example "18.2 MB on disk" implies binary math used with MB label. Documented choice.
- Non-TTY output (piped to file): same format, no ANSI codes. Portal's existing `fmt.Fprintln` path does not colorise unless explicitly directed.
- The command is exempt from bootstrap per task 5-1 (`state` in `skipTmuxCheck`); `CollectStatus` reads disk only. Running with no tmux server running is fine — daemon is just "not running".
- `time.Now()` captured once in `RunE` and passed to both `CollectStatus` and `renderStatus` + `isUnhealthy` for a consistent cutoff.

**Context**:
> Spec "CLI Surface → `portal state status`":
> "**Output (example):**
> ```
> Portal state:
>   Save daemon: running (pid 12345, version v0.4.2)
>   Last save: 12 seconds ago
>   Sessions captured: 10
>   Panes captured: 34
>   State size: 18.2 MB on disk
>   Recent warnings: 0 (last: none)
> ```
> **Exit code:**
> - `0` — healthy (daemon running, last save recent, no recent warnings).
> - non-zero — notable problem (daemon not running, last save older than 5 minutes, recent errors in the log)."
>
> Spec "CLI Surface → `portal state status` → Scan window semantics": consistent 1-hour window for display and exit code; `portal.log.old` not scanned; missing log file is healthy/zero.
>
> Task 6-4 produces the `StatusReport`; this task renders + decides exit code. `--json` is explicitly excluded from v1 per the spec's silence on structured output and the broader YAGNI stance.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "CLI Surface → `portal state status`".

## built-in-session-resurrection-6-6 | approved

### Task 6-6: Add daemon-kill action to `portal state cleanup` (SIGHUP-final-flush via `kill-session -t _portal-saver`)

**Problem**: Phase 1 task 1-9 shipped `portal state cleanup` with only the hook-removal action. The spec defines three actions in a specific order: (1) kill `_portal-saver`, (2) remove hook entries, (3) optionally `--purge` the state directory. Action (1) relies on `_portal-saver` existing — Phase 2 landed that. This task adds action (1): invoke `kill-session -t _portal-saver` which delivers SIGHUP to the daemon via PTY close; the daemon's handler performs a final atomic flush (Phase 2 task 2-12) and exits. Ordering matters: daemon-kill runs BEFORE hook removal so the daemon's final flush captures pre-cleanup state (with hooks still registered, the flush's captured sessions represent the real "as-of-cleanup" tmux state; removing hooks first would merely leave a dead hook-registered server for the brief flush window). The action is idempotent: "no `_portal-saver` session" is not an error.

**Solution**: In `cmd/state_cleanup.go`, prepend a `killSaver(client)` step before the existing `UnregisterPortalHooks` call. `killSaver` runs `client.HasSession("_portal-saver")` first; if false, returns nil (idempotent). Else calls `client.KillSession("_portal-saver")`. Tmux's `kill-session -t _portal-saver` closes the session's PTY; the kernel delivers SIGHUP; the daemon's signal handler (Phase 2 task 2-7 + 2-12) flushes final state; daemon exits; tmux auto-destroys the (now unpopulated) session. A failure from `KillSession` for any reason OTHER than "session-absent" (which we already short-circuited) is logged and contributes to a non-zero overall exit code, but the cleanup command still proceeds to the hook-removal step. Partial failures never short-circuit — spec: "cleanup never aborts partway to leave mixed state."

**Outcome**: `portal state cleanup` on a server with `_portal-saver` running kills the saver session first (daemon flushes final state), then removes Portal hooks, then (task 6-7) optionally purges. If any step fails, the remaining steps still run; the exit code is non-zero; each failure is logged to `portal.log`. A user running `portal state cleanup` twice back-to-back: first invocation kills + removes; second invocation is a clean no-op (zero failures, exit 0).

**Do**:
- Edit `cmd/state_cleanup.go`:
  - Extend `CleanupDeps` / deps struct (or introduce if not yet present) with a `SaverKiller` interface:
    ```go
    type SaverKiller interface {
        HasSession(name string) (bool, error)
        KillSession(name string) error
    }
    ```
    `*tmux.Client` already implements both methods per existing `internal/tmux/tmux.go`.
  - Refactor `RunE` body into ordered action list with per-action error accumulation:
    ```go
    RunE: func(cmd *cobra.Command, args []string) error {
        logger, closer := openNoRotateLogger()
        defer closer.Close()
        client := buildCleanupClient()
        var errs []error
        // Step 1: kill _portal-saver
        if err := killSaver(client, logger); err != nil {
            errs = append(errs, fmt.Errorf("daemon kill: %w", err))
        }
        // Step 2: hook removal
        if client.ServerRunning() {
            if err := tmux.UnregisterPortalHooks(client); err != nil {
                errs = append(errs, fmt.Errorf("hook removal: %w", err))
            }
        }
        // Step 3: --purge (task 6-7 lands this)
        // ...
        return errors.Join(errs...)
    },
    ```
  - Implement `killSaver(c SaverKiller, logger *state.Logger) error`:
    1. `exists, err := c.HasSession("_portal-saver")` — if err, log Warn + return wrapped err.
    2. If `!exists`, return nil (idempotent: "absent session is not an error").
    3. `if err := c.KillSession("_portal-saver"); err != nil { ... }`.
       - Some tmux errors are "session already gone" (race with auto-destroy). Check `strings.Contains(err.Error(), "can't find session")` or tmux's actual output — treat as success (idempotent). Otherwise log + return.
    4. Log Info at `ComponentDaemon`: "killed _portal-saver; daemon will flush final state on SIGHUP".
- Ordering: `killSaver` MUST run before `UnregisterPortalHooks` so the daemon's final flush (Phase 2) captures the pre-cleanup world. Document this ordering in a comment block above `RunE`.
- Partial-failure semantics: collect all errors into `errs`; return `errors.Join(errs...)` at the end. Cobra surfaces a joined error as exit 1 with the joined message. Set `SilenceErrors = true` so Cobra does not re-print; we print our own error via logger + the command's own stderr if needed.
- Cobra concern: `errors.Join` with 3 errors prints a multi-line error message. For user-facing cleanup, that's acceptable — the user wants to see every failure. Alternative: print each error on its own line in `stderr` and return a sentinel that conveys only the non-zero exit. Prefer the simpler joined-error route.
- Tests in `cmd/state_cleanup_test.go` (extending Phase 1 task 1-9's suite):
  - `"it kills _portal-saver before removing hooks"` — ordered-call recorder asserts `HasSession` + `KillSession` called before `UnregisterPortalHooks`.
  - `"it is idempotent when _portal-saver is absent"` — `HasSession` returns false; assert zero `KillSession` calls.
  - `"it tolerates KillSession 'can't find session' as idempotent success"` — `HasSession` true, `KillSession` returns `can't find session` error; assert no error bubbled.
  - `"it contributes to non-zero exit when KillSession fails for another reason"` — `HasSession` true, `KillSession` returns permission error; assert joined error contains "daemon kill".
  - `"it still attempts UnregisterPortalHooks even when KillSession fails"` — regression guard for partial-failure semantics.
  - `"it logs the kill to portal.log at INFO level"` — inject logger, assert a `ComponentDaemon` + INFO line.
  - `"the cleanup command is still exempt from bootstrap"` — Phase 1 task 1-9 test preserved.
  - `"a second invocation back-to-back is a clean no-op"` — first call kills + removes; second call observes `HasSession=false`, returns zero errors.

**Acceptance Criteria**:
- [ ] `killSaver(client, logger)` runs BEFORE `UnregisterPortalHooks` in `cmd/state_cleanup.go`'s `RunE`.
- [ ] `killSaver` checks `HasSession("_portal-saver")` first and short-circuits if absent (idempotent).
- [ ] `killSaver` treats tmux's "can't find session" error from `KillSession` as idempotent success.
- [ ] Other `KillSession` errors are logged to `portal.log` at `ComponentDaemon` / WARN and contribute to the joined error returned by `RunE`.
- [ ] `RunE` accumulates errors across all three actions via `errors.Join`; no short-circuit between actions.
- [ ] Second back-to-back invocation is a clean no-op (zero errors, exit 0).
- [ ] The INFO-level log line identifies the kill: `"killed _portal-saver; daemon will flush final state on SIGHUP"` or similar.
- [ ] `@portal-restoring` set during cleanup still causes the daemon to skip its final flush per Phase 2 contract (nothing for this task to do — that's the daemon's business; verified by integration test in 5-8 already).
- [ ] Phase 1 task 1-9's existing acceptance (hook removal alone works when no daemon was running) continues to pass.
- [ ] Cobra's `SilenceErrors` + `SilenceUsage` are true so joined-error output is not doubly-printed.

**Tests**:
- `"it kills _portal-saver before removing hooks"`
- `"it is idempotent when _portal-saver is absent"`
- `"it tolerates KillSession 'can't find session' as idempotent success"`
- `"it contributes to non-zero exit when KillSession fails for another reason"`
- `"it still attempts UnregisterPortalHooks even when KillSession fails"`
- `"it logs the kill to portal.log at INFO level with ComponentDaemon"`
- `"a second invocation back-to-back is a clean no-op"`
- `"@portal-restoring set during cleanup still allows the command to complete"` (no new assertion beyond the existing flow; regression guard)

**Edge Cases**:
- `_portal-saver` absent → `HasSession` returns false → `killSaver` returns nil → no error, no KillSession call.
- `KillSession` returns an error matching "can't find session" / "session not found" (race with tmux auto-destroy): treat as idempotent success. Implementation uses `strings.Contains(err.Error(), "can't find session")` — tmux's exact error wording varies slightly by version but this substring is stable across 3.0+.
- `KillSession` permission error or server-down error: logged + contributes to joined error. `UnregisterPortalHooks` still runs after.
- Running before hook removal: the daemon's SIGHUP handler (Phase 2) flushes state BEFORE the daemon exits. By the time hook removal runs, the daemon has already persisted. If the daemon had crashed before the handler fired, the save file is as of the previous tick — acceptable degradation.
- `@portal-restoring` set during cleanup (e.g., bootstrap is mid-restore and the user concurrently runs cleanup): the daemon's SIGHUP handler checks the flag and skips the flush. No corruption; just lose the in-memory changes since the last tick. Spec documents this trade-off.
- Second back-to-back invocation: `HasSession` false → no kill; `UnregisterPortalHooks` observes no Portal entries → no removals; exit 0.
- Ordering is load-bearing: the reverse order (hooks first, then kill) would mean the final flush happens on a hook-less server; the daemon's captures would still succeed (daemon uses `list-panes`, `show-environment`, `capture-pane` — none depend on hooks), so the ordering is a defensive measure against subtle side-effects rather than a strict correctness requirement. Documented in the code comment; kept in the spec-mandated order for alignment.

**Context**:
> Spec "CLI Surface → `portal state cleanup`":
> "Actions (in order):
> 1. `kill-session -t _portal-saver` to terminate the daemon (SIGHUP → final flush on the way out). Idempotent: absent session is not an error.
> 2. Remove Portal's `set-hook -ga` entries via index-based `set-hook -gu '<EVENT>[N]'` for each event/command pair Portal registers (see tmux Hook Registration Lifecycle for the removal protocol). Already-absent entries are not an error.
> 3. Remove `~/.config/portal/state/` only when explicitly requested via the `--purge` flag."
>
> Spec "CLI Surface → `portal state cleanup` → Exit codes":
> "`0` — all requested actions completed successfully (including idempotent no-ops when nothing needed to be cleaned). non-zero — one or more actions failed (e.g., tmux `set-hook -gu` errored, `kill-session` failed for non-'session-absent' reasons, `--purge` specified but rmdir failed). Partial failures still attempt subsequent actions — `cleanup` never aborts partway to leave mixed state — but the exit code reflects that at least one action did not succeed. Failures are also logged to `portal.log`."
>
> Spec "Save-Side Architecture → Execution Model → Lifecycle Summary":
> "Creation: `EnsureServer()` calls `has-session -t _portal-saver`. If absent, `new-session -d -s _portal-saver 'portal state daemon'`." — kill + recreate is the canonical lifecycle op; `kill-session` is the teardown primitive.
>
> Spec "Save-Side Architecture → Signal Handling → Handler behavior":
> "1. If the `@portal-restoring` marker is set, skip the final flush (an in-progress restore is underway; capturing now would capture mid-transition state). 2. Otherwise, flush the current state atomically via `AtomicWrite` and exit."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "CLI Surface → `portal state cleanup`", "Save-Side Architecture → Signal Handling", "Save-Side Architecture → Execution Model → Lifecycle Summary".

## built-in-session-resurrection-6-7 | approved

### Task 6-7: Add `--purge` flag to `portal state cleanup` for state-directory removal

**Problem**: Phase 1 task 1-1 declared `--purge` as a parseable boolean on `stateCleanupCmd` with no body; Phase 1 task 1-9 left it as a stub. The spec's action (3) requires actually removing `~/.config/portal/state/` when `--purge` is passed: all state files (`sessions.json`, `save.requested`, `daemon.pid`, `daemon.version`, `portal.log`, `portal.log.old`, scrollback files, FIFOs) and then the state directory itself. Order within the overall cleanup: `--purge` runs AFTER daemon kill (task 6-6) AND after hook removal — purging while the daemon is alive would race the daemon's writes; purging while hooks point at a missing binary is benign (defensive `command -v portal` guard handles it) but staged after hook removal for tidiness. Defensive consideration: refuse to remove paths outside the resolved state directory — a symlink shenanigan where `~/.config/portal/state/` itself is a symlink to `/` would otherwise be catastrophic.

**Solution**: Add a third action `purgeStateDir(dir string, logger *state.Logger) error` that runs `os.RemoveAll(dir)` *only after* verifying the path resolves cleanly and matches the expected shape. Hook it into `cmd/state_cleanup.go`'s `RunE` after the hook-removal step, conditional on the `--purge` flag. Missing directory is not an error (idempotent). Per-file removal failures during the walk are logged and aggregated into the joined error — `os.RemoveAll` handles this internally for the bulk case, but for finer-grained logging we can opt for a two-phase approach (walk + log failures + then `RemoveAll` the dir itself), or rely on `RemoveAll`'s behaviour and log the final error only. Choose the simpler path: call `os.RemoveAll(dir)` once; on error, log and contribute to the joined error. The symlink defensive check uses `filepath.EvalSymlinks` and asserts the resolved path matches the expected base (`~/.config/portal/state` or the env-var override).

**Outcome**: `portal state cleanup --purge` on a fresh install with 20MB of state: kills daemon, removes hooks, deletes the entire `state/` directory, exits 0. `portal state cleanup` without `--purge`: state directory left intact — reinstalling picks up where the user left off. `portal state cleanup --purge` on a state directory that's already gone: exit 0 (idempotent). `portal state cleanup --purge` on a state directory where `portal.log` is held open by a running daemon: task 6-6 already killed the daemon, so all FDs are released by the time purge runs.

**Do**:
- Edit `cmd/state_cleanup.go`:
  - Read the `--purge` flag via `cmd.Flags().GetBool("purge")` at the top of `RunE`.
  - After the hook-removal step, add:
    ```go
    if purge {
        if err := purgeStateDir(dir, logger); err != nil {
            errs = append(errs, fmt.Errorf("purge state dir: %w", err))
        }
    }
    ```
    where `dir` was resolved at the top of `RunE` via `state.ResolveDir()`.
  - Implement `purgeStateDir(dir string, logger *state.Logger) error`:
    1. `info, err := os.Lstat(dir)` — if `ENOENT`, return nil (idempotent).
    2. If `info.Mode()&os.ModeSymlink != 0`, return an error: `"refusing to purge symlinked state dir: %s"` with a `logger.Warn` line. The caller can `readlink` the path and `rm -rf` the target manually if intentional.
    3. (Removed: the prior `EvalSymlinks` strict-equality check is dropped — intermediate symlinks in the path resolution are tolerated; only the leaf being a symlink triggers refusal. This avoids a false-positive on macOS legacy installs whose intermediate paths route through other symlinked directories.)
    4. `if err := os.RemoveAll(dir); err != nil { logger.Error(state.ComponentDaemon, "purge failed: %v", err); return err }`.
    5. Log Info: `"purged state directory %s"`.
- Task 6-6 sets up the ordering infrastructure (joined errors, logger); this task adds step 3 in the existing error-accumulation harness.
- Tests in `cmd/state_cleanup_test.go`:
  - `"it does not touch the state dir when --purge is absent"` — pre-populate state dir with files; run cleanup; assert dir still exists with files.
  - `"it removes the state dir when --purge is passed"` — pre-populate; run `--purge`; assert `os.Stat` returns `ENOENT`.
  - `"it is idempotent when --purge is passed on a missing state dir"` — dir absent; run `--purge`; assert no error.
  - `"it logs the purge to portal.log at INFO level"`.
  - `"it refuses to purge a symlinked state dir"` — create symlink as state dir; run `--purge`; assert error contains "refusing to purge symlinked".
  - `"it refuses to purge when the state dir resolves outside itself via symlinks nested inside"` — actually `EvalSymlinks(dir)` resolves the full path; if `dir` itself is not a symlink but a component is, `EvalSymlinks` returns the resolved form. The check `resolved != dir` catches both cases. Test this by constructing a state dir whose canonical form differs from the given path.
  - `"--purge runs after daemon kill and hook removal"` — ordered-call recorder asserts kill → hooks → purge order.
  - `"--purge contributes to non-zero exit when os.RemoveAll fails"` — chmod a file inside state dir to `0000`; run `--purge`; assert error surfaced.
  - `"purge removes FIFOs and .bin files alongside regular files"` — populate with a FIFO + `.bin`; verify cleared after purge.
  - `"--purge absent keeps state dir even when cleanup's other steps ran"`.

**Acceptance Criteria**:
- [ ] `cmd/state_cleanup.go` reads the `--purge` flag via `cmd.Flags().GetBool("purge")`.
- [ ] When `--purge` is absent, the state directory is untouched after cleanup completes.
- [ ] When `--purge` is present, `purgeStateDir(dir, logger)` runs AFTER `killSaver` and AFTER `UnregisterPortalHooks`.
- [ ] `purgeStateDir` on a missing directory returns nil (idempotent no-op).
- [ ] `purgeStateDir` refuses to remove paths where `state/` itself is a symlink (defensive — only the leaf check, not the resolution chain).
- [ ] Intermediate symlinks in the resolved path (e.g., `~/.config` is a symlink) are tolerated; `--purge` succeeds.
- [ ] On `os.RemoveAll` failure, the error is logged to `portal.log` at ERROR level and contributes to the joined exit error; subsequent actions (none in v1; this is the last step) are not affected.
- [ ] FIFO files, `.bin` scrollback files, and all other contents are swept (verified by fixture test).
- [ ] Command prints a single info-level log line on successful purge.
- [ ] `--purge` is documented in the command's `Short`/`Long` help text.

**Tests**:
- `"it does not touch the state dir when --purge is absent"`
- `"it removes the state dir when --purge is passed"`
- `"it is idempotent when --purge is passed on a missing state dir"`
- `"it logs the purge to portal.log at INFO level"`
- `"it refuses to purge a symlinked state dir"`
- `"--purge succeeds when an intermediate path component is a symlink"`
- `"--purge runs after daemon kill and hook removal"`
- `"--purge contributes to non-zero exit when os.RemoveAll fails"`
- `"purge removes FIFOs and .bin files alongside regular files"`
- `"--purge absent keeps state dir intact even when the other cleanup steps ran"`

**Edge Cases**:
- State dir missing → `os.Lstat` returns `ENOENT` → `purgeStateDir` returns nil. No error. Idempotent.
- State dir is itself a symlink (user redirected `state/` to elsewhere): refuse to purge — log the refusal at WARN level and contribute a non-zero exit. User must manually clean up.
- State dir contains a symlink (e.g., `portal.log` → `/var/log/something`): `os.RemoveAll` unlinks the symlink, not the target. Safe. `filepath.EvalSymlinks` only resolves the outer path; nested symlinks within `state/` are handled by `os.RemoveAll`'s own semantics.
- State dir's canonical resolution differs from the given path (e.g., `~/.config/portal` is a symlink to `~/Library/Application Support/portal`): the check uses `os.Lstat` on the leaf path only — only the final component being a symlink triggers refusal. Intermediate symlinks (the user's `~/.config` is a symlink, or `~/Library` resolves through one) are tolerated because the final `state/` directory is what `os.RemoveAll` operates on. Concrete implementation: `if info.Mode()&os.ModeSymlink != 0 { refuse }` — only the leaf symlink is rejected, not the resolution chain. This avoids the false-positive on macOS legacy installs whose intermediate paths route through other symlinked directories.
- Users with `state/` itself as a symlink (deliberately redirected to e.g. an external drive): receive the refusal with a clear log line: `refusing to purge symlinked state dir <path>: remove it manually if intentional`. This is a deliberate friction, not a bug — purging through an opaque symlink could nuke unrelated data.
- FIFO files within state dir: `os.RemoveAll` handles FIFOs on both Linux and macOS. No special case needed.
- `portal.log` held open by the daemon: task 6-6 killed the daemon, so the FD is closed by the time purge runs. Sub-millisecond race window between SIGHUP and fd close is tolerated — if the fd is still open, `os.RemoveAll` unlinks the directory entry but the file's blocks stay allocated until the daemon closes; the user sees the dir gone and doesn't care.
- Per-file removal failures during `RemoveAll`: `os.RemoveAll` returns the first error encountered; by the time it returns, as much as could be removed IS removed. Partial cleanup + error reported. User can retry.
- `--purge` on Phase-1-era state dir (only `projects.json`, `aliases`, `hooks.json` exist; no `state/` subdirectory yet): `ResolveDir()` returns `~/.config/portal/state/` which doesn't exist; `os.Lstat` returns `ENOENT`; idempotent no-op. Does NOT delete `~/.config/portal/` itself.
- User running `--purge` on a post-v1 install with `state/scrollback/` present: full recursive removal works; no special handling.

**Context**:
> Spec "CLI Surface → `portal state cleanup`":
> "Actions (in order):
> 1. `kill-session -t _portal-saver` ...
> 2. Remove Portal's `set-hook -ga` entries ...
> 3. Remove `~/.config/portal/state/` only when explicitly requested via the `--purge` flag. Default behaviour leaves the state directory intact so re-installing Portal picks up where the user left off."
>
> Spec "CLI Surface → `portal state cleanup` → Exit codes":
> "non-zero — one or more actions failed (e.g., ... `--purge` specified but rmdir failed). Partial failures still attempt subsequent actions — `cleanup` never aborts partway to leave mixed state — but the exit code reflects that at least one action did not succeed."
>
> Spec "Scope & Constraints → Storage Location": state is under `~/.config/portal/state/` resolved via Portal's existing `configFilePath` mechanism. The resolved path is what `--purge` targets. Env-var overrides (`PORTAL_STATE_DIR`, if defined in Phase 2) are respected automatically because `state.ResolveDir()` honours them.
>
> Spec "Observability & Diagnostics → What gets logged":
> "Save failures (disk full, write errors, permission issues). ... " — `--purge` failures are a category of "write errors" and log to `portal.log` at ERROR.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "CLI Surface → `portal state cleanup`", "Save Format & Schema → Storage Location".

## built-in-session-resurrection-6-8 | approved

### Task 6-8: Emit single-line stderr + non-zero exit for fatal bootstrap errors

**Problem**: The spec enumerates four fatal bootstrap failure modes that must terminate Portal with a single-line stderr message and non-zero exit:
1. `tmux -V` fail (version < 3.0 or binary absent) — Phase 1 task 1-2 already produces the copy; the `PersistentPreRunE` wiring surfaces it.
2. `EnsureServer()` fail — tmux server cannot start.
3. `set-hook -ga` calls fail en masse (all events errored).
4. `@portal-restoring` set-option fail.

Task 5-2's orchestrator already produces errors for each; task 5-3 propagates them via `PersistentPreRunE`. But today's error-propagation is plain Cobra: the error's `Error()` string gets printed (often with "Error: " prefix and usage block), which violates the spec's "single line, no banners, no colors" requirement. This task enforces the single-line + non-zero-exit contract: Cobra's `SilenceErrors = true` + `SilenceUsage = true` at the root; the `main.go` / `cmd.Execute()` entry point is the single place that writes to stderr on a `PersistentPreRunE` failure. Each fatal also logs to `portal.log` at ERROR (task 6-2 wired this; this task ensures the log-write happens even when stderr is the primary signal).

**Solution**: Introduce a `bootstrap.FatalError` sentinel type (in `cmd/bootstrap/errors.go`) that wraps the underlying cause and carries a user-facing single-line message. The orchestrator (task 5-2) returns `*FatalError` for each of the four fatal conditions. `PersistentPreRunE` (task 5-3) propagates it as-is. At the top-level entry point (`main.go` or `cmd.Execute()`), a single error-handler checks `errors.As(&fatalErr)` and emits `fatalErr.UserMessage` to stderr on a single line, then exits non-zero. Cobra's `SilenceErrors=true` + `SilenceUsage=true` on `rootCmd` suppress Cobra's own error/usage output. Every fatal also logs ERROR to `portal.log` at the point of detection (task 6-2's migration already logs; this task confirms the log-write path fires before the stderr path).

**Outcome**: A user on tmux 2.9 runs `portal open`: stderr contains exactly `"Portal requires tmux ≥ 3.0 (found 2.9). Please upgrade."` followed by a newline, exit code is non-zero, `portal.log` (if it exists) has a matching ERROR entry. A user whose tmux server cannot start sees `"Portal failed to start tmux server: <underlying error>"`. Mass hook-registration failure: `"Portal failed to register tmux hooks: <joined underlying errors>"`. `@portal-restoring` set-option failure: `"Portal failed to set @portal-restoring marker: <underlying error>"`. All four one-liners, no banners, no colors, non-zero exit. TUI path: `PersistentPreRunE` returns before `openTUI` is called, so no TUI ever launches — no loading page to tear down.

**Do**:
- Create `cmd/bootstrap/errors.go`:
  ```go
  package bootstrap

  // FatalError is a sentinel for bootstrap failures that must terminate Portal
  // with a single-line stderr message and non-zero exit per spec
  // "Observability & Diagnostics → Fatal Bootstrap Errors".
  type FatalError struct {
      UserMessage string // single-line, no newlines
      Cause       error  // underlying cause for logging
  }

  func (e *FatalError) Error() string { return e.UserMessage }
  func (e *FatalError) Unwrap() error { return e.Cause }

  // NewFatal constructs a FatalError with the given user-facing single-line message.
  func NewFatal(userMsg string, cause error) *FatalError {
      return &FatalError{UserMessage: userMsg, Cause: cause}
  }
  ```
- Edit `cmd/bootstrap/bootstrap.go` (task 5-2's `Run`):
  - Step 1 `EnsureServer` failure: wrap as `NewFatal("Portal failed to start tmux server: " + err.Error(), err)`.
  - Step 2 `RegisterPortalHooks` failure: detect "mass" failure condition — `HookRegistrar.RegisterPortalHooks` returns a `joined-errors-for-all-events` error when every event's registration failed; only this case is fatal. (If a subset errored, log WARN and continue — implementation concern inside `RegisterPortalHooks`, not here.) On mass failure: wrap as `NewFatal("Portal failed to register tmux hooks: " + err.Error(), err)`.
  - Step 3 `Restoring.Set` failure: wrap as `NewFatal("Portal failed to set @portal-restoring marker: " + err.Error(), err)`.
  - Step 6 `Restoring.Clear` failure: NOT fatal — log WARN via ComponentBootstrap and continue. Spec's "Fatal Bootstrap Errors" list names `@portal-restoring set-option fails` for the SET only; clear failure is soft (matches task 3-7's clear-failure-is-soft contract and task 5-2's step-6 handling). The marker stays set; the daemon will skip ticks until next server restart (volatile server-option self-heals).
  - At each wrap site, also log ERROR to `portal.log` via `logger.Error(state.ComponentBootstrap, "<single-line>: %v", err)` BEFORE returning — ensures log entry exists even if stderr write later fails.
- Edit `cmd/root.go` (or `main.go` — wherever `cmd.Execute()` lives):
  - Set `rootCmd.SilenceErrors = true` and `rootCmd.SilenceUsage = true` at declaration.
  - In `main.go` (or `Execute()`):
    ```go
    if err := cmd.Execute(); err != nil {
        var fatal *bootstrap.FatalError
        if errors.As(err, &fatal) {
            fmt.Fprintln(os.Stderr, fatal.UserMessage)
            os.Exit(1)
        }
        // Non-fatal error: Cobra already swallowed printing due to SilenceErrors;
        // emit minimal diagnostic.
        fmt.Fprintln(os.Stderr, err.Error())
        os.Exit(1)
    }
    ```
  - For Cobra usage errors (invalid args, missing required flag): preserve existing Cobra behaviour via conditional unwrap — if the error is a Cobra usage error (not a `*FatalError`), print usage and exit. Implementation: check `strings.HasPrefix(err.Error(), "unknown command") || strings.HasPrefix(err.Error(), "required flag")` etc., or rely on Cobra's own classification. Simpler: let Cobra's normal path handle usage errors by NOT setting `SilenceErrors = true` globally; instead, set it only on commands whose errors are already fully rendered (status, cleanup). For bootstrap: the FatalError path at the `Execute()` handler takes priority via `errors.As` check.
    - Recommended: keep `SilenceErrors = false` (Cobra default) but override in the `main.go` handler. The `errors.As` check intercepts `FatalError` first and emits the single line; all other errors fall through to Cobra's default path. This preserves usage-error UX (Cobra prints usage + "Error: ...").
- Tests in `cmd/bootstrap/bootstrap_test.go` (extending task 5-2's suite):
  - `"EnsureServer failure returns a FatalError with the specified user message"`.
  - `"RegisterPortalHooks mass failure returns a FatalError"`.
  - `"Restoring.Set failure returns a FatalError"`.
  - `"Restoring.Clear failure does NOT return a FatalError (soft path)"`.
  - `"FatalError wraps the underlying cause for unwrap"`.
  - `"EnsureSaver failure does NOT return a FatalError (soft path — produces a Warning instead)"` — regression guard; `EnsureSaver` is a soft failure per spec, produces a `bootstrap.Warning` via the accumulator pattern (task 6-9), not `FatalError`.
- Tests in `cmd/root_test.go` / `main_test.go`:
  - `"FatalError from PersistentPreRunE produces stderr single-line and non-zero exit"` — mock orchestrator returns `NewFatal("test msg", causeErr)`; run `Execute()`; assert stderr == `"test msg\n"` and exit code is non-zero. (Integration-style: build the binary, run via `exec.Command`.)
  - `"Cobra usage error still prints Cobra's default usage block"` — run `portal nonexistent-command`; assert stderr contains "unknown command" (Cobra's standard output).
  - `"every fatal also logs ERROR to portal.log"` — inject logger, assert ERROR line with `ComponentBootstrap` precedes the stderr write.
  - `"tmux 2.9 fatal produces exact single-line copy from Phase 1 task 1-2"` — error text assertion pinned to the spec wording.
- Acceptance requires that the four fatal copy strings are **exact**:
  - tmux version: `"Portal requires tmux ≥ 3.0 (found <V>). Please upgrade."` (Phase 1 task 1-2 already produces this verbatim; FatalError preserves it).
  - EnsureServer: `"Portal failed to start tmux server: <underlying>"`.
  - Mass hook-register: `"Portal failed to register tmux hooks: <underlying>"`.
  - `@portal-restoring` set: `"Portal failed to set @portal-restoring marker: <underlying>"`.
  - (Note: `@portal-restoring` CLEAR failure is NOT fatal — soft path per task 5-2 step 6 / task 3-7. Logs WARN and continues.)

**Acceptance Criteria**:
- [ ] `cmd/bootstrap/errors.go` defines `FatalError` with `UserMessage` + `Cause` + `Error()` + `Unwrap()`.
- [ ] Orchestrator returns `*FatalError` for: step 1 (EnsureServer), step 2 mass-register, step 3 (Set @portal-restoring). Step 6 (Clear @portal-restoring) failure is soft — logs WARN and continues.
- [ ] `EnsureSaver` failure does NOT return `FatalError` — produces a soft `bootstrap.Warning` via the accumulator pattern (task 6-9).
- [ ] Every fatal wrap site also logs ERROR to `portal.log` with `ComponentBootstrap` BEFORE returning.
- [ ] `main.go` / `Execute()` intercepts `*FatalError` via `errors.As` and emits `UserMessage` on a single stderr line, followed by `os.Exit(1)`.
- [ ] Stderr output is exactly one line (no banners, no colors, no usage block).
- [ ] Cobra usage errors (unknown command, missing required flag) still print Cobra's default output.
- [ ] The four fatal user-message copies match the spec verbatim.
- [ ] TUI path: on fatal, `openTUI` is never called (PersistentPreRunE returns first); no loading page to tear down.
- [ ] Exit code is distinguishable from Cobra usage errors via the absence of the usage-block stderr signal.

**Tests**:
- `"EnsureServer failure returns a FatalError with the specified user message"`
- `"RegisterPortalHooks mass failure returns a FatalError"`
- `"Restoring.Set failure returns a FatalError"`
- `"Restoring.Clear failure does NOT return a FatalError (soft path)"`
- `"EnsureSaver failure does NOT return a FatalError (soft path — produces a Warning instead)"`
- `"FatalError wraps the underlying cause via Unwrap"`
- `"Execute() emits FatalError.UserMessage on a single stderr line and exits non-zero"`
- `"Cobra usage error still prints usage block (unknown command)"`
- `"tmux 2.9 fatal produces exact single-line copy"`
- `"every fatal also logs ERROR to portal.log with ComponentBootstrap"`
- `"stderr output contains no banners, colors, or ANSI codes on fatal"`

**Edge Cases**:
- `tmux -V` fail: Phase 1 task 1-2 surfaces the error from `CheckTmuxVersion`; `PersistentPreRunE`'s version-guard-wrapping (Phase 1 task 1-3) returns it directly. This task wraps the returned error as `FatalError` at the top of `PersistentPreRunE` if it's not already one. Implementation: in `cmd/root.go`, after `CheckTmuxAvailable()` / `CheckTmuxVersion()`, if err != nil: `return bootstrap.NewFatal(err.Error(), err)`.
- `EnsureServer` fail: permission error starting tmux, socket-dir unwritable, etc. Wrapped as FatalError at step 1.
- Hook registration: partial failure (some events OK, some failed) is NOT fatal — log WARN per failed event, continue. Mass failure (every event failed — detectable by checking all-or-none) IS fatal. Implementation: `RegisterPortalHooks` returns a typed error indicating "all events failed"; the orchestrator promotes only that specific shape to FatalError.
- `@portal-restoring` set-option failure: rare (only fires if tmux server died between steps 2 and 3). Fatal per spec; cannot continue because the daemon's restore-guard depends on this marker.
- `@portal-restoring` clear-option failure: rare but critical; leaving the marker set means every subsequent daemon tick skips, so saves pause forever. Fatal.
- `EnsureSaver` is soft (task 6-9) — the user can still use Portal without the save daemon; sessions just don't get captured.
- Multiple fatals in the same invocation: impossible by construction — the orchestrator short-circuits on the first fatal.
- `errors.As` correctness: `FatalError` is a pointer receiver type; `errors.As(err, &fatal)` where `fatal` is `*bootstrap.FatalError` matches. Verified by test.
- Stderr output is unbuffered (`os.Stderr` is line-buffered by default on Go; `Fprintln` flushes on newline). No explicit flush needed.
- Exit code choice: use `os.Exit(1)` for all fatals. Cobra usage errors exit via Cobra's own path (usually also 1). Distinguishability is via the content of stderr, not the exit code itself. Spec says "distinguishable from Cobra usage errors" — achieved via the single-line-no-usage-block signal.

**Context**:
> Spec "Observability & Diagnostics → Fatal Bootstrap Errors":
> "'Soft' bootstrap failures (corrupt `sessions.json`, one session fails to restore, `_portal-saver` creation fails after retry) degrade locally and continue. **Fatal** failures — the underlying machinery can't even start — are handled differently:
> - **`tmux -V` check fails** (version < 3.0 or `tmux` binary absent): Portal emits the user-facing error immediately to stderr, exits non-zero, does not enter the TUI.
> - **`EnsureServer()` fails** (tmux server cannot start, e.g., permission error): emit stderr error, exit non-zero.
> - **`set-hook -ga` calls fail** en masse (version check passed but hook calls error anyway): log, emit one-line stderr warning if on CLI path; on TUI path, dismiss loading page cleanly, emit error, exit non-zero.
> - **`@portal-restoring` set-option fails**: same as `set-hook` failure.
> TUI path: loading page never 'hangs forever.' Any unrecoverable error tears down the Bubble Tea program cleanly, emits the error, exits. The loading page is only kept up while bootstrap is making progress."
>
> Spec "Observability & Diagnostics → Proactive Health Signals → Exception":
> "Portal emits a single line to stderr when a critical problem is detected at bootstrap: ... One line. No banners, no colors, no interactive UI. Just enough signal that the user knows to investigate if they care, and quiet enough not to intrude on normal use."
>
> Spec "Scope & Constraints → Minimum Versions → Runtime version check":
> "errors out with a clear user-facing message if the version is below 3.0 (e.g., `'Portal requires tmux ≥ 3.0 (found 2.9). Please upgrade.'`)."
>
> Phase 5 task 5-2 `Orchestrator.Run` returns errors at each step; this task types them as `*FatalError` where the spec mandates fatal semantics. Phase 5 task 5-3 `PersistentPreRunE` propagates; this task ensures the propagation path preserves the typed sentinel.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Observability & Diagnostics → Fatal Bootstrap Errors", "Observability & Diagnostics → Proactive Health Signals", "Scope & Constraints → Minimum Versions → Runtime version check".

## built-in-session-resurrection-6-9 | approved

### Task 6-9: Emit single-line stderr warnings for soft bootstrap failures (CLI path direct write)

**Problem**: The spec defines two soft bootstrap warnings that must surface as single-line stderr one-liners without terminating Portal:
1. **Corrupt `sessions.json`**: restoration is skipped, bootstrap continues, user sees empty state.
   - Copy: `"Portal state file is corrupt — restoration skipped."` + second line: `"Check \`portal state status\` or ~/.config/portal/state/portal.log."`
2. **`_portal-saver` failed to start after retries**: the save daemon didn't come up, no captures will happen, bootstrap continues.
   - Copy: `"Portal save daemon failed to start — sessions won't be captured."` + second line: `"Run \`portal state status\` for details."`

Today's orchestrator (task 5-2) already treats these as soft — returning `*SaverDownError` for #2 and swallowing corrupt-JSON inside `Restore()` for #1. The CLI path needs to write those warning lines to stderr directly; the TUI path (task 6-10) buffers them and emits after the loading page dismisses. This task handles the CLI half: detect the soft-warning sentinel on return from `PersistentPreRunE`, write to stderr, continue execution into the command's `RunE`. Both warnings also log WARN to `portal.log` at the point of detection — task 6-2 wired the log side.

**Solution**: Introduce a `bootstrap.Warning` sentinel type that carries a single-line stderr message plus any follow-up lines. The orchestrator collects warnings during `Run` into a `[]Warning` slice returned alongside the error. `PersistentPreRunE` inspects the slice: on the CLI path (TUI not entered), write each warning's lines to stderr immediately. On the TUI path (task 6-10), buffer them in a shared sink for emission post-loading-page dismissal. A shared `cmd.BootstrapWarningsSink` (in-memory slice guarded by a mutex) is the canonical carrier. The orchestrator appends to the sink; `PersistentPreRunE` + `openTUI` both read from it.

**Outcome**: A user with corrupt `sessions.json` runs `portal list`: stderr shows:
```
Portal state file is corrupt — restoration skipped.
Check `portal state status` or ~/.config/portal/state/portal.log.
```
then `portal list` outputs the (empty) session list and exits 0. The daemon-down case shows:
```
Portal save daemon failed to start — sessions won't be captured.
Run `portal state status` for details.
```
same continue-to-RunE behaviour. Both warnings also appear in `portal.log` at WARN level (task 6-2's log-side wiring already fires at the point of detection inside `Restore()` / `EnsureSaver`; this task confirms the log-write precedes the stderr-write).

**Do**:
- Extend `cmd/bootstrap/errors.go`:
  ```go
  // Warning is a soft bootstrap failure that must not terminate Portal.
  // Lines are emitted in order to stderr by the CLI path; the TUI path
  // buffers and flushes post-dismissal.
  type Warning struct {
      Lines []string
  }

  // Canonical constructors for the two v1 soft warnings.
  func CorruptSessionsJSONWarning() Warning {
      return Warning{Lines: []string{
          "Portal state file is corrupt — restoration skipped.",
          "Check `portal state status` or ~/.config/portal/state/portal.log.",
      }}
  }

  func SaverDownWarning() Warning {
      return Warning{Lines: []string{
          "Portal save daemon failed to start — sessions won't be captured.",
          "Run `portal state status` for details.",
      }}
  }
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
- Create `cmd/bootstrap_warnings.go`:
  ```go
  package cmd

  import (
      "io"
      "sync"
      "github.com/leeovery/portal/cmd/bootstrap"
  )

  // BootstrapWarningsSink accumulates soft warnings across bootstrap.
  // CLI path emits to stderr immediately; TUI path buffers.
  type BootstrapWarningsSink struct {
      mu       sync.Mutex
      warnings []bootstrap.Warning
  }

  func (s *BootstrapWarningsSink) Add(w bootstrap.Warning) {
      s.mu.Lock()
      defer s.mu.Unlock()
      s.warnings = append(s.warnings, w)
  }

  func (s *BootstrapWarningsSink) Drain() []bootstrap.Warning {
      s.mu.Lock()
      defer s.mu.Unlock()
      out := s.warnings
      s.warnings = nil
      return out
  }

  func (s *BootstrapWarningsSink) EmitTo(w io.Writer) {
      for _, warn := range s.Drain() {
          for _, line := range warn.Lines {
              fmt.Fprintln(w, line)
          }
      }
  }

  var bootstrapWarnings = &BootstrapWarningsSink{}
  ```
- Edit `cmd/root.go` `PersistentPreRunE` (task 5-3):
  - After `orchestrator.Run` returns warnings: `for _, w := range warnings { bootstrapWarnings.Add(w) }`.
  - After warnings are accumulated, detect whether we're on the CLI path or TUI path: the TUI path is only entered via `cmd/open.go`'s `openTUI` (no path argument). For every other command (CLI path), call `bootstrapWarnings.EmitTo(cmd.ErrOrStderr())` BEFORE returning from `PersistentPreRunE`. This ensures warnings precede the command's own output.
  - How to detect TUI vs CLI path: check the invoked command name. If `cmd.Name() == "open"` AND no path argument was provided, defer to `openTUI` (task 6-10). Otherwise, emit immediately. Simpler implementation: always drain at the start of `RunE` — but that requires modifying every `RunE`. Chosen approach: emit from `PersistentPreRunE` for non-`open` commands; for `open`, emit only if a path argument is present (CLI shape); else leave buffered for the TUI (task 6-10 drains).
    - Even simpler: emit unconditionally from `PersistentPreRunE` for all commands except `open` without args. `cmd/open.go`'s `RunE` checks `args` and delegates to `openTUI` only when `len(args) == 0` — same check can gate the emission.
  - Cleanest solution: pass the sink as an argument to `openTUI` via context or the existing `openDeps` seam. `openTUI` drains the sink AFTER the loading page dismisses (task 6-10 implements this). `PersistentPreRunE` emits for every other command.
- Tests in `cmd/root_test.go` / `cmd/bootstrap/bootstrap_test.go`:
  - `"corrupt sessions.json produces CorruptSessionsJSONWarning with exact spec copy"`.
  - `"EnsureSaver persistent failure produces SaverDownWarning with exact spec copy"`.
  - `"orchestrator's Warnings slice accumulates multiple soft failures"`.
  - `"PersistentPreRunE emits warnings to stderr on the CLI path"` — non-TUI command; assert stderr contains both warning lines.
  - `"PersistentPreRunE emits multiple warnings each on their own lines in order"`.
  - `"PersistentPreRunE does NOT emit warnings to stderr when the command is portal open with no args (TUI path)"` — the sink retains the warnings for `openTUI` to drain (task 6-10).
  - `"both warnings also log WARN to portal.log"` — injected logger, assert matching lines with `ComponentBootstrap`.

**Acceptance Criteria**:
- [ ] `cmd/bootstrap/errors.go` defines `Warning` struct with `Lines []string`.
- [ ] `CorruptSessionsJSONWarning()` returns the exact spec copy: `["Portal state file is corrupt — restoration skipped.", "Check `portal state status` or ~/.config/portal/state/portal.log."]`.
- [ ] `SaverDownWarning()` returns the exact spec copy: `["Portal save daemon failed to start — sessions won't be captured.", "Run `portal state status` for details."]`.
- [ ] Orchestrator `Run` signature is `(serverStarted bool, warnings []Warning, err error)`.
- [ ] Orchestrator accumulates `SaverDownWarning` in step 4 on persistent `EnsureSaver` failure.
- [ ] Orchestrator accumulates `CorruptSessionsJSONWarning` in step 5 on `errors.Is(err, restore.ErrCorruptIndex)`.
- [ ] Each accumulated warning also logs WARN to `portal.log` with `ComponentBootstrap` at the point of detection (task 6-2's logger is reused).
- [ ] `cmd/bootstrap_warnings.go` defines `BootstrapWarningsSink` with `Add`, `Drain`, `EmitTo` methods; package-level `bootstrapWarnings` sink is the canonical carrier.
- [ ] `PersistentPreRunE` emits sink contents to stderr for non-TUI commands BEFORE returning.
- [ ] `PersistentPreRunE` does NOT drain the sink when the invoked command is `portal open` with zero args (TUI path); task 6-10 drains from `openTUI`.
- [ ] Each warning's lines are emitted in order, one per line, no banners, no prefixes.
- [ ] Multiple warnings in the same bootstrap produce interleaved stderr output (all first's lines, then all second's lines, etc.).

**Tests**:
- `"corrupt sessions.json produces CorruptSessionsJSONWarning with exact spec copy"`
- `"EnsureSaver persistent failure produces SaverDownWarning with exact spec copy"`
- `"orchestrator's Warnings slice accumulates multiple soft failures"`
- `"PersistentPreRunE emits warnings to stderr on the CLI path"`
- `"PersistentPreRunE emits multiple warnings each on their own lines in order"`
- `"PersistentPreRunE does NOT emit warnings when the command is portal open with no args (TUI path)"`
- `"both warnings also log WARN to portal.log with ComponentBootstrap"`
- `"sink's Drain returns warnings and clears the buffer atomically"`
- `"BootstrapWarningsSink is safe for concurrent Add and Drain"`

**Edge Cases**:
- Zero warnings: `Drain()` returns empty slice; `EmitTo` is a no-op — no stderr noise.
- Multiple warnings in a single bootstrap (e.g., corrupt `sessions.json` + daemon down): both appear, each on their own line, in orchestrator-observation order. Spec: "multiple soft warnings each get their own line".
- CLI vs TUI detection: `PersistentPreRunE` has access to the `*cobra.Command` — inspect `cmd.Name()` and `args`. When `cmd.Name() == "open" && len(args) == 0`, defer emission to `openTUI` (task 6-10). Otherwise emit immediately.
- `EnsureSaver` failure is tracked separately from fatal `EnsureServer` failure: `EnsureSaver` warns and continues (soft); `EnsureServer` fatal-terminates (task 6-8). The orchestrator disambiguates by which step the error surfaces in.
- `CorruptSessionsJSONWarning` is emitted for ANY parse error in `sessions.json` (unparseable JSON, schema mismatch on known-fatal fields). Phase 3 task 3-1's reader defines the error type; this task relies on that existing sentinel.
- Log-side write (task 6-2) fires at the detection point (inside `Restore()` / `EnsureSaver`), BEFORE the orchestrator appends to `Warnings`. Redundant WARN line is harmless; single WARN is preferred. Document: the orchestrator appends the warning AFTER the detection-site logger.Warn has fired, so by the time the warning emits to stderr, `portal.log` already has the corresponding line.
- Soft warning during a FatalError-inducing bootstrap: the orchestrator short-circuits on the fatal, so no soft warnings are emitted on a fatal exit. The fatal's stderr message is the sole surface.
- Buffer is not persisted across Portal invocations — it's per-process state. Each `portal <cmd>` invocation gets fresh bootstrap + fresh sink.

**Context**:
> Spec "Observability & Diagnostics → Proactive Health Signals → Exception":
> "Exception: genuinely broken states detected during `PersistentPreRunE`. Portal emits a single line to stderr when a critical problem is detected at bootstrap:
> - `_portal-saver` cannot be created after retry attempts:
>   ```
>   Portal save daemon failed to start — sessions won't be captured.
>   Run `portal state status` for details.
>   ```
> - `sessions.json` exists but is unparseable:
>   ```
>   Portal state file is corrupt — restoration skipped.
>   Check `portal state status` or ~/.config/portal/state/portal.log.
>   ```
> One line. No banners, no colors, no interactive UI. Just enough signal that the user knows to investigate if they care, and quiet enough not to intrude on normal use."
>
> Spec "Observability & Diagnostics → Proactive Health Signals → TUI interaction":
> "While the Bubble Tea loading page is active, direct stderr writes would corrupt the rendered UI. The TUI path therefore **buffers** bootstrap warnings in memory during the loading window and emits them to stderr *after* the loading page is dismissed (before the TUI picker renders, or immediately before exit on fatal error). The CLI path writes to stderr directly as described. Both paths log the same content to `portal.log` regardless of stderr behaviour."
>
> Spec "Failure Modes & Recovery → Consolidated Failure-Handling Table":
> "`sessions.json` corrupt / unparseable | Log warning, emit one-line stderr warning (see Observability), skip restoration entirely, continue bootstrap."
> "`_portal-saver` creation fails at bootstrap | Portal retries a small number of times. On persistent failure: log, emit stderr warning (see Observability), continue bootstrap without the save daemon."
>
> Note that the spec says "One line" for the warning but the example bodies span two lines each. Interpretation: "one-line warning" means the PRIMARY message is one line; the second line is a follow-up pointer. Implementation emits both lines verbatim per the spec example.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Observability & Diagnostics → Proactive Health Signals", "Failure Modes & Recovery → Consolidated Failure-Handling Table".

## built-in-session-resurrection-6-10 | approved

### Task 6-10: TUI buffers bootstrap warnings during loading window and flushes after page dismissal; tears down loading page cleanly on fatal

**Problem**: Task 6-9 handles the CLI path — stderr emission happens immediately in `PersistentPreRunE`. The TUI path requires different treatment: while the Bubble Tea loading page is rendering, direct stderr writes corrupt the rendered UI (alt-screen mode intermingles with error text). The TUI path must buffer warnings and emit them AFTER the loading page is dismissed (before the TUI picker renders), or immediately before a fatal-induced exit. Additionally, on fatal bootstrap errors that surface AFTER the TUI launches (rare — orchestrator synchronous in `PersistentPreRunE` runs before TUI) AND on panics during the loading window, the TUI program must be torn down cleanly (exit alt-screen, quit Bubble Tea) before the stderr write so the terminal is in a sane state.

**Solution**: The TUI path already has a seam: task 5-7's `BootstrapCompleteMsg` signals the orchestrator finished. Extend `BootstrapCompleteMsg` to carry `Warnings []bootstrap.Warning` (repurposing the empty struct — task 5-7's edge-case note already flagged this). When the TUI's `Update` handler receives `BootstrapCompleteMsg`, it stashes the warnings on the model; when `transitionFromLoading` is called (after both `minElapsed` and `bootstrapComplete` are true), the transition handler sends a second command: `emitWarningsCmd` that returns `tea.Quit`-style cleanup is overkill — instead, queue the stderr emission via a post-transition side effect. The cleanest path: `tea.ExitAltScreen` → side-effect stderr write → `tea.EnterAltScreen` — but re-entering alt-screen is jarring. Simpler: after the TUI's alt-screen is closed on exit (the TUI exits when the user selects a session and transitions to attach), emit any still-buffered warnings at that point. But the spec says "before the TUI picker renders" — i.e., emit the warnings before the user sees the session list, not after they've interacted.

Best approach: use `tea.Sequence(tea.ExitAltScreen, emitCmd, tea.EnterAltScreen)` — atomic sequence that leaves alt-screen, writes to stderr, re-enters alt-screen. This matches the spec's "before the TUI picker renders" intent. The picker renders on re-enter; warnings are on the user's scrollback just before it.

Alternative (simpler, acceptable degradation): emit warnings to stderr only at TUI exit (when the user selects a session via `syscall.Exec` or when `q`/`Esc` quits). Spec says "before the TUI picker renders" which is stricter. Compromise: emit at both points — (a) after loading page dismisses but before picker renders, via the `tea.Sequence` trick, and (b) if that fails (no warnings drained by then), at TUI exit. The sink's `Drain` is idempotent (returns empty on second call), so dual emission is safe.

For fatal during loading: the `PersistentPreRunE` path already handles fatals BEFORE `openTUI` is reached (task 5-2 is synchronous; task 5-3 `bootstrapOnce.Do` completes before `RunE` is called). A fatal error propagates as `PersistentPreRunE` return, Cobra does NOT call `RunE`, the TUI never launches. No teardown needed for that case. The only "fatal during loading" case is a panic inside the TUI goroutine — Bubble Tea's own recover suffices; there's no Portal-specific work.

**Outcome**: A user with corrupt `sessions.json` runs `portal open`: TUI loading page shows for 1.2s, dismisses cleanly (alt-screen exited), stderr shows:
```
Portal state file is corrupt — restoration skipped.
Check `portal state status` or ~/.config/portal/state/portal.log.
```
then TUI picker renders (alt-screen re-entered) with the (empty) session list. User can proceed normally. If no warnings: no alt-screen toggle, no stderr output — transition is seamless.

**Do**:
- Edit `internal/tui/model.go` (extends task 5-7):
  - Change `BootstrapCompleteMsg` from `struct{}` to:
    ```go
    type BootstrapCompleteMsg struct {
        Warnings []bootstrap.Warning
    }
    ```
  - Add field to `Model`: `bufferedWarnings []bootstrap.Warning`.
  - Update `Update`'s `BootstrapCompleteMsg` branch:
    ```go
    case BootstrapCompleteMsg:
        m.bootstrapComplete = true
        m.bufferedWarnings = msg.Warnings
        if m.activePage == PageLoading && m.minElapsed {
            return m, m.transitionFromLoadingCmd()
        }
        return m, nil
    ```
  - Similarly update the `LoadingMinElapsedMsg` branch:
    ```go
    case LoadingMinElapsedMsg:
        m.minElapsed = true
        if m.activePage == PageLoading && m.bootstrapComplete {
            return m, m.transitionFromLoadingCmd()
        }
        return m, nil
    ```
  - Implement `transitionFromLoadingCmd()` — returns a `tea.Cmd`:
    ```go
    func (m *Model) transitionFromLoadingCmd() tea.Cmd {
        m.activePage = PageSessions
        m.sessionsLoaded = true
        m.evaluateDefaultPage()
        if len(m.bufferedWarnings) == 0 {
            return nil
        }
        warnings := m.bufferedWarnings
        m.bufferedWarnings = nil
        return tea.Sequence(
            tea.ExitAltScreen,
            func() tea.Msg {
                for _, w := range warnings {
                    for _, line := range w.Lines {
                        fmt.Fprintln(os.Stderr, line)
                    }
                }
                return nil // no followup message
            },
            tea.EnterAltScreen,
        )
    }
    ```
    Note: `tea.Sequence` is a Bubble Tea primitive that runs commands in order. Check the installed Bubble Tea version; if `tea.Sequence` is unavailable, fall back to `tea.Batch` + sequencing via `tea.Msg` chain.
- Edit `cmd/open.go` `openTUI`:
  - Drain `bootstrapWarnings` (task 6-9's sink) immediately after bootstrap returns (at `openTUI` entry) and pass them to the initial model via the `tea.NewProgram(m)` construction — NOT via the `BootstrapCompleteMsg` path from `Init`. Revised Do-step: drain at entry, pass to model field `pendingBootstrapWarnings`; in `Init`, include them in the `BootstrapCompleteMsg{Warnings: ...}` command that task 5-7 schedules.
  - Revised snippet:
    ```go
    warnings := bootstrapWarnings.Drain()
    m := tui.NewModel(...)
    m.SetPendingBootstrapWarnings(warnings)  // new setter on Model
    p := tea.NewProgram(m, tea.WithAltScreen())
    ```
  - `Model.SetPendingBootstrapWarnings(ws []bootstrap.Warning)` — exposed on `*Model` because `Init` needs access to them; alternatively pass via constructor arg if the TUI's `NewModel` is extensible.
  - `Init` emits `BootstrapCompleteMsg{Warnings: m.pendingBootstrapWarnings}`. Per task 5-7, the message fires from Init's batch.
- Tests in `internal/tui/model_test.go` (extending task 5-7's suite):
  - `"it buffers Warnings from BootstrapCompleteMsg into Model.bufferedWarnings"`.
  - `"transitionFromLoadingCmd returns nil when bufferedWarnings is empty"` — no alt-screen toggle, no emit.
  - `"transitionFromLoadingCmd returns a tea.Sequence with ExitAltScreen, stderr emit, EnterAltScreen when bufferedWarnings is non-empty"`.
  - `"it emits all warning lines in order on transition"` — inject `os.Stderr` override (via `Fprintln`-capable writer); assert all lines appear in order.
  - `"it does NOT emit warnings twice on repeat transitions"` — `m.bufferedWarnings` set to nil after drain; second call is a no-op.
  - `"fatal error during loading does not reach this path"` — regression guard noting task 6-8 handles fatal via `PersistentPreRunE` before TUI launches.
- Tests in `cmd/open_test.go`:
  - `"openTUI drains bootstrapWarnings and passes them to the TUI model"`.
  - `"openTUI drains returns empty slice when no warnings were accumulated"`.
  - `"openTUI emits warnings via the TUI path, not directly to stderr"`.
- Integration-style test (subprocess-based, uses Bubble Tea's `tea.WithInput`/`tea.WithOutput`):
  - `"TUI flushes buffered warnings to stderr after loading page dismisses"` — pre-populate the sink with a fake warning; start the TUI in a goroutine; wait for dismissal; assert stderr has the warning lines.

**Acceptance Criteria**:
- [ ] `BootstrapCompleteMsg` carries `Warnings []bootstrap.Warning` (non-empty struct).
- [ ] `Model.bufferedWarnings []bootstrap.Warning` field stores warnings between message receipt and transition.
- [ ] `transitionFromLoadingCmd` returns `nil` when `bufferedWarnings` is empty (zero noise case).
- [ ] `transitionFromLoadingCmd` returns a `tea.Sequence(ExitAltScreen, emit, EnterAltScreen)` when warnings are present.
- [ ] Warnings are emitted in order (outer slice preserved; inner `Lines` preserved).
- [ ] Warnings are NOT emitted twice across repeat transitions — `m.bufferedWarnings` is cleared on emission.
- [ ] `cmd/open.go` `openTUI` drains the global `bootstrapWarnings` sink at entry and passes them to the TUI model via `SetPendingBootstrapWarnings`.
- [ ] `Init` emits `BootstrapCompleteMsg{Warnings: pendingBootstrapWarnings}` so the transition path has access.
- [ ] CLI path (task 6-9) and TUI path (this task) both use the same sink; warnings are never emitted twice across paths.
- [ ] Log-side write (task 6-2 at detection) fires regardless of path (TUI or CLI); `portal.log` has warnings regardless of stderr buffering.
- [ ] Fatal errors are handled by task 6-8 BEFORE TUI launches — no in-TUI fatal-handling code path is added by this task.

**Tests**:
- `"it buffers Warnings from BootstrapCompleteMsg into Model.bufferedWarnings"`
- `"transitionFromLoadingCmd returns nil when bufferedWarnings is empty"`
- `"transitionFromLoadingCmd returns a tea.Sequence when bufferedWarnings is non-empty"`
- `"it emits all warning lines in order on transition"`
- `"it does NOT emit warnings twice on repeat transitions"`
- `"openTUI drains bootstrapWarnings and passes them to the TUI model"`
- `"openTUI drains returns empty slice when no warnings were accumulated"`
- `"TUI flushes buffered warnings to stderr after loading page dismisses"`
- `"empty warnings slice does not toggle alt-screen"`
- `"log file always receives warnings regardless of stderr buffering path"`

**Edge Cases**:
- Zero warnings → no alt-screen toggle, no stderr output, seamless transition. Tests assert this explicitly (regression guard against spurious alt-screen flickers).
- Multiple warnings → all emitted in order, interleaved within a single `tea.Sequence`.
- Fatal error during loading: doesn't happen — task 6-8 handles fatal in `PersistentPreRunE`, which runs before `openTUI`. TUI never launches on a fatal.
- TUI panic during loading: Bubble Tea's built-in panic recovery handles teardown; Portal doesn't need to intervene.
- Buffered warnings are per-model-instance; a TUI that's launched + exited + re-launched within the same process (rare — Portal commands typically don't re-enter TUI) would see a fresh sink only if the sink drain happened. The sink drain is atomic: `Drain()` returns current contents and clears. Subsequent `Drain()` returns empty. Safe.
- Warnings accessible from non-TUI callers: the sink is package-level in `cmd/`, accessible to any `cmd/*.go` file. Task 6-9's CLI-path `EmitTo(cmd.ErrOrStderr())` uses the same sink. Both paths drain from the same place.
- Log file ALWAYS receives warnings — task 6-2 ensured the log-write happens at detection point, which is before either path's emission. No stderr path affects the log path.
- `tea.Sequence` requires Bubble Tea v0.24+; verify version compatibility at task start. If unavailable, fall back to a manual sequence: return a Cmd that does `ExitAltScreen + Print + EnterAltScreen` all in one function — violates Bubble Tea's "commands return messages" pattern slightly, but works.
- Re-entering alt-screen after stderr write: the alt-screen toggle briefly shows the user's regular terminal (where warnings appear), then re-enters alt-screen for the picker. The visible flash is acceptable — the spec says "before the TUI picker renders" which this satisfies.

**Context**:
> Spec "Observability & Diagnostics → Proactive Health Signals → TUI interaction":
> "While the Bubble Tea loading page is active, direct stderr writes would corrupt the rendered UI. The TUI path therefore **buffers** bootstrap warnings in memory during the loading window and emits them to stderr *after* the loading page is dismissed (before the TUI picker renders, or immediately before exit on fatal error). The CLI path writes to stderr directly as described. Both paths log the same content to `portal.log` regardless of stderr behaviour."
>
> Spec "Observability & Diagnostics → Fatal Bootstrap Errors → TUI path":
> "loading page never 'hangs forever.' Any unrecoverable error tears down the Bubble Tea program cleanly, emits the error, exits. The loading page is only kept up while bootstrap is making progress."
>
> Phase 5 task 5-7 Edge Cases note: "Phase 6 will extend this seam: the `BootstrapCompleteMsg` carrier will include a `[]bootstrap.Warning` slice of soft warnings buffered during bootstrap, and the handler will emit them to stderr AFTER the loading page dismisses. This task leaves the type as `struct{}` for v1 — Phase 6's extension is a structural change, not a semantic one." This task fulfils that phase-5-anticipated extension.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Observability & Diagnostics → Proactive Health Signals → TUI interaction", "Observability & Diagnostics → Fatal Bootstrap Errors".

## built-in-session-resurrection-6-11 | approved

### Task 6-11: Ship README updates: Privacy Considerations, Uninstall (both paths), hooks-fire-on-reboot-only, tmux >= 3.0 requirement, storage location

**Problem**: This feature ships a user-visible behavioural change (scrollback is now persisted to disk, hooks fire only on reboot recovery, tmux ≥ 3.0 is required, and `~/.config/portal/state/` is a new directory). Without documentation, users discover these through surprise. The spec's "Documentation Deliverables" section lists five concrete README additions: Privacy Considerations, Uninstall, hooks behaviour clarification, tmux version requirement, and storage location note. This task lands all five as distinct README edits, adhering to the spec's explicit non-goals (no exhaustive tmux API references, no internal architecture diagrams, no changelog).

**Solution**: Edit `README.md` in-place with five distinct additions:
1. **Privacy Considerations** (new top-level section, positioned after `Configuration` and before `License`) — covers `0600` file mode, local-filesystem trust model, no encryption at rest, `tmux set-option -w history-limit 0` and `tmux clear-history` as workarounds.
2. **Uninstall** (new top-level section, positioned near the end) — documents both paths: (a) just remove the binary, (b) `portal state cleanup` first then remove binary. Optional `--purge` for full state wipe.
3. **Hooks behaviour clarification** (edit existing `### xctl hooks` section) — note that hooks fire on reboot recovery, not on every detach/reattach within a server lifetime.
4. **tmux ≥ 3.0 requirement** (edit `## Install` or top of README) — add a requirements subsection or inline callout.
5. **Storage location** (edit existing `## Configuration` section) — extend the file table to include `state/` directory with `PORTAL_STATE_DIR` env override.

Explicitly NOT covered per spec non-goals: exhaustive tmux API reference, internal architecture diagrams, changelog entries.

**Outcome**: A user reading the README before installing understands:
- Scrollback is persisted, where, with what permissions, and how to opt out for sensitive workflows.
- How to uninstall cleanly (both "leave data" and "nuke everything" paths).
- Hooks now fire only on reboot recovery (behaviour change from pre-Phase-4 Portal).
- tmux 3.0 is a hard requirement; upgrade path mentioned.
- `~/.config/portal/state/` joins the existing Portal config files on disk.

A user with a running Portal from a pre-feature version upgrading to this release can read the README and understand why behaviour changed.

**Do**:
- Edit `README.md`:

  **Insert new section before `## License`** (around line 238):
  ```markdown
  ## Privacy Considerations

  Portal persists tmux pane scrollback to `~/.config/portal/state/` so your sessions can be restored across reboots. Files are written with mode `0600` (owner read/write only); the `state/` and `state/scrollback/` directories are created with mode `0700`.

  This matches the trust model of shell history (`~/.zsh_history`, `~/.bash_history`) and debug logs already on your filesystem. Scrollback is not encrypted at rest.

  **If you have workflows with sensitive output** (secrets, credentials, PII), tmux offers two native mitigations:

  - `tmux set-option -w history-limit 0` on the window prevents scrollback from accumulating there at all — Portal has nothing to capture.
  - `tmux clear-history` clears a pane's scrollback on demand; run it after sensitive output and before Portal's next save (at most 30s later).

  Portal does not ship an explicit per-session opt-out in v1. The tmux-native workarounds are the supported path.
  ```

  **Insert new section before the Privacy section** (or adjacent):
  ```markdown
  ## Uninstall

  Two supported paths:

  **1. Just remove the binary.**

  ```bash
  brew uninstall portal    # macOS via Homebrew
  rm $(which portal)       # manual installs
  ```

  Portal's defensive tmux hooks use `command -v portal` to short-circuit when the binary is absent — no error spam, no broken hooks. Your data on disk (`~/.config/portal/state/`, `hooks.json`, `projects.json`, `aliases`) is preserved. Reinstalling picks up where you left off.

  **2. Explicit teardown before removing the binary.**

  ```bash
  portal state cleanup              # kills save daemon, removes Portal's tmux hooks
  portal state cleanup --purge      # ... and wipes ~/.config/portal/state/
  brew uninstall portal
  ```

  Use `--purge` if you want a completely clean slate — it removes the scrollback, saved session index, log files, and daemon state. Non-state config (`hooks.json`, `projects.json`, `aliases`) is kept; remove them manually if desired.
  ```

  **Edit `### xctl hooks` section** (around line 160) — add a paragraph note:
  ```markdown
  **When hooks fire:** Portal fires resume hooks when a pane is freshly recreated from saved state on reboot recovery — i.e., the tmux server started fresh and Portal restored your sessions. Hooks do NOT fire on every detach/reattach within a single tmux server lifetime. If the pane still exists, its hook's process either already ran or was explicitly killed; firing again would double-launch long-running processes like `claude --resume`. This behaviour is deliberate.
  ```

  **Edit `## Install` section** (around line 23) — add a requirements subsection at the top of Install:
  ```markdown
  ### Requirements

  - **tmux ≥ 3.0** (February 2020). Portal uses array-indexed global hooks (`set-hook -ga`) which require tmux 3.0 or later. Earlier versions are not supported; Portal emits `Portal requires tmux ≥ 3.0 (found <version>). Please upgrade.` and exits non-zero on startup.
  - **Go, macOS, Linux** — no change from Portal's existing requirements.
  ```

  **Edit `## Configuration` table** (around line 230) — add the state directory:
  ```markdown
  | File / Directory | Purpose | Env override |
  |---|---|---|
  | `aliases` | Path aliases (key=value, one per line) | `PORTAL_ALIASES_FILE` |
  | `projects.json` | Remembered project directories | `PORTAL_PROJECTS_FILE` |
  | `hooks.json` | Per-pane resume hooks (pane → event → command) | `PORTAL_HOOKS_FILE` |
  | `state/` | Saved session structure + scrollback for automatic restoration on reboot | `PORTAL_STATE_DIR` |

  The `state/` directory contains `sessions.json`, per-pane scrollback files under `state/scrollback/`, the daemon's PID + version markers, and `portal.log`. See [Privacy Considerations](#privacy-considerations).
  ```

  **Edit `## Automatic Server Bootstrap` section** (around line 211) — replace the tmux-continuum/resurrect-oriented paragraph with the new reality:
  ```markdown
  ## Automatic Server Bootstrap & Restoration

  Portal automatically starts the tmux server if it isn't already running and restores your saved sessions in the same step. After a reboot, your sessions come back with their structure, layout, zoom state, working directories, and pane scrollback (including ANSI colors) — automatic, on by default, no configuration required.

  **How it works:**

  - On any command that needs tmux (`open`, `list`, `attach`, `kill`), Portal checks for a running server and starts one if missing.
  - Portal then re-creates any saved sessions not already live. Scrollback is injected lazily when you attach — fast boot, no perceptible wait.
  - Resume hooks (registered via `xctl hooks set --on-resume`) fire automatically on the freshly-recreated panes.
  - The **TUI** shows a brief "Restoring sessions…" loading screen for at most ~1.2 seconds.
  - **CLI commands** run silently — no progress output in the common case.

  Portal replaces reliance on tmux-continuum and tmux-resurrect; those plugins are no longer needed for session persistence. If you have them installed alongside Portal, uninstall them (or disable `@continuum-restore on`) to avoid duplicate restoration attempts.
  ```

- Do NOT add:
  - Exhaustive tmux API references (spec non-goal).
  - Internal architecture diagrams (spec non-goal).
  - Changelog entries (out of scope for docs; handled by release process).
  - Descriptions of internal commands (`daemon`, `notify`, `signal-hydrate`, `hydrate`, `migrate-rename`) — they are internal.
- Describe `xctl state status` and `xctl state cleanup` briefly in the `### xctl` command list so users know how to invoke them. Add:
  ```markdown
  ### `xctl state`

  Portal owns tmux session persistence end-to-end. The `state` subcommand exposes inspection and cleanup:

  - `xctl state status` — prints daemon status, last save time, captured session/pane counts, state size, and recent warnings. Non-zero exit if daemon is down, last save > 5 min, or recent warnings in log.
  - `xctl state cleanup [--purge]` — removes Portal's tmux hooks and kills the save daemon. `--purge` additionally wipes `~/.config/portal/state/`.
  ```
  Positioned near `### xctl clean` in the commands section.
- Run `grep -n "tmux-continuum\|tmux-resurrect" README.md` after edits to verify no stale references remain (the original automatic-bootstrap section referenced continuum/resurrect; the replacement paragraph removes this dependency from Portal's messaging).

**Acceptance Criteria**:
- [ ] A `## Privacy Considerations` section exists in `README.md`, covering `0600` mode, local trust model, no encryption at rest, and the `history-limit 0` / `clear-history` workarounds.
- [ ] A `## Uninstall` section exists, documenting both the "just remove binary" and `portal state cleanup [--purge]` paths.
- [ ] The `### xctl hooks` section (or its equivalent) includes a paragraph stating hooks fire on reboot recovery only — not on every detach/reattach within a server lifetime.
- [ ] The `## Install` section includes a `### Requirements` subsection listing `tmux ≥ 3.0` with the spec's error message text.
- [ ] The `## Configuration` section's file table includes `state/` as a directory entry with `PORTAL_STATE_DIR` env override.
- [ ] A `### xctl state` subsection describes `status` and `cleanup` with their flags.
- [ ] The pre-existing `## Automatic Server Bootstrap` section is updated to reflect Portal-owned restoration (not plugin-agnostic waiting) — renamed to `## Automatic Server Bootstrap & Restoration` or similar.
- [ ] No exhaustive tmux API references added (spec non-goal).
- [ ] No internal architecture diagrams added (spec non-goal).
- [ ] No internal-command descriptions (daemon / notify / signal-hydrate / hydrate / migrate-rename) in the README.
- [ ] No changelog entries added (handled by release process, not the README).
- [ ] `grep -n "tmux-continuum\|tmux-resurrect" README.md` returns zero matches (the pre-feature section that referenced them is rewritten).
- [ ] Markdown renders correctly (lint via local `glow` or GitHub preview before merge).

**Tests**:
- Automated doctest-style: `"README.md contains a '## Privacy Considerations' heading"` (manual verification; no Go test).
- `"README.md contains a '## Uninstall' heading"`.
- `"README.md mentions tmux ≥ 3.0 as a requirement"`.
- `"README.md's xctl hooks section explains hooks-fire-on-reboot-only"`.
- `"README.md's configuration table includes the state/ directory"`.
- `"README.md no longer references tmux-continuum or tmux-resurrect as dependencies"`.
- Manual review: read the README end-to-end, verify coherence and no orphan references to old behaviour.

**Edge Cases**:
- Markdown link anchors: `[Privacy Considerations](#privacy-considerations)` — GitHub auto-generates lowercase-dashed anchors from heading text. Verified by rendering via GitHub.
- Consistency with pre-feature sections: the `## Automatic Server Bootstrap` section mentions continuum/resurrect as friendly workflows — rewriting it removes that framing. Users upgrading who relied on the old text see the replacement's clarity.
- The existing `### xctl hooks` description (line 160ff) should not be rewritten — just augmented with the firing-time clarification paragraph.
- The `xctl` vs `portal state` naming: README uses `xctl` consistently; state commands accessed via `xctl state status` / `xctl state cleanup`. Users who alias with `--cmd p` get `pctl state ...`. README already accommodates this with the prefatory "Examples below use the default `x` / `xctl` function names" note.
- `--purge` flag documented in the Uninstall section with a clear "completely clean slate" warning.
- `PORTAL_STATE_DIR` env override — Phase 2 task 2-1 added the env var support; README documents it.
- Backward-compat: pre-feature Portal versions did NOT restore sessions. Users reading the README post-upgrade see the new Restoration section and understand why behaviour changed. No explicit "behaviour change" callout is required in the README itself (changelog handles that).
- Do NOT document internal-only subcommands (daemon, notify, signal-hydrate, hydrate, migrate-rename). They are hidden from `--help` and absent from README.

**Context**:
> Spec "Documentation Deliverables → README 'Privacy Considerations' Section":
> "The v1 release includes a brief **Privacy Considerations** section in the README covering:
> - Scrollback is persisted to `~/.config/portal/state/` with file mode `0600` (owner-only read/write).
> - Same local-filesystem trust model as shell history and debug logs users already have on disk.
> - No encryption at rest.
> - Users with genuinely sensitive workflows can set `tmux set-option -w history-limit 0` on the relevant window so nothing accumulates in scrollback for Portal to capture; `tmux clear-history` after sensitive output is a related manual mitigation.
> This documentation exists because v1 ships without an ephemeral-session opt-out mechanism (deferred per Scope). The README note gives users the tmux-native workarounds so they are not surprised by scrollback persistence for sensitive contexts."
>
> Spec "Documentation Deliverables → README Uninstall Section":
> "Document the two supported uninstall paths:
> 1. **Just remove the binary** (standard package manager uninstall). The defensive `command -v portal` hook guard handles residual tmux state transparently — no error spam, no broken hooks. User data on disk is preserved (standard Unix convention).
> 2. **Explicit teardown first** via `portal state cleanup` — kills the saver daemon, removes Portal's `set-hook -ga` entries, optionally clears the state directory. For users who want a deliberate clean slate before removing the binary."
>
> Spec "Documentation Deliverables → Existing User-Facing Documentation Updates":
> "Changes to existing docs required by this specification:
> - **Hooks documentation** ... clarify that hooks fire **on reboot recovery** (when the pane is freshly recreated from saved state), not on every detach/reattach within a server lifetime.
> - **Installation requirements:** document the **tmux ≥ 3.0** requirement.
> - **Storage location:** note that Portal now writes to `~/.config/portal/state/` in addition to `~/.config/portal/{hooks.json,projects.json,aliases}`."
>
> Spec "Documentation Deliverables → Not Documentation Scope":
> "- **Exhaustive tmux API references** — users don't need to understand `capture-pane -e` internals.
> - **Internal architecture diagrams** — the hidden `portal state daemon` / `notify` / `signal-hydrate` / `hydrate` commands are internal; user docs don't need to explain their interplay.
> - **Changelog entries** — handled by standard release process, not specified here."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "Documentation Deliverables".
