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
  Log-level discipline (DEBUG/INFO/WARN/ERROR contract) [decided]
  Subsystem prefix taxonomy [decided]
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

## Log-level discipline (DEBUG/INFO/WARN/ERROR contract)

### Context

The slog adoption pinned four levels (`Debug`, `Info`, `Warn`, `Error`) but didn't specify what events go where. Without a contract, "log everywhere" devolves into noise — DEBUG and INFO blur, WARN gets used for events that should be ERROR or vice versa, and `PORTAL_LOG_LEVEL` loses meaning because changing it doesn't predictably change what you see.

The contract also has to match portal's production posture: the daemon swallows-and-continues on per-pane failures, so genuinely unrecoverable conditions are rare. ERROR sites that existed historically (e.g. the non-contention lock-acquire failure) were downgraded to WARN during the `slow-open-empty-previews-and-zombie-sessions` remediation precisely because the operational reality didn't match "unrecoverable".

### Options Considered

**Default level for production:**
- A. WARN (current). Only anomalies surface. ~50–200 KB/day. Quiet at the cost of forensic baseline.
- B. INFO (new). Steady-state production signal — bootstrap transitions, cycle summaries, lifecycle events. ~1–5 MB/day. Continuous forensic baseline. Trivial within the 30-day window (well under 500 MB cap).

**Level granularity:**
- Stay with slog's four standard levels (Debug/Info/Warn/Error).
- Custom sublevels (e.g. Trace under Debug, Notice between Info and Warn) for more granularity.

### Journey

The seed framing was "DEBUG aggressive dump; INFO steady-state when all good". That collapses to: DEBUG is bounded by code paths reached (not judgement); INFO is bounded by the rate of *meaningful* events. WARN is "every line is signal"; ERROR is "we're about to give up".

Default-level discussion: WARN-only is what we have today, and is exactly the posture that left no evidence on 2026-05-28 — no INFO-level breadcrumbs were emitted around the hooks.json wipe or the saver disappearance. The point of the broader observability initiative is to *have* a steady-state baseline to grep through; defaulting to WARN defeats that. INFO trades trivial disk for a continuous forensic record.

Custom sublevels were considered briefly and rejected — slog's four is the standard; introducing Trace/Notice creates a contract drift the rest of the codebase has to learn. Keep it simple.

ERROR usage: the prior downgrade of non-contention lock-failure from ERROR to WARN reflects portal's reality. ERROR is for "we're exiting because we cannot continue" — the lock-contention case is exactly that (the loser daemon exits because it lost the lock). Most other "errors" in portal are recoverable and warrant WARN.

### Decision

**Locked: slog four-level (Debug/Info/Warn/Error) with the semantic contract below; production default `PORTAL_LOG_LEVEL=info`.**

| Level | Purpose | Volume |
|---|---|---|
| **DEBUG** | Kitchen sink for reconstruction: breadcrumbs on swallowed-error paths, per-event state changes under cycle summaries, observed transient values, decision-point inputs. Off in production. | Bound by code paths reached, not judgement. |
| **INFO** | Decision/terminal-point summaries, cycle summaries, lifecycle events. One line per *meaningful choice*, not per state change. The steady-state production signal. | Bound by rate of meaningful events (~1–5 MB/day). |
| **WARN** | Unexpected-but-recoverable conditions. Per-session capture anomalies, retries triggered, transient probe failures inside hysteresis, invalid config falling back to default. | Every line is a signal worth looking at. |
| **ERROR** | Genuinely unrecoverable — process is about to exit because it cannot continue. Rare in portal due to swallow-and-continue posture. | Few sites total; most candidates warrant WARN instead. |

Two implications carry forward:
1. **Subsystem prefix taxonomy** (next subtopic) must be designed so `grep` on a prefix produces a useful trace at INFO level — INFO is the production baseline that has to be greppable.
2. **State-mutation audit trail** breadcrumbs are INFO (decision points), not DEBUG — they need to survive at production level.

---

## Subsystem prefix taxonomy

### Context

The grep affordance is what makes the INFO baseline forensically useful. Without a stable, predictable prefix shape, `grep "<subsystem>:" portal.log` doesn't produce a clean per-subsystem audit trail — it produces a mix of subsystem mentions in arbitrary log lines.

The inbox framing was: `grep 'hydrate:' portal.log` should reconstruct exactly which pane took which exit path on every helper invocation. That intuitive idiom — `<component>:` as a literal at the start of the human-readable message — is the design target.

Under slog, the natural model is a structured `component` attr that handlers render however they choose. The mechanism decision below preserves both the literal `component:` grep idiom for tail-and-eyeball use AND the structured attr for programmatic filtering and JSON output.

The subtopic scope (per review-002 G11) absorbs two adjacent concerns: the canonical attr-key vocabulary across call sites (G3), and the mandatory baseline attrs every log line should carry (G4). These are part of the same "what does a portal log line look like" decision and shouldn't be re-litigated when the first call site lands.

### Options Considered (rendering mechanism)

**A. Standard slog `TextHandler`.** `... component=hydrate path=/x/y took=1.2s`. Standard, predictable, JSON-friendly. Grep: `component=hydrate`. Noisier when scanning by eye.

**B. Custom `slog.Handler` rendering `component` as a literal `<component>:` prefix at the start of the message body.** `... INFO hydrate: ok paneKey=foo:0.0 took=1.2s`. Matches the inbox grep idiom; preserves `component` as a structured attr under the hood (JSON output still works). One small custom handler — and we already need a custom handler for the rotation machinery, so the cost is essentially zero.

**C. Manual `<component>:` prefix in the message text only.** Simplest but loses the structured attr — `component` is buried inside `msg` and programmatic filtering / JSON output regress.

### Decision (partial — mechanism locked, taxonomy and attr vocabulary still open)

**Locked: Option B — `component` is a structured slog attr at every call site; the custom `slog.Handler` renders text output with `component:` as a literal prefix at the start of the message body and the timestamp + level preceding it.**

Example text-mode line:
```
2026-05-29T08:38:00Z INFO hydrate: ok pane_key=foo:0.0 took=1.2s
```

Example JSON-mode line (same call site, different handler):
```json
{"time":"2026-05-29T08:38:00Z","level":"INFO","component":"hydrate","msg":"ok","pane_key":"foo:0.0","took":"1.2s"}
```

Grep idiom preserved: `grep "hydrate:" portal.log` produces the per-subsystem audit trail. Programmatic filtering also works: handlers can route by `component` attr, JSON tooling can index it.

**Locked: call-site shape — factory pattern.**

A central `internal/log` package owns the shared `*slog.Logger` (the root, configured at process start with our custom handler for rotation, prefix render, and baseline attrs). It exposes one factory function:

```go
func For(component string) *slog.Logger
```

Each consumer package binds its component name once, at init:

```go
package state // package: internal/state

import "github.com/leeovery/portal/internal/log"

var logger = log.For("daemon")
```

Call sites then use the package-level logger with no component argument repeated:

```go
logger.Info("tick complete", "panes", 12, "took", "18ms")
// renders: 2026-05-29T08:38:00Z INFO daemon: tick complete panes=12 took=18ms
```

The factory returns a thin child wrapper around the root via `root.With("component", component)` — cheap, no shared-state surface. Test code injects a silent logger via existing DI seams (`slog.New(slog.NewTextHandler(io.Discard, nil))`) or constructs its own via `log.For("test")`.

**Existing `Component*` constants (`internal/state/logger.go:30-38`) are deleted as part of the migration sweep.** The factory's string argument is the only place a component name appears in Go code, so the typo surface is the ~12 package-init call sites — easy to review by eye. CLAUDE.md gets updated at lock time to reflect the new shape. (Closes review-002 G6.)

**Locked: component taxonomy (12 components, kebab-case where multi-word).**

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

`tmux` deliberately excluded — `internal/tmux` is a thin wrapper; logging at that layer produces extreme volume per tmux call. Better as DEBUG breadcrumbs under the caller's component (e.g. `daemon` logs "tmux command failed: …").

**Locked: attr-key vocabulary (snake_case, slog/JSON ecosystem convention).**

| Key | What |
|---|---|
| `pane_key` | Structural pane key (canonical, persisted form like `session__window.pane`) |
| `tmux_pane` | `$TMUX_PANE` env var (live tmux pane id like `%42`) |
| `session` | tmux session name |
| `project` | project name from `projects.json` |
| `path` | filesystem path |
| `took` | duration (`time.Duration` rendered) |
| `error` | error message (slog idiom: `slog.Any("error", err)`) |
| `error_class` | for swallowed errors: `expected` / `unexpected` |
| `hook_key` | hooks subsystem — the structural hook key |
| `op` | state-mutation breadcrumbs — `set` / `rm` / `clean-stale` |

Conventions:
- **snake_case** for all attr keys.
- **Message string is a terse phrase**, data lives in attrs: `logger.Info("ok", "pane_key", k, "took", d)` — NOT `logger.Info(fmt.Sprintf(...))`.
- **Sticky context via `.With(...)`** when multiple events share context: `paneLogger := logger.With("pane_key", k); paneLogger.Info("scrollback replayed"); paneLogger.Info("hook fired")`.

The vocabulary is the closed set today; new keys are added by amendment in spec/follow-on PRs when a use case appears. The point is *no ad-hoc invention at call-site time* — every contributor consults the list.

**Locked: mandatory baseline attrs (every line carries these, root-injected once).**

| Key | Set where |
|---|---|
| `component` | Per-package via `log.For("...")` |
| `pid` | Root logger construction (`os.Getpid()`) |
| `version` | Root logger construction (build-time `cmd.version`) |
| `process_role` | Root logger construction — one of `daemon` / `bootstrap` / `hydrate` / `hooks_cli` / `tui` / `clean`. Identifies which portal binary invocation emitted the line. Critical for multi-writer disambiguation on reboot-recovery days. |

Example full line (INFO mode, text handler):
```
2026-05-29T08:38:00Z INFO hydrate: ok pane_key=foo:0.0 took=1.2s pid=12345 version=0.5.0 process_role=hydrate
```

Baseline attrs add ~50 bytes per line — negligible at INFO steady-state (~3 MB/day total). They make every line self-describing for forensic use across multi-writer days. (Closes review-002 G3 and G4.)

---

## Summary

### Key Insights

1. The 2026-05-28 evidence loss is rotation-churn (1 MiB threshold + single `.old` overwritten on each rotation), not literal truncation. Reframes the inbox premise.
2. Realistic per-day log sizing makes calendar-daily rotation the right primary boundary; size cap is a disk-fill safety valve, not the main mechanism.
3. Silent destruction (no log line on retention deletion or rotation) was the actual incident-multiplier. Every destructive action must emit a breadcrumb.
4. State-mutation breadcrumbs need to cover *internal* mutations too (e.g. `hookStore.CleanStale`), not just user-CLI mutations — the bash hook log can only see user-driven calls.
5. The scope is broader than the inbox's seven patterns — it's "use logging anywhere it aids debugging or insight under a disciplined level taxonomy". That makes level discipline and prefix taxonomy foundational, not auxiliary.
6. The foundation is `log/slog` (Go 1.21 standard library). Structured fields, handler abstraction, standard-library posture all compose better than extending the existing printf wrapper.
7. Default production level shifts from WARN to INFO. WARN-only is exactly the posture that left no evidence on 2026-05-28; a continuous INFO-level baseline costs ~5 MB/day and gives the forensic record the whole initiative is about.

### Open Threads

- **Current `portal.log` zeroing bug** — no `.old` exists, no `O_TRUNC` in `logger.go`, so the destruction mechanism is currently unidentified. Not logged as a separate inbox bug — likely surfaced or resolved as a side effect of the rotation rewrite; investigate during implementation.
- **Hook command privacy** — verbatim vs SHA-256 hash prefix vs truncation. To resolve when state-mutation audit trail subtopic is explored.

### Considered and Rejected / Closed by Prior Decisions

Documenting review-set 001 finding resolutions so future-us knows omissions were deliberate, not oversights.

- **F1** (rotation when daemon down at midnight boundary) — closed by library-encapsulated date-aware open. Daemon presence is no longer load-bearing for boundary crossing.
- **F2** (multi-writer concurrency at rotation boundary) — closed by same library-encapsulated decision; `O_CREAT|O_EXCL` handles cross-process create race.
- **F7** (out-of-band rotation audit channel) — considered and rejected. Locked invariants (rotated-file immutability + per-process startup markers + `O_EXCL` on today's file creation) provide sufficient post-hoc detectability for portal's scale. Cost of a second log file with divergent rotation policy outweighs benefit for a single-user dev tool.
- **F3, F4, F5, F6, F8, F9, F11, F13** (rotation/retention operational edges: timezone/DST, first-startup migration, disk-full/EACCES, retention scheduling and missed-day catchup, version-upgrade boundary, `.N` ordering, open-fd-after-unlink, rotation-INFO placement) — captured in the locked rotation/retention Decision sections as spec-phase work.
- **F14** (subsystem prefix taxonomy sequencing) — closed by scope-expansion call to promote level discipline and prefix taxonomy to foundational subtopics ahead of further pattern decisions.
- **F10** (investigation gate for the unknown zeroing bug) — closed. Ship the rewrite without blocking on root-cause understanding. If destruction recurs in the new system with no clear cause within 30 days post-ship, file a separate investigation bug; startup-marker tripwires will provide concrete evidence. Until then, treat the original bug as resolved-by-rewrite or detectable-when-it-recurs.
- **F12** (compress rotated logs) — considered and rejected. Worst-case 30-day window at ~600 MB uncompressed is already trivial; introducing `zgrep` as a precondition for searching anything older than today adds friction at exactly the moment the user is investigating an incident. Greppability outweighs disk savings at portal's scale.

### Current State

- Five subtopics decided: Logger library (slog), Log rotation mechanism (Option C, library-encapsulated, 500 MB default), Retention policy and audit (30d default), Log-level discipline (slog four-level contract, INFO production default), Subsystem prefix taxonomy (Option B rendering + factory pattern + 12-component taxonomy + snake_case attr vocab + 4 baseline attrs).
- Scope expansion confirmed: instrument the whole codebase wherever logging would aid debugging/insight, under the disciplined level contract. Inbox's seven patterns remain the minimum scope.
- Review-set 001 fully drained: 14 findings closed.
- Review-set 002 in flight: G6 + G11 closed by factory-pattern decision; G3 + G4 closed by attr-vocab + baseline-attrs lock. Remaining: G1, G2, G5, G7, G8, G9, G10 — most pertain to migration mechanics and level-discipline edge cases.
- Remaining map: 9 pending subtopics on defensive invariants, state-mutation audit trail, patterns, lifecycle events, and rollout.
