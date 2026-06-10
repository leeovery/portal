---
status: in-progress
created: 2026-06-10
cycle: 1
phase: Gap Analysis
topic: file-browser-alias-breadcrumb
---

# Review Tracking: file-browser-alias-breadcrumb - Gap Analysis

## Findings

### 1. `internal/ui` deletion lists only three files but does not assert "no other files"

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Removal Manifest → "Packages deleted entirely" (`internal/ui/` bullet)

**Details**:
The manifest deletes `internal/ui/` and names three files: `browser.go`, `browser_test.go`, `testmain_isolation_test.go`, with the assertion "Nothing but the file browser lives here." For a "delete the whole package" instruction, naming individual files is potentially misleading — an implementer who deletes only the three named files would be correct today, but the safe and unambiguous instruction is "`rm -rf internal/ui/` — the entire directory is the file browser." If a future file were added to the directory before this work lands (or if the implementer reads the file list as exhaustive-and-exclusive), a stray file could survive. The same applies to the `internal/browser/` bullet. This is a clarity/robustness gap, not a correctness error — the named file sets are accurate as of review.

**Proposed Addition**:
State the deletion as "remove the entire `internal/ui/` directory (and `internal/browser/` directory)" with the file list given as confirmation of current contents rather than as the deletion target. Optionally add to the acceptance gate: "the `internal/ui` and `internal/browser` directories no longer exist."

**Resolution**: Pending
**Notes**:

---

### 2. "Zero remaining references" acceptance gate omits several deleted exported symbols

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria & Testing → "Acceptance gate" (the zero-references bullet)

**Details**:
The zero-references gate enumerates a specific symbol list (`internal/ui`, `internal/browser`, `pageFileBrowser`, `DirLister`, `WithDirLister`, `osDirLister`, `mockDirLister`, `stubDirLister`, `handleBrowseKey`, `updateFileBrowser`, `b`/"browse"). The `internal/ui` package also exports symbols not on the list: `FileBrowserModel`, `NewFileBrowser`, `NewFileBrowserWithChecker`, `NewFileBrowserWithAlias`, `BrowserDirSelectedMsg`, `BrowserCancelMsg`, `BrowserDirSelectErrMsg`, `BrowserAliasSavedMsg`, `BrowserAliasSaveErrMsg`, `AliasSaver`, `GitRootResolver`, `PathChecker`, plus `browser.DirEntry`/`browser.ListDirectories`. In practice a clean `go build ./...` plus `go test ./...` (the other two gate lines) catches any surviving reference to these, so the gate is not unsound. But because the explicit symbol list reads as a checklist, an implementer grepping only the listed names could believe they are done while a reference to e.g. `BrowserDirSelectedMsg` or `FileBrowserModel` lingers in an un-rebuilt test path. Worth either completing the list or stating explicitly that the symbol list is illustrative and the compile+test pass is the authoritative gate.

**Proposed Addition**:
Either (a) append the remaining ui/browser exported symbols (`FileBrowserModel`, `NewFileBrowser`, `NewFileBrowserWithChecker`, `NewFileBrowserWithAlias`, `BrowserDirSelectedMsg`, `BrowserCancelMsg`, `BrowserDirSelectErrMsg`, `BrowserAliasSavedMsg`, `BrowserAliasSaveErrMsg`, `AliasSaver`, `GitRootResolver`, `PathChecker`, `browser.DirEntry`, `browser.ListDirectories`) to the zero-references list, or (b) reword the bullet so the green compile + green test is the authoritative gate and the symbol list is "spot-check grep targets, non-exhaustive."

**Resolution**: Pending
**Notes**:

---

### 3. `TestCommandPendingBrowseAndNKey` survivor rename is "optional" — leaves an implementer decision

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Removal Manifest → `internal/tui/model_test.go` → "Delete browser subtests" (`TestCommandPendingBrowseAndNKey` bullet)

**Details**:
The manifest says: after deleting the two browse subtests, the surviving function is n-key-only — "optional rename to `TestCommandPendingNKey`." "Optional" hands the implementer a naming decision the spec is meant to settle. Leaving a function named `TestCommandPendingBrowseAndNKey` that now contains only an n-key subtest is mildly misleading, but renaming changes a test identifier. Either choice is defensible; the spec should pick one so planning produces a deterministic task and reviewers know the expected end state. Minor.

**Proposed Addition**:
Replace "optional rename" with a definite instruction — either "rename the survivor to `TestCommandPendingNKey`" or "leave the function name as-is (the stale `Browse` in the name is acceptable; renaming is out of scope)."

**Resolution**: Pending
**Notes**:

---

### 4. Manual / spot-check acceptance steps have no defined pass criteria or executor

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria & Testing → "Acceptance gate" (manual check bullet) and "Testing requirements" (spot-check bullet)

**Details**:
Two acceptance items are manual: "the Projects page no longer reacts to `b` (pressing `b` does nothing browser-related)" and "Spot-check that the Sessions, Projects, and Preview pages, the alias CLI, and the projects-modal alias editor are unchanged and functional." The spec is explicit that this bugfix writes no new tests (net test delta is removal-only), so these manual checks are the *only* behavioural verification that the removal didn't break adjacent surfaces. Yet there's no defined expected behaviour for pressing `b` post-removal beyond "nothing browser-related." Today, `b` on the Projects page is a bound key (`projectHelpKeys`) that opens the browser; after removal the help binding is gone and the dispatch case is removed, so `b` will fall through to the default `projectList.Update(msg)` path. Whether `b` then does *nothing* vs. *types into / interacts with* the underlying list (e.g. filter, or no-op) is not stated — an implementer/verifier needs to know the intended post-removal behaviour to judge "pass." This affects the only behavioural gate the fix has.

**Proposed Addition**:
State the expected post-removal behaviour of `b` on the Projects page precisely (e.g. "`b` is no longer a recognised Projects-page command; it falls through to the default list handler and is a visible no-op / does not open any view"). Optionally note who runs the manual checks and that they are blocking for the acceptance gate.

**Resolution**: Pending
**Notes**:

---

### 5. Line-number drift policy is stated for the manifest but not extended to the "set is complete" guarantee under code change

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Removal Manifest (intro paragraph) and overall planning readiness

**Details**:
The manifest opens with "Line numbers are as of the investigation date — the implementation must re-confirm each, but the *set* of sites below is complete." This is the right caveat, but it guarantees set-completeness only relative to the codebase *at investigation time*. The spec gives the implementer a re-confirmation duty for line numbers but no equivalent instruction for re-confirming the *set* if the tree has moved on (e.g. a new test importing `internal/ui` added between investigation and implementation). For a removal whose whole correctness rests on "every importer is accounted for," the planning-ready instruction is to make the authoritative completeness check a fresh repo-wide grep at implementation time (`grep -rn "internal/ui\|internal/browser"` etc.), with the manifest as the expected baseline. Without this, an implementer could treat the listed set as immutable and miss a newly-added reference. Minor, since the compile+test gate would still catch a build break — but a dangling *doc* or comment reference would not break the build.

**Proposed Addition**:
Add an instruction that the implementer re-runs a repo-wide reference sweep (grep for `internal/ui`, `internal/browser`, `pageFileBrowser`, `DirLister`, `WithDirLister`, and the `b`/browse strings) at implementation start and reconciles any sites not in the manifest before deleting, treating the manifest as the verified baseline rather than a closed set.

**Resolution**: Pending
**Notes**:

---
