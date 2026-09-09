TASK: resume-hooks-silently-lost-4-4 — One Fake For The Pane-Token Enumeration (tick-5241be)

ACCEPTANCE CRITERIA:
- One `rows`/`err`/`calls` enumeration fake exists in package `cmd`, in `cmd/hookkey_vocabulary_test.go`
- `recordingHookKeyLister` embeds it and declares only `TryGetServerOption` and its restoring fields of its own
- `loudPaneHookLister` is untouched
- The `PaneHookLister` interface is unchanged
- No assertion changes and no test changes its verdict
- `go test ./...` and `go test -tags integration -p 1 ./cmd/...` pass

STATUS: complete

SPEC CONTEXT:
The spec (§4.4, and the §9 acceptance table row "`hook list` fourth column") gives `hook list` a fourth
tab-separated column carrying the token's resolved `<session>:<window>.<pane>` location, resolved through a
single `ListAllPaneHookKeys()` all-pane enumeration that "serves the sweep, `portal doctor` and `hook list`
alike — there is no second enumeration and no second tmux read" (spec:150). The location half is display-only
and never a key. This task touches none of that behaviour: it is a test-fixture consolidation removing the
second copy of the fake that answers that one enumeration seam. The spec's relevance is that the *seam* is
one, so the fake answering it should be one too.

IMPLEMENTATION:
- Status: Implemented (commit e6f27a50, "one fake for the pane-token enumeration"), later superseded in part
  by task 6-11 (commit ad9c7c0b) — see Notes.
- Location:
  - `cmd/hookkey_vocabulary_test.go:70-84` — `recordingPaneHookLister` (`rows` / `err` / `calls`), its
    `ListAllPaneHookKeys` method, and the `var _ PaneHookLister = (*recordingPaneHookLister)(nil)`
    compile-time assertion, sited in the file whose header declares itself the home of "the enumeration rows
    they arrive in, and the seam fakes that answer with them" (`cmd/hookkey_vocabulary_test.go:1-7`).
  - `cmd/hooks_test.go` — the duplicate `mockPaneHookLister` type is gone; its eleven construction sites now
    build `recordingPaneHookLister` (`:29`, `:91`, `:125`, `:141`, `:157`, `:177`, `:205`, `:223`, `:240`,
    `:259`, plus `cmd/hooks_read_lock_test.go:75` and `cmd/hooks_seams_test.go:41` since adopted it).
    `git grep mockPaneHookLister` at the task's own commit returns nothing — no dangling reference was left.
  - `cmd/hooks.go:21-25` — `PaneHookLister` is unchanged by this commit (`git show e6f27a50 --stat` lists
    only `.tick/tasks.jsonl`, the manifest, and three test files); it remains the one-method interface.
  - `cmd/hooks_test.go:272-280` — `loudPaneHookLister` was not in the task's diff and still fails on read.
  - At the task's commit `recordingHookKeyLister` embedded the plain fake and declared only
    `TryGetServerOption` plus `restoring`/`restoringErr`, exactly as the Do list specifies.
- Notes: Every acceptance criterion held as delivered. Two of them no longer read literally true at HEAD
  because task 6-11 (`ad9c7c0b`, "one sweep-seam fake") merged `recordingHookKeyLister` and
  `stubAllPaneLister` into a single `stubStaleSweepReader` (`cmd/hookkey_vocabulary_test.go:86-113`), which
  redeclares `rows`/`err`/`calls` rather than embedding, because its `during` hook has to run inside the
  enumeration between the count and the return. That is a later task's own decision with its own reasoning,
  judged in its own review; 4-4's substance survives intact — the plain fake is still one declaration shared
  by four files (`hooks_test.go`, `hooks_read_lock_test.go`, `hooks_seams_test.go`, and the vocabulary file
  itself), and no suite carries a private copy. Nothing this task delivered was lost.

TESTS:
- Status: Adequate (correctly: no new test).
- Coverage: The task's own judgement — "No new test — a fixture merge earns none" — is right; a fixture with
  no behaviour of its own has nothing to assert. Its coverage is the suites it feeds, and all three of its
  fields are exercised: `rows` at every site, `err` at `cmd/hooks_test.go:177-180` (the "rows alongside the
  error" case, which pins that a failed read is judged by the error and not by the rows), and `calls` at
  `cmd/hooks_test.go:250-268` ("it takes exactly one enumeration read for many entries" — the assertion that
  backs the spec's single-read claim).
- Notes: No verdict moved. The `hookKeyCalls` → `calls` rename in the sweep suite (`git show e6f27a50 --
  cmd/run_hook_stale_cleanup_test.go`) is identifier-only: every `want 0` / `want 1` and every message string
  is character-identical either side. Those assertions have since moved with the sweep into
  `internal/hooksweep/sweep_test.go:191,264,284,304,320,499`, still under the `calls` name and still carrying
  the same wording ("the sweep must stand down before enumerating"), so the rename cost no coverage.
  Not over-tested: nothing was added.

CODE QUALITY:
- Project conventions: Followed. Unit lane, no `t.Parallel()`, `*Deps` seams staged through `withHooksDeps`
  (the seam guard at `cmd/seam_guard_test.go` forbids direct assignment; the sites this task rewrote were
  later moved onto the helper and are on it now). The fake sits with the other seam fakes, as the file's own
  header contract requires.
- SOLID principles: Good. Interface segregation is the point of the change — the plain fake satisfies the
  one-method `PaneHookLister` and the sweep's fake adds only the second seam it needs, rather than every
  consumer carrying a fake wide enough for both.
- Complexity: Low. Two struct declarations and one method between them.
- Modern idioms: Yes. Embedding for the shared half, a `var _ I = (*T)(nil)` assertion so a drifted fake
  fails at compile time rather than at a construction site.
- Readability: Good. The doc comment on `recordingPaneHookLister` states what it answers with and why it
  counts, and the fake's name says what it is rather than how it was built.
- Issues: None. The embedded composite literals the merge produced at the sweep-test sites
  (`recordingHookKeyLister{recordingPaneHookLister: recordingPaneHookLister{rows: ...}}`) were the one
  legibility cost, and they no longer exist — 6-11 removed the outer type.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
