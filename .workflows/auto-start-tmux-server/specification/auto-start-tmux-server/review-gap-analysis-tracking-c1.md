---
status: in-progress
created: 2026-03-19
cycle: 1
phase: Gap Analysis
topic: auto-start-tmux-server
---

# Review Tracking: auto-start-tmux-server - Gap Analysis

## Findings

### 1. Bootstrap placement relative to TUI lifecycle is ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Bootstrap Mechanism, User Experience

**Details**:
The spec says bootstrap is a "shared function called early by every Portal command" (suggesting PersistentPreRunE), and separately describes a TUI loading interstitial ("blank screen with centered text"). These are architecturally incompatible without clarification. PersistentPreRunE runs before the Bubble Tea program starts -- there's no screen to render an interstitial on. Either: (a) bootstrap runs in PersistentPreRunE and the interstitial is the first state of the TUI model, or (b) bootstrap logic lives inside the TUI model's Init/Update. The spec needs to clarify whether the "shared function" handles server start only (with the TUI owning its own loading state), or whether it encompasses the full wait-with-feedback flow. This directly determines the implementation architecture.

**Proposed Addition**: Two-phase ownership section added to Bootstrap Mechanism

**Resolution**: Approved
**Notes**: Added as "Two-phase ownership" subsection under Bootstrap Mechanism

---

### 2. Session detection mechanism during wait period is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Timing, User Experience

**Details**:
The spec says "transition out of the loading state as soon as sessions are detected" with min/max bounds. But it never specifies how sessions are detected during the wait. For the TUI path, it mentions "sessions appear naturally via the TUI's refresh cycle" -- but the TUI refresh cycle is an existing mechanism for listing sessions periodically, not a wait-loop. For the CLI path, there is no refresh cycle at all. The spec needs to define: What command is polled? At what interval? Is it `tmux list-sessions`? Every 250ms? Every second? Without this, an implementer must guess both the detection method and the polling frequency, which directly affect responsiveness and system load.

**Proposed Addition**: Detection method added to Timing section

**Resolution**: Approved
**Notes**: Added poll method (tmux list-sessions, 500ms) with TUI using existing refresh cycle

---

### 3. "Server not running" detection method is ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Bootstrap Mechanism

**Details**:
The spec says: "Detection: `tmux list-sessions` failing (or equivalent check) indicates no server is running." Two issues: (1) "or equivalent check" leaves the actual mechanism undefined, and (2) `tmux list-sessions` fails both when no server exists AND when the server has no sessions -- these are different states requiring different responses. Since `tmux start-server` is idempotent (no-op if server already running), the spec could simplify by always calling `start-server` as the detection+action in one step, bypassing the ambiguity entirely. An implementer needs to know the exact detection approach.

**Proposed Addition**: Replaced Detection line with idempotent start-server approach

**Resolution**: Approved
**Notes**: Simplified — always call start-server, no detection needed

---

### 4. Commands that skip tmux check vs. bootstrap not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Bootstrap Mechanism

**Details**:
The spec says bootstrap is "called early by every Portal command." The codebase has a `skipTmuxCheck` set (version, init, help, alias, clean) where commands bypass the tmux availability check in PersistentPreRunE. The spec doesn't address whether these commands also skip bootstrap. Logically they should (they don't need tmux), but since the spec says "every Portal command" without qualification, an implementer would need to decide. Minor in practice since the existing skip pattern is clear, but the spec should be internally consistent.

**Proposed Addition**: Updated Trigger line to qualify "every command that requires tmux"

**Resolution**: Approved
**Notes**: Clarified skip-check commands also skip bootstrap

---

### 5. Bootstrap one-shot vs. session-detection polling distinction is unclear

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Bootstrap Mechanism, Timing

**Details**:
The spec repeatedly says bootstrap is "one-shot" and "no retry loop," but the timing section describes waiting up to 6 seconds with session-detection that transitions early. These are two different things: (1) starting the server (one-shot), and (2) waiting for sessions to appear (polling with bounds). The spec conflates them under "bootstrap." An implementer could reasonably read "one-shot, no retry loop" as meaning no waiting at all. The spec should clearly separate "server start" (one-shot, fire-and-forget) from "session wait" (bounded poll) as distinct phases of the bootstrap flow.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 6. CLI path end-to-end flow after bootstrap wait is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: User Experience

**Details**:
For the CLI path, the spec says: print stderr message, block briefly, then proceed. Consider `x list` after reboot: bootstrap starts server, waits 2-6s for sessions, then... does the list command re-query sessions? The spec describes the wait period but not the handoff. If bootstrap waits and detects sessions, does it pass them to the command? Or does the command just re-run `list-sessions` after the wait completes? This matters for implementation -- the bootstrap function's return type and the command's awareness of the wait both depend on this.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:
