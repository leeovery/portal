# Discussion: Portal Observability Layer

## Context

Portal's logging today is incidental — lines added when someone needed them — not a deliberate observability layer. A real incident on 2026-05-28 (a `hooks.json` wipe at 08:18 BST followed by a saver-disappearance event, with `portal.log` then truncated to 0 bytes at 08:38 BST) destroyed all diagnostic evidence before the symptom could be investigated. The same shape of gap shows up across several unrelated subsystems: silent error paths, missing tick-level summaries, discarded diagnostic context at boundaries, and inconsistent log prefixes that defeat grep-based audit trails.

The seed for this work was a reboot where some Claude `--resume` hooks fired and others didn't, and `portal.log` couldn't tell us which path each helper actually took (`project_reboot_hooks_followup` in MEMORY.md). The investigation surfaced parallel gaps during the cycle-1 review of `slow-open-empty-previews-and-zombie-sessions`. Together these point to a coherent set of patterns to apply consistently across the codebase — not a one-off patch.

The feature also has to cover **log rotation**: the current "rotate to 0 bytes whenever it feels like it" behaviour is the wrong default and was the proximate cause of the 2026-05-28 evidence loss.

### References

- Seed: `.workflows/.inbox/.archived/ideas/2026-05-25--portal-observability-layer.md`
- Memory: `project_reboot_hooks_followup`
- Related: `slow-open-empty-previews-and-zombie-sessions` (cycle-1 review surfaced parallel gaps)

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Logger library (slog adoption) [decided]
  Log rotation mechanism [decided]
  Retention policy and audit [decided]
  Log-level discipline (DEBUG/INFO/WARN/ERROR contract) [pending]
  Subsystem prefix taxonomy [pending]
  Defensive invariants against unknown-cause log destruction [pending]
  State-mutation audit trail for user config files [pending]
  Decision-point INFO line shape [pending]
  DEBUG breadcrumb pattern on swallowed errors [pending]
  Diagnostic context preservation at boundaries [pending]
  Cycle-level summary cadence and shape [pending]
  Log-level propagation verification [pending]
  Saver and daemon lifecycle event taxonomy [pending]
  Hook-firing observability limit (syscall.Exec) [pending]
  Rollout sequencing and scope bundling [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture.*

---

## Log rotation mechanism

### Context

Today's logger (`internal/state/logger.go`) rotates `portal.log` to `portal.log.old` when a daemon write would push the file past 1 MiB. Only the daemon rotates (`OpenLogger(path, rotate=true)`); non-daemon writers (bootstrap, hooks CLI, hydrate helpers) call `OpenLogger(path, false)` and append. Only one rotated file is ever kept — `os.Remove(oldPath)` runs before every rename, so back-to-back rotations destroy the previous `.old`.

This is the actual mechanism behind the 2026-05-28 evidence loss. The inbox premise ("rotate to 0 bytes whenever it feels like it") describes the symptom; the mechanism is rotation churn — at 1 MiB threshold under any active load, the file rotates every few hours, and any second rotation within a short window overwrites the previous `.old`. The 08:18 BST hooks.json wipe lived in a `.old` file that was overwritten by a subsequent rotation before the user looked.

There is a separate, currently-unidentified zeroing bug: the user's portal.log is at 0 bytes with no `.old` on disk, which suggests something other than rotation zeroed it (no `O_TRUNC` exists in `logger.go`). Recorded in Open Threads — investigate during implementation; may be resolved as a side effect of replacing the rotation system.

### Options Considered

**A. Calendar-only daily rotation.** Local midnight boundary. Files named `portal.log.YYYY-MM-DD` for completed days; same-day overflow appends `.N`. No size threshold.
- Pros: Mirrors `logrotate daily dateext` / Go `lumberjack` daily mode. Burst of WARNs in one hour can't push out yesterday's history. Simple model: one file per day, period.
- Cons: A pathological runaway emitting 100+ MB/min could fill the disk between midnight ticks before any rotation fires.

**B. Size-only with larger headroom.** Same shape as today but threshold raised to 50–100 MiB, retaining multiple rotated files instead of one.
- Pros: Bounds disk use deterministically. No clock-dependence.
- Cons: A WARN burst can still flush yesterday's history out of the rotation queue. The forensic horizon depends on activity, not on calendar time — opposite of what we want.

**C. Calendar primary + size-cap safety valve.** Daily rotation as in A; if today's file reaches a generous threshold (e.g. 500 MiB or 1 GiB), force a same-day rotation (`.N` overflow file).
- Pros: Normal-day behaviour identical to A. Disk-fill safety net for runaway scenarios.
- Cons: Adds a code path that almost never fires in production. Slightly more complex than A.

### Journey

The inbox proposed calendar-daily as the fix; first instinct was "daily + size-cap safety valve" because that's the logrotate default. Sizing the actual log volumes flipped that:

| Mode | Steady-state | Stressed |
|---|---|---|
| WARN (default) | 50–200 KB/day | 1–10 MB/day during an incident |
| INFO (cycle summaries enabled) | 1–5 MB/day | 10–50 MB/day |
| DEBUG (`PORTAL_LOG_LEVEL=debug`) | 5–20 MB/day | 50–500 MB/day during a stuck loop |

Across realistic modes, the rolling 30-day window peaks at ~600 MB even at DEBUG — trivial disk cost. Size-cap only fires on stuck loops, and in those cases the runaway *is* the evidence: capping it at 100 MB doesn't help debug it, it just splits the same loop across two files. Retention bounds total disk use either way.

That made size-cap defensible only as a disk-fill defence at a very generous threshold (500 MB / 1 GB) where it'd never fire in normal operation. Open question: is the disk-fill defence worth the extra code path?

The current 1 MiB threshold being laughably small also explains why the `.old` keeps getting overwritten before the user can read it — rotation under any active load fires every few hours, not every few days.

### Decision

**Locked: Option C — calendar-daily rotation primary, configurable size-cap safety valve, library-encapsulated ownership.**

**Rotation ownership (library-level, every-writer date-aware):**
- The `slog.Handler` in every portal process is date-aware. On each write, it computes today's filename via `time.Now().Format("2006-01-02")` and ensures the open fd points at today's file.
- First write of the day across all processes opens the new day's file via `O_CREAT|O_EXCL`. First writer wins the create race atomically; losers detect the file exists and open `O_APPEND` — race-safe on Unix for slog-sized line writes (< PIPE_BUF).
- A symlink `portal.log → portal.log.YYYY-MM-DD` is swung atomically (`os.Symlink` + `os.Rename`) at the boundary so `tail -f portal.log` always follows today's file regardless of which process owns the swing.
- Daemon-down across midnight is solved by construction — any waking process's first write opens today's file. No explicit catchup logic needed. (Closes review F1 and F2.)

**Boundary, filenames, size cap, immutability:**
- Calendar boundary: local midnight (timezone/DST handling deferred to spec phase).
- Filenames: `portal.log.YYYY-MM-DD` for completed days; same-day overflow on size-cap appends `.N`. Semantics for N — monotonic via `O_CREAT|O_EXCL` retry against highest existing `.N` — deferred to spec.
- Size-cap safety valve: default **500 MB**. Configurable via `PORTAL_LOG_ROTATE_SIZE` env var accepting K/M/G suffixes (e.g. `500M`, `1G`). When today's file reaches the threshold, library rotates to `portal.log.YYYY-MM-DD.N`.
- Rotated files are immutable (`chmod 0400` once they are no longer today's file).
- Default 500 MB chosen as: never fires in normal use even at DEBUG steady-state (20 MB/day), catches a runaway within ~1 day before disk can fill.

**Operational edges deferred to spec phase**: DST/timezone transitions and the local-midnight definition; behaviour across portal version upgrades mid-day; first-startup migration from any existing `portal.log.old`; disk-full and EACCES failure modes; retention-pass scheduling and missed-day catchup; `.N` ordering details. These are tactical — none invalidates the strategic shape locked above.

---

## Retention policy and audit

### Context

Lost evidence is also lost retention. The new rotation system needs a bounded retention window: keep rotated logs for N days, delete older ones. Deletion must be auditable so an operator can grep the rotation history.

### Options Considered

**Window**: 7 / 14 / 30 / 90 days hardcoded, or env-configurable.
**Audit shape**: Silent deletion / per-deletion INFO line / batched daily summary.
**Configuration locus**: env var (`PORTAL_LOG_RETENTION_DAYS`) / config file entry / both.

### Journey

30 days is the inbox proposal and matches the "this happened last week" forensic horizon — the primary use case is investigating incidents 1–14 days after they occurred. logrotate's defaults sit in the 4–7 week range. Shorter windows risk the same evidence-gone-by-the-time-you-look problem we have today; longer windows give more cushion at trivial disk cost (worst-case ~600 MB at DEBUG over 30 days).

Making the window configurable matters for users with constrained disk budgets or for users who want longer history. Env var is the simplest locus — matches existing portal envs (`PORTAL_LOG_LEVEL`, `PORTAL_PROJECTS_FILE`, etc.). No config-file entry needed.

Per-deletion INFO line is required: the 2026-05-28 incident taught that silent destruction is the actual bug. A single INFO `log-rotate: deleted portal.log.2026-04-29 (retention 30d)` per deleted file means `grep 'log-rotate:' portal.log` reconstructs the rotation history. Batched daily summary is harder to correlate against specific deletions.

`portal clean` should NOT touch rotated logs by default — clean is a hygiene command, not an evidence-destroyer. An explicit `portal clean --logs` opts in to log cleanup; without it, only retention-based deletion (by the daemon's daily rotation pass) removes rotated files.

### Decision

**Locked:**
- Default retention: **30 days** of rotated history kept on disk.
- Configurable via `PORTAL_LOG_RETENTION_DAYS` environment variable. Invalid values fall back to default with a startup WARN.
- Per-deletion INFO breadcrumb with stable prefix `log-rotate: deleted <file> (retention <N>d)`. One INFO line per deleted file.
- `portal clean` preserves rotated logs; `portal clean --logs` opts in to log cleanup.

**Open in spec phase**: where the rotation/retention breadcrumb lands across the midnight boundary (yesterday's file vs today's, or both); whether rotated logs are compressed (would cut 30d worst-case from ~600 MB to under 80 MB); how an open-fd-on-yesterday's-file interacts with retention `unlink` (zombie writers post-deletion). Plus whether to mirror retention breadcrumbs to a separate out-of-band file so the audit trail survives even if rotation itself is the corruptor.

---

## Logger library (slog adoption)

### Context

The existing `internal/state/logger.go` is a thin printf-style wrapper around `os.File`: line format `timestamp | level | component | message\n`, levels `LevelDebug/Info/Warn/Error`, component constants (`daemon`, `restore`, `hydrate`, ...), file-mutex serialisation. It works but is bespoke — no structured fields, no handler abstraction, no contextual propagation. Any extension (e.g. structured per-key fields for state-mutation breadcrumbs) means inventing conventions on top of printf.

The broader framing — "use logging anywhere it helps, with disciplined levels, learn from known patterns" — pushes toward a structured leveled logger as the foundation. Go's standard library `log/slog` (stable since Go 1.21) is the canonical choice: handler-based (swap text/JSON without changing call sites), `slog.Attr`-based field passing, level + context propagation via `slog.Logger`.

### Options Considered

- **A. Keep printf + extend.** Add structured field conventions on top of the existing format. Minimal migration. Loses standardisation; future tooling expects slog-shaped logs.
- **B. Adopt `log/slog`.** Migrate call sites, replace `state.Logger` with a thin wrapper around `slog.Logger` (or use slog directly). Standard library, no new dependency, future-proof, structured fields native. Requires a sweep of every existing `logger.Info/Warn/...` call site, then becomes the foundation for the broader instrumentation rollout.

### Decision

**Locked: Option B — adopt `log/slog`.**

Reasoning: standard library, structured by default, handler-based (one set of call sites can emit human-readable text for tail/grep and JSON for tooling/aggregation depending on handler), and future-proof. The broader scope of "instrument everywhere" means call sites multiply, and we want them ergonomic and conventional from day one. Migration cost is manageable today (small N of existing call sites) and savings compound as we add new sites.

**Implementation detail for spec phase**: the existing rotation/multi-writer machinery in `logger.go` needs to be re-expressed as a custom `slog.Handler` (or wrap a `slog.TextHandler` with rotation-aware `io.Writer`) so rotation behaviour is preserved end-to-end. The text handler's line format can be retained for backward compatibility with existing greps, or replaced with an `slog`-canonical key=value layout — TBD during spec.

---

## Summary

### Key Insights

1. The 2026-05-28 evidence loss is rotation-churn (1 MiB threshold + single `.old` overwritten on each rotation), not literal truncation. Reframes the inbox premise.
2. Realistic per-day log sizing makes calendar-daily rotation the right primary boundary; size cap is a disk-fill safety valve, not the main mechanism.
3. Silent destruction (no log line on retention deletion or rotation) was the actual incident-multiplier. Every destructive action must emit a breadcrumb.
4. State-mutation breadcrumbs need to cover *internal* mutations too (e.g. `hookStore.CleanStale`), not just user-CLI mutations — the bash hook log can only see user-driven calls.
5. The scope is broader than the inbox's seven patterns — it's "use logging anywhere it aids debugging or insight under a disciplined level taxonomy". That makes level discipline and prefix taxonomy foundational, not auxiliary.
6. The foundation is `log/slog` (Go 1.21 standard library). Structured fields, handler abstraction, standard-library posture all compose better than extending the existing printf wrapper.

### Open Threads

- **Current `portal.log` zeroing bug** — no `.old` exists, no `O_TRUNC` in `logger.go`, so the destruction mechanism is currently unidentified. Not logged as a separate inbox bug — likely surfaced or resolved as a side effect of the rotation rewrite; investigate during implementation.
- **Size-cap safety valve decision** — Option A (no cap) vs Option C (500 MB / 1 GB cap). Blocks final lock on Log rotation mechanism.
- **Hook command privacy** — verbatim vs SHA-256 hash prefix vs truncation. To resolve when state-mutation audit trail subtopic is explored.

### Current State

- Three subtopics decided: Logger library (slog), Log rotation mechanism (Option C, 500 MB default), Retention policy and audit (30d default).
- Scope expansion confirmed: instrument the whole codebase wherever logging would aid debugging/insight, under a disciplined level taxonomy. Inbox's seven patterns remain the minimum scope.
- Pivoting next to log-level discipline as the foundational contract every log line follows.
- 14 review findings pending walk-through; several pertain directly to rotation operational edges deferred to spec.
- Remaining map: 11 pending subtopics on level discipline, prefix taxonomy, defensive invariants, state-mutation audit trail, patterns, lifecycle events, and rollout.
