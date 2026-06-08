---
topic: session-tagging-and-grouping
cycle: 2
total_proposed: 4
---
# Analysis Tasks: session-tagging-and-grouping (Cycle 2)

## Task 1: Route WithInsideTmux through the rebuildSessionList chokepoint
status: approved
severity: medium
sources: architecture

**Problem**: `rebuildSessionList` is designed as the single chokepoint over (sessions, projects, dir-resolution, mode). `WithInsideTmux` (internal/tui/model.go:461-471) is the one production path that pushes session items directly via `SetItems(ToListItems(filtered))`, skipping `resolveSessionDirs`, the grouping builders, and the mode switch. Today this is harmless only because in production `WithInsideTmux` runs immediately after `tui.New` (cmd/open.go:408) while `m.sessions` is still empty, so it pushes an empty list that the first `SessionsMsg → applySessions → rebuildSessionList` overwrites. The chokepoint holds over its full input set only by accident of call ordering. The existing `panic("unreachable")` guard covers only the filter-applied case, not the "sessions already populated + grouped mode" case — `NewModelWithSessions` already populates sessions before `WithInsideTmux` in tests, so a future refactor adopting that ordering in production would silently render flat/un-resolved/un-grouped items until the next refresh, with no guard tripping.

**Solution**: Route `WithInsideTmux`'s item population through `rebuildSessionList` so inside-tmux exclusion composes with mode/grouping/resolution through the single chokepoint. If the direct push is deliberately retained as a "no sessions yet" fast path, replace the partial panic guard with one that also asserts `m.sessions` is empty, making the construction-ordering assumption explicit and enforced rather than implicit.

**Outcome**: `WithInsideTmux` no longer bypasses resolution/grouping/mode. Either it flows through `rebuildSessionList` (preferred), or its fast-path is guarded by an assertion that trips if `m.sessions` is non-empty — so a future ordering change fails loudly instead of silently rendering ungrouped items.

**Do**:
1. Read internal/tui/model.go:461-471 (`WithInsideTmux`) and the surrounding `rebuildSessionList` / `applySessions` flow to confirm receiver types and the existing panic-guard placement.
2. Preferred path: change `WithInsideTmux` to set the inside-tmux exclusion state (the filtered-out current session) on the model, then call `rebuildSessionList` (already a pointer-receiver call elsewhere) instead of `SetItems(ToListItems(filtered))`. Verify the exclusion is applied inside `rebuildSessionList`'s input set (it already uses m.filteredSessions()) so it composes with grouping.
3. Fallback path (only if the direct push must be kept as a fast path): keep the direct `SetItems` but replace the partial `panic("unreachable")` guard so it also asserts `len(m.sessions) == 0`, panicking with a clear message if sessions are already populated when `WithInsideTmux` runs.
4. Confirm `cmd/open.go:408-410` construction still behaves identically (empty-sessions-at-construction case).

**Acceptance Criteria**:
- `WithInsideTmux` either delegates to `rebuildSessionList` or asserts `m.sessions` is empty before its direct push.
- The construction path in cmd/open.go produces identical user-visible behaviour to today (no regression for the empty-at-construction case).
- The "sessions already populated + grouped mode + inside tmux" case no longer renders un-resolved/un-grouped items silently — it either groups correctly or trips a guard.

**Tests**:
- Add a tui test that constructs a model with sessions already populated (à la `NewModelWithSessions`) in grouped mode, then applies `WithInsideTmux`, and asserts the rendered items are grouped/resolved (preferred path) OR that the guard panics (fallback path).
- Retain/verify the existing empty-at-construction test still passes (the production fast-path ordering).
- Verify a grouped-mode render after inside-tmux exclusion still excludes the current session and groups the remainder.

## Task 2: Pre-canonicalise stored project paths once per project-load instead of per grouped render
status: approved
severity: medium
sources: architecture

**Problem**: Each grouped render calls `MatchProjectByDir` once per session (buildByProject directly; buildByTag via `resolveSessionTags`). `MatchProjectByDir` (internal/project/pathkey.go:50-60) loops all projects and calls `CanonicalDirKey(p.Path)` for every project on every call, and `CanonicalDirKey` performs `filepath.EvalSymlinks` — a filesystem syscall. So a single grouped render costs ~N_sessions × M_projects `EvalSymlinks` calls, recomputing the same stored-path keys repeatedly. The spec deliberately made grouping cheap (the `@portal-dir` stamp avoids per-session `git rev-parse`), but stored-side canonicalisation reintroduces a per-render filesystem cost scaling with the product of two unbounded sets. Only the project side is wastefully recomputed; the session-side want key is already computed once per call.

**Solution**: Canonicalise stored project paths once when projects are cached — in the `ProjectsLoadedMsg` handler that sets `m.projects` (internal/tui/model.go ~1028) — building a `map[canonicalKey]Project` once per project-load. Have the grouping builders look up by pre-canonicalised key instead of calling `MatchProjectByDir`'s per-project canonicalisation loop. Keep `CanonicalDirKey` as the single source of truth for the key form. This collapses render cost from O(sessions × projects) syscalls to O(projects) syscalls once per project change plus O(sessions) map lookups.

**Outcome**: A grouped render performs zero `EvalSymlinks` syscalls over stored project paths; canonicalisation of stored paths happens once per project-load. Grouping output is identical to today (`CanonicalDirKey` remains the key form), but render cost is O(sessions) map lookups instead of O(sessions × projects) filesystem syscalls.

**Do**:
1. Read internal/project/pathkey.go:50-60 (`MatchProjectByDir`, `CanonicalDirKey`), internal/tui/grouping.go:54 and :130 (the builder call sites), and internal/tui/model.go (the `ProjectsLoadedMsg` handler setting `m.projects`).
2. Decide the design: a new project lookup helper. Option A — add `func BuildProjectIndex(projects []Project) map[string]Project` (keyed by `CanonicalDirKey(p.Path)`) + `func MatchProjectByCanonicalKey(index map[string]Project, sessionDir string) (Project, bool)` (canonicalises only the session-side key once, single map lookup) in internal/project. The builders take the index instead of the slice. Document collision policy (last-write-wins). Keep MatchProjectByDir for any other callers (or migrate them).
3. Cache the index on the model when `m.projects` is set (ProjectsLoadedMsg + anywhere projects are seeded, e.g. test seams). Rebuild on every project change.
4. Update buildByProject / resolveSessionTags (buildByTag) to look up via the index. Keep grouping output identical.
5. Keep `CanonicalDirKey` as the sole key-form function so stamped/stored key forms cannot drift.

**Acceptance Criteria**:
- Stored project paths are canonicalised once per project-load, not once per session per render.
- A grouped render uses map lookups keyed by `CanonicalDirKey`; no per-render loop re-runs `CanonicalDirKey` over every stored project path.
- Grouping output (project group membership, headings, catch-all) is byte-identical to the current behaviour for the same inputs.
- The lookup index is rebuilt whenever `m.projects` changes (no stale matches after project add/remove/edit).

**Tests**:
- Add a project test asserting the canonical-key index resolves a session to the same project as the current `MatchProjectByDir` for representative paths (including symlinked paths via t.TempDir + EvalSymlinks).
- Add a test asserting the index is rebuilt after a project mutation so a newly-added/removed project is reflected in grouping.
- Verify grouped-render output equivalence against the pre-change behaviour for a fixture with multiple sessions and projects (including a symlinked path).

## Task 3: Remove the dead SessionItem.Tag field, derive a tag accessor if needed
status: approved
severity: low
sources: architecture

**Problem**: `buildByTag` (internal/tui/grouping.go:107-113) sets `SessionItem.Tag`, `GroupKey`, and `GroupHeading` to the identical canonical-tag value for every By-Tag instance, but no production code reads `SessionItem.Tag` (internal/tui/session_item.go:61-66). Boundary detection and counting read `GroupKey`, the heading reads `GroupHeading`, and selection/attach/preview/kill all key on `Session.Name` (the field's own doc concedes this). The Untagged catch-all is distinguished by the `CatchAll` flag, not an empty `Tag`. So `Tag` is a third parallel copy of the same value that must be kept in sync, read only by test-assertion convenience handles — pure redundancy with `GroupKey`.

**Solution**: Remove the `Tag` field. Update affected tests to assert on `GroupKey`/`GroupHeading` instead. If a tag accessor is wanted for readability, derive it (a method returning `GroupKey` when `!CatchAll`) rather than storing a third copy.

**Outcome**: `SessionItem` no longer carries a write-only `Tag` field; `GroupKey`/`GroupHeading` are the single representation of a By-Tag item's tag value. Tests assert on `GroupKey`/`GroupHeading` (or a derived accessor). No behaviour change.

**Do**:
1. Read internal/tui/session_item.go:61-66 (`Tag` field + doc) and internal/tui/grouping.go:107-113 (`buildByTag` assignment).
2. Grep the codebase for all reads of `.Tag` on session items to confirm only tests (and the write in `buildByTag`) reference it.
3. Remove the `Tag` field and its assignment in `buildByTag`.
4. Update affected tests to assert on `GroupKey` (and `GroupHeading` where the heading is the assertion target); for catch-all items assert via the `CatchAll` flag.
5. Optional: if a readability accessor is desired, add a method (e.g. `func (i SessionItem) Tag() string { if i.CatchAll { return "" }; return i.GroupKey }`) instead of re-introducing a stored field.

**Acceptance Criteria**:
- The stored `SessionItem.Tag` field is removed; no production read or write of it remains.
- All By-Tag grouping behaviour (heading, counts, boundary detection, catch-all distinction) is unchanged.
- Tests assert on `GroupKey`/`GroupHeading` (or a derived accessor), and the suite passes.

**Tests**:
- Update existing By-Tag grouping tests that referenced `.Tag` to assert on `GroupKey`/`GroupHeading`.
- Verify a catch-all (Untagged) item is still correctly identified via `CatchAll` (not via an empty `Tag`).
- `go build` and `go test ./internal/tui/...` pass with no remaining `Tag`-field references.

## Task 4: Add an end-to-end @portal-dir stamp → ListSessions(Dir) round-trip integration test
status: approved
severity: low
sources: architecture

**Problem**: The two halves of the `@portal-dir` stamp seam are each unit-tested in isolation — `CreateFromDir`'s `SetSessionOption(@portal-dir)` write (internal/session/create.go:96) and `ListSessions` parsing `#{@portal-dir}` from a fabricated commander line (internal/tmux/tmux.go:198/239, internal/tmux/tmux_test.go:140) — but no test exercises the round trip through a real tmux server. Correctness hinges on the stamped value and the format-field read agreeing; a tmux quoting/escaping or format-string drift would pass both unit tests while breaking grouping in production. The package already has real-tmux integration fixtures (`tmuxtest`, `portal_saver_integration_test`) to model this on.

**Solution**: Add one integration-tagged test that creates a session via the real path, calls `ListSessions` against the real socket, and asserts `Session.Dir` equals the canonical git-root. Use the existing real-tmux fixtures and isolation helpers.

**Outcome**: A single integration-tagged test stamps a session via the production create path, reads it back via `ListSessions` on a real tmux socket, and asserts `Session.Dir` matches the resolved/canonical git-root — closing the gap where quoting/format-string drift would pass both unit tests but break grouping in production.

**Do**:
1. Read internal/session/create.go:96 (the stamp write), internal/tmux/tmux.go:198/239 (the `#{@portal-dir}` format read), and the existing real-tmux fixtures (`tmuxtest`, `portal_saver_integration_test`) to model the socket setup and build tags.
2. Add an integration-tagged test (matching the repo's integration build-tag convention) that: spins up a real tmux socket via `tmuxtest`, creates a session against a known directory (so the `@portal-dir` stamp is written — either via the production create path or a direct SetSessionOption mirroring it), then calls `ListSessions` against the same socket.
3. Assert the returned `Session.Dir` equals the canonical git-root for that directory (run the expected value through the same resolution the production path uses, so the assertion matches the stamped form). Cover a directory value that would exercise tmux quoting (e.g. a path that is fine, plus optionally a path with a space) to catch quoting/format drift.
4. Apply `portaltest.IsolateStateForTest` env to any spawned subprocess if the path touches state/daemon, and reap subprocesses per the canonical pattern.
5. Ensure the test is excluded from the default `go test ./...` run via the integration build tag, consistent with existing integration tests.

**Acceptance Criteria**:
- A new integration-tagged test creates a session with an `@portal-dir` stamp and reads it back via `ListSessions` against a real tmux socket.
- The test asserts `Session.Dir` equals the canonical git-root (using the same key form the production path stamps).
- The test follows the repo's integration build-tag and real-tmux fixture conventions, and does not run under the default `go test ./...`.
- The test uses isolated state env if it spawns any portal subprocess.

**Tests**:
- The task IS the test: the round-trip integration test described above.
- Verify it passes when run with the integration build tag against a real tmux server, and is excluded from the default test run.
