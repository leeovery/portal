TASK: resume-hooks-silently-lost-4-6 — The New readFileBytes In internal/hooks Has An Un-Migrated Twin In Its Own File

ACCEPTANCE CRITERIA:
- The inline read at `internal/hooks/store_test.go:1170-1176` is gone, replaced by `readFileBytes`
- The comparison and its failure message are unchanged
- `store_shape_test.go` is untouched
- No test changes its verdict
- `go test ./internal/hooks/` passes

STATUS: complete

SPEC CONTEXT:
Phase 4 of this plan ("Removal and listing report the truth") makes `hooks.Store.Remove` report whether it
removed anything and makes `hook rm`'s exit code follow from that answer. Task 4-1 re-pointed
`internal/hooks/store_test.go` at byte-identity assertions ("hooks.json is byte-identical across every
no-removal call") and introduced the `readFileBytes` helper to serve them. 4-6 is the consolidation
follow-up inside that phase: the helper 4-1 introduced still had one hand-rolled twin in its own file.
It touches no production code and no specified behaviour, so the specification's substance is untouched by
it — its authority is its own body.

IMPLEMENTATION:
- Status: Implemented
- Location: commit db8002f6 ("Tresume-hooks-silently-lost-4-6 — the un-migrated readFileBytes twin"),
  `internal/hooks/store_test.go` — the only non-workflow file in the commit; 5 lines changed
  (four-line `os.ReadFile` + `t.Fatalf` block replaced by `after := readFileBytes(t, seeded)`).
- Notes:
  - Every acceptance criterion is met literally. The diff replaces exactly the named block; the
    `bytes.Equal(after, body)` comparison and its `"hooks.json changed on a failed save\nbefore: %s\nafter:  %s"`
    message are byte-identical across the commit.
  - `internal/hooks/store_shape_test.go` is absent from the commit's file list — the explicit
    not-in-scope guard held.
  - Verdict equivalence holds by reading: the removed block fatalled on a read error
    (`t.Fatalf("re-read hooks.json: %v", err)`) and `readFileBytes` fatals on the same condition
    (`internal/hooks/store_test.go:26-33`, then and now), so no path changes colour. The read-failure
    *message* differs, which is inherent to routing through a shared helper and is not the message the
    criterion pins (that criterion names the comparison's message, which is unchanged).
  - Type-compatibility is exact: `readFileBytes` returns `[]byte`, consumed by `bytes.Equal` and `%s`.
    Both the `bytes` and `os` imports still had other users in the file at the commit (`bytes.Equal` at
    the changed site, `os.ReadFile`/`os.Stat` in the two helpers), so no import went stale.
  - Drift since delivery, and it is sound: at HEAD this assertion has been consolidated a second time
    into the cross-package helper — `internal/hooks/store_test.go:1034` now reads
    `hookstest.AssertHooksFileUnchanged(t, seeded, []byte(body), "changed on a failed save")`, backed by
    `internal/hookstest/hooks.go:113-122` and `HooksFileBytes` at `:97-107`. The read-and-compare intent
    this task delivered is preserved, one level further out; nothing this task established was lost.
  - The one remaining inline `os.ReadFile` + `t.Fatalf` in the file (`internal/hooks/store_test.go:165-168`,
    the "file is empty" non-emptiness check) is not the twin the task names — it asserts a different
    property and was present before the task. Leaving it is what the Do list's scope says.

TESTS:
- Status: Adequate
- Coverage: This is a test-only substitution inside an existing subtest
  (`TestCleanStaleLogging` / "it emits no per-key lines and warns in the summary when the save fails"), so
  the correct test count is zero new tests — the plan says so, and adding one would test the standard
  library. The subtest that owns the assertion still proves what it proved: a `CleanStale` that fails its
  save leaves `hooks.json` byte-identical to the seeded body.
- Notes: Judged by reading rather than execution, as required. Nothing in the substitution can change a
  verdict — same read, same fatal-on-error class, same comparison, same message.

CODE QUALITY:
- Project conventions: Followed. Test-only change in the unit lane; no build tag, isolation, logging or
  tmux-boundary rule is engaged.
- SOLID principles: N/A at this size (a four-line-to-one-line helper substitution).
- Complexity: Low — the edit strictly reduces it.
- Modern idioms: Yes.
- Readability: Good. The call site now reads as the one thing it asserts.
- Comment accuracy: The helper's doc comment at the time of the commit ("returns the file's exact bytes, so
  a test can assert a no-op left the file untouched rather than rewritten to equivalent content") holds
  true for the new caller, which asserts exactly that about a failed save.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
