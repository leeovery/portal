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

## Working Notes
