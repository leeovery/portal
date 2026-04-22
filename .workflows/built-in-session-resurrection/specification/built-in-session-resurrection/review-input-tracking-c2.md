---
status: in-progress
created: 2026-04-21
cycle: 2
phase: Input Review
topic: built-in-session-resurrection
---

# Review Tracking: built-in-session-resurrection - Input Review

## Findings

### 1. Recursion-risk rationale for rejecting generic options capture omitted

**Source**: discussion §Save Content & Scope line 206 ("Items removed from inventory post-review" → "Deviating session options")
**Category**: Enhancement to existing topic
**Affects**: Scope & Constraints → Explicit Non-Goals → "Generic tmux option capture" bullet

**Details**:
The discussion grounds the rejection of generic options capture in two reasons: (a) complexity from diffing `show-options` against global defaults, and (b) "a recursion risk if Portal's own `set-hook -g` definitions were captured." The specification captures reason (a) but drops reason (b). The recursion-risk angle is a meaningful safety-grounded justification — Portal's own plumbing would otherwise appear in captured state and be "restored" on next boot, creating a feedback loop. Worth preserving as rationale so planning doesn't re-open the topic under a "just capture everything" framing.

**Current**:
> - **Generic tmux option capture** (session/window/pane options). Nearly all tmux options are set globally via `~/.tmux.conf` and apply on restore automatically. Per-session overrides are niche. Capturing them requires diffing `show-options` against global defaults — complexity not justified. If a specific flag becomes important, add it as an explicit field later.

**Proposed Addition**:
Append a sentence: "Also carries a recursion risk — Portal's own `set-hook -g` definitions would be captured and replayed on restore, creating a feedback loop on its own plumbing. If a specific flag becomes important, add it as an explicit per-window/per-session field later."

**Resolution**: Pending
**Notes**:

---

### 2. Ordering rationale for hook registration before `_portal-saver` creation missing the "initial-save-trigger" angle

**Source**: discussion §tmux Hook Registration Lifecycle / Ordering note line 641
**Category**: Enhancement to existing topic
**Affects**: Bootstrap Flow (Integrated) → Ordering Rationale / tmux Hook Registration Lifecycle → Scenario 1 ordering

**Details**:
The spec's Ordering Rationale (line 1016) explains why `@portal-restoring` must be set before `_portal-saver` is created — to prevent the daemon's first tick from capturing mid-restore. But the discussion notes a second, complementary ordering concern: hook registration must also happen **before** `_portal-saver` is created, because creating `_portal-saver` fires `session-created`, which — if hooks are already registered — runs `portal state notify` and trips the dirty flag. Under the current design this touch is suppressed by `@portal-restoring`, but the positive ordering rationale ("register hooks first so the save-daemon's first-tick pathway is intact from the moment the daemon exists") is captured in the discussion and not in the spec. Minor but useful grounding for planning.

**Proposed Addition**:
Extend the Ordering Rationale section to note: "Hook registration (step 2) similarly precedes `_portal-saver` creation (step 4). Creating `_portal-saver` fires a `session-created` event; with hooks already registered, the notify pathway is intact from the daemon's very first moment of existence. The `@portal-restoring` marker suppresses the initial capture, but the ordering keeps the hook pipeline fully wired rather than racing registration against the new session's first event."

**Resolution**: Pending
**Notes**:

---
