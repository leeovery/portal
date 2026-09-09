TASK: resume-hooks-silently-lost-5-9 — Two Strings Name Things That Are No Longer True

ACCEPTANCE CRITERIA:
- The sweep's load-failure WARN names `LoadSnapshot`, and its test constant matches
- The open-branch wrap renders the sidecar path once, not twice
- `internal/hooks/lock.go`'s other two wraps are unchanged
- The existing assertion over the lock text (`strings.Contains(err.Error(), "hooks lock")`) still holds — no test changes
- No structured attr, `op`, `reason` or `via` value changes anywhere
- `go test ./...` and `go test -tags integration -p 1 ./...` pass

STATUS: complete

SPEC CONTEXT:
The specification's only bearing on this task is the degraded-read emission (§ around line 387): DEBUG, `op=load-unlocked`, the lock error carried in `error`, `via` naming the caller — and the two-bound corrigendum (2026-09-01), which confirms the clean's advisory pre-read degrades to an unlocked read routinely on a contended or pre-sidecar install, so that breadcrumb is a high-volume line rather than an edge case. The task's own note that neither string is spec-governed holds: the spec fixes the structured `op`/`reason`/`via` surface, not the freeform WARN wording or the wrap text. No corrigendum touches this task.

IMPLEMENTATION:
- Status: Implemented (part (a) subsequently superseded, legitimately)
- Location:
  - `internal/hooks/lock.go:55` — `return nil, fmt.Errorf("open hooks lock: %w", err)` (the `%s` is gone)
  - `internal/hooks/lock.go:66` and `:70` — the flock wrap (`"flock hooks lock %s: %w"`) and the `ErrLockHeld` wrap (`"%w: %s"`) both still carry the path, as the task required
  - commit `f08f1418` — the delivered change-set: `cmd/run_hook_stale_cleanup.go` (WARN renamed to `hookStore.LoadSnapshot failed`), `cmd/run_hook_stale_cleanup_test.go` (the one `loadWarnFmt` constant), `internal/hooks/lock.go` (the wrap). Nothing else in code.
- Notes:
  - Part (b) is intact in the current tree and correct. `acquireLock`'s open branch is reached from both `acquireMutationLock` (`internal/hooks/lock.go:80-83`) and `acquireSharedLock` (`:90-92`); in both cases the wrapped error is the `*os.PathError` returned by `os.OpenFile`, which renders the path itself, so the DEBUG breadcrumb at `internal/hooks/store.go:73` now carries the sidecar path once. No other caller of `acquireLock` exists, so no path is lost anywhere.
  - Part (a)'s string no longer exists: task 9-12 (`a4898f41`) re-homed the sweep into `internal/hooksweep`, deleting `cmd/run_hook_stale_cleanup.go` and its test outright. This is supersession, not loss — the failed read is still reported, now as a stand-down under `ReasonStoreReadFailed` with the read error in the `error` attr (`internal/hooksweep/sweep.go:186`), and `CleanStale` wraps both of its reads with `hooks.ErrStoreRead` (`internal/hooks/store.go:301` and `:329`), which CLAUDE.md documents as one deliberate condition. The method-naming distinction 5-9 restored was thereby collapsed on purpose by a later design, so nothing the intent still needs is gone.
  - The task's residual concern about the WARN being pinned by a prior work unit's specification (`bootstrap-cleanstale-wipes-hooks-on-tmux-transient`, which named the exact string `"stale-hook cleanup: hookStore.Load failed: <error>"`) is moot on the same grounds: the reporting obligation — never silent on a failed load — is met by the stand-down WARN.

TESTS:
- Status: Adequate
- Coverage: The task prescribed no new test and one constant update; that is exactly what landed. The lock-text assertion the task named survives at `internal/hooks/lock_test.go:303` (`strings.Contains(err.Error(), "hooks lock")`) and still passes against `"open hooks lock: …"` by reading. The load-failure branch's coverage moved with the code: `internal/hooksweep/sweep_test.go:92` asserts the read failure stands the cycle down with `ReasonStoreReadFailed` and no counts Debug; `internal/hooks/cleanstale_read_sentinel_test.go:33,47,59` pins `ErrStoreRead` at both reads and its absence elsewhere.
- Notes: No test observes the "path rendered once" property — `hookstest.AssertDegradedRead` (`internal/hookstest/hooks_lock.go:135-150`) checks only that the `error` attr is non-empty alongside `op`/`via`/level. That is the task's own instruction ("No new test"), and the property is a rendering detail of a DEBUG breadcrumb, so the absence is deliberate rather than a gap. Not over-tested: no assertion was added for a one-token string change.

CODE QUALITY:
- Project conventions: Followed. No log component, `op`, `reason` or `via` value moved; the closed vocabulary is untouched. Error wrapping stays `%w`-based and the sentinel discrimination (`ErrLockHeld`, `unix.EWOULDBLOCK`) is unaffected.
- SOLID principles: Good — the change is confined to message text at the site that owns it.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. The three wraps in `acquireLock` now differ meaningfully — the open branch defers the path to the `*os.PathError`, while the flock and timeout branches state it because their wrapped values (`unix.Errno`, `ErrLockHeld`) carry none — which is the distinction the task set out to make legible.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
