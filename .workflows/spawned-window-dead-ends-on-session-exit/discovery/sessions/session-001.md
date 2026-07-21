# Discovery Session 001

Date: 2026-07-21
Work unit: spawned-window-dead-ends-on-session-exit

## Description (as of session)

Windows opened by a multi-select spawn burst dead-end on "Process exited.
Press any key to close the terminal." when their session exits, because
each spawned window runs `portal attach` as its sole command and execs
straight into tmux with no interactive shell to fall back to. Fix them to
land in a real shell — the way the trigger window does — so exiting a
session drops back to a usable prompt (or closes cleanly).

## Seed

- seeds/2026-07-17-spawned-window-dead-ends-on-session-exit.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Originated from an inbox bug captured 2026-07-17, after the multi-window
spawn feature started working. The reported symptom: when a multi-select
burst opens several host-terminal (Ghostty, macOS) windows and the user
later exits the process running inside one of those sessions (e.g. quitting
Claude), the spawned window does not close cleanly or drop to a shell —
instead it detaches and dead-ends on tmux/terminal end-of-command text
("[detached …]" then "Process exited. Press any key to close the
terminal."), requiring a keypress to close.

The asymmetry is the key signal: only the *spawned* N−1 windows show this;
the trigger window (where the multi-selection happened) lands back at a
normal shell prompt when its session exits. The bug file's hypothesised
mechanism — carried as a lead, not a confirmed cause — is that each spawned
window runs `portal attach <session> --spawn-ack …` as its direct command,
and `portal attach` outside tmux `syscall.Exec`s into `tmux attach-session
-A`, making the tmux client the window's one-and-only process. When the
session exits/detaches the client returns, the window's command is finished,
and there is nothing to fall back to. The trigger window avoids this because
portal was launched there from an existing interactive login shell.

Shape was settled quickly as a bugfix (present-broken behaviour, concrete
symptom + error text, a root-cause still to confirm, no new thing to build)
and routes to investigation first.

On direction, the user has a preferred *outcome* but no fixed mechanism:
the spawned window should behave "as if I opened Ghostty myself" — a
standard interactive shell (zsh) — with the session launched into it, so
exiting lands back at a prompt. They explicitly deferred the "is this
possible / what's the standard approach" feasibility and approach questions
(chain a shell after attach vs. have the shell exec portal vs. lean on the
host terminal's post-command config) to the investigation phase. They also
asked for correctly-written test commands to validate — to be provided in
investigation, sandboxed on a throwaway `-L` tmux socket (never the live
server, which hosts ~31 real sessions), so the earlier Ghostty spawn
misfires are not repeated.

Relevant areas flagged for investigation: `internal/spawn` (command
composition — the `<os.Executable()> attach <session> --spawn-ack …`
env-self-sufficient argv), `cmd/attach.go` / `AttachConnector` (the
`syscall.Exec` handoff to `tmux attach-session -A`), and the Ghostty
adapter (`OpenWindow`).

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
