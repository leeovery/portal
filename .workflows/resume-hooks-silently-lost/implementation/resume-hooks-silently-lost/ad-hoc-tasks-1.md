# Ad Hoc Tasks: Resume Hooks Silently Lost

## Task 1: A Dependency Guard's Verdict Can Be Served Stale From The Test Cache
placement: new phase "Guard Cache Correctness"

**Problem**: `PackageDeps` resolves a package's dependency set by shelling out to `go list -deps` (`internal/sourceguardtest/packagedeps.go`, `listDeps`), and Go's test cache tracks only the files the *test binary itself* opens — a subprocess's reads are invisible to it. A file added to a package behind `//go:build integration` is also not compiled into that package's default-lane test binary, so nothing about the test's cache key moves. The guard reports `(cached)` and passes without re-running, while the forbidden dependency it would have caught sits in the tree.

Observed during task 9-19 and independently confirmed by that task's reviewer: staging an `integration`-tagged file importing `internal/tmux` into `internal/shellquote`, `internal/harnesstest` and `internal/xdg` left `go test ./internal/...` reporting `(cached)` and green; `-count=1` failed immediately. This caps the practical strength of the tag-aware coverage task 9-19 added — the guard is correct when it runs, and the cache decides whether it runs.

The reach is every site driving `sourceguardtest.Lanes()` — the eight leaf guards in `internal/{nanoid,shellquote,harnesstest,xdg,prefs,hooks,theme,sourceguardtest}` — plus `cmd/capturetool/import_guard_test.go`, which calls `PackageDeps` directly. It is a property of the shared primitive, not of any one guard.

**Solution**: Make the package's own source files inputs to the test that judges them, so adding one moves the cache key and the guard re-runs. Go's test cache is populated from the `testlog` hook, which records the test binary's own `open`/`stat` calls and a directory listing for a directory it opens — so reading the package directory inside `PackageDeps`, before the `go list` subprocess runs, is what closes the gap.

**Outcome**: Adding a build-tagged file to a guarded package invalidates that package's test cache, so the dependency guard re-runs and bites rather than reporting a stale pass.

**Do**:
- In `internal/sourceguardtest/packagedeps.go`, have `PackageDeps` read the resolved package's own directory before invoking `go list`, so the directory listing and its `.go` files enter the test binary's cache inputs. `PackageGoFiles` already enumerates a single directory and is the natural route; resolve the directory from the `depsConfig` (`InDir`, else the test binary's working directory).
- State in the function's doc comment why the read exists — that `go list` runs as a subprocess whose reads the test cache cannot see, so without it a tagged file added to the package leaves the cache key unmoved.
- Prove the fix by experiment rather than by construction, in a scratch copy outside the repository: run a leaf guard so it caches, stage an `integration`-tagged file importing a forbidden package, run the guard again *without* `-count=1`, and observe it re-run and fail. Discard the copy; never edit the tree back.
- Confirm the same experiment fails to invalidate before the change, so the RED is observed rather than assumed.
- Leave `cmd/capturetool/import_guard_test.go`'s default-only reading as it is — its subject is the untagged release binary, and this change reaches it through the shared primitive without altering what it judges.

**Acceptance Criteria**:
- [ ] Adding an `integration`-tagged file to a package guarded through `Lanes()` invalidates that package's test cache, observed without `-count=1`.
- [ ] The same experiment run against the pre-change primitive is observed *not* to invalidate, so the fix is shown to be what closes it.
- [ ] The directory read happens inside `PackageDeps` for every caller, rather than being added per guard.
- [ ] The reason for the read is stated where the read is, in terms of the subprocess boundary.
- [ ] No guard's verdict changes: every leaf guard passes on the unmodified tree and still reports the dependencies it reported before.

**Tests**:
- `"it reads the judged package's directory before resolving its dependencies"`
- `"it resolves the same dependency set as before the read"`
- `"it reports a forbidden dependency reachable only from an integration-tagged file"` stays green with no assertion edited
