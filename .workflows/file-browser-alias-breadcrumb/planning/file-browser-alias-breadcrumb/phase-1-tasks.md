---
phase: 1
phase_name: Re-sweep and Reconcile the Removal Manifest
total: 4
---

## file-browser-alias-breadcrumb-1-1 | approved

### Task 1: Run the mandated repo-wide reference sweep for all required tokens

**Problem**: The Removal Manifest's line numbers and site set are "as of the investigation date" — the spec explicitly mandates a fresh repo-wide reference sweep at implementation start before any deletion, because a reference added between investigation and implementation (especially a doc/comment reference invisible to the compile+test gate) would otherwise slip through. The deletion phases (2 and 3) cannot safely proceed without a current, complete hit set.

**Solution**: Execute the exact sweep the spec mandates — grep the working tree for every required token, including the bare quoted package-name tokens that path-prefixed greps miss — and capture the full raw hit set (file:line:content) as the documented input for Task 1-2's reconciliation.

**Outcome**: A complete, documented hit set covering every required token, with each hit recorded as `file:line` plus its surrounding context, ready to be reconciled against the manifest. No code is edited or deleted in this task.

**Do**:
- Run a repo-wide grep (working directory root: `/Users/leeovery/Code/portal`) for each of these tokens and record every hit as `file:line:content`:
  - `internal/ui`
  - `internal/browser`
  - `pageFileBrowser`
  - `DirLister`
  - `WithDirLister`
  - The `b` / `"browse"` keybinding strings (search for `"browse"` and the `isRuneKey(msg, "b")` dispatch form).
- Additionally — and this is the load-bearing extra step the spec calls out — grep for the **bare quoted tokens** `"ui"` and `"browser"` (with the literal double-quotes). These are the allow-list / map-key references (e.g. the surface-audit `preExistingPackages` map in `internal/tui/pagepreview_surface_audit_test.go`) that use the bare package name and which the path-prefixed `internal/ui` / `internal/browser` greps cannot see.
- Also sweep the wider symbol set named in the acceptance gate so the hit set is a superset, not a subset, of the manifest: `osDirLister`, `mockDirLister`, `stubDirLister`, `handleBrowseKey`, `updateFileBrowser`, `FileBrowserModel`, `NewFileBrowser`, `NewFileBrowserWithChecker`, `NewFileBrowserWithAlias`, `BrowserDirSelectedMsg`, `BrowserCancelMsg`, `BrowserDirSelectErrMsg`, `BrowserAliasSavedMsg`, `BrowserAliasSaveErrMsg`, `AliasSaver`, `GitRootResolver`, `PathChecker`, `browser.DirEntry`, `browser.ListDirectories`.
- For each hit, classify its **context type** at capture time so Task 1-2 can reconcile efficiently: compiled Go code, Go test code, doc comment / prose inside a Go file, Markdown doc (`README.md`, `CLAUDE.md`), or non-code artifact (anything under `.workflows/` or `.tick/`). Do NOT yet decide disposition (in-manifest / new / false-positive) — that is Task 1-2's job; this task only produces the raw, fully-captured hit set.
- Record the hit set as the task's evidence artifact (e.g. in the task's completion notes / a scratch record consumed by Task 1-2). Do not write a standalone summary `.md` report.

**Acceptance Criteria**:
- [ ] Every token in the mandated sweep list (the six core tokens plus the bare `"ui"` / `"browser"` quoted tokens plus the wider acceptance-gate symbol set) has been grepped against the working tree and its results captured.
- [ ] The bare quoted `"ui"` and `"browser"` greps were run explicitly and their hits captured — including the surface-audit `preExistingPackages` map keys, which the path-prefixed greps would otherwise miss.
- [ ] Each captured hit records `file:line:content` and a context-type tag (compiled code / test code / Go doc-comment / Markdown doc / `.workflows`-or-`.tick` artifact).
- [ ] The hit set is complete enough that Task 1-2 can reconcile every site without re-running the sweep.

**Tests**:
- `"it captures every hit for each of the six mandated core tokens"`
- `"it captures the bare quoted \"ui\" / \"browser\" hits that path-prefixed greps miss (surface-audit preExistingPackages keys)"`
- `"it tags doc-comment and prose hits in non-compiled contexts so the compile gate's blind spots are visible"`
- `"it tags .workflows / .tick artifact hits as non-code sites distinct from real code sites"`

**Edge Cases**:
- Bare `"ui"` / `"browser"` tokens missed by path-prefixed greps — the explicit bare-quoted grep is mandatory, not optional. The spec calls out the `preExistingPackages` allow-list keys at `internal/tui/pagepreview_surface_audit_test.go` L295/L321 specifically.
- Doc/comment hits in non-compiled contexts (Go doc-comments, Markdown prose) — these will not be caught by the build/test gate, so they must be captured here even though they do not break compilation.
- `.workflows/` and `.tick/` artifact hits (the specification, investigation, and planning files themselves contain these tokens) — capture them but tag them as non-code so Task 1-2 dispositions them as out-of-scope artifacts, not edit sites.

**Context**:
> Spec — Removal Manifest, "Re-sweep at implementation start (required)": "Before deleting anything, run a fresh repo-wide reference sweep — grep for `internal/ui`, `internal/browser`, `pageFileBrowser`, `DirLister`, `WithDirLister`, and the `b`/'browse' strings — **and also for the bare quoted tokens `\"ui\"` and `\"browser\"`** (allow-list / map-key references such as the surface-audit `preExistingPackages` map use the bare package name, which the path-prefixed `internal/ui` grep misses)."
> The acceptance gate (spec "Acceptance gate") gives the wider non-exhaustive symbol list reproduced in the Do steps; sweeping the superset here de-risks Phases 2-3.

**Spec Reference**: `.workflows/file-browser-alias-breadcrumb/specification/file-browser-alias-breadcrumb/specification.md` — "Removal Manifest" (Re-sweep at implementation start) and "Acceptance gate".

## file-browser-alias-breadcrumb-1-2 | approved

### Task 2: Reconcile each sweep hit against the Removal Manifest and record dispositions

**Problem**: The sweep from Task 1-1 produces a raw hit set, but raw hits are not actionable — each must be classified as either already-in-the-manifest (a known edit/delete site), newly-added-since-investigation (must be reconciled into the edit set), a false positive (unrelated package, incidental string), or an explicit scope-boundary site that must NOT be touched. Without this disposition record, the deletion phases risk both missing a new reference and accidentally deleting a survivor.

**Solution**: Walk every hit from Task 1-1, match it against the manifest's enumerated edit/delete sites, and assign a per-hit disposition. Flag any hit not in the manifest as a reconciliation item, and explicitly mark the spec's named survivors so Phases 2-3 cannot touch them.

**Outcome**: A per-hit disposition record covering every Task 1-1 hit, where each hit is labelled one of: `in-manifest` (with the manifest site it maps to), `new-reconciled` (not in manifest — added since investigation, now folded into the edit set with a chosen disposition), `false-positive` (unrelated, no action), or `scope-boundary-keep` (a named survivor that MUST NOT be edited).

**Do**:
- For each hit from Task 1-1, attempt to map it to a manifest site. The manifest enumerates edit sites in: `internal/tui/model.go`, `cmd/open.go`, `internal/tui/model_test.go`, `cmd/open_test.go`, the preview tests (`internal/tui/pagepreview_entry_test.go`, `pagepreview_refetch_test.go`, `pagepreview_bracket_test.go`, `pagepreview_surface_audit_test.go`), and Docs (`README.md`, `CLAUDE.md`). Whole-directory deletions: `internal/ui/`, `internal/browser/`.
- Label each mapped hit `in-manifest` and record which manifest bullet it corresponds to.
- For any hit **not** in the manifest, treat it as a reconciliation item: determine whether it is a genuine new reference (added between investigation and implementation) or a false positive. Per the spec, doc/comment references newly added are the highest-risk class because the compile+test gate would not catch them. Record a disposition: `new-reconciled` (with the edit action it now requires) or `false-positive` (with the reason it is unrelated).
- Explicitly mark the spec's named **scope-boundary survivors** as `scope-boundary-keep` so the deletion phases have a do-not-touch list:
  - The alias CLI (`cmd/alias.go`; `portal alias set/rm/list`).
  - The projects-modal alias editor (`internal/tui/model.go` `aliasEditor` → `SetAndSave`).
  - `createSession` — survives (3 non-browser callers: project-enter, `createSessionInCWD`); only the browser→create-session entry point is removed.
  - `cfg.cwd` / `m.cwd` — consumed by `WithCWD` and `viewCWD` / `createSession(m.cwd)`, independent of the browser. `WithCWD(cfg.cwd)` (open.go L371) and `cwd: cwd` (open.go L505) STAY.
  - The resolver chain, and the Sessions / Projects / Preview pages.
- Tag every `.workflows/` and `.tick/` hit as `false-positive` (non-code artifact — the spec/investigation/planning files naturally contain these tokens and are not edit sites).
- Produce the disposition record as the task evidence artifact consumed by Task 1-3. Do not edit any code.

**Acceptance Criteria**:
- [ ] Every hit from Task 1-1 has exactly one disposition: `in-manifest`, `new-reconciled`, `false-positive`, or `scope-boundary-keep`.
- [ ] Every `in-manifest` hit names the specific manifest bullet it maps to.
- [ ] Any hit not present in the manifest is explicitly reconciled — recorded as either a genuine new reference folded into the edit set, or a justified false positive.
- [ ] All named scope-boundary survivors (alias CLI, projects alias editor, `createSession`, `cfg.cwd`/`m.cwd` with `WithCWD`/`cwd: cwd`) are listed as `scope-boundary-keep` and flagged do-not-touch for Phases 2-3.
- [ ] Doc/comment hits in non-compiled contexts are dispositioned (not silently dropped) since the compile gate would not catch a surviving one.

**Tests**:
- `"it maps every manifest-listed hit to its manifest bullet"`
- `"it flags a reference newly added since investigation as new-reconciled rather than dropping it"`
- `"it flags a doc/comment reference (invisible to the compile gate) as a real reconciliation item"`
- `"it marks unrelated-package and incidental-string hits as false-positive with a reason"`
- `"it lists the alias CLI, projects alias editor, createSession, and cfg.cwd/m.cwd as scope-boundary-keep do-not-touch sites"`

**Edge Cases**:
- References newly added since the investigation date — especially doc/comment refs invisible to the compile gate. These are the spec's explicitly-called-out highest risk; they must be caught here, not at compile time.
- False positives in unrelated packages or strings — e.g. a `"ui"` substring inside an unrelated identifier, or `b` keybindings on non-Projects pages. Disposition as `false-positive` with the reason, do not edit.
- Scope-boundary sites that must NOT be touched — the alias CLI, the projects-modal alias editor (`aliasEditor` → `SetAndSave`), `createSession`, and `cfg.cwd`/`m.cwd`. The `WithCWD(cfg.cwd)` opt (open.go L371) and `cwd: cwd` cfg field (open.go L505) explicitly stay even though `WithDirLister`/`dirLister` adjacent to them are removed.

**Context**:
> Spec — "Scope boundary — what must stay green and unchanged": the alias CLI, the projects-modal alias editor (`aliasEditor` → `SetAndSave`), the resolver chain, the Sessions/Projects/Preview pages, `createSession` (3 non-browser callers), and `cfg.cwd` / `m.cwd`.
> Spec — Removal Manifest: "Reconcile any site **not** in this manifest before proceeding. Treat the manifest as the verified baseline, not a closed set: a reference newly added between investigation and implementation (especially a doc/comment reference, which the compile+test gate would not catch) must be caught here."
> The full enumerated manifest edit-site list (model.go, open.go, model_test.go, open_test.go, the four pagepreview tests, README.md, CLAUDE.md) is the matching key for `in-manifest` dispositions.

**Spec Reference**: `.workflows/file-browser-alias-breadcrumb/specification/file-browser-alias-breadcrumb/specification.md` — "Removal Manifest" and "Scope boundary — what must stay green and unchanged".

## file-browser-alias-breadcrumb-1-3 | approved

### Task 3: Re-confirm manifest line numbers against HEAD and produce the corrected edit set

**Problem**: The manifest's line numbers are "as of the investigation date" and the spec mandates the implementation re-confirm each one — line drift since investigation will silently mis-target a deletion (e.g. removing the wrong `case` arm or import line) if the deletion phases trust the stale numbers. Phases 2-3 need a corrected, HEAD-accurate edit set to execute against.

**Solution**: For every manifest edit site (across `model.go`, `cmd/open.go`, `model_test.go`, `cmd/open_test.go`, the four pagepreview tests, and the docs), open the file at HEAD and re-confirm the cited symbol/line, recording the corrected line number where it has drifted, and capturing the special-case details (the rename target, the allow-list keys) so the deletion phases consume an exact, HEAD-accurate edit set.

**Outcome**: A corrected edit set — one entry per manifest site — where each entry pairs the manifest's described edit with its HEAD-confirmed (and corrected, where drifted) line number, plus explicit notes for the two special cases (the `TestCommandPendingBrowseAndNKey` → `TestCommandPendingNKey` rename, and the surface-audit allow-list `"browser"`/`"ui"` keys).

**Do**:
- For each manifest edit site, open the target file at HEAD and locate the cited symbol/construct. Record `manifest-cited-line → HEAD-confirmed-line` (mark "unchanged" or "drifted to N"). Cover all sites:
  - `internal/tui/model.go`: the `internal/ui` import (~L20), `pageFileBrowser` const+comment (~L33-34), `DirLister` alias (~L119-120), fields `dirLister`/`startPath`/`fileBrowser` (~L189/190/194), `WithDirLister` Option (~L574-580), `projectHelpKeys` `b` binding (~L721), `commandPendingHelpKeys` doc comment (~L728-729), `commandPendingHelpKeys` `b` binding (~L732), `BrowserDirSelectedMsg`/`BrowserCancelMsg` handlers (~L1363-1366), `case pageFileBrowser:` update arm (~L1544-1545), `case isRuneKey(msg, "b"):` dispatch (~L1637-1638), `handleBrowseKey()` (~L1649-1656), `updateFileBrowser()` (~L1971-1977), `case pageFileBrowser:` view arm (~L2269-2270).
  - `cmd/open.go`: `internal/browser` import (~L11), `osDirLister` type+method (~L332-338), `dirLister` field (~L349), `WithDirLister` opt (~L370 — keep L371 `WithCWD`), cfg literal `dirLister` (~L505 — keep `cwd: cwd`).
  - `internal/tui/model_test.go`: imports (~L13/L17), `mockDirLister` (~L775-785), whole functions `TestFileBrowserIntegration` (~L787-1034) and `TestFileBrowserFromProjectsPage` (~L5551-5758), and the browser subtests in `TestCommandPendingMode` (~L2371-2414), `TestNewWithFunctionalOptions` "WithDirLister enables file browser" (~L3031-3066), `TestCommandPendingBrowseAndNKey` (~L6737-6829), `TestCommandPendingEscAndQuit` "Esc in file browser" (~L7182-7229), plus the reworks in `TestKillSession` (~L1671/1673) and `TestNewWithFunctionalOptions` "all options combined" (~L3080-3092).
  - `cmd/open_test.go`: `internal/browser` import (~L18), `stubDirLister` (~L703-708), `dirLister` cfg literal (~L773 — keep `cwd`).
  - Preview tests: `pagepreview_entry_test.go` (`TestSpaceOnFileBrowserPageDoesNotCallNewPreviewModel` ~L264-286; header comment ~L12), `pagepreview_refetch_test.go` (comment ~L27), `pagepreview_bracket_test.go` (comment ~L14), `pagepreview_surface_audit_test.go` (`preExistingPackages` keys `"browser":` ~L295 and `"ui":` ~L321).
  - Docs: `README.md:253`, `CLAUDE.md` L48/L52/L60.
- For the **rename special case**, confirm `TestCommandPendingBrowseAndNKey` exists at HEAD (Task 1-1's sweep already located it in `internal/tui/model_test.go`), confirm both browser subtests ("browse directory selection forwards command…" and "browse cancel returns to locked Projects page…") are present for deletion, and record the decided end-state: the surviving n-key-only function is renamed to `TestCommandPendingNKey`. This rename is the decided end-state, not optional.
- For the **allow-list special case**, open `internal/tui/pagepreview_surface_audit_test.go` and confirm the `preExistingPackages` map (block begins ~L292) contains the `"browser":` and `"ui":` keys; record their HEAD-confirmed lines. Note explicitly that the build/test gate does NOT catch a stale key left here (the audit only errors on a directory present on disk but absent from the allow-list), so these must be removed by hand in Phase 2/3 — and that the bare `"ui"` key escapes the path-prefixed re-sweep greps (caught only by Task 1-1's bare-quoted grep).
- Fold in any `new-reconciled` items from Task 1-2 as additional corrected-edit-set entries.
- Produce the corrected edit set as the task evidence artifact consumed by Phases 2-3. Do not edit any code.

**Acceptance Criteria**:
- [ ] Every manifest edit site has a corrected-edit-set entry pairing the described edit with its HEAD-confirmed line number (marked unchanged or corrected).
- [ ] Line drift since the investigation date is detected and the corrected line recorded — no entry relies on a stale, unverified line number.
- [ ] The `TestCommandPendingBrowseAndNKey` → `TestCommandPendingNKey` rename is recorded as a confirmed, mandatory end-state with both browser subtests confirmed present for deletion and the n-key survivor identified.
- [ ] The surface-audit `preExistingPackages` `"browser":` (~L295) and `"ui":` (~L321) keys are confirmed at HEAD with a note that the build/test gate does not catch a stale key, so they require explicit hand-removal.
- [ ] Any `new-reconciled` item from Task 1-2 is included as a corrected-edit-set entry.

**Tests**:
- `"it records the HEAD-confirmed line for every manifest edit site (unchanged or drifted)"`
- `"it detects and corrects line drift since the investigation date"`
- `"it confirms the TestCommandPendingBrowseAndNKey rename target as TestCommandPendingNKey with both browser subtests marked for deletion"`
- `"it confirms the surface-audit allow-list keys at L295/L321 and notes they escape the build/test gate"`
- `"it folds new-reconciled hits from reconciliation into the corrected edit set"`

**Edge Cases**:
- Line drift since the investigation date — any cited line may have moved; every number must be re-confirmed against HEAD, not trusted.
- The `TestCommandPendingBrowseAndNKey` → `TestCommandPendingNKey` rename target — the survivor is n-key-only after both browse subtests are deleted; the rename is the decided end-state, not optional.
- Surface-audit allow-list keys at L295/L321 — the `"browser":`/`"ui":` map keys are dangling references after package deletion that the build/test gate will NOT flag (the audit only errors on an on-disk directory missing from the allow-list, not on an extra key), so they must be removed explicitly; the bare `"ui"` key additionally escapes path-prefixed greps.

**Context**:
> Spec — Removal Manifest: "Line numbers are as of the investigation date — the implementation must re-confirm each, but the *set* of sites below is complete as of that date."
> Spec — `internal/tui/model_test.go`: "`TestCommandPendingBrowseAndNKey` → ... Survivor is n-key-only — **rename the survivor to `TestCommandPendingNKey`** (the `Browse` in the old name is now stale; this is the decided end-state, not optional)."
> Spec — preview tests: the `preExistingPackages` allow-list keys `"browser":` (L295) and `"ui":` (L321) "name the two deleted packages; after removal they are dangling references... **The build/test gate does not catch this** ... **The bare `\"ui\"` key also escapes the path-prefixed re-sweep greps**."
> Spec — "Iota-safety + dangling-reference verification": removing `pageFileBrowser` and letting `pagePreview` renumber 4→3 is transparent (no int↔page cast / serialization); `m.startPath` / `m.dirLister` are read only inside removed sites; `cfg.cwd` / `m.cwd` MUST stay.

**Spec Reference**: `.workflows/file-browser-alias-breadcrumb/specification/file-browser-alias-breadcrumb/specification.md` — "Removal Manifest" (all edit-site subsections) and "Iota-safety + dangling-reference verification".

## file-browser-alias-breadcrumb-1-4 | approved

### Task 4: Confirm the green go build + go test baseline on unchanged HEAD

**Problem**: The acceptance gate for the whole removal is "green `go build ./...` + green `go test ./...`". To attribute any post-deletion failure correctly, Phases 2-3 need a confirmed baseline on **unchanged** HEAD — without it, a pre-existing flaky test could be misread as removal-induced breakage (or worse, a genuine break could be excused as "pre-existing"). The known load-flaky kill-barrier timing test makes this distinction essential.

**Solution**: Run `go build ./...` and `go test ./...` on the current, unmodified HEAD, capture the result, and explicitly characterise any failure as either a genuine baseline break (which blocks Phase 2 until resolved) or a known pre-existing flake (which is recorded and excused), so the deletion phases inherit an unambiguous green/known-flaky baseline.

**Outcome**: A baseline confirmation record showing `go build ./...` green and `go test ./...` either fully green or green-modulo-a-documented-known-flake, with the flaky kill-barrier timing test explicitly distinguished from any genuine baseline breakage.

**Do**:
- On the current unmodified HEAD (make no edits), run `go build ./...` from `/Users/leeovery/Code/portal` and record the result (expected: green / exit 0).
- Run `go test ./...` from the repo root and record the result (per CLAUDE.md, tests must not use `t.Parallel()`; run the full suite).
- If `go test ./...` reports a failure, classify it:
  - The `internal/tmux` kill-barrier timing test is **known load-flaky** under the full `go test ./...` run (per project memory `reference_flaky_killbarrier_test.md` and the `internal/tmux` kill-barrier note). If it fails, re-run that package in isolation (`go test ./internal/tmux/...`) to confirm it passes alone — a pass-in-isolation confirms it is the known flake, not a baseline break. Record it as `known-flake`.
  - Any other failure is a `genuine-baseline-break` and must be flagged as blocking Phase 2 (deletion cannot start against a red baseline, because post-deletion failure attribution would be impossible).
- Produce the baseline confirmation as the task evidence artifact (build result, test result, and the classification of any failure). Do not edit any code.

**Acceptance Criteria**:
- [ ] `go build ./...` on unchanged HEAD is recorded as green (exit 0).
- [ ] `go test ./...` on unchanged HEAD is recorded with its full result.
- [ ] Any test failure is classified as either `known-flake` (the load-flaky `internal/tmux` kill-barrier timing test, confirmed by passing in isolation) or `genuine-baseline-break`.
- [ ] If a `genuine-baseline-break` is found, it is flagged as blocking Phase 2; if only the known flake is present, the baseline is recorded as green-modulo-known-flake and Phase 2 is cleared to proceed.
- [ ] No code was edited — the baseline reflects unmodified HEAD.

**Tests**:
- `"it confirms go build ./... is green on unchanged HEAD"`
- `"it confirms go test ./... is green (or green-modulo-the-documented-known-flake) on unchanged HEAD"`
- `"it re-runs internal/tmux in isolation to confirm a kill-barrier failure is the known flake, not a baseline break"`
- `"it flags any non-flake failure as a genuine-baseline-break that blocks Phase 2"`

**Edge Cases**:
- Pre-existing flaky tests distinguished from genuine baseline breakage — the `internal/tmux` kill-barrier timing test is load-flaky under the full `./...` run; a failure there must be re-confirmed by an isolated package run before being excused as the known flake. A failure anywhere else is a genuine break and blocks the deletion phases.

**Context**:
> Spec — "Acceptance gate": "`go build ./...` is green. `go test ./...` is green." This baseline confirmation establishes the pre-deletion green state the gate is measured against.
> Spec — "Risk": "easy to leave a dangling reference, so a clean compile + full-test pass is the gate." A trustworthy gate requires a trustworthy baseline.
> Project memory (`reference_flaky_killbarrier_test.md`) and CLAUDE.md note: the `internal/tmux` kill-barrier timing test is load-flaky under full `go test ./...`; re-run in isolation before treating it as a regression. CLAUDE.md also mandates tests must not use `t.Parallel()`.

**Spec Reference**: `.workflows/file-browser-alias-breadcrumb/specification/file-browser-alias-breadcrumb/specification.md` — "Acceptance gate" and "Risk".
