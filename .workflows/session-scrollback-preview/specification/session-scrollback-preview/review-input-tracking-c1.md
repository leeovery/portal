---
status: in-progress
created: 2026-05-06
cycle: 1
phase: Input Review
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Input Review

## Findings

### 1. Override of research-locked "in-preview stepping" not traced in spec

**Source**: Discussion lines 716-719 ("Overrides a research-locked Stated Feature Shape constraint. Documented here so the override is traceable."), lines 24-27 (Locked Feature Shape parenthetical noting the override), and lines 1095-1098 (Summary: "The override of the research-locked 'in-preview stepping' constraint in favour of the Esc → arrow → Space loop is the only material deviation from the Stated Feature Shape; it is documented in two places...")
**Category**: Enhancement to existing topic
**Affects**: § No In-preview Between-Session Stepping

**Details**:
The discussion makes a deliberate point of flagging this as the *only* material deviation from the research-locked Stated Feature Shape, and documents the override in two places explicitly for traceability. The spec describes the no-stepping decision and its cascading consequences correctly, but does not mark it as overriding a previously-locked constraint. This matters because future readers (including the build phase or future spec revisions) lose the signal that an earlier constraint was deliberately reversed — without this traceability, someone re-reading the research could re-introduce the original "Claude Code resume-style stepping" assumption.

**Current**:
> ### No In-preview Between-Session Stepping
>
> Preview is bound to **one session per open**. There are no key bindings inside preview that move between candidate sessions.
>
> **Cascading consequences (anchored here so the spec is self-contained):**
> ...
> **Reversibility.** Adding in-preview between-session stepping later is additive (a new keymap entry plus cursor-sync semantics), not a rewrite. It is intentionally out of scope for v1.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: Added "Override traceability" paragraph at the end of § No In-preview Between-Session Stepping.

---

### 2. Memory footprint trade-off at N=1000 not pinned

**Source**: Discussion lines 466-469 ("Memory: holds N lines per previewed pane during preview. Negligible at N≤1000.")
**Category**: Enhancement to existing topic
**Affects**: § History Depth

**Details**:
The discussion's History Depth decision explicitly accepts a memory trade-off and pins it as "negligible at N≤1000". The spec captures the upper bound on slice size and the cost-decoupling property of the tail-N read, but doesn't restate the memory-footprint trade-off acceptance. Minor but it's part of the trade-offs-accepted list in the discussion's decision and helps future reviewers understand why N=1000 (and not, say, N=10000) is the cap.

**Current**:
> **Slice size.** The last **N lines** of the pane's `.bin` file are read on each focus event, where **N = 1000**. This pins the working figure of "generous N (e.g. ~500–1000 lines)" at the upper end. The exact value is a constant in the read pipeline.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 3. CapturePane shared with save-daemon semantics — rationale for "no new tmux wrapper"

**Source**: Discussion lines 121-123 ("The existing `CapturePane` hardcodes `-S -` and is shared with save-daemon semantics; a bounded variant (`CapturePaneTail(target, n)`) would be net-new.")
**Category**: Enhancement to existing topic
**Affects**: § Source of Preview Bytes / § Cross-cutting Seams › State Package API Reuse

**Details**:
The discussion's Source of Preview Bytes options analysis grounds the "no new tmux wrapper" benefit of always-disk in a specific concrete fact: existing `CapturePane` hardcodes `-S -` and is shared with the save daemon, so a bounded variant (`CapturePaneTail`) would be a net-new addition that the always-disk choice avoids. The spec asserts "No new tmux wrapper" and "No new methods on `tmux.Client`" but doesn't anchor *why* the live path would have required a new wrapper. Useful context for the build phase (and for future readers considering re-introducing a live path).

**Current**:
> **Single read path consequences.**
> - No marker check; no rendering fork.
> - No per-preview tmux IPC.
> - The same path that already serves skeleton panes during restore is reused for hydrated panes.
> - The rapid-stepping race between two in-flight tmux captures cannot occur (file reads are microseconds and synchronous).

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 4. Real-world pane-count distribution rationale dropped

**Source**: Discussion lines 215-217 ("Real-world distribution sample is N=1 — 14 of 16 sessions on the user's machine are 1-pane (research F6) — so the dominant case collapses regardless of the choice. Decision matters only for the 2+ pane minority.") and the user quote at lines 263-267 ("95% of the time it's single window, single pane per session.")
**Category**: Enhancement to existing topic
**Affects**: § Multi-pane Rendering Shape

**Details**:
The discussion's Multi-pane decision is partly justified by an empirical observation: "14 of 16 sessions on the user's machine are 1-pane" + "95% of the time it's single window, single pane per session". The spec mentions "the dominant ~95% case" once in passing under degenerate cases but doesn't ground that figure in the actual distribution data, nor justify the rendering-shape choice via "Distribution kills the case for fidelity" (discussion line 270). This is part of the deciding-factors logic that the spec drops. Including it makes the v1 minimalism easier to defend in future revisions.

**Current**:
> **Degenerate cases.** In single-window single-pane sessions (the dominant ~95% case), all three cycle keys silently no-op. No flicker, no error feedback, just nothing.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 5. Self-documenting nature of preview as privacy mitigation

**Source**: Discussion lines 929-931 ("The behaviour is also self-documenting in use: the first time a user opens preview on a session with sensitive content, they see what preview shows. No mystery.")
**Category**: Enhancement to existing topic
**Affects**: § Privacy / Threat Model

**Details**:
The discussion's Privacy decision lists "self-documenting in use" as one of the deciding factors — the user discovers preview's exposure surface organically the first time they open it, eliminating the need for documentation. The spec captures the no-design-response decision and rationale (single-user developer tool, false sense of security argument, reversibility) but drops this self-documenting argument. Worth including because it pre-empts the obvious counter-argument "but shouldn't we at least document the exposure?" by explaining why doc would be redundant.

**Current**:
> **Rationale.** Portal is a single-user developer tool; the user is the operator and the audience. Mitigation of secret-exposure during sharing contexts (screen-shares, demos, OBS recording, pairing) is the user's responsibility — accomplished simply by not pressing `Space`. Redaction would create a false sense of security that is worse than no protection.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 6. F13 rapid-stepping race anchored to live-path option, not the spec's "cannot occur" assertion

**Source**: Discussion lines 117-120 ("F13 rapid-stepping race largely vanishes — the window where a slow capture overwrites a newer view is too small to matter when the read is file I/O.") and lines 138-141 (Option B con: "F13: rapid stepping → in-flight captures for session N landing after user has stepped to N+1; needs generation/sequence tokens to ignore stale replies.")
**Category**: Enhancement to existing topic
**Affects**: § Source of Preview Bytes › Single read path consequences

**Details**:
The spec asserts "The rapid-stepping race between two in-flight tmux captures cannot occur (file reads are microseconds and synchronous)." This is correct but flattened. The discussion's framing is more nuanced: in the live-capture path the race genuinely required generation/sequence tokens to mitigate (a real F13 mitigation cost the always-disk path eliminates entirely). The spec could note that this is one of the F13-mitigation costs the always-disk decision avoided rather than a property that simply doesn't exist. Minor — but the current phrasing slightly understates the architectural simplification.

**Current**:
> **Single read path consequences.**
> - No marker check; no rendering fork.
> - No per-preview tmux IPC.
> - The same path that already serves skeleton panes during restore is reused for hydrated panes.
> - The rapid-stepping race between two in-flight tmux captures cannot occur (file reads are microseconds and synchronous).

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
