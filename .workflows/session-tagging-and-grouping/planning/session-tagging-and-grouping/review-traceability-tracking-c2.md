---
status: complete
created: 2026-06-07
cycle: 2
phase: Traceability Review
topic: Session Tagging and Grouping
---

# Review Tracking: Session Tagging and Grouping - Traceability

## Outcome

**Clean.** The plan remains a faithful, complete translation of the specification in both directions. No findings.

## Scope of Analysis (cycle 2)

- Specification re-read in full: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (336 lines).
- Planning file re-read: `planning.md` (4 phases, 30 tasks; all phases `status: approved`).
- All 30 task details re-read across `phase-1-tasks.md` (8), `phase-2-tasks.md` (6), `phase-3-tasks.md` (9), `phase-4-tasks.md` (7).
- Tick store verified: `tick list --parent tick-7d0ed3` returns 30 tasks + 4 phase features (no task added/removed since cycle 1).

## Delta since cycle 1 (which was clean)

The only change since the cycle-1 clean assessment was the addition of three task-level dependency (`blocked_by`) edges. No task content, acceptance criteria, edge cases, Problem/Solution, tests, or spec coverage changed. Verified against the tick store:

| Consumer task | New blocked_by edges | Grounding |
|---|---|---|
| 2-2 By Project grouping builder (`tick-4358f8`) | 2-1 `SessionItem` metadata (`tick-0ccac8`), 1-6 `Session.Dir` via ListSessions (`tick-596ae9`), 1-4 `CanonicalDirKey` (`tick-5a49ee`) | 2-2's Do/Solution explicitly consume the enriched `SessionItem` fields, `Session.Dir`, and `project.CanonicalDirKey`/`MatchProjectByDir` — correct producer→consumer ordering |
| 2-3 By Tag grouping builder (`tick-dc8a90`) | 2-1 `SessionItem` metadata (`tick-0ccac8`), 1-4 `CanonicalDirKey` (`tick-5a49ee`), 1-2 `NormaliseTag` (`tick-b97407`) | 2-3's Do/Solution explicitly consume the enriched `SessionItem` fields, canonical dir keying via `MatchProjectByDir`, and the canonical tag form delivered by Phase 1 — correct producer→consumer ordering |

These edges encode real intra/cross-phase data dependencies already named in the task bodies. They are pure sequencing metadata: they add no scope, remove no coverage, and do not alter any acceptance criterion. Traceability in both directions is therefore unaffected.

## Direction 1: Specification → Plan (completeness)

Re-confirmed: every spec element traces to a task with matching, implementer-grade acceptance criteria. The cycle-1 coverage matrix still holds verbatim (no task content changed):

| Spec section / decision | Covering task(s) |
|---|---|
| Tag data model — `tags []string` on `Project`; back-compat (missing → nil/empty, no migration) | 1-1 |
| Tag normalisation & validation (trim / lower-case / reject empty / per-project dedup; canonical form used everywhere compared) | 1-2 (helper), 1-3 (store set add/remove) |
| Implicit tags / no registry / cross-project union | 1-2/1-3 (canonical form), 2-3 (union via grouping), 3-7 (`anyTagsExist` gate) |
| Taggable surface — projects only; lifecycle / no orphan sweep | 1-3, 4-x (modal-only origin); lifecycle is a no-build decision (deleted-project routing in 2-2/2-4) |
| Session→Dir resolution — `@portal-dir` stamp (fast path) | 1-5 (stamp at create), 1-6 (expose via ListSessions) |
| Lazy stamp-on-render fallback; failure & ordering semantics; first-ship amortisation | 1-7 (active-pane→git-root resolve), 1-8 (best-effort re-stamp, render-uses-derived-value-first) |
| Path-keying canonicalisation (symlinks / trailing slash / `~`; lookup-key-matches-stored-Path invariant) | 1-4 |
| Grouping modes — Flat / By Project (Pattern A) / By Tag (Pattern B) | 2-2, 2-3 |
| Untagged / Unknown catch-all buckets (pinned last, counted, empty-suppressed) | 2-4 |
| Heading label text (By Project = project name keyed on canonical path; By Tag = canonical tag) | 2-2, 2-3 |
| Ordering — static alphabetical within/across groups, catch-all pinned last | 2-2, 2-3, 2-4 |
| Group headers — dimmed, non-selectable (render-layer separators), counted (`H ··· N`) | 2-5 (render), 2-6 (cursor/selection contract) |
| Item model — pre-sorted grouped slice; one item per (session,tag); shared underlying session | 2-1, 2-2, 2-3 |
| Toggle key `s` — single unconditional cycle; `s` literal while filter focused; persist per press | 3-4 |
| Mode indication in title (`Sessions` / `— by project` / `— by tag`) | 3-5 |
| Footer `s switch view` hint (sessions page only) | 3-6 |
| Rendering stack — lipgloss layered into SessionDelegate; no `lipgloss/tree` | 2-5 |
| Mode persistence — `prefs.json`, `configFilePath`+`migrateConfigFile`, AtomicWrite, tolerant decode | 3-1, 3-2, 3-9 |
| Empty states — By-Tag zero-tags "No tags yet" signpost; By-Project empty/Unknown | 3-7, 2-4, 3-3 |
| Filter composition — flatten-on-filter, restore-on-clear, name-based scope | 3-8, 2-1 (FilterValue = name) |
| Mode-aware re-render core dispatching to builders; refresh preserves mode | 3-3 |
| TUI construction wiring (initial mode + persister seam) | 3-9 |
| Assigning/managing tags — Tags field, Tab 3-way cycle, add-on-Enter, `x`-remove, empty state | 4-1, 4-2, 4-3, 4-4, 4-6 |
| Persist on confirm via `ProjectEditor` AddTag/RemoveTag seam | 4-5 |
| Refresh contract — re-group on projects-edit → sessions transition | 4-7 |
| TUI only / no `portal tags …` CLI | Phase 4 goal + acceptance (no CLI task authored — correct) |
| No-regression / additive invariant (Flat byte-for-byte today) | 2-1, 2-5 (no-heading guarantee), 3-3 (Flat = ToListItems) |

All 15 spec Acceptance Criteria still map to tasks: #1→3-4/3-5/3-6; #2→2-3; #3→2-3/2-4; #4→2-2; #5→2-5/2-6; #6→1-3/4-3; #7→2-3; #8→1-5/1-6; #9→1-7/1-8; #10→2-4; #11→3-9; #12→3-1/3-9; #13→3-7; #14→3-8; #15→2-1/2-5/3-3.

Coverage depth remains sufficient — each task carries the spec's essence (rules, edge cases, ordering, failure semantics) inline.

## Direction 2: Plan → Specification (fidelity / anti-hallucination)

Re-confirmed: every task's Problem, Solution, acceptance criteria, tests, and edge cases trace to a specific spec section. The small set of implementation-level mechanisms remain idiomatic realisations of spec decisions, each explicitly flagged in-task as a divergence or owned implementation call:

- **`CatchAll bool` field on `SessionItem` (2-1)** — implements the pinned-last Unknown/Untagged buckets (§ Catch-all bucket rendering). Mechanism, not new scope.
- **`prefs` leaf package, typed enum, `parseMode` (3-1)** — direct realisation of the spec's "Concrete shape (idiomatic, owned)" for `prefs.json`.
- **Inside-tmux title reconciliation (3-5)** — flags that the spec title scheme does not mention the pre-existing `(current: %s)` decoration and reconciles deterministically. Correctly surfaced.
- **`prefs.json` mapped to `""`/unmapped in `configFileComponents` (3-2)** — flags that `prefs` is not in the closed 15-component log catalogue and suppresses the migrate-log emission rather than introducing a non-catalogued component. Correctly surfaced.
- **`via` argument ambiguity for `Store.AddTag`/`RemoveTag` (4-5)** — explicitly flagged as an ambiguity to align rather than guess. Appropriate.

The three new dependency edges introduce no new content to trace and therefore no new hallucination surface. No task contains a requirement, behaviour, edge case, or acceptance criterion that cannot be pointed back to a concrete spec section.

## Findings

None.
