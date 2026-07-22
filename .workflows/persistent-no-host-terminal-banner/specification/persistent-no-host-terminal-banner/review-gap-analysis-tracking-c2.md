---
status: in-progress
created: 2026-07-22
cycle: 2
phase: Gap Analysis
topic: Persistent No Host Terminal Banner
---

# Review Tracking: Persistent No Host Terminal Banner - Gap Analysis

## Findings

### 1. §7 help-suppression coverage mis-describes §4's condition and omits the A1 in-mode case

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: §7 (Testing Requirements — "Help suppression" bullet), §4 (Help-Modal `m`-Suppression), §3 (Async in-flight window / Fork A1)

**Details**:
§4 pins the help filter condition precisely as `DetectUnsupported() && !m.multiSelectMode`, and its "Consistency with A1" subsection is emphatic that `m` **stays listed** in the `?` help when detection resolves unsupported *while multi-select is already open* (the A1 in-flight-entered state that §3 deliberately does not eject) — "the help never hides a working key." So §4 defines two "lists `m`" cases: supported, **and** in multi-select mode on any terminal.

§7's "Help suppression" coverage bullet describes the rule with the coarse predicate only: *"`?` help omits the `m` row when `DetectUnsupported()`, lists it when supported."* This diverges from §4 in two ways:

- It drops the `!m.multiSelectMode` qualifier. A test written literally to §7's wording would assert the help omits `m` **whenever** `DetectUnsupported()` is true — which directly contradicts §4's A1 case (unsupported **and** in multi-select → `m` is listed).
- It presents "lists when supported" as the sole list case, omitting §4's second list case (in multi-select mode, any terminal).

The consequence is a planning-readiness hole precisely on the behaviour that cycle-1's top (Important) gap finding introduced — the `&& !m.multiSelectMode` guard. Following §7 as the acceptance-criteria list, an implementer would leave that guard untested, or worse encode the contradicting assertion for the in-mode case. `keymap_dispatch_guard_test` cannot backstop it because it runs with detection unwired (`DetectUnsupported()` false), so the guard would ship with no test protecting it. This is a cross-section inconsistency between §7's test description and §4's specified condition, not merely a missing test.

Fix direction: restate §7's help-suppression bullet to mirror §4 exactly — help omits `m` iff `DetectUnsupported() && !m.multiSelectMode`; help lists `m` when supported **or** when in multi-select mode — and add explicit coverage for the in-mode-unsupported case (help lists `m`) alongside the existing omit/supported cases.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 2. Named two-row co-render leaves the duplicate `⚠` glyph unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: §5 (Unsupported-Terminal Copy — "Blocked-entry flash behaviour" and "Named non-repetition constraint" bullets), §7 (New coverage — the named co-render assertion)

**Details**:
The named two-row co-render is a *new* rendering combination this fix introduces: on a named unsupported terminal, pressing `m` while not in multi-select shows the persistent banner on the header row **and** the blocked-entry flash on the notice-band row (§5, §7). The section-header banner renders a literal `⚠` (`renderUnsupportedHeader` / `unsupportedLeftCluster`), and the blocked-entry flash reuses the §11 warning flash slot (`setFlash` → `flashWarning`), whose notice band prepends its own `⚠` via `statusGlyph`. So both rows render an orange `⚠`, stacked.

§5's "Named non-repetition constraint" was added specifically to keep the two co-rendered rows non-redundant — but it governs only *text* ("unsupported terminal" / identity string / `see docs`) and is silent on the duplicated `⚠` glyph. §7 requires a named-co-render fixture/test but states no expectation for the glyph. An implementer reusing the warning flash slot will naturally emit two stacked `⚠`, with no way to tell from the spec whether that is intended or whether the block flash should be glyphless (or a non-warning kind) to avoid the redundancy the non-repetition constraint is otherwise trying to prevent. (This redundancy is unique to the block path: the reactive no-op flash never co-renders with the banner, because the banner is suppressed while in multi-select mode.)

Given the spec's explicit attention to non-redundancy in exactly this state, confirm the intended glyph treatment for the named two-row co-render (accept two `⚠`, or drop/alter the flash glyph).

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
