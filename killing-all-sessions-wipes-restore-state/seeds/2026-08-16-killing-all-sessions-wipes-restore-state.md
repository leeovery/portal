# Killing all tmux sessions wipes sessions.json and every scrollback file within one daemon tick

A user who kills their tmux sessions while the daemon is running — the obvious thing to do before a reboot, and exactly what someone tidying up would try — destroys their entire restore state about one second later. There is no confirmation, no warning, and the scrollback deletion is irreversible.

The sequence, from a read of the code rather than a live reproduction:

`keepSessionNames` (`internal/state/capture.go`) filters out every session whose name starts with `_`. That means the internal `_portal-saver` and `_portal-bootstrap` sessions never count. So when the user's own sessions are gone but the server is still up hosting the saver, `ListSessionNames` **succeeds** and returns those two internal names, while `keep` comes back empty.

`CaptureStructure` then takes the empty-`keep` path: it skips the pane enumeration entirely, builds `sessions` as an empty slice, and never reaches the all-sessions-failed guard at `capture.go:93`, because that guard is conditioned on `len(keep) > 0`. It returns a structurally valid `Index` with zero sessions and a **nil error**, which is indistinguishable to the caller from a healthy capture of a machine with nothing running.

`Commit` (`internal/state/commit.go:22`) has no emptiness check. Going from 41 sessions to zero is a structural change, so it writes the empty index over `sessions.json` via `AtomicWrite0600`. It then calls `gcOrphanScrollback`, whose referenced set derived from an empty index is empty, so every `.bin` file in the scrollback directory is treated as an orphan and removed.

The net effect is that one tick of the 1s capture loop replaces a full restore state with an empty one and deletes all the saved scrollback behind it. On the machine this was found on that would have been 41 sessions and 42 panes.

The existing guard at `capture.go:93` protects the case where tmux is readable but every session errored — a broken read must not be committed over good state. The gap is the adjacent case: tmux reporting nothing at all is treated as authoritative truth, even though "the user just killed everything" and "everything vanished unexpectedly" produce an identical observation, and even though the second is exactly when the saved state matters most.

Worth noting the asymmetry in blast radius. `sessions.json` is at least reconstructable by reopening sessions; the scrollback `.bin` files are not recoverable once `gcOrphanScrollback` has removed them. A collapse from a populated index to an empty one in a single tick, with irreversible deletion attached, is the shape of the problem rather than the empty commit on its own — legitimately closing every session should of course eventually persist as empty.

## A second route to the same loss: sessions closing one at a time

The daemon tick is not the only committer that reads "fewer sessions" as truth. The `session-closed` global hook runs `portal state commit-now` synchronously on every session close (`internal/tmux/hooks_register.go` — deliberately synchronous so a killed session cannot be resurrected by the next tick), and commit-now commits through the same `Commit` (`internal/state/commit.go`), which writes the shrunken index and then runs `gcOrphanScrollback` against it. So a shutdown or reboot that tears sessions down one by one while the hooks are registered commits a progressively smaller index on each close, removing each closed session's scrollback as it goes — no empty capture is needed to reach the same end state. This is from a code read, not a live reproduction; whether a plain reboot with a live daemon actually walks this path (the ordering of tmux's teardown, whether `run-shell` hooks still execute as the server dies) is unverified. A fix scoped only to the empty-`keep` capture case in the daemon tick would leave this route open.

## Current workaround: the verified safe-reboot procedure

Until this is fixed, the only reboot path verified to preserve everything is to stop Portal's runtime first, so no committer is alive while sessions die. Verified 2026-09-30 on Portal 0.12.0: 38/38 sessions restored, 35/35 resume hooks fired, 38/38 scrollbacks replayed, zero warnings.

1. Back up `~/.config/portal`.
2. `portal uninstall` — kills `_portal-saver`, whose SIGHUP makes the daemon flush a final atomic state and exit, then unregisters the global hooks (including `session-closed`); touches no files.
3. Confirm the final flush in `portal.log` (`flush_completed=true`).
4. Reboot without running `x` / `portal open` first — either re-bootstraps the runtime and re-arms both committers.
5. `x` after login.

Never kill the tmux server or its sessions before a reboot while the runtime is installed. A plain reboot with a live daemon is unverified: the one reboot on record (2026-09-02) happened with the daemon already dead, so it says nothing about the live-daemon case.

What a fix has to make true, stated as the user-visible outcome: a reboot or a kill-everything with the runtime live must not leave the user with an emptied restore state and deleted scrollback, while legitimately closing every session must still eventually persist as empty. The irreversibility sits in the scrollback deletion, not the `sessions.json` write — the collapse from populated to empty (or populated to near-empty in quick succession) with deletion attached is the shape to guard.
