# Specification: Slow Open Empty Previews And Zombie Sessions

## Specification

## Problem Statement

This bugfix addresses three user-visible symptoms produced by a single underlying defect: Portal's daemon-singleton invariant is not enforced end-to-end. The same broken-singleton state surfaces as three different downstream effects.

**Symptoms:**

1. **Slow `portal open` (5–8 s)** — Every invocation pays a 5 s timeout before the TUI renders. Caused by the bootstrap kill-barrier in `killSaverAndWaitForDaemon` polling for the recorded `daemon.pid` to exit after `tmux kill-session _portal-saver`; when the recorded daemon is not the saver pane's process, the kill is structurally unreachable and the barrier always times out at its 5 s limit. `portal open` is expected to be sub-second.

2. **Empty session previews** — Pressing `Space` on any session in the picker shows "no saved content" even though the scrollback exists inside tmux. Caused by competing daemons each running `gcOrphanScrollback` against the same state directory with divergent indexes — the scrollback directory oscillates between 0 and 1 `.bin` file as each daemon's commit deletes files referenced only by the other's view. Expected: the highlighted session's captured scrollback renders in the preview pane.

3. **Killed sessions resurrect** — Sessions removed via `K` in the picker (or via the user's `Option-Q` tmux shortcut) reappear on the next `portal open` and persist indefinitely. Caused by multiple daemons independently committing `sessions.json` every tick — the legitimate daemon's post-kill commit (without the dead session) is overwritten seconds later by a competing daemon whose stale `prev` state still includes it. Restore on next bootstrap reconstructs the dead session as a skeleton pane. Expected: `K` removes the session permanently.

## Scope

Bundle all seven fix components (A–G, defined below) into a single bugfix work unit. Each independently closes a real defect or latent fragility; the user has explicitly chosen defence-in-depth over a minimum-viable patch. The framing is "fix Portal so this type of thing never happens" — A+B+G handle the consequences and the known triggers, C closes the underlying *mechanism* (the inode-replacement gap that lets divergent daemons coexist) so unforeseen future triggers cannot recreate the same bug class, and D bounds orphan lifetime to one tick *between* bootstraps so the daemon is polite about its own existence even when no `portal` invocation runs.

## Out of Scope

- **Re-architecting the saver/daemon ownership model.** The current "saver pane process IS the daemon" model is retained; this bugfix hardens the surrounding invariants rather than replacing them.
- **Replacing `flock` with an alternative locking primitive.** Component C tightens the existing `flock`+inode contract rather than swapping primitives. The "flock `sessions.json` itself" alternative was ruled out during investigation synthesis because `fileutil.AtomicWrite0600` replaces sessions.json's inode on every Commit, which would itself break flock semantics.
- **Migrating away from per-tick `sessions.json` rewrites.** The commit + GC pipeline shape is unchanged; only per-session error tolerance and cross-daemon coexistence are hardened.

## Root Cause

Portal's daemon-singleton contract is not enforced end-to-end. Three independent assumptions in the surrounding code, each unverified at runtime, can be violated simultaneously to produce the observed state:

1. **`daemon.lock` excludes per-inode, not per-path.** `state.AcquireDaemonLock` (`internal/state/daemon_lock.go:55-77`) opens whatever inode `daemon.lock` currently resolves to and `flock`s it. There is no cross-check that the inode it locked is still the inode at the path. If `daemon.lock` is unlinked + recreated between two daemon spawns (by any external cause — older code path, manual `rm`, leaked test scaffolding), the two daemons end up `flock`-ing different inodes and the singleton invariant is silently broken. On the reporter's install, three concurrent daemons each held `flock` on a different `daemon.lock` inode (171463046, 171582571, 170216314).

2. **The kill-barrier can only reach daemons bound to the saver pane process.** `killSaverAndWaitForDaemon` in `internal/tmux/portal_saver.go:212-248` polls the recorded `daemon.pid` for death after issuing `tmux kill-session _portal-saver`. If the recorded PID is alive but not the saver pane's process (orphan from a prior bootstrap, leaked test daemon with a different parent tmux server, etc.), the kill is structurally unreachable — the polled process never exits and the barrier times out at 5 s. No SIGTERM/SIGKILL escalation is attempted.

3. **`CaptureStructure` aborts the whole tick on any per-session error.** `internal/state/capture.go:86-90` returns immediately when `ShowEnvironment` fails for any single session. The downstream `captureAndCommit` (`cmd/state_daemon.go:132-207`) then returns before writing scrollback or calling `Commit` — a single bad session at the alphabetical head poisons capture for every later session in the same tick. Latent since commit `7dc990be4` (2026-04-27), present in every v0.5.x release. The per-pane loop in `captureAndCommit:185-192` correctly logs and continues; the per-session loop in `CaptureStructure` is missing the same defensive pattern.

When these are violated together, multiple daemons concurrently write `sessions.json` and execute destructive scrollback GC against the same state directory. `gcOrphanScrollback` (`internal/state/commit.go:102-138`) deletes any `.bin` not referenced by the just-committed index — and trusts whatever index the calling daemon produced, with no cross-check against any other daemon's view. With multiple daemons each committing different views every ~1–2 s, `.bin` files are constantly being deleted and rewritten, and `sessions.json` flips between divergent session lists.

**Trigger on this install:** A test-fixture tmux server at `/tmp/test_hook_debug2/s` is still alive from the prior evening. A test binary at `/private/tmp/portalbin/portal` was launched against this socket and is still running. It inherited `XDG_CONFIG_HOME` from the user's environment because no test isolated it, so its daemon writes to the user's real state directory while enumerating sessions from the test-fixture tmux server (a single session "A"). This is the trigger but not the *cause* — the underlying defects above allow this trigger (and any unforeseen future equivalent) to produce the observed end-state.

### Symptom → mechanism mapping

- **Slow open** → kill-barrier polling an unreachable orphan PID for the full 5 s window.
- **Empty previews** → `gcOrphanScrollback` race between divergent daemons deleting each other's `.bin` writes; further amplified by the `CaptureStructure` abort-on-error path when any single session enumeration fails.
- **Zombie sessions** → competing daemon overwrites the legitimate daemon's post-kill `sessions.json` with stale `prev` state; Restore on next bootstrap reconstructs the dead session.

---

## Working Notes
