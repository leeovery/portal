# Plan: File Browser Alias Breadcrumb

## Phases

### Phase 1: Re-sweep and Reconcile the Removal Manifest
status: draft

**Goal**: Re-confirm the Removal Manifest against the current HEAD before deleting anything — run the mandated fresh repo-wide reference sweep and reconcile any site not already in the manifest, producing a verified edit set for the deletion phases to consume.

**Why this order**: The specification explicitly requires a fresh repo-wide reference sweep at implementation start, because the manifest's set-completeness guarantee is relative to the investigation-date codebase. A reference added between investigation and implementation (especially a doc/comment reference the compile+test gate would not catch) must be caught here, before any destruction.

**Acceptance**:
- [ ] Repo-wide sweep run for all mandated tokens: `internal/ui`, `internal/browser`, `pageFileBrowser`, `DirLister`, `WithDirLister`, the `b`/"browse" strings, and the bare quoted tokens `"ui"` and `"browser"` (which the path-prefixed greps miss).
- [ ] Every hit cross-checked against the manifest; any site not in the manifest is identified and folded into the reconciled edit set (or explicitly recorded as a false positive).
- [ ] Manifest line numbers re-confirmed against current HEAD (line numbers may have drifted since the investigation date).
- [ ] No deletion or edit performed in this phase; `go build ./...` and `go test ./...` remain green as the unchanged baseline.

### Phase 2: Remove the Consumers
status: draft

**Goal**: Delete every reference to the file browser from its consumers — the central TUI model (`internal/tui/model.go`), the `cmd/open.go` wiring, and all coupled test files — while the `internal/ui` and `internal/browser` packages still physically exist, so the tree never enters a transient non-compiling state.

**Why this order**: The specification's sequencing constraint mandates removing the consumers before the packages ("delete the two packages last … remove the consumers first"). Severing all importers first means package deletion in Phase 3 lands against a zero-importer target and the build stays green throughout.

**Acceptance**:
- [ ] `internal/tui/model.go` edited per the manifest: `internal/ui` import, `pageFileBrowser` const + comment, `DirLister` alias, the `dirLister`/`startPath`/`fileBrowser` fields, `WithDirLister` option + doc, the `b`/"browse" help bindings and the `commandPendingHelpKeys` comment, the `BrowserDirSelectedMsg`/`BrowserCancelMsg` handlers, the `pageFileBrowser` update and view arms, the Projects-page `b` dispatch, `handleBrowseKey()`, and `updateFileBrowser()` — all removed.
- [ ] `createSession` and `cfg.cwd`/`m.cwd` are left untouched (they have non-browser callers/consumers per the scope boundary).
- [ ] `cmd/open.go` edited per the manifest: `internal/browser` import, `osDirLister` type + method, and the `dirLister` field/option/literal removed; `tui.WithCWD(cfg.cwd)` and `cwd: cwd` retained.
- [ ] All coupled test edits applied per the manifest across `internal/tui/model_test.go`, `cmd/open_test.go`, and the three `pagepreview_*_test.go` files — including removal of the stale `"browser"` and `"ui"` keys from the surface-audit `preExistingPackages` allow-list, and the rename of the `TestCommandPendingBrowseAndNKey` survivor to `TestCommandPendingNKey`.
- [ ] `go build ./...` and `go test ./...` are green with `internal/ui` and `internal/browser` still present on disk but now carrying zero importers.

### Phase 3: Delete the Packages, Docs, and Verify the Gate
status: draft

**Goal**: Remove the two now-unreferenced packages with `rm -rf`, bring the docs into line with the post-removal reality, and satisfy the full acceptance gate — green build/test, zero remaining references, and the blocking manual checks that are this fix's only behavioural verification.

**Why this order**: The packages are deleted last per the sequencing constraint, against the zero-importer state Phase 2 established. This phase closes out the doc-consistency edits (which the compile/test gate cannot catch) and runs the final acceptance verification.

**Acceptance**:
- [ ] `internal/ui/` and `internal/browser/` directories deleted entirely via `rm -rf` (whole directories, including any file added since investigation) and confirmed no longer present on disk.
- [ ] Docs updated: `README.md` four-views→three-views; `CLAUDE.md` `tui`-row page state machine and the `pagePreview`/`pageFileBrowser` phrasing corrected, and the `browser` and `ui` package-table rows deleted.
- [ ] `go build ./...` is green and `go test ./...` is green.
- [ ] Zero remaining references: the acceptance-gate spot-check grep targets all come back clean, including non-compiled doc/prose hits the build/test gate would miss.
- [ ] Blocking manual check passes: on the Projects page, `b` opens nothing — a visible no-op that opens no view (it must not open the file browser or any other page).
- [ ] Blocking manual check passes: the Sessions, Projects, and Preview pages, the alias CLI (`portal alias set/rm/list`), and the projects-modal alias editor all behave exactly as before the removal — no page fails to open, no command regresses.
