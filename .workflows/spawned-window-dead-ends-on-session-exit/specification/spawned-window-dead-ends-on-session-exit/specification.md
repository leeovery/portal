# Specification: Spawned Window Dead-Ends On Session Exit

## Specification

### Background & Bug Behaviour

#### Background

Portal's multi-window burst spawn — both the picker multi-select burst and the `portal open` multi-target burst — opens N surfaces for N selected targets. The first surface (the *trigger*) reuses the invoking terminal; the other N−1 surfaces are spawned into fresh host-terminal windows via `internal/spawn`. On macOS the native adapter is Ghostty.

#### Observed Bug

When the session running inside a *spawned* (N−1 external) Ghostty window exits or detaches — the user quits the program running in the session, or the session detaches/ends — the window dead-ends on terminal end-of-command text instead of returning to a usable prompt:

```
[detached (from session <name>)]

Process exited. Press any key to close the terminal.
```

A keypress then closes the window — an awkward dead-end rather than a landing.

#### Expected Behaviour

A spawned window whose session exits should behave "as if the user opened Ghostty themselves": land back at a normal interactive shell prompt, with the session having been launched into it. Closing cleanly is an acceptable fallback, but a usable prompt is the goal. The trigger window already behaves this way.

#### Asymmetry (the key signal)

Only the spawned N−1 windows dead-end; the trigger window lands cleanly at a shell prompt. This is **structural, not incidental**: the trigger self-connects in-process from the already-running Portal, which is itself a child of the user's interactive login shell — so exiting the tmux attach returns to that shell's prompt. Each spawned window's root process is instead a single one-shot command that `syscall.Exec`s into `tmux attach-session`, with no parent interactive shell to fall back to.

#### Affected Surface

- **Affected:** every burst-spawned (N−1 external) window on the native Ghostty adapter. Both burst entry points route through the same `internal/spawn` composition + adapter.
- **Not affected:** the trigger window; single-session `portal open`/attach (run from an existing shell); detection, pre-flight, the ack channel, and selection mutation — all correct. This is purely post-exit window lifecycle.

#### Severity & Release

Low — a cosmetic/UX rough edge with no data loss. Ships as a regular release, not a hotfix.

---

### Root Cause

Two design choices compose into the bug; neither alone is the whole cause.

1. **The spawned command is a single, one-shot argv with no shell fallback.** `internal/spawn` composes the spawned window's command as an env-self-sufficient argv — `/usr/bin/env -u TMUX -u TMUX_PANE PATH=<picker PATH> <exePath> open --session <name> --ack <batch>:<token>` — deliberately a *real argv run verbatim* (so it drops cleanly into config-`terminals.json` recipes). Ghostty runs that command string via `bash -c`, so the window's root process is a single non-interactive command with no surrounding interactive shell.

2. **`portal open` `syscall.Exec`s into tmux.** Outside tmux (the spawned window is always outside tmux — composition strips `TMUX`/`TMUX_PANE`), `portal open --session` writes its ack marker, then `AttachConnector.Connect` `syscall.Exec`s into `tmux attach-session`, *replacing* the process image. The tmux client becomes the window's one-and-only process — no Portal or shell frame is left behind.

3. **The Ghostty adapter opens the window with `wait after command:true`.** When the session exits/detaches and the exec'd tmux client returns, the command is finished. Because of `wait after command:true`, Ghostty holds the dead window awaiting a keypress ("Process exited. Press any key to close the terminal.") instead of closing or dropping to a shell. This flag was introduced by the `ghostty-spawn-zero-windows` fix (2026-07-16) as "the normal-detach window lifecycle for a spawned session" — which also explains why the dead-end appeared only after the multi-window spawn feature started working.

**Net effect:** the spawned window's exec chain (`bash -c` → `env` → `portal open` → `tmux attach-session`) has no parent to return to when tmux exits, and `wait after command:true` converts "command finished" into a keypress dead-end. The trigger window is exempt only because it self-attaches from an already-running Portal that is a child of the user's interactive login shell.

#### Contributing / Why-Not-Caught (context)

- The osascript / Ghostty boundary has no automated coverage — the only real-Ghostty test is `//go:build manual`.
- The dead-end is a *post-exit lifecycle* behaviour, not a spawn failure: the burst succeeds (windows open, acks land, `spawn: opened N/N`), and the defect only appears later when the user exits the session — outside anything the spawn feature tests or logs observe.
- `wait after command:true` was newly composed days before the report; its interaction with the one-shot exec chain was never exercised end-to-end on a real Mac.

_(This section is contextual background for planning; the fix itself is specified in the following sections.)_

---

## Working Notes

_Optional - capture in-progress discussion if needed._
