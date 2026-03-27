---
status: complete
created: 2026-03-27
cycle: 1
phase: Gap Analysis
topic: resume-sessions-after-reboot
---

# Review Tracking: resume-sessions-after-reboot - Gap Analysis

## Findings

### 1. Command delivery mechanism to pane is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Execution Mechanics

**Details**:
The spec says "restart commands fire" in panes after reboot, but never specifies HOW a command is delivered to an existing pane. After tmux-resurrect restores layout, each pane contains a dead process — tmux replaces it with a shell. Portal needs to send the restart command into that pane. The two main options have different semantics:

- `tmux send-keys -t %3 "claude --resume abc123" Enter` — types the command into the pane's shell as if the user typed it
- `tmux respawn-pane -t %3 "claude --resume abc123"` — kills the current pane process and replaces it

An implementer would have to guess which approach to use. This is a critical implementation detail because it affects whether the user sees the command in their shell history, whether the pane's shell survives, and error recovery behavior.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Added send-keys as delivery mechanism to Execution Mechanics section.

---

### 2. Missing error behavior when $TMUX_PANE is absent

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Surface

**Details**:
The spec states pane ID is "inferred from `$TMUX_PANE`" for `hooks set` and `hooks rm`, but doesn't define what happens when this env var is missing (e.g., user runs `portal hooks set --on-resume "cmd"` from a bare shell outside tmux). Should it error? What error message? This is a required validation since the entire registry model depends on having a valid pane ID.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Added $TMUX_PANE validation to CLI Surface section.

---

### 3. Execution insertion point in connection flow is ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Execution Mechanics

**Details**:
The spec says hooks execute "during Portal's connection flow" via `portal open`, but doesn't specify:

1. **Which connection paths trigger it** — `portal open` has two code paths: TUI picker (selects existing session) and direct path (creates/attaches). The `portal attach` command is a separate connection flow. Should all three trigger hook execution? The `attach` command also connects to sessions and calls `bootstrapWait`.

2. **When in the flow** — Before or after connecting to the session? If after connection via `SwitchConnector`, the user is already in the session when hooks fire. If before, Portal can report what it's doing. If it's `AttachConnector` (syscall.Exec), Portal's process is replaced entirely — it can't do anything after connect.

The AttachConnector case (outside tmux) is particularly important: `syscall.Exec` replaces the process, so hook execution MUST happen before connect. But for SwitchConnector (inside tmux), either order could work.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Added insertion point and connection path details to Execution Mechanics.

---

### 4. hooks list output format undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Surface

**Details**:
The spec says `hooks list` "shows all registered hooks across all panes" but doesn't define the output format. The alias list uses `name=path\n` per line. Hooks have a different structure (pane ID + event type + command), so the alias format doesn't directly apply. An implementer needs to know the exact format to build it — and users/scripts need to know what to expect.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Added tab-separated output format to CLI Surface section.

---

### 5. hooks rm behavior when no hook exists for current pane

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Surface

**Details**:
The spec defines `hooks rm --on-resume` but doesn't state behavior when no hook is registered for the current pane. The existing `alias rm` pattern returns an error ("alias not found: X"). Should `hooks rm` follow the same pattern, or be a silent no-op? This matters for scripting — tools calling `hooks rm` in cleanup paths may not want an error if the hook was already removed.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Silent no-op — supports scripting use case. Added to CLI Surface section.

---

### 6. Multiple pane hooks execution ordering unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Execution Mechanics

**Details**:
A session can have many panes, each with registered hooks. The spec doesn't define whether hooks are executed sequentially or concurrently, and if sequential, in what order. It also doesn't address whether Portal waits for each command to "complete" or fires them all and moves on. Since the commands are being sent to tmux panes (not run by Portal's process), "completion" may not apply — but the ordering/timing gap remains. For most use cases fire-and-forget sequential is fine, but this should be stated.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Sequential fire-and-forget. Added to Execution Mechanics section.

---

### 7. hooks command tmux requirement unclear

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Surface, root.go integration

**Details**:
The `skipTmuxCheck` map in `root.go` controls which commands bypass the tmux server bootstrap. The spec doesn't state whether `hooks` should be in this map. `hooks set` and `hooks rm` need `$TMUX_PANE` (implies tmux is running). `hooks list` only reads a JSON file and doesn't need tmux at all. The `alias` command is in `skipTmuxCheck` because it only reads/writes files. Should `hooks` follow the same pattern? If so, `hooks set` would need its own `$TMUX_PANE` validation rather than relying on the tmux bootstrap check. The spec also needs to clarify whether `hooks set` needs the tmux client (it does — for setting the volatile server option).

**Proposed Addition**:

**Resolution**: Approved
**Notes**: hooks added to skipTmuxCheck, subcommands validate $TMUX_PANE themselves. Added to CLI Surface section.

