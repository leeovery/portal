---
phase: 2
phase_name: Remove the Consumers
total: 2
---

## file-browser-alias-breadcrumb-2-1 | approved

### Task 1: Remove the file browser from the internal/tui package (model.go + coupled tui test files)

**Problem**: The file-browser feature is being removed in full (the reported alias-audit bug sits on unreachable dead code; the user confirmed they never use the file browser and want it gone). `internal/tui` is the *only* production consumer of `internal/ui` — `internal/tui/model.go` wires the file browser into the TUI page state machine (the `pageFileBrowser` page, the `WithDirLister` option, the `b`-key dispatch on the Projects page, and the `ui.BrowserDirSelectedMsg`/`ui.BrowserCancelMsg` cross-view handlers). Several `internal/tui` test files (`model_test.go` and four `pagepreview_*_test.go` files) reference the removed symbols in the *same package*, so if model.go is stripped without editing them in the same task the `internal/tui` package fails to build/test (a same-package test referencing a deleted symbol reds the whole package). This is the consumer-side removal that must land before the `internal/ui` directory itself is deleted in Phase 3.

**Solution**: Strip every file-browser reference from `internal/tui/model.go` and from the coupled same-package test files (`model_test.go`, `pagepreview_entry_test.go`, `pagepreview_refetch_test.go`, `pagepreview_bracket_test.go`, `pagepreview_surface_audit_test.go`) in a single task, so the `internal/tui` package builds and tests green with `internal/ui` reduced to zero importers — while leaving the `internal/ui` directory physically on disk (Phase 3 deletes it).

**Outcome**: `internal/tui` builds and its tests pass; `internal/ui` has zero importers; the `pageFileBrowser` page, `WithDirLister` option, `b`-key browse dispatch, browser message handlers, `handleBrowseKey`/`updateFileBrowser`, the `dirLister`/`startPath`/`fileBrowser` fields, and the `DirLister` type alias are all gone; the surviving non-browser machinery (`createSession`, `cfg.cwd`/`m.cwd`, `viewCWD`, the Sessions/Projects/Preview pages, the projects-modal alias editor) is intact and functional; the `pagePreview` iota silently renumbers 4→3 with no behavioural change; and on the Projects page the `b` key opens no view (falls through to `projectList.Update` as a visible no-op).

**Do**:
- **Authority for exact lines**: use the Phase 1 corrected edit set (`file-browser-alias-breadcrumb-1-3` output) for HEAD-accurate line numbers. The line numbers below are the investigation-dated manifest references — locate each by the cited *symbol/construct*, not the bare number, and confirm against the corrected edit set before editing.
- **`internal/tui/model.go`** — remove:
  - The `internal/ui` import (manifest L20 — `"github.com/leeovery/portal/internal/ui"`). It becomes unused after the deletions below.
  - The `pageFileBrowser` const + its doc comment from the `page` iota block (manifest L33-34). `pagePreview` then renumbers 4→3 — this is iota-safe (no int↔page cast, no numeric comparison, no JSON/prefs serialization; both constants are unexported and all tests compare the symbolic constant), so no other change is needed for the renumber.
  - The `DirLister` doc comment + `type DirLister = ui.DirLister` alias (manifest L119-120).
  - The struct fields `dirLister`, `startPath`, `fileBrowser` (manifest L189 / L190 / L194). These are read only inside the removed sites — no dangling reads remain.
  - The `WithDirLister` Option function + its doc comment (manifest L574-580).
  - The `projectHelpKeys` `b`/"browse" key binding (manifest L721).
  - The `b` mention in the `commandPendingHelpKeys` doc comment (manifest L728-729 — it currently reads *"Only enter (run here), n, b, /, and q are shown"*; drop the `b`).
  - The `commandPendingHelpKeys` `b`/"browse" key binding (manifest L732).
  - The `ui.BrowserDirSelectedMsg` and `ui.BrowserCancelMsg` cross-view handlers (manifest L1363-1366). **`createSession` STAYS** — it has 3 other callers (project-enter ~L1666, `createSessionInCWD` ~L2241); remove only the browser→create-session entry point, not `createSession` itself.
  - The `case pageFileBrowser:` arm in the update path (manifest L1544-1545).
  - The `case isRuneKey(msg, "b"):` dispatch on the Projects page (manifest L1637-1638).
  - The `handleBrowseKey()` method (manifest L1649-1656).
  - The `updateFileBrowser()` method (manifest L1971-1977).
  - The `case pageFileBrowser:` arm in the view path (manifest L2269-2270).
  - **KEEP** `cfg.cwd` / `m.cwd` and everything consuming them (`WithCWD`, `viewCWD`, `createSession(m.cwd)`) — they are independent of the browser and must remain.
- **`internal/tui/model_test.go`** — edit:
  - Remove the `internal/browser` and `internal/ui` imports (manifest L13 / L17) — both become unused after the deletions below.
  - Remove the `mockDirLister` type + its method (manifest L775-785).
  - Delete the whole functions `TestFileBrowserIntegration` (manifest L787-1034) and `TestFileBrowserFromProjectsPage` (manifest L5551-5758) — every subtest in each is the browser flow.
  - Delete the browser subtests whose enclosing function survives: `TestCommandPendingMode` → "browse selection applies pending command" (manifest L2371-2414); `TestNewWithFunctionalOptions` → "WithDirLister enables file browser" (manifest L3031-3066); `TestCommandPendingEscAndQuit` → "Esc in file browser…" (manifest L7182-7229).
  - In `TestCommandPendingBrowseAndNKey` (manifest L6737-6829): delete both browser subtests ("browse directory selection forwards command…" L6737-6785 and "browse cancel returns to locked Projects page…" L6787-6829). The survivor is n-key-only, so **rename the surviving function from `TestCommandPendingBrowseAndNKey` to `TestCommandPendingNKey`** — the `Browse` in the name is now stale. This rename is the decided end-state, not optional.
  - Rework (keep the test, strip only the browser setup): `TestKillSession` → "NewWithAllDeps supports kill" — drop the `mockDirLister` declaration (manifest L1671) and the `WithDirLister(...)` argument (manifest L1673); keep the kill assertion. `TestNewWithFunctionalOptions` → "all options combined" — drop the `dirLister` var (manifest L3080-3084) and the `WithDirLister(...)` argument (manifest L3092); keep the rest.
- **`internal/tui/pagepreview_entry_test.go`** — edit:
  - Delete `TestSpaceOnFileBrowserPageDoesNotCallNewPreviewModel` (manifest L264-286): its premise (being on the file-browser page) ceases to exist; the same guarantee ("Space only previews from the Sessions page") is already covered by the sibling `TestSpaceOnProjectsPageDoesNotCallNewPreviewModel` (manifest L240-262), which stays.
  - Drop the stale `internal/ui/browser_test.go` reference from the file-header doc comment (manifest L12).
- **`internal/tui/pagepreview_refetch_test.go`** — update the comment at manifest L27 to drop the `pageFileBrowser → PageSessions` transition mention.
- **`internal/tui/pagepreview_bracket_test.go`** — drop the stale `internal/ui/browser.go` reference from the doc comment at manifest L14 (it compiles, but is a dangling pointer to a soon-deleted file).
- **`internal/tui/pagepreview_surface_audit_test.go`** — in `TestSurfaceAudit_NoNewPackageForPreview`'s `preExistingPackages` allow-list map, remove the stale `"browser":` key (manifest L295) and the `"ui":` key (manifest L321). The build/test gate does NOT catch a stale key here — the audit only errors on a directory present on disk but *absent* from the allow-list, so an unused extra key leaves the test green. The bare `"ui"` key also escapes the path-prefixed re-sweep greps (`internal/ui` requires the path prefix), so it must be removed explicitly by hand here.
- **Boundary**: do NOT delete the `internal/ui` directory in this task — it must remain physically on disk; Phase 3 removes it. This task only severs `internal/tui`'s use of it.
- After editing, run `go build ./internal/tui/...` and `go test ./internal/tui/...`, then the full `go build ./...` + `go test ./...` gate (the full gate confirms `internal/ui` becoming zero-importer did not break any other package). Use isolated env for any daemon-spawning test paths per `portaltest.IsolateStateForTest`.

**Acceptance Criteria**:
- [ ] `go build ./...` is green after the edits.
- [ ] `go test ./...` is green after the edits (green-modulo the documented known-flaky `internal/tmux` kill-barrier timing test — re-run that package in isolation to confirm it is the known flake, not a regression).
- [ ] `internal/ui` has zero importers (no Go file outside `internal/ui` imports it) — confirmed by grep for `internal/ui` returning no compiled-code hit.
- [ ] No remaining references in `internal/tui` to `pageFileBrowser`, `DirLister`, `WithDirLister`, `dirLister`, `startPath`, `fileBrowser`, `handleBrowseKey`, `updateFileBrowser`, `ui.BrowserDirSelectedMsg`, `ui.BrowserCancelMsg`, `mockDirLister`, or the `b`/"browse" keybinding.
- [ ] The surviving symbols `createSession`, `cfg.cwd`/`m.cwd`, `viewCWD`, and `WithCWD` are still present and referenced (not removed as collateral).
- [ ] `pagePreview` renumbers 4→3 transparently — no test fails on a page-constant comparison, and there is no int↔page cast anywhere that the renumber would break.
- [ ] The renamed survivor `TestCommandPendingNKey` exists (no `TestCommandPendingBrowseAndNKey` remains) and passes; it exercises the n-key path only.
- [ ] The surface-audit `preExistingPackages` allow-list no longer contains the `"browser":` or `"ui":` keys, and `TestSurfaceAudit_NoNewPackageForPreview` still passes.
- [ ] On the Projects page, pressing `b` opens no view — it falls through to `projectList.Update(msg)` as a visible no-op (does not open the file browser or any other page). This is the spec's only behavioural verification for the tui side.
- [ ] The `internal/ui` directory still exists on disk (deletion is Phase 3).

**Tests**:
- `"internal/tui builds and its tests pass with internal/ui now a zero-importer package"`
- `"go build ./... and go test ./... are green after the tui consumer edits (modulo the known internal/tmux kill-barrier flake)"`
- `"the pagePreview iota renumbers 4→3 with no page-constant comparison failing and no int↔page cast breaking"`
- `"createSession, cfg.cwd/m.cwd, viewCWD, and WithCWD are still referenced (not removed as collateral)"`
- `"the survivor TestCommandPendingNKey exists, exercises only the n-key path, and passes; no TestCommandPendingBrowseAndNKey remains"`
- `"TestSurfaceAudit_NoNewPackageForPreview passes with the stale browser/ui allow-list keys removed"`
- `"pressing b on the Projects page opens no view (falls through to projectList.Update as a visible no-op)"`

**Edge Cases**:
- **`pageFileBrowser` iota renumber 4→3** — removing the const lets `pagePreview` shift down one. Verify every page-constant comparison still passes and that no int↔page cast or serialization exists anywhere (the spec confirms none does; this is a verification, not a fix).
- **`createSession` and `cfg.cwd`/`m.cwd` MUST stay** — `createSession` has 3 non-browser callers (project-enter, `createSessionInCWD`); `m.cwd` feeds `viewCWD` and `createSession(m.cwd)`, and `cfg.cwd` feeds `WithCWD`. Remove only the browser→create-session entry point and the browser-coupled state, never these.
- **Survivor rename** — after deleting both browser subtests from `TestCommandPendingBrowseAndNKey`, the remaining function is n-key-only; rename it to `TestCommandPendingNKey`. Leaving the stale `Browse` in the name is not the decided end-state.
- **Projects-page `b` fall-through** — with the `projectHelpKeys` binding and the `case isRuneKey(msg, "b")` dispatch removed, `b` must fall through to the default `projectList.Update(msg)` and be a visible no-op that opens *no* view (not the file browser, not any other page).
- **Same-package test coupling** — `model_test.go` and all four `pagepreview_*_test.go` files reference removed symbols in the same `internal/tui` package; they MUST be edited in this same task or the `internal/tui` build/test reds. They cannot be split into a later task.
- **Bare `"ui"` allow-list key** — the surface-audit `preExistingPackages` `"ui"` key escapes path-prefixed greps and the build/test gate (the audit only errors on a missing dir, not an extra key). Remove it explicitly by hand; do not rely on a grep or the gate to surface it.
- **`internal/ui` stays on disk** — this task removes only the *usage*; the directory itself is deleted in Phase 3. Do not `rm -rf` it here.

**Context**:
> Spec — Removal Manifest, `internal/tui/model.go` edit sites: the full enumerated list of import / iota / field / option / keybinding / handler / case-arm / method removals reproduced in the Do steps. **`createSession` STAYS** (3 other callers); `cfg.cwd`/`m.cwd` MUST stay (consumed by `WithCWD` and `viewCWD`/`createSession`).
> Spec — Removal Manifest, `internal/tui/model_test.go`: whole-function deletions (`TestFileBrowserIntegration`, `TestFileBrowserFromProjectsPage`), browser-subtest deletions, the `TestCommandPendingBrowseAndNKey` → `TestCommandPendingNKey` rename ("the decided end-state, not optional"), and the two reworks (`TestKillSession` "NewWithAllDeps supports kill", `TestNewWithFunctionalOptions` "all options combined").
> Spec — Removal Manifest, "Other `*_test.go` (incidental coupling — preview tests)": the four `pagepreview_*_test.go` edits, including the surface-audit allow-list keys — "**The build/test gate does not catch this** … **The bare `\"ui\"` key also escapes the path-prefixed re-sweep greps**".
> Spec — "Iota-safety + dangling-reference verification": removing `pageFileBrowser` and letting `pagePreview` renumber 4→3 is transparent; `m.startPath` / `m.dirLister` are read only inside removed sites; `cfg.cwd` / `m.cwd` MUST stay.
> Spec — "Acceptance gate", Manual check (blocking): on the Projects page, `b` is no longer recognised — it falls through to `projectList.Update(msg)` as a visible no-op that opens no view.
> Spec — "Sequencing": delete the two packages **last** — remove the consumers first, then the packages. This task is the consumer removal; the `internal/ui` directory deletion is Phase 3.
> Phase 1 corrected edit set (`file-browser-alias-breadcrumb-1-3`) is the HEAD-accurate authority for exact line numbers — the manifest numbers are investigation-dated.

**Spec Reference**: `.workflows/file-browser-alias-breadcrumb/specification/file-browser-alias-breadcrumb/specification.md` — "Removal Manifest" (`internal/tui/model.go` edit sites, `internal/tui/model_test.go` test edits, "Other `*_test.go` (incidental coupling — preview tests)"), "Iota-safety + dangling-reference verification", and "Acceptance gate".

## file-browser-alias-breadcrumb-2-2 | approved

### Task 2: Remove the file browser from the cmd package (open.go + open_test.go)

**Problem**: `cmd/open.go` is the second (and last) production consumer of file-browser machinery: it imports `internal/browser`, defines the `osDirLister` adapter (a `tui.DirLister` implementation over `browser.ListDirectories`), carries a `dirLister tui.DirLister` field on `tuiConfig`, passes `tui.WithDirLister(cfg.dirLister, cfg.cwd)` when launching the TUI, and seeds the cfg literal with `dirLister: &osDirLister{}`. Its test file `cmd/open_test.go` mirrors this with a `stubDirLister` type and a `dirLister:` entry in `defaultTestTUIConfig`. After Task 2-1 removes `tui.WithDirLister`/`tui.DirLister`, these `cmd` references must also go, or `cmd` fails to build. `internal/browser` only becomes a zero-importer package once `osDirLister` (and the test's `stubDirLister`) are removed — and the cfg-literal `dirLister:` entry becomes a compile error the instant the production `tuiConfig.dirLister` field is gone, so the production field and both literals must be removed together.

**Solution**: Strip every file-browser reference from `cmd/open.go` (the `internal/browser` import, the `osDirLister` type+method, the `tuiConfig.dirLister` field, the `WithDirLister` option, and the cfg-literal `dirLister` entry) and from `cmd/open_test.go` (the `internal/browser` import, the `stubDirLister` type+method, and the `defaultTestTUIConfig` `dirLister` entry) in one task, while keeping the adjacent `WithCWD(cfg.cwd)` option and the `cwd: cwd`/`cwd` cfg fields — leaving the `internal/browser` directory physically on disk (Phase 3 deletes it).

**Outcome**: `cmd` builds and its tests pass; `internal/browser` has zero importers; the file-browser wiring (`osDirLister`, `stubDirLister`, `dirLister` field/literals, `WithDirLister`, the `internal/browser` import) is gone from both `cmd/open.go` and `cmd/open_test.go`; and the surviving cwd plumbing (`WithCWD(cfg.cwd)`, the `cwd: cwd` production literal, and the test `cwd` field) is intact.

**Do**:
- **Authority for exact lines**: use the Phase 1 corrected edit set (`file-browser-alias-breadcrumb-1-3` output) for HEAD-accurate line numbers; locate each site by its cited symbol/construct, not the bare manifest number, and confirm against the corrected edit set before editing.
- **`cmd/open.go`** — remove (as one coherent change, because the import goes unused only after `osDirLister` is removed, and the cfg-literal `dirLister:` becomes a compile error the instant the `tuiConfig.dirLister` field is gone):
  - The `internal/browser` import (manifest L11 — `"github.com/leeovery/portal/internal/browser"`).
  - The `osDirLister` type + its `ListDirectories` method + the doc comment (manifest L332-338).
  - The `dirLister tui.DirLister` field from the `tuiConfig` struct (manifest L349).
  - The `tui.WithDirLister(cfg.dirLister, cfg.cwd)` option in the TUI-launch option list (manifest L370). **KEEP the adjacent `tui.WithCWD(cfg.cwd)` option (manifest L371)** — it is independent of the browser.
  - The `dirLister: &osDirLister{}` entry from the `tuiConfig` literal (manifest L505). **KEEP `cwd: cwd`** in the same literal — it is still consumed by `WithCWD`.
- **`cmd/open_test.go`** — remove:
  - The `internal/browser` import (manifest L18) — unused only after `stubDirLister` is removed, so strip the type+method+import together.
  - The `stubDirLister` type + its `ListDirectories` method (manifest L703-708; the `browser.DirEntry` use lives inside this method and is removed with it).
  - The `dirLister: &stubDirLister{}` entry from `defaultTestTUIConfig`'s literal (manifest L773) — **required**: the production `tuiConfig.dirLister` field is gone, so leaving this entry is a compile error. **KEEP the `cwd` entry** in the same literal.
- **Boundary**: do NOT delete the `internal/browser` directory in this task — it must remain physically on disk with zero importers; Phase 3 removes it. This task only severs `cmd`'s use of it.
- After editing, run `go build ./cmd/...` and `go test ./cmd/...`, then the full `go build ./...` + `go test ./...` gate (the full gate confirms `internal/browser` becoming zero-importer broke nothing else). Use isolated env for any daemon-spawning test paths per `portaltest.IsolateStateForTest`.

**Acceptance Criteria**:
- [ ] `go build ./...` is green after the edits.
- [ ] `go test ./...` is green after the edits (green-modulo the documented known-flaky `internal/tmux` kill-barrier timing test — re-run that package in isolation to confirm it is the known flake, not a regression).
- [ ] `internal/browser` has zero importers — confirmed by grep for `internal/browser` and `browser.DirEntry` / `browser.ListDirectories` returning no compiled-code hit.
- [ ] No remaining references in `cmd` to `osDirLister`, `stubDirLister`, `dirLister`, `WithDirLister`, or the `internal/browser` import.
- [ ] The surviving `tui.WithCWD(cfg.cwd)` option, the `cwd: cwd` production cfg literal, and the test `cwd` field in `defaultTestTUIConfig` are all still present.
- [ ] The `internal/browser` directory still exists on disk (deletion is Phase 3).

**Tests**:
- `"cmd builds and its tests pass with internal/browser now a zero-importer package"`
- `"go build ./... and go test ./... are green after the cmd consumer edits (modulo the known internal/tmux kill-barrier flake)"`
- `"WithCWD(cfg.cwd) and the cwd cfg literals (production cwd: cwd and the test cwd field) survive the removal and are still referenced"`
- `"no reference to osDirLister, stubDirLister, dirLister, WithDirLister, or internal/browser remains in cmd"`
- `"internal/browser is a zero-importer package on disk (no compiled-code hit for internal/browser, browser.DirEntry, or browser.ListDirectories)"`

**Edge Cases**:
- **Keep `WithCWD` / `cwd`, drop the adjacent `WithDirLister` / `dirLister`** — `WithCWD(cfg.cwd)` (option list) and `cwd: cwd` (production cfg literal) and the test `cwd` field must survive even though the `WithDirLister(cfg.dirLister, cfg.cwd)` option and the `dirLister` field/literals adjacent to them are removed. The two are co-located but independent; remove only the browser one.
- **Import goes unused only after the adapter is removed** — in `open.go` the `internal/browser` import is live until `osDirLister` is gone; in `open_test.go` it is live until `stubDirLister` is gone. Strip type + method + import together so the package never transiently fails on either an unused import or an undefined symbol.
- **Cfg-literal entry is a compile error once the field is gone** — removing the production `tuiConfig.dirLister` field makes both the production `dirLister: &osDirLister{}` literal and the test `dirLister: &stubDirLister{}` literal in `defaultTestTUIConfig` compile errors. All three (field + both literals) must be removed; leaving either literal after the field is gone reds the build.
- **`internal/browser` stays on disk** — this task removes only the *usage*, driving the package to zero importers; the directory itself is deleted in Phase 3. Do not `rm -rf` it here.

**Context**:
> Spec — Removal Manifest, `cmd/open.go` edit sites: remove the `internal/browser` import (L11), `osDirLister` type+method+comment (L332-338), `dirLister tui.DirLister` field (L349), the `tui.WithDirLister(cfg.dirLister, cfg.cwd)` opt (L370) — **keep L371 `tui.WithCWD(cfg.cwd)`** — and the cfg-literal `dirLister: &osDirLister{}` (L505) — **keep `cwd: cwd`**.
> Spec — Removal Manifest, `cmd/open_test.go` edit sites: remove the `internal/browser` import (L18), `stubDirLister` type+method (L703-708, the `browser.DirEntry` use lives inside it), and the `dirLister: &stubDirLister{}` literal in `defaultTestTUIConfig` (L773, **required** — the production field is gone; keep `cwd`).
> Spec — Removal Manifest, "`internal/browser/`": after `internal/ui` and `cmd/open.go`'s `osDirLister` are gone it has **zero** importers; the directory is removed (whole-directory `rm -rf`) in Phase 3, not here.
> Spec — "Scope boundary — what must stay green and unchanged": `cfg.cwd` / `m.cwd` are consumed by `WithCWD` and `viewCWD` / `createSession(m.cwd)`, independent of the browser.
> Spec — "Sequencing": remove the consumers first, then the packages. This task is the second consumer removal; the `internal/browser` directory deletion is Phase 3.
> Phase 1 corrected edit set (`file-browser-alias-breadcrumb-1-3`) is the HEAD-accurate authority for exact line numbers — the manifest numbers are investigation-dated.

**Spec Reference**: `.workflows/file-browser-alias-breadcrumb/specification/file-browser-alias-breadcrumb/specification.md` — "Removal Manifest" (`cmd/open.go` edit sites, `cmd/open_test.go` edit sites, "Packages deleted entirely" → `internal/browser/`), "Scope boundary — what must stay green and unchanged", and "Acceptance gate".
