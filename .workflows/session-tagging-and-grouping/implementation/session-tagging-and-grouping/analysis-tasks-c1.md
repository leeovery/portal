---
topic: session-tagging-and-grouping
cycle: 1
total_proposed: 4
---
# Analysis Tasks: Session Tagging and Grouping (Cycle 1)

## Task 1: Wire lazy stamp-on-render fallback into the render path
status: approved
severity: high
sources: standards, architecture

**Problem**: The spec mandates a lazy stamp-on-render fallback as a core mechanism (Session → Directory Resolution; Acceptance Criterion #9): when the grouped render encounters a live session with no `@portal-dir`, it must resolve that session's directory from active pane → git-root, group it on THIS render, and stamp it for subsequent renders. `ResolveAndStampDir` / `ResolveSessionDir` (plus the `PaneStamper` / `PaneCurrentPathReader` seams and the `ActivePaneCurrentPath` tmux method) implement exactly this and are unit-tested — but they are dead code. No production code calls them: grep confirms no caller outside their own files, `cmd/open.go`'s `tuiConfig` never constructs a stamper/resolver dependency, and `rebuildSessionList → buildByProject/buildByTag` consume `tmux.Session.Dir` verbatim. An empty `Dir` routes straight to Unknown (By Project) at `grouping.go:48-51` / Untagged (By Tag) at `grouping.go:146-149`. Consequences: (1) post-reboot restored sessions return without `@portal-dir` (it lives in in-memory tmux state, not persisted in `sessions.json`) and sit permanently in Unknown/Untagged — breaks AC #9 and the "no restart-to-appear gap"; (2) pre-existing live sessions on first ship are never grouped; (3) QuickStart-created sessions deliberately skip the create-time stamp on the explicit promise that the lazy fallback re-stamps on first grouped render — but it never runs, so every `x` / `portal open <path>` session is mis-bucketed. Only `SessionCreator.CreateFromDir`-stamped sessions group correctly.

**Solution**: Make `rebuildSessionList` the true chokepoint over directory resolution by invoking `ResolveAndStampDir` for each session whose `Dir` is empty, before the grouping builders consume `Session.Dir`. Thread a `session.PaneStamper` + `resolver.CommandRunner` seam into the Model via a new functional Option (mirroring the existing seam-injection pattern), wired in `cmd/open.go`'s `tuiConfig` to the concrete `*tmux.Client`. Builders stay pure — they receive already-resolved `Session.Dir`.

**Outcome**: A live session with no `@portal-dir` whose active pane resolves to a git root appears under its project (By Project) / its tags (By Tag) on the first grouped render, and is best-effort stamped so subsequent renders are cheap. A session lands in Unknown/Untagged ONLY when the lazy fallback also fails (no readable `current_path` / unresolvable git root), per `dirstamp.go`'s contract. AC #9 is satisfied; post-reboot, first-ship, and QuickStart sessions all group correctly without a restart.

**Do**:
1. Add a new functional Option on the TUI Model (e.g. `WithDirResolver(stamper session.PaneStamper, runner resolver.CommandRunner)`), mirroring the existing seam-injection options. Store the stamper + runner on the Model.
2. In `cmd/open.go`'s `tuiConfig`, construct the production `PaneStamper` + `resolver.CommandRunner` against the concrete `*tmux.Client` and pass them via the new Option.
3. In `rebuildSessionList` (the chokepoint, before `buildByProject`/`buildByTag` are called), map each session whose `Dir` is empty through `ResolveAndStampDir(s.Name, s.Dir, stamper, runner)` and use the derived directory for this render's grouping. Keep the resolution best-effort: an unresolvable session retains an empty `Dir` and falls through to Unknown/Untagged as before.
4. Keep the builders pure — they must continue to receive an already-resolved `Session.Dir` and contain no resolution logic. Remove or correct the stale comment at `grouping.go:28` asserting `Session.Dir` is "already canonicalised upstream by Phase 1's lazy fallback/restamp".
5. Guard the nil-seam case (e.g. tests that construct the Model without the Option) so resolution is skipped gracefully rather than panicking.

**Acceptance Criteria**:
- A live session with empty `Dir` whose active pane resolves to a git root appears under its project (By Project mode) and under its tags (By Tag mode) on the first grouped render — NOT in Unknown/Untagged.
- The derived directory is best-effort stamped (`@portal-dir`) after the first grouped render, so the resolver is not re-invoked on subsequent renders for the same session.
- A session whose active pane has no readable `current_path` or does not resolve to a git root falls through to Unknown (By Project) / Untagged (By Tag) — the fallback failing is the only path to those buckets for a live session.
- The grouping builders contain no directory-resolution logic; they receive `Session.Dir` already resolved.
- `cmd/open.go`'s `tuiConfig` constructs and injects the production stamper + runner against `*tmux.Client`.

**Tests**:
- Integration/Model test: an empty-`Dir` session that resolves via the active-pane fallback appears under its project (not Unknown). Assert it is stamped after the render.
- Update the existing `grouping_test` "empty Dir → Unknown" cases so Unknown is reached ONLY when the fallback also fails (no readable `current_path` / unresolvable git root), per `dirstamp.go`'s contract.
- Test the By-Tag analogue: empty-`Dir` session resolving to a tagged project appears under its tags, not Untagged.
- Nil-seam test: Model constructed without the new Option does not panic; unresolved sessions route to Unknown/Untagged.

## Task 2: Re-group on ProjectsLoadedMsg so grouped first paint is independent of startup message order
status: approved
severity: medium
sources: architecture

**Problem**: `rebuildSessionList` groups against cached `m.projects`, but the `ProjectsLoadedMsg` handler caches `m.projects` WITHOUT calling `rebuildSessionList`, and `transitionFromLoading`/`evaluateDefaultPage` don't re-group either. At startup `fetchSessions` and `loadProjects` are batched concurrently. If `SessionsMsg` arrives before `ProjectsLoadedMsg` (common — session listing is one fast tmux call), `applySessions → rebuildSessionList` groups against an empty `m.projects`, so in a persisted By-Project/By-Tag mode every session lands in Unknown/Untagged on first paint and is never corrected until the next session refresh (only preview-dismiss or a projects-page round-trip). The render path is not the single chokepoint it claims to be: it reacts to session-slice changes but is blind to project-slice changes. `TestProjectsTransitionRegroupsWithUpdatedTags` only passes because it pre-seeds `m.projects` directly rather than exercising real load → navigate ordering, so the gap is untested.

**Solution**: Make the chokepoint reactive to its full input set. Have the `ProjectsLoadedMsg` handler call `rebuildSessionList()` (batched with the existing `setItemsCmd`) when sessions are already loaded and a grouped mode is active, so a late project load re-groups the visible list regardless of message arrival order.

**Outcome**: Regardless of whether `SessionsMsg` or `ProjectsLoadedMsg` arrives first, the grouped first paint reflects the loaded projects — sessions are bucketed under their correct project/tags on first render in a persisted grouped mode, with no transient Unknown/Untagged flash that requires a refresh to correct.

**Do**:
1. In the `ProjectsLoadedMsg` handler, after caching `m.projects`, detect whether sessions are already loaded AND a grouped mode (By Project / By Tag) is active.
2. If so, call `rebuildSessionList()` and batch the resulting command with the existing `setItemsCmd` (do not drop either command).
3. Ensure the path is a no-op (or harmless) when in flat/ungrouped mode or when sessions are not yet loaded — re-grouping should only fire when it would change the visible list.

**Acceptance Criteria**:
- With a persisted grouped mode active, if `SessionsMsg` arrives before `ProjectsLoadedMsg`, the visible list is re-grouped against the loaded projects on the late project load — no session is left stranded in Unknown/Untagged when it has a matching project/tag.
- The existing `setItemsCmd` continues to fire; the new `rebuildSessionList` is batched, not substituted.
- In flat/ungrouped mode, or before sessions load, the `ProjectsLoadedMsg` handler does not trigger a spurious re-group.

**Tests**:
- A test exercising real load ordering (sessions-before-projects) in a grouped mode that asserts the list is correctly grouped after `ProjectsLoadedMsg` — NOT pre-seeding `m.projects`. This closes the gap that `TestProjectsTransitionRegroupsWithUpdatedTags` leaves untested.
- A test asserting no re-group is triggered when in flat mode or when sessions have not yet loaded.

## Task 3: Reconcile remove-then-re-add of the same tag within one edit-modal session
status: approved
severity: medium
sources: standards

**Problem**: In a single open of the projects edit modal, if the user highlights an existing tag and presses `x` (appends to `editRemovedTags`, removes from `editTags`) then re-adds the same tag via the Add input + Enter (`editTags` gets it back, but `editRemovedTags` still holds it), `handleEditProjectConfirm` executes removals THEN additions: `RemoveTag(tag)` runs, but the addition loop skips it because the diff `!slices.Contains(m.editProject.Tags, tag)` is false (the tag was in the originally-loaded set). Net result: the tag is removed from `projects.json` even though the user re-added it before saving — silent data loss.

**Solution**: Reconcile the two buffers before persisting. Compute `removedTags := editRemovedTags MINUS final editTags` (equivalently, drop the tag from `editRemovedTags` on re-add), so the persisted delta reflects the true (original → final `editTags`) transition.

**Outcome**: A tag that the user removes and then re-adds within the same modal session survives the save — `projects.json` reflects the final state of `editTags`, with no silent drop.

**Do**:
1. In `handleEditProjectConfirm`, before applying removals, reconcile `editRemovedTags` against the final `editTags`: only tags present in `editRemovedTags` AND absent from the final `editTags` are genuine removals.
2. Apply `RemoveTag` only to that reconciled removal set; apply additions for tags in `editTags` not in the original set, as before.
3. Verify the addition/removal ordering no longer produces a net-loss for a re-added tag.

**Acceptance Criteria**:
- Removing an existing tag with `x` and then re-adding the same tag via the Add input before saving leaves the tag present in `projects.json` after confirm.
- Removing a tag and NOT re-adding it still removes it (no regression to the genuine-removal path).
- Adding a brand-new tag still persists it.

**Tests**:
- A modal test for remove-then-re-add of the same tag, asserting the tag survives the save.
- Regression test: remove-only (no re-add) still removes the tag.

## Task 4: Extract shared grouping-assembly skeleton from buildByProject/buildByTag
status: approved
severity: medium
sources: duplication

**Problem**: `buildByProject` (`internal/tui/grouping.go:43-78`) and `buildByTag` (`internal/tui/grouping.go:106-138`) are structurally parallel near-duplicates. Both (a) iterate sessions, partitioning into a typed `[]SessionItem` "resolved" slice and a `[]list.Item` catch-all slice; (b) sort the resolved slice with a byte-identical `slices.SortFunc` comparing `GroupKey` then `Session.Name` (`grouping.go:66-71` vs `126-131` are the same five lines); (c) convert the typed resolved slice into `[]list.Item` via the same `make(...); for range; append` loop (`grouping.go:73-77` vs `133-137`); (d) hand off to `appendCatchAll`. Only the per-session resolution (project-match vs tag-expansion) genuinely differs. A future ordering or item-construction change must be made in lockstep in both — copy-paste-drift risk. Additionally, the same `[]SessionItem → []list.Item` boxing loop recurs a third time at `grouping.go:225-230` (a related LOW duplication finding that folds into this extraction).

**Solution**: Extract the shared tail — the `(GroupKey, Session.Name)` sort + the typed → `[]list.Item` conversion + the `appendCatchAll` handoff — into one helper, e.g. `assembleGroups(resolved []SessionItem, catchAll []list.Item, heading string) []list.Item`. Each builder then owns only its resolution loop. Extract the boxing loop into `sessionItemsToList(items []SessionItem) []list.Item` and call it from `assembleGroups` and the third site at `grouping.go:225-230`; two of the three original boxing call sites collapse automatically into `assembleGroups`.

**Outcome**: A single source of truth for grouped-list assembly ordering and item construction. `buildByProject` and `buildByTag` retain only their genuinely-distinct resolution loops; a future ordering or boxing change is made once, not in lockstep across two (or three) sites.

**Do**:
1. Add `sessionItemsToList(items []SessionItem) []list.Item` performing the capacity-sized allocate + box-via-append loop.
2. Add `assembleGroups(resolved []SessionItem, catchAll []list.Item, heading string) []list.Item` that sorts `resolved` by `(GroupKey, Session.Name)`, converts via `sessionItemsToList`, and hands off to `appendCatchAll`.
3. Refactor `buildByProject` and `buildByTag` to own only their per-session resolution loop and delegate the tail to `assembleGroups`.
4. Replace the third boxing site at `grouping.go:225-230` with a `sessionItemsToList` call.
5. Confirm behaviour is byte-identical — this is a pure refactor with no behavioural change.

**Acceptance Criteria**:
- `buildByProject` and `buildByTag` no longer contain a duplicated sort comparator or boxing loop; both delegate to `assembleGroups`.
- The `(GroupKey, Session.Name)` sort ordering and `appendCatchAll` handoff are defined in exactly one place.
- All three former `[]SessionItem → []list.Item` boxing sites route through `sessionItemsToList`.
- No behavioural change: existing grouping output (ordering, catch-all placement, headings) is unchanged.

**Tests**:
- Existing grouping tests for By Project and By Tag continue to pass unchanged (regression guard for the pure refactor).
- A test asserting identical output ordering before/after for a representative session set across both grouped modes.
