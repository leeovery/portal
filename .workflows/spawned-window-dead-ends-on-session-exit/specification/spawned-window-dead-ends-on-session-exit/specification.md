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

## Working Notes

_Optional - capture in-progress discussion if needed._
