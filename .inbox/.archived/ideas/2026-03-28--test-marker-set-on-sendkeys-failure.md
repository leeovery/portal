# Test Volatile Marker Set Even When SendKeys Fails

The hook executor's fire-and-forget semantics mean that when a restart command is delivered to a pane via `send-keys`, the volatile marker (`@portal-active-{paneID}`) must be set regardless of whether `send-keys` succeeded or failed. This prevents re-execution on subsequent `portal open` calls even if the initial delivery hit a transient error.

The implementation in `internal/hooks/executor.go` correctly does this — `SetServerOption` is called after `SendKeys` regardless of the `SendKeys` error — but there's no dedicated test in `internal/hooks/executor_test.go` that proves it. The existing tests cover the happy path (marker set after successful execution) and the error-silencing behavior (SendKeys failure doesn't block other panes), but none specifically assert that the marker is set *when* SendKeys fails for a given pane.

This is worth having as regression coverage because the spec explicitly calls out fire-and-forget semantics, and a future refactor that accidentally gates `SetServerOption` on `SendKeys` success would silently break the two-condition execution check — hooks would re-fire on every `portal open` for panes where delivery failed once.

A test named something like `"sets volatile marker even when send-keys fails"` would configure a mock where `SendKeys` returns an error for a specific pane, then assert that `SetServerOption` was still called with the correct marker name for that pane.
