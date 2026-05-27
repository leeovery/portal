---
status: in-progress
created: 2026-05-27
cycle: 1
phase: Input Review
topic: bootstrap-cleanstale-wipes-hooks-on-tmux-transient
---

# Review Tracking: bootstrap-cleanstale-wipes-hooks-on-tmux-transient - Input Review

## Findings

### 1. Audit Conclusion — Blast Radius Bounded to Two Callsites

**Source**: investigation §"Blast Radius — Other `ListAllPanes` Callers" (lines 205-218); §"Audit Scope" (lines 92-94)
**Category**: Enhancement to existing topic
**Affects**: Fix Specification (intro paragraph or new sub-section); Out of Scope

**Details**:
The investigation devotes an explicit section to auditing every caller of `ListAllPanes` and `ListAllPanesWithFormat` and concludes "the defect class is bounded to the two `ListAllPanes` callers. Both must be fixed." It also names the `ListAllPanesWithFormat` consumers that are correct by construction: `cmd/bootstrap/stale_marker_cleanup.go:119` (step 9, the prior-art), `internal/state/capture.go:99` (daemon capture loop — per-tick skip), and `cmd/bootstrap/orphan_sweep.go` (Component B — log-and-skip).

The spec lists the two affected callsites but does not record the audit conclusion that bounds the defect class. Without this, a future implementer (or reviewer) cannot tell whether the spec authors *considered* other consumers or simply listed the two they happened to know about. Recording the audited universe is what makes "two callsites, no more" defensible.

**Current**:
> `(*tmux.Client).ListAllPanes` swallows every error class ... The peer helper `ListAllPanesWithFormat` (same file, lines 655-665) does the opposite — it propagates errors.
>
> Two callsites pass the (possibly empty) live-pane slice straight to `hooks.Store.CleanStale` ...
> - `cmd/bootstrap_production.go:76-83` — `cleanStaleAdapter.CleanStale` (bootstrap step 11).
> - `cmd/clean.go:75-91` — `portal clean` subcommand's hook-cleanup tail.

**Proposed Addition**:
{blank — to be drafted during discussion}

**Resolution**: Pending
**Notes**:

---

### 2. Trigger Windows of Highest Empirical Risk

**Source**: investigation §"Environment" / "Trigger windows of highest empirical risk" (lines 43-48)
**Category**: Enhancement to existing topic
**Affects**: Problem Statement — Defect / Failure Modes Covered (or a new "Trigger Windows" sub-section)

**Details**:
The investigation enumerates four high-risk windows: (i) saver-respawn after the zombie-sessions bugfix, (ii) version-upgrade kill cycle in `EnsurePortalSaverVersion`, (iii) tmux server under heavy load, (iv) the same race that produced the observed Component B WARN. The spec mentions saver-respawn in passing inside the root-cause narrative ("transient tmux failure during saver-respawn or under load") but does not enumerate the four windows, and `EnsurePortalSaverVersion` is not named at all.

These windows matter for both reviewers (sizing the fix's coverage) and implementers (where to seed the integration test). Capturing them explicitly anchors the fix's empirical scope.

**Current**:
> - **(a)** `list-panes -a` exit ≠ 0 (transient tmux failure during saver-respawn or under load) — evidenced by the observed Component B WARN.

**Proposed Addition**:
{blank — to be drafted during discussion}

**Resolution**: Pending
**Notes**:

---

### 3. Relationship to v0.5.11 `hooks-skip-bootstrap` Quickfix

**Source**: investigation §"Relationship to Recent Releases" (lines 96-98)
**Category**: New topic (small)
**Affects**: Notes (or Risk Assessment / Release cadence)

**Details**:
The investigation explicitly notes: "The fix in v0.5.11 (`hooks-skip-bootstrap`) reduces trigger frequency by eliminating the `SessionStart` cascade but does **not** change the latency of this bug. `portal open` / `x` / attach during a tmux transient can still wipe everything." The spec's Notes section discusses recent releases increasing exposure but doesn't mention v0.5.11 specifically or that the inversion-shape used by Phase-4 subtests in `hooks-skip-bootstrap` is the test-inversion pattern this fix lifts.

The spec already references the "Same inversion shape used by Phase-4 subtests in the earlier-shipped `hooks-skip-bootstrap` quickfix" in Test Requirements, so v0.5.11 is implicitly in scope; making the release relationship explicit clarifies that v0.5.11 was a *frequency* mitigation, not a *latency* fix — useful framing for release notes and for any reader trying to assess "didn't we already fix this?"

**Proposed Addition**:
{blank — to be drafted during discussion}

**Resolution**: Pending
**Notes**:

---

### 4. Adapter Docstring Update at `cleanStaleAdapter`

**Source**: investigation §"Code Trace" (lines 116-131, esp. line 131)
**Category**: Enhancement to existing topic
**Affects**: Fix Specification — Change 3 (or Change 4)

**Details**:
The investigation quotes the existing docstring on `cleanStaleAdapter.CleanStale` at `cmd/bootstrap_production.go:71-75`: *"A `ListAllPanes` failure degrades to no-op (returns nil) so a transient tmux error during bootstrap never aborts the user's command — matches the safety-net semantic in `portal clean`."*

Once the helper is repurposed to propagate errors and the adapter gains a hazard guard, this docstring is actively misleading — the adapter no longer degrades to a no-op on `ListAllPanes` failure; it surfaces the error as a soft warning and refuses the wipe on empty results. The spec lays out the behavioural change but does not flag that the docstring must be rewritten as part of Change 3.

**Current**:
> Before passing the live-pane slice to `hooks.Store.CleanStale`, check the combination `len(livePanes) == 0 && len(persistedHooks) > 0`. When that combination holds:
> 1. Emit `Logger.Warn(ComponentBootstrap, ...)` with both counts.
> 2. Skip the destructive call.
> 3. Return nil (next bootstrap retries).

**Proposed Addition**:
{blank — to be drafted during discussion}

**Resolution**: Pending
**Notes**:

---

### 5. Prior-Art Comment-Block Lift Alongside Code Lift

**Source**: investigation §"The Prior-Art Sibling — Why Step 9 Is Not Vulnerable" (lines 178-202, esp. lines 196-201)
**Category**: Enhancement to existing topic
**Affects**: Fix Specification — Change 3

**Details**:
Step 9's hazard guard at `cmd/bootstrap/stale_marker_cleanup.go:80-92` carries an explicit comment block that names the failure mode: *"Treating an empty live set as authoritative would destabilise a still-live tmux server by unsetting every marker — including markers protecting legitimate hydrate-in-progress panes. The deferral is a successful soft outcome ('skip this run; next bootstrap retries'), not a failure."*

The spec instructs Change 3 to "mirror the prior-art at `cmd/bootstrap/stale_marker_cleanup.go:126-141` exactly" but the prior-art comment block at lines 80-92 is the load-bearing prose that makes the guard self-documenting at its callsite. Without explicit guidance, an implementer may copy the code but skip the comment, leaving the next reader without the *why*.

**Current**:
> This mirrors the prior-art at `cmd/bootstrap/stale_marker_cleanup.go:126-141` exactly.

**Proposed Addition**:
{blank — to be drafted during discussion}

**Resolution**: Pending
**Notes**:

---

### 6. Deterministic Repro Mechanism — `Commander` Injection

**Source**: investigation §"Reproduction Steps" (lines 32-38)
**Category**: Enhancement to existing topic
**Affects**: Test Requirements — Integration sub-sections

**Details**:
The investigation states: "Should be deterministic in a unit test by injecting a `Commander` that returns `exit 1` or empty stdout for the `list-panes -a` call." The spec describes integration tests at a high level ("arrange for `list-panes -a` to return exit ≠ 0 (e.g., via a `Commander` stub at the integration boundary)") which is good — but does not separately name the unit-test variant where Commander injection makes the deterministic repro cheap and fast.

This matters because the unit-test path is the easiest one to require explicitly (no real tmux server, no subprocess). The hazard guard unit subtests already use stub `LivePaneLister`s, but the spec doesn't name `Commander`-level injection as the canonical mechanism for reproducing failure mode (a) deterministically.

**Current**:
> Spawn a real tmux server, populate `hooks.json`, kill `_portal-saver` mid-bootstrap, and arrange for `list-panes -a` to return exit ≠ 0 (e.g., via a `Commander` stub at the integration boundary). Assert `hooks.json` is unchanged at the end of the bootstrap.

**Proposed Addition**:
{blank — to be drafted during discussion}

**Resolution**: Pending
**Notes**:

---

### 7. Audited `ListAllPanesWithFormat` Consumers as Non-Defective Reference Set

**Source**: investigation §"Blast Radius — Other `ListAllPanes` Callers" (lines 212-216)
**Category**: New topic (small / informational)
**Affects**: Notes (or a new "Audit Findings" section under Fix Specification)

**Details**:
Companion to finding #1. The investigation lists the three `ListAllPanesWithFormat` consumers and how each correctly handles errors:
- `cmd/bootstrap/stale_marker_cleanup.go:119` — propagates + hazard guard (the prior-art).
- `internal/state/capture.go:99` — daemon capture loop; treats non-nil as a per-tick skip.
- `cmd/bootstrap/orphan_sweep.go` — Component B; logs and skips on non-nil.

These three are the "how the propagating helper *should* be consumed" reference set. The spec only names step 9 as the prior-art. Capturing all three demonstrates the pattern is already idiomatic across the codebase and grounds the architectural-consistency argument.

**Proposed Addition**:
{blank — to be drafted during discussion}

**Resolution**: Pending
**Notes**:

---

### 8. Historical Near-Miss — `saver-kill-respawn-loop-leaks-daemons` Investigation

**Source**: investigation §"Why It Wasn't Caught" (lines 245-246)
**Category**: New topic (small / historical)
**Affects**: Notes

**Details**:
The investigation records that the 2026-05-19 `saver-kill-respawn-loop-leaks-daemons` investigation "explicitly listed CleanStale as a candidate destructive force for disappearing `daemon.version`, but the line of inquiry didn't lead back to this hooks-wipe defect." This is a documented prior signal that this defect class was within the field-of-view of recent investigations but was not closed.

Capturing the near-miss in Notes anchors the post-mortem timing and prevents the same gap from being missed again — future analogous investigations can grep this work unit for the cross-reference.

**Proposed Addition**:
{blank — to be drafted during discussion}

**Resolution**: Pending
**Notes**:

---
