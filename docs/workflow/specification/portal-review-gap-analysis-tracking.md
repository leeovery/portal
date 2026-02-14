---
status: in-progress
created: 2026-02-14
phase: Gap Analysis
topic: Portal
---

# Review Tracking: Portal - Gap Analysis

## Findings

### 1. Command delivery to tmux session

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: tmux Integration > Command Execution in Sessions, Session Operations

**Details**:
The spec defines the shell incantation (`zsh -ic '<cmd>; exec zsh'`) and the tmux commands separately, but never connects them. An implementer needs to know that the command is passed as tmux's `shell-command` argument on `new-session` (e.g., `tmux new-session -A -s <name> -c <dir> "zsh -ic '<cmd>; exec zsh'"`). Also, the example hardcodes `zsh` — should use `$SHELL` or detect the user's shell.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Updated Command Execution in Sessions section with full tmux invocations (outside and inside tmux), using $SHELL instead of hardcoded zsh.

---

### 2. Query fallback when command is pending

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Interface > Query Resolution, x — The Launcher

**Details**:
Query resolution step 4 says: "Fall back to the main session picker with the query pre-filled as the filter text." But when a command is pending (`-e`/`--`), the TUI shows only the project picker (no session list). The fallback should be the project picker with the query as filter, not the session picker.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Updated step 4 to route to project picker when command is pending, session picker otherwise.

---

### 3. File browser hidden directory visibility

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: File Browser > Behavior

**Details**:
The file browser shows "directories only" but doesn't specify whether hidden directories (`.dotdirs`) are included. Many config directories (`.config`, `.ssh`) are hidden. Should they appear by default? Should there be a toggle?

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Hidden by default, `.` key toggles visibility, resets on next open.

---

### 4. Alias path expansion

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Configuration & Storage > Alias Storage, CLI Interface > xctl

**Details**:
When setting an alias via `xctl alias set m2api ~/Code/mac2/api`, is `~` expanded to the absolute path before storage? The aliases file shows absolute paths in examples. Should Portal expand `~` and resolve relative paths to absolute before writing? Without this, aliases with `~` would require shell expansion at resolution time.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Always expand ~ and resolve relative paths to absolute before storage.

---

### 5. File browser "select current directory" shortcut

**Source**: Specification analysis
**Category**: Ambiguity
**Affects**: File Browser > Behavior

**Details**:
The spec says: "Enter on `.` (current dir indicator) or dedicated shortcut (e.g., `Space`)". The `(e.g., Space)` is non-committal — an implementer must decide. Should be a concrete binding.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Space as concrete binding, Enter on `.` as alternative.

---

### 6. `portal` Direct Commands table completeness

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Interface > portal — Direct Commands

**Details**:
The "portal — Direct Commands" table lists only `open`, `init`, `version`, `help`. Since `xctl` passes through to `portal`, subcommands like `portal list`, `portal attach`, `portal kill`, `portal clean`, `portal alias` also exist but aren't in this table. The table could mislead readers into thinking portal only has 4 subcommands.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Added clarifying note after the table cross-referencing xctl section.

---

### 7. `xctl kill` error behavior

**Source**: Specification analysis
**Category**: Enhancement to existing topic
**Affects**: CLI Interface > xctl — The Control Plane

**Details**:
`xctl attach` documents error behavior ("No session found: {name}" + non-zero exit). `xctl kill` has the same failure mode (targeting non-existent session) but its error behavior is undocumented.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Added matching error behavior paragraph after attach errors.

---

### 8. `x <path>` with non-existent directory

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Interface > x — The Launcher, Query Resolution

**Details**:
When `x /nonexistent/path` is used, the path detection heuristic identifies it as a path (contains `/`). But the directory doesn't exist. What happens? Error message and exit? tmux would create a session but the shell would start in a fallback directory. The spec doesn't address invalid paths.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Broadened to cover all resolution sources (literal path, alias, zoxide). Validate after resolution, error if directory doesn't exist.
