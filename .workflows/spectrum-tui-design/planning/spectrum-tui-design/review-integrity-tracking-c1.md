---
status: complete
created: 2026-06-18
cycle: 1
phase: Plan Integrity Review
topic: Spectrum TUI Design
---

# Review Tracking: Spectrum TUI Design - Integrity

## Summary

Read end-to-end: planning.md (5-phase structure + per-phase goal/why/acceptance/task tables) and all five per-phase task files (phase-1..5-tasks.md — 9/9/9/7/8 = 42 tasks). Evaluated every integrity dimension as the implementer would.

**Overall: the plan is structurally strong and implementation-ready.** Template compliance is complete (every task carries Problem/Solution/Outcome/Do/Acceptance/Tests/Edge Cases/Context/Spec Reference). Vertical slicing holds throughout — each task delivers a complete, independently-verifiable increment, no horizontal "all-models-then-all-wiring" layering. Phase structure is foundation→consumers, and every cross-phase dependency consumes EARLIER work (Phase 1 tokens/canvas/detect-gate everywhere; 2-1 descriptor → 2-4/3-3/3-4/4-5/4-7; 2-9 empty-state pattern → 4-5; 3-1 blank-screen → 3-5/3-6/3-7/3-9; 3-4 help renderer → 4-7; 4-1 notice band → 4-2/4-3/4-4/5-7; 4-6 preview chrome → 4-7) so natural ID/phase order produces the correct sequence with no forward references and no missing convergence edges. Acceptance criteria are pass/fail; the VERIFICATION MANDATE (vhs capture + named-Paper-frame compare + behaviour parity) is embedded per UI-task, and the vhs-exemption + behavioural-acceptance pattern is stated explicitly on every non-visual/plumbing task (1-2, 1-3, 1-4, 1-5, 2-1, 3-3, 3-8, 4-1, 5-1..5-4, 5-7, 5-8). The reskin-vs-behaviour-change-vs-new-work classification is applied consistently (edit modal = the one deliberate behaviour change, banner-flagged; kill/rename/delete/Projects/preview/edge-states = parity-preserving reskins; blank-screen layer + ? help + canvas/detection + cold-path flip = new). The two prior traceability fixes are present and correct (§12.3 caveat on 1-9; §15.6 light-eyeball on 3-4/3-6/3-9).

Findings are proportional and few: one Important (an unresolved design decision the plan leaves to the implementer) and two Minor (consistency / acceptance-criterion completeness polish). None blocks implementation.

## Findings

### 1. Task 5-7 leaves the post-load warning-notice lifetime as an unresolved implementer decision

**Severity**: Important
**Plan Reference**: Phase 5, task spectrum-tui-design-5-7 (Soft warnings ride the progress channel → post-load notice)
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Change Type**: update-task

**Details**: 5-7's own Edge Cases bullet instructs the implementer to "document expected lifetime (auto-clear like a flash, or persists until keypress) and confirm against §11 single-slot semantics" — but neither the **Do** steps nor the **Acceptance Criteria** resolve that lifetime. The result is a genuine observable-behaviour decision (does a startup SaverDown/CorruptSessionsJSON warning auto-dismiss on the next keypress like a flash, or sit persistently in the slot until explicitly cleared?) left open at implementation time, which is exactly the "implementer would have to guess / make a design decision" case this review guards against. Every other notice consumer in Phase 4 pins its slot behaviour: 4-2 flash is transient/auto-clear, 4-3 signpost and 4-4 command-pending banner are persistent. 5-7 is the only band routed through the 4-1 arbiter whose persistence class is undeclared, which also makes the 4-1 arbiter hand-off behaviour for this band untestable as written (the single-slot rule depends on knowing whether the band is transient or persistent).

The decision is resolvable from the existing material without inventing scope: §10.5 frames these as a *post-load notice* surfaced once *after the picker appears* (i.e. not a standing mode band), and today's delivery (`flushBufferedWarningsCmd`) is a one-shot surfacing — so the spec-faithful default is a transient/auto-clearing band on the next actionable keypress (the §11.2 flash lifecycle, reusing the existing `flashGen`/`isActionableKey` machinery the 4-1/4-2 tasks already preserve). The fix pins that as the default in Do + Acceptance while still allowing the implementer to record a deviation if §11 semantics demand otherwise.

**Current** (task 5-7 — the third **Do** bullet, verbatim):
```markdown
- In the model, hold the warnings through the loading window and, on transition to Sessions, surface them as a post-load notice via the Phase 4 notice-band primitive (the `▌` left-bar band routed through the single-slot arbiter; task 4-1). Decide which role variant fits a warning — these are warnings, so the orange/warning treatment is the natural fit; confirm against the notice-band role variants. Replace today's stderr alt-screen flush (`flushBufferedWarningsCmd`) for the cold/TUI path with the in-TUI notice band.
```

**Proposed** (task 5-7 — replace that **Do** bullet, pinning role + lifetime):
```markdown
- In the model, hold the warnings through the loading window and, on transition to Sessions, surface them as a post-load notice via the Phase 4 notice-band primitive (the `▌` left-bar band routed through the single-slot arbiter; task 4-1). Use the **orange/warning role variant** (these are warnings — the §11.2 `accent.orange` treatment) and surface the notice as a **transient band that auto-clears on the next actionable keypress** (reuse the existing `flashGen`/`isActionableKey` lifecycle the 4-1/4-2 tasks preserve), consistent with §10.5's "post-load notice surfaced once after the picker appears" framing and today's one-shot `flushBufferedWarningsCmd` delivery — NOT a standing persistent mode band. (If §11 single-slot semantics force a different lifetime at implementation, record the deviation + rationale in a code comment.) Replace today's stderr alt-screen flush (`flushBufferedWarningsCmd`) for the cold/TUI path with this in-TUI notice band.
```

**Current** (task 5-7 — the fifth Edge Cases bullet, verbatim):
```markdown
- The single-slot rule: if a transient flash or persistent band would also want the slot, the post-load warning notice must coexist per the Phase 4 arbiter's hand-off rules — it is a notice surfaced once after load, not a permanent band; document expected lifetime (auto-clear like a flash, or persists until keypress) and confirm against §11 single-slot semantics.
```

**Proposed** (task 5-7 — replace that Edge Cases bullet):
```markdown
- The single-slot rule: the post-load warning notice is a **transient** band (auto-clears on the next actionable keypress per the §11.2 flash lifecycle — pinned in Do), so it follows the 4-1 arbiter's transient hand-off rules exactly as the inline flash (4-2) does: it wins the slot while shown, then yields to any persistent band (e.g. the By-Tag signpost) once it auto-clears. It is surfaced once after load, never a permanent band.
```

**Proposed** (task 5-7 — add one Acceptance Criterion pinning the lifetime so it is verifiable):
```markdown
- [ ] The post-load warning notice uses the orange/warning role and is transient — it auto-clears on the next actionable keypress (reusing the existing `flashGen`/`isActionableKey` lifecycle), following the 4-1 arbiter's transient hand-off rules (yields the slot to a persistent band on clear); it is not a standing persistent band
```

**Resolution**: Fixed (auto-approved cycle 1) — task 5-7 Do bullet + Edge Case replaced and a new acceptance criterion added in phase-5-tasks.md (orange/warning role + transient/auto-clear lifetime); note added on tick-93c4fc.
**Notes**: The fix resolves the open decision toward the spec-faithful default (§10.5 one-shot post-load notice + today's one-shot stderr flush ⇒ transient), keeps the implementer escape hatch for a §11-forced deviation, and makes the 4-1 arbiter interaction testable. No new scope — it only pins a choice the task already required the implementer to make.

---

### 2. Task 5-5 acceptance does not pin the step-list to exactly the five §10.4 labels in order

**Severity**: Minor
**Plan Reference**: Phase 5, task spectrum-tui-design-5-5 (Honest loading-screen render — VISUAL)
**Category**: Acceptance Criteria Quality
**Category-detail**: criteria-cover-the-actual-requirement
**Change Type**: add-to-task

**Details**: 5-5's render consumes the 5-friendly-label mapping from 5-4 and its Do says "Render the step-list as a real list of rows (one per friendly label)". The acceptance criteria pin the tick glyphs/tokens, the `(N/M)`-only-on-Restoring rule, the real-list-not-text-swap rule, and the frame compare — but no criterion pins that the rendered list is exactly the **five** §10.4 labels (`Started tmux server` / `Registered hooks` / `Restoring sessions (N/M)` / `Replaying scrollback` / `Running resume commands`) in that order. Since 5-4 owns the canonical mapping and is the single source of truth, a 5-5 render that drew a different count/order than 5-4 produced would be a defect that the current acceptance set would not directly catch (it would only surface indirectly via the `Loading 6 — Combined (thick bar)` frame compare, which is agent-judged, not a structural assertion). Adding one criterion makes the label-set/order a direct pass/fail check and reinforces 5-4 as the single source. Low impact because the frame compare provides backstop coverage and 5-4's mapping is already authoritative — hence Minor.

**Current** (task 5-5 — first Acceptance Criterion, verbatim):
```markdown
- [ ] `viewLoading` renders centred `PORTAL ▌` (`text.primary` + `accent.violet` caret) over a thick block bar (filled `accent.violet`, track `bg.track`) and a real ticking step-list
```

**Proposed** (task 5-5 — replace that criterion with two, pinning the label set/order):
```markdown
- [ ] `viewLoading` renders centred `PORTAL ▌` (`text.primary` + `accent.violet` caret) over a thick block bar (filled `accent.violet`, track `bg.track`) and a real ticking step-list
- [ ] The step-list is exactly the five §10.4 friendly labels in order — `Started tmux server` · `Registered hooks` · `Restoring sessions (N/M)` · `Replaying scrollback` · `Running resume commands` — sourced from the task-5-4 mapping (one row per label, no label invented or dropped at the render layer)
```

**Proposed** (task 5-5 — add the matching test name to the Tests list):
```markdown
- `"it renders exactly the five §10.4 friendly labels in order, sourced from the task-5-4 mapping"`
```

**Resolution**: Fixed (auto-approved cycle 1) — task 5-5 first acceptance criterion split into two (pinning the five §10.4 labels in order) + matching test added in phase-5-tasks.md; note added on tick-c9b7f5.
**Notes**: Reinforces 5-4 as the single source of truth for the label set and makes the render's label-set/order directly verifiable rather than only frame-compare-implied.

---

### 3. Task 4-7 keeps a stale "AdaptiveColor →" edge-case label that does not apply to the Preview help task

**Severity**: Minor
**Plan Reference**: planning.md Phase 4 task table — spectrum-tui-design-4-7 Edge Cases column is clean; the stale label is in phase-4-tasks.md task 4-6's Edge Cases, carried verbatim into the table — see below
**Category**: Consistency
**Change Type**: (none — withdrawn)

**Details**: WITHDRAWN on re-read. The `previewBorderColor` AdaptiveColor → `accent.cyan` edge case belongs to task 4-6 (which re-targets the migrated colour) and is correctly placed there; task 4-7 (help wiring) does not carry it. No inconsistency. Recorded only to document that the 4-6/4-7 split of the preview work was checked and is clean (4-6 = chrome reskin incl. the colour re-target; 4-7 = `?` overlay wiring), with no duplicated or misplaced edge cases between them.

**Resolution**: N/A — withdrawn (no change)
**Notes**: Left in the tracking file as an audit trail entry showing the preview-task split was verified, per the "read everything" mandate. No fix proposed.

---
