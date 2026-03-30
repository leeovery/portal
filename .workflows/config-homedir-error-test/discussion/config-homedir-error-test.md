# Discussion: Config Homedir Error Test

## Context

The `configFilePath` function in `cmd/config.go` (line 49-51) has an untested error branch where `os.UserHomeDir()` fails. The code is a standard `if err != nil { return "", fmt.Errorf(...) }` pattern — trivially correct. However, it's the only untested branch in the function.

This surfaced during review of the config-dir-wrong-path-macos bugfix. The existing test pattern in `cmd/config_test.go` uses `t.Setenv` for env var manipulation but has no way to force `os.UserHomeDir` to fail since the stdlib function reads from the environment and OS state with no injection point.

The project uses a consistent DI pattern: small interfaces and package-level `*Deps` structs for external dependencies, injected in tests via `t.Cleanup`. The question is whether to extend this pattern to cover `os.UserHomeDir`, and whether the coverage gain justifies the indirection.

### References

- `cmd/config.go` — the function under discussion
- `cmd/config_test.go` — existing test coverage
- Inbox item: `.workflows/.inbox/.archived/ideas/2026-03-28--config-homedir-error-test.md`

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Worth testing at all (pending)

  DI mechanism (pending)
  ├─ homeDirFunc injection (pending)
  └─ ConfigDeps struct (pending)

  Scope of change (pending)

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent.*

---

## Summary

### Key Insights
*(To be populated during discussion)*

### Open Threads
*(To be populated during discussion)*

### Current State
- Discussion initialized with seed subtopics
- No decisions yet
