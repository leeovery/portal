# Pause a running session into the waiting state on demand

Lazy resume shipped in 0.12.1 and its first real reboot showed what the waiting state is worth. Before the reboot, 36 idle sessions each running a long-lived program cost 12.3 GB of memory and about 23% of a CPU core between them, roughly 340 MB per session and a constant small CPU draw even while doing nothing. After the reboot, 32 sessions sitting in the waiting panel cost 209 MB in total, about 6.5 MB each, and no measurable CPU. The waiting state is cheap, and it already keeps everything needed to come back: the session's scrollback, its place in the session list, and its registered resume command.

Today the only way into that state is a reboot. The idea is to make it something the user can enter on purpose: pause a session that is running its registered resume command, so the program stops and the session drops into the same waiting panel a restored session shows, with Enter resuming it exactly as after a reboot and discard working as it does today. The session stays open, keeps its history, and its resources are freed until the user comes back to it. With a few dozen sessions open and only a handful in active use, pausing the rest would reclaim most of the memory they hold without closing anything or losing where each one was.

The motivating case is long-running assistant sessions left idle between bursts of work, but the mechanism is about sessions with a resume command, so Portal's own wording stays generic and names no tool.

Points that came up while discussing it:

- It only makes sense for a session that has a registered resume command; a session without one has nothing to resume into.
- The user's external registration hook removes a session's resume command when the program inside it exits explicitly (`/exit`), and keeps it when the program is ended by a signal. Pausing must leave the registration in place.
- The daemon stops saving a waiting session's scrollback, so whatever was on screen just before a pause has to make it into the saved state.
- Portal cannot tell whether the program in a session is busy; pausing a busy session interrupts it, and that is the user's call.
- Natural ways in are a picker key on the Sessions page (including over a multi-select), a CLI verb, or a key the attach runner watches for.
