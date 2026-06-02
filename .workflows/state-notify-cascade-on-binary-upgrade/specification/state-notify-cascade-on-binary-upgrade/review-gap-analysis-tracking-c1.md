---
status: in-progress
created: 2026-06-02
cycle: 1
phase: Gap Analysis
topic: state-notify-cascade-on-binary-upgrade
---

# Review Tracking: state-notify-cascade-on-binary-upgrade - Gap Analysis

## Findings

### 1. Convergence path's logging behavior is undefined (Acceptance Criterion 7 references an undefined "eviction log line")

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Registration Redesign — Ensure Exactly One" (§ Per-event convergence algorithm), "Migration-Helper Consolidation", Acceptance Criteria #7, Testing Requirements #5

**Details**:
The spec deletes `migrateHydrationHooks` and `migrateSessionClosedHook` and folds them into the unified ensure-exactly-one path, but never specifies what the new convergence path *logs*. This is a real gap because:

- The deleted `migrateHydrationHooks` emits a specific INFO line on eviction: `"evicted stale signal-hydrate hooks lacking '--' separator"` with the `reaped` cycle-summary attr. Per-index unset failures emit `"failed to evict stale signal-hydrate hook"` WARNs.
- `migrateSessionClosedHook` emits `"failed to evict stale notify hook"` WARNs on per-index failure.
- Acceptance Criterion 7 ("Idempotent and churn-free") explicitly asserts "no eviction log line" on an already-converged table — which *presupposes* there IS an eviction log line on the convergence (non-idempotent) path. But the spec never defines that line: its message text, level, the attr key (is `reaped` reused? a new key?), the component, and whether it is emitted per-event or once-per-bootstrap-cycle.
- Testing Requirement 5 asserts "no eviction log line" as a no-churn signal, so a test author must know the exact line that is being asserted *absent*.

CLAUDE.md states the log vocabulary is a closed taxonomy ("New components/attrs require amending the spec — never invent at call-site"). An implementer cannot invent the convergence log line; the spec must define it (or explicitly state the convergence path is silent and AC7/Testing-5 must be reworded to a non-log signal such as hook-index stability).

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 2. The `#{session_name}` token in the hydration desired body is omitted from the spec's body description

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Registration Redesign → Per-event parameters table (row: hydration events); § "Hook body shapes" paragraph

**Details**:
The actual production `signalHydrateCommand` is:

`run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate -- #{session_name}"`

The trailing `#{session_name}` tmux format token (expanded by tmux at fire time) is load-bearing — it's how `signal-hydrate` learns which session to act on, and the leading `--` separator exists precisely to protect session names beginning with `-`. The spec describes the hydration desired body only as "`signalHydrateCommand` (the `--`-separated form)" (table) and "`signal-hydrate` follow the same wrapper shape" (§ Hook body shapes), never showing or mentioning the `#{session_name}` token. An implementer reconstructing the desired body from the spec alone could drop the token, breaking hydration on every client-attach. The spec's notify/commit-now body examples are shown verbatim but the hydration body — the one with the non-obvious format token — is not.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 3. Fast-path full-body equality vs. the unexpanded `#{session_name}` literal is not addressed

**Priority**: Important
**Source**: Specification analysis
**Affects**: § Per-event convergence algorithm step 3 ("idempotent fast path"); § Hook body shapes (the "compares the full wrapped body" claim)

**Category**: Gap/Ambiguity

**Details**:
Step 3 says the idempotent fast-path "compares the **full wrapped body**, not the bare subcommand." For the hydration events the desired body contains a literal, unexpanded `#{session_name}`. The fast-path therefore compares the stored hook body (as returned by `ShowGlobalHooksForEvent` → `ParseShowHooks`, which retains the `run-shell "..."` wrapper but strips tmux's outer `%q` quoting) against the desired-body constant *which still contains the literal `#{session_name}` token*.

This works only if the assumption "tmux stores the hook body verbatim with the unexpanded token" holds (it does today — `#{...}` is expanded at fire time, not store time). The spec asserts the equality comparison "compares the full wrapped body" but never states the load-bearing assumption that the stored body equals the literal desired constant *including* the unexpanded token. Without this stated, an implementer may worry the stored body has the token expanded (it doesn't) and add unnecessary normalization, OR may not realize the comparison is a plain string equality against a constant containing `#{session_name}`. The spec should make the comparison rule explicit: equality is byte-for-byte against the desired-body constant (token unexpanded), no expansion/normalization performed.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 4. `ShowGlobalHooksForEvent` failure behavior per-event is underspecified

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Concrete mechanism (the new seam); § Per-event convergence algorithm; § Teardown Rewrite

**Details**:
The fix replaces a *single* global `show-hooks -g` read with a *per-event* read inside a loop over all managed events (registration) and `portalEvents` (teardown). The old code did one read; a failure aborted the whole operation with `show-hooks failed: %w`. The new code does N reads. The spec does not define what happens when `ShowGlobalHooksForEvent(event)` fails for *one* event mid-loop:

- Does that event's convergence get skipped while the loop continues to the remaining events (best-effort, error folded into `errors.Join`), matching the existing per-event best-effort register pattern?
- Or does the first read failure abort the entire registration (matching today's single-read abort semantics)?
- Same question for teardown: today a single read failure means "remove nothing"; per-event, a failure on event K leaves events 1..K-1 already torn down.

The existing register path folds per-event failures into `errors.Join` and never short-circuits ("every event is attempted") — the spec should state whether the new per-event read failures follow that same fold-and-continue contract, including the error-wrap string and whether the WARN (`show-hooks failed` / `error_class=unexpected`) is emitted per-failed-event.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 5. Cross-category eviction-fingerprint collision on `session-closed` is not analyzed

**Priority**: Important
**Source**: Specification analysis
**Affects**: § Per-event parameters table (`session-closed` row); § Per-event convergence algorithm step 2/4

**Category**: Gap/Ambiguity

**Details**:
The `session-closed` row lists two eviction fingerprints: `portal state notify` AND `portal state commit-now`. Note that `portal state commit-now` does **not** contain `portal state notify` as a substring, but the desired body `commitNowCommand` does NOT match the `portal state notify` fingerprint — good. However, consider convergence step 2 ("collect Portal-authored entries — those whose body contains *any* of the event's eviction fingerprints") combined with step 4 ("unset every Portal-authored entry, then append one desired body").

The algorithm collects entries matching *either* fingerprint and evicts *all* of them, then appends one `commitNowCommand`. This is correct for the stated goal. But the spec's step-3 idempotent fast-path says "if exactly one Portal-authored entry exists and its body already equals the desired body, do nothing." On an already-converged `session-closed` (one `commitNowCommand` entry), the entry matches the `portal state commit-now` fingerprint, count == 1, body == desired → fast-path fires correctly. This works, but the spec never walks the two-fingerprint event through the fast-path to confirm the "exactly one Portal-authored entry" count is computed across the *union* of both fingerprints (not per-fingerprint). An implementer could reasonably read "exactly one Portal-authored entry" as ambiguous when two fingerprints are in play. Make explicit that "Portal-authored entry count" for the fast path is the count of entries matching the union of the event's fingerprints.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 6. "Migrate-rename not reaped by registration" relies on an unstated guarantee that no managed event carries it

**Priority**: Minor
**Source**: Specification analysis
**Affects**: § Registration Redesign → "Notes on the table" (3rd bullet); § "What is intentionally not consolidated"

**Category**: Gap/Ambiguity

**Details**:
The spec states a legacy `portal state migrate-rename` on `session-renamed` is "*not* reaped by registration — it remains the responsibility of the teardown/clean path." This is correct only because `session-renamed`'s eviction fingerprint is `portal state notify`, and `portal state migrate-rename` does not contain that substring. The spec asserts the outcome but does not state the underlying invariant that makes it safe: *no managed event's eviction fingerprint is a substring of `portal state migrate-rename`, and migrate-rename is not a substring of any desired body*. Since the teardown path's `portalCommandSubstrings` still includes `portal state migrate-rename` while the registration fingerprint table deliberately omits it, an implementer must understand these are two distinct predicate sets. Worth a one-line statement that the registration eviction-fingerprint set and the teardown `portalCommandSubstrings` set are intentionally different (registration omits migrate-rename) and why that divergence is safe — otherwise an implementer "unifying the predicates" (which the spec elsewhere encourages for notify) might wrongly add migrate-rename to the registration table.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 7. Acceptance Criterion 6 ("no production caller remains") does not address test-fixture callers of `ShowGlobalHooks`

**Priority**: Minor
**Source**: Specification analysis
**Affects**: § Concrete mechanism ("Delete ShowGlobalHooks"); § Teardown Rewrite (last paragraph); Acceptance Criteria #6

**Category**: Gap/Ambiguity

**Details**:
AC6 and the Concrete-mechanism section say `ShowGlobalHooks` (the no-arg seam) is deleted and "no production caller remains." The `Client.ShowGlobalHooks` method on the tmux client is the public seam being removed. The spec is clear on production callers, but does not state what happens to the method itself on `*Client` (delete the method, or keep it for tests?) and is silent on whether the new `ShowGlobalHooksForEvent` must mirror the existing method's error-wrap shape (`failed to show global hooks: %w`) and verbatim-output (no-trim) contract. Since the spec says output is "byte-identical" and `ParseShowHooks` needs "zero changes," the new seam must preserve the verbatim/no-trim behavior of the old method — worth stating explicitly so an implementer doesn't introduce trimming. Minor because the byte-identical claim strongly implies it, but the method-level contract (error wrap, no-trim) is load-bearing for the parser and is currently only implied.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 8. Convergence ordering across categories/events for the unified path is not restated

**Priority**: Minor
**Source**: Specification analysis
**Affects**: § Registration Redesign; § Per-event parameters table

**Category**: Gap/Ambiguity

**Details**:
The existing `RegisterPortalHooks` documents a load-bearing processing order (save-trigger category before hydration-trigger; events in declaration order). The spec's per-event parameter table groups events differently (six notify events in one row, session-closed separate, hydration separate) and never states whether the unified convergence loop preserves a specific event-processing order, or whether order is now irrelevant because each event converges independently. Given that the per-event convergence is independent and self-contained, order likely no longer matters — but the spec should say so explicitly (order is no longer significant) since the prior code's doc comments emphasize order is significant, and an implementer porting the loop may preserve or discard ordering arbitrarily. If any test asserts a particular sequence (e.g. log-line ordering), this becomes load-bearing.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---
