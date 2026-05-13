---
status: complete
created: 2026-05-13
cycle: 1
phase: Input Review
topic: distinguish-transport-errors-in-getserveroption
---

# Review Tracking: distinguish-transport-errors-in-getserveroption - Input Review

## Findings

### 1. Rejected design alternatives not recorded

**Source**: investigation §Fix Direction → "Options Explored" (B, C, D, E)
**Category**: New topic
**Affects**: Design section (could be a new "Alternatives Considered" subsection)

**Details**:
The investigation enumerates four rejected approaches with concrete rejection reasoning:
- **B.** `fmt.Errorf("%w: %s", err, stderr)` wrap — rejected: discriminator becomes substring-match against formatted error string, brittle, harder to test the boundary cleanly.
- **C.** New `Commander.RunWithStderr(args...) (out, stderr string, err error)` method — rejected: parallel API surface forces every mock to stub a second method; doesn't fix similar latent conflations.
- **D.** Inline type-assert against `*exec.ExitError` inside `GetServerOption` — rejected: couples `internal/tmux` to `os/exec` semantics through the public discriminator site; mocks would need synthetic `*exec.ExitError` instances.
- **E.** String-match against `err.Error()` (no wrap) — rejected on examination: `(*exec.ExitError).Error()` returns `"exit status 1"`; stderr is on `.Stderr`, not in the error string. Mechanically broken without first wrapping.

**Proposed Addition**:
Added a new "Alternatives Considered" section after "Risk & Rollout" enumerating B/C/D/E with rejection reasoning, plus a closing line on why the chosen approach was preferred.

**Resolution**: Approved
**Notes**: Added as new section after Risk & Rollout.

---

### 2. Empty option name → "ambiguous option:" probe result

**Source**: investigation §Code Trace → tmux error surface table (row 3)
**Category**: Enhancement to existing topic
**Affects**: Design: Discrimination in `GetServerOption` → "Option-absent pattern family"

**Details**:
The investigation's probing table documents that `tmux show-option -sv ""` (empty option name) returns `ambiguous option: ` (with trailing space, treated as ambiguous match). Tightening the bullet to capture this empirical origin.

**Proposed Addition**:
Replaced the parenthetical "(including the empty string case observed during investigation)" with an explicit probe note naming `show-option -sv ""`, Darwin 25.3.0, and the trailing-space artefact.

**Resolution**: Approved
**Notes**: Applied via in-place replacement of the third pattern-family bullet.

---

### 3. RealCommander uses cmd.Output() to auto-populate exec.ExitError.Stderr

**Source**: investigation §Code Trace → paragraph following the stderr table
**Category**: Enhancement to existing topic
**Affects**: Design: `CommandError` at the Commander Layer → "Wiring at `RealCommander`"

**Details**:
The `cmd.Stderr == nil` precondition for `cmd.Output()` to auto-populate `(*exec.ExitError).Stderr` is load-bearing. If a future change ever assigns `cmd.Stderr`, wrapping silently breaks.

**Proposed Addition**:
Extended the `*exec.ExitError` bullet to state the precondition explicitly and call out the invariant the wiring must preserve (or capture stderr explicitly via `StderrPipe`).

**Resolution**: Approved
**Notes**: Applied to the first wiring bullet.

---

### 4. Contributing factor: Commander interface signature erases stderr distinction

**Source**: investigation §Analysis → "Contributing Factors" (first bullet)
**Category**: Enhancement to existing topic
**Affects**: Problem & Goal (Problem section) — or could fold into Design rationale

**Details**:
Structural justification for fixing at the Commander layer (vs inside `GetServerOption`) was implicit. Added a "Why this layer" preamble to the CommandError design section.

**Proposed Addition**:
Added a "Why this layer" subsection to "Design: `CommandError` at the Commander Layer" anchoring the choice in the interface signature's stderr-erasure.

**Resolution**: Approved
**Notes**: Folded into the Design section rather than the Problem section to keep Problem focused on the symptom.

---

### 5. Why-not-caught: existing test uses synthetic string never inspected

**Source**: investigation §Analysis → "Why It Wasn't Caught" (first bullet)
**Category**: Enhancement to existing topic
**Affects**: Testing → "Reshape existing TestGetServerOption" entry

**Details**:
Existing test passes because every error becomes `ErrOptionNotFound` regardless of stderr content. The reshape must replace the bare `errors.New(...)` with a `*CommandError` that actually exercises stderr inspection.

**Proposed Addition**:
Rewrote the reshape bullet to call out that the existing error string is decorative, motivating reshape (not an additive test).

**Resolution**: Approved
**Notes**: Applied to the first Testing bullet under `internal/tmux/tmux_test.go`.

---

### 6. Compatibility-floor pin for pattern family

**Source**: investigation §Fix Direction → "Discussion" (second deferred item)
**Category**: Gap/Ambiguity
**Affects**: Design: Discrimination in `GetServerOption` → "Option-absent pattern family"

**Details**:
The project does not pin a tmux minimum version anywhere. The pattern set is best-effort across versions; discriminator unit tests lock the contract behaviourally.

**Proposed Addition**:
Added a "Compatibility floor" paragraph stating the project pins no tmux floor, the set is empirically derived from the investigation baseline (Darwin 25.3.0), and discriminator unit tests are the contract-locking mechanism.

**Resolution**: Approved
**Notes**: Replaced the bare "compatibility window" claim with explicit best-effort framing.

---

### 7. Original motivation context (hook-executor two-condition check)

**Source**: investigation §Notes (first bullet)
**Category**: Enhancement to existing topic
**Affects**: Problem & Goal (Problem section) — historical context

**Details**:
Original bug framing was about a hook-executor "two-condition check" that no longer exists; concern migrated to daemon restoration-window reads. Captured as a historical note in the Problem section.

**Proposed Addition**:
Added a "Historical note" paragraph after the latency-bug paragraph in the Problem section, referencing the archived inbox entry and the migration that re-framed the bug.

**Resolution**: Approved
**Notes**: Applied to Problem section.

---

### 8. Reproduction harness shape (fault-injection mock pattern)

**Source**: investigation §Symptoms → "Reproduction Steps"
**Category**: Enhancement to existing topic
**Affects**: Testing section

**Details**:
Added the `lost server` case (mid-session server crash) to the transport-error test catalog and named the fault-injection harness shape explicitly.

**Proposed Addition**:
Reshaped `TestGetServerOption_TransportError` into a parametrised test over a set of representative non-absent stderr shapes (socket-connect failure + `lost server`), and named the harness as the existing `Commander` mock returning synthetic exit-1 + stderr.

**Resolution**: Approved
**Notes**: Applied to the TestGetServerOption_TransportError bullet.

---

### 9. Wide-scope audit results (clean adjacent patterns) not explicit

**Source**: investigation §Analysis → "Adjacent Patterns Audited (Wide Scope)" and §Notes (second bullet)
**Category**: Enhancement to existing topic
**Affects**: Scope → "Out of scope"

**Details**:
Enumerated audited-clean surfaces (`ShowAllServerOptions`, `ListSkeletonMarkers`, `SetServerOption`, `UnsetServerOption`) in Out-of-scope to document the audit findings in the spec rather than only in the investigation.

**Proposed Addition**:
Expanded the "no other internal/tmux method" bullet into a sub-list naming each audited-clean surface and its propagation shape, plus a closing "isolated to GetServerOption" statement.

**Resolution**: Approved
**Notes**: Applied to Out of scope.

---

### 10. Environment scope: Darwin + Linux, no platform branching

**Source**: investigation §Symptoms → "Environment"
**Category**: Enhancement to existing topic
**Affects**: Scope or Risk & Rollout

**Details**:
Added an explicit platform-applicability line to Risk & Rollout, stating Darwin + Linux and no platform-conditional code.

**Proposed Addition**:
Added a "Platform applicability" bullet to Risk & Rollout.

**Resolution**: Approved
**Notes**: Folded into Risk & Rollout rather than Scope (sits naturally with Compatibility and Rollout claims).

---
