---
status: complete
created: 2026-06-01
cycle: 3
phase: Gap Analysis
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Gap Analysis

## Findings

### 1. `error_class` value space is unpinned across the single-mutation WARN path and the batch per-entry WARN path

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: State-mutation audit trail § "Mechanical rule" (lines 688–689) and § "Batch operations" (line 715); cross-refs Subsystem prefix taxonomy § closed attr-key value space (`error_class` definition, line 188)

**Details**:
The single `error_class` attr key carries two distinct closed value spaces, enumerated together in its definition (line 188): "swallowed-error classification: `expected` / `unexpected`; or AtomicWrite failure phase." The state-mutation section then references both spaces on two WARN paths without pinning which value space applies to which path:

- The whole-mutation WARN rule (line 689): "On failure (WARN path): `error_class` from the **closed AtomicWrite failure space** below" — i.e. `write-failed-temp-create` / `write-failed-write` / `write-failed-fsync` / `write-failed-rename` (line 708).
- The batch per-entry WARN rule (line 715): "Per-entry WARN with `error_class=unexpected`" — i.e. the swallowed-error *classification* value, not a phase value.

So within the same section, the store-mutation WARN path is told to emit phase values (`write-failed-rename`) while the `CleanStale` per-entry WARN path is told to emit `unexpected`. An implementer instrumenting `hookStore.CleanStale` (the only enumerated batch site, lines 712–715) cannot mechanically determine which value space governs the per-entry WARN: line 689 declares the store-mutation WARN path uses the AtomicWrite phase space, while line 715 hard-codes `unexpected` for the per-entry WARN inside that same batch method. The two are reconcilable under the reading that a *per-entry* failure (e.g. malformed entry, pane-key resolution failure) is a generic swallowed-unexpected error rather than an AtomicWrite phase failure (the actual write happens once at end-of-batch) — but the spec never states that distinction, so the value-space selection is left to implementer judgment on a vocabulary the spec otherwise treats as emphatically closed and mechanically selectable.

Impact is narrow (one attr value on the CleanStale per-entry WARN line) and does not block the broader work, but it can yield inconsistent `error_class` values across stores and undercuts the "no per-site judgment" promise the level-discipline section makes for this attr.

**Current**:
> **Mechanical rule** (lines 688–689):
> - On failure (WARN path): `error_class` from the closed AtomicWrite failure space below.
>
> **Batch operations** (line 715):
> - Per-entry WARN with `error_class=unexpected` on per-entry failure mid-loop (regardless of whether the batch continues).
>
> **`error_class` definition** (line 188):
> | `error_class` | swallowed-error classification: `expected` / `unexpected`; or AtomicWrite failure phase |

**Proposed Addition**:
Pin which `error_class` value space applies to which WARN path. Suggested: state explicitly that the **whole-mutation WARN** (when the store's single `AtomicWrite` fails) carries the AtomicWrite phase value (`write-failed-*`), while a **batch per-entry WARN** (a single entry failing mid-loop, before/independent of the end-of-batch write) carries the swallowed-error classification value (`unexpected`) per the level-discipline table — i.e. the two value spaces correspond to two structurally different failure surfaces, not the same one. Optionally tighten the line-188 definition to note this split.

**Resolution**: Approved
**Notes**: Priority: Minor. Added a "Which `error_class` space applies at which WARN site" clause to the State-mutation section: whole-mutation WARN (AtomicWrite failed) → `write-failed-*` phase space; per-entry batch WARN (entry dropped mid-loop) → `error_class=unexpected`. The two value spaces map to two structurally distinct failure surfaces and never overlap.

---
