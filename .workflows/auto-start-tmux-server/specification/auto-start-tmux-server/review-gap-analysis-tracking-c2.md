---
status: complete
created: 2026-03-19
cycle: 2
phase: Gap Analysis
topic: auto-start-tmux-server
---

# Review Tracking: auto-start-tmux-server - Gap Analysis

## Findings

### 1. Session wait phase activation condition undefined -- "fast path" contradicts "always call"

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Bootstrap Mechanism, Timing, User Experience, Error Handling & Edge Cases

**Details**:
The spec creates a contradiction between three statements:

1. **Bootstrap Mechanism / Detection**: "Always call it; skip the detection step entirely" -- `tmux start-server` runs unconditionally.
2. **Error Handling table**: "tmux already running | No bootstrap needed, fast path" -- implies something is skipped.
3. **Timing**: "Minimum 2 seconds -- prevents a jarring flash if sessions appear very quickly."

If the session wait phase (phase 2) always activates, then every normal Portal launch (tmux already running, sessions already present) would show "Starting tmux server..." for a minimum of 2 seconds. This is clearly not the intent -- the error table calls this the "fast path."

But the spec never defines when the session wait phase activates vs. is skipped. The likely intent is:

- **Phase 1 (server start)**: Always runs (idempotent, instant).
- **Phase 2 (session wait + UX)**: Only activates when no sessions exist at the time phase 2 begins. If `tmux list-sessions` returns sessions immediately, skip the interstitial/stderr message entirely.

This distinction is critical because it determines whether the interstitial is the TUI's initial state on every launch (bad UX) or only on cold-start launches. An implementer would need to make this design decision themselves.

The min 2-second bound also needs context: it applies only when the wait phase activates (no sessions yet), not on every launch. This should be explicit.

**Proposed Addition**: Reintroduced server detection, reduced min to 1s, updated error table

**Resolution**: Approved
**Notes**: Reverted "always call start-server" — bootstrap only runs when server not detected. Min bound reduced from 2s to 1s per user preference. Error table row updated to clarify fast path skips bootstrap entirely.
