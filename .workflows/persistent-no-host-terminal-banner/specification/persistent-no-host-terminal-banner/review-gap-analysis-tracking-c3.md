---
status: complete
created: 2026-07-22
cycle: 3
phase: Gap Analysis
topic: Persistent No Host Terminal Banner
---

# Review Tracking: Persistent No Host Terminal Banner - Gap Analysis

## Findings

### 1. §1 Solution Shape understates scope and its "No CLI change" claim conflicts with §5/§8's CLI-coordination requirement

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: §1 (Solution Shape), §5 (`UnsupportedNoopMessage` is in scope / Named reactive-CLI line), §8 (In scope — `internal/spawn/message.go`; Risks & coordination — CLI copy coordination)

**Details**:
§1's Solution Shape is the orienting summary a planner reads first to scope the work. It states: *"Four coordinated, independently-testable TUI-side sub-fixes (banner split, proactive `m`-entry block, help-modal `m`-suppression, blocked-entry flash copy). No CLI change; no state/daemon/`sessions.json`/`prefs.json` footprint — spawn's near-zero state footprint is unchanged."* Two parts of this conflict with the detailed scope in §5/§8:

1. **Scope understated.** The enumerated "four TUI-side sub-fixes" omit the in-scope `internal/spawn/message.go` rewrite of `UnsupportedNoopMessage` (both shapes), which §8 In-scope lists explicitly and describes as *"This widens the fix beyond the TUI, as decided in Topic 5."* That change is neither TUI-side nor one of the four. (§6's dead-NULL-branch removal is also in-scope per §8 but not in the "four"; it is at least conceptually folded into sub-fix 1 via §2's forward-reference to Topic 6, so it is the lesser half of the gap.)

2. **"No CLI change" contradicts the flagged CLI coordination.** §5 states the `UnsupportedNoopMessage` copy is *"shared with the CLI open-burst"* and §8 lists **"CLI copy coordination"** as an explicit risk: the rewritten wording is rendered by the CLI open-burst and *"must be coordinated with `cli-verb-surface-redesign` so the two surfaces stay coherent."* §8's non-goal refines the true position — *"does not change the CLI's block **logic**; it only touches the shared message renderer"* — i.e. the CLI's user-facing unsupported message text **does** change through the shared renderer. §1's flat "No CLI change" therefore reads as denying any CLI-surface impact, masking the one cross-package, coordination-sensitive dimension of this fix that §8 treats as a genuine (non-blocking) risk and sequencing dependency.

A reader relying on §1's Solution Shape in isolation would under-scope the task set (missing the `internal/spawn/message.go` edit and its test/copy fallout) and would not anticipate the `cli-verb-surface-redesign` coordination that §8 flags. The full spec (§5/§8) is internally correct and unambiguous; the defect is confined to §1's summary being looser than — and on the CLI point, in tension with — the sections it summarises. Because §5/§8 fully and correctly scope the spawn change, a thorough whole-spec reader is not misled, which keeps this Minor rather than Important — but for a spec otherwise meticulous about scope enumeration and single-sourcing, tightening the orienting summary removes a real cross-section inconsistency.

**Proposed Addition**:
§1 Solution Shape tightened: adds the `internal/spawn/message.go` `UnsupportedNoopMessage` rewrite as in-scope (beyond the four TUI sub-fixes) and replaces the blanket "No CLI change" with the precise form — no change to CLI burst/block **logic**, but the shared message the CLI open-burst renders is rewritten and must stay coherent with `cli-verb-surface-redesign`.

**Resolution**: Approved
**Notes**: Auto-applied (Minor). Consequence of the Topic 5 copy-scope widening; §1 summary brought in line with §5/§8.

---
