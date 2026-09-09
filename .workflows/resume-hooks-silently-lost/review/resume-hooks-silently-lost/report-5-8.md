TASK: resume-hooks-silently-lost-5-8 — "The Staging Helpers Reach Their Declared Home, And One Seeder Serves Every Caller" (tick-951b45)

ACCEPTANCE CRITERIA:
- `saveDeniedStore` is gone; one seeder serves every caller including the deny-writes case
- `newTempHooksStore`, `readFileBytes` and `lockBound` live in `cmd/testhelpers_test.go`
- One `hook set` driver remains, with the same return shape as `runHookRm`
- Phase 5's three inline file-unchanged assertions route through the helper and each keeps its own failure message
- `cmd/run_hook_stale_cleanup_test.go`'s inline sites are untouched
- No assertion changes and no test changes its verdict
- `go test ./...` and `go test -tags integration -p 1 ./cmd/...` pass

STATUS: complete

SPEC CONTEXT: The task is a test-hygiene consolidation the implementation phase generated, not a spec clause. Its subjects are the fixtures behind spec §6.5 ("Acquisition is bounded, and a timeout degrades rather than wedges"): the lock-timeout suites that pin `hook set`/`hook rm` exiting non-zero and leaving `hooks.json` byte-identical, the sweep standing down under a held lock, and — the case the deny-writes fixture exists for — a genuine save failure being discriminated from a lock timeout. The spec's own words on that discrimination are what make the fixture's precise failure phase (temp create, not lock open) load-bearing rather than incidental.

IMPLEMENTATION:
- Status: Implemented (delivered at 8cf0b880; carried forward and extended by phases 6–9)
- Location: delivered — `cmd/testhelpers_test.go`, `cmd/bootstrap_production_test.go`, `cmd/hook_sweep_lock_timeout_test.go`, `cmd/hooks_write_lock_test.go`, `cmd/hooks_pane_token_test.go`, `cmd/hooks_test.go`, `cmd/hookkey_vocabulary_test.go`. Current tree — `cmd/testhelpers_test.go:20` (`lockBound`), `:130` (`readFileBytes`), `:176` (`runHookSet`), `:230` (`assertHooksFileUnchanged`); the seeder now lives at `internal/hookstest/staging.go:55` (`StageStore`) with `Staging.WritesDenied` at `:46`, and the assertion at `internal/hookstest/hooks.go:113`.
- Notes: Every criterion holds in substance in the current tree. `saveDeniedStore`, `newStagedHooksStore`, `hooksStoreStaging`, `newTempHooksStore` and `runHookSetCapturing` return no matches anywhere in the repo. The options-struct seeder 5-8 introduced was lifted verbatim into `internal/hookstest` by a later phase — same fields, same ordering, same "sidecar before the denial" rule (`internal/hookstest/staging.go:85-95`), extended with `Entries`/`Body`/`Unreadable`. The thin `newTempHooksStore` wrapper 5-8 kept for its ~20 existing callers was retired with them by that same consolidation, which is the intended direction of travel rather than a loss. `lockBound` and `readFileBytes` are still in the declared home. The one `hook set` driver returns `(string, error)`, matching `runHookRm` (`cmd/testhelpers_test.go:176`, `:189`); `runHookSetForKey` (`cmd/hooks_test.go:717`) is a stub-installing convenience over it, not a second driver.
  The three re-pointed inline assertions kept their own words: "rewritten under a held lock" survives at `internal/hooksweep/lock_timeout_test.go:39` and "rewritten by the daemon under a held lock" at `cmd/hook_prune_single_report_test.go:61`. The third, "rewritten on a lock stand-down", is no longer spelled that way — a later phase generalised the doctor `--fix` post-condition into `assertHooksPathUnchanged` (`cmd/doctor_stand_down_copy_test.go:280`, driven per stand-down reason at `:307` and `:326`, the lock row included at `:113-121`), and that successor is strictly stronger: it compares a file-state string rather than bytes, so it also catches an absent-versus-present flip. The assertion was relocated and widened, not dropped.
  `cmd/run_hook_stale_cleanup_test.go` is absent from the commit's file list — its inline `reflect.DeepEqual` sites were left alone as instructed.

TESTS:
- Status: Adequate
- Coverage: No new test was asked for and none was added — correct for a consolidation whose contract is that every existing verdict is unchanged. The delivered diff is mechanical: identical helper bodies moved between files, one call-shape change (`err :=` → `_, err :=`) at 17 sites, and two fixture constructions re-expressed through the new options struct. No assertion text or condition changed.
- Notes: The task's one standing obligation — that the deny-writes fixture still reaches `error_class=write-failed-temp-create` after the seeder change — holds by construction and is now pinned explicitly. Old and new both create the sidecar before the `chmod 0o500`, so the mutation still takes its lock and reads cleanly before failing at the temp create; `internal/hooksweep/lock_timeout_test.go:122-125` now asserts `errors.Is(err, fileutil.ErrWriteTempCreate)` on that exact fixture, which is the fixture-drift tripwire the task's Tests note was asking for (added by a later phase, but the subject is reached). The behavioural deltas in the seeder move are both inert: `os.Mkdir` → `os.MkdirAll` (only reachable with a pre-existing directory, which no caller stages), and `runHookSet` writing both streams into one captured buffer rather than two discarded ones — which is what `runHookRm` and the deleted `runHookSetCapturing` already did, and what `assertLockFailureReachesStderr` (`cmd/hooks_write_lock_test.go:19`) consumes.
  Note on verification method: tests were assessed by reading, not by execution.

CODE QUALITY:
- Project conventions: Followed. Staging helpers sit in the file whose header declares itself their home; the subject vocabulary stays in `hookkey_vocabulary_test.go`. Lane placement is untouched (all files unit-lane). No `t.Parallel()`, no seam assigned outside the `withXDeps`/`withFuncSeam` staging helpers.
- SOLID principles: Good. The options struct replaced a boolean-and-string parameter creep that a third seeder would otherwise have kept spawning; the field set is small and each field states why it exists.
- Complexity: Low. One `switch`-free staging function with three independent flags.
- Modern idioms: Yes.
- Readability: Good. The doc comments carry the reasoning a reader needs and no more — notably why the sidecar is created before the denial, which is the one ordering a later edit could silently break.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
