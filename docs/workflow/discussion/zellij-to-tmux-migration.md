---
topic: zellij-to-tmux-migration
status: in-progress
date: 2026-02-10
---

# Discussion: Migrating from Zellij to tmux

## Context

ZW (Zellij Workspaces) has a concluded specification built around Zellij as the terminal multiplexer. The user has decided to switch to tmux. The core UX design, project memory, TUI architecture, CLI structure, and distribution approach are all multiplexer-agnostic and carry forward. The Zellij-specific integration layer needs reworking.

This discussion identifies what changes, what stays, and resolves the tmux-specific design decisions before revising the specification.

### References

- [ZW Specification](../specification/zw.md) - Current Zellij-based spec (concluded)
- [cx-design discussion](cx-design.md) - Original design discussion
- [zellij-multi-directory discussion](zellij-multi-directory.md) - Model pivot to workspace-centric
- [fzf-output-mode discussion](fzf-output-mode.md) - `zw list` and `zw attach`
- [git-root-and-completions discussion](git-root-and-completions.md) - Git root resolution, shell completions
- [tmux-session-managers-analysis](../research/tmux-session-managers-analysis.md) - Comparative analysis of tmux session managers

## Questions

- [x] What are the tmux equivalents for all Zellij session operations?
- [x] What happens to exited/resurrectable sessions (Zellij-native feature)?
- [x] How should the layout system work with tmux?
- [x] Should the tool be renamed (ZW = "Zellij Workspaces")?
- [ ] How does utility mode work with tmux?
- [ ] What session metadata can we display from outside tmux?
- [ ] How does process handoff (exec) work with tmux?
- [ ] What changes for the runtime dependency?

---

*Each question gets its own section below. Check off as concluded.*

---

## What are the tmux equivalents for all Zellij session operations?

### Context

The current spec references Zellij CLI commands throughout. Need verified tmux 3.6a equivalents.

### Decision

Verified against `man tmux` on the target system (tmux 3.6a):

| Operation | Zellij | tmux (verified) |
|---|---|---|
| Create or attach | `zellij attach -c <name>` | `tmux new-session -A -s <name>` (alias: `new`) |
| Create w/ start dir | cd + create | `tmux new-session -A -s <name> -c <dir>` |
| Attach to existing | `zellij attach <name>` | `tmux attach-session -t <name>` (alias: `attach`) |
| List sessions | `zellij list-sessions` | `tmux list-sessions` (alias: `ls`) |
| Kill session | `zellij kill-session <name>` | `tmux kill-session -t <name>` |
| Delete exited session | `zellij delete-session <name>` | N/A — tmux sessions don't persist after exit |
| Check session exists | N/A | `tmux has-session -t <name>` (alias: `has`) — exit 0/1 |
| Query tab/window names | `zellij --session <name> action query-tab-names` | `tmux list-windows -t <name>` (alias: `lsw`) |
| Rename session | `zellij action rename-session <new>` (inside only) | `tmux rename-session -t <name> <new>` (alias: `rename`) — works from outside |

**Key differences from Zellij:**
- tmux `new-session -A` combines create-or-attach in one command
- tmux supports `-c <dir>` to set the working directory at creation — no need to `cd` first
- tmux `rename-session` works from outside the session (Zellij could only rename from inside)
- tmux `has-session` provides a clean existence check (useful for argument resolution)
- tmux `list-sessions` output is structured, no ANSI codes to strip

**Directory change for new sessions**: The spec's current model (cd to dir, then create) simplifies to just passing `-c <resolved-dir>` on `new-session`. No directory change needed in ZW's process. Git root resolution still applies — resolve first, then pass to `-c`.

---

## What happens to exited/resurrectable sessions?

### Context

The Zellij spec had an "EXITED" section in the TUI showing dead-but-resurrectable sessions. Zellij natively persists session state after exit and allows individual session resurrection. tmux doesn't — sessions are alive or gone.

### Journey

Initially explored whether tmux-resurrect (installed on target system with tmux-continuum auto-saving every 10 min) could fill this gap. Research found:

- Resurrect stores snapshots at `~/.local/share/tmux/resurrect/` as tab-delimited text files
- The `last` symlink points to the most recent save
- Files contain session names, window info, working directories, running commands
- Detection is possible via directory existence or `tmux list-keys | grep resurrect`

However, resurrect's restore is **all-or-nothing** — it restores the entire saved state, not individual sessions. This is fundamentally different from Zellij's model where exited sessions are individually addressable objects.

**Key realisation**: Resurrect is disaster recovery (machine crash, tmux server dies), not a session management workflow. Users don't interact with dead sessions — they either have running sessions or they don't. The "exited sessions" concept was a Zellij-specific feature that doesn't map to tmux's model.

### Decision

**Drop the EXITED section entirely.**

- TUI shows only running sessions + new session option
- No resurrect integration — it's outside ZW's scope
- `zw clean` simplifies to only cleaning stale projects (directories that no longer exist on disk) — no "delete exited sessions" operation
- `zellij delete-session` has no tmux equivalent and is removed from the command mapping

---

## How should the layout system work with tmux?

### Context

The Zellij spec included a layout picker during new session creation — users could choose from `.kdl` layout files. tmux has no equivalent single-file layout format.

### Options Considered

**A. Drop layout selection entirely** — always create single-window sessions. Users split/arrange after attaching. Simplest path.

**B. tmux built-in layout strings** — `even-horizontal`, `main-vertical`, etc. Limited to pane arrangements, no commands per pane.

**C. Shell scripts as layouts** — small scripts that create windows/panes. How tmuxinator/smug work conceptually. Flexible but adds complexity.

**D. Structured layout config** — YAML/TOML files defining windows and panes. Essentially reimplementing tmuxinator inside ZW.

### Decision

**A — Drop layouts completely.** Not "defer to later" — not needed.

The core use case is SSH from phone → pick session → get working. Fastest path to a session is the most valuable thing. Layout selection was already optional in the Zellij spec (skipped when no custom layouts existed).

This simplifies the new session flow significantly. When starting a session in a saved project with no other prompts needed, session creation is immediate — no layout picker step.

**Spec impact:**
- Remove Layout Discovery section
- Remove layout picker from new session flow
- Remove `--layout` flag from session creation command
- New session flow for saved projects becomes: select project → session created immediately

---

## Should the tool be renamed?

### Context

"ZW" stands for "Zellij Workspaces" — no longer fits after switching to tmux.

### Options Considered

**tmux-flavoured (2 chars):**
- `tx` — "tmux execute/extend". Clear tmux connection, no known CLI conflicts.
- `tw` — "tmux workspaces". Direct successor to "zw". Keys on different rows.
- `tm` — "tmux manager". Could be confused with Time Machine on macOS.

**z-prefix (familiar territory):**
- `zx` — Already appeared in project history. Adjacent bottom-row keys, fast. Google has an npm `zx` tool but it's a Node package — no real conflict.
- `zz` — Same key twice, fastest possible. No CLI conflicts. But no semantic meaning, and `zz` is a vim motion.

**Other:**
- `mux` — 3 chars but immediately clear. "I'm a multiplexer tool." Distinctive, no conflicts.
- `sx` — "session execute". Short but anonymous.

### Journey

User wanted something that acknowledges tmux without hiding it. Considered keyboard ergonomics — already has `cx` for Claude Code, `c` for Composer. The `z` prefix had history from the Zellij era but no longer carried meaning.

`tx` was appealing for being 2 chars with clear tmux association. But `mux` stood out — instantly communicable, self-documenting, and can be aliased to anything shorter (e.g., `alias x=mux`) per user preference.

### Decision

**Rename to `mux`.**

- Command: `mux`
- Config location: `~/.config/mux/`
- Homebrew formula: `mux`
- Repository: rename from `zw` to `mux`
- 3 chars is fine — user will alias to a single char if needed

**Spec impact:** Global find-replace of `zw` → `mux` throughout. All CLI commands become `mux`, `mux .`, `mux list`, `mux attach`, etc.

---
