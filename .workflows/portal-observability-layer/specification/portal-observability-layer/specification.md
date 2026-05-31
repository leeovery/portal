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

## Working Notes
