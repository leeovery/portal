# Implementation Review: File Browser Alias Breadcrumb

**Plan**: file-browser-alias-breadcrumb
**QA Verdict**: Approve

## Summary

The bugfix — whose decided remedy is the *full removal* of the dead file-browser feature rather than an in-place audit-fix — is implemented cleanly and completely. All 9 plan tasks across 3 phases are done and independently verified. Both `internal/ui` and `internal/browser` are deleted from disk, every consumer reference in `internal/tui` and `cmd` is severed, the coupled test files (model_test.go, the four pagepreview_*_test.go) are reconciled, the two gate-blind classes the spec flagged (the bare-name surface-audit allow-list keys and the prose docs) are both handled, and the `TestCommandPendingBrowseAndNKey → TestCommandPendingNKey` rename landed. I independently ran the authoritative acceptance gate: `go build ./...` is green and `go test ./...` is green-modulo a single load-induced tmux-server flake in `internal/restore` (a package this removal never touched), which passes cleanly in isolation. No blocking issues; one zero-risk doc-comment clarity note.

## QA Verification

### Specification Compliance

Implementation matches the specification's Removal Manifest exactly. The scope boundary was respected — every named survivor is intact: the alias CLI (`cmd/alias.go`), the projects-modal alias editor (`aliasEditor → SetAndSave`, model.go:103/181/1897/1914), `createSession` with its three non-browser callers (model.go:1529), and `cfg.cwd`/`m.cwd` with `tui.WithCWD(cfg.cwd)` (open.go:360) + `cwd: cwd` (open.go:504). The iota-safety prediction held — `pageFileBrowser` removed, `pagePreview` renumbers 4→3 with no int↔page cast or numeric comparison anywhere. No `SetAndSave` rewiring was attempted (correct — the chosen approach was removal, not in-place fix). Net test delta is removal, not addition, as the spec mandated.

### Plan Completion

- [x] Phase 1 (re-sweep + reconcile) acceptance criteria met — sweep completeness confirmed by a clean post-removal end state; scope-boundary survivors correctly identified; corrected edit set drove an accurate deletion.
- [x] Phase 2 (remove consumers) acceptance criteria met — `internal/tui` and `cmd` carry zero file-browser references; both packages reduced to zero importers before deletion.
- [x] Phase 3 (delete packages + docs + gate) acceptance criteria met — directories absent, README/CLAUDE reconciled, final gate green.
- [x] All 9 tasks completed and verified.
- [x] No scope creep — only the manifest's edit/delete sites were touched (plus the spec-sanctioned scope-add of the surface-audit allow-list keys, moved from task 2-1 to 3-1 because the audit reds if the key is removed before the on-disk directory).

### Code Quality

No issues found. The edits are idiomatic Go: clean import blocks with no orphaned imports, no dangling helpers, page constants compared symbolically throughout, and no leftover comments referencing the removed feature in the affected packages. The change is a net complexity reduction.

### Test Quality

Tests adequately verify requirements. As a pure-removal task no new tests were required (and none were added — correct). Deleted whole functions (`TestFileBrowserIntegration`, `TestFileBrowserFromProjectsPage`, `TestSpaceOnFileBrowserPageDoesNotCallNewPreviewModel`) are gone; the reworked survivors (`TestKillSession`, `TestNewWithFunctionalOptions` "all options combined", `TestCommandPendingNKey`) retain their non-browser assertions with the browser scaffolding stripped. `TestSurfaceAudit_NoNewPackageForPreview` remains coherent — it errors only on an on-disk dir absent from the allow-list, so removing the deleted dirs' keys keeps it green while clearing the dangling references. Surviving `cwd` plumbing is still exercised by `TestBuildTUIModel` "cwd wired correctly".

### Acceptance gate (independently re-run during review)

- `go build ./...` — **green** (exit 0).
- `go test ./...` — **green modulo a known-class flake**. `internal/restore`'s `TestPhase3Integration_RestoreUsesLiveIndicesUnderBaseIndexDrift` failed once under the full parallel suite with `tmux new-session … server exited unexpectedly`, then **passed in isolation** (`go test ./internal/restore/...` → ok, 0.698s). `internal/restore` was not modified by this removal; the symptom is the documented load-induced tmux-server flakiness class. Excused, not a regression.
- `internal/ui` and `internal/browser` confirmed absent on disk.
- Zero-reference grep across `*.go`, README.md, CLAUDE.md — clean (remaining hits live only under `.workflows/`/`.tick/`, expected and out of scope).
- Blocking manual check 1 (Projects-page `b` is a no-op) — source preconditions confirmed: no `b` binding in `projectHelpKeys`, no `isRuneKey(msg, "b")` dispatch, `b` falls through to `projectList.Update` (model.go:1617) opening no view. The live interactive confirmation is the lander's to run.
- Blocking manual check 2 (survivors functional) — source preconditions confirmed intact.

### Required Changes (if any)

None.

## Recommendations

### Do now

1. `internal/tui/switch_view_key_test.go:29` — reword the comment `// keyS is the browse-mode switch-view key.` (Report 2-1)
   - "browse-mode" is misleading: `keyS` (`'s'`) is the session-list grouping switch-view key (Flat / By Project / By Tag), unrelated to the removed file browser. Suggest `// keyS is the session-list grouping switch-view key.` Pre-existing wording outside this task's manifest; zero-risk doc-only clarity fix.
