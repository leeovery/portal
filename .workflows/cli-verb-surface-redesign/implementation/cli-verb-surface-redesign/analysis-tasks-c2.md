---
topic: cli-verb-surface-redesign
cycle: 2
total_proposed: 3
---
# Analysis Tasks: CLI Verb Surface Redesign (Cycle 2)

## Task 1: Narrow `portal doctor`'s daemon-liveness probe off the over-scoped `CollectStatus` and share one pane counter
status: approved
severity: medium
sources: architecture, duplication

**Problem**: After `state status` was deleted, `state.CollectStatus` was retained wholesale as doctor's daemon-liveness probe, but `checkDaemonAlive` (cmd/doctor.go:492-509) reads only 3 of its ~10 fields (`DaemonRunning`/`DaemonPID`/`DaemonVersion`). Every `portal doctor` run still eagerly performs a full state-dir tree walk (`computeStateSize`), a full portal.log line-by-line scan (`scanRecentWarnings`), and a sessions.json parse (`collectIndexState`) — all discarded. The sessions/panes counts are computed twice per run: once inside `CollectStatus` (thrown away) and again by doctor's `checkSessionsJSON` → `state.ReadIndex` + `doctorPaneCount` (cmd/doctor.go:635-643), where `doctorPaneCount` is a byte-identical copy of the unexported `state.countPanes` (internal/state/status.go:127-135, the identical triple-nested pane loop). This bakes heavy redundant I/O into a command explicitly designed as a fast scriptable health gate (`portal doctor && …`) and leaves a mirrored pane counter that must stay in lockstep.

**Solution**: Give doctor a narrow daemon-liveness probe (daemon.pid read + `IsProcessAlive` + recorded version) in place of `CollectStatus`. Trim `CollectStatus` to just what is still consumed, or delete it if doctor is its only remaining production caller. Export `state.CountPanes(idx Index) int` (have the unexported `countPanes` delegate to or be renamed into it) and replace `doctorPaneCount` with a call to it. Source doctor's sessions/panes counts from its single existing `state.ReadIndex`.

**Outcome**: A routine `portal doctor` performs no state-dir tree walk and no full portal.log scan, reads the index once, and uses a single shared pane counter. Doctor's reported daemon / sessions / panes diagnostics are behaviourally identical to before.

**Do**:
1. In cmd/doctor.go, replace `checkDaemonAlive`'s call into `state.CollectStatus` with a narrow probe: read `daemon.pid`, check liveness via `IsProcessAlive`, read the recorded daemon version — surfacing the same `DaemonRunning`/`DaemonPID`/`DaemonVersion` facts it uses today.
2. Export `state.CountPanes(idx Index) int` in internal/state/status.go (rename `countPanes` or have it delegate to the exported form); delete `doctorPaneCount` from cmd/doctor.go and call `state.CountPanes` from `checkSessionsJSON`.
3. Trim `state.CollectStatus` down to the fields still consumed, removing the now-unconsumed `StateSize`/`RecentWarnings`/`LastWarning`/`LastSaveAt`/`HasLastSave`/`SessionsCount`/`PanesCount` machinery and their helpers (`computeStateSize` / `scanRecentWarnings` / `collectIndexState`) where nothing else consumes them; delete `CollectStatus` entirely if doctor was its last production caller.
4. Update or remove any tests that only exercised the removed `CollectStatus` fields.

**Acceptance Criteria**:
- `portal doctor` no longer walks the full state-dir tree or scans the whole portal.log on a routine run.
- Sessions and panes counts derive from a single index read per `portal doctor` invocation.
- Exactly one pane-counting function exists in the tree (no `doctorPaneCount` duplicate of `state.countPanes`).
- Doctor's daemon / sessions / panes output is behaviourally identical to the pre-change command.

**Tests**:
- Update existing doctor tests to assert the narrowed daemon probe still reports running / pid / version correctly against a seeded `daemon.pid`.
- A test that a healthy seeded state dir passes doctor's daemon + sessions checks via the narrowed probe.
- If `CollectStatus` is trimmed or deleted, update the state-package tests accordingly (remove assertions on dropped fields).

## Task 2: Remove the production-dead, divergent session-glob branch from `QueryResolver.Resolve`
status: approved
severity: medium
sources: architecture

**Problem**: `open` routing decides single-vs-burst via `isMultiTarget(orderedOpenTargets(openOwnArgs()))` (cmd/open_burst_run.go:43-70) — a raw `os.Args` re-scan that runs parallel to cobra's own parse and hinges on the assumption that the "open" token is present and no value-taking global flag precedes it. Because any single glob-bearing target is classified multi and diverted to the burst, `QueryResolver.Resolve`'s glob branch (internal/resolver/query.go:134-141) is unreachable in production. But it is not a harmless no-op: it independently re-implements session-glob expansion (duplicating `expandSessionGlobAll` / `ResolveBareAll`) and returns only `matches[0]` — the FIRST match — whereas the live burst path opens ALL matches. The moment the `os.Args` assumption breaks (a future persistent value-flag, an invocation shape that omits the "open" token, or the fallback returning nil), a single glob silently degrades from "burst every matching session" to "attach only the first" — a semantic fork with no error and no test signal.

**Solution**: Eliminate the silent divergence. Either remove the dead first-match glob branch so a single glob routes through the same `expandSessionGlobAll` the burst uses, or make that branch hard-fail (return an explicit error) instead of silently returning a first-match — so a routing miss can never quietly change glob semantics.

**Outcome**: There is no code path where a multi-match session glob resolves to only its first match. A glob reaching the resolver either fans out to all matches (consistent with the burst) or fails loudly.

**Do**:
1. Confirm `Resolve`'s glob branch (internal/resolver/query.go:134-141) and its callers (cmd/open.go:262, `ResolveBareAll` at internal/resolver/query.go:219) never receive glob input in the production routing path.
2. Remove the first-match glob branch from `Resolve`. If a glob must remain reachable there, route it through `expandSessionGlobAll` (all matches) rather than `matches[0]`, or return an explicit "glob not resolvable on this path" error instead of a silent first-match.
3. Ensure the single-target `open` path and the burst path share one glob-expansion primitive so their match sets cannot diverge.

**Acceptance Criteria**:
- No `Resolve` path returns `matches[0]` for a multi-match session glob.
- A glob reaching the resolver either expands to all matches or returns an explicit error — never silently first-match.
- Single-target and burst glob expansion go through the same expansion primitive.

**Tests**:
- A resolver test proving a multi-match session glob does not collapse to the first match (it either expands to all matches or returns an error).
- A regression test pinning that single-glob and burst-glob expansion agree on the match set.

## Task 3: Refresh stale post-redesign documentation and comments
status: approved
severity: low
sources: standards

**Problem**: Two documentation artefacts still describe CLI surfaces the redesign removed. (1) internal/log/process_role.go still comments that bare `portal` "is the TUI picker" (line 44 and lines 66-68, plus the routing-table "bare -> tui" note), but the redesign made bare `portal` print help/usage — it no longer launches the picker. (2) CLAUDE.md's "Incident of record #2" (line 114) cites "two cmd tests (`TestVersionGuard_NotInvokedForExemptCommands`, `TestStateUserFacingSubcommandsExitZero`) ran the real `portal state cleanup` body" — but `portal state cleanup` no longer exists (subsumed by `uninstall`) and `TestStateUserFacingSubcommandsExitZero` is absent from the tree. Both passages assert removed behaviour and will mislead a future reader; the underlying lesson in each remains valid.

**Solution**: Correct the stale comments and the incident note so they describe current behaviour, without changing any classification. Leave the process-role mapping for bare `portal` (`roleTUI`) and its pinned test case exactly as-is — reclassifying is churn against a closed, forensically-inert taxonomy — and fix only the comments. Re-anchor or explicitly period-mark the CLAUDE.md incident note.

**Outcome**: No comment or doc asserts a removed CLI surface as current behaviour. The process-role classification, taxonomy, and tests are unchanged.

**Do**:
1. In internal/log/process_role.go, update the two comments (line 44 and lines 66-68, including the routing-table note) so they no longer call bare `portal` "the TUI picker" — state that bare `portal` prints help/usage. Do NOT change the `roleTUI` mapping or the pinned `process_role_test.go` case.
2. In CLAUDE.md line 114, either mark "Incident of record #2" explicitly as pre-redesign history (so the removed `state cleanup` command and the deleted `TestStateUserFacingSubcommandsExitZero` read as period detail, not current guidance) or re-anchor the example on a surviving command body — keeping the still-valid lesson intact (Execute a real command body ⇒ inject every tmux-touching `*Deps`).

**Acceptance Criteria**:
- No comment in process_role.go describes bare `portal` as launching the TUI picker.
- The process-role classification and process_role_test.go are unchanged (comment-only edit in that file).
- CLAUDE.md's incident note no longer presents `state cleanup` / the deleted test as current, still-runnable surfaces.

**Tests**:
- No code test required (comment / doc-only change). Existing `process_role` tests must remain green, unchanged.
