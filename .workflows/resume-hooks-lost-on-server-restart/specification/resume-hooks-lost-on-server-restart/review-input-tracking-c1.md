---
status: in-progress
created: 2026-04-02
cycle: 1
phase: Input Review
topic: resume-hooks-lost-on-server-restart
---

# Review Tracking: resume-hooks-lost-on-server-restart - Input Review

## Findings

### 1. `hooks rm` command not mentioned in Component Changes

**Source**: Investigation, Blast Radius section (line 113-114) + cmd/hooks.go lines 133-164
**Category**: Enhancement to existing topic
**Affects**: Component Changes section

**Details**:
The `hooks rm` command uses `requireTmuxPane()` / `$TMUX_PANE` to identify which hook to remove, and also deletes the volatile marker using the pane-ID-based `MarkerName(paneID)`. It needs the same structural key change as `hooks set`. The specification's Component Changes section covers registration (`hooks set`), execution, storage, clean, and volatile markers, but omits the removal path.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. `hooks list` command and `Hook` struct not mentioned

**Source**: Investigation, Blast Radius section (line 113-114) + store.go lines 16-19, 99-125
**Category**: Enhancement to existing topic
**Affects**: Component Changes section

**Details**:
The `Hook` struct has a `PaneID` field, and the `List()` method populates it. The `hooks list` CLI command outputs this field. Both need updating to use structural keys. The specification's storage model section describes the key format change but doesn't mention the `Hook` struct, `List()` method, or `hooks list` command output.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 3. `ListPanes` interface change needed for structural key lookup

**Source**: Investigation, Code Trace section (line 57-65) + executor.go lines 6-8, 78-93
**Category**: Enhancement to existing topic
**Affects**: Component Changes - Hook execution section

**Details**:
The specification says ExecuteHooks should "query the session's panes with their window/pane indices and look up hooks by `sessionName:windowIndex.paneIndex`." However, the current `ListPanes(sessionName)` interface only returns pane IDs (`[]string` of `%N` values). The specification doesn't address that either `ListPanes` needs to return richer data (window index, pane index per pane) or a new query method is needed. This is also relevant to `CleanStale` which needs to build structural keys from live tmux state.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 4. Session name durability assumption unstated

**Source**: Investigation, Root Cause section (lines 87, 99) + Fix Direction Discussion (lines 155-158)
**Category**: Gap/Ambiguity
**Affects**: Storage Model section, Behavioral Requirements section

**Details**:
The investigation notes session names use `{project}-{nanoid}` format and are "non-durable" (line 99). The structural key approach depends entirely on session names surviving tmux-resurrect. The investigation's Fix Direction confirms resurrect preserves session names (line 143: "session names, window indices, and pane indices DO survive resurrect"), but the specification never explicitly states this assumption or why it holds. Given that the entire bug stems from a false assumption about pane ID persistence, it would be prudent to explicitly state the verified assumption about session name persistence.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:
