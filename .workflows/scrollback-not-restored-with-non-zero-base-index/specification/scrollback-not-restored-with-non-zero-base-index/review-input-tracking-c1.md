---
status: complete
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
**Affects**: Problem & Root Cause → Primary Root Cause

**Details**:
The investigation calls out that `signal-hydrate` failures are invisible because the cobra/pflag parse error is written to stderr, which `tmux run-shell` captures into its own output stream rather than `portal.log`. This is load-bearing context for understanding why this bug went undetected — it explains why a non-zero exit produces no log line and the only observable artefact is the downstream `hydrate timeout` WARN. The spec describes the parse failure but never mentions where the parse-error text goes, leaving a reader to wonder why the failure wasn't logged.

**Current**:
> cobra/pflag parses the leading-dash token as a short-flag cluster, fails with `unknown shorthand flag: 'd'`, and exits non-zero before `runSignalHydrate` executes. No FIFO byte is written; the hydrate helper times out at 3s and exec's a bare `$SHELL` with no scrollback replay.

**Proposed Addition**:
Append a new paragraph after the existing one: "The parse-error text is written to stderr, which `tmux run-shell` captures into its own output stream rather than `portal.log`. As a result, the failure produces no Portal log line — the only observable artefact is the downstream `hydrate timeout` WARN."

**Resolution**: Approved
**Notes**: Added to Primary Root Cause; co-applied with Finding 3's file references in the same paragraph.

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
Convert "Potentially affected" to a bulleted list and add: "User-issued `portal <subcommand> -<dashed-name>` from a shell prompt — same parse-failure class. **Not addressed** by the chosen fix: the `--` separator is added only to the hook command, so a user invoking the CLI manually with a leading-dash positional argument would still hit the parse error. This case is intentionally out of scope (see Out of Scope below)."

**Resolution**: Approved
**Notes**: Added under Blast Radius. Explicitly clarifies that the `--` fix targets the hook path only; user-issued CLI invocations remain a known limitation deferred from this fix.

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
Inline the file/line refs into the existing sentence: "the hydrate helper times out at 3s (`openFIFOWithTimeout` at `cmd/state_hydrate.go:100`) and exec's a bare `$SHELL` (`handleHydrateTimeout` at `cmd/state_hydrate.go:248`) with no scrollback replay."

**Resolution**: Approved
**Notes**: Co-applied with Finding 1 in the same Primary Root Cause paragraph.

---

### 4. Delivery guidance: bundle Part 1 + Part 2 in one PR

**Source**: investigation.md § "Discussion" (line 201) and § "Risk Assessment" (line 214)
**Category**: New topic
**Affects**: Fix Scope (closing paragraph) or a new "Delivery" / "Sequencing" sub-section

**Details**:
Investigation states twice that Part 1 + Part 2 should ship in a single PR — both touch restoration-diagnostics correctness, both are low-complexity, and bundling closes the misleading-WARN loop the bug report opened with. The spec describes the two parts as both required but says nothing about delivery sequencing or whether they can/should land separately. For a bug fix this is normally implicit, but the investigation makes it an explicit recommendation worth preserving.

**Proposed Addition**:
(N/A — see resolution)

**Resolution**: Skipped
**Notes**: User explicitly directed during construction that PR/delivery concerns are out of scope for the spec — "you dont need to say ship together in a single pr. its out of scope to worry about PRs in this spec." Skipping this finding preserves that direction.

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
Append to the existing bullet: "Worth re-evaluating in a separate, larger discussion later — not as a fix for this bug."

**Resolution**: Approved
**Notes**: Added to Out of Scope bullet — preserves the follow-up framing without re-litigating now.

---
