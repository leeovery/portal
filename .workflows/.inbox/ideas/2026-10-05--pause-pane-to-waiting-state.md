# Pause a running pane into the waiting state on demand

Lazy resume shipped in 0.12.1 and its first real reboot showed what the waiting state is worth. Before the reboot, 36 idle panes each running a long-lived program cost 12.3 GB of memory and about 23% of a CPU core between them, roughly 340 MB per pane and a constant small CPU draw even while doing nothing. After the reboot, 32 panes sitting in the waiting panel cost 209 MB in total, about 6.5 MB each, and no measurable CPU. Everything under tmux dropped from 12.4 GB and 194 processes to 0.9 GB and 86. The waiting state is cheap, and it already keeps everything needed to come back: the pane's scrollback, its place in the session list, and its registered resume command.

Today the only way into that state is a reboot. The idea is to make it something the user can enter on purpose: pause a pane that is running its registered resume command, so the program stops and the pane drops into the same waiting panel a restored pane shows, with Enter resuming it exactly as after a reboot and discard working as it does today. The session stays open, the pane keeps its history, and its resources are freed until the user comes back to it. With a few dozen sessions open and only a handful in active use, pausing the rest would reclaim most of the memory they hold without closing anything or losing where each one was.

The motivating case is long-running assistant sessions left idle between bursts of work, but the mechanism is about panes with a resume command, so Portal's own wording stays generic and names no tool.

Points that came up while discussing it:

- It only makes sense for a pane that has a registered resume command; a pane without one has nothing to resume into.
- The user's external registration hook removes a pane's resume command when the program inside it exits explicitly (`/exit`), and keeps it when the program is ended by a signal. Pausing must leave the registration in place.
- The daemon stops saving a waiting pane's scrollback, and its last save can be up to 30 seconds old, so whatever was on screen just before a pause has to make it into the saved state.
- Whether swapping a live pane's program, the way restore does, keeps that pane's tmux history has not been measured.
- Portal cannot tell whether the program in a pane is busy; pausing a busy pane interrupts it, and that is the user's call.
- A paused pane would run in the same shape as a waiting pane, which the `killing-all-sessions-wipes-restore-state` investigation found exposed at reboot until that fix ships (the fix makes waiting panes outlast the shutdown signal). The user wants this picked up after that bugfix lands.
- Natural ways in are a picker key on the Sessions page (including over a multi-select), a CLI verb, or a tmux binding.
