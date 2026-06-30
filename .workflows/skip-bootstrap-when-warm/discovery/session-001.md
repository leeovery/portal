# Discovery Session 001

Date: 2026-06-30
Work unit: skip-bootstrap-when-warm

## Description (as of session)

Server-lifetime tmux latch set at the end of a successful bootstrap so only the first command on a cold server runs the full 11-step orchestrator; warm commands fast-skip restore/sweep/clean. Prerequisite for restore-host-terminal-windows.

## Seed

- seeds/2026-06-30-warm-command-bootstrap-latch.md (inbox:idea)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

The work originated from an inbox idea: Portal's 11-step bootstrap orchestrator (`cmd/bootstrap`, run from `cmd/root.go`'s `PersistentPreRunE`) fires on every command not in the `skipTmuxCheck` allow-list, even though its meaningful work (EnsureServer, RegisterPortalHooks, SweepOrphanDaemons, EnsureSaver, Restore, the CleanStale sweeps) is logically a once-per-tmux-server-lifetime concern. On a warm server those steps are idempotent no-ops — pure redundant work plus an avoidable concurrency surface when N commands hit the server near-simultaneously.

The shaped intent: set a single server-scoped latch (a tmux server option that dies with the server, e.g. `@portal-bootstrapped`) at the end of a successful bootstrap. Later commands in that lifetime see the latch and fast-skip the 11 steps while still building their tmux client. First (cold) touch bootstraps and sets the latch; warm commands do a cheap latch-check and move on.

The user confirmed this is a single, self-contained piece of work living on the bootstrap entry path — not several things, not something broken, and not a trivially mechanical change. They explicitly flagged it as needing discussion: there are real design calls (where the latch lives, how the cold+TUI concurrent bootstrap path sets it, the tolerated first-touch race already absorbed by the daemon flock + idempotent hook convergence) rather than an already-obvious edit. Confirmed as a **feature** routing to discussion.

Noted dependency context (held for discussion, not resolved here): this surfaced as review finding F1 during the `restore-host-terminal-windows` feature discussion — that feature's multi-select reopen spawns N−1 windows each running `portal attach`, which is not in `skipTmuxCheck`, so a post-crash rebuild would fire many near-simultaneous full bootstraps. This latch is intended to land first, dissolving that. Blast radius touches load-bearing core machinery (`shouldRunConcurrentBootstrap`, `cmd/bootstrap_progress.go`, `AcquireDaemonLock`, the `@portal-restoring` window).

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
