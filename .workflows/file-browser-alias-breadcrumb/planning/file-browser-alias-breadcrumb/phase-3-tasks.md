---
phase: 3
phase_name: Delete the Packages, Docs, and Verify the Gate
total: 3
---

## file-browser-alias-breadcrumb-3-1 | approved

### Task 1: Delete the internal/ui and internal/browser packages

**Problem**: The file-browser feature is being removed in full (the reported alias-audit bug sits on unreachable dead code; the user confirmed they never use the file browser and want it gone). After Phase 2, both `internal/ui` and `internal/browser` are zero-importer packages — every production consumer (`internal/tui`, `cmd/open.go`) and every test consumer has been stripped — but the two directories still sit physically on disk. They are pure dead weight now and must be deleted. The spec's load-bearing sequencing rule requires the two packages be deleted **last** ("remove the consumers first, then the packages"); Phase 2 satisfied the precondition, so the directories can now be removed with a clean compile.

**Solution**: Remove the entire `internal/ui/` and `internal/browser/` directories with `rm -rf` (whole directory, not just the named files), then confirm the tree still compiles with `go build ./...`. If any stray new importer was reintroduced after Phase 2, the build will catch it.

**Outcome**: The `internal/ui` and `internal/browser` directories no longer exist on disk; `go build ./...` is green with both packages gone, proving no compiled code anywhere in the tree still imports them.

**Do**:
- **Precondition (do not skip)**: Phase 2 (`file-browser-alias-breadcrumb-2-1` and `2-2`) must be complete — both `internal/ui` and `internal/browser` must be zero-importer. Confirm with a quick grep that no compiled Go file outside the two directories imports `github.com/leeovery/portal/internal/ui` or `github.com/leeovery/portal/internal/browser`. If any importer remains, stop — Phase 2 is incomplete and deletion would red the build.
- From the repo root `/Users/leeovery/Code/portal`, delete the **whole** `internal/ui/` directory: `rm -rf internal/ui/`. The current contents (browser.go, browser_test.go, testmain_isolation_test.go) are a confirmation of what was there at investigation time, **not** the deletion target — if the directory holds any file not in that list (e.g. one added between investigation and implementation), it is STILL part of the removal. Remove the directory in its entirety regardless of its exact file set.
- Delete the **whole** `internal/browser/` directory: `rm -rf internal/browser/`. Same rule — current contents (listing.go, listing_test.go, testmain_isolation_test.go) are confirmation only; delete the entire directory regardless of which files are present.
- Run `go build ./...` from the repo root and confirm it is green (exit 0). This is the catch for the edge case where a stray new importer was reintroduced after Phase 2 — the build would fail on the now-missing package. A green build proves the deletions left the tree compiling.
- Do NOT run the broader acceptance gate, docs reconciliation, or manual behavioural checks here — those are Task 3-2 (docs) and Task 3-3 (full gate). This task is the package deletion and a compile-only confirmation.

**Acceptance Criteria**:
- [ ] The `internal/ui/` directory no longer exists on disk (verified by `ls`/glob returning nothing for the path).
- [ ] The `internal/browser/` directory no longer exists on disk.
- [ ] Each directory was removed in its entirety (`rm -rf`), not just the files named in the investigation content list — any additional file present at deletion time was removed with the directory.
- [ ] `go build ./...` is green (exit 0) after both deletions, proving no compiled code still imports either package.
- [ ] No source file was edited in this task — the only changes are the two directory removals (Phase 2 already performed the consumer edits).

**Tests**:
- `"go build ./... is green with both internal/ui and internal/browser deleted"`
- `"the internal/ui directory no longer exists on disk after rm -rf"`
- `"the internal/browser directory no longer exists on disk after rm -rf"`
- `"the whole directory is removed even if it holds a file not in the investigation content list"`
- `"a stray new importer reintroduced after Phase 2 is caught by the post-deletion go build (build fails on the missing package)"`

**Edge Cases**:
- **Directory holds a file not in the investigation content list** — a file added between investigation and implementation does not change the action: `rm -rf` the whole directory regardless. The named file lists (ui: browser.go/browser_test.go/testmain_isolation_test.go; browser: listing.go/listing_test.go/testmain_isolation_test.go) are confirmation of contents, not the deletion target.
- **A stray new importer reintroduced after Phase 2** — if some code added a fresh import of `internal/ui` or `internal/browser` after Phase 2's consumer sweep, the post-deletion `go build ./...` fails on the missing package. The build is the safety net; a green build confirms no such importer exists.
- **Deletion must happen last in the sequence** — per the spec's sequencing rule, packages are deleted after consumers. Phase 2 removed all consumers, so deletion now compiles clean. If the precondition grep finds a surviving importer, deletion is premature — resolve it (it belongs in Phase 2) before deleting.

**Context**:
> Spec — Removal Manifest, "Packages deleted entirely": "Remove the **entire directory** in each case (`rm -rf`), not just the files named below. The file lists are a confirmation of current contents as of the investigation, not the deletion target — if the directory holds any file not listed (e.g. one added between investigation and implementation), it is still part of the removal."
> Spec — `internal/ui/`: "remove the whole directory. Current contents: `browser.go`, `browser_test.go`, `testmain_isolation_test.go`. Nothing but the file browser lives here. Sole importers are `internal/tui/model.go` (+ its test), both edited [in Phase 2]."
> Spec — `internal/browser/`: "remove the whole directory. Current contents: `listing.go`, `listing_test.go`, `testmain_isolation_test.go`. After `internal/ui` and `cmd/open.go`'s `osDirLister` are gone it has **zero** importers."
> Spec — "Sequencing": "delete the two packages **last** (or expect transient compile breaks) — remove the consumers first, then the packages." Phase 2 is the consumer removal; this task is the package deletion.
> This is a REMOVAL — no new tests, no TDD increment. The verification is the post-deletion `go build ./...` and the on-disk absence of both directories. The deleted packages take their own tests with them.

**Spec Reference**: `.workflows/file-browser-alias-breadcrumb/specification/file-browser-alias-breadcrumb/specification.md` — "Removal Manifest" ("Packages deleted entirely", `internal/ui/`, `internal/browser/`) and "Sequencing".

## file-browser-alias-breadcrumb-3-2 | approved

### Task 2: Update the non-Go docs (README.md, CLAUDE.md)

**Problem**: The file-browser removal is mostly enforced by the build/test gate, but the project's prose documentation is invisible to that gate. `README.md` and `CLAUDE.md` still describe the file browser as a live feature — a "four views" TUI claim in the README and three architecture rows in CLAUDE.md (the `tui` page-state-machine row, the `browser` package row, the `ui` package row). Left unreconciled, these become stale, misleading references to a feature that no longer exists. Because they are prose, no compiler or test will flag them — they must be reconciled BY HAND.

**Solution**: Edit `README.md` and `CLAUDE.md` to drop every file-browser reference, locating each edit site by its CONTENT (the line numbers may have drifted since investigation and since the Phase 1-2 edits), and additionally hand-grepping both files for any further stale file-browser prose beyond the four cited sites (the spec's Docs list is the verified baseline, not necessarily a closed set for prose).

**Outcome**: `README.md` and `CLAUDE.md` no longer mention the file browser anywhere — the README describes three TUI views, the CLAUDE.md `tui` page-state-machine drops `FileBrowser` and the `(peer of pageFileBrowser)` phrasing, and the `browser` and `ui` package-table rows are gone; a hand-grep of both files for file-browser tokens returns no surviving reference.

**Do**:
- **Locate by content, not bare line number.** The spec's cited line numbers (README L253; CLAUDE L48/L52/L60) are as of investigation and may have drifted since Phases 1-2. At HEAD they currently still hold, but confirm each edit site by matching the cited prose text, not the number.
- **`README.md`** — the line currently reading *"The TUI has four views: session list, project picker, file browser, and scrollback preview."* (spec cites L253): change it to describe **three** views by dropping "file browser" (and fixing the count word "four" → "three"). Result: *"The TUI has three views: session list, project picker, and scrollback preview."*
- **`CLAUDE.md` — `tui` package-table row** (spec cites L48): make two edits within this row:
  1. The page state machine `Loading → Sessions → Projects → FileBrowser → Preview` — drop `FileBrowser` so it reads `Loading → Sessions → Projects → Preview`.
  2. The phrasing "The `pagePreview` arm (peer of `pageFileBrowser`)" — remove the now-dangling `(peer of pageFileBrowser)` reference (e.g. reword to "The `pagePreview` arm renders a read-only scrollback preview…").
  - **Do NOT touch** the `pagePreview → pageSessions` transition mention later in the same row — that is a valid, surviving reference (it describes the preview dismiss handler, not the file browser).
- **`CLAUDE.md` — `browser` package-table row** (spec cites L52, currently `| \`browser\` | Directory listing with symlink detection |`): delete the entire row.
- **`CLAUDE.md` — `ui` package-table row** (spec cites L60, currently the `| \`ui\` | Shared Bubble Tea file-browser model (\`BrowserCancelMsg\`, \`BrowserDirSelectedMsg\`, \`DirLister\`) consumed by both \`internal/tui\` and \`cmd/open.go\` |` row): delete the entire row.
- **Hand-grep both files for residual references** beyond the four cited sites: search `README.md` and `CLAUDE.md` (case-insensitive) for `file browser`, `file-browser`, `FileBrowser`, `pageFileBrowser`, `internal/ui`, `internal/browser`, `DirLister`, `BrowserCancelMsg`, `BrowserDirSelectedMsg`, and a `browse` keybinding. CLAUDE.md is the project's living architecture doc, so it may carry stale file-browser prose the four-site list doesn't enumerate. Reconcile (edit/delete) any genuine file-browser reference the grep surfaces; leave unrelated matches (e.g. `browser` appearing in an unrelated sentence, or `DirLister` if it survives only as a deleted-symbol mention in a changelog you must not rewrite) alone with a noted reason.
- **No other non-Go files change.** Per the spec: `go.mod` / `go.sum` need no change (internal packages), and there are no `.goreleaser*`, `Makefile`, shell-completion, embed, or build-tag references (swept — none). Do not invent edits to these.
- Do NOT run the full acceptance gate or the manual behavioural checks here — those are Task 3-3. This task reconciles prose only. (A `go build ./...` is not required for a docs-only change, but is harmless if run.)

**Acceptance Criteria**:
- [ ] `README.md` describes **three** TUI views (session list, project picker, scrollback preview) — "file browser" is gone and the count word is corrected from "four" to "three".
- [ ] `CLAUDE.md` `tui` row page state machine reads `Loading → Sessions → Projects → Preview` (no `FileBrowser`), and the `(peer of pageFileBrowser)` phrasing is removed — while the valid `pagePreview → pageSessions` transition mention in the same row is left intact.
- [ ] The `CLAUDE.md` `browser` package-table row is deleted.
- [ ] The `CLAUDE.md` `ui` package-table row is deleted.
- [ ] A case-insensitive hand-grep of `README.md` and `CLAUDE.md` for `file browser` / `file-browser` / `FileBrowser` / `pageFileBrowser` / `internal/ui` / `internal/browser` / `DirLister` / `BrowserCancelMsg` / `BrowserDirSelectedMsg` / a `browse` keybinding returns no surviving genuine file-browser reference (any remaining match is justified as unrelated).
- [ ] Each edit site was located by matching its prose CONTENT, not by trusting a possibly-drifted bare line number.
- [ ] No edits were made to `go.mod`, `go.sum`, `.goreleaser*`, `Makefile`, shell-completion, embed, or build-tag files (the spec confirms none are needed).

**Tests**:
- `"README.md no longer mentions the file browser and states three TUI views, not four"`
- `"CLAUDE.md tui-row page state machine drops FileBrowser and the (peer of pageFileBrowser) phrasing"`
- `"the valid pagePreview → pageSessions transition mention in the CLAUDE.md tui row is preserved (not mistaken for a file-browser ref)"`
- `"the CLAUDE.md browser and ui package-table rows are both deleted"`
- `"a hand-grep of README.md and CLAUDE.md surfaces no surviving file-browser prose reference"`
- `"each edit site was found by content match, not by a stale line number that had drifted"`

**Edge Cases**:
- **Doc/prose references are invisible to the build/test gate** — these are prose-only sites that no compiler or test catches, so they MUST be reconciled by hand-grep, not relied on the gate. This is precisely why this task exists as a distinct deliverable.
- **Line numbers may have drifted** — since the investigation AND since the Phase 1-2 edits, the cited README L253 / CLAUDE L48/L52/L60 may no longer be exact. Locate each edit site by its CONTENT (the cited prose), not the bare line number. (At current HEAD the numbers happen to still hold, but do not trust that blindly.)
- **CLAUDE.md is the living architecture doc — the four-site list is a baseline, not a closed set** — the spec's Docs list is the verified baseline; fold in any other stale file-browser reference the hand-grep surfaces in either file beyond the four cited sites.
- **Surviving non-file-browser references that merely contain a matched token** — e.g. the `pagePreview → pageSessions` mention in the CLAUDE.md `tui` row, or the word "browser" in an unrelated sentence. Do not over-delete: only remove genuine file-browser references; leave valid prose intact with a noted reason.

**Context**:
> Spec — "Docs / non-Go":
> - `README.md:253` — *"The TUI has four views: session list, project picker, file browser, and scrollback preview."* → three views (drop "file browser").
> - `CLAUDE.md` L48 (`tui` row) — update the page state machine (`Loading → Sessions → Projects → FileBrowser → Preview` → drop FileBrowser) and the "`pagePreview` arm (peer of `pageFileBrowser`)" phrasing.
> - `CLAUDE.md` L52 — delete the `browser` package-table row.
> - `CLAUDE.md` L60 — delete the `ui` package-table row.
> - `go.mod` / `go.sum` — no change (internal packages).
> - No `.goreleaser*`, `Makefile`, shell-completion, embed, or build-tag references (swept — none).
> Spec — Removal Manifest: "Treat the manifest as the verified baseline, not a closed set: a reference newly added between investigation and implementation (especially a doc/comment reference, which the compile+test gate would not catch) must be caught here." The build/test gate does NOT catch stale doc/prose references — reconcile by hand-grep.
> This is a REMOVAL — no new tests. Verification is the post-edit hand-grep returning no surviving file-browser prose.

**Spec Reference**: `.workflows/file-browser-alias-breadcrumb/specification/file-browser-alias-breadcrumb/specification.md` — "Removal Manifest" ("Docs / non-Go").

## file-browser-alias-breadcrumb-3-3 | approved

### Task 3: Run the final acceptance gate (build/test + zero references + two blocking manual checks)

**Problem**: The whole removal must be verified end-to-end before it can be considered done. The spec defines a concrete acceptance gate: a green `go build ./...` and `go test ./...` (the authoritative catch for any surviving compiled/tested reference), a spot-check that no file-browser symbol survives, confirmation that the two deleted directories are gone, and — critically — two BLOCKING manual behavioural checks. Because this removal adds no new tests (net test delta is removal, not addition), those two manual checks are the fix's ONLY behavioural verification. Skipping them would land an unverified change.

**Solution**: Run the full acceptance gate against the post-deletion, post-docs tree — `go build ./...`, `go test ./...` (with the known-flaky `internal/tmux` kill-barrier test handled by isolated re-run), the zero-references spot-check grep, the on-disk directory-absence check, and the two blocking manual checks (Projects-page `b` is a visible no-op; Sessions/Projects/Preview pages + alias CLI + projects-modal alias editor unchanged) — and gate acceptance on all of them passing.

**Outcome**: `go build ./...` and `go test ./...` are green (the kill-barrier flake, if it fires, confirmed as the known flake by isolated re-run); the spot-check grep returns no compiled-code hit for any removed symbol and no surviving prose reference; the `internal/ui` and `internal/browser` directories are confirmed absent; and both blocking manual checks pass — Projects-page `b` opens no view, and the Sessions/Projects/Preview pages, alias CLI, and projects-modal alias editor are unchanged and functional.

**Do**:
- **Precondition**: Tasks 3-1 (package deletion) and 3-2 (docs) must be complete — this task verifies the end state after deletion + docs reconciliation. Run from the repo root `/Users/leeovery/Code/portal`.
- **Build gate**: run `go build ./...` and confirm green (exit 0).
- **Test gate**: run `go test ./...` and confirm green. Per CLAUDE.md, tests must not use `t.Parallel()` — run the full suite. **Known flake handling**: the `internal/tmux` kill-barrier timing test is load-flaky under the full `go test ./...` run (project memory `reference_flaky_killbarrier_test.md`). If it fails, re-run that package in isolation (`go test ./internal/tmux/...`); a pass-in-isolation confirms it is the known flake, not a removal-induced regression. Any OTHER failure is a genuine break that blocks acceptance.
- **Zero-references spot-check** (NON-EXHAUSTIVE grep target list — the authoritative gate is the green build+test above; this grep is a defence-in-depth spot-check, NOT a closed checklist): grep the working tree for each of `internal/ui`, `internal/browser`, `pageFileBrowser`, `DirLister`, `WithDirLister`, `osDirLister`, `mockDirLister`, `stubDirLister`, `handleBrowseKey`, `updateFileBrowser`, `FileBrowserModel`, `NewFileBrowser`, `NewFileBrowserWithChecker`, `NewFileBrowserWithAlias`, `BrowserDirSelectedMsg`, `BrowserCancelMsg`, `BrowserDirSelectErrMsg`, `BrowserAliasSavedMsg`, `BrowserAliasSaveErrMsg`, `AliasSaver`, `GitRootResolver`, `PathChecker`, `browser.DirEntry`, `browser.ListDirectories`, and a `b`/"browse" keybinding. Expect zero hits in compiled Go code and Go tests. A hit in a NON-COMPILED context (a doc comment, README/CLAUDE prose) that the build/test gate would miss must ALSO be reconciled — route it back to Task 3-2 (docs) or fix the surviving comment in place. Hits inside `.workflows/`, `.tick/`, or this planning/spec/investigation artifact tree are expected and out of scope (the spec/investigation/planning files naturally contain these tokens).
- **Directory-absence check**: confirm `internal/ui` and `internal/browser` no longer exist on disk (glob/`ls` returns nothing). This re-asserts Task 3-1's deletion held.
- **BLOCKING MANUAL CHECK 1 — Projects-page `b` is a visible no-op**: this is the fix's only behavioural verification on the TUI side. Launch the portal TUI, navigate to the Projects page, and press `b`. With the `projectHelpKeys` binding and the `case isRuneKey(msg, "b")` dispatch removed (Phase 2), `b` falls through to the default `projectList.Update(msg)` handler. Its expected post-removal behaviour is a visible NO-OP that opens NO view — it must NOT open the file browser or any other page. Confirm pressing `b` opens nothing. This check is BLOCKING; a failure (any page opening on `b`) gates acceptance.
- **BLOCKING MANUAL CHECK 2 — no regression in the surviving surfaces** (spot-check, blocking): confirm the Sessions, Projects, and Preview pages, the alias CLI (`portal alias set` / `portal alias rm` / `portal alias list`), and the projects-modal alias editor are unchanged and functional. Pass criterion: each behaves exactly as before the removal — no page fails to open, no command regresses. Exercise: open the Sessions page; open the Projects page; open the Preview page (Space on a session); run an alias set/list/rm round-trip via the CLI; open the projects edit modal and add/remove an alias (the `aliasEditor` → `SetAndSave` path). This check is BLOCKING.
- **These manual checks are run by whoever lands the change** and gate acceptance alongside the build/test pass. Record the outcome of each (pass/fail) as the task evidence.
- **Net test delta is REMOVAL, not addition** — do NOT add any new test. The deleted packages took their own tests with them; the gate is the existing green build+test plus the two manual checks.

**Acceptance Criteria**:
- [ ] `go build ./...` is green (exit 0).
- [ ] `go test ./...` is green — fully green, or green-modulo the documented known-flaky `internal/tmux` kill-barrier timing test (confirmed as the known flake by an isolated `go test ./internal/tmux/...` pass, not a removal regression). Any other failure blocks acceptance.
- [ ] The zero-references spot-check grep returns no hit for any listed symbol in compiled Go code or Go tests; any hit in a non-compiled context (doc comment / prose) is reconciled, not left standing; `.workflows`/`.tick`/spec/planning artifact hits are correctly treated as out of scope.
- [ ] The `internal/ui` and `internal/browser` directories are confirmed absent on disk.
- [ ] BLOCKING MANUAL CHECK 1 passes: on the Projects page, pressing `b` opens no view (visible no-op via fall-through to `projectList.Update(msg)`; does not open the file browser or any other page).
- [ ] BLOCKING MANUAL CHECK 2 passes: the Sessions, Projects, and Preview pages, the alias CLI (`portal alias set/rm/list`), and the projects-modal alias editor are unchanged and functional — no page fails to open, no command regresses.
- [ ] No new test was added (net test delta is removal).

**Tests**:
- `"go build ./... is green after deletion and docs reconciliation"`
- `"go test ./... is green (or green-modulo the known internal/tmux kill-barrier flake confirmed by isolated re-run)"`
- `"the spot-check grep returns zero compiled-code/test hits for the removed file-browser symbols"`
- `"a grep hit surviving in a non-compiled doc/prose context is reconciled rather than left standing"`
- `"the internal/ui and internal/browser directories are confirmed absent on disk"`
- `"Projects-page b opens no view — it is a visible no-op that does not open the file browser or any other page (blocking manual check)"`
- `"the Sessions, Projects, and Preview pages, the alias CLI, and the projects-modal alias editor are unchanged and functional (blocking manual check, no regression)"`

**Edge Cases**:
- **Grep hit surviving in a non-compiled doc/prose context the gate misses** — the build/test gate only catches compiled/tested references. A surviving doc comment or README/CLAUDE prose reference passes the gate silently, so the spot-check grep must explicitly cover non-compiled contexts; any such hit is reconciled (route to Task 3-2 or fix the comment), not waved through.
- **Projects-page `b` must fall through to a visible no-op opening no view** — the post-removal behaviour is the default `projectList.Update(msg)` handler doing nothing visible; it must NOT open the file browser or any other page. A failure here (any page opening) is a blocking gate failure.
- **No regression in Sessions/Projects/Preview pages, alias CLI, or projects-modal alias editor** — these are the named scope-boundary survivors. Each must behave exactly as before the removal; a page failing to open or a command regressing is a blocking gate failure.
- **Known-flaky kill-barrier test** — a failure in the `internal/tmux` kill-barrier timing test under the full run is the documented load-flake; re-run that package in isolation to confirm before treating it as a regression. A failure that does NOT reproduce in isolation is the known flake (excused); a failure anywhere else (or a kill-barrier failure that reproduces in isolation) is a genuine break that blocks acceptance.

**Context**:
> Spec — "Acceptance gate":
> - `go build ./...` is green. `go test ./...` is green.
> - "**Zero remaining references** to the removed feature. The authoritative gate is the green `go build ./...` + green `go test ./...` above — those catch any surviving reference in compiled or tested code. The following symbol list is a **non-exhaustive set of spot-check grep targets**, not a closed checklist: [the symbol list reproduced in the Do steps]. A grep hit in a non-compiled context (doc comment, prose) that the build/test gate would miss must also be reconciled."
> - "The `internal/ui` and `internal/browser` directories no longer exist."
> - "Manual check (blocking — this is the fix's only behavioural verification, since no new tests are added): on the Projects page, `b` is no longer a recognised command. With the `projectHelpKeys` binding and the `case isRuneKey(msg, "b")` dispatch removed, `b` falls through to the default `projectList.Update(msg)` handler — its expected post-removal behaviour is a visible **no-op that opens no view** (it must not open the file browser or any other page). Confirm pressing `b` opens nothing."
> Spec — "Testing requirements": "Spot-check (blocking) that the Sessions, Projects, and Preview pages, the alias CLI (`portal alias set/rm/list`), and the projects-modal alias editor are unchanged and functional. Pass criterion: each behaves exactly as before the removal — no page fails to open, no command regresses. These manual checks are run by whoever lands the change and gate acceptance alongside the build/test pass." Also: "Net test delta is **removal, not addition** — the deleted packages take their own tests with them; no new tests are required."
> Spec — "Risk": "easy to leave a dangling reference, so a clean compile + full-test pass is the gate."
> Project memory (`reference_flaky_killbarrier_test.md`) + CLAUDE.md note: the `internal/tmux` kill-barrier timing test is load-flaky under full `go test ./...`; re-run in isolation before treating it as a regression. CLAUDE.md also mandates tests must not use `t.Parallel()`.
> This task depends on Tasks 3-1 (deletion) and 3-2 (docs) — it verifies the end state after both. The two manual checks are BLOCKING and are the fix's only behavioural verification.

**Spec Reference**: `.workflows/file-browser-alias-breadcrumb/specification/file-browser-alias-breadcrumb/specification.md` — "Acceptance Criteria & Testing" ("Acceptance gate", "Testing requirements", "Risk").
