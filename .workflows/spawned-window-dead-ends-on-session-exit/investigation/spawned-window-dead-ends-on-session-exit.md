# Investigation: Spawned Window Dead-Ends on Session Exit

## Symptoms

### Problem Description

**Expected behavior:**
When a session running inside a burst-spawned host-terminal window exits (e.g. the
user quits Claude / the session detaches), the window should land back at a usable
interactive shell prompt — behaving "as if I opened Ghostty myself" — or close
cleanly. The same clean landing the *trigger* window gets.

**Actual behavior:**
The spawned (N−1 external) windows dead-end on tmux/terminal end-of-command text
when their session exits/detaches:

```
[detached (from session agentic-workflows-codify)]

Process exited. Press any key to close the terminal.
```

Pressing any key then closes the window. An awkward dead-end rather than a prompt.

### Manifestation

- Terminal shows `[detached (from session <name>)]` then `Process exited. Press any
  key to close the terminal.`
- Requires a keypress to dismiss.
- **Asymmetry is the key signal:** only the *spawned* N−1 burst windows show this.
  The trigger window (where the multi-selection happened) lands back at a normal
  shell prompt when its session exits.

### Reproduction Steps

1. In the TUI Sessions page, multi-select 2+ sessions (`m`) and Enter to dispatch a
   burst.
2. Burst opens N−1 external host-terminal (Ghostty) windows + reuses the trigger.
3. In one of the *spawned* windows, exit the process running in the session (quit
   Claude, or detach the session).
4. Observe: spawned window dead-ends on "Process exited. Press any key to close."
   The trigger window, by contrast, drops back to a shell prompt.

**Reproducibility:** Always (for spawned windows), per the report.

### Environment

- **Affected environments:** local (macOS)
- **Browser/platform:** macOS, Ghostty host terminal
- **User conditions:** Multi-select burst spawn (N≥2). Observed after the
  multi-window spawn feature started working (post recent patch).

### Impact

- **Severity:** Low (cosmetic/UX rough edge — no data loss)
- **Scope:** Anyone using multi-select burst spawn on the affected terminal
- **Business impact:** Spawned windows feel broken / half-finished on exit;
  inconsistent with the trigger window's clean landing.

### References

- Seed: `seeds/2026-07-17-spawned-window-dead-ends-on-session-exit.md` (inbox:bug,
  captured 2026-07-17)
- Discovery: `discovery/sessions/session-001.md`

---

## Analysis

### Initial Hypotheses

**Hypothesis carried from discovery/seed (a lead, NOT a confirmed cause):**
Each spawned window runs the burst-composed argv as the window's *sole* command,
with no surrounding interactive shell. The command `syscall.Exec`s (or otherwise
execs) into `tmux attach-session`, making the tmux client the window's one-and-only
process. When the session exits/detaches, the client returns, the window's command
is finished, and the terminal has nothing left to fall back to — hence "Process
exited. Press any key to close." The trigger window avoids this because portal was
launched there from an existing interactive login shell, so exiting attach returns
control to the parent shell and its prompt.

**Caveat to reconcile:** the seed/discovery notes describe the spawned command as
`portal attach <session> --spawn-ack …` and reference `cmd/attach.go` /
`AttachConnector`. Per current project docs, `portal attach` is retired — spawned
burst windows now run `portal open --session <name> --ack <batch>:<token>`. The
investigation must trace the *current* command composition and exec path, not the
historical one.

### Prior Context (knowledge base)

**`ghostty-spawn-zero-windows` (investigation + spec, 2026-07-16) — highly relevant.**
That fix rewrote the Ghostty adapter's AppleScript to the sdef-correct form:

```applescript
tell application "Ghostty"
	new window with configuration {command:"%s", wait after command:true}
end tell
```

The spec explicitly documents `wait after command:true` as intentional — *"keeps the
window up after its command exits, the normal-detach lifecycle for a spawned
session."* This is the strongest candidate for the mechanism producing "Process
exited. Press any key to close the terminal." — that is Ghostty's `wait-after-command`
end-of-command message. It also matches the seed's "observed after the multi-window
spawn feature started working (post recent patch)": this bug is a direct consequence
of that 2026-07-16 fix landing.

**`restore-host-terminal-windows` (spec, 2026-07-11) — adapter contract.** The spawn
layer *composes the command* and hands `{command}` to the adapter verbatim; the
adapter only opens a window running that command (`OpenWindow(command)`). So the fix
lever — if it's in the composed command rather than the terminal config — lives in
`internal/spawn` command composition, not in the adapter.

### Code Trace

_(to be filled during code analysis — confirm the actual current command
composition + exec path; the seed's `portal attach` references are stale, current is
`portal open --session … --ack …`.)_

### Root Cause

_(to be determined)_

---

## Fix Direction

_(to be determined — user has a preferred outcome: spawned window lands in a real
interactive shell like the trigger; no fixed mechanism. Feasibility/approach
questions were explicitly deferred to investigation. User also wants
correctly-written test commands, sandboxed on a throwaway `-L` tmux socket — never
the live server, which hosts ~31 real sessions.)_

---

## Notes

- **Sandbox rule:** any validation commands must run on a throwaway `-L <socket>`
  tmux server, never the live default server (~31 real sessions). Earlier Ghostty
  spawn misfires must not be repeated.
