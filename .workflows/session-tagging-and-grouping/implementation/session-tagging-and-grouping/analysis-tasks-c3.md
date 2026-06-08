---
topic: session-tagging-and-grouping
cycle: 3
total_proposed: 1
---
# Analysis Tasks: Session Tagging and Grouping (Cycle 3)

## Task 1: Gate lazy stamp-on-render resolution to grouped modes only
status: approved
severity: medium
sources: architecture

**Problem**: `rebuildSessionList` (internal/tui/model.go) calls `filtered := m.resolveSessionDirs(m.filteredSessions())` unconditionally at line 1 of the function, BEFORE the mode switch. Only the `ModeByProject` and `ModeByTag` grouping arms consume `Session.Dir`; the Flat (`default`) and `byTagSignpost` arms route through `ToListItems(filtered)`, which ignores `Dir` entirely. For any un-stamped session (every live session on first ship, all restored sessions post-reboot), `resolveSessionDirs` → `session.ResolveAndStampDir` falls into its lazy path and performs an `ActivePaneCurrentPath` pane read + a `ResolveGitRoot` `git rev-parse` subprocess + a best-effort `SetSessionOption` stamp write — per session, per render — and the resolved `Dir` is then discarded because Flat does not group. Flat is BOTH the no-regression default AND the first-ever-launch default, so this is the common path, and the cost re-incurs on every Flat render until the stamps happen to land. The spec frames the lazy fallback strictly as a grouped-render mechanism ("when the grouped render encounters a session with no @portal-dir"), so applying it in Flat mode is a chokepoint over-application.

**Solution**: Gate the resolution pass on a grouped mode so `resolveSessionDirs` only runs when grouping will actually consume `Dir`. Move the resolution into the two grouping arms of the switch (or guard it with the same `grouped := m.sessionListMode == prefs.ModeByProject || m.sessionListMode == prefs.ModeByTag` predicate already used at the `ProjectsLoadedMsg` re-group site). The Flat (`default`) and `byTagSignpost` arms consume `m.filteredSessions()` directly, restoring byte-for-byte today's Flat behaviour with zero pane reads. The seam is otherwise clean (value-copy, nil-guarded, best-effort) — this is purely a placement guard, no signature or semantics change.

**Outcome**: Flat mode and the byTagSignpost arm perform zero per-session pane reads / git rev-parse subprocesses / stamp writes during render (byte-for-byte today's pre-feature Flat behaviour). `resolveSessionDirs` fires only on `ModeByProject` / `ModeByTag` (non-signpost) renders, where the resolved `Dir` is consumed by `buildByProject` / `buildByTag`. The "bounded one-time amortisation" of N git-root derivations + N stamp writes is incurred only by the grouped modes that derive value from it, matching the spec's grouped-only contract.

**Do**:
1. In `internal/tui/model.go`, in `rebuildSessionList`, remove the unconditional `filtered := m.resolveSessionDirs(m.filteredSessions())` at the top of the function.
2. Replace the single `filtered` binding so the un-resolved filtered slice is the base: e.g. `filtered := m.filteredSessions()`. Keep the `m.byTagSignpost = ...` recompute exactly where it is.
3. In the mode switch, apply `m.resolveSessionDirs(...)` only inside the two grouping arms: `case m.sessionListMode == prefs.ModeByProject:` → `items = buildByProject(m.resolveSessionDirs(filtered), m.projectIndex)`, and `case m.sessionListMode == prefs.ModeByTag:` → `items = buildByTag(m.resolveSessionDirs(filtered), m.projectIndex)`. Leave the `byTagSignpost` and `default` arms consuming the un-resolved `filtered` via `ToListItems(filtered)` unchanged.
4. Confirm the `byTagSignpost` arm precedes the `ModeByTag` arm in the switch, so a zero-tags By-Tag render does NOT trigger resolution.
5. Update the `resolveSessionDirs` doc comment if it asserts it runs for all renders / before all grouping — it now runs only for the grouped arms.

**Acceptance Criteria**:
- `resolveSessionDirs` is not called on the Flat (`default`) render path nor on the `byTagSignpost` render path; both consume `m.filteredSessions()` output directly.
- `resolveSessionDirs` IS called on the `ModeByProject` and `ModeByTag` (non-signpost) render paths, feeding `buildByProject` / `buildByTag`.
- No behaviour change to grouping output: `ModeByProject` / `ModeByTag` renders produce identical items to before the change (same resolved `Dir` values reach the builders).
- Flat-mode rendered list is byte-for-byte identical to today's output.
- `m.sessions` is still never mutated (value-copy semantics in `resolveSessionDirs` preserved).

**Tests**:
- Add/extend a `rebuildSessionList` test asserting that with a `WithDirResolver` seam wired to a counting `dirRunner` / `dirStamper`, a Flat-mode rebuild over N un-stamped sessions performs ZERO pane reads / git resolutions / stamp writes (counter == 0).
- Assert that a `byTagSignpost` rebuild (ByTag mode, zero tags anywhere) also performs zero resolutions.
- Assert that a `ModeByProject` rebuild over the same N un-stamped sessions DOES invoke resolution (counter == N) and that the resulting grouped items match the pre-change grouped output (regression oracle).
- Assert a `ModeByTag` (with tags present) rebuild invokes resolution and groups identically to before.
- Confirm existing grouping / signpost / flatten-on-filter tests still pass unchanged.
