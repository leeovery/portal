---
status: in-progress
created: 2026-03-19
cycle: 1
phase: Input Review
topic: auto-start-tmux-server
---

# Review Tracking: auto-start-tmux-server - Input Review

## Findings

### 1. "tmux not installed" edge case not in error handling table

**Source**: ideas/auto-start-tmux-server.md - Edge cases section
**Category**: Enhancement to existing topic
**Affects**: Error Handling & Edge Cases

**Details**:
The idea doc explicitly lists "tmux not installed: existing `CheckTmuxAvailable` error handles this" as an edge case. The spec's error handling table covers four scenarios (continuum + saved sessions, continuum + no data, no continuum, tmux already running) but doesn't address the case where tmux itself isn't installed. Since bootstrap introduces a new step that runs `tmux start-server`, the spec should clarify that this is gated behind the existing tmux-availability check — i.e., bootstrap doesn't attempt to start a server if tmux isn't even present. The ordering relationship matters: availability check first, then bootstrap.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:
