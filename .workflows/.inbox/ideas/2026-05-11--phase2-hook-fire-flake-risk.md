# CI parallelism flake risk in phase2_hook_fire_integration_test.go

`cmd/bootstrap/phase2_hook_fire_integration_test.go` documents a CI parallelism caveat at lines 49-56: under `go test -tags=integration ./...` with default parallelism, the test can flake because the 500ms `WriteFIFOSignal` retry budget gets squeezed by concurrent `go build` invocations across other test packages building the portal binary.

Mitigation is currently documented in the file header but not enforced — nothing prevents a CI runner from triggering the flake. If this test becomes a source of intermittent CI failures, options include:

- Bump the eager-signal retry budget for integration runs (env-gated).
- Serialise the integration suite (`-p 1` in the integration job).
- Cache the built portal binary across packages so the build cost is paid once.

Worth tracking if the test becomes a flake source. Not actionable until then.

Source: review of killed-sessions-resurrect-on-restart/killed-sessions-resurrect-on-restart
