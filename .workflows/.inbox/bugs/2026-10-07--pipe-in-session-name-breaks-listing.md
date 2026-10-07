# A `|` in a session name breaks Portal's session listing

A tmux session whose name contains a `|` corrupts the session listing that Portal's save path, picker, completion and restore all read from. Two outcomes have been observed, depending on what follows the `|`.

When digits follow the pipe, as in a session named `api|2`, the listing parses cleanly but reports the wrong name: the session reads as one called `api`. The save cycle then treats the real session as vanished, drops it from `sessions.json` and deletes its scrollback in the housekeeping pass. This happens on every tick for as long as the session exists. The only trace is a single `session dropped` INFO line in portal.log. A reboot then restores without that session, and its scrollback is unrecoverable.

When non-numeric text follows the pipe, as in a session named `work|notes`, the listing fails to parse at all. Every daemon tick then logs `tick failed` and every session close logs `commit cycle failed`, and nothing is committed for as long as that session exists. The saved state goes stale, so a reboot restores an old snapshot. Because the whole listing fails, the picker, shell completion and restore also lose every other session, not just the offending one.

Both tmux and Portal accept such names: a session can be created or renamed to include a `|` from the picker's rename modal, which raises no objection, or directly through tmux. The session listing's format in `internal/tmux/tmux.go` (`listSessionsArgs`, parsed by `parseSessionList`) separates its fields with `|`, which is where the name collides. The pane capture format uses `|||` as its separator and has the same exposure for a name containing that sequence.

This predates the killing-all-sessions-wipes-restore-state fix, which surfaced it during implementation. That fix changed only how an unparseable listing is logged, from a tmux-stopped-answering back-off to a plain failure. It breaks the same rule that fix exists to uphold: a session the user did not kill must stay restorable with its scrollback intact.
