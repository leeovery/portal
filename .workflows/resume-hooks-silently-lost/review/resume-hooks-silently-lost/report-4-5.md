TASK: resume-hooks-silently-lost-4-5 — "The Hook Verb Drivers Get One Home, And Read Through The Reader The Package Already Has" (tick-c6cae7)

ACCEPTANCE CRITERIA:
1. `runHookSet`, `runHookRm` and `runHookList` all live in `cmd/testhelpers_test.go`, as do `seedHooksFile` and `assertHooksFileUnchanged`
2. Both helpers read through `readFileBytes`; no new `os.ReadFile` + `t.Fatalf` pair remains in either
3. Every `hooks …` alias block still drives the alias, not the canonical verb — alias coverage unchanged in count and in what it drives
4. The two helper-file headers cross-reference each other
5. No assertion changes and no test changes its verdict
6. `go test ./...` and `go test -tags integration -p 1 ./cmd/...` pass

STATUS: complete

SPEC CONTEXT: The specification's §9 (Testing) governs *coverage* — which behaviours get tests and in which lane — and says nothing about which test file a shared driver lives in. This task is a housekeeping consolidation raised by the implementation-analysis of phase 4, so its authority is its own body plus the repo convention it cites: `cmd/testhelpers_test.go`'s own header rule ("staging — how a test is set up and driven" lives here; subject vocabulary lives in a file named for its subject). The relevant spec-side constraint it must not damage is §4.2/§4.3 coverage of `hook rm`'s exit contract and §4.4's `hook list` fourth column, both of which are driven through the helpers being moved, plus the permanent `hooks` cobra alias (CLAUDE.md, "Resume-hook command"), whose back-compat coverage guard (a) protects.

IMPLEMENTATION:
- Status: Implemented (later superseded in one part, deliberately and soundly)
- Location: commit `4ba93d06` — `cmd/testhelpers_test.go`, `cmd/hooks_rm_exit_test.go`, `cmd/hookkey_vocabulary_test.go`. Current state: `cmd/testhelpers_test.go:176` (`runHookSet`), `:189` (`runHookRm`), `:202` (`runHookList`), `:230` (`assertHooksFileUnchanged`), `:130` (`readFileBytes`).
- Notes:
  - The move is verbatim: the diff relocates `runHookRm`, `seedHooksFile` and `assertHooksFileUnchanged` out of `cmd/hooks_rm_exit_test.go` into `cmd/testhelpers_test.go` with bodies unchanged apart from the reader re-point, and leaves no duplicate declaration (`git grep` at that commit finds exactly one of each in package `cmd`).
  - Import hygiene is correct at the commit: `bytes` is dropped from `cmd/hooks_rm_exit_test.go` (no remaining `bytes.` use there) while `os` is kept for the `os.Stat` at its `:66`; `bytes` was already imported by `cmd/testhelpers_test.go` for `runHookSet`.
  - Both helpers were re-pointed at the pre-existing `readFileBytes` (then at `cmd/bootstrap_production_test.go:130`) rather than re-declaring a reader — the Do list asked for a re-point, not a relocation, and the residual cross-file reach was recorded as a bank entry and closed later (`readFileBytes` now lives at `cmd/testhelpers_test.go:130` and delegates to `hookstest.HooksFileBytes`).
  - Divergence at HEAD, judged sound: `seedHooksFile` (and `writeHooksJSON`) no longer exist — commit `a01ef932` (task 8-27) replaced them with `hookstest.StageStore` via `hooksFileInTempDir(t, body)` (`cmd/testhelpers_test.go:166`), consolidating hooks.json staging behind the shared stager. That is a supersession of this task's placement decision by a later, broader one, not a loss: nothing this task delivered has gone unreplaced, and the "one home" property it was after still holds.
  - Second sound divergence: `assertHooksFileUnchanged` now takes a `context ...string` tail and delegates to `hookstest.AssertHooksFileUnchanged` (`cmd/testhelpers_test.go:230`) — exactly the follow-up this task's own bank entry predicted (bespoke per-site failure messages would otherwise be flattened).

TESTS:
- Status: Adequate (this is a test-refactor task; its acceptance is the unchanged suite plus the two guards)
- Coverage:
  - Guard (a) verified by enumeration rather than inspection: the commit does not touch `cmd/hooks_test.go` at all (`git show --stat 4ba93d06` lists `.tick/tasks.jsonl`, the manifest, `cmd/hookkey_vocabulary_test.go`, `cmd/hooks_rm_exit_test.go`, `cmd/testhelpers_test.go`). Counting across the commit: 29 `"hooks"` occurrences before and 29 after, of which 26 are `rootCmd.SetArgs([]string{"hooks", …})` alias drives — identical in count and in target. No alias block was re-pointed at a canonical-verb driver, which is the hazard the guard names.
  - Guard (b) verified in both directions: `cmd/testhelpers_test.go:1-4` names `hookkey_vocabulary_test.go`, and `cmd/hookkey_vocabulary_test.go:1-7` names `testhelpers_test.go`. Both cross-references survive at HEAD.
  - Verdict preservation: the only semantic change is the ENOENT nuance the Do list flagged. `seedHooksFile` reads immediately after `writeHooksJSON`, which itself fatals on a failed write, so the nil-on-ENOENT return is unreachable there; `assertHooksFileUnchanged` on a deleted file now fails through `bytes.Equal(before, nil)` → `t.Errorf` instead of fatalling on the read — same verdict, different message, exactly as the task predicted. The assertion text and comparison are byte-identical to the pre-move version.
- Notes: No new tests are warranted — the subject is where existing drivers live, and the suite that drives them is the observation. `runHookSetForKey` (`cmd/hooks_test.go:717`) stays in its suite file correctly: it is a seam-staging wrapper that calls the shared `runHookSet`, not a fourth driver.

CODE QUALITY:
- Project conventions: Followed. The placement matches `cmd/testhelpers_test.go`'s stated staging/vocabulary split; the `withHooksDeps`-style seam staging is untouched; no lane rule is engaged (unit-lane test files only, no binary built, no daemon spawned).
- SOLID principles: Good — each driver does one thing; `seedHooksFile`'s write/read split and the reader's single ENOENT rule stay separate concerns.
- Complexity: Low. Straight-line helpers, no branching added.
- Modern idioms: Yes — `append([]string{…}, extra...)` variadic composition, `t.Helper()` on every helper.
- Readability: Good. Both moved helpers keep their doc comments, and the comment on `seedHooksFile` ("so a caller can prove a failing route left the file untouched byte for byte") still described the code accurately at the commit.
- Issues: None. The three `runHook*` drivers share a buf/reset/SetArgs/Execute shape that could be folded into one parameterised driver, but they differ in stream handling and error contract, and a fold is a preference no standard here requires — not reported.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
