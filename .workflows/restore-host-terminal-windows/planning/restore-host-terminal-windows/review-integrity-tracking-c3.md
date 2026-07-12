---
status: complete
created: 2026-07-12
cycle: 3
phase: Plan Integrity Review
topic: restore-host-terminal-windows
---

# Review Tracking: Restore Host Terminal Windows - Integrity

Cycle-3 integrity review — fresh full pass over `planning.md` and all six `phase-*-tasks.md` files (6 phases, 45 tasks), with a targeted verification of the cycle-2 fix (resolution-based `DetectUnsupported()` predicate, single config-aware `m.resolve` injection site in 6-1, copy branched on `IsNull()`, 6-11 fixture seeding `detectResolution`).

## Cycle-2 fix verification summary

- **(a) `m.resolve` injected exactly once (6-1), reused by 6-3/6-9 without re-injection** — HOLDS. 6-1's `build.go` bullet is the single injection site; 6-3 line 135 explicitly reuses `m.resolve` (not re-injected) and its "remaining seams" list correctly excludes `Resolve`; 6-9 consumes only `DetectUnsupported()`. One minor omission in 6-1's field enumeration (Finding 2).
- **(b) `DetectUnsupported()` defined once (6-1), used consistently in 6-2/6-3/6-9** — HOLDS. Single definition; every gate reads it.
- **(c) 6-2 banner copy vs 6-9 flash copy vs CLI 2-7** — MOSTLY HOLDS; one internal copy asymmetry in 6-9 (Finding 3).
- **(d) 6-11 non-NULL Apple Terminal fixture + seeded resolution drives the banner** — HOLDS. `WithInitialDetection` seeds `detectResolution` via `ResolveAdapter`; `com.apple.Terminal` → `ResolutionUnsupported` → `DetectUnsupported()` true → named banner renders.
- **(e) no dangling `IsNull()`-based gate where a resolution gate is intended** — **DOES NOT HOLD**. 6-2's **Solution** paragraph still specifies the pre-cycle-2 `IsNull()` gate, contradicting the corrected `DetectUnsupported()` gate in its own **Do** section (Finding 1, Important).

## Findings

### 1. Dangling `IsNull()`-based unsupported-banner gate in 6-2's Solution paragraph contradicts its corrected Do section

**Severity**: Important
**Plan Reference**: Phase 6, task `restore-host-terminal-windows-6-2` (Solution paragraph, phase-6-tasks.md line 66)
**Category**: Task Self-Containment / internal consistency (cycle-2 fix residue)
**Change Type**: update-task

**Details**:
The cycle-2 fix moved the proactive unsupported-banner gate from `Identity.IsNull()` to the resolution-based `DetectUnsupported()` predicate. That fix was applied to 6-2's **Do** section (step 3, line 79), which now reads `m.DetectUnsupported()` and even carries the explicit warning: "Do **not** gate on `m.detectIdentity.IsNull()`: that hides the banner for the design's own Apple Terminal example (a non-NULL identity) and fires only for NULL — where the 'unsupported terminal — · ' copy is wrong."

But the task's **Solution** paragraph (line 66) was left describing the OLD gate: "…gated on `m.detectResolved && m.detectIdentity.IsNull() && !m.multiSelectMode`." This is the exact `IsNull()`-based gate the cycle-2 fix eliminated. An implementer who reads the Solution first (its natural role is to state the approach) and implements that gate reintroduces the pre-cycle-2 bug: the banner would fire only for a NULL remote/mosh identity and would NOT render for the design's own non-NULL Apple Terminal (`com.apple.Terminal`) frame — the frame this task is built to reproduce (6-11 asserts it). The two statements within one task directly contradict each other on the load-bearing gate.

Fix: update the Solution paragraph's gate description to the resolution-based `DetectUnsupported()` test so it agrees with the Do section (and with 6-3/6-9, `unsupportedBannerActive()`, and the acceptance criteria).

**Current**:
> **Solution**: Add `renderUnsupportedHeader(name, bundleID, width, mode, colourless) string` in `internal/tui/section_header.go` — an amber `⚠` + "unsupported terminal" (amber) + "— {name} · {bundleID}" (dim `text.detail`) left cluster, right-anchored blue `see docs`, composed through the shared `renderSectionHeaderRow`/`assembleRightAnchoredRow` geometry (same as the multi-select banner from Task 5.3). Insert it as a claimant in the Sessions-page section-header resolver (`applySectionHeader`, `internal/tui/model.go`) below the multi-select banner and above the standard section header, gated on `m.detectResolved && m.detectIdentity.IsNull() && !m.multiSelectMode`. Extend the `activeNoticeBand` gating (`internal/tui/notice_band.go`) so the By-Tag "No tags yet" signpost is also suppressed while the unsupported banner owns the section-header row (the banner outranks the signpost per the precedence).

**Proposed**:
> **Solution**: Add `renderUnsupportedHeader(name, bundleID, width, mode, colourless) string` in `internal/tui/section_header.go` — an amber `⚠` + "unsupported terminal" (amber) + "— {name} · {bundleID}" (dim `text.detail`) left cluster, right-anchored blue `see docs`, composed through the shared `renderSectionHeaderRow`/`assembleRightAnchoredRow` geometry (same as the multi-select banner from Task 5.3). Insert it as a claimant in the Sessions-page section-header resolver (`applySectionHeader`, `internal/tui/model.go`) below the multi-select banner and above the standard section header, gated on `m.DetectUnsupported() && !m.multiSelectMode` — the resolution-based test (`detectResolved && detectResolution == ResolutionUnsupported`, true for a NULL remote/mosh identity **and** a non-NULL recognised-but-undriven identity like Apple Terminal), **not** `m.detectIdentity.IsNull()` (which would hide the banner for the design's own non-NULL `com.apple.Terminal` frame and fire only for NULL — see the Do section). Extend the `activeNoticeBand` gating (`internal/tui/notice_band.go`) so the By-Tag "No tags yet" signpost is also suppressed while the unsupported banner owns the section-header row (the banner outranks the signpost per the precedence).

**Resolution**: Fixed
**Notes**:

---

### 2. 6-1 Model-fields enumeration omits the `resolve` seam field

**Severity**: Minor
**Plan Reference**: Phase 6, task `restore-host-terminal-windows-6-1` (Do, "add `Model` fields" bullet, phase-6-tasks.md line 19)
**Category**: Task Template Compliance / internal consistency
**Change Type**: update-task

**Details**:
The cycle-2 fix designated 6-1 as the single injection site for the config-aware `Resolve` seam, "stored as the `m.resolve` field" (line 18 build.go bullet), and `m.resolve` is consumed heavily downstream: the `terminalDetectedMsg` arm (`_, m.detectResolution = m.resolve(msg.identity)`, line 21), 6-3's burst dispatch (`adapter, resolution := m.resolve(m.detectIdentity)`, line 141), and 6-9's gate. But 6-1's explicit "In `internal/tui/model.go` add `Model` fields:" enumeration (line 19) lists only `detector`, `detectIdentity`, `detectResolution`, `detectResolved`, `detectDispatched` — it does not declare the `resolve` field. By contrast, 6-3's parallel "Model fields to add:" list (line 143) is complete for its burst fields.

Because `m.resolve` is referenced so prominently, an implementer would add the field and a missed declaration would be caught by a compile error at `m.resolve(...)`, so impact is low (self-correcting). But the field-list bullet is the canonical declaration site, and the omission of the centerpiece seam of the cycle-2 fix from it is a real inconsistency worth correcting so the field's ownership (6-1, single site) is unambiguous.

**Current**:
> - In `internal/tui/model.go` add `Model` fields: `detector TerminalDetector`, `detectIdentity spawn.Identity`, `detectResolution spawn.Resolution`, `detectResolved bool`, `detectDispatched bool`. Add test accessors mirroring the existing convention: `func (m Model) DetectDispatched() bool`, `func (m Model) DetectResolved() bool`, `func (m Model) DetectedIdentity() spawn.Identity`, `func (m Model) DetectedResolution() spawn.Resolution`, and `func (m Model) DetectUnsupported() bool { return m.detectResolved && m.detectResolution == spawn.ResolutionUnsupported }` — the single "this terminal cannot spawn host windows" predicate, **true for a NULL remote/mosh identity AND a non-NULL recognised-but-undriven identity (e.g. Apple Terminal → `com.apple.Terminal`)**. This is the resolution-based unsupported test the proactive banner (6-2) and the N≥2 gate (6-3/6-9) share; `IsNull()` alone is NOT the unsupported test (a non-NULL undriven identity is unsupported by *resolution*, not by NULL-ness — same as CLI task 2-7).

**Proposed**:
> - In `internal/tui/model.go` add `Model` fields: `detector TerminalDetector`, `resolve func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` (the config-aware resolve seam injected via the `build.go` bullet above — the single field the `terminalDetectedMsg` arm consumes and that Task 6.3's burst dispatch and Task 6.9's gate reuse; never re-injected downstream), `detectIdentity spawn.Identity`, `detectResolution spawn.Resolution`, `detectResolved bool`, `detectDispatched bool`. Add test accessors mirroring the existing convention: `func (m Model) DetectDispatched() bool`, `func (m Model) DetectResolved() bool`, `func (m Model) DetectedIdentity() spawn.Identity`, `func (m Model) DetectedResolution() spawn.Resolution`, and `func (m Model) DetectUnsupported() bool { return m.detectResolved && m.detectResolution == spawn.ResolutionUnsupported }` — the single "this terminal cannot spawn host windows" predicate, **true for a NULL remote/mosh identity AND a non-NULL recognised-but-undriven identity (e.g. Apple Terminal → `com.apple.Terminal`)**. This is the resolution-based unsupported test the proactive banner (6-2) and the N≥2 gate (6-3/6-9) share; `IsNull()` alone is NOT the unsupported test (a non-NULL undriven identity is unsupported by *resolution*, not by NULL-ness — same as CLI task 2-7).

**Resolution**: Fixed
**Notes**:

---

### 3. 6-9 re-assert flash copy is internally inconsistent (named case omits "— nothing opened", NULL case includes it)

**Severity**: Minor
**Plan Reference**: Phase 6, task `restore-host-terminal-windows-6-9` (Do setFlash bullet line 457; acceptance criterion line 465)
**Category**: Acceptance Criteria Quality / copy consistency (review item c)
**Change Type**: update-task

**Details**:
6-9's N≥2-Enter unsupported no-op re-asserts the warning as a transient flash whose copy branches on `IsNull()`:
- named (non-NULL, e.g. Apple Terminal): `⚠ unsupported terminal — <name> · <bundleID>` (no outcome suffix)
- NULL (remote/mosh): `⚠ no host-local terminal — nothing opened` (carries the outcome suffix)

Both are the response to the *same* N≥2 Enter no-op, so the asymmetric outcome suffix is internally inconsistent: the named flash tells the user the terminal is unsupported but not that nothing opened, while the NULL flash conveys both. (CLI task 2-7's stderr message carries "— nothing opened" on BOTH branches — `spawn: unsupported terminal — <name> · <bundleID> — nothing opened` and `spawn: no host-local terminal — nothing opened` — so the named flash also diverges from the CLI it claims to mirror.) The related 6-2 *banner* copy (a persistent state indicator, not an Enter response) legitimately omits the suffix on both branches; the divergence there is banner-vs-flash context. The issue is confined to 6-9's flash, where the two branches should agree.

Fix: make the named flash carry "— nothing opened" too, so both 6-9 flash branches convey the no-op outcome and align with CLI 2-7. (This keeps the flash distinct from the 6-2 proactive banner, which stays suffix-free as a state indicator — an intentional banner-vs-flash distinction that already exists for the NULL case.) Copy is explicitly "not pinned by the spec beyond naming the identity + conveying nothing opened", so this is a low-risk copy-polish alignment.

**Current**:
> - Re-assert the unsupported warning: `m.setFlash(unsupportedFlashText(m.detectIdentity))` where the **copy branches on `IsNull()`** (mirroring CLI Task 2.7): a named identity (non-NULL — e.g. Apple Terminal) yields `⚠ unsupported terminal — <name> · <bundleID>`; a NULL-with-no-identity (`Name`/`BundleID` empty, remote/mosh) folds to `⚠ no host-local terminal — nothing opened`. (A transient flash because the multi-select banner owns the section-header row in mode.)
>
> …(acceptance criterion)…
> - [ ] A non-NULL undriven identity re-asserts `⚠ unsupported terminal — <name> · <bundleID>`; a bare-NULL identity re-asserts `⚠ no host-local terminal — nothing opened` (copy branches on `IsNull()`, gate branches on resolution — matching CLI Task 2.7).

**Proposed**:
> - Re-assert the unsupported warning: `m.setFlash(unsupportedFlashText(m.detectIdentity))` where the **copy branches on `IsNull()`** (mirroring CLI Task 2.7): a named identity (non-NULL — e.g. Apple Terminal) yields `⚠ unsupported terminal — <name> · <bundleID> — nothing opened`; a NULL-with-no-identity (`Name`/`BundleID` empty, remote/mosh) folds to `⚠ no host-local terminal — nothing opened`. Both flash branches carry the `— nothing opened` outcome because this flash is the response to the N≥2 Enter no-op (distinct from 6-2's *proactive banner*, a persistent state indicator that omits the suffix on both branches). (A transient flash because the multi-select banner owns the section-header row in mode.)
>
> …(acceptance criterion)…
> - [ ] A non-NULL undriven identity re-asserts `⚠ unsupported terminal — <name> · <bundleID> — nothing opened`; a bare-NULL identity re-asserts `⚠ no host-local terminal — nothing opened` (both flash branches carry the `— nothing opened` outcome; copy branches on `IsNull()`, gate branches on resolution — matching CLI Task 2.7).

**Resolution**: Fixed
**Notes**:

---

### 4. 6-6 `spawnCompleteMsg` handler does not handle `msg.Err != nil` (Burster.Run pre-spawn abort) → degenerate empty-named flash

**Severity**: Minor
**Plan Reference**: Phase 6, task `restore-host-terminal-windows-6-6` (Do, `case spawnCompleteMsg:` completion, phase-6-tasks.md lines 296-297)
**Category**: Acceptance Criteria Quality / edge-case coverage
**Change Type**: add-to-task

**Details**:
`spawn.Burster.Run` returns a non-nil `err` (with empty `results`) for a pre-spawn abort — an `os.Executable` resolution failure or an ack-id generation failure, before any window opens (Task 3.5). In the picker this surfaces as `spawnCompleteMsg{Err: err, Results: nil}`. Task 6.4's `allConfirmed` requires `msg.Err == nil`, so an `Err != nil` message falls to Task 6.6's "not-all-confirmed" arm. But 6-6 computes `confirmed`/`failed` from `msg.Results` — which is empty on the Err path — so `failed` is empty, and the else-branch flash `fmt.Sprintf("⚠ %s failed to open — others left open", quoteJoin(failedNames))` renders with an empty name list (`⚠  failed to open — others left open`), while `confirmed` being empty unmarks nothing and leaves every session marked. The user sees a malformed, nameless failure message and a retry re-hits the same pre-spawn error.

The CLI handles the same `Burster.Run` error by returning it (exit 1, Task 3.5). The picker has no explicit branch for it, forcing the implementer to decide how to render it. This is a rare path (both failure modes are uncommon on macOS), hence Minor, but it is a genuine gap that produces incorrect user-facing output.

Fix: add a leading `msg.Err != nil` guard to the completion arm that surfaces a generic flash (opaque error to the DEBUG log only), leaves the selection unchanged, stays in multi-select mode, and skips the confirmed/failed split. Add a matching acceptance criterion and test.

**Current**:
> - In `internal/tui/model.go`, complete the `case spawnCompleteMsg:` arm (the branch not taken by Task 6.4's `allConfirmed`):
>   - Compute `confirmed := {r.Session : r.Ack == spawn.AckConfirmed}` and `failed := {r.Session : r.Ack != spawn.AckConfirmed}` from `msg.Results`. (Un-attempted external sessions after a permission stop are neither confirmed nor in `Results` — they stay marked because they are not in `confirmed`.)

**Proposed**:
> - In `internal/tui/model.go`, complete the `case spawnCompleteMsg:` arm (the branch not taken by Task 6.4's `allConfirmed`):
>   - **First handle `msg.Err != nil`** — a pre-spawn abort returned by `Burster.Run` (an `os.Executable` resolution failure or an ack-id generation failure, Task 3.5) that occurred *before any window opened*, so `msg.Results` is empty: `m.setFlash("⚠ could not start opening windows")` (the opaque `msg.Err` string rides only to the DEBUG log — Task 6.10 — never the user-facing flash), leave the selection **unchanged** (nothing opened → nothing to unmark), clear burst-pending (`m.burstPending = false`, nil the pipe/cancel, zero counters), stay in multi-select mode, and **return without** running the confirmed/failed computation below (there are no results to split). This is the picker analogue of the CLI's `return err` on the same `Burster.Run` error (Task 3.5), surfaced as a flash instead of an exit. The confirmed/failed logic below runs only when `msg.Err == nil`.
>   - Compute `confirmed := {r.Session : r.Ack == spawn.AckConfirmed}` and `failed := {r.Session : r.Ack != spawn.AckConfirmed}` from `msg.Results`. (Un-attempted external sessions after a permission stop are neither confirmed nor in `Results` — they stay marked because they are not in `confirmed`.)
>
> …add to Acceptance Criteria…
> - [ ] A `Burster.Run` pre-spawn error (`msg.Err != nil`, empty `Results`) surfaces a generic `⚠ could not start opening windows` flash, leaves the selection unchanged, clears burst-pending, and stays in multi-select mode — no degenerate empty-named "failed to open" message.
>
> …add to Tests…
> - `"it surfaces a generic flash and leaves selection unchanged on a Burster.Run pre-spawn error"`

**Resolution**: Fixed
**Notes**:

---

## Areas verified clean (no findings)

- **Burster.Run signature evolution** (2-6 `SpawnWindows` → 3-5 `Burster.Run(external)` → 6-3 `Run(ctx, external, progress)`): each transition is documented; 6-3 explicitly updates the CLI call site (`context.Background(), external, nil`) and Phase-3 test call sites in the same change. Clean.
- **`Opening n/N…` denominator (N vs N−1)**: 6-3 sets `burstTotal = len(ordered) = N`; 6-5 explicitly holds the denominator at N and ignores `msg.Total` (= N−1), advancing `burstDone` 0…N−1 so the band never reads `N/N` (consistent with "no N/N nag" and 6-10's `total=N`). Reconciled.
- **`AckChannelFull` declaration/consumers** (3-2 declares; 3-5 `SpawnDeps.Ack`, 6-3 `tui.Deps.AckChannel` reference it; `FakeAckChannel` satisfies it). Consistent.
- **Esc precedence across 5.1 / 6-5 / 6-7**: burst-pending Esc → cancel (6-8) at top; abort-banner Esc → dismiss (6-7); else mode-exit (5.1). Coherently ordered.
- **6-11 capture seeds**: `WithInitialDetection` seeds `detectResolution` via `ResolveAdapter`; the non-NULL `com.apple.Terminal` fixture correctly drives `DetectUnsupported()` and the named banner (the fixture is not in multi-select mode, so `applySectionHeader` step 3 renders it).
- **Count semantics** (`total=N`, `opened` includes trigger on full success only) consistent across 2-6 / 3-5 / 6-10.
- **Detection dispatch ordering** (post-`BootstrapCompleteMsg` / warm `SessionsMsg`, off the appearance gate, `detectDispatched` latch) — never re-walks; concurrency with hydration avoided.

## Note on deferred visual-placement ambiguity (not a finding)

Tasks 6-6, 6-7, and 6-9 repeatedly defer the exact flash-vs-banner placement (`▌` notice band vs section-header row) to the 6-11 visual gate, consistent with the 5.3/6.2 design-anchored placement decisions. This is an acknowledged, uniformly-flagged design residual settled at the visual gate — acceptable for a plan and not raised as a finding.
