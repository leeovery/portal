TASK: resume-hooks-silently-lost-9-29 (tick-2750b0) — The Failed-Write Tail Is Hand-Rolled In The cmd Migrate Suite In A Weaker Form Than The Shared Helper Enforces

ACCEPTANCE CRITERIA:
- Both migrate WARNs carry an `error` attr wrapping the named `fileutil` write-phase sentinel.
- Both assertions route through `AssertWriteFailure` and no longer use `HasAttr` for the error.
- A migrate error misclassified against its sentinel now fails the test.
- The rendered `error_class` tokens are unchanged.
- `internal/storelog/clean_stale_test.go` is byte-unchanged.

STATUS: complete

SPEC CONTEXT: This is a phase 9 implementation-analysis (duplication) task, not a spec-derived bugfix task — per the shared verifier context, its authority is its own body. The substance sits in the tree-wide failed-write breadcrumb contract described in CLAUDE.md: every store mutation that fails a write emits a WARN carrying `error` plus an `error_class` token from `fileutil.ClassifyWriteError`, and `logtest.AssertWriteFailure(t, rec, wantClass, sentinel)` is the shared assertion pinning that pair (the sentinel is a parameter so `logtest` takes no `internal/fileutil` edge — pinned by `internal/logtest/leaf_guard_test.go`).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/config.go:29-30 (temp-create branch), cmd/config.go:37-38 (rename branch); cmd/config_migrate_logging_test.go:30-47 (`denyRenameFixture`), :159-186, :188-215, :217-229, :231-243. Commit c1542d9b.
- Notes:
  - The MkdirAll failure is wrapped `fmt.Errorf("%w: failed to create directory: %w", fileutil.ErrWriteTempCreate, err)` and the rename failure `fmt.Errorf("%w: failed to rename: %w", fileutil.ErrWriteRename, err)`. The temp-create classification of a directory-create failure is correct rather than a stretch: `internal/fileutil/atomic.go:13-15` states `ErrWriteTempCreate` covers creating the parent directory as well as the temp file, and `atomic.go:57` wraps its own MkdirAll failure with the identical verb ("failed to create directory"), so the migrate path now mirrors the canonical writer's wording.
  - The hardcoded `error_class` literals are untouched ("write-failed-temp-create", "write-failed-rename") and each is byte-identical to what `fileutil.ClassifyWriteError` returns for the sentinel now wrapped (atomic.go:26-39), so the token and the carried sentinel cannot disagree today.
  - The change is confined to the two `component != ""` WARN branches; the early-return branches, the INFO success line, the `os.Remove` of the old dir, and the function's no-return best-effort contract are unchanged. `migrateConfigFile` returns nothing and no caller inspects the error, so the widened error text reaches only the log line — no production consumer parses it (grep for "failed to rename"/"failed to create directory" finds only the two producers plus unrelated tmux/tui messages).
  - Scope discipline held: `git show --stat c1542d9b` touches exactly `cmd/config.go` and `cmd/config_migrate_logging_test.go`. `internal/storelog/clean_stale_test.go` is untouched by this commit and still carries its `loggedErr != saveErr` identity check (clean_stale_test.go:63-66), which is the stronger property the task said not to fold in.
  - Consolidation is complete tree-wide: `AssertWriteFailure` is now the sole route for every failed-write tail — cmd/config_migrate_logging_test.go:179,214, internal/alias/store_logging_test.go:150,228, internal/project/store_logging_test.go:116,222,312,472, internal/hooks/store_test.go:1032,1092,1251,1392. No `HasAttr("error")` remains beside an `error_class` check anywhere.

TESTS:
- Status: Adequate
- Coverage: All four named tests exist with the exact wording the task specified. (1) `it wraps the migrate rename failure in the rename write-phase sentinel` (:159) — `AssertRecord` for the five shared properties, `path`, `AssertWriteFailure(..., ErrWriteRename)`, plus the retained "old file survives a failed rename" assertion carried over from the deleted test. (2) `it wraps the migrate temp-create failure in the temp-create write-phase sentinel` (:188) — same shape against `ErrWriteTempCreate`, with the `path` attr pinned to `filepath.Dir(newPath)`. (3) `it fails when the carried error does not wrap the classified phase sentinel` (:217) — drives `AssertWriteFailure` through a `harnesstest.Recorder` spy with a deliberately wrong sentinel (`ErrWriteWrite`) and asserts exactly one reported failure; this is the test that proves the strengthening is real, and it would fail if the wrap were removed. (4) `it renders the same error_class token as before` (:231) — pins the token both structurally and in `sink.Body()`, which is the acceptance criterion about the rendered breadcrumb.
- Notes:
  - No coverage was lost in the rewrite: the two deleted tests' distinct assertions (path attr, old-file survival, WARN level/component/op/via) are all carried forward; only the weaker `HasAttr("error")` and the inline class-string check were dropped, both subsumed by the helper.
  - The `denyRenameFixture` extraction is genuinely shared (three call sites: :160, :220, :234) rather than speculative, and its three returns line up positionally with `migrateConfigFile(oldPath, newPath, component)`.
  - Test 4 re-checks `error_class` structurally before checking the rendered body; the structural half overlaps `AssertWriteFailure`'s own check in tests 1-2. It is a single line and the body check is the property the criterion actually names, so this is not over-testing worth acting on.
  - Lane and isolation rules hold: unit lane, no binary build, no daemon, no tmux, no `t.Parallel()`, temp dirs only, chmod restores registered via `t.Cleanup`. `logtest.Install(t)` precedes the fixture call in tests 3-4, which is safe because the fixture emits no log records; cleanup order (LIFO: chmod restore, then handler restore) is correct.

CODE QUALITY:
- Project conventions: Followed. The log line keeps the closed attr vocabulary (`op`/`via`/`path`/`error`/`error_class`) and the existing component threading; no new component or attr key is invented. `internal/fileutil` is a legal dependency for `cmd` (top layer), and no edge is added to `logtest`, whose leaf guard would have caught it.
- SOLID principles: Good. The sentinel stays a parameter at the assertion boundary; the production change is a wrap at the emission site, not a new abstraction.
- Complexity: Low. Two added lines in production, one extracted test fixture.
- Modern idioms: Yes. Multi-`%w` `fmt.Errorf` (Go 1.26 module), `errors.Is` matching through the helper.
- Readability: Good. `wrapped` is named at the point of use and the message mirrors `fileutil`'s own phrasing, so the two producers read alike.
- Issues: None reaching the reporting bar. One observation deliberately not raised as a finding: the migrate branches still hardcode the class literal rather than deriving it via `fileutil.ClassifyWriteError(wrapped)` the way the sibling store emitters do (internal/project/store.go:107,200,234; internal/hooks/store.go:145,203). The task explicitly required the literals be left unchanged, the values are identical to what the classifier would return, and test 4 pins them — so nothing is broken and the alternative is a preference.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
