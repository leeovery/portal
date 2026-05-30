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
  Call-site logging pattern (DEBUG breadcrumbs + INFO terminal) [decided]
  State-mutation audit trail for user config files [decided]
  Defensive invariants against unknown-cause log destruction [decided]
  Diagnostic context preservation at boundaries [decided]
  Cycle-level summary cadence and shape [decided]
  Log-level propagation verification [decided]
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

**Mechanical rule (spec-phase intake):**

For each `Handle(record)` call into the custom `internal/log` `slog.Handler`:

1. Compute `today := time.Now().Format("2006-01-02")`.
2. If the handler's currently-open fd is for `portal.log.<today>`, reuse it. Otherwise:
   a. Open `${stateDir}/portal.log.<today>` with `O_CREAT|O_EXCL|O_APPEND|O_WRONLY`, mode `0600`.
   b. On `EEXIST`, retry with `O_APPEND|O_WRONLY` (lost the cross-process create race; another writer beat us).
   c. On either path: swing the symlink `${stateDir}/portal.log → portal.log.<today>` atomically via `os.Symlink(target, tmp)` + `os.Rename(tmp, link)`.
   d. `chmod 0400` any other `portal.log.<date>*` files in `${stateDir}` that are not `<today>` and not already mode 0400.
3. After fd is open, check `current_size + len(serialized) >= PORTAL_LOG_ROTATE_SIZE` (parsed once at handler init from env var, default `500*1024*1024`). If true, rotate to `portal.log.<today>.N`:
   a. Find max existing `N` for today (`portal.log.<today>.*` listing); next N = max + 1, or 1 if none.
   b. Open `portal.log.<today>.<N>` with `O_CREAT|O_EXCL|O_APPEND|O_WRONLY`.
   c. On `EEXIST`, retry with `N+1`.
   d. Swing the symlink to the new file. `chmod 0400` the previous file.
4. Write the serialized record to the now-current fd.

The above applies to ONE seam: the `slog.Handler` in `internal/log`. No call site outside that package implements rotation logic.

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

**Mechanical rule (spec-phase intake):**

On the first `Handle(record)` call of each calendar date (detected via the date-change check from the rotation rule above), the handler runs a retention sweep:

1. Parse `cutoff := today.AddDate(0, 0, -PORTAL_LOG_RETENTION_DAYS)`. Default `PORTAL_LOG_RETENTION_DAYS = 30`. Invalid env value (non-integer, negative, > 365) → use default and emit one WARN: `log-rotate: invalid PORTAL_LOG_RETENTION_DAYS=<v>, using 30d`.
2. List `${stateDir}/portal.log.*` files. Extract the date portion of each filename. For each file whose date < `cutoff`:
   a. Emit one INFO line BEFORE deletion: `log-rotate: deleted path=<filename> retention=<N>d`.
   b. `os.Remove(filename)`. On error, emit one WARN with `error` attr and continue (don't abort the sweep).
3. The sweep is best-effort and re-entrant; running it twice on the same day is a no-op.

`portal clean` does NOT trigger this sweep on its own; it preserves rotated logs by default. `portal clean --logs` triggers the sweep with `cutoff = today` (i.e. delete every rotated file, leaving only the current one).

The above applies to ONE seam: the `slog.Handler` in `internal/log` (and `portal clean --logs`, which calls into the same sweep function).

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

**Mechanical rule (spec-phase intake):**

A new package `internal/log` exposes exactly two public functions:

```go
package log

// Init constructs the root *slog.Logger with baseline attrs bound and the
// custom rotating handler attached. Called once per process from main.go
// before any other portal code runs. Idempotent only via panic-on-second-call.
func Init(stateDir string, version string, processRole string) error

// For returns a child logger pre-bound to the given component name. Cheap;
// callers are expected to cache the returned logger at package init via a
// package-level var.
func For(component string) *slog.Logger
```

Every other portal package that needs to log:

```go
import "github.com/leeovery/portal/internal/log"

var logger = log.For("<component-name-from-locked-taxonomy>")
```

Then call sites use `logger.{Debug,Info,Warn,Error}` directly with `slog.Attr` args (`"key", value` variadic form).

Migration sweep:
- The `internal/state.Logger` type is deleted.
- The `internal/state.Component*` constants are deleted.
- All call sites of `state.Logger.{Debug,Info,Warn,Error}(component, fmt, args...)` are rewritten to `logger.{Debug,Info,Warn,Error}(msg, attrs...)` with the component bound at package-init via `log.For`.
- `state.NopLogger()` is deleted; tests requiring a silent logger use `slog.New(slog.NewTextHandler(io.Discard, nil))` directly.
- All test mock surfaces in `bootstrapDeps` and friends that previously held `*state.Logger` are updated to hold `*slog.Logger`.

Big-bang in one PR (no adapter shim, no co-existence period).

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

### Placement clarifications (refines contract per review-002 G1)

- **Idempotent no-ops** (e.g. `RegisterPortalHooks` deciding "already current, no action"): DEBUG by default. INFO only when the no-op IS the user-visible decision (e.g. "saver already at version V, skipping respawn" — the operator wants to see we considered upgrading and chose not to act). A purely-internal idempotent skip clutters the INFO baseline.
- **Hysteresis-internal failures** (e.g. saver-membership probe failure inside the 3-tick self-supervision hysteresis): DEBUG per spurious tick. ONE INFO or WARN on the trip (when the threshold is crossed and the eject decision lands). WARN per tick during transient tmux contention would fire continuously and invert the "every WARN is signal" promise.
- **Recoverable-but-rare** (e.g. corrupt `sessions.json` falling back to empty state; pane decode failures dropping one pane and continuing): WARN. These are signal even when recovered. "Rare" doesn't bump them to ERROR — ERROR is strictly "process exiting because it cannot continue".

### Mechanical rule (spec-phase intake)

Level selection per call site is determined mechanically by the code shape. Spec writers and code reviewers apply this table without judgment:

| Code shape at the call site | Level |
|---|---|
| `_ = expectedErr` / `if errors.Is(err, KnownExpected) { return }` swallow path | `Debug` with `error_class="expected"` |
| `_ = unexpectedErr` / `log-and-continue` on an error the codebase does not expect on the happy path | `Debug` with `error_class="unexpected"`, OR `Warn` if production visibility is wanted |
| Terminal line just before successful return from a function representing a meaningful choice (saver respawn, hook fire, capture-tick complete, lifecycle transition) | `Info` |
| Cycle-end summary at the end of a tick/iteration/batch (`tick complete`, `bootstrap step done`, `clean-stale entries=N`) | `Info` |
| Idempotent no-op decision (key already correct, version already current, hook already registered) | `Debug` UNLESS the no-op is the user-visible decision (e.g. "skipping respawn because version matches"), in which case `Info` |
| Probe failure inside a hysteresis window (`probe failed, retries=N/M`) | `Debug` per failure |
| Hysteresis threshold trip (escalation, eject, give-up) | `Info` (the resolved decision) OR `Warn` if the trip represents an anomaly |
| Unexpected-but-recoverable condition (anomalous capture, fallback to default after invalid config, retry triggered) | `Warn` |
| Line immediately preceding `os.Exit(N)` / `return err` from `main` / panic | `Error` |

Default `PORTAL_LOG_LEVEL = info`. Invalid env value (any value that is not exactly `debug` / `info` / `warn` / `error` after lowercase) → fall back to `info` and emit one WARN at process start: `bootstrap: invalid PORTAL_LOG_LEVEL=<v>, using info`.

Spec writers MUST verify each new log call site against this table during spec authoring. Code reviewers verify the same at PR review time.

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

### Mechanical rule (spec-phase intake)

**Closed component value space** (15 total — 12 original + 2 added by State-mutation audit trail + 1 added by Defensive invariants):

```
bootstrap  daemon  restore  hydrate  notify  hooks  preview
saver  capture  signal  log-rotate  clean  aliases  projects
process
```

`process` is reserved for portal-binary lifecycle markers (start, exit, panic) only; subsystem-level lifecycle events have their own components.

**Closed attr-key value space** (10 contextual + 11 cycle-summary + 4 baseline):

Contextual (set per call as relevant): `pane_key`, `tmux_pane`, `session`, `project`, `path`, `took`, `error`, `error_class`, `hook_key`, `op`.

Cycle-summary (set per summary line as relevant; enumerated by the Cycle-level summary subtopic): `sessions`, `panes`, `entries`, `steps`, `warnings`, `natural_churn`, `anomalous`, `reaped`, `killed`, `unset`, `entries_failed`.

Baseline (auto-injected at root logger construction in `internal/log.Init`): `component` (set per package via `log.For`), `pid`, `version`, `process_role`.

**Custom `slog.Handler` text-mode rendering rule:**

```
<RFC3339Nano timestamp> <LEVEL> <component>: <msg> <attrs as key=value pairs>
```

Where:
- `<component>` is read from the bound `component` attr and emitted as a literal prefix immediately before the colon. The `component` attr is NOT also rendered in the attrs key=value list.
- `<msg>` is the slog record's message field.
- `<attrs>` are emitted as space-separated `key=value` pairs in the order: contextual attrs (in slog.Record order), then the four baselines (`pid`, `version`, `process_role`; `component` was already rendered as the prefix).
- Multi-word string values are quoted with `"`.
- `time.Duration` values render with Go's default `String()` (e.g. `1.234s`).

**Custom `slog.Handler` JSON-mode rendering rule:**

Standard `slog.NewJSONHandler` output, no special handling — `component` becomes a normal `"component":"<name>"` JSON field.

**Extension policy:**

- New components require explicit amendment of THIS discussion file's closed component list (or a successor specification amendment).
- New attr keys require the same amendment process.
- Spec writers and code reviewers MAY NOT introduce new component or attr names ad hoc.

---

## Call-site logging pattern (DEBUG breadcrumbs + INFO terminal)

This subtopic absorbs what was originally split between "Decision-point INFO line shape" and "DEBUG breadcrumb pattern on swallowed errors" — both are facets of the same call-site discipline.

### Context

The level discipline contract names DEBUG breadcrumbs and INFO terminal-point summaries but doesn't specify the call-site mechanics. Two questions follow: (1) does a function emit one log call that renders differently per level (some kind of "trace" abstraction), or multiple independent calls each at its chosen level? (2) what does the resulting log shape look like for a typical multi-step operation?

### Options Considered

**A. Multiple independent calls.** Each `logger.Debug(...)` and `logger.Info(...)` is a standalone call. Slog handles level filtering — drops calls below threshold cheaply. The function author explicitly chooses level per call.

**B. Wrapper that bundles levels.** A custom helper like `logger.Trace(msg, debugAttrs, infoAttrs)` that emits at one or both levels based on enabled state.

**C. OpenTelemetry-style span wrapper.** A `trace := logger.Span("hydrate.process"); defer trace.End()` pattern that records breadcrumbs as DEBUG events and emits the span-end as INFO.

### Journey

Option B couples two distinct concerns (the per-step breadcrumb and the terminal-point summary) into one API call, and hides level discipline from code review — a reviewer can't point at a line and say "this should be DEBUG, not INFO" because the level choice is buried in the wrapper logic. Rejected.

Option C is designed for distributed tracing systems where span hierarchies and correlation IDs matter across hosts. Portal is single-host, file-logged. The wrapper adds API surface and conceptual load for benefits we don't need. Rejected.

Option A is Go's `log/slog` idiom — independent calls, lazy formatting, near-zero cost for filtered-out calls. Working with the grain of the standard library means future contributors don't have to learn anything portal-specific.

### Decision

**Locked: Option A — multiple independent log calls per function, slog handles level filtering.**

Canonical call-site pattern for a multi-step operation:

```go
package hydrate

import "github.com/leeovery/portal/internal/log"

var logger = log.For("hydrate")

func Process(paneKey, fifoPath string) error {
    log := logger.With("pane_key", paneKey)  // sticky context bound once
    start := time.Now()

    log.Debug("opening fifo", "path", fifoPath)
    fd, err := openFifo(fifoPath)
    if err != nil {
        return err
    }
    log.Debug("fifo opened", "fd", fd, "took", time.Since(start))

    if err := awaitSignal(fd); err != nil {
        return err
    }
    log.Debug("signal received", "took", time.Since(start))

    n, err := replayScrollback(...)
    if err != nil {
        return err
    }
    log.Debug("scrollback replayed", "bytes", n)

    log.Info("ok", "took", time.Since(start))  // terminal-point summary
    return nil
}
```

At INFO (production): one INFO line per invocation lands in portal.log.
At DEBUG (investigating): all four DEBUG lines + the INFO summary.

**Allowed ergonomic helpers:**

1. **`.With(...)` for sticky context** — bind shared attrs once when a function/scope has many log calls sharing context (e.g. `pane_key`, `session`). Stops attr-key repetition.
2. **`logger.Enabled(ctx, slog.LevelDebug)` guard** — only for the rare case where computing the attrs is itself expensive (e.g. JSON-marshalling something just to attach as a debug attr). Slog's lazy formatting makes this irrelevant 99% of the time.
3. **Shared helpers in `internal/log`** — only after the same idiom appears 5+ times in production code and earns its weight. Don't pre-build helpers for theoretical cases.

**Anti-pattern (explicitly rejected):** custom wrappers like `logger.Trace(LEVEL_BREADCRUMB, LEVEL_TERMINAL, msg, debugAttrs, infoAttrs)` that bundle levels into one call. Couples concerns, obscures level discipline at the call site, makes review harder. Each log call has ONE level chosen explicitly.

### Mechanical rule (spec-phase intake)

Per function authored or amended, spec writers and code reviewers apply this discipline mechanically:

1. **DEBUG breadcrumbs** at each meaningful state transition inside the function (resource acquired, event received, sub-operation completed, branch chosen). One `logger.Debug(<terse-msg>, <attrs>...)` per transition.
2. **INFO at terminal decision points** — one `logger.Info(<terse-msg>, <attrs>...)` immediately before each successful return path. The line MUST capture the chosen outcome and the resolved decision attrs.
3. **WARN per recoverable error path** — one `logger.Warn(<terse-msg>, "error", err, <attrs>...)` before swallowing/returning. WARN exists at any code path that classifies as "unexpected-but-recoverable" per the level discipline table.
4. **ERROR only at lines immediately preceding** `os.Exit(N)` / `panic(...)` / `return err` from a main entry point.

**Sticky-context rule:** when ≥ 3 subsequent log calls in the same lexical scope share an attr, bind it once via `local := logger.With(<attrs>...)` and use `local.<Level>(...)` thereafter. Below the 3-call threshold, repeat attrs at each call.

**Expensive-attr guard:** wrap with `if logger.Enabled(ctx, slog.LevelDebug) { ... }` ONLY when computing an attr value involves measurable cost (JSON marshalling, slice formatting > 100 elements, syscall to read state). For ordinary attrs (strings, ints, durations, pre-computed values), use slog's lazy formatting directly.

**Prohibited (PR review must reject):**
- Custom helpers that bundle multiple levels into one call (e.g. `Trace(msg, debugAttrs, infoAttrs)`).
- `fmt.Sprintf` inside log message strings to embed values that should be attrs (`logger.Info(fmt.Sprintf("ok %s", k))` — wrong; use `logger.Info("ok", "key", k)`).
- Direct construction of `*slog.Logger` outside `internal/log` package.
- Pre-formatting attrs into the message string (use slog attrs instead).
- Using attr keys not in the closed value space (extend the vocabulary first via discussion-file amendment).

---

## State-mutation audit trail for user config files

### Context

Direct answer to the 2026-05-28 incident's "I have no record of who wiped `hooks.json`" gap. Every mutation of a portal-owned user-config file must leave a breadcrumb in `portal.log` so the next time a file changes unexpectedly, `grep "<file>:" portal.log` reconstructs the change history.

The subtopic's *design intent* is straightforward — log mutations. The *mechanical rule* below is what spec phase ingests to produce the per-call-site enumeration with zero implementation-time judgment. This is the first locked subtopic written with that explicitness target; the existing five will be retro-sharpened in the same shape.

### Decision

**Locked.**

**Files in scope (closed set):**
- `hooks.json`
- `aliases`
- `projects.json`

`sessions.json` is **out of scope** for this subtopic — it's daemon-managed high-frequency state (mutated every tick), covered by the pending cycle-summary subtopic (one INFO per tick under `daemon` / `capture` components, not per-write).

**Taxonomy addition:** `aliases` and `projects` are added as components. Total taxonomy: **14 components**.

**Mechanical rule (the seam spec phase enumerates against):**

> *Every call to `internal/fileutil.AtomicWrite` (or any successor primitive that performs a temp-file + rename of a portal-owned config file) whose target path matches one of `hooks.json` / `aliases` / `projects.json` emits, immediately after `AtomicWrite` returns:*
>
> - *On `error == nil`: ONE INFO log line.*
> - *On `error != nil`: ONE WARN log line.*
>
> *The log line's component (prefix) is the file's owning component: `hooks` for `hooks.json`, `aliases` for `aliases`, `projects` for `projects.json`.*
>
> *Required attrs:*
> - `op` — drawn from the closed value space below.
> - Key identifying the affected entry: `hook_key` (hooks), `alias` (aliases), `project` (projects).
> - On failure (WARN path): `error_class` from the closed AtomicWrite failure space below.
>
> *Optional attrs:*
> - `value` — verbatim new value for `set` / `modify`; absent for `rm` / `clean-stale`.
> - `via` — `cli` for user-facing commands, `internal` for code-driven mutations (e.g. `CleanStale`), `migrate` for the one-shot `migrateConfigFile` path.

**Closed `op` value space:**

| `op` value | Meaning |
|---|---|
| `set` | Create new entry (key did not exist before this write) |
| `modify` | Update existing entry (key existed; value differs) |
| `rm` | Remove existing entry |
| `clean-stale` | Internal cleanup of an entry (always batched) |
| `migrate` | One-shot migration from old config path (e.g. `~/Library/Application Support/portal/`) |

**Closed `error_class` value space for AtomicWrite failures (per phase):**

`write-failed-temp-create` / `write-failed-write` / `write-failed-fsync` / `write-failed-rename`

**No-op handling:** A `set` call where the entry already exists and the value matches → DEBUG with `op=set-noop`. NOT INFO. Matches the level-discipline placement clarification for idempotent no-ops.

**Batch operations (e.g. `CleanStale` iterating entries):**

> *Every batch loop that mutates multiple entries emits:*
> - *Per-entry DEBUG inside the loop.*
> - *ONE INFO summary at the end of the batch with attrs `op=<batch-op>`, `entries=N`, and `entries_failed=M` if any per-entry failures occurred.*
> - *Per-entry WARN with `error_class=unexpected` on per-entry failure mid-loop (regardless of whether the batch continues).*

This applies the hysteresis-trip pattern (from level discipline) to mutation batches: detail at DEBUG, summary at INFO, exceptions at WARN. Replaces the per-entry-INFO proposal that contradicted that pattern.

**Privacy posture: verbatim.** Hook commands, alias values, project paths logged as-is. Threat model accepted: portal is a single-user dev tool; `portal.log` lives on the same disk as the config files which already store these values plaintext. Users sharing logs in bug reports redact manually (same posture as `hooks.json` itself).

**Closes review-003 H1, H2, H3, H4, H5, H7.** Defers H6, H9, H10 to spec phase (boundary-rule formalization, aliases read-side scope, source-distinguishability per call site).

---

## Defensive invariants against unknown-cause log destruction

### Context

The locked rotation/retention machinery is robust against the known destruction mechanism (rotation churn at the old 1 MiB threshold). The 2026-05-28 incident also suggested an unidentified zeroing path exists somewhere — currently deferred as Open Thread for investigation during implementation.

Even with the new design, an unknown bug could destroy today's `portal.log`. The defense-in-depth invariants here make destruction **detectable post-hoc** so an operator who later looks at the log can tell that destruction happened and roughly when, even if the cause remains unknown.

### Decision

**Locked.** Three invariants. Two are already enforced inside the rotation subtopic — re-stated here for completeness; their authoritative rule lives in the rotation mechanical rule above. The third is new to this subtopic.

**Invariant 1: Rotated-file immutability.** (Already locked in Log rotation mechanism.) Files older than today are `chmod 0400` so even a buggy library can't overwrite past evidence. The destruction surface narrows to today's file only.

**Invariant 2: `O_CREAT|O_EXCL` on first-of-day open.** (Already locked in Log rotation mechanism.) Every process's first write of a day creates `portal.log.<today>` via `O_CREAT|O_EXCL` (with append-fallback on `EEXIST`). If something deletes today's file mid-day, the next writer's create-or-append races safely and observably.

**Invariant 3 (new this subtopic): Per-process lifecycle markers.** Every portal process emits ONE INFO line at the very start of its execution and ONE INFO line on clean exit. These are the tripwires that make destruction visible: if today's `portal.log` contains only lines from 09:15 forward but you know portal was running before, the first line of today's file is a "process: start" marker that timestamps when destruction had to have happened.

**Taxonomy addition:** add `process` as a 15th component. Used exclusively for lifecycle markers (start, exit, panic) for the portal binary itself, regardless of which subcommand is running. (Subsystem-level lifecycle events — saver respawn, daemon spawn — go under the pending Saver/daemon lifecycle subtopic, not under `process`.)

### Mechanical rule (spec-phase intake)

The `internal/log.Init(stateDir, version, processRole)` function, after constructing the root logger and wiring the rotating handler, MUST emit exactly one INFO line as its final action before returning:

```go
log.For("process").Info("start",
    "cmd", filepath.Base(os.Args[0]),
    "args", strings.Join(os.Args[1:], " "),
)
```

Renders (text mode):
```
2026-05-30T14:00:00Z INFO process: start cmd=portal args="open ." pid=12345 version=0.5.0 process_role=tui
```

(Note: `pid`, `version`, `process_role` are baseline attrs auto-injected; the call site does not pass them.)

The `internal/log` package additionally exposes `func Close(exitCode int)` to be invoked from `main()` via deferred call OR explicit invocation immediately before `os.Exit(N)`. `Close` emits one INFO line:

```go
log.For("process").Info("exit",
    "code", exitCode,
    "took", time.Since(startTime),
)
```

Renders:
```
2026-05-30T14:00:02Z INFO process: exit code=0 took=2.1s pid=12345 version=0.5.0 process_role=tui
```

`startTime` is captured at `Init` time and stored package-private.

**Coverage requirement:** every binary entry point (currently only `main.go`, but extending in principle to any future entry binary) must:
1. Call `log.Init` BEFORE any other portal code that might log.
2. Either defer `log.Close(0)` (for normal-return paths) or invoke `log.Close(N)` explicitly before any `os.Exit(N)`.

**Panic path:** `Init` MUST also install a `defer func() { if r := recover(); r != nil { ... } }()` in main if practical, emitting `process: panic reason=<r>` at ERROR before re-panicking. Implementation detail deferred to spec.

**Privacy on `args` attr:** verbatim. Same posture as state-mutation audit trail. CLI commands like `portal hooks set --on-resume "claude --resume X"` will have the full args string in `portal.log`. Acceptable for portal's single-user threat model.

---

## Diagnostic context preservation at boundaries

### Context

Pattern (4) from the inbox: when a subprocess fails or an external command returns unexpected output, capture stderr alongside stdout and propagate both into the wrapped error. Discarding stderr is the most common form of "we lost the debug context exactly where we needed it most." Same principle applies to syscalls (errno text) and tmux command failures.

The cycle-1 review of `slow-open-empty-previews-and-zombie-sessions` surfaced `defaultIdentifyPS` (`internal/state/daemon_identity.go`) as a concrete example: stderr was discarded on failure, leaving the wrapped error context-free.

This subtopic governs **error wrapping at external boundaries**. It is orthogonal to the level discipline (which governs how the resulting wrapped error is logged). Together they guarantee: when an external call fails, the failure context survives all the way to the log line.

### Decision

**Locked.** Every external-boundary call site MUST preserve stderr / errno / phase-of-failure context in the wrapped error returned to callers. The locked level discipline + call-site pattern then determines how that error reaches `portal.log`.

### Mechanical rule (spec-phase intake)

Four boundary classes; each has a concrete wrapping pattern.

**Boundary class 1: `exec.Cmd`** (currently used in `internal/state/daemon_identity.go`, `internal/tmux/commander.go`, etc.).

Every call site MUST either use `cmd.CombinedOutput()` OR capture `cmd.Stderr` into a `bytes.Buffer` before `cmd.Run/Output`. On error, the wrapped error MUST embed both:
- The exit status (or signal) if the process exited.
- The stderr text (trimmed).

Canonical wrapping pattern:

```go
var stderr bytes.Buffer
cmd.Stderr = &stderr
out, err := cmd.Output()
if err != nil {
    return nil, fmt.Errorf("%s %v: %w (stderr: %s)",
        cmd.Path,
        cmd.Args[1:],
        err,
        strings.TrimSpace(stderr.String()),
    )
}
```

PROHIBITED: `_, _ = cmd.Run()`, `cmd.Output()` without `cmd.Stderr` assignment, or any error path that returns a wrapped error WITHOUT stderr text included.

**Boundary class 2: `internal/tmux.RealCommander.Run` / `RunRaw`** (the wrapper layer for all tmux command execution).

The commander interface MUST capture both stdout and stderr on every invocation. On non-zero exit:
- The error returned MUST embed the exit code, the tmux argv, and the trimmed stderr text.
- Tmux-specific sentinel errors (`ErrNoSuchSession`, `ErrEmptyPaneList` per `internal/tmuxerr`) MUST be detected via the stderr text and wrapped with the sentinel using `fmt.Errorf("%w: %s", sentinel, stderr)`.

PROHIBITED: returning a generic error from a tmux invocation without the stderr context.

**Boundary class 3: `os` package syscalls** (`os.Stat`, `os.OpenFile`, `os.Rename`, `os.Remove`, etc.).

Go's standard `os` package wraps syscall errors with path + errno text by default (e.g. `open /tmp/x: permission denied`). The rule: do NOT replace these errors with a wrapper that loses the path/errno context. When additional context is wrapped on top, use `fmt.Errorf("...: %w", ..., err)` so the underlying error is preserved verbatim and accessible via `errors.Unwrap`.

PROHIBITED: `return errors.New("file operation failed")` style wrapping that discards the original `*os.PathError`.

**Boundary class 4: stdlib `io` / `bufio` / scrollback FIFO reads** (`internal/state/scrollback.go`, hydrate helper FIFO reads).

EOF and timeout conditions are valid expected outcomes, not boundary failures — they take the "expected" classification in the level discipline contract. Other I/O errors (read error mid-stream, write error mid-write) wrap with `fmt.Errorf("read %s: %w", path, err)` to preserve path context.

**Across all four classes — `slog` attr usage at the eventual log site:**

```go
logger.Warn("tmux command failed", "error", err, "session", sessionName)
```

The `"error"` attr value MUST be the wrapped error directly (`err`, not `err.Error()`); slog handles serialization. Custom handler renders the full chain of wrapped messages including the stderr text.

**Boundary helper (allowed shared idiom, internal/log):**

After 3+ identical boundary-wrapping patterns appear in production code, a shared helper in `internal/log` MAY be added. Examples of allowed helpers:

```go
// CombinedOutputWithContext runs cmd and returns its stdout. On error,
// returns a wrapped error embedding exit status, argv, and trimmed stderr.
func CombinedOutputWithContext(cmd *exec.Cmd) ([]byte, error)
```

Until 3+ sites need it, write the wrapping at each call site directly.

---

## Cycle-level summary cadence and shape

### Context

Pattern (5) from the inbox: daemon ticks, capture loops, bootstrap sequences, orphan sweeps — every cycle emits a single INFO summary at completion so an operator can reconstruct what happened over a window without needing per-event lines. Per-event WARNs still fire on anomalies; the summary is the steady-state grep target.

The 2026-05-28 incident's reconstruction would have benefited from cycle summaries: a daemon tick that emitted `capture: tick complete sessions=3 panes=12 natural_churn=0 anomalous=0 took=18ms` once per second across the morning would have given a forensic timeline of when the saver disappeared, without scrolling through per-pane DEBUG breadcrumbs.

This subtopic governs **when a cycle emits a summary, what attrs it carries, and what falls below it (DEBUG breadcrumbs) vs alongside it (per-event WARNs)**.

### Decision

**Locked.** Every cycle in portal emits ONE INFO summary at completion, with per-item events emitted at DEBUG (steady state) or WARN (anomaly).

### Mechanical rule (spec-phase intake)

A "cycle" is a function or method whose body matches one of these shapes:

1. **Loop cycle** — a `for` loop iterating distinct items (sessions, panes, files, entries, orphans).
2. **Sequence cycle** — an orchestrator running discrete named steps (e.g. the 11-step bootstrap orchestrator, the two-phase restore engine).
3. **Tick cycle** — a periodic loop driven by a ticker (the daemon's 1Hz capture loop).

For every cycle in portal, the function/method MUST:

1. Capture `start := time.Now()` before the loop / sequence / tick body.
2. Track counts of items processed and per-item anomalies (failures that did not terminate the loop).
3. At the end of the cycle body (just before the function returns / the tick completes), emit exactly ONE INFO log line:

```go
logger.Info("<verb> complete",
    "<unit>", count,
    // additional counts for sub-categories if relevant, e.g.:
    "natural_churn", churnCount,
    "anomalous", anomCount,
    "took", time.Since(start),
)
```

Where:
- `<verb>` is the cycle's purpose phrase: `tick`, `sweep`, `step`, `phase`, `orchestration`, `replay`, etc.
- `<unit>` is the item being iterated: `sessions`, `panes`, `entries`, `orphans`, `steps`, `files`, etc.
- Additional counts (sub-categorisations) ride as attrs on the same summary line. Examples: `natural_churn` (sessions that ended cleanly mid-capture), `entries_failed` (per-item failures), `warnings`.
- `took` is always present.

**Per-item event level inside a cycle:**

- Per-item DEBUG breadcrumb ALWAYS for items where the per-item path is interesting (the capture loop's per-pane state, the bootstrap step's invocation, etc.). These flood at DEBUG and are silent at INFO — the summary is the INFO truth.
- Per-item WARN ONLY for items that fail anomalously (count goes into the summary's anomalous attr).

**Concrete cycle catalog (sites that this rule mandates a summary at):**

| Cycle | Owning component | Summary line shape |
|---|---|---|
| Daemon tick (1Hz capture + commit) | `capture` | `capture: tick complete sessions=N panes=N natural_churn=N anomalous=N took=T` |
| Bootstrap orchestration | `bootstrap` | `bootstrap: orchestration complete steps=11 warnings=N took=T` |
| Each bootstrap step | `bootstrap` | `bootstrap: step complete step=<StepName> took=T` |
| Restore phase A (skeleton) | `restore` | `restore: skeleton complete sessions=N windows=N panes=N took=T` |
| Restore phase B (geometry + replay) | `restore` | `restore: geometry complete panes=N took=T` |
| Orphan FIFO sweep | `clean` | `clean: orphan-fifo sweep complete reaped=N skipped=N took=T` |
| Orphan daemon sweep | `clean` | `clean: orphan-daemon sweep complete killed=N took=T` |
| Marker cleanup | `clean` | `clean: marker sweep complete unset=N took=T` |
| Hooks CleanStale | `hooks` | (already locked in State-mutation audit trail — same summary shape) |
| Retention sweep (rotated logs) | `log-rotate` | (already locked in Retention policy — same summary shape) |

**Closed attr extension (added to the prefix taxonomy attr vocabulary):** `sessions`, `panes`, `entries`, `steps`, `warnings`, `natural_churn`, `anomalous`, `reaped`, `killed`, `unset`, `entries_failed`. These were implicit in the locked attr vocab's "counts" but are explicitly enumerated here so spec writers don't invent names.

Spec writers consulting this rule will produce one INFO call site per cycle in the codebase, with the verb-phrase + counts + `took` triplet, matching the catalog shape.

---

## Log-level propagation verification

### Context

Pattern (7) from the inbox: `PORTAL_LOG_LEVEL` must actually take effect through the test → tmux server → respawn-pane'd daemon chain. Today this is implicit and fragile; integration tests can set `PORTAL_LOG_LEVEL=debug` and assume the spawned daemon process receives it, but no positive verification exists. If the env var fails to propagate (because tmux clears it on `respawn-pane`, or because a test harness forgets to pass it), DEBUG coverage silently degrades and the test still passes (just with less log output than expected).

The fix is a positive log-marker: every process emits one INFO line at start declaring the resolved level and its source (env / default / fallback). Tests assert on that line.

### Decision

**Locked.** Every portal process emits exactly one additional INFO line as part of its lifecycle init sequence, declaring the resolved log level and how it was resolved. Tests that depend on a specific log level for coverage assert on this line.

### Mechanical rule (spec-phase intake)

`internal/log.Init(stateDir, version, processRole)` MUST emit one INFO line immediately AFTER the `process: start` line (defined in Defensive invariants subtopic) and BEFORE returning:

```go
log.For("process").Info("log-level resolved",
    "resolved", resolvedLevelStr,
    "source", levelSource,
    "raw", rawEnvValue,
)
```

Where:
- `resolved` is one of `debug` / `info` / `warn` / `error` — the level slog will actually filter at.
- `source` is one of:
  - `env` — `PORTAL_LOG_LEVEL` was set to a valid value.
  - `default` — `PORTAL_LOG_LEVEL` was unset → `info`.
  - `fallback` — `PORTAL_LOG_LEVEL` was set to an invalid value → fell back to `info` (also emits the WARN defined in the Log-level discipline mechanical rule).
- `raw` is the raw env var value as observed (empty string if unset, the verbatim string if set — including invalid values).

Renders (text mode):
```
2026-05-30T14:00:00Z INFO process: log-level resolved resolved=debug source=env raw="DEBUG" pid=12345 version=0.5.0 process_role=daemon
2026-05-30T14:00:00Z INFO process: log-level resolved resolved=info source=default raw="" pid=12345 version=0.5.0 process_role=tui
2026-05-30T14:00:00Z INFO process: log-level resolved resolved=info source=fallback raw="trace" pid=12345 version=0.5.0 process_role=daemon
```

**Test assertion contract:** any integration test that sets `PORTAL_LOG_LEVEL` MUST scan `portal.log` for the `process: log-level resolved resolved=<expected> source=env` line for the spawned process (matched by `pid` attr if multiple processes were involved). If the line is absent or `source` is not `env`, the test fails — the env var did not propagate.

A canonical assertion helper SHOULD live in `internal/portaltest`:

```go
// AssertLogLevelResolved scans portal.log for the process: log-level resolved
// line matching the given pid and asserts the resolved level matches expected
// with source="env". Used by integration tests that set PORTAL_LOG_LEVEL.
func AssertLogLevelResolved(t *testing.T, logPath string, pid int, expected string)
```

This helper closes the env-propagation gap for ALL daemon-spawning integration tests, not just the test that motivated the assertion.

**Coverage requirement:** every binary entry point that calls `log.Init` automatically emits this line; no separate per-entry-point work needed. The propagation assertion is the test-side coverage requirement.

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
8. Call-site discipline is "multiple independent log calls, slog filters by level" — no portal-specific abstraction. Trace-style wrappers couple DEBUG breadcrumbs to INFO summaries and obscure level discipline at code-review time; we reject them explicitly. Each log call has ONE level chosen explicitly.

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
- **G6, G11** (Component constants reconciliation and prefix taxonomy scope) — closed by the factory pattern: `internal/log` exposes `log.For(component string) *slog.Logger`; each package binds its component once at init; existing `Component*` constants are deleted as part of the migration sweep. Prefix taxonomy subtopic scope explicitly absorbed attr-key vocabulary (G3) and mandatory baseline attrs (G4).
- **G3, G4** (attr-key vocabulary and mandatory baseline attrs) — closed by the locked attr-key vocabulary (snake_case, 10 canonical keys) and the 4-attr mandatory baseline (`component`, `pid`, `version`, `process_role`) injected at root logger construction.
- **G7, G10** (PORTAL_LOG_LEVEL default flip user-visible impact and deprecation path for existing `warn` users) — closed. Resolution: release notes only, no in-band breadcrumb. `portal.log` is a forensic artifact users only look at after the fact, so an in-band INFO line announcing the default change is invisible at the moment it would matter. Existing users who explicitly set `PORTAL_LOG_LEVEL=warn` continue to work unchanged. Users without an explicit value get the new INFO baseline; the "more volume than expected" friction is one mental moment + a changelog glance, acceptable cost for the continuous forensic baseline win.
- **G1** (level edge classes — idempotent no-ops, hysteresis-internal anomalies, recoverable-but-rare) — closed via the placement clarifications added under "Log-level discipline § Placement clarifications": no-ops default to DEBUG (INFO only when the no-op IS the user-visible decision), hysteresis-internal failures stay DEBUG until the threshold trips, recoverable-but-rare events warrant WARN.
- **G8** (NopLogger sentinel / nil-receiver semantics under slog) — closed by the factory pattern. `log.For(...)` always returns non-nil; the migration mandate is "every consumer holds a `*slog.Logger` from `log.For` or accepts one via DI". Tests use `slog.New(slog.NewTextHandler(io.Discard, nil))` as the silent-logger idiom. No `NopLogger()` sentinel survives the rewrite.
- **G9** (expected vs unexpected swallowed errors) — closed by the `error_class` attr already in the vocab. DEBUG breadcrumbs carry `error_class=expected|unexpected`; sites that genuinely want production visibility for unexpected swallowed errors emit at WARN instead. Per-site judgment call, not a level-contract refinement.
- **G2** (volume math for reboot/upgrade burst at INFO) — closed. Reboot burst on a 30-pane install at INFO is ~50-100 lines across the first 10 seconds (30 hydrate-helper INFOs + 11 bootstrap-step INFOs + ~5-10 saver lifecycle INFOs + initial capture cycle summaries). Steady-state with 1Hz cycle summaries at INFO peaks at ~8 MB/day even under churn. Both well under the 500 MB cap; no rate limiting or sampling needed. The cap exists as runaway-loop protection, not steady-state ceiling.
- **G5** (migration plan from existing `state.Logger` printf API to slog) — closed. The factory pattern lock makes this structurally simple: each of ~12 packages gets `var logger = log.For("<component>")` at init, and `state.Logger.Info(component, fmt, args)` call sites become `logger.Info(msg, attrs...)`. Big-bang sweep in one PR — no adapter shim, no co-existence period. The pipe-delimited line format and `state.Logger` type are deleted in the same PR. Test mock surfaces (`bootstrapDeps` and friends) drop the logger mock entirely and accept `*slog.Logger` directly via `slog.New(slog.NewTextHandler(io.Discard, nil))`.

### Current State

- Seven subtopics decided: Logger library (slog), Log rotation, Retention, Log-level discipline, Subsystem prefix taxonomy (now 14 components), Call-site logging pattern, State-mutation audit trail.
- Scope expansion confirmed: instrument the whole codebase wherever logging would aid debugging/insight, under the disciplined level contract.
- Approach choice: mechanical-rule discussion (each locked subtopic gets a rule spec phase can apply with zero judgment), not per-site enumeration. State-mutation audit trail is the first subtopic written in this shape; the other six will be retro-sharpened to match before discussion concludes.
- Review-sets 001, 002, 003 fully drained: 32 findings closed across all three reviews.
- Remaining map: 6 pending subtopics on defensive invariants, diagnostic context preservation, cycle summaries, log-level propagation verification, lifecycle events, and rollout. All will be written with the mechanical-rule explicitness target.
