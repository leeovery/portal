# Decide whether a burst that never started should emit `spawn: opened 0/N`

In `handleBurstPartialFailure` the observability emission (`emitBurstSummary`) runs BEFORE the `msg.Err != nil` early return, so a pre-spawn `Burster.Run` error (nothing opened, possibly an empty `msg.Batch`) still emits a `spawn: opened 0/N` INFO batch summary on the picker path — whereas the CLI's `runSpawn` returns the error and emits nothing.

Decide whether a `0/N` (possibly empty-batch) summary is the intended forensic breadcrumb for a burst that never started, or whether the pre-spawn-error arm should skip the summary to match the CLI's silence. This is a CLI/picker observability parity question, pre-existing and out of the analysis cycles' targeted scope.

Location: `internal/tui/burst_partial_failure.go:43-47`. (Reports 6-6, 7-1.)

Source: review of restore-host-terminal-windows/restore-host-terminal-windows
