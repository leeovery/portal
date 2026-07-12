---
status: in-progress
created: 2026-07-12
cycle: 4
phase: Traceability Review
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Traceability

Cycle-4 late-convergence pass. Full fresh bidirectional trace of the specification against the 6-phase / 45-task plan (planning.md + phase-1..6-tasks.md).

**Both directions verified clean except the one finding below.** Every spec section — Overview, Spawn Architecture, Multi-Select Mode, Burst & Partial-Failure Contract, Trigger-Context Matrix, Terminal Identity & Detection, Adapter Contract, Config Schema (terminals.json), Permissions & Error Quarantine, Observability & State Footprint, Concurrency & Post-Reboot Safety, Testing Strategy, Design References, and Dependencies/Deferred/Build-Time Residuals — has task coverage with implementer-grade detail, and every task traces to a cited spec section. All deferred scope (group-select, Spaces/workspace-restore, parallel spawn, headless `portal spawn`/`--terminal`, defensive marker sweep, detect-and-wait one-bootstrap-cap) is correctly excluded from the plan. All prior-cycle fixes were re-verified as holding: the config-resolver parity (picker reuses the config-aware `spawn.NewResolver(terminals.json).Resolve`, 6-1/6-3), the `Opening n/N…` denominator = N reasoning (6-5/6-10/6-11), the `Burster.Run` ctx/progress signature migration + call-site updates (6-3), the `AckChannelFull` declaration (3-2), the resolution-based `DetectUnsupported()` classification (6-1/6-2/6-3/6-9), the `IsNull()`-only copy-branch (not gate) in 6-2, the 6-1 single `resolve` field, the 6-9 flash copy, and the 6-6 pre-spawn-error (`msg.Err != nil`, empty results) branch.

## Findings

### 1. Pre-flight abort banner copy drops "is" from the delivered design copy (Task 6.7 Do contradicts its own Acceptance Criterion)

**Type**: Incomplete coverage
**Spec Reference**: *Burst & Partial-Failure Contract → Stance: pre-flight + all-or-nothing* (design copy `⚠ '<session>' is gone — nothing opened`, spec line 161); *Design References → Sessions — Multi-Select (pre-flight abort)* (spec line 504, same copy)
**Plan Reference**: `restore-host-terminal-windows-6-7` (primary — Do bullet, `renderPreflightAbortHeader` message; the design-gated site) and `restore-host-terminal-windows-3-4` (parallel CLI abort message); visually gated by `restore-host-terminal-windows-6-11`
**Change Type**: update-task

**Details**:
The delivered, approved design copy for the pre-flight abort message — stated twice in the spec (Stance §, line 161; Design References §, line 504) — is `⚠ '<session>' is gone — nothing opened`, with the verb **is**.

Task 6.7's Do section constructs the banner text as `fmt.Sprintf("%s gone — nothing opened", quoteJoin(msg.Gone))`, which for a single gone session renders `⚠ 'fab-flowx-explore' gone — nothing opened` — **dropping the "is"**. This diverges from the frame in three ways:

1. **Internal contradiction inside Task 6.7.** The task's own Outcome (line 351: `shows '⚠ '<session>' is gone — nothing opened''`) and Acceptance Criterion (line 365: `renders '⚠ '<session>' is gone — nothing opened' (red)…`) both specify **is gone**, while the Do-section format string produces **gone** (no "is"). The Do as written cannot satisfy the task's own AC.
2. **Design-frame divergence.** Task 6.11's visual gate captures `sessions-multi-select-preflight-abort` and compares it against `testdata/vhs/reference/sessions-multi-select-preflight-abort-mv.png` (the delivered frame, which reads `is gone`). The rendered capture (`… gone …`) would mismatch the reference at the gate.
3. **CLI parity.** Task 3.4's CLI abort uses the identical `%s gone` format (`spawn: %s gone — nothing opened`). The spec requires `portal spawn` to emit "the same one-line message the picker would show" (*Reporting & exit codes*), so both sites must carry the fix in lockstep. (Task 3.4's own note that "the exact CLI wording is not pinned" is the reason it drifted; parity with the picker's design-pinned copy resolves it.)

The plan clearly chose `%s gone` for plural-safety (`'s2', 's4' gone` reads better than `'s2', 's4' is gone`). A count-aware verb (`is` for one, `are` for several) preserves that plural-safety **and** matches the singular design copy exactly. This keeps the plan a faithful translation of the approved frame without re-litigating the plural handling the spec's "naming the gone session(s)" language allows.

**Current** (Task 6.7 — Do bullet, "The abort banner"):
> - The abort banner: set a transient abort state (reuse the `flashText`/`flashKind` mechanism with a red/warning kind, or a dedicated `abortBannerText` field) rendered at the section-header row via `func renderPreflightAbortHeader(message string, width int, mode theme.Mode, colourless bool) string` in `section_header.go` — a red `⚠` (`theme.MV.StateRed` — the existing error token) + `fmt.Sprintf("%s gone — nothing opened", quoteJoin(msg.Gone))` (matching the design copy `⚠ '<session>' is gone — nothing opened`), right-anchored dim `esc dismiss` (`theme.MV.TextDetail`), through `renderSectionHeaderRow`. It sits above the multi-select banner in the section-header precedence (the transient flash/abort claimant, per the notice-band precedence).

**Proposed** (Task 6.7 — Do bullet, "The abort banner"):
> - The abort banner: set a transient abort state (reuse the `flashText`/`flashKind` mechanism with a red/warning kind, or a dedicated `abortBannerText` field) rendered at the section-header row via `func renderPreflightAbortHeader(message string, width int, mode theme.Mode, colourless bool) string` in `section_header.go` — a red `⚠` (`theme.MV.StateRed` — the existing error token) + `fmt.Sprintf("%s %s gone — nothing opened", quoteJoin(msg.Gone), goneVerb(len(msg.Gone)))`, right-anchored dim `esc dismiss` (`theme.MV.TextDetail`), through `renderSectionHeaderRow`. Add the tiny count-aware verb helper `func goneVerb(n int) string { if n == 1 { return "is" }; return "are" }`, so a single gone session renders `⚠ 'fab-flowx-explore' is gone — nothing opened` — **byte-matching the delivered design copy `⚠ '<session>' is gone — nothing opened` and Task 6.7's own Outcome/Acceptance Criterion** — while several gone sessions render `⚠ 's2', 's4' are gone — nothing opened` (grammatical plural, preserving the plan's plural-safety). It sits above the multi-select banner in the section-header precedence (the transient flash/abort claimant, per the notice-band precedence).

**Current** (Task 3.4 — Do bullet, the abort return):
> - `return fmt.Errorf("spawn: %s gone — nothing opened", quoteJoin(gone))` where `quoteJoin` renders `'s2'` for one and `'s2', 's4'` for several (mirrors the design copy `⚠ '<session>' is gone — nothing opened`; the exact CLI wording is not pinned beyond naming the gone session(s) + conveying nothing opened). A plain, non-`UsageError`, non-silenced error → exit 1 on stderr.

**Proposed** (Task 3.4 — Do bullet, the abort return):
> - `return fmt.Errorf("spawn: %s %s gone — nothing opened", quoteJoin(gone), goneVerb(len(gone)))` where `quoteJoin` renders `'s2'` for one and `'s2', 's4'` for several and `goneVerb` returns `"is"` for one / `"are"` for several — so the one-line message is `spawn: 's2' is gone — nothing opened` (singular) / `spawn: 's2', 's4' are gone — nothing opened` (plural), the **same one-line message the picker shows** (spec *Reporting & exit codes*), matching the delivered design copy `⚠ '<session>' is gone — nothing opened` in the singular case. A plain, non-`UsageError`, non-silenced error → exit 1 on stderr.

**Resolution**: Pending
**Notes**: Low severity (a single dropped verb), but it is a concrete divergence from a twice-stated approved design copy, it makes Task 6.7's Do inconsistent with its own Outcome + Acceptance Criterion, and it is compared against the delivered frame at the 6-11 visual gate. `goneVerb` can live wherever `quoteJoin` lives (shared `internal/spawn` helper, since both `cmd/spawn.go` and `internal/tui` already reference `quoteJoin`-style joining) so the picker and CLI stay in lockstep. No other spec/plan divergence was found this cycle.

---
