---
status: complete
created: 2026-07-22
cycle: 1
phase: Gap Analysis
topic: Persistent No Host Terminal Banner
---

# Review Tracking: Persistent No Host Terminal Banner - Gap Analysis

## Findings

### 1. Help-modal `m`-suppression contradicts the A1 "no mid-mode eject" decision

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: §4 (Help-Modal `m`-Suppression), §3 (Async in-flight window / Fork A1)

**Details**:
Sub-fix 3 (§4) filters the `m` row out of the `?` help "when `DetectUnsupported()` is true," and justifies it as "consistent with `m` being blocked at entry." But §3's Fork-A1 decision explicitly permits a state where `DetectUnsupported()` is true **and** multi-select mode is already open: the user enters multi-select during the async in-flight window, detection then resolves unsupported, and the mode is **not** ejected. In that state `m` is *not* blocked — the entry gate only guards `if !m.multiSelectMode`, so `m` continues to work as a live row-toggle for the entire duration of that multi-select session. Yet the help modal (reachable via `?` during multi-select — the `?` handler has no `multiSelectMode` guard) would hide the `m` row while `m` is actively functional. So §4's stated rationale does not hold in the A1 state, and the two sub-fixes disagree about whether `m` is "available."

This forces the implementer to make an undirected design decision at implementation time: gate the help filter on `DetectUnsupported()` alone (spec-literal — hides a working key), or additionally on `!m.multiSelectMode` (self-consistent, but the spec doesn't say so). Note the existing help is otherwise non-mode-contextual (it lists k/r/n/x even though those are no-ops in multi-select), so "leave help static except this one filter" is a defensible alternative the spec should explicitly choose. User-facing impact is small and the triggering path is rare, but it is a genuine cross-section inconsistency the spec's own reasoning surfaces.

**Proposed Addition**:
Option A chosen (make it self-consistent): §4 filter gated on `DetectUnsupported() && !m.multiSelectMode`; behaviour reworked so help lists `m` iff it is functional; new "### Consistency with A1 (in-flight entry)" subsection added to §4.

**Resolution**: Approved
**Notes**: User picked recommended Option A. Rule: `m` in help ⟺ `m` functional; hidden only when unsupported AND not already in the mode. Guard-safe.

---

### 2. Blocked-entry flash renderer has no specified home, name, or shape-selection

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: §3 (Change), §5 (copy table — Blocked-entry flash row), §8 (In scope)

**Details**:
§5 introduces a brand-new copy element — the blocked-entry flash — with two shape-specific strings ("…isn't available over a remote connection" / "…isn't available on this terminal"). Unlike every other copy renderer in the spec (`UnsupportedNoopMessage`, `unsupportedFlashText`, `GoneMessage`, `PartialFailureMessage`), the spec never states where these strings live, what function renders them, or how the NULL-vs-named branch is selected (presumably `m.detectIdentity.IsNull()`, mirroring `unsupportedFlashText`). §8's "In scope" enumerates the only `internal/spawn` change as `UnsupportedNoopMessage`, which implies the blocked-entry copy is TUI-local and, unlike `UnsupportedNoopMessage`, is **not** shared with the CLI and therefore needs no `cli-verb-surface-redesign` coordination — but this is left to inference. Given the spec's otherwise meticulous single-sourcing discipline and explicit scope enumeration, stating the renderer's home (TUI-local), its shape-selection input, and that it is CLI-uncoupled would remove the guess.

**Proposed Addition**:
New §5 bullet "Blocked-entry flash renderer (TUI-local)": strings live in `internal/tui`, rendered by a new TUI-local helper (e.g. `multiSelectBlockedFlashText(id)`) selecting shape via `m.detectIdentity.IsNull()`; not shared with the CLI, no `cli-verb-surface-redesign` coordination.

**Resolution**: Approved
**Notes**: Auto-applied (Minor). Logged to §5.

---

### 3. Inconsistent flash self-clear trigger wording ("next keypress" vs "next actionable key")

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: §5 (Notes & decisions — Blocked-entry flash behaviour), §6 (Confirmed end-state), §7 (New coverage — copy)

**Details**:
The blocked-entry flash's self-clear trigger is described two different ways: §5 and §7 say it self-clears "on the next actionable key," while §6 says it self-clears "on the next keypress." These are not the same predicate — a non-actionable key would clear under one wording but not the other, and a test asserting the clear behaviour needs one authoritative trigger. Additionally, the reused §11 flash slot has an auto-clear *timer* as well as key-driven clearing (the existing `setFlash` lifecycle), which the spec doesn't mention; an implementer reusing the slot inherits the timer, but the spec's acceptance wording ("self-clears on the next actionable key") could be read as forbidding it. Pin one phrase and note the inherited timer.

**Proposed Addition**:
Pinned "next actionable key" as authoritative in §5 (with "next keypress" noted as shorthand) and corrected §6's "next keypress" → "next actionable key". Added a note in §5 that reusing the §11 slot inherits its auto-clear timer, which is expected and not forbidden.

**Resolution**: Approved
**Notes**: Auto-applied (Minor). Edited §5 and §6.

---

### 4. New NULL visual fixture underspecified — name, seed, and what it proves vs `sessions-flat`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: §7 (Visual)

**Details**:
§7 requires "add a NULL-identity fixture (standard header, no banner)" but leaves three things open. (a) No fixture name is given, though the spec names other delivered frames/fixtures precisely (`sessions-unsupported-terminal`, `sessions-multi-select-active`, etc.). (b) The NULL detection seed is unstated — it would be `InitialDetection = &spawn.Identity{}` (empty BundleID → `IsNull()` true), reusing the existing seed seam the named fixture uses. (c) Most substantively: by design the NULL fixture renders the *standard* `Sessions ··· N` header with no banner, which is visually indistinguishable from the existing `sessions-flat` fixture's header — so the spec should clarify what a dedicated fixture + committed reference PNG proves that a render-level test does not, or whether the visual assertion is really "renders identically to the normal flat header." Without this the implementer must decide the fixture name, seed, and whether a new reference PNG is even warranted.

**Proposed Addition**:
§7 Visual reworked: fixture name `sessions-unsupported-null`, seed `InitialDetection = &spawn.Identity{}` (empty BundleID → IsNull true) via the existing seed seam; states the render-level banner-split test is the primary NULL assertion, and the fixture + committed PNG are added for parity with `sessions-unsupported-terminal` and as a regression anchor against banner intrusion on the NULL seed path.

**Resolution**: Approved
**Notes**: Auto-applied (Minor). Owned the testing-detail call: render-level test primary, fixture+PNG for parity/regression.

---
