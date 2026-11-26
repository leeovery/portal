# Session scrollback preview from the open panel

When the portal open panel lists a lot of running sessions, there's no way to tell them apart without actually attaching. This hurts most with Claude sessions and team-up sessions, especially when several belong to the same project — the session names are just `{directory}-{nanoid}`, so multiple sessions in one project look nearly identical.

The current failure mode is a painful loop: open a session, realise it's the wrong one, detach, try to remember which one I just opened so I don't pick it again, scroll to the next candidate, open that, repeat. Every wrong guess costs an attach/detach cycle and mental bookkeeping about which entries I've already ruled out.

The idea is to let me peek at a session's scrollback without committing to it. Behaviour modelled on macOS Finder's Quick Look:

- **Space** on a highlighted session — opens the scrollback content in a preview screen.
- **Enter** — attach to the session normally.
- **Escape** — return to the list.

From the preview I should be able to see enough recent terminal output to recognise *what's going on* in that session — which Claude conversation, which team-up, which task — so I can decide whether to enter it or move on.

**Explicitly out of scope:** AI-based auto-renaming of sessions. It came up as an adjacent solution to the "can't tell sessions apart" problem, but I don't want it. Keep the focus on preview-based disambiguation; names stay as they are.

Relevant surface area lives in the TUI picker (`internal/tui`) and the sessions page of the page state machine. The scrollback itself presumably comes from tmux — likely `capture-pane` on the session's active pane(s), which would be a new method on the `tmux.Client` in `internal/tmux`. Interaction is purely within the picker TUI; attach/switch behaviour on Enter is unchanged from today.

Open questions for later (don't answer now): which pane's scrollback to show when a session has multiple panes/windows, how much history to capture, whether the preview should be scrollable or just a fixed snapshot, and how to render ANSI colour from captured output inside Bubble Tea.
