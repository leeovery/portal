---
status: in-progress
created: 2026-04-30
cycle: 1
phase: Input Review
topic: scrollback-not-restored-with-non-zero-base-index
---

# Review Tracking: scrollback-not-restored-with-non-zero-base-index - Input Review

## Findings

### 1. Stderr capture explains invisibility of the parse error

**Source**: investigation.md § "Contributing Factors" (line 155) and § "Why It Wasn't Caught" (line 159)
**Category**: Enhancement to existing topic
**Affects**: Problem & Root Cause → Primary Root Cause (or a new "Why It Wasn't Caught" sub-section)

**Details**:
The investigation calls out that `signal-hydrate` failures are invisible because the cobra/pflag parse error is written to stderr, which `tmux run-shell` captures into its own output stream rather than `portal.log`. This is load-bearing context for understanding why this bug went undetected — it explains why a non-zero exit produces no log line and the only observable artefact is the downstream `hydrate timeout` WARN. The spec describes the parse failure but never mentions where the parse-error text goes, leaving a reader to wonder why the failure wasn't logged.

**Current**:
> cobra/pflag parses the leading-dash token as a short-flag cluster, fails with `unknown shorthand flag: 'd'`, and exits non-zero before `runSignalHydrate` executes. No FIFO byte is written; the hydrate helper times out at 3s and exec's a bare `$SHELL` with no scrollback replay.

**Proposed Addition**:
(blank — to be discussed)

**Resolution**: Pending
**Notes**:

---

### 2. Blast radius missing user-issued CLI invocations

**Source**: investigation.md § "Blast Radius" → "Potentially affected" bullet 2 (line 169)
**Category**: Enhancement to existing topic
**Affects**: Problem & Root Cause → Blast Radius

**Details**:
Investigation explicitly notes that `portal attach -dashed-session` invoked from a shell prompt hits the same parse failure class — unrelated to hook firing but the same root cause and therefore fixed (or not) by the same change. The spec's Blast Radius restricts "Potentially affected" to hook-invoked subcommands only and omits the user-issued CLI angle. Whether the chosen fix (`--` on the hook only) covers this case is a relevant scoping detail; today the spec is silent.

**Current**:
> **Potentially affected:** Any other Portal subcommand invoked from a tmux hook with `#{session_name}` as a positional arg. `signalHydrateCommand` is currently the only such site (per `internal/tmux/hooks_register.go`); `notifyCommand` is argument-free and unaffected.

**Proposed Addition**:
(blank — to be discussed)

**Resolution**: Pending
**Notes**:

---

### 3. Hydrate-helper timeout path file/line references

**Source**: investigation.md § "Code Trace" (lines 130-138)
**Category**: Enhancement to existing topic
**Affects**: Problem & Root Cause → Primary Root Cause

**Details**:
The investigation's Code Trace pinpoints where the 3s timeout originates (`cmd/state_hydrate.go:100`, `openFIFOWithTimeout`) and where the WARN + `$SHELL` exec happens (`handleHydrateTimeout` at `state_hydrate.go:248`). The spec mentions "the hydrate helper times out at 3s and exec's a bare `$SHELL`" but provides no file references for that side of the path, while it does provide line references for every other component (hooks_register.go:39, session.go:153/195/215/354, etc.). The asymmetry is minor but the helper-side citations help readers verify the trace end-to-end.

**Current**:
> No FIFO byte is written; the hydrate helper times out at 3s and exec's a bare `$SHELL` with no scrollback replay.

**Proposed Addition**:
(blank — to be discussed)

**Resolution**: Pending
**Notes**:

---

### 4. Delivery guidance: bundle Part 1 + Part 2 in one PR

**Source**: investigation.md § "Discussion" (line 201) and § "Risk Assessment" (line 214)
**Category**: New topic
**Affects**: Fix Scope (closing paragraph) or a new "Delivery" / "Sequencing" sub-section

**Details**:
Investigation states twice that Part 1 + Part 2 should ship in a single PR — both touch restoration-diagnostics correctness, both are low-complexity, and bundling closes the misleading-WARN loop the bug report opened with. The spec describes the two parts as both required but says nothing about delivery sequencing or whether they can/should land separately. For a bug fix this is normally implicit, but the investigation makes it an explicit recommendation worth preserving.

**Proposed Addition**:
(blank — to be discussed)

**Resolution**: Pending
**Notes**:

---

### 5. `SanitiseProjectName` substitution flagged as a separate future discussion

**Source**: investigation.md § "Notes" (line 221)
**Category**: Enhancement to existing topic
**Affects**: Fix Scope → Out of Scope

**Details**:
The investigation's closing Notes section flags that `SanitiseProjectName`'s `.` → `-` substitution "is itself questionable (could be `_` instead) but changing it is a separate, larger discussion (existing users have sessions named with the current scheme)." The spec's Out of Scope rejects renaming the substitution but frames the rejection purely on technical grounds (doesn't address the broader class, backwards-incompatible). It does not capture the investigation's framing that this is a legitimate follow-up worth a separate discussion later — only that it's rejected here. Preserving the "separate, larger discussion" hook keeps the door open without re-litigating it now.

**Current**:
> - **Renaming `SanitiseProjectName`'s `.` → `-` substitution to `_` or another safe char.** Fixes one symptom (no more leading-dash names from dotfiles projects) but leaves the broader class — any user-issued or scripted invocation passing `-anything` to a hook-invoked Portal subcommand would still break. Also a backwards-incompatible change for existing users whose projects/sessions use the current scheme.

**Proposed Addition**:
(blank — to be discussed)

**Resolution**: Pending
**Notes**:

---
