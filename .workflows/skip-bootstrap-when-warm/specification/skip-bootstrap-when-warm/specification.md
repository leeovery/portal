# Specification: Skip Bootstrap When Warm

## Overview & Goals

### Problem

Portal's bootstrap orchestrator (`cmd/bootstrap`, run from `cmd/root.go`'s `PersistentPreRunE`) fires on **every** command not in the `skipTmuxCheck` allow-list. Its meaningful work — `EnsureServer`, `RegisterPortalHooks`, `SweepOrphanDaemons`, `EnsureSaver`, `Restore`, and the cleanup sweeps — is logically a **once-per-tmux-server-lifetime** concern, run once-per-command purely defensively: Portal has no "has this server already been bootstrapped this lifetime?" signal, so each command re-ensures the whole world. On a warm server these steps are idempotent no-ops — redundant work plus an avoidable concurrency surface when N commands hit the server near-simultaneously.

### Goal

Set a single **server-scoped latch** at the end of a successful bootstrap. Later commands in that server lifetime see the latch and take a cheap **abridged** path instead of re-running the full orchestrator. First (unlatched) touch runs the full bootstrap and sets the latch; latched commands do a cheap latch-check and skip.

### Motivation (what this is and isn't for)

- **Primary driver — collapse the concurrency surface.** This latch is the pretext for the downstream `restore-host-terminal-windows` feature, whose multi-select reopen spawns N−1 windows each running `portal attach <session>` (and `attach` is *not* in `skipTmuxCheck`). A 20-window post-crash rebuild would otherwise fire ~20 near-simultaneous full bootstraps against one server — a stability hazard. The latch collapses that to ~20 cheap latch-checks.
- **Secondary driver — stop redundant per-command work.** Every warm command stops re-running restore/sweep/clean.
- **Explicit non-goal — single-command safety.** A lone warm bootstrap is *already* safe today: `Restore` skips already-live sessions (`internal/restore/restore.go`), so on a warm server it is a near-no-op. The feature is **not** about correctness of one warm bootstrap; it is about concurrency and redundancy.

### Hard constraint — long-lived servers

The user routinely keeps a tmux server alive for **weeks**; server restarts are rare and must not be relied on for recovery. Anything that self-heals today on the *next command* (because bootstrap re-runs every command) must keep a path to self-heal within a single, possibly weeks-long, server lifetime. Recovery cannot be pushed to "next server restart."

### Scope

- A **version-stamped server-option latch** (`@portal-bootstrapped`) gating a **full** vs a single **abridged** bootstrap path.
- The abridged path runs a **liveness-only EnsureSaver** and nothing else.
- **Hooks stale-cleanup (former step 11) is removed from the orchestrator entirely** and re-homed on the `_portal-saver` daemon (orchestrator drops from 11 → 10 steps).

### Dependency

This feature is built **before** `restore-host-terminal-windows`, which depends on it landing first. It surfaced as review finding **F1** during that feature's discussion. Once landed, reopen can spawn plain `portal attach` with no special bootstrap-exempt command or hidden flag.

---

## The Two Bootstrap Paths

The all-or-nothing "latch set ⇒ skip all steps" framing is rejected — the orchestrator's steps are not the same *kind* of work. Some are genuinely once-per-lifetime and categorically pointless on a latched server; one is an ongoing safety net that must keep running. The design splits into two named paths, selected by the latch.

### Terminology

- **Full bootstrap** vs **abridged bootstrap** — *not* "cold"/"warm". "Cold/warm" collides with "is the tmux server running"; the real trigger is the **latch** ("has Portal bootstrapped *this* server yet"). These usually coincide with server-was-off but are not identical: a hand-started tmux server hit by `x` has no latch → gets the full bootstrap.
- **Same orchestrator, two invocation modes.** Full and abridged are not different programs. The full path is the existing `Orchestrator.Run`; on a cold `portal open` it runs concurrently behind the loading screen, otherwise synchronously. The loading screen is a slow-path wrapper, not a distinct bootstrap.

### Step classification

Three classes:

| Class | Steps | Path behaviour when latched |
|---|---|---|
| **1 — Cold-only** (once-per-lifetime; idempotent no-op when warm) | EnsureServer, RegisterPortalHooks, SetRestoring, SweepOrphanDaemons, Restore, EagerSignalHydrate, ClearRestoring | **Skipped.** Server is up (else the latch would have died with it); hooks converged once and nothing re-adds them mid-lifetime; restore is a cold-boot concern; the orphan-daemon sweep targets *prior-lifetime* leftovers (within a lifetime the daemon flock + self-supervision keep N≤1). |
| **2 — Protective liveness** (safety net against mid-lifetime death) | EnsureSaver | **Kept on every abridged command** as a cheap probe + re-ensure if down. (Reduced to liveness-only — see the EnsureSaver topic.) |
| **3 — Cleanup / hygiene** (accrues over the lifetime) | CleanStaleMarkers, SweepOrphanFIFOs, ~~CleanStale (hooks)~~ | Markers/FIFO sweeps stay in the full bootstrap for cold-boot leftovers (a warm server produces none). Hooks `CleanStale` is **removed from the orchestrator entirely** and re-homed on the daemon — see that topic. |

### The two paths

- **Full bootstrap** (latch not satisfied): the full orchestrator — **10 steps** (the original 11 minus `CleanStale`, which is removed and relocated to the daemon; the marker/FIFO sweeps stay for cold-boot leftovers) — then set the latch as the final action of a successful run.
- **Abridged bootstrap** (latch satisfied): **EnsureSaver liveness probe only** — the single, uniform reduced path. Everything else is skipped.

### Single-abridged constraint (user directive)

There is **exactly one** abridged path, run identically by every command against an already-bootstrapped server. Multiple abridged variants (e.g. an `open`-flavour that cleans + an `attach`-flavour that doesn't) are explicitly rejected. This constraint is load-bearing: it is what forces hooks cleanup out of the orchestrator entirely (a per-command-classified cleanup would be exactly the rejected multi-variant design, and keeping it in the one abridged path would run it under the 20× `attach` burst).

---

## The Version-Stamped Latch

### Storage

`@portal-bootstrapped`, a tmux **server option** whose value is the **binary version** (a *version-stamped* latch, not a bare presence flag):

- Set via `SetServerOption("@portal-bootstrapped", <version>)` (`set-option -s`).
- Read via `TryGetServerOption` → `(value, found, err)`.
- Cleared via `UnsetServerOption` (`set-option -su`, idempotent) — used by the manual escape hatch; production code never needs to unset it.

Same server-option mechanism as `@portal-restoring`: it **dies with the tmux server**, so a server restart auto-clears it and the next command full-bootstraps. It reuses the existing `internal/state/markers.go` seam vocabulary (`RestoringChecker` for the read, `ServerOptionWriter` for set/unset) and the `internal/tmux/tmux.go` server-option API. The difference from `@portal-restoring` is that the **value is load-bearing** (a version), not presence-only. `@portal-restoring` is the direct precedent — an already-existing presence-latch on the same mechanism.

### Semantics — "satisfied"

The latch is **satisfied** only when it is **present *and* its stored version equals the running binary's `cmd.version`**. A single `TryGetServerOption` read drives a three-way outcome:

| Read result | Meaning | Path |
|---|---|---|
| Absent (not found) | Cold / fresh server | Not satisfied → **full bootstrap** |
| Present, version **matches** | Already bootstrapped this binary | Satisfied → **abridged bootstrap** |
| Present, version **mismatches** | Post-upgrade | Not satisfied → **full bootstrap** |
| Read error / down-server | Unreadable | Not satisfied → **full bootstrap** |

Both "value mismatch" and "unreadable/error" fold into *not satisfied → full bootstrap*. A separate `ServerRunning()` probe is not required — the read fails gracefully on a down server. Single read chosen for minimalism.

### Why version-stamped, not presence

The user upgrades Portal often (brew) and restarts tmux rarely (weeks). A **presence** latch would keep `RegisterPortalHooks` from ever re-running mid-lifetime, so a new binary's changed global-hook bodies would silently lag the installed version until a tmux restart — potentially weeks. **Version-stamping** makes a release upgrade re-converge hooks and recreate the daemon on the new binary on the **first post-upgrade command**, then re-stamp the latch with the new version. It self-heals with no special-casing. The stored value also serves forensics: the version is the load-bearing field; optional cheap additions (set-timestamp, pid) are an implementation detail.

### Dev-build nuance (accepted)

Local/unversioned builds carry a constant version string, so version-stamping only re-bootstraps on real version bumps (releases), not local rebuilds. The user rarely runs local builds (they interfere with the brew-installed version and aren't easily isolated), so this is a non-issue. Testing local hook changes uses the escape hatch: `tmux set-option -u @portal-bootstrapped` forces the next command back to a full bootstrap.

---

## Latch Set-Point & Timing

This is the load-bearing decision: a full bootstrap can take seconds (it restores N sessions), so the window between "full bootstrap starts" and "latch set" is where all concurrency/atomicity risk lives.

### Decision

**Set the latch as the final action of a *successful* `Orchestrator.Run` — after the last step, gated on no fatal error.**

1. **Atomic-with-success, uniform across both invocation modes.** The latch is set *inside* `Run`, not by the two callers, so the synchronous path and the concurrent cold+TUI goroutine both get it identically — no second set-point to keep in sync. "Latch present" ⟺ "a full bootstrap ran to completion."

2. **Set at the *end*, not early.** Early-setting (e.g. right after the server is up) is **unsafe**: a concurrent command would see the latch and take the abridged path *before Restore recreated the sessions*, then attach to a session that doesn't exist yet. End-setting is **sufficient** for the target scenario: the reopen burst can't fire until the user multi-selects in the picker, and the picker only appears *after* bootstrap completes (loading screen on cold, synchronous on warm) — so by the time the ~20 `attach` fire, the latch is already set and they all take the abridged path.
   - **Explicitly accepted non-goal:** a *pure cold-burst* — N commands hitting a genuinely serverless tmux simultaneously, *not* via the picker — is **not** collapsed by end-setting. That isn't the reopen flow, and it is already tolerated today (daemon flock + idempotent hook convergence). We accept it rather than complicate the set-point.

3. **"Successful" = no *fatal* error; soft warnings still latch.** A soft-step warning (`SaverDownWarning`, `CorruptSessionsJSONWarning`, partial restore) **still sets the latch**, because those either self-heal on the abridged path (EnsureSaver re-probes every command) or are non-retryable (a corrupt file won't un-corrupt next command). Requiring a totally-clean run would let one transient `SaverDownWarning` force every command back to full bootstrap for the whole server lifetime — defeating the feature. Only a **fatal** step (EnsureServer / RegisterPortalHooks / SetRestoring / ClearRestoring — the steps that already abort with a non-zero exit / red TUI frame) leaves the latch **unset**, so the next command correctly retries the full bootstrap.

### Write posture

The terminal `SetServerOption("@portal-bootstrapped", version)` is **best-effort**: on failure, log WARN and swallow — never fatal. A failed write simply leaves the latch unset, so the next command reads "not satisfied" and re-runs the (idempotent, near-no-op on warm) full bootstrap, retrying the write. (Full failure/invalidation treatment is in **Edge Cases & Latch Invalidation**.)

### Ordering bonus

Because the latch is set only *after* `EagerSignalHydrate` and `Clear @portal-restoring` have run, "latch present" **guarantees** hydrate signalling finished and `@portal-restoring` was cleared. Two consequences fall out with no extra logic:

- The latch and `@portal-restoring` can never both be set on an abridged command.
- A late-arming skeleton pane can't be stranded unsignalled.

---

## Latch-Check Placement & Abridged-Path Wiring

### Placement — a single latch-read drives a three-way branch

In `PersistentPreRunE`, after the tmux client is built, read the latch (`TryGetServerOption("@portal-bootstrapped")`) and compare its value to the running binary version:

- **Latch satisfied** (present **and** version matches) → **abridged path**.
- **Latch not satisfied** (absent, unreadable/down-server, **or** version-mismatch) → **full bootstrap**: concurrent + loading screen on the TUI path (`open`, no args), synchronous otherwise.

A separate `ServerRunning()` probe is not required — the latch-read fails gracefully on a down server, so "unreadable" folds into "not satisfied → full bootstrap."

### Loading-screen trigger: latch-absent, not server-down

The concurrent/loading path (`shouldRunConcurrentBootstrap`) currently fires only for `portal open` (no args) **and** server-not-running. It now fires whenever a **full** bootstrap runs on the TUI path — keyed off **latch-not-satisfied**, not server-down. This retires the warm-unlatched edge as an improvement: a hand-started tmux server + `x` now gets the loading screen + progress during its first full bootstrap instead of a synchronous no-progress stall. Conceptually, "loading screen" now means exactly "a full bootstrap is in progress." *What* the full bootstrap does is unchanged (Restore etc. already ran on warm-unlatched today) — only the presentation improves.

### Outcome matrix

| Command | Latch | Outcome |
|---|---|---|
| `open` (no args) TUI | not satisfied (absent / version-mismatch) | full bootstrap, concurrent + loading screen |
| `open` (no args) TUI | satisfied (present + version match) | abridged (sync plumbing, instant picker) |
| `attach` / `open <path>` / CLI | not satisfied | full bootstrap, synchronous |
| `attach` / CLI | satisfied | abridged (sync plumbing) |

### Abridged wiring reuses the sync plumbing

The abridged path runs through the **same entry-path plumbing** (warning sink + context injection) as the synchronous full path, differing only in executing a reduced step set (EnsureSaver only). This is what makes the following inherit existing, tested handling:

- **Context injection.** The abridged path still injects `serverStartedKey` + `tmuxClientKey` into `cmd.Context()` (exactly as the sync path does) — it just doesn't run the orchestrator. `serverStarted` is injected as **`false`** (correct: the command did not start the server). Its sole production consumer is `openTUI`'s loading-page gate → `false` → no loading page → instant picker, which is exactly right for a warm command. There is no hidden "third state" to disambiguate.
- **Warnings.** EnsureSaver's `SaverDownWarning` funnels into the same package-level `bootstrapWarnings` sink the sync path already uses → the CLI flushes to stderr; the TUI drains to the notice band. Identical to a warm command today; no new emission mechanism.

---

## Abridged EnsureSaver — Liveness-Only

EnsureSaver (Class 2) is the one step that stays on the abridged path. It is the safety net against the `_portal-saver` daemon dying mid-lifetime — the daemon's own self-supervision can `os.Exit(0)`, tearing down its pane and killing the `_portal-saver` session, and a per-command re-ensure brings it back. With weeks-long servers, dropping this net would let a self-ejected daemon stay dead for weeks (silent loss of scrollback capture and resurrection-state), so it must keep running per-command.

### EnsureSaver's two duties, split across the two paths

EnsureSaver does two things:

- **(a) Liveness** — create `_portal-saver` + daemon if absent (`BootstrapPortalSaver`).
- **(b) Version-gate** — if the running daemon's binary is stale, kill + recreate it on the new binary via a guarded kill-barrier (`EnsurePortalSaverVersion`).

The version-stamped latch splits these:

- **Abridged path → liveness only.** A *satisfied* latch (present **and** version-matching) already proves the running daemon is the current binary, so the version-gate is redundant. Abridged EnsureSaver reduces to a pure liveness probe (`SaverPanePIDOrAbsent` + re-ensure if absent).
- **Full bootstrap → keeps the version-gate.** A version bump makes the latch mismatch → full bootstrap → the version-gate kill-barrier recreates the daemon on the new binary → re-stamp.

### Liveness-only is *not* reduced crash recovery

Dropping the version re-check is **not** a reduction in crash/death recovery. On every abridged command the liveness probe asks "is the `_portal-saver` daemon alive?" (`SaverPanePIDOrAbsent`) and recreates it if absent. A random daemon crash → pane process exits → `_portal-saver` session gone → the next warm command's liveness probe revives it. That is exactly the fail-safe from the "keep EnsureSaver on the warm path" decision, preserved unchanged. The **only** thing dropped from the abridged path is the redundant *version* re-check.

### Concurrency benefit (dissolves the kill-barrier race)

Because no abridged command ever runs the version-gate kill-barrier, N concurrent warm commands can never race to kill-barrier a stale daemon — the single post-upgrade full bootstrap does the recreate once. Liveness re-ensure of a genuinely-absent saver stays serialised by the daemon flock as before.

### Two independent daemon safety nets (both preserved)

1. **Self-supervision** — the daemon self-ejects if it detects it is no longer the legitimate `_portal-saver` pane (split-brain guard).
2. **Abridged liveness revival** — on every warm command (plus a full bootstrap on cold boot / upgrade).

This is belt-and-suspenders: keep the fail-safe *and* separately pursue making the daemon as robust as possible (ongoing work, not gated by this feature).

### Restore-window note

On the warm path EnsureSaver runs **outside** the `@portal-restoring` window (no restore in flight), which is correct — a revived daemon should capture normally, not suppress.

---

## Daemon-Owned Hooks Cleanup

The weeks-long-server constraint raised a worry: cleanup steps (marker sweep, FIFO sweep, hooks `CleanStale`) are framed as once-per-lifetime, but if cruft *accrues* during a weeks-long lifetime, skipping them on abridged commands would let it pile up for weeks. Tracing each cleanup target to its producer resolves this.

### The trace — what a warm server actually produces

- **Skeleton markers (`@portal-skeleton-*`)** — `SetSkeletonMarker` is called from exactly **one** place: `internal/restore/session.go` during bootstrap restore. A warm server creates **zero**. Any stale ones are cold-boot restore leftovers, already cleaned by the marker sweep during that same cold boot. ⇒ **no mid-lifetime workload.**
- **Hydrate FIFOs (`hydrate-*.fifo`)** — `CreateFIFO` is called from exactly **one** place: `internal/restore/session.go` during restore. A warm server creates **zero**. ⇒ **no mid-lifetime workload.**
- **Hook entries (`hooks.json`)** — created by `portal hooks set` (user action, any time) and go stale when the keyed pane/session is killed (normal warm activity). This is the **only** cleanup target a warm server genuinely produces over time.

So the marker/FIFO sweeps have no warm workload and stay in the full bootstrap for cold-boot leftovers. Only hooks cleanup needs a new home.

### A stale hook entry cannot misfire

The user's concern is side-effects, not bloat — can a stale hook fire on the wrong target? No. The hook key is the structural key `#{session_name}:#{window_index}.#{pane_index}` (e.g. `myproj-AbC123:0.0`). Session names are `{project}-{nanoid}` and `GenerateSessionName` **guarantees uniqueness**. A "stale" entry = a key not in the live pane set. For that key to become live again, a session with that exact nanoid-bearing name must exist again — which only happens when Portal **restores that same saved session** (same identity) after a reboot, where firing is the hook's *intended* behaviour. A different, newly-created session gets a new nanoid → new key → never collides. Within-session index reuse keeps the key **live** (never classed as stale). **Conclusion: a genuinely-stale hook entry cannot fire on the wrong session — the only cost of leaving it is inert JSON bloat.**

### Decision — remove hooks cleanup from the orchestrator; home it on the daemon

The single-abridged constraint forces this: a command-classified cleanup ("clean on `open`, not `attach`") is the rejected multi-variant design, and keeping cleanup in the one abridged path would run it under the 20× `attach` burst (the anti-recommended `list-panes -a` + `hooks.json` rewrite concurrency surface). So cleanup can live in **neither** abridged variant.

- **Steps 9 & 10 (marker/FIFO sweeps):** stay in the full bootstrap, skipped on the abridged path (a warm server produces none of their targets).
- **Former step 11 (`CleanStale` hooks):** **removed from the orchestrator entirely** — the step *and* its seam/adapter — taking the orchestrator from 11 → 10 steps. The `_portal-saver` daemon (`portal state daemon`) becomes its **sole automatic home**.

Rationale for full removal (not just skipping on abridged): a bootstrap-time cleanup would only *uniquely* help when a full bootstrap runs **and** EnsureSaver fails to start the daemon — a scenario already catastrophic (no daemon ⇒ no scrollback capture), where an inert stale-hook entry is noise. What it cleans is inert anyway. At cold boot the freshly-started daemon cleans on its first eligible tick (~10s) rather than during bootstrap — fine, since it's inert. (Trade-off acknowledged: slightly more surgery than leaving a harmless idempotent double-clean in place; the clean single-home model won.)

### Operational contract

- **Home:** the existing background process inside the hidden `_portal-saver` tmux session — **not launchd**. `portal clean` remains the manual, daemon-independent backstop.
- **Reuse, don't reinvent:** the daemon calls the existing shared `cmd/run_hook_stale_cleanup.go` `runHookStaleCleanup` helper. That helper already carries the **mass-deletion hazard guard** (`len(livePanes)==0 && hooks present` → skip + WARN, never wipe) and drives `hooks.Store.CleanStale`, which emits the existing `EmitCleanStaleSummary` **audit breadcrumb** — so no new audit event/vocabulary is invented.
- **No layering problem:** `runHookStaleCleanup` and the daemon (`cmd/state_daemon.go`) are both in package `cmd` — same package, so no new import and no cycle.
- **Cadence:** **not** every 1s tick. Throttled to ~10s via a cheap `time.Since(lastCleanup) >= interval` check evaluated per tick; the cleanup body fires only when the interval has elapsed. The 1s tick must stay light (capture/scrollback save is the priority and can exceed 1s); stale hooks are inert so precise timing is irrelevant. Exact interval is a tuning detail (**10s default**).
- **Priority / non-interference:** cleanup never competes with a pending capture — the tick loop is single-threaded and already skips entirely while `@portal-restoring` is set and on the `!dirty && !gap` idle fast-path; cleanup is gated so scrollback saving always wins.
- **Failure posture:** log WARN and retry next cadence (mirrors the tick loop's existing "tick failed" handling); a cleanup error never escalates or crashes the daemon.

---

## Edge Cases & Latch Invalidation

`@portal-bootstrapped` is a *persistent* lifetime latch (unlike the transient `@portal-restoring`), so its failure/staleness modes need explicit treatment. **Guiding principle:** don't program around anything that self-heals via an idempotent, no-op full bootstrap.

### Invalidation & failure modes

- **Auto-invalidation by design.** The latch is a server option → dies with the server → restart auto-clears it → next command full-bootstraps. No explicit invalidation code.
- **Upgrade invalidation.** Version-mismatch is treated as "not satisfied" → the first post-upgrade command full-bootstraps (re-registers hooks, recreates the daemon on the new binary) and re-stamps. Self-healing; no special-casing.
- **Two markers can't both be set.** The latch is set *last* (after `Clear @portal-restoring`), so "latch satisfied" ⇒ restoring was cleared. A crash mid-bootstrap leaves the latch unset → next command full-bootstraps and re-clears any leaked restoring marker. No inconsistent state reachable on a steady server.
- **Latch-set write failure.** The terminal `SetServerOption("@portal-bootstrapped", version)` is **best-effort**: on failure, log WARN and swallow. The next command reads "not satisfied" → re-runs the (idempotent, near-no-op on warm) full bootstrap → retries the write. Self-heals; **never fatal**.
- **Manual escape hatch.** `tmux set-option -u @portal-bootstrapped` forces the next command back to a full bootstrap — handy for debugging or forcing a re-converge without a tmux restart.
- **Abridged EnsureSaver hard-failure.** With the version-gate moved off the abridged path, abridged EnsureSaver is liveness-only; a failure to re-ensure an absent saver surfaces as a soft `SaverDownWarning` (via the existing sink) and the command **proceeds** — attach/switch still works; capture simply resumes on the next successful revival. No kill-barrier runs on the abridged path, so there is no kill-barrier-failure branch to handle there (it lives in the full bootstrap, already a soft step).

### Accepted residues (harmless bloat — reviewed & tolerated)

- **Cold-boot cleanup leftovers.** If a cold boot's marker/FIFO cleanup soft-fails, that residue isn't retried until the next full bootstrap. Accepted: markers/FIFOs are inert (the daemon-merge live-set filter already prevents dead-session resurrection), and version-stamped upgrades now give *extra* full-bootstrap cleanup passes beyond just restarts.
- **Daemon-death vs cleanup home.** Hooks cleanup was relocated from an always-runs path (per-command bootstrap) to a conditionally-alive one (the daemon) and removed from the orchestrator — a named trade. Homes: the daemon (revived by the abridged liveness EnsureSaver) plus `portal clean` (manual, daemon-independent). Worst case (daemon dead *and* revival failing *and* no `portal clean`) leaves only inert hooks bloat until the daemon next revives. Accepted given the misfire trace (stale hooks can't fire on the wrong session); a bootstrap-time pass was explicitly weighed and rejected — it would only help when the daemon can't start at all, already a catastrophic (capture-down) state.

---

## Working Notes

_Optional - capture in-progress discussion if needed._
