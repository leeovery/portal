# Specification: Portal Observability Layer

## Overview

### Purpose

Portal's logging is incidental — lines added ad hoc, not a deliberate observability layer. On 2026-05-28 a `hooks.json` wipe followed by a saver-disappearance event was undiagnosable: `portal.log` had rotated the evidence away (1 MiB threshold, single `.old` overwritten on each rotation) and was later found at 0 bytes. This feature replaces incidental logging with a disciplined, structured observability layer built on Go's `log/slog`, fixes the rotation system that caused the evidence loss, and instruments the codebase so that `grep "<subsystem>:" portal.log` reconstructs any subsystem's behaviour at the production-default level.

### What we're building

- A new `internal/log` package wrapping `log/slog` with a custom rotating handler (factory `For`, plus `Init`, `Close`, `SetTestHandler`).
- Calendar-daily log rotation with a configurable size-cap safety valve, replacing the 1 MiB churn-prone scheme.
- Bounded, auditable retention (default 30 days) with per-deletion breadcrumbs.
- A locked log-level contract (DEBUG/INFO/WARN/ERROR), production default INFO.
- A closed subsystem-prefix taxonomy (15 components), a closed attr-key vocabulary, and mandatory per-record baseline attrs.
- Defensive invariants (per-process lifecycle markers, rotated-file immutability, `O_CREAT|O_EXCL` first-of-day open) that make log destruction detectable post-hoc.
- Instrumentation catalogs: state-mutation audit trail, cycle-level summaries, saver/daemon lifecycle events, hydrate-helper forensic trail, and a boundary-context-preservation sweep.

### Scope boundaries

**In scope:** everything above, delivered across two PRs.

**Out of scope** (explicitly deferred to separate future work):
- Shell-envelope hook-wrapping to capture post-exec hook exit status.
- An out-of-band rotation audit channel (`portal-rotation.log`).
- Compression of rotated logs.
- Migrating `sessions.json` / daemon-internal state files to the user-config audit-trail pattern.

### Delivery structure

Two-PR rollout gated by a 7-day production observation window:
- **PR 1** — `internal/log` foundation + migration sweep + process lifecycle markers + log-level propagation + full hydrate-helper instrumentation.
- **PR 2** — pattern rollout across all remaining subsystems (state-mutation audit, cycle summaries, lifecycle catalog, boundary sweep, gap-closures).

### Specification roadmap

1. Overview (this section)
2. The `internal/log` package — foundation, API, migration sweep
3. Subsystem prefix taxonomy — components, attrs, baselines, rendering
4. Log-level discipline — the level contract + mechanical selection table
5. Call-site logging pattern
6. Log rotation mechanism
7. Retention policy and audit
8. Defensive invariants against log destruction
9. Log-level propagation verification
10. State-mutation audit trail
11. Diagnostic context preservation at boundaries
12. Cycle-level summary cadence and shape
13. Saver and daemon lifecycle event taxonomy
14. Hook-firing observability limit
15. Rollout sequencing & PR scope
16. Open threads & out of scope

---

## The `internal/log` package

### Decision

Adopt Go's standard-library `log/slog` (stable since Go 1.21) as the logging foundation. The existing bespoke printf-style `internal/state/logger.go` is replaced wholesale. Rationale: standard library (no new dependency), structured fields native, handler-based (one set of call sites emits human-readable text for tail/grep *or* JSON for tooling), and future-proof as instrumentation call sites multiply.

A new package `internal/log` is the single owner of all logging machinery. No `*slog.Logger` is constructed anywhere outside it.

### Public API

```go
package log

// Init configures the process-wide logger: builds the custom rotating handler
// (baseline attrs pid/version/process_role injected per-record) and atomically
// swaps it in behind the stable root logger. Called from main.go before any
// other portal code runs. IDEMPOTENT and re-entrant — a second call re-points
// the handler, it does NOT panic. By convention only main calls Init in prod.
func Init(stateDir string, version string, processRole string) error

// For returns a component-bound child logger (root.With("component", name)).
// Safe to call before Init — always returns a valid, non-nil *slog.Logger.
// Callers cache it at package init via a package-level var; the cached logger
// picks up Init's handler automatically because the swap lives inside the
// shared handler indirection.
func For(component string) *slog.Logger

// Close emits the "process: exit" marker, computing took from the
// package-private startTime captured at Init. Does NOT call os.Exit — the
// logger owns no control flow. (Marker semantics: see Defensive invariants.)
func Close(exitCode int)

// SetTestHandler swaps in h for the duration of the test and restores the
// previous handler via t.Cleanup. Test-only seam for capturing or silencing
// log output in-process — no subprocess required.
func SetTestHandler(t *testing.T, h slog.Handler)
```

### Init/For contract — swappable-handler indirection

- The root `*slog.Logger` is constructed in `internal/log`'s own package `init`, over a small custom handler whose inner delegate is **replaceable** (mutex- or `atomic.Pointer`-guarded). Because every consumer imports `internal/log`, Go runs its `init` first, so `root` exists before any `For` call.
- `Init` swaps the configured rotating handler into that indirection. Every `For`-created logger shares the indirection, so loggers cached at package-init (before `Init` ran) route to the configured handler once `Init` lands — no stale-logger footgun, no nil returns.
- Before `Init` runs, the indirection holds a **safe default handler that writes INFO-and-above as text to stderr**. (The discussion left this as "discard or stderr-text"; pinned to stderr-text-at-INFO so any unexpected pre-`Init` log surfaces rather than silently vanishing. `main` calls `Init` first thing, before any TUI rendering, so this window carries no expected output.)
- The "configured once in prod" invariant is preserved by **convention** (only `main` calls `Init`) plus the test-only `SetTestHandler` seam — **not** by panicking. In-process tests swap a capture / `io.Discard` handler; no subprocess required.
- **Cost:** one synchronized read (atomic load / RLock) per `Handle`. Negligible behind slog's level filter, and free-riding on the custom handler already needed for rotation.
- **Baseline-attr injection:** baseline attrs (`pid`, `version`, `process_role`) are injected by the configured handler **per-record**, NOT via `root.With(...)` at construction — otherwise package-init children created before `Init` would miss them.

The custom handler also owns rotation (see Log rotation mechanism), retention sweeps (see Retention policy), the `component:`-prefix text rendering and the lifecycle-marker level-filter bypass (see Subsystem prefix taxonomy and Defensive invariants). The text-mode line format is fully specified in the Subsystem prefix taxonomy section.

### Consumer usage

Every package that logs binds its component name once at package init:

```go
import "github.com/leeovery/portal/internal/log"

var logger = log.For("<component-name-from-taxonomy>")
```

Call sites then use `logger.{Debug,Info,Warn,Error}` directly with `slog.Attr` args (`"key", value` variadic form), attr keys drawn from the closed vocabulary.

### Migration sweep (single PR, big-bang — no adapter shim, no co-existence period)

- The `internal/state.Logger` type is **deleted**.
- The `internal/state.Component*` constants (`internal/state/logger.go:30-38`) are **deleted**.
- The pipe-delimited line format (`timestamp | level | component | message`) is **deleted**.
- All call sites of `state.Logger.{Debug,Info,Warn,Error}(component, fmt, args...)` are rewritten to `logger.{Debug,Info,Warn,Error}(msg, attrs...)`, component bound at package-init via `log.For`.
- `state.NopLogger()` is **deleted**; tests requiring a silent logger use `slog.New(slog.NewTextHandler(io.Discard, nil))` directly.
- All test mock surfaces in `bootstrapDeps` and friends that previously held `*state.Logger` are updated to hold `*slog.Logger`.

---

## Subsystem prefix taxonomy

### Rendering mechanism

`component` is a structured slog attr set at every call site. The custom `slog.Handler` renders **text** output with `component:` as a literal prefix at the start of the message body, timestamp + level preceding it. `component` remains a real structured attr under the hood, so **JSON** output and programmatic filtering work unchanged.

Example text-mode line:
```
2026-05-29T08:38:00Z INFO hydrate: ok pane_key=foo:0.0 took=1.2s pid=12345 version=0.5.0 process_role=hydrate
```

Same call site, JSON handler:
```json
{"time":"2026-05-29T08:38:00Z","level":"INFO","component":"hydrate","msg":"ok","pane_key":"foo:0.0","took":"1.2s","pid":12345,"version":"0.5.0","process_role":"hydrate"}
```

Grep idiom preserved: `grep "hydrate:" portal.log` produces the per-subsystem audit trail. Programmatic filtering also works: handlers route by `component` attr, JSON tooling indexes it.

Call-site binding is the factory pattern defined in *The `internal/log` package* — each consumer binds its component once at init via `var logger = log.For("<component>")`.

### Closed component value space (15 total)

```
bootstrap  daemon  restore  hydrate  notify  hooks  preview
saver  capture  signal  log-rotate  clean  aliases  projects
process
```

This list is the **single source of truth** for the component count.

| Component | Owns |
|---|---|
| `daemon` | `portal state daemon` runtime — tick loop, self-supervision |
| `bootstrap` | The 11-step bootstrap orchestrator |
| `restore` | Two-phase restore engine (skeleton + geometry + scrollback) |
| `hydrate` | Per-pane `portal state hydrate` helper — FIFO open, scrollback replay, exec chain |
| `notify` | Notification helpers |
| `hooks` | `hooks.json` mutations (`portal hooks set/rm`, `hookStore.CleanStale`) |
| `preview` | TUI scrollback preview page |
| `saver` | `_portal-saver` session lifecycle |
| `capture` | The daemon's per-tick capture loop (promoted from inside `daemon`) |
| `signal` | FIFO signaling — `EagerSignalHydrate`, hydrate-helper signal receipt |
| `log-rotate` | Rotation and retention events |
| `clean` | `portal clean` command path |
| `aliases` | `aliases` store mutations |
| `projects` | `projects.json` store mutations |
| `process` | Portal-binary lifecycle markers (`start` / `exit` / `exec` / `panic` / `log-level resolved`) only, regardless of subcommand |

`process` is reserved for portal-binary lifecycle/diagnostic markers only; subsystem-level lifecycle events have their own components. `tmux` is **deliberately excluded** — `internal/tmux` is a thin wrapper; logging at that layer produces extreme volume per tmux call. Tmux-call detail rides as DEBUG breadcrumbs under the caller's component.

### Closed attr-key value space (45 keys)

**Contextual** (set per call as relevant) — 13:

| Key | What |
|---|---|
| `pane_key` | Structural pane key (canonical persisted form, e.g. `session__window.pane`) |
| `tmux_pane` | `$TMUX_PANE` env var (live tmux pane id, e.g. `%42`) |
| `session` | tmux session name |
| `project` | project name from `projects.json` |
| `path` | filesystem path |
| `took` | duration (`time.Duration` rendered) |
| `error` | error message (slog idiom: `slog.Any("error", err)`) |
| `error_class` | swallowed-error classification: `expected` / `unexpected`; or AtomicWrite failure phase |
| `hook_key` | hooks subsystem — the structural hook key |
| `op` | state-mutation breadcrumbs — `set` / `modify` / `rm` / `clean-stale` / `migrate` / `set-noop` |
| `alias` | aliases-store key |
| `value` | state-mutation — verbatim new value for `set` / `modify` |
| `via` | state-mutation origin — `cli` / `internal` / `migrate` |

**Cycle-summary** (set per summary line as relevant) — 11: `sessions`, `panes`, `entries`, `steps`, `warnings`, `natural_churn`, `anomalous`, `reaped`, `killed`, `unset`, `entries_failed`.

**Lifecycle** (set per saver/daemon lifecycle event) — 7: `target_pid`, `from_pid`, `to_pid`, `reason`, `ticks`, `threshold`, `flush_completed`.

**Hydrate** (set per hook-firing exec-chain event) — 3: `result`, `hook_present`, `bytes`.

**Process** (set per `process:` lifecycle/diagnostic line) — 7: `cmd`, `args`, `target`, `code`, `resolved`, `source`, `raw`. `target` + `args` are the **shared exec-handoff attrs** used by both `process: exec` and `hydrate: exec`, so the two `syscall.Exec` markers are structurally parallel.

**Baseline** (auto-injected per-record by the configured handler) — 4: `component` (set per package via `log.For`), `pid`, `version`, `process_role`.

### Mandatory baseline attrs

Every line carries these four, injected **per-record** by the configured handler (not via `root.With` — so package-init children created before `Init` still carry them):

| Key | Set where |
|---|---|
| `component` | Per-package via `log.For("...")` |
| `pid` | Root logger construction (`os.Getpid()`) |
| `version` | Root logger construction (build-time `cmd.version`) |
| `process_role` | Root logger construction — one of `daemon` / `bootstrap` / `hydrate` / `hooks_cli` / `tui` / `clean`. Identifies which portal binary invocation emitted the line; critical for multi-writer disambiguation on reboot-recovery days. |

Baseline attrs add ~50 bytes per line — negligible at INFO steady-state (~3 MB/day). They make every line self-describing for forensic use across multi-writer days.

### Conventions

- **snake_case** for all attr keys.
- **Message string is a terse phrase**; data lives in attrs: `logger.Info("ok", "pane_key", k, "took", d)` — never `logger.Info(fmt.Sprintf(...))`.
- **Sticky context via `.With(...)`** when multiple events share context.

### Custom `slog.Handler` text-mode rendering rule

```
<RFC3339Nano timestamp> <LEVEL> <component>: <msg> <attrs as key=value pairs>
```

- `<component>` is read from the bound `component` attr and emitted as a literal prefix immediately before the colon. It is **not** also rendered in the attrs key=value list.
- `<msg>` is the slog record's message field.
- `<attrs>` are emitted space-separated `key=value` in order: contextual attrs (in `slog.Record` order), then the three remaining baselines (`pid`, `version`, `process_role`).
- Multi-word string values are quoted with `"`.
- `time.Duration` values render with Go's default `String()` (e.g. `1.234s`).
- `slog.Group` attrs flatten to **dotted keys** (`group.key=value`), mirroring the JSON handler's nesting.

### Custom `slog.Handler` JSON-mode rendering rule

Standard `slog.NewJSONHandler` output, no special handling — `component` becomes a normal `"component":"<name>"` JSON field.

### Extension policy

- New components require explicit amendment of THIS specification's closed component list.
- New attr keys require the same amendment process.
- Spec writers and code reviewers MAY NOT introduce new component or attr names ad hoc.
- The space is **genuinely closed** — every contributor consults these lists; no ad-hoc invention at call-site time.

---

## Working Notes
