---
status: in-progress
created: 2026-07-11
cycle: 1
phase: Gap Analysis
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Gap Analysis

## Findings

### 1. Contradiction: ack-failure "abort + roll back" vs. the settled leave-what-opened stance

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Critical
**Affects**: *Burst & Partial-Failure Contract* — "Confirmation mechanism: explicit token ack" (line 162) and "Cleanup" (line 170); cross-references *Testing Strategy* (line 412) and *Spawn Architecture* line 71

**Details**:
The settled failure contract (Stance section, lines 142–155, and Cancellation, line 178) is explicit and internally consistent: post-pre-flight, any per-window spawn failure is handled by *leave-what-opened* — Portal does **not** close or undo opened windows, it skips the trigger self-attach, and shows a one-line error. "Portal can't cleanly perform" a teardown is stated as the reason.

But the ack-mechanism paragraph still carries pre-pivot language: line 162 reads "A missing token at timeout = a failed spawn → **abort + roll back**." Line 170 says the picker self-cleans batch markers "on **abort/rollback**," and line 412 lists "abort/**rollback** logic" as a test target. Line 71 also references "all-or-nothing **rollback**." An implementer reading line 162 would build a window-teardown path; an implementer reading lines 153–155 would build leave-what-opened. These directly conflict on the core failure behaviour.

The word "rollback" is doing two incompatible jobs: (a) marker self-cleanup (which *does* happen on partial failure), and (b) window teardown (which the Stance explicitly forbids). This must be disambiguated so a planner does not build a teardown path the spec elsewhere says is impossible/unwanted.

**Proposed Addition**:
Reconcile line 162 to the leave-what-opened contract, e.g.: "A missing token at timeout = a failed spawn → **leave-what-opened** (per *Stance*): the windows that opened stay in place, the trigger window's self-attach is skipped, the picker self-cleans its batch markers, and a one-line error names the failed window. No window teardown is attempted." Then replace "rollback" at lines 71, 170, and 412 with the disambiguated meaning — "marker self-cleanup" where marker cleanup is meant, and drop "all-or-nothing rollback" (line 71) since all-or-nothing lives only at the pre-flight gate.

**Resolution**: Pending
**Notes**:

---

### 2. Unsupported / NULL-terminal outcome of the multi-select Enter flow is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: *Spawn Architecture* "Order is load-bearing" (step 1 Detect, lines 66–71); *Multi-Select Mode* "N=0 / N=1 boundary" (lines 98–100); *Terminal Identity & Detection* (lines 221, 249); *Design References* frame 3 (line 442)

**Details**:
Detection is step 1 of the load-bearing spawn order, and the spec repeatedly states a NULL/unsupported identity → "honest no-op → clean error/banner." But the flow's actual behaviour when the user has marked sessions and presses `Enter` on an unsupported terminal is never defined. Three concrete branches are left to the implementer to guess:

1. **When does detection run and the banner appear?** Detection is described as a "separately-callable operation" backing the banner, but nothing says whether it runs eagerly on Sessions-page entry (so the banner shows proactively — as design frame 3 implies, "over the normal Sessions list") or only at Enter. This determines whether the user even knows the terminal is unsupported before marking.
2. **Is `m` / multi-select available on an unsupported terminal?** Not stated. If detection is unsupported, is entering the mode blocked, or allowed?
3. **What does Enter do on unsupported for N=1 vs N≥2?** N=1 degenerates to a plain self-attach (`AttachConnector`/`SwitchConnector`) which needs **no adapter**, so it can logically still work on an unsupported terminal — but the "detect first → no-op" ordering doesn't carve this out. N≥2 needs the adapter for the N−1 external windows, which is unavailable → so what happens: atomic no-op with the unsupported banner (nothing opens), or self-attach-to-one-and-report-the-rest? Undefined.

This is the one edge branch of the primary flow with no described outcome; a planner cannot write the Enter handler's unsupported path without inventing the policy.

**Proposed Addition**:
Add a short subsection (e.g. under *Multi-Select Mode* or *Terminal Identity*) pinning: (a) detection runs on Sessions-page entry so the unsupported banner surfaces proactively; (b) `m`/multi-select remains available even when unsupported (so single-attach still works); (c) on `Enter` with **N=1**, self-attach proceeds regardless of detection (no adapter needed); (d) on `Enter` with **N≥2** on an unsupported/NULL terminal, abort atomically — nothing opens — and (re)assert the unsupported banner naming the detected identity (consistent with "honest no-op"). Confirm the N=1-still-works vs. N≥2-blocked asymmetry is the intended behaviour.

**Resolution**: Pending
**Notes**:

---

### 3. Per-window ack timeout budget has no defined value

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: *Burst & Partial-Failure Contract* — "Confirmation mechanism" (line 162) and "Timeout is per-window, not global" (line 166)

**Details**:
The ack mechanism hinges on a per-window timeout: "the picker watches for the token set with a **timeout**," and "each window's ack timer starts when *its* spawn fires." But the timeout's **value** (or a rule to derive it) is never given. This is behaviourally load-bearing: too short → spurious "failed spawn" → self-attach skipped and a false error even though the window is coming up; too long → the user waits on a genuinely-failed window. The window must accommodate osascript-open + host-window launch + the spawned command's own abridged bootstrap/attach before it writes its token. The spec provides osascript-open timing (~260ms/window) but no attach-side headroom figure and no resulting budget. Given the spec pins other timing constants precisely (1.2s LoadingMinDuration, ~50ms appearance gate, 3-tick self-supervision hysteresis with a documented derivation), the missing ack timeout stands out as a gap a planner would otherwise guess.

**Proposed Addition**:
Specify a concrete per-window ack timeout as a named constant (e.g. `spawnAckTimeout`), with a value and a one-line derivation like the hysteresis constant (measured osascript-open ~260ms + abridged-attach headroom + a safety factor; suggest an initial ~8–10s, tunable). State that each window's timer starts when its own spawn fires (already stated) and that expiry classifies that window as a failed spawn (feeding the leave-what-opened path per Finding 1).

**Resolution**: Pending
**Notes**:

---

### 4. `terminals.json` validation / malformed-config behaviour unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: *Config Schema (`terminals.json`)* — "Recipe: explicit fields" (lines 298–304) and "Recipe execution contract" (lines 322–328)

**Details**:
`terminals.json` is a shipped, user-authored escape hatch that drives command *execution*, so malformed input is expected and its handling is behaviourally significant — yet the spec defines only the happy-path schema. Undefined cases a planner must resolve:

- **Malformed JSON** for the whole file — hard error, or tolerant-ignore-and-fall-through-to-native (consistent with Portal's other tolerant-decode stores: prefs/projects)?
- **An entry with neither `argv` nor `script`, or with both** — the recipe is "argv **or** script"; what happens when a user supplies neither or both? Skip the entry? Precedence?
- **Unknown / future capability sub-keys** (`introspect`/`place`) present today — the spec says they're "additive," implying they must be ignored gracefully now; confirm.
- **A recipe whose template omits `{command}`** — silently run a window with no attach, or reject?

Because a silently-dropped bad entry degrades to "unsupported" (a no-op the user may not connect to their typo), the resolution should also say whether/how these surface (the `spawn` log component already exists for breadcrumbs).

**Proposed Addition**:
Add a "Validation & error handling" paragraph to *Config Schema*: tolerant-decode the file consistent with Portal's other JSON stores (unreadable/malformed JSON → whole file ignored, fall through to native → unsupported, WARN under the `spawn` component); per-entry require **exactly one** of `argv`/`script` (neither or both → skip that entry with a WARN, fall through); ignore unknown capability sub-keys (forward-compat); and state the `{command}`-placeholder expectation for a valid recipe. Confirm these emit `spawn`-component breadcrumbs so a user's config typo is diagnosable.

**Resolution**: Pending
**Notes**:

---

### 5. Selection state after a post-pre-flight partial failure is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: *Burst & Partial-Failure Contract* — "Stance" (line 153)

**Details**:
On a rare post-pre-flight per-window failure, Portal "skips the trigger window's self-attach so you stay in the picker … Re-select if you want to retry the missing one." What happens to the existing marks is left unstated: are **all** marks cleared (so "re-select" means start over), or are only the **confirmed** sessions unmarked leaving the failed ones still marked (so a second `Enter` retries exactly the missing set)? By contrast, the pre-flight-abort path is explicit ("stay put … with the remaining selections intact"). The consequence is small (re-opening an already-attached session is a harmless no-op per the no-dup-guard rule), but the retry ergonomics differ and an implementer will pick one blindly.

**Proposed Addition**:
State the post-partial-failure selection state, e.g.: "Unmark the confirmed sessions (their windows are now open) and keep the failed/un-acked sessions marked, so a second `Enter` retries exactly the missing set." (Alternatively: clear all marks and require full re-selection — pick one; keep-failed-marked is the smoother retry and mirrors the pre-flight 'selections intact' behaviour.)

**Resolution**: Pending
**Notes**:

---
