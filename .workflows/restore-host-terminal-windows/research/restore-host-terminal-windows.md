# Research: Restore Host Terminal Windows

Portal's resurrection layer restores the tmux/server layer after reboot, but not the **host-local terminal layer** — the actual terminal-emulator windows that were attached to sessions on this machine. This research explores whether Portal can track which terminal windows on *this host* were attached to which sessions, and then re-spawn and re-attach them on demand after a reboot, sparing the user from manually rebuilding their working state by hand.

## Starting Point

What we know so far:

- **Prompted by:** Portal's resurrection layer restores the tmux/server layer after reboot but NOT the host-local terminal layer. After a crash with ~32 Claude sessions, ~28 reattached correctly at the server level, but the user still had to manually rebuild every macOS terminal window by hand (~14 Spaces, one project zone per Space, each window holding a few sessions) — roughly an hour of work: open Ghostty, Cmd+N, drag to the right Space, attach, navigate, preview, confirm.
- **Already knows:** Feasibility is genuinely uncertain on two fronts — (1) distinguishing host-local attachment from tmux-level attachment that a mobile or other client could also hold, and (2) programmatically spawning terminal windows and re-attaching them to their sessions.
- **Starting point:** Technical feasibility — can Portal track which terminal windows on THIS host were attached to which sessions, and can it re-spawn + re-attach them on demand?
- **Constraints:** macOS Spaces placement (dropping each reopened window back onto its original Space) is explicitly out of scope — deferred as a separate Mac-specific follow-up. The user is biting this off in chunks; "reopen the windows" is its own job.

---

## Workflow & Topology (confirmed)

- **One Claude session ≈ one Ghostty window** is the norm. tmux *windows* used occasionally, panes rarely. So the unit of restore is: one host terminal surface per session.
- The tmux/server layer is **not in scope** — the existing resurrection layer already rebuilds tmux sessions/windows/panes on attach. This feature only restores the **host terminal surface** (the Ghostty window/tab) that fronts a session.
- macOS Spaces placement is **out of scope** (parked in inbox as a follow-up to build on top of this).

## The Refined Ask

Portal offers (never forces) a way to reopen N sessions, each springing into its own host terminal surface — **a Ghostty window or tab depending on how it was consumed before the reboot**.

Shape the user proposed: inside the Portal picker, a **multi-select keybinding** (e.g. `M`), select several sessions, `Enter`, and each attaches into its own new window/tab. Concrete scenario: go to Space 3, open Ghostty, `x`, `M`, select the project's 3 sessions, `Enter` → 3 surfaces spring open, each attached. *"Even if we stop there, that would be an amazing feature."*

## Key Reframe — multi-select dissolves most of the tracking problem

The manual multi-select trigger means we **do not** need to track the live window inventory ("what's open right now") or persist a window→session map for replay. The user drives the reopen by hand from a live Ghostty window. So:

- **Sessions already exist** post-reboot (resurrection layer restored them). Portal already knows them.
- The **only** new thing tracking buys is the **window-vs-tab consumption mode** per session — so a session that was a tab reopens as a tab, a window as a window.
- **Implication:** the MVP (multi-select + spring-open) needs *near-zero new tracking*. Window-vs-tab fidelity is a **separable refinement** — v1 could default to "always window" and add mode-tracking later. This is a natural scope seam, not a decision to make here.

## Two Feasibility Risks (the real research)

1. **Spawn** — Can Portal programmatically open a new Ghostty **window** *or* **tab**, each running a specific command (`portal`/`tmux attach -t <session>`)? This is the central risk; the whole feature rests on it. Ghostty's automation surface (CLI actions, IPC, AppleScript dictionary, or fallback keystroke injection via System Events) needs verifying.
2. **Detect mode** — Can Portal tell, *right now*, whether session S is being consumed in a window vs a tab? The tmux client can't see this (terminal-level state, invisible to the shell). Requires querying Ghostty's surface structure externally and correlating to the tmux client (by tty or by `client_pid` → process tree). Harder than spawn; only needed for the window-vs-tab refinement, not the MVP.

## Mechanism Notes (from KB + tmux knowledge)

- `tmux list-clients -F '#{client_tty} #{client_session} #{client_pid} ...'` exposes the live client→session map plus the client's tty and pid (verified tmux 3.6a in the zellij-to-tmux-migration discussion). `client_pid` → ppid chain is the likely path to "is this client a local Ghostty surface."
- Portal already runs a **1s tick daemon** (`portal state daemon` inside `_portal-saver`) that captures session structure to `sessions.json`. The user's "keep a tick / log of how it's consumed" maps directly onto this existing seam — consumption-mode could ride the same capture loop.
- The **"host-local vs other client" distinction** (original feasibility worry) softens under manual multi-select: the daemon runs on *this* host and the user reopens from *this* host, so a future mobile/relay client (per `agent-first-portal`) attaching to the same session wouldn't masquerade as a local window to reopen. The user has final say via the selection anyway.

## Open Questions

- **Ghostty spawn:** does Ghostty expose a clean way to open a new window/tab running a given command? (CLI `+` actions, IPC socket, AppleScript dictionary, `open -na`, or keystroke fallback?) — *central feasibility risk, candidate for deep dive.*
- **Mode detection:** can window-vs-tab be read externally and correlated to a tmux client? Or is it impractical enough that v1 defaults to "always window"?
- **Other terminals:** Ghostty-only for v1, or must the design stay terminal-agnostic? (iTerm/Terminal.app have rich AppleScript; Ghostty may not.)
- **Trigger surface:** multi-select (`M`) in the picker — does it coexist cleanly with the existing single-select attach, grouping modes, and the §12.2 keymap? Any conflict with `Space` preview / `Enter` attach semantics?
- **Where does spawn run from?** The reopen must execute from a live terminal (to launch sibling windows). Is it a Portal subcommand, a picker action, or both?

## Triage

(none)
