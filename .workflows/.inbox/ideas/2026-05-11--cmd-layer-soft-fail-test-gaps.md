# Cmd-layer soft-fail / error-path test gaps

Three related test-coverage gaps in the cmd-layer signaling and hydration paths. Each gap is guaranteed architecturally today but not pinned by a regression test; a future change to the surrounding code could silently break the soft-fail contract.

1. **`cmd/state_signal_hydrate.go:42-44`** — no dedicated test for the list-markers (`show-options`) failure → soft-warn → return-nil arm. Coverage exists by analogy via `TestSignalHydrate_SoftFailsWhenSessionDoesNotExist` (list-panes failure arm), but the show-options failure path has no direct exercise. A `recordingCommander` returning an error on `show-options` would close the parallel.

2. **`cmd/state_signal_hydrate_test.go`** — no multi-pane scenario asserting "pane A `SendSignal` fails, pane B still receives `SendSignal`". The single-pane case is covered by `TestSignalHydrate_PerFIFOFailureDoesNotUnsetMarker`; loop-continuation is structurally evident but no N≥2 test directly asserts sibling isolation. `statetest.RecordingFIFOSignaler.ErrOn` is purpose-built for path-keyed selective failure injection.

3. **`cmd/state_hydrate.go:260-277`** — no test forces the underlying `set-option -su` to return an error and asserts (i) WARN log line is emitted, (ii) shell exec still proceeds on the timeout branch. The soft-fail-and-continue posture is guaranteed by the void-returning `unsetSkeletonMarkerOrLog` helper, but a regression that promotes the marker-unset failure to a hard error would not be caught. Symmetric gap exists on the file-missing branch.

Source: review of killed-sessions-resurrect-on-restart/killed-sessions-resurrect-on-restart
