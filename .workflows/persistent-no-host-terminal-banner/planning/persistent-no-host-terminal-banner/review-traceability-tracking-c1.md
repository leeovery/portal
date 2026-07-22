---
status: complete
created: 2026-07-22
cycle: 1
phase: Traceability Review
topic: Persistent No Host Terminal Banner
---

# Review Tracking: Persistent No Host Terminal Banner - Traceability

## Result

**CLEAN — no findings.** The plan is a faithful, complete translation of the specification in both directions. Every specification element has plan coverage, and every element of plan content traces back to the specification.

## Direction 1: Specification → Plan (completeness)

Every spec element maps to a task with matching acceptance criteria:

| Spec element | Plan coverage |
|---|---|
| §1 Defect 1 — persistent banner on remote clients | Phase 1 / Task 1.1 (banner gate split) |
| §1 Defect 2 — walkable dead-end multi-select | Phase 2 / Task 2.2 (proactive entry block) |
| §1 Target — NULL: no banner, standard `Sessions ··· N` header, signpost normal | Task 1.1 |
| §1 Target — named banner kept unchanged | Tasks 1.1, 1.2 |
| §1 Target — `m` blocked on unsupported, transient flash, omitted from `?` help | Tasks 2.2, 2.3 |
| §1 Solution shape — 4 sub-fixes + `UnsupportedNoopMessage` rewrite + no state footprint | Phases 1–3 |
| §2 Sub-fix 1 — `IsNull()` discriminator on `unsupportedBannerActive()` | Task 1.1 |
| §2 — single gate fixes both `applySectionHeader` + `activeNoticeBand` | Task 1.1 |
| §2 Scope guard — detection cache left untouched, no re-detection on rebuild | Task 1.1 (+ Phase 1 acceptance) |
| §3 Sub-fix 2 — entry block on `DetectUnsupported()`, both shapes, identity-blind | Task 2.2 |
| §3 — only keypress gated; `WithInitialMultiSelect` not gated | Task 2.2 |
| §3 — retain reactive backstop (Fork A → A1) | Tasks 2.1, 2.2 |
| §3 — async in-flight window, no mid-mode eject | Task 2.1 (in-flight-entry burst test), Task 2.2 |
| §3 — visible flash not silent no-op (Fork B → B1) | Task 2.2 |
| §4 Sub-fix 3 — help `m`-suppression at call site | Task 2.3 |
| §4 — gated `DetectUnsupported() && !multiSelectMode`; `sessionsKeymap()` stays static; footer unchanged | Task 2.3 |
| §4 — A1 consistency; why call-site not parameterised | Task 2.3 |
| §4 / §8 — latent guard-coupling inline source note | Tasks 2.2, 2.3 |
| §5 — blocked-entry flash copy (both shapes), TUI-local `multiSelectBlockedFlashText` | Task 2.2 |
| §5 — reactive no-op copy (both shapes) `UnsupportedNoopMessage` | Task 3-1 |
| §5 — named banner copy kept; setup guidance retained | Tasks 1.1, 1.2 |
| §5 — named non-repetition constraint; shared `⚠` two-row co-render | Task 2.2 |
| §5 — `UnsupportedNoopMessage` in scope, shared with CLI | Tasks 3-1, 3-2 |
| §5 — accepted NULL imprecision (remote + transient-error fold) | Tasks 2.1, 3-1 |
| §5 — out-of-scope adjacent copy (session-gone / failed / permission) untouched | Task 3-1 (explicit) |
| §6 — dead NULL render branch removed; renderers named-only; `see docs` unconditional | Task 1.2 |
| §6 — confirmed NULL end-state (header / help / footer / flash) | Tasks 1.1, 2.2, 2.3 |
| §7 Rework — `burst_unsupported_noop_test.go` onto in-flight path | Task 2.1 |
| §7 Rework — invert `TestApplySectionHeader_UnsupportedNullShowsHonestLine` | Task 1.1 |
| §7 Rework — copy assertions to new strings | Tasks 3-1, 3-2 |
| §7 Remove — `TestUnsupportedHeader_NullIdentityNoHostLocal` | Task 1.2 |
| §7 New — banner split; `m`-entry block + named co-render; help a/b/c; copy | Tasks 1.1, 2.2, 2.3, 3-1 |
| §7 Guard — supported unchanged path (banner absent, `m` enters, help lists `m`, burst dispatches) | Tasks 1.1, 2.1, 2.2, 2.3, 3-1 |
| §7 Visual — `sessions-unsupported-null` fixture + reference PNG | Task 1.3 |
| §8 In scope — `internal/tui` sub-fixes; `internal/spawn/message.go` | Phases 1–3 |
| §8 Non-goal — CLI block logic unchanged | Task 3-2 (explicit no-change) |
| §8 Non-goal — `see docs` link / docs page (separate quickfix) | Out of scope (no task) |
| §8 Non-goal — adjacent spawn copy | Task 3-1 |
| §8 Non-goal — no state/daemon/`sessions.json`/`prefs.json` footprint | Honored by omission |
| §8 Risk — CLI copy coordination with `cli-verb-surface-redesign` | Task 3-2 |
| §8 Sequencing — `cli-verb-surface-redesign` lands first | Phase 3 "why this order" |

Depth of coverage is high: each task carries a `Spec Reference`, quoted `Context` blocks, byte-exact copy strings, and concrete file/method anchors — an implementer would not need to return to the specification.

## Direction 2: Plan → Specification (fidelity, anti-hallucination)

Every task's Problem / Solution / Do / Acceptance Criteria / Tests / Edge Cases traces to a named specification section:

- **Task 1.1** → §2, §7. **Task 1.2** → §6, §7 (the `TestUnsupportedHeader_ExactlyOneRow` "null" subcase conversion is the mechanical consequence of §6's named-only renderers, not invented scope). **Task 1.3** → §7 Visual.
- **Task 2.1** → §3, §7. **Task 2.2** → §3, §5, §4/§8. **Task 2.3** → §4, §7 (leaving the Projects help call site untouched is faithful — `m` is a Sessions-only key, so Projects has no `m` entry).
- **Task 3-1** → §5, §7, §8 (observability `logemit` line explicitly preserved per §5 out-of-scope; adjacent copy preserved). **Task 3-2** → §5, §7, §8.

No plan content lacks a specification anchor. The one elaboration worth noting — Task 3-2 adding an explicit byte-literal CLI regression plus NULL-shape coverage (rather than the pre-existing self-referencing `want := spawn.UnsupportedNoopMessage(id)` assertion) — traces directly to §7 ("any test asserting the old `UnsupportedNoopMessage` strings … in the CLI open-burst suites updates to the new plain-language strings") and §8 ("The rewritten wording must read correctly for the CLI's 'something was attempted' case and be coordinated with `cli-verb-surface-redesign`"). It is a faithful means of enforcing the coordination contract, and the NULL shape is a spec-identified shape of the shared message — not invention.

## Findings

None.
