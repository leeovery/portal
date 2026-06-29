# Research: Restore Host Terminal Windows

Portal's resurrection layer restores the tmux/server layer after reboot, but not the **host-local terminal layer** — the actual terminal-emulator windows that were attached to sessions on this machine. This research explores whether Portal can track which terminal windows on *this host* were attached to which sessions, and then re-spawn and re-attach them on demand after a reboot, sparing the user from manually rebuilding their working state by hand.

## Starting Point

What we know so far:

- **Prompted by:** Portal's resurrection layer restores the tmux/server layer after reboot but NOT the host-local terminal layer. After a crash with ~32 Claude sessions, ~28 reattached correctly at the server level, but the user still had to manually rebuild every macOS terminal window by hand (~14 Spaces, one project zone per Space, each window holding a few sessions) — roughly an hour of work: open Ghostty, Cmd+N, drag to the right Space, attach, navigate, preview, confirm.
- **Already knows:** Feasibility is genuinely uncertain on two fronts — (1) distinguishing host-local attachment from tmux-level attachment that a mobile or other client could also hold, and (2) programmatically spawning terminal windows and re-attaching them to their sessions.
- **Starting point:** Technical feasibility — can Portal track which terminal windows on THIS host were attached to which sessions, and can it re-spawn + re-attach them on demand?
- **Constraints:** macOS Spaces placement (dropping each reopened window back onto its original Space) is explicitly out of scope — deferred as a separate Mac-specific follow-up. The user is biting this off in chunks; "reopen the windows" is its own job.

---

## Open Questions

- Can Portal reliably distinguish a **host-local** tmux client/attachment from one held by another client (e.g. mobile, SSH, another machine)?
- What terminal emulators are in scope? (Primary: Ghostty on macOS.)
- How does a terminal window map to tmux sessions/clients — one window = one client = one attached session?
- What mechanisms exist to **programmatically spawn** a terminal window and have it attach to a specific session? (CLI launch flags, AppleScript, `open`, URL schemes, etc.)
- Where/how should the host-local window→session mapping be **persisted** so it survives reboot?

## Triage

(none)
