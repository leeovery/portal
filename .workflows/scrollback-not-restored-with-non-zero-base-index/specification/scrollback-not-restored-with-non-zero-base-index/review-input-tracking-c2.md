---
status: in-progress
created: 2026-04-30
cycle: 2
phase: Input Review
topic: scrollback-not-restored-with-non-zero-base-index
---

# Review Tracking: scrollback-not-restored-with-non-zero-base-index - Input Review

## Findings

### 1. Save-side is unaffected — bounds the fix scope

**Source**: investigation.md § "Manifestation" — "Save side is unaffected; per-pane scrollback files contain the expected ANSI-coloured terminal history."
**Category**: Enhancement to existing topic
**Affects**: Problem & Root Cause § Observed Symptom (or a new sub-bullet under "Why the End-to-End Path Otherwise Works")

**Details**:
The investigation explicitly establishes that the save daemon and on-disk scrollback files are healthy — only the replay/hydrate path fails. The spec never states this, which leaves an implicit ambiguity for an implementer: a reader could reasonably ask "is the saved scrollback intact, or do we also need to investigate save-side capture?" Stating it closes that question and bounds the fix to the restore/hydrate path. It also reinforces why acceptance criterion 1 ("saved scrollback replayed") is verifiable — the saved bytes are known to exist.

**Current**:
> After `tmux kill-server` and reattach, Portal restores sessions/windows/panes, cwd, and layout, but saved scrollback never appears in the pane.

**Proposed Addition**:
[blank — pending discussion]

**Resolution**: Pending
**Notes**:

---

### 2. Why the reporter's "removing base-index fixes it" observation appeared true

**Source**: investigation.md § "Why 'removing base-index makes it work' appears true (but isn't)"
**Category**: Enhancement to existing topic
**Affects**: Problem & Root Cause § Observed Symptom (the paragraph noting the bug report's framing is incorrect)

**Details**:
The spec asserts the bug report's base-index framing is incorrect, but does not explain why the reporter's empirical observation seemed to corroborate it. The investigation provides the explanation: with base-index unset the misleading WARN goes quiet (predicted == live == 0), so the reporter likely conflated WARN-disappearance with hydration-success without verifying scrollback actually replayed; alternatively, the reporter tested a non-dash session. Without this context the spec's flat assertion that the framing is wrong may read as dismissive, and a future reader retracing the bug could be confused by the same false signal. One or two sentences would inoculate against this.

**Current**:
> The bug report attributed this to non-zero `base-index` / `pane-base-index`. That framing is incorrect: base-index is a confound that surfaces a misleading diagnostic WARN, not the cause of hydration failure.

**Proposed Addition**:
[blank — pending discussion]

**Resolution**: Pending
**Notes**:

---

### 3. Reproduction steps not captured

**Source**: investigation.md § "Reproduction Steps" (4-step repro)
**Category**: Enhancement to existing topic
**Affects**: Problem & Root Cause § Observed Symptom, or Acceptance Criteria § criterion 1

**Details**:
The investigation provides explicit reproduction steps: (1) project basename starting with `.` (or any tmux config with non-zero base-index that surfaces the misleading WARN), (2) tmux.conf with `set -g base-index 1` and `setw -g pane-base-index 1`, (3) open a Portal session in that project and generate scrollback, (4) `tmux kill-server` and reattach. The spec's acceptance criterion 1 implicitly describes the verification path but does not lay out a deterministic repro. For an implementer wanting to manually verify the fix (or an integration test author seeding a scenario), having the canonical repro recorded would be useful — particularly the "(a) leading-dot project basename + (b) non-zero base-index" decomposition that disentangles the two confounding factors.

**Current**:
> 1. **Hydration succeeds for leading-dash session names.** After `tmux kill-server` and reattach, a Portal-managed session whose name begins with `-` (e.g. `-dotfiles-HM9Zhw`) has its saved scrollback replayed into each pane. No `hydrate timeout` WARN appears in `~/.config/portal/state/portal.log`. This holds regardless of `base-index` / `pane-base-index` values in the user's tmux config.

**Proposed Addition**:
[blank — pending discussion]

**Resolution**: Pending
**Notes**:

---
