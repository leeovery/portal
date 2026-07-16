---
status: in-progress
created: 2026-07-16
cycle: 1
phase: Gap Analysis
topic: ghostty-spawn-zero-windows
---

# Review Tracking: ghostty-spawn-zero-windows - Gap Analysis

## Findings

### 1. Fix 4 — `osacompile` invocation mechanics are unspecified (implementer must design them)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Fix 4 (Prevention) — Compile-check regression guard"; "Testing & Validation Requirements" (Prevention compile-check bullet)

**Details**:
The spec pins the *input* (`ghosttyOpenScript(<representative composed argv>)`), the *tool* (`osacompile`), and the *assertion* (zero exit), but leaves the actual invocation mechanics undefined — and `osacompile` is not a drop-in for the `osascript -e` form the rest of the file uses. Open decisions the implementer is forced to make:
- **How the script is fed.** `ghosttyOpenScript` returns a raw script string; `ghosttyOpenArgv` wraps it as `osascript -e <script>`. Does the compile-check reuse `-e <script>` (`osacompile -e <script> …`) or write the script to a temp `.applescript` file first? The spec says only "feed … through a compile-only osascript path."
- **Where compiled output goes.** `osacompile` writes a compiled artifact and effectively requires an `-o <path>` output target (unlike `osascript`, it does not just parse-and-discard). The spec does not name an output target (temp `.scpt`? a throwaway path?) nor whether/how that artifact is cleaned up.
- **What "representative composed argv" is concretely.** Guidance is given ("`/usr/bin/env -u TMUX -u TMUX_PANE …` argv"), but no exact fixture value; the implementer must synthesise one that exercises the template + `ghosttyEmbed` escaping.

Because `osacompile`'s CLI contract differs from the `osascript` path already in the file, an implementer cannot lift the existing `ghosttyOpenArgv` shape — they must design the compile-check argv and output handling from scratch. This is the one place in the spec where a real design decision is left open.

**Proposed Addition**:
Specify the compile-check invocation concretely: the exact `osacompile` argv form (e.g. `-e <script>` vs temp-file input), the output target and its cleanup (e.g. compile to a `t.TempDir()` `.scpt`), and a concrete representative composed-argv fixture — so the test is a mechanical build, not a design task.

**Resolution**: Approved
**Notes**: Appended a concrete "Invocation (concrete)" paragraph to Fix 4 pinning the `osacompile -e <script> -o <out>` form, `t.TempDir()` output, zero-exit assertion, and a fixed representative argv fixture.

---

### 2. Fix 2 — WARN condition also fires for ack-timeout windows the adapter reported as Success (intent vs. literal rule)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Fix 2 (Rider #1) — Surface per-window failure reason at WARN"

**Details**:
The WARN condition is stated literally as: failed (`!r.Confirmed()`, i.e. `r.Ack` is `AckTimeout` or `AckFailed`) **and** `r.Result.Outcome != OutcomePermissionRequired`. But `AckTimeout` (per `burst.go`) means "the window **opened** but its token never appeared before the per-window timeout" — i.e. the adapter returned `OutcomeSuccess`. So a burst window that osascript opened cleanly, but whose token ack simply didn't arrive in time, satisfies the WARN condition and would emit `external window failed` at WARN with `detail` = the *success* detail (`"ghostty osascript exit 0"` / trimmed clean output).

This mismatches the fix's stated purpose. The Problem paragraph frames the WARN as surfacing "the per-window `detail` (the osascript error text — the actual diagnosis)". For an `AckTimeout`-after-`OutcomeSuccess` window there is no osascript error text — the diagnosis is an ack-channel timeout, not an open failure, and `detail` carries a benign success string. An implementer following the literal rule will emit a "failed" WARN with a non-error detail; an implementer following the stated intent might restrict the WARN to `OutcomeSpawnFailed`. The two readings diverge, so the boundary needs to be pinned.

**Proposed Addition**:
State explicitly whether `AckTimeout` windows whose adapter `Outcome` is `OutcomeSuccess` are intended to WARN (and accept that `detail` will be the success string), or whether the WARN is restricted to adapter open-failures (`OutcomeSpawnFailed`) — and adjust the condition wording and the Rider #1 test matrix to match. If ack-timeout is intended to WARN, add a sentence clarifying that `detail` for that sub-case is the ack outcome, not an osascript error.

**Resolution**: Approved
**Notes**: Resolved by keeping the WARN across BOTH non-permission failure modes (`AckFailed` open-failure and `AckTimeout`-after-`OutcomeSuccess`), keyed by the `ack` attr — restricting to open-failures would re-introduce the invisibility gap. Added a clarifying paragraph to Fix 2 and pinned the `AckTimeout`-after-Success case in the Rider #1 test matrix.

---

### 3. Live-validation ownership — "(next topic)" vs. an in-scope deliverable of this spec

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Scope & Non-Goals" (Release posture); "Testing & Validation Requirements" (Mandatory live validation)

**Details**:
The mandatory live validation is described two ways that a planner cannot reconcile from this document alone:
- In "Testing & Validation Requirements" it is spelled out **inline** as a hard requirement of this work ("the fix is **not 'done'** until … `TestManual_…` passes … A real ≥3-session picker burst confirms `opened 3/3` …"), i.e. it reads as a deliverable/acceptance gate of *this* spec.
- In "Scope & Non-Goals" and "Release posture" it is parenthesised as "**(next topic)**", implying a *separate* topic/spec owns it.

For planning readiness this matters: does the plan for this topic create tasks for the live-validation gate (and treat "opened 3/3 on a live Mac" as an acceptance criterion here), or is that owned by a downstream topic and merely referenced? The "(next topic)" phrase is also a dangling forward reference — a standalone reader has no way to resolve what that next topic is. Note that the automated tests (Fix 4 compile-check, Rider #1/#2 tests) *are* clearly in-scope; only the live-validation gate's ownership is ambiguous.

**Proposed Addition**:
Clarify whether the live-validation gate is a deliverable of this topic (planned + tracked here) or owned by a separate topic. If separate, either name it or drop the inline "the fix is not done until…" framing so the two sections agree on where the gate lives; if in-scope here, remove the "(next topic)" parenthetical.

**Resolution**: Approved
**Notes**: Clarified the live-validation gate is an in-scope acceptance gate owned by THIS topic; removed the dangling "(next topic)" parenthetical from the Release posture line and added a paragraph stating the two checks are acceptance criteria of this work, spelled out under Testing & Validation Requirements.

---

### 4. Fix 3 — new `PartialFailureMessage` signature is not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Fix 3 (Rider #2) — Honest total-failure banner copy"

**Details**:
The spec is exhaustively precise about the output copy (exact suffix strings, the condition table) but never states the modified signature of the single renderer. Today it is `PartialFailureMessage(failed []string) string`; the fix says the renderer "gains a signal for 'did any other window open'" and both callers "pass `len(confirmed) > 0`", which implies a new parameter (presumably `PartialFailureMessage(failed []string, othersOpened bool)`), but the name/type/order is left to the implementer. Given the rest of the spec pins exact identifiers and strings, the signature is the one interface detail left implicit. Low risk (inferable), but it is a small design decision the implementer must make and that the parity tests depend on.

**Proposed Addition**:
State the intended signature (e.g. `PartialFailureMessage(failed []string, othersOpened bool) string`) so callers and the parity tests key off a fixed shape.

**Resolution**: Approved
**Notes**: Pinned the intended signature `PartialFailureMessage(failed []string, othersOpened bool) string` in Fix 3's Change paragraph.

---

### 5. Fix 3 — total-failure copy row lacks the single-name annotation the partial row carries

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Fix 3 (Rider #2)" copy table; "Testing & Validation Requirements" (Rider #2 parity bullet)

**Details**:
In the copy table, the `othersOpened == true` row is annotated "(unchanged; single and multiple names)", but the `othersOpened == false` (total-failure) row shows only a two-name example (`'s2', 's3' failed to open — nothing opened`) with no equivalent annotation. A total failure can legitimately involve exactly one failed external window (an N=2 burst: one external + the trigger), producing `'s2' failed to open — nothing opened`. The surrounding prose ("no count-aware verb: 'failed to open' agrees with one or several names") does cover it, so this is a clarity/symmetry gap rather than a behavioural one — but the Rider #2 test matrix only says "Total failure (`othersOpened == false`)" without pinning the name count, leaving the single-name total-failure case implicit for the test author.

**Proposed Addition**:
Annotate the total-failure row "(single and multiple names)" to match the partial row, and have the Rider #2 test matrix name both a single-name and multi-name total-failure case explicitly.

**Resolution**: Pending
**Notes**:

---
