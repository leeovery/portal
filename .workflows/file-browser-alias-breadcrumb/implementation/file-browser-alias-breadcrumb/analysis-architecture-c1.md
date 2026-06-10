AGENT: architecture
CYCLE: 1
STATUS: clean
FINDINGS: none

SUMMARY: The file-browser removal left a coherent surviving surface — no dangling seams,
orphaned fields/Options/interfaces/messages, or page-state-machine inconsistencies.

Verified:
- Page state machine consistent: 4 surviving consts (PageLoading/PageSessions/PageProjects/
  pagePreview); pagePreview correctly renumbered 4→3. Update switch (model.go:1512) and View
  switch (:2210) cover Loading/Projects/pagePreview explicitly and Sessions via the
  pre-existing default: arm. No orphaned case pageFileBrowser:.
- No orphaned surface: zero references to dirLister/startPath/fileBrowser model fields,
  WithDirLister, the DirLister alias, osDirLister/mockDirLister/stubDirLister, or any
  Browser*Msg handler. tuiConfig.dirLister removed.
- cwd plumbing intact: field (model.go:183) → WithCWD Option (:534) → accessor (:434) →
  createSession(m.cwd) via createSessionInCWD (:2196); browser-independent. cmd/open.go
  retains cwd field, WithCWD(cfg.cwd), cwd: cwd literal.
- createSession chokepoint retained with exactly its 3 surviving callers.
- b-key seam clean: removed from updateProjectsPage switch; falls through to
  m.projectList.Update (:1616) — visible no-op opening no view (matches spec AC).
- helpKeys consistent: projectHelpKeys and commandPendingHelpKeys both dropped the b/"browse"
  binding; the latter's doc comment (:710) updated to match.
- Surface-audit guard still meaningful: preExistingPackages allow-list now exactly matches
  the 26 on-disk internal/ packages — no stale keys, no missing keys. Guard still pins the
  package set against scope creep.
- Docs reconciled; preview-test comments updated to drop stale ui/browser pointers.
- Gate green: go build ./... exit 0; go test ./... all ok.
