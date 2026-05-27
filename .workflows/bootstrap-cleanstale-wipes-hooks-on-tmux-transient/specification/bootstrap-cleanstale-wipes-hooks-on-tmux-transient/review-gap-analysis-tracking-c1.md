---
status: in-progress
created: 2026-05-27
cycle: 1
phase: Gap Analysis
topic: bootstrap-cleanstale-wipes-hooks-on-tmux-transient
---

# Review Tracking: bootstrap-cleanstale-wipes-hooks-on-tmux-transient - Gap Analysis

## Findings

### 1. Promoted-parser destination left as "likely"

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 2 — Promote `parseLivePaneSet` to a Shared Utility

**Details**:
Change 2 specifies the parser must be promoted to a shared location but qualifies the destination as "likely `internal/tmux` or a new helper next to it." A planning agent breaking this into tasks needs a concrete target package/file to write the move task. The choice has downstream consequences for the test-coverage move in Change 2 ("Promoted Parser Coverage") and for the import paths in three consumers (`ListAllPanes`, `CleanStaleMarkers`, both `CleanStale` callsites). Leaving the location open forces the implementer to make a design decision.

Sub-question that needs resolving: does the promoted parser live in `internal/tmux` (alongside the helper that now consumes it), a new leaf like `internal/tmuxparse` (analogous to `tmuxout`/`tmuxerr`), or stay in `cmd/bootstrap` with an exported name? Each has implications for import graph and test placement.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 2. Logger parameter in repurposed `ListAllPanes` left as `nil` placeholder

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 1 (code sketch)

**Details**:
The example body for the repurposed `ListAllPanes` shows `parseLivePaneSet(raw, /* logger */ nil)`. The existing `parseLivePaneSet` in `cmd/bootstrap/stale_marker_cleanup.go` apparently takes a logger to record malformed-line skips. Passing `nil` from `(*tmux.Client).ListAllPanes` raises two questions a planner needs answered:

- Does the promoted parser need to accept a nil-safe logger, or should its signature change to drop the logger?
- If kept, where does `ListAllPanes` (which lives in `internal/tmux`) source a logger? The tmux client does not currently hold one — adding it is a non-trivial wiring change touching every construction site.

If the answer is "nil-safe and silently skip malformed lines from `ListAllPanes`," that needs to be stated so the planner does not bikeshed the seam.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 3. Format string alignment between sketch and existing parser not asserted

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 1 and Change 2

**Details**:
The Change 1 sketch passes `"#{session_name}:#{window_index}.#{pane_index}"` to `ListAllPanesWithFormat`. The existing `parseLivePaneSet` in `cmd/bootstrap/stale_marker_cleanup.go` is presumed to parse the same format, but the spec never asserts the format matches (or that the existing parser tolerates the chosen format). If the existing parser was written against a different format string (e.g. one that the step-9 caller currently uses), the repurposed `ListAllPanes` will silently produce empty sets.

This is a one-line verification the planner needs done (or asserted in spec) before writing the implementation task.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 4. `persistedHooks` count source not specified at the adapter callsites

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 3 (Mass-Deletion Hazard Guard)

**Details**:
The hazard guard condition is `len(livePanes) == 0 && len(persistedHooks) > 0`. To evaluate `len(persistedHooks)` the caller must read the hooks store *before* calling `hooks.Store.CleanStale`. The spec does not state which API surface on the hooks store the adapter/clean.go uses to obtain the count:

- An existing `Load`/`List`/`Count` method?
- A new method introduced by this work unit?
- Inspecting `hooks.json` directly?

This determines whether Change 3 is a pure-callsite change or whether it adds a method to `internal/hooks/store.go`. The prior-art at `stale_marker_cleanup.go:126-141` has a different shape (markers are read via tmux server options), so the lift is not as verbatim as the spec implies — the hooks side needs an explicit count source.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 5. `cleanStaleAdapter.CleanStale` current signature not described

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 3, Test Requirements → New file

**Details**:
The spec references the adapter at `cmd/bootstrap_production.go:76-83` and its docstring at `:71-75`, but never reproduces the current method signature, the seam interface it implements, or what dependencies it currently receives (e.g. `LivePaneLister`, `HooksStore`). A planning agent will need to either re-derive this from source or be told what the seam looks like to write the "Adapter docstring rewrite" + "Logger plumbing" task accurately.

In particular: does the adapter consume a `LivePaneLister` interface (with a `ListAllPanes() ([]string, error)` method), or does it call `tmux.Client.ListAllPanes` directly? The fix changes the underlying helper to call `ListAllPanesWithFormat`, but if the seam is `LivePaneLister`, the interface signature may need to change too (or stay the same with a new internal implementation). Test stubs in `cmd/bootstrap_production_test.go` depend on this.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 6. `portal clean` logger availability not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 3 (Logger plumbing) and Change 4 (Adapter-Level Logging)

**Details**:
Change 3 spells out logger plumbing for `cleanStaleAdapter` (gain a `Logger` field, populate from orchestrator-scope logger at lines 109-110). Change 4 then requires `Debug`/`Warn` emission at **both** `CleanStale` callsites — including `cmd/clean.go:75-91`. But `portal clean` runs outside the bootstrap orchestrator and the spec does not state whether `cmd/clean.go` has access to a logger today or how it should acquire one.

Specifically:
- Does `cmd/clean.go` already construct/receive a `Logger`, or is one introduced by this work unit?
- Is the log destination the same `portal.log` used by bootstrap, or does `portal clean` log to stderr/stdout per its CLI ergonomics?
- If `portal.log` for `portal clean`, does writing to that file require state-dir resolution that may not already happen in `cmd/clean.go`?

Without this resolved, the planner must invent the wiring.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 7. Soft-warning surfacing path for `ListAllPanesWithFormat` errors underspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 3 ("Adapter docstring rewrite"), Acceptance Criteria #2, Test Requirements → "Error from `ListAllPanesWithFormat` propagates as soft warning"

**Details**:
The spec states the adapter "surfaces propagated errors as soft warnings" and that "the orchestrator surfaces it as a soft warning." But the mechanical contract by which a non-nil error returned from step 11 becomes a `Warning` in the orchestrator's warnings slice is not described. Two plausible interpretations:

- (i) The orchestrator already converts step-11 errors into warnings under the "never abort `PersistentPreRunE`" rule; the adapter simply returns the error.
- (ii) The adapter itself must convert the error into a `warning.Warning` value and return it via a separate channel (e.g. via the orchestrator-scope warnings slice).

These have different implementation shapes. The current bootstrap orchestrator step contract for warnings vs errors needs stating, or a pointer to the existing precedent (e.g. how `SaverDownWarning` or step-9 errors surface today) needs to be in-spec so the planner does not have to spelunk.

The test requirement "orchestrator (or test harness) wraps as a soft warning" further hedges this — the planner needs to know which one to test.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 8. Return-type narrowing of `ListAllPanes` (nil vs empty slice) may affect callers

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 1

**Details**:
The pre-fix `ListAllPanes` returns `([]string{}, nil)` on error — a non-nil, empty slice. The post-fix sketch returns `nil, err` on error. The spec asserts "every existing call site compiling unchanged" but does not address runtime behaviour for any caller that:

- Iterates the returned slice (safe — `range nil` is a no-op).
- Checks `len(slice) == 0` to decide a branch (safe — both produce 0).
- Distinguishes `nil` from non-nil slices (unsafe — semantic shift).
- Treats `err == nil` as "result is authoritative" (unsafe — previously always true, now sometimes false).

The audit at "Defect Class Scope" bounds production consumers of `ListAllPanes` to two sites, both of which this work unit modifies. But test code and any indirect consumers (e.g. via stub `LivePaneLister`s that return `ListAllPanes` output) may exhibit the contract shift. A planner needs explicit confirmation that no caller depends on the never-error, always-non-nil contract — or that the audit covers tests too.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 9. Hazard-guard return value's interaction with "Debug on completion" not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 4 (Adapter-Level Logging), Acceptance Criteria #4

**Details**:
Change 4 lists four log emissions: Debug on entry, Warn on hazard fire, Debug on normal-path completion, Warn on propagated error. Acceptance Criterion #4 says every invocation emits "Debug on entry" and "either Debug on completion or Warn on hazard-guard skip / propagated error."

Unclear: when the hazard guard fires, is the Debug-on-completion line *also* emitted (with removed=0), or is the Warn the *only* terminal log line for that path? The spec's "either/or" phrasing in AC#4 implies mutual exclusivity, but the Change 4 emission list is additive. A planner will pick one; assertions in `cmd/bootstrap_production_test.go` will codify it.

This is small but matters for the "Hazard guard fires on empty live set" subtest — what exact log records does it assert?

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 10. "Same inversion shape used by Phase-4 subtests" reference is unanchored

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Requirements → Inverted Subtest

**Details**:
The subtest-inversion guidance ends with "Same inversion shape used by Phase-4 subtests in the earlier-shipped `hooks-skip-bootstrap` quickfix." No file path or PR/commit reference is given. A planner who has not lived through that quickfix has to grep history to find the shape. Either inline the shape (a 3-line skeleton would suffice) or cite the file/commit.

This is minor — the inversion instruction in the section above is concrete enough that an implementer can proceed without the reference — but the dangling pointer adds friction.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 11. `parseLivePaneSet` signature and whether it accepts a logger is implicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 2

**Details**:
The spec promotes `parseLivePaneSet` to a shared utility without stating its current signature (does it return `map[string]struct{}`? `map[string]bool`? `[]string`?), whether it takes a logger, and whether the signature is preserved verbatim in the move or whether the move is a chance to tighten the contract (e.g. drop the logger parameter, return a set type).

Combined with finding #2 (logger placeholder) and finding #3 (format string alignment), the planner is being asked to do an in-place move while three signature questions remain unanswered. A concrete target signature for the promoted helper would let the move be a single, mechanical task.

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---

### 12. Acceptance Criterion #4 wording leaves "Debug on entry" universality unclear

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria #4

**Details**:
AC #4 reads: *"every invocation of `cleanStaleAdapter.CleanStale` and the `portal clean` hook tail emits `Debug` on entry (with live and persisted counts)."* But at the entry point, `livePanes` is not yet known — the helper call hasn't happened. Either:

- The Debug-on-entry fires *before* the helper call and only logs `persisted count`, with `live count` logged separately after the helper returns, or
- The Debug-on-entry fires *after* the helper call (and before the guard / `Save`), at which point both counts are known but it is no longer strictly "on entry."

The Change 4 description ("Debug on entry — live count, persisted count, what would be removed") implies the latter, but calling it "on entry" is misleading. A planner will resolve this one way or the other; the spec should either clarify the position or rename the log point (e.g. "Debug after enumeration").

**Proposed Addition**:
*(to be filled during discussion)*

**Resolution**: Pending
**Notes**:

---
