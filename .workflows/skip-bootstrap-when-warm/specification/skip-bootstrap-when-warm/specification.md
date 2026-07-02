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

## Working Notes

_Optional - capture in-progress discussion if needed._
