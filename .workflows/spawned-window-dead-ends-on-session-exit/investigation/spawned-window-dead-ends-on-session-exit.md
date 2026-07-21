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

**Confirmed:** the seed's `portal attach` references are stale. The current spawned
command is `portal open --session <name> --ack <batch>:<token>`. Everything else in
the seed's hypothesised mechanism holds.

**1. What the spawned window actually runs (command composition).**
`internal/spawn/command.go:47` `composeOpenArgv` builds, for an ATTACH surface, the
env-self-sufficient argv:

```
/usr/bin/env -u TMUX -u TMUX_PANE PATH=<path> <exePath> open --session <name> --ack <batch>:<token>
```

This is a **real argv, never shell syntax** (by design — it's substituted verbatim
into config-`terminals.json` recipes too). `renderCommandString`
(`internal/spawn/recipe.go:110`) POSIX-single-quotes each element and space-joins
them into the `{command}` string.

**2. How Ghostty runs it (the native adapter).**
`internal/spawn/ghostty.go:20-22` — the adapter opens the window via:

```applescript
tell application "Ghostty"
	new window with configuration {command:"<embedded>", wait after command:true}
end tell
```

Per the file's own comment (`ghostty.go:24-40`): **Ghostty runs the `command` string
via `bash -c`**, which word-splits it. So the window's root process is effectively
`bash -c "/usr/bin/env … <exe> open --session <name> --ack …"` — a **single**
command, so `bash -c` exec-optimises (replaces itself with the command chain) rather
than staying as a parent. There is **no surrounding interactive shell**.

**3. The exec into tmux (why nothing is left behind).**
`bash -c` → `env` → `portal open --session`. In `cmd/open.go`, `openResolved`
(`:360`) hits the `*resolver.SessionResult` arm, writes the ack marker
(`writeAckMarker`, `:369` — the LAST act before handoff), then calls
`openSessionFunc` → `AttachConnector.Connect` (`:107`). Outside tmux this does
`syscall.Exec("tmux", ["tmux","attach-session","-t","=<name>"], os.Environ())`
(`:124-131`) — **replacing the process image with the tmux client**. The tmux client
is now the window's one-and-only process.

**4. Session exit / detach → dead-end.**
When `tmux attach-session` returns (session detach — `[detached (from session …)]` —
or session end), the exec'd process exits. `bash -c` is done. Because the adapter set
`wait after command:true`, Ghostty does **not** close the window and does **not** drop
to a shell — it shows its wait-after-command end-of-command prompt: **"Process exited.
Press any key to close the terminal."** A keypress then closes the window. This is the
reported dead-end, verbatim.

**5. Why the trigger window is exempt (the asymmetry).**
The trigger window is **never spawned via the Ghostty adapter and never runs
`composeOpenArgv`**. It self-connects **in-process**:
`cmd/open_burst_run.go:267` calls `deps.Connector.Connect(trigger.Value)` from the
already-running portal process — a child of the user's **existing interactive login
shell** (bare-shell → `AttachConnector`/`syscall.Exec`; inside-tmux →
`SwitchConnector`/`switch-client`). When tmux exits, control returns to that parent
interactive shell → prompt. The spawned windows have no such parent: their root
process is a one-shot `bash -c` that gets replaced by the exec chain, so there is
nothing to fall back to.

**Key files involved:**
- `internal/spawn/command.go` — `composeOpenArgv`, the spawned `open` argv (the
  single command with no shell fallback).
- `internal/spawn/ghostty.go` — `wait after command:true` (holds the dead window
  with the "Press any key" prompt instead of closing/dropping to a shell).
- `internal/spawn/recipe.go` — `renderCommandString`/`shellQuote` (the shared
  `{command}` rendering; also drives config adapters).
- `cmd/open.go` — `AttachConnector.Connect` `syscall.Exec` into tmux; `writeAckMarker`
  ordering (relevant to any fix: the ack must still be written before handoff).
- `cmd/open_burst_run.go` — trigger self-connect (the exempt path).

### Root Cause

The burst-spawned host window runs the spawned session's `portal open --session …`
argv as its **sole, one-shot root process** (via Ghostty's `bash -c "<command>"`),
with **no surrounding interactive shell to fall back to**. `portal open` `syscall.Exec`s
into `tmux attach-session`, so the tmux client *becomes* that root process. When the
session exits/detaches, the process exits and the window's command is finished — and
because the Ghostty adapter opens the window with **`wait after command:true`**, the
terminal holds the dead window and shows "Process exited. Press any key to close the
terminal." instead of returning to a prompt.

The trigger window is exempt purely because it self-attaches from an already-running
portal that is a child of the user's interactive login shell — so exiting the attach
returns to that shell's prompt. The spawned windows lack that parent context.

**Why this happens:** two design choices compose into the bug. (a) The spawned command
is a *single* `open` invocation with no shell wrapper — reasonable for a "run this exact
argv" contract, but it means the exec chain has no parent to return to. (b) The Ghostty
adapter sets `wait after command:true` (added by `ghostty-spawn-zero-windows`,
2026-07-16, described there as "the normal-detach window lifecycle for a spawned
session"), which converts "command finished" into a "Press any key" dead-end rather than
a clean close or a shell. Neither alone is the whole bug; together they produce the
dead-end. This also explains "observed after the multi-window spawn feature started
working (post recent patch)" — the `wait after command:true` fix is that patch.

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
