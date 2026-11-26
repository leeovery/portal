# Split `cmd/state_commit_now_test.go` into per-task files

The unit-test file is 1,273 lines and bundles tests for tasks 1-1 (happy path), 1-2 (`@portal-restoring` short-circuit + presume-set), and 1-3 (failure-path discipline) behind section banners. Navigation across the file relies on those banners; a future reader looking for a specific test set would benefit from physical separation.

Suggested split:

- `state_commit_now_test.go` — happy-path tests (zero/one/multi-pane sessions, PrevIndex passthrough, underscore filter, ENOENT/corrupt `sessions.json`, no-touch-on-success, no-`.bin`-writes, subcommand registration).
- `state_commit_now_restoring_test.go` — `@portal-restoring` short-circuit + `IsRestoring`-presume-set behaviours.
- `state_commit_now_failure_test.go` — failure-path discipline (capture/commit errors, touch-also-fails, no-panic, sentinel identity).

Pure structural refactor — no logic or assertion changes. Care needed around shared fixtures (`commitNowFixture`, `installCommitNowDeps`, `fakeCaptureClient`) — keep them in the happy-path file and let the siblings import via package-level visibility (same `package cmd`).

Source: review of killed-session-resurrects-within-tick-window/killed-session-resurrects-within-tick-window
