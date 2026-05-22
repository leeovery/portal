---
phase: 1
phase_name: Foundations — Daemon Identity Primitive & Test Isolation
total: 5
---

## slow-open-empty-previews-and-zombie-sessions-1-1 | approved

### Task 1.1: Implement state.IdentifyDaemon primitive

**Problem**: Components A (kill-barrier escalation), B (bootstrap orphan sweep), and C (`AcquireDaemonLock` pre-check) all need the same primitive — "is PID `p` a live `portal state daemon`?" Without this shared check, direct signalling and lock-pre-checks risk hitting recycled PIDs (destructive false positive) or refusing legitimate succession (false positive contention). The spec calls this out as the Shared Primitive used by A/B/C.

**Solution**: Add a new leaf file `internal/state/daemon_identity.go` exporting `state.IdentifyDaemon(pid int) (IdentifyResult, error)` with a three-result contract plus a transient-error case. Implementation shells out to `ps -o comm=,args= -p <pid>` and matches `comm == "portal"` AND argv against the anchored regex `^portal state daemon( |$)`.

**Outcome**: A single, well-tested identity primitive that A/B/C all consume; callers can branch deterministically on `IdentifyIsPortalDaemon` / `IdentifyNotPortalDaemon` / `IdentifyDead`, and on non-nil error they apply caller-specific transient-error semantics (spec § Shared Primitive Caller semantics).

**Do**:
- Create `internal/state/daemon_identity.go` with the public type `IdentifyResult int` and constants `IdentifyIsPortalDaemon`, `IdentifyNotPortalDaemon`, `IdentifyDead` (iota order matches spec).
- Implement `IdentifyDaemon(pid int) (IdentifyResult, error)`:
  - Reject `pid <= 0` as `IdentifyDead, nil` (defensive guard; no `ps` call needed).
  - Exec `ps -o comm=,args= -p <pid>`. Use `os/exec.Command` with no shell interpolation.
  - On non-zero exit with empty stdout: treat as `IdentifyDead, nil` (the canonical "PID not found" shape).
  - On non-zero exit with non-empty stdout, or zero exit with malformed stdout (cannot split into a non-empty comm and an argv tail): return `0, error` (transient).
  - On zero exit with parseable stdout: trim the line, split on the first run of whitespace into `comm` and `argv`. Match `comm == "portal"` (case-sensitive) AND `argv` against `regexp.MustCompile("^portal state daemon( |$)")`.
  - If both match: `IdentifyIsPortalDaemon, nil`. Otherwise: `IdentifyNotPortalDaemon, nil`.
- Expose a package-level `var identifyPS = ...` seam (default invokes `exec.Command("ps", ...)`) so tests can stub `ps` output without forking a real process for the parsing branches.
- Place the compiled regex in a package-level `var` (compile once).
- Add docstring on `IdentifyDaemon` that documents the three-result contract verbatim from the spec and the err-non-nil semantics for each component caller.

**Acceptance Criteria**:
- [ ] `state.IdentifyDaemon` is callable with signature `func(pid int) (IdentifyResult, error)`.
- [ ] Against the current process PID (`os.Getpid()`) running under `go test` (binary name `state.test`, not `portal`), returns `IdentifyNotPortalDaemon, nil`.
- [ ] Against a known-dead PID (spawn a process, `Wait`, then call with its PID), returns `IdentifyDead, nil`.
- [ ] Against a stubbed `identifyPS` returning `"portal portal state daemon\n"`, returns `IdentifyIsPortalDaemon, nil`.
- [ ] Against a stubbed `identifyPS` returning `"portal portal state daemon-foo\n"`, returns `IdentifyNotPortalDaemon, nil` (anchored regex rejects suffixes).
- [ ] Against a stubbed `identifyPS` returning `"sleep sleep 30\n"`, returns `IdentifyNotPortalDaemon, nil` (recycled PID case).
- [ ] Against a stubbed `identifyPS` that errors with non-empty stdout, returns `(0, err)` with a non-nil error.
- [ ] `pid <= 0` returns `IdentifyDead, nil` without invoking `ps`.

**Tests** (in `internal/state/daemon_identity_test.go`):
- `"it returns IdentifyIsPortalDaemon when ps reports portal binary with state daemon argv"` — stub `identifyPS` returning canonical match line.
- `"it returns IdentifyNotPortalDaemon when comm is not portal"` — stub returning `"sleep sleep 30\n"`.
- `"it returns IdentifyNotPortalDaemon when argv suffix breaks the anchored regex"` — stub returning `"portal portal state daemon-foo"`.
- `"it returns IdentifyNotPortalDaemon when argv is portal something-else"` — stub returning `"portal portal open"`.
- `"it returns IdentifyDead for a non-existent PID"` — call against `os.Getpid()` of a known-reaped child (spawn `sleep 0.01`, `Wait`, then identify).
- `"it returns IdentifyDead when ps exits non-zero with empty stdout"` — stub returning empty + exit 1.
- `"it returns a transient error when ps exits non-zero with non-empty stdout"` — stub returning garbage + exit 1.
- `"it returns a transient error when ps output is malformed (single token)"` — stub returning `"portal\n"`.
- `"it returns IdentifyDead for pid <= 0 without invoking ps"` — pass `pid = 0` and `pid = -1`; assert the stubbed `identifyPS` was never called (counter on the seam).
- `"it handles whitespace-padded ps output"` — stub returning `"  portal   portal state daemon  \n"`; expect `IdentifyIsPortalDaemon`.
- `"it does not match portal-state-daemon (no spaces)"` — stub returning `"portal portal-state-daemon"`; expect `IdentifyNotPortalDaemon`.

**Edge Cases**:
- Dead PID (already reaped) → `IdentifyDead`, no error.
- Recycled PID now hosting an unrelated process (`sleep`, login shell) → `IdentifyNotPortalDaemon`.
- `ps` exec failure (binary missing, permissions denied) with non-empty stderr → transient error.
- Malformed `ps` output (empty, single token, embedded null bytes) → transient error.
- Leading/trailing whitespace in `ps` output → stripped before parsing.
- `pid <= 0` → cheap reject as `IdentifyDead`; never shells out.
- Argv with suffix (`portal state daemon-foo`) → rejected by anchored regex.
- Argv exact match with trailing arguments (`portal state daemon --flag`) → accepted (regex permits trailing space + content).

**Context**:
> Spec § Shared Primitive — Daemon Identity Check defines the three-result contract verbatim. Implementation uses `ps -o comm=,args= -p <pid>` (macOS-compatible; portable across Linux). Caller transient-error semantics differ per component but are NOT encoded in this primitive — each caller (A/B/C) applies its own policy on the err-non-nil return. Component C's pre-check biases toward "let legitimate succession proceed", A and B skip the kill on transient error. This task implements the shared primitive only; the per-caller policies land in their respective phases.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § "Shared Primitive — Daemon Identity Check".

## slow-open-empty-previews-and-zombie-sessions-1-2 | approved

### Task 1.2: Implement portaltest.NewIsolatedStateEnv helper

**Problem**: The trigger that caused the reporter's bug was a leaked test-fixture daemon that inherited the developer's `$XDG_CONFIG_HOME` and wrote to `~/.config/portal/state/` while enumerating sessions from an unrelated tmux server. Without a structurally-mandatory test helper, future ad-hoc tests can recreate the same hazard. Components A/B/C/D all require integration tests that spawn `portal state daemon`; running those tests safely demands per-test state-dir isolation.

**Solution**: Add a new leaf package `internal/portaltest/` with `NewIsolatedStateEnv(t *testing.T) (env []string, stateDir string)`. The helper derives a per-test `t.TempDir()` value, builds an `env` slice (starting from `os.Environ()`) that **removes** any inherited `XDG_CONFIG_HOME` and **sets** `XDG_CONFIG_HOME=<tempDir>/config`, `MkdirAll`s that path, and resolves the state directory underneath it. The `*testing.T` signature ensures the helper cannot be imported into production code.

**Outcome**: A test helper that any daemon-spawning test calls before `exec.Cmd.Env = env`; the spawned subprocess writes only to the isolated state dir, never to the developer's real `~/.config/portal/state/`. This task lands the helper itself; the `t.Cleanup` fingerprint backstop lands in Task 1.3.

**Do**:
- Create `internal/portaltest/` as a new leaf package with `doc.go` describing the package's test-only purpose.
- Create `internal/portaltest/isolated_env.go` exporting `func NewIsolatedStateEnv(t *testing.T) (env []string, stateDir string)`:
  - Call `t.Helper()`.
  - `tempDir := t.TempDir()`.
  - `configDir := filepath.Join(tempDir, "config")`; `os.MkdirAll(configDir, 0o700)`; fatal-fail via `t.Fatalf` on error.
  - Resolve `stateDir := filepath.Join(configDir, "portal", "state")`; `os.MkdirAll(stateDir, 0o700)` (so callers that pass `stateDir` to seeders can write immediately).
  - Build `env` from `os.Environ()`, filtering out any entry whose key (`strings.SplitN(e, "=", 2)[0]`) equals `XDG_CONFIG_HOME`.
  - Append `"XDG_CONFIG_HOME=" + configDir` to the filtered slice.
  - Return `(env, stateDir)`.
- Add a top-of-file comment stating: "Test-only. Importing this package from non-`*_test.go` files is prohibited — the `*testing.T` parameter enforces this structurally."
- Do NOT add the fingerprint backstop in this task — Task 1.3 owns it.

**Acceptance Criteria**:
- [ ] `portaltest.NewIsolatedStateEnv(t)` compiles and is callable from `*_test.go` files in any package.
- [ ] Returned `env` contains exactly one entry beginning with `XDG_CONFIG_HOME=`, and its value points inside `t.TempDir()`.
- [ ] Returned `env` does NOT contain the developer's pre-test `XDG_CONFIG_HOME` value (verified by setting `XDG_CONFIG_HOME=/decoy` before the call and asserting it is gone from the returned slice).
- [ ] Returned `env` preserves all other entries from `os.Environ()` (e.g., `HOME`, `PATH`) — verified by spot-checking `HOME`.
- [ ] Returned `stateDir` path exists on disk and resolves to `<tempDir>/config/portal/state`.
- [ ] `env` is directly usable as `exec.Cmd.Env` — verified by spawning a trivial subprocess (`sh -c 'echo $XDG_CONFIG_HOME'`) with the returned env and asserting the output equals the returned tempDir-derived path.
- [ ] Helper signature requires `*testing.T`; cannot be imported into production code without a `testing` dependency.

**Tests** (in `internal/portaltest/isolated_env_test.go`):
- `"it sets XDG_CONFIG_HOME to a path inside t.TempDir()"` — call helper, parse env, assert XDG_CONFIG_HOME entry is present and prefixed by the tempDir.
- `"it removes a pre-existing XDG_CONFIG_HOME entry"` — `t.Setenv("XDG_CONFIG_HOME", "/decoy")`, call helper, assert no `/decoy` entry appears anywhere in returned env.
- `"it preserves HOME and PATH from os.Environ"` — assert presence of `HOME=` and `PATH=` in returned env.
- `"it returns a stateDir under XDG_CONFIG_HOME/portal/state"` — assert `stateDir` is exactly `<configDir>/portal/state` and exists on disk.
- `"it returns an env usable as exec.Cmd.Env"` — spawn `sh -c 'echo $XDG_CONFIG_HOME'` with the env; assert stdout matches.
- `"it produces distinct stateDir paths for independent t calls"` — call twice with subtests; assert distinct return values.
- `"it MkdirAll the configDir with 0700 perms"` — stat the returned configDir; assert dir mode bits.

**Edge Cases**:
- Pre-existing `XDG_CONFIG_HOME` in `os.Environ()` → must be removed, not duplicated.
- Pre-existing `XDG_CONFIG_HOME` set to empty string → must be removed.
- `HOME` must remain inherited unchanged (daemon and TUI both read it).
- Multiple subtests within the same parent `t` → each `t.TempDir()` is per-subtest, so two calls produce two paths.
- Caller passes the env to `exec.Cmd.Env =` directly without re-merging with `os.Environ()` — the contract is that the returned slice is complete and self-sufficient.

**Context**:
> Spec § Component G calls for a new leaf package `internal/portaltest/` (NOT attached to `portalbintest`) — rationale: env isolation is orthogonal to binary building, and a new leaf keeps the import graph cleaner. The `*testing.T` parameter is the structural mechanism that prevents production-code import. This task delivers the helper shell and env construction. Task 1.3 layers the fingerprint-diff `t.Cleanup` backstop on top.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § "Component G — Test Isolation Contract for `portal state daemon`", item 1 (helper).

## slow-open-empty-previews-and-zombie-sessions-1-3 | approved

### Task 1.3: Add fingerprint-diff t.Cleanup backstop to isolation helper

**Problem**: The env override in Task 1.2 is the primary defence, but a test that bypasses the env (direct file write to `~/.config/portal/state/sessions.json`, ad-hoc subprocess that ignores `Cmd.Env`, etc.) could still corrupt the developer's real state directory. The spec mandates a `t.Cleanup`-time backstop that fingerprints the real state directory pre-test, walks it again post-test, and fails on any delta.

**Solution**: Extend `portaltest.NewIsolatedStateEnv` so it captures a pre-test `map[string]fileFingerprint` of `~/.config/portal/state/` (using lstat semantics throughout), then registers `t.Cleanup` to walk the same directory post-test and `t.Errorf` on any delta. The fingerprint captures (a) existence, (b) size, (c) mtime nanoseconds, (d) ctime nanoseconds, and (e) SHA-256 of contents for files ≤ 1 MiB.

**Outcome**: Any test that uses the helper but accidentally writes to the developer's real state dir fails with a clear error citing the changed path and delta type. The backstop never fires on legitimate test runs because legitimate tests only write through the redirected `XDG_CONFIG_HOME`.

**Do**:
- Define an unexported `fileFingerprint` struct in `internal/portaltest/`: `{ exists bool; size int64; mtimeNanos int64; ctimeNanos int64; sha256 [32]byte; hashed bool }` (`hashed` distinguishes "content hash captured" from "file too large for hash").
- Resolve the developer's real state dir via the same resolution as production (`xdg.ConfigHome()` + `/portal/state`, or directly read `os.Getenv("XDG_CONFIG_HOME")` falling back to `~/.config`). Note: this resolution must use the developer's **pre-override** env. Capture `XDG_CONFIG_HOME` from `os.Environ()` BEFORE the helper modifies anything; if the developer had no `XDG_CONFIG_HOME` set, fall back to `$HOME/.config`.
- Implement `snapshotStateDir(root string) (map[string]fileFingerprint, error)`:
  - If `root` does not exist (lstat returns `os.IsNotExist`): return an empty map and nil error.
  - Walk `root` recursively using `filepath.WalkDir` with `os.Lstat` for each entry (NOT `Stat` — symlink mutations must be visible).
  - For each entry, populate `fileFingerprint` with size, mtimeNanos, ctimeNanos (via syscall stat on darwin/linux — extract from `Sys().(*syscall.Stat_t)`).
  - If the entry is a regular file ≤ 1 MiB: read and SHA-256; set `hashed=true`.
  - If > 1 MiB: leave `hashed=false`; the size/mtime/ctime comparison still detects mutation.
  - Key the map by path relative to `root`.
- Call `snapshotStateDir` at the start of `NewIsolatedStateEnv` (before any env mutation).
- Register `t.Cleanup` that:
  - Walks `root` again with the same function.
  - Diffs old vs new map; for each path: if the entry was added, removed, or any field changed (`exists`, `size`, `mtimeNanos`, `ctimeNanos`, `sha256` when both `hashed`), call `t.Errorf("portaltest backstop: developer state dir mutated at %s: %s", path, deltaType)`.
  - `deltaType` is one of: `"created"`, `"deleted"`, `"size-changed"`, `"mtime-changed"`, `"ctime-changed"`, `"content-changed"`, `"became-symlink"`, `"symlink-target-changed"`.
  - Report ALL deltas (do not return after the first) so a developer sees the full picture.
- Walk scope: ONLY `~/.config/portal/state/` (and descendants). Sibling files like `~/.config/portal/projects.json` are out of scope.

**Acceptance Criteria**:
- [ ] On a test that uses the helper and writes nothing to the real state dir, the `t.Cleanup` passes silently (no `t.Errorf` calls).
- [ ] On a test that uses the helper and deliberately writes a new file to `~/.config/portal/state/sessions.json` via direct path resolution (bypassing the env), the `t.Cleanup` fails with `"created"` delta citing the file's relative path.
- [ ] On a test that mutates the size or content of a pre-existing file in the real state dir, the cleanup fails with `"size-changed"` or `"content-changed"`.
- [ ] On a test that modifies mtime/ctime via `os.Chtimes`, the cleanup fails with `"mtime-changed"` / `"ctime-changed"`.
- [ ] On a test that creates a new symlink in the state dir, the cleanup fails with `"created"` (lstat-detected).
- [ ] On a test that changes a symlink target inside the state dir, the cleanup fails with `"symlink-target-changed"`.
- [ ] Files > 1 MiB are not content-hashed; size/mtime/ctime mutations are still detected.
- [ ] If `~/.config/portal/state/` does not exist pre-test, ANY file or subdirectory created during the test counts as a delta and fails.
- [ ] Sibling files (e.g., `~/.config/portal/projects.json`) are NOT walked and are NOT in scope.
- [ ] Multiple deltas in one test are all reported (not just the first).

**Tests** (in `internal/portaltest/isolated_env_test.go` — meta-tests that invoke the helper inside a sub-`*testing.T` to assert cleanup behaviour):
- `"it passes cleanup when the test writes nothing to the developer state dir"` — happy path; assert subtest fails == false.
- `"it fails cleanup when a new file is created in the developer state dir"` — meta-test creates `<realStateDir>/intruder.json`; assert subtest fails with `"created"` and the relative path.
- `"it fails cleanup when an existing file's size changes"` — seed a file pre-snapshot, mutate post-snapshot; assert `"size-changed"`.
- `"it fails cleanup when an existing file's content changes"` — seed, snapshot, rewrite same size with different content; assert `"content-changed"` (via SHA-256 mismatch).
- `"it fails cleanup when mtime is bumped"` — `os.Chtimes` on an existing file; assert `"mtime-changed"`.
- `"it fails cleanup when a file is deleted"` — seed, snapshot, remove; assert `"deleted"`.
- `"it fails cleanup when a symlink target changes"` — seed symlink, snapshot, recreate with new target; assert `"symlink-target-changed"` (lstat).
- `"it skips content hash for files larger than 1 MiB but still detects size changes"` — seed 2 MiB file, snapshot, append; assert `"size-changed"` (not `"content-changed"`).
- `"it reports all deltas, not just the first"` — make three mutations; assert three `t.Errorf` calls.
- `"it walks only the state subdir, not siblings"` — mutate `~/.config/portal/projects.json`; assert no failure.
- `"it handles a non-existent state dir as an empty pre-snapshot"` — point at a fresh `$HOME` without the state dir; create a file during the test; assert `"created"` is reported.

**Edge Cases**:
- Pre-test state dir does not exist → empty pre-snapshot; any post-test file counts as `"created"`.
- Pre-test state dir is a symlink → resolved via lstat at the root level; walks follow the symlink target normally for descendants (`filepath.WalkDir` does not follow symlinks by default, which is the desired behaviour).
- File becomes a symlink (or vice versa) → detected as type change via `Sys().Mode()`; reported as `"became-symlink"` or similar.
- File > 1 MiB → no content hash; mtime/ctime/size diff still triggers.
- Concurrent mutation by another process during the snapshot walk → outside scope; the backstop is best-effort and is meant to catch the test under inspection, not ambient activity.
- Capture of `XDG_CONFIG_HOME` must happen BEFORE the helper modifies env, otherwise the backstop would walk the temp dir.

**Context**:
> Spec § Component G item 1, sub-bullet 5 details the fingerprint shape: existence, size, mtime nanoseconds, ctime nanoseconds, SHA-256 of contents for files ≤ 1 MiB. Edge cases enumerated in the spec: pre-test missing dir (empty snapshot, any creation fails the test), symlink mutations via lstat, sibling dirs out of scope. The backstop is a defence-in-depth measure, NOT a substitute for the env override — its purpose is to catch contract violations during development before they corrupt the real install.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § "Component G — Test Isolation Contract for `portal state daemon`", item 1 sub-bullet 5 (fingerprint backstop).

## slow-open-empty-previews-and-zombie-sessions-1-4 | approved

### Task 1.4: Audit and migrate existing test helpers to isolated env

**Problem**: Helpers in `internal/portalbintest`, `internal/tmuxtest`, and `internal/restoretest` that spawn `portal` or `portal state daemon` as subprocesses currently inherit `os.Environ()` directly. Any test using them risks corrupting the developer's real state directory — exactly the trigger that produced the reporter's bug. The spec mandates an audit + migration with a `grep`-based completion criterion.

**Solution**: Enumerate every helper in the three packages that calls `exec.Command` / `exec.CommandContext` (or equivalent) with a `portal` binary path. For each: either update the signature to take the isolated env as a parameter (preferred), call `portaltest.NewIsolatedStateEnv` internally before spawning, or tag the helper as out-of-scope with a one-line justification. Produce an audit document captured in the PR description.

**Outcome**: Every repo helper that spawns `portal` is either env-isolated or explicitly justified as out-of-scope. The completion grep — `grep -rn "exec.Command.*portal\b" internal/portalbintest internal/tmuxtest internal/restoretest` — yields zero call sites that are not (a)-tagged or (b)-justified in the audit.

**Do**:
- Run the audit grep across the three packages and enumerate every `exec.Command` / `exec.CommandContext` call site that takes a `portal` binary path. From an initial check, the known offenders are:
  - `internal/restoretest/restoretest.go:181` — `exec.Command(binary, "state", "signal-hydrate", "--", session)` — spawns portal binary.
  - `internal/portalbintest/build.go:108` — `exec.Command("go", "build", "-o", binary, ".")` — builds the binary; does NOT spawn `portal` itself; tag (b) out-of-scope.
  - `internal/tmuxtest/socket.go:79,123` — `exec.Command("tmux", ...)` — spawns tmux, not portal; tag (b) out-of-scope.
  - Verify by re-running the grep against current HEAD before changes; record any sites missed by the spec investigation.
- For each (a) site that spawns portal:
  - Preferred: change the helper signature to accept `env []string` as a parameter; callers obtain it via `portaltest.NewIsolatedStateEnv(t)`. Update every call site in test files.
  - Alternate: if the helper has no `*testing.T` already and adding one would be intrusive, call `portaltest.NewIsolatedStateEnv` internally — but this only works for helpers that already take `*testing.T`.
  - For `internal/restoretest/restoretest.go:181` specifically: the surrounding helper already takes `*testing.T` (verify by reading the file). Update it to call `portaltest.NewIsolatedStateEnv(t)` and apply the returned `env` to the `exec.Command` before `Run`. If the helper already takes an env or state-dir argument, route the isolated env through that.
- For each (b) site: append a one-line justification in the audit (e.g., "builds the binary; spawns `go`, not `portal` — does not write to state dir").
- Audit deliverable: produce `.workflows/slow-open-empty-previews-and-zombie-sessions/planning/slow-open-empty-previews-and-zombie-sessions/audit-G-test-helpers.md` containing a table with columns `Helper | File:Line | Disposition (a/b/c) | Notes`. Capture the post-change `grep -rn "exec.Command.*portal\b" internal/portalbintest internal/tmuxtest internal/restoretest` output verbatim in the audit footer; the completion criterion is that every line in the grep output is either tagged (a) "updated" or (b) "out of scope".
- Also widen the grep to catch `exec.CommandContext` and any helper that wraps these via a thin adapter (e.g., a local `runPortal` function). If a wrapper is found, treat it as a single audit entry.
- Run the existing integration test suite (`go test -tags integration ./...` if integration tags are used, plus the standard `go test ./...`) after migration; all tests must pass without modification beyond signature updates.

**Acceptance Criteria**:
- [ ] Audit file exists at `.workflows/slow-open-empty-previews-and-zombie-sessions/planning/slow-open-empty-previews-and-zombie-sessions/audit-G-test-helpers.md` and enumerates every `exec.Command`/`exec.CommandContext` call site in the three packages.
- [ ] Every (a)-tagged helper has been updated to take or call the isolated env; verified by reading each updated helper and confirming the env path.
- [ ] Every (b)-tagged helper has a one-line justification.
- [ ] Post-change `grep -rn "exec.Command.*portal\b" internal/portalbintest internal/tmuxtest internal/restoretest` yields only call sites accounted for in the audit (zero un-tagged sites).
- [ ] All existing tests pass: `go test ./...` exits 0.
- [ ] All callers of updated helpers compile and pass.
- [ ] No helper takes an "overload" that omits the env (per spec: "no overload that omits it").

**Tests**:
- `"the audit file's grep footer matches the live grep output"` — meta-check during code review; the audit's captured grep is reproducible.
- `"helpers that spawn portal accept env as parameter"` — for each updated helper, a unit test or compile-time check confirms the signature; the helper rejects nil/empty env if that policy is chosen.
- `"existing daemon-saver integration test passes with the updated helper"` — run an existing test that uses `restoretest`'s portal-spawning helper end-to-end; assert no failures.
- `"the updated helper does not write to the real state dir"` — wrap an existing integration test in `portaltest.NewIsolatedStateEnv`; assert the backstop from Task 1.3 does not fire.

**Edge Cases**:
- Helpers that build the binary but don't spawn portal (`portalbintest.BuildPortalBinary`) → tag (b); justify; no change required.
- Indirect spawn wrappers (a local helper that calls another helper that calls `exec.Command`) → traced to the leaf and tagged once.
- Inline subprocess calls inside `_test.go` files outside the three packages → out of scope for the audit (those are individual tests, not helpers); a note in `CLAUDE.md` (Task 1.5) covers them via contributor discipline.
- A helper currently uses `Cmd.Env = nil` (implicit inheritance) → must be changed to explicit `Cmd.Env = env` with the isolated env.
- A helper currently sets `Cmd.Env` to a hand-curated slice → migrate to `portaltest.NewIsolatedStateEnv` as the canonical source; assert no `XDG_CONFIG_HOME` divergence.
- A helper takes a `stateDir` parameter independent of env → still must set `XDG_CONFIG_HOME` because the spawned binary resolves config via env, not arg.

**Context**:
> Spec § Component G item 2 (audit) and the Audit Deliverable paragraph define the (a)/(b)/(c) classification scheme and the grep-based completion criterion. Item 4 explicitly defers lint enforcement to a future work unit — this task is contributor-discipline + structural helper signature, NOT static analysis.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § "Component G — Test Isolation Contract for `portal state daemon`", items 2 (audit) and 4 (no lint).

## slow-open-empty-previews-and-zombie-sessions-1-5 | approved

### Task 1.5: Document test-isolation contract in CLAUDE.md

**Problem**: The audit + helper land mechanically-mandatory isolation for current callers, but future contributors writing new tests that spawn `portal state daemon` need a discoverable rule. Without documentation, a contributor would not know the helper exists or why it is mandatory.

**Solution**: Add a short subsection to `CLAUDE.md` under "DI / testing pattern" describing the test-isolation contract: the helper, when to use it, why it exists, and the backstop's role. The section is searchable via "test isolation" or "XDG_CONFIG_HOME".

**Outcome**: A contributor encountering a new daemon-spawning test can locate the rule by grep in under 30 seconds. The reasoning (the reporter's bug as canonical example) is captured so the rule's existence is not mysterious.

**Do**:
- Open `CLAUDE.md` and locate the existing "DI / testing pattern" subsection inside the "Architecture" section.
- Append a new subsection titled "Test isolation for daemon-spawning tests" (or similar; the exact heading must contain the phrase "test isolation"). Suggested location: immediately after the existing paragraph about `*Deps` mocks and `t.Cleanup`.
- Content must cover:
  1. **The rule**: any test that runs `portal state daemon` directly OR via `portal open` / bootstrap MUST call `portaltest.NewIsolatedStateEnv(t)` and apply the returned env to the spawned subprocess (`exec.Cmd.Env = env`).
  2. **The reasoning**: a leaked test daemon inheriting the developer's `$XDG_CONFIG_HOME` corrupts the developer's live install; the slow-open / empty-previews / zombie-session incident is the canonical example.
  3. **The helper**: link to `internal/portaltest/isolated_env.go` and document the signature `NewIsolatedStateEnv(t *testing.T) (env []string, stateDir string)`.
  4. **The backstop**: the `t.Cleanup` fingerprint-diff is a defence-in-depth check, NOT a substitute for the env override.
  5. **No lint**: state explicitly that no static enforcement exists; the rule is contributor-discipline + structurally-mandatory helper signature.
- Keep the section short (≤ 15 lines) — CLAUDE.md is reference, not narrative.
- Verify discoverability: after the edit, `grep -i "test isolation" CLAUDE.md` and `grep -i "XDG_CONFIG_HOME" CLAUDE.md` both return the new section.
- Do NOT create a separate `TESTING.md` — the spec accepts either, but a single source of truth in `CLAUDE.md` matches the project's existing pattern (CLAUDE.md is the canonical contributor doc).

**Acceptance Criteria**:
- [ ] `CLAUDE.md` contains a new subsection under "DI / testing pattern" covering the test-isolation contract.
- [ ] The section is searchable via `grep -i "test isolation" CLAUDE.md` AND `grep -i "XDG_CONFIG_HOME" CLAUDE.md` — both return the new section.
- [ ] The section names the helper (`portaltest.NewIsolatedStateEnv`) with its full signature.
- [ ] The section cites the reporter's bug as the canonical example justifying the rule.
- [ ] The section explicitly notes the backstop is defence-in-depth, NOT a substitute for the env override.
- [ ] The section explicitly notes no lint or CI enforcement exists.
- [ ] Section length ≤ 15 lines (concise reference, not narrative).

**Tests**:
- `"CLAUDE.md grep for 'test isolation' returns the new subsection"` — manual verification during PR review; record the grep output in the PR description.
- `"CLAUDE.md grep for 'XDG_CONFIG_HOME' returns the new subsection"` — same.
- `"CLAUDE.md grep for 'portaltest.NewIsolatedStateEnv' returns at least one match"` — confirms the helper is named.
- `"the new section is ≤ 15 lines"` — line-count check.

**Edge Cases**:
- Future tests that spawn portal indirectly (via a shell script, a Makefile target, an integration harness in a different language) → out of scope for this section; the rule covers Go test code only.
- Tests in packages outside `internal/portalbintest` / `internal/tmuxtest` / `internal/restoretest` that spawn portal inline → the rule applies; the audit (Task 1.4) is scoped to those three packages, but the contributor-facing rule applies project-wide.
- `TESTING.md` was an alternative option in the spec — explicitly NOT taken; rationale captured in the "Do" notes above.

**Context**:
> Spec § Component G item 3 (contributor documentation) accepts either CLAUDE.md or a new TESTING.md. The spec's three required content points: (i) any daemon-spawning test MUST use the helper; (ii) the reasoning is that leaked daemons corrupt the developer's install (the canonical example is this incident); (iii) the post-test backstop is defence-in-depth, not a substitute. Item 4 of the spec affirms no lint enforcement — the rule is contributor-discipline plus the structurally-mandatory helper signature.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § "Component G — Test Isolation Contract for `portal state daemon`", items 3 (documentation) and 4 (no lint).
