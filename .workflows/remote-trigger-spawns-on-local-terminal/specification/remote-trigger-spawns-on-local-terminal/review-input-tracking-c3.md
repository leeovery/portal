---
status: in-progress
created: 2026-07-23
cycle: 3
phase: Input Review
topic: remote-trigger-spawns-on-local-terminal
---

# Review Tracking: remote-trigger-spawns-on-local-terminal - Input Review

## Findings

### 1. CLI mixed-case no-op message string may be stale (pre-banner-fix)

**Source**: investigation.md — Symptoms → Expected behavior (line 8): the mixed case should take "the same atomic no-op as the pure-remote case (`⚠ no host-local terminal — nothing opened`)"; AND Blast Radius → "Interaction to carry into spec" (line 144) + Notes → "Spec coherence check" (line 197): the flagged coherence check with `persistent-no-host-terminal-banner`.
**Category**: Gap/Ambiguity
**Affects**: "Scope: Affected Surfaces" — item 1 (CLI multi-target burst); cross-references the "Coherence with `persistent-no-host-terminal-banner`" section.

**Details**:
The investigation's Expected-behavior states the mixed remote-trigger case should produce the same no-op as the pure-remote case, and gives that message verbatim as `⚠ no host-local terminal — nothing opened`. The spec carries this string forward into Scope item 1, where the CLI mixed case is described as "atomic no-op with the honest **'no host-local terminal'** message."

However, the spec's own "Coherence with `persistent-no-host-terminal-banner`" section — resolving the coherence check the investigation explicitly flagged (lines 144, 197) — establishes that after that prior fix the NULL/remote outcome uses `spawn.UnsupportedNoopMessage` → **"can't open new windows over a remote connection — nothing opened"**, and describes this as the reactive/shared no-op copy for the NULL case.

The spec therefore presents two different user-facing strings for the same NULL no-op: the CLI scope item says "no host-local terminal," while the coherence section says "can't open new windows over a remote connection." The investigation's `no host-local terminal` phrasing predates the banner fix it flags for coherence, so the CLI scope item appears to have carried forward a stale string rather than the reconciled shared copy. Which message the CLI mixed-case user actually sees is left ambiguous. Both surfaces should state the honest no-op identically, pinned to the single shared `spawn.UnsupportedNoopMessage` copy the coherence section already establishes — otherwise an implementer could hard-code / assert the stale "no host-local terminal" wording for the CLI burst.

**Proposed Addition**:
[blank until discussed]

**Resolution**: Pending
**Notes**:

---
