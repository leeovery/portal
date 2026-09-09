TASK: resume-hooks-silently-lost-9-35 (tick-3abf95) — "The Isolated-Env Shell Pins Are Inert On This Platform, And The Sandbox Temp Dir Leaks On Every Run"

ACCEPTANCE CRITERIA:
- [x] `ZDOTDIR` points outside the framework temp tree, and the temp HOME holds no `.zsh_history` after a fixture whose tmux server hosted an interactive shell.
- [x] Every remaining shell pin has a stated effect on this platform; the ones that neutralise nothing are gone or re-voiced.
- [x] `go test ./internal/portaltest` leaves no `portaltest-self-sandbox-*` directory in `$TMPDIR`.
- [x] The self-test's exit code is the run's code, not the cleanup's.
- [x] The existing quiescence wait and fingerprint backstop are unchanged in behaviour.

STATUS: complete

SPEC CONTEXT: This is a phase-9 implementation-analysis task, so its own body is the authority rather than the specification. Its subject is `internal/portaltest`'s test-isolation helper, which CLAUDE.md makes binding for every daemon- or server-spawning fixture in the tree ("ABSOLUTE INVARIANT — a test must NEVER mutate or affect the real system"). The concrete failure it names is narrower than that invariant: the helper's shell env pins read as isolation but were measured inert on macOS (`/etc/zshrc` assigns `HISTFILE=${ZDOTDIR:-$HOME}/.zsh_history` unconditionally, so a pane shell's history landed in the temp HOME the framework is about to `RemoveAll`), and the package's own `TestMain` leaked a sandbox directory per run because `os.Exit` skips `defer`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/portaltest/isolated_env.go:37-47` — rewritten comment block plus the two surviving pins: `HISTFILE=os.DevNull` (:46) and `ZDOTDIR` (:47) taken from the new helper. `SHELL_SESSIONS_DISABLE` is gone.
  - `internal/portaltest/isolated_env.go:113-127` — `shellConfigDirOutsideTempTree`, an `os.MkdirTemp("", "portaltest-shellconfig-*")` outside the framework tree with its own `t.Cleanup` removal.
  - `internal/portaltest/isolated_env_test.go:14-36` — `TestMain` now `os.Exit(runInSelfSandbox(m.Run))`; the helper captures `run()`'s code, removes the sandbox, and returns that code.
- Notes:
  - Cleanup ordering is correct and matches what the code claims. Registration order is: first `t.TempDir()` base RemoveAll (`:33`) → shellconfig RemoveAll (`:125`) → `ZDOTDIR` restore (`:47`) → quiescence wait (`:53`) → backstop (`:107`). LIFO therefore runs the quiescence wait *before* the shellconfig removal and well before the framework's TempDir RemoveAll, which is exactly what the doc comment at `:116-117` asserts.
  - `resolveDevStateDir` (`internal/portaltest/fingerprint.go:286-294`) and `registerDirQuiescenceGuard` (`internal/portaltest/teardown_guard.go:69-79`) are untouched, so AC 5 holds; the pre-snapshot still runs after the HOME scrub.
  - The comment's claim that the retained `HISTFILE` pin "buys the shells with no such override: a bash pane writes `.bash_history` into HOME without it" holds on this platform — macOS `/etc/bashrc` sets no `HISTFILE`, so bash honours an inherited one. The pin is therefore stated rather than decorative.
  - Pointing `ZDOTDIR` at a freshly-created empty directory also keeps a pane shell off the developer's `~/.zshrc`, consistent with `tmuxtest.New`'s `-f /dev/null`.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/portaltest/isolated_env_test.go:310-339` — `ZDOTDIR` is set from a decoy, then asserted non-empty, outside the framework tree (`under` at :341), present on disk as a directory, and carried exactly once in the returned env slice.
  - `internal/portaltest/isolated_env_realtmux_test.go:16-48` — the real-shell verification the task demanded rather than asserting by construction: a real `/bin/zsh -i` pane on an isolated `tmuxtest` socket runs a command (so zsh has history to save), exits, the server is awaited gone, and the temp HOME is asserted *entirely* empty. This is a true regression detector — revert `ZDOTDIR` to `homeDir` and `/etc/zshrc` puts `.zsh_history` back in HOME, failing the read.
  - `internal/portaltest/isolated_env_test.go:38-62` — `TestRunInSelfSandbox` drives the helper with a stub run returning 7, asserts the returned code is the run's own and the sandbox is stat-not-exist afterwards. The `t.Setenv("HOME", os.Getenv("HOME"))` pair at :42-43 correctly restores the binary-wide values the nested `os.Setenv` clobbers.
  - `internal/portaltest/teardown_guard_test.go:90-98` — the "still registers the quiescence guard over the isolated HOME" check survives, asserting the registrar is handed `os.Getenv("HOME")`.
  - `internal/portaltest/backstop_ordering_test.go:27-65` — the backstop's resolution-after-scrub and delta-reporting behaviour still pinned, covering AC 5's second half.
- Notes:
  - The new real-tmux fixture is untagged (unit lane) and complies with the lane rule: it builds no portal binary, spawns no daemon and execs no built binary. It has in-package precedent in `internal/portaltest/tmux_server_wait_realtmux_test.go:13`, and it skips cleanly when tmux or `/bin/zsh` is absent (:17-21).
  - It also satisfies the repo's own coverage guard `TestTeardownGuardCoversEveryServerHostingFixture` (`internal/portaltest/teardown_guard_coverage_test.go:160`): isolate (:23) → guard (:24) → server (:27), in that order.
  - Not over-tested: the two `ZDOTDIR` subtests assert different properties (location on disk vs propagation into the env slice), and no assertion duplicates another.

CODE QUALITY:
- Project conventions: Followed. Test-only helper stays in `internal/portaltest` with the structurally-mandatory `*testing.T` first parameter (`shellConfigDirOutsideTempTree(t *testing.T)`); no `t.Parallel()`; no production import of the package; the fixture isolates before starting its server as CLAUDE.md requires.
- SOLID principles: Good — the new helper does one thing (create the out-of-tree dir and register its removal) and the caller composes it inline at the `t.Setenv` site.
- Complexity: Low.
- Modern idioms: Yes — `os.MkdirTemp` + `t.Cleanup`, `strings.CutPrefix` in the env readers, `filepath.Rel`-based containment check rather than string prefixing.
- Readability: Good. The comment at `isolated_env.go:37-45` names the actual writer (`/etc/zshrc`'s unconditional `HISTFILE` assignment), says why only `ZDOTDIR` moves it, and states what the retained `HISTFILE` pin still buys — which is precisely the "re-voice or retire" the task asked for. The `RemoveAll` error at `:125` is deliberately discarded, which is right: a shell still flushing there must not fail a test.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
