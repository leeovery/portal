---
topic: session-launch-command
status: in-progress
date: 2026-02-12
---

# Discussion: Session Launch with Command Execution

## Context

When launching a new tmux session via `x <project>`, the user wants the ability to also execute a command inside that session after it's created. Current `cx` hardcodes running `claude` (or `claude --continue`, `claude --resume` depending on variant). The new `portal`/`x` design should generalise this — any command can be passed through, and the whole thing should be aliasable.

### References

- [mux specification](../specification/mux.md)
- [x-xctl-split discussion](x-xctl-split.md)
- [cx-design discussion](cx-design.md)

## Questions

- [x] What CLI syntax for passing the command through?
- [x] Should the command replace the shell or run then drop to shell?
- [x] How does this interact with the TUI flow?
- [ ] Should projects.json support default commands per project?
- [ ] How does this relate to the existing cx variants (cx, cx c, cx r)?

---

## What CLI syntax for passing the command through?

### Options Considered

**A) Double-dash separator**: `x myproject -- claude --resume`
- Standard POSIX convention — everything after `--` is not parsed as flags
- Handles compound commands with their own flags cleanly, no quoting
- Slightly more verbose

**B) Explicit flag**: `x myproject --exec "claude --resume"` / `x -e "claude --resume" myproject`
- Self-documenting
- Aliases naturally: `alias xc='x -e claude'` → `xc myproject` works
- Needs quoting for compound commands

**C) Positional after known args**: `x myproject claude --resume`
- Shortest but ambiguous — is `claude` a project or a command?
- Fragile, rejected immediately

### Journey

Started with double-dash as the obvious POSIX choice. It handles arbitrary compound commands cleanly without quoting. But when exploring aliasing, a problem surfaced: with `--`, the project must come *before* the separator, so `alias xc='x -- claude'` then `xc myproject` expands to `x -- claude myproject` — `myproject` becomes an arg to claude, not to x.

Shell functions solve this (`xc() { x "${1:-.}" -- claude; }`) but that's a step up from simple aliases. The `--exec`/`-e` flag aliases naturally because the flag and its value stay together, with the project as a trailing positional.

Realised the two aren't mutually exclusive — both just express "run this command after session creation." Both normalise to the same internal representation: a `[]string` of command + args.

### Decision

**Support both `-e` and `--`.** They're mutually exclusive (error if both provided). Both resolve to the same internal exec command slice early in CLI parsing. Single downstream code path.

- `-e` / `--exec` for simple commands and clean aliasing: `x -e claude myproject`
- `--` for compound commands with flags, no quoting: `x myproject -- claude --resume --model opus`

---

## Should the command replace the shell or run then drop to shell?

### Options Considered

**A) Run then drop to shell** — command runs, when it exits you land in zsh at the project dir. Session stays alive.

**B) Replace shell (exec)** — command exits, session dies.

**C) User decides via flag** — default A, add `--replace` for B.

### Journey

Current `cx` uses A: `zsh -ic '$cmd; exec zsh'`. If claude crashes or you ctrl+c, you don't lose the session. Option C adds surface area for a niche case — if someone really wants exec-and-die they can pass `exec claude` as the command itself.

### Decision

**A — run then drop to shell.** No flag needed. Users wanting exec behaviour can literally pass `exec` as part of the command: `x myproject -- exec claude`.

---

## How does this interact with the TUI flow?

### Context

When no project is specified, `x` opens the TUI picker. What happens when a command is specified but no project?

### Options Considered

**A) TUI opens, command applies after selection** — the command is "sticky" through the selection flow. Pick project → session created → command runs.

**B) Error — require project when command specified.**

### Journey

B would kill the alias use case. `alias xc='x -e claude'` then just `xc` to get picker + claude would fail. The command is an instruction for what to do *after* session creation, orthogonal to *how* the project was chosen (argument, alias, zoxide, TUI).

Confirmed this means `xc` (→ `x -e claude`) works consistently:
- `xc` → TUI → pick project → claude
- `xc myproject` → direct → claude
- `xc .` → current dir → claude

### Decision

**A — command is orthogonal to project resolution.** TUI opens normally, command applies after selection. Consistent behaviour regardless of how the project was resolved.

---
