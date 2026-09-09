TASK: resume-hooks-silently-lost-8-47 — "Hydrate-Config Residue: One Fall-Through Has No Test And One Literal Survives The Builder" (tick-21c3e5, phase 8, implementation-analysis cycle)

ACCEPTANCE CRITERIA:
- Both file-missing fall-throughs fail if the branch is deleted.
- No fixture carries a load-bearing nil implicitly.
- The builder either expresses explicit-nil, or its stated reach matches the two places it actually is.

STATUS: complete

SPEC CONTEXT: The specification does not legislate `runHydrate`'s handler fall-throughs; it governs the hooks store's locking and the fact that the hydrate helper is the *sole* firing path for a resume hook (`LookupOnResume`, spec lines 349/379). Per the shared verifier context, a phase 6–10 task's authority is its own body, so this task is judged against its own Do list and criteria. The surrounding invariant that makes the task meaningful is real: `cmd/state_hydrate.go` is the one process that turns a saved pane back into a hooked shell, and its degraded routes (signal timeout, scrollback absent, scrollback unreadable mid-stream) are the paths a user actually lands on after a reboot, so each needs a standing proof it still exists.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_hydrate_file_missing_fallthrough_test.go:1-83` (new file; the two fall-through proofs)
  - `cmd/state_hydrate_test.go:952-962` (`hydrateAbsentHandler` + `hydrateHandlersWired`/`hydrateAbsentTimeout`/`hydrateAbsentFileMissing`)
  - `cmd/state_hydrate_test.go:989` (`hydrateCfgOpts.AbsentHandler`), `:1009-1032` (stand-in defaults assigned aside, then `clearAbsentHandler`), `:1034-1055` (`clearAbsentHandler`)
  - `cmd/state_hydrate_timeout_log_test.go:114-141` (the former inline `hydrateConfig` literal now routes through the builder with `AbsentHandler: hydrateAbsentTimeout`)
  - `cmd/state_hydrate_timeout_log_test.go:52-58` (the implicit nil `HookStore` made explicit with its reason)
- Notes:
  - Do 1 (proofs for both nil-`HandleFileMissing` branches) — done. The two branches are still exactly where the task named them: `cmd/state_hydrate.go:122` (`return fmt.Errorf("open scrollback %s: %w", …)`) and `cmd/state_hydrate.go:137` (`return err`, the mid-copy path), re-read at HEAD.
  - Do 2 (the one remaining implicit load-bearing nil) — done at `cmd/state_hydrate_timeout_log_test.go:52-58`. That case asserts the timeout path's `INFO exec` line, which is only reachable through the bare-shell branch of `execShellOrHookAndExit` (`cmd/state_hydrate.go:174-178`), so the nil is genuinely load-bearing and is now stated with its reason. Sampling the sibling cases: `state_hydrate_test.go:422/454/480/876/1191/1213` and `state_hydrate_file_missing_log_test.go:164` all already carry an explicit `HookStore: nil` with a reason; `state_hydrate_empty_hookkey_test.go:70/115` and `hooks_read_lock_test.go:126` name a real store. The one case I found that omits `HookStore` while touching the exec line — `state_hydrate_replayed_log_test.go:161-162` — asserts only that an `INFO exec` line exists and follows `scrollback replayed`, which holds for a hit, a miss or an error alike, so its nil is not load-bearing.
  - Do 3 (the builder's explicit-nil gap) — done by the first branch of the disjunction: `AbsentHandler` gives the builder a vocabulary for "this case's subject IS a nil handler". `hydrateConfig{…}` composite literals now number exactly two in the whole repo — the builder at `cmd/state_hydrate_test.go:1019` and the production cobra wiring at `cmd/state_hydrate.go:275` — so the "a new required field is added once" claim is now literally true for tests. No stale statement of the old two-places figure survives in source (searched `cmd/` for the claim; the only "one route" comments are in `config_precedence_single_source_test.go`/`config_identity_test.go` and concern config paths, not this builder).
  - The design is sound rather than merely compliant: `clearAbsentHandler` runs *after* the loud-stand-in defaults, so an omitted handler stays loud and only a case that names its absence gets a nil — which preserves the property the suite's stand-ins exist for (a stray excursion into an unnamed route surfaces as an error rather than satisfying another route's assertions). Naming a handler and declaring it absent is rejected with `t.Fatal` rather than silently resolved.
  - No production code changed; the only later edit to `cmd/state_hydrate.go` (`git diff ee845058 HEAD`) is a different task's `LookupOnResume` call-shape change at line 179, which does not touch these branches.

TESTS:
- Status: Adequate
- Coverage:
  - `TestHydrate_NilHandleFileMissing_OpenFailureReturnsError` / "it falls through when no file-missing handler is set" (`cmd/state_hydrate_file_missing_fallthrough_test.go:12-44`) — real FIFO + signalling goroutine, a non-existent scrollback path, `AbsentHandler: hydrateAbsentFileMissing`. Asserts a non-nil error, `errors.Is(err, fs.ErrNotExist)`, that the message carries `open scrollback <path>`, and that `ExecShell` was NOT called.
  - `TestHydrate_NilHandleFileMissing_CopyFailureReturnsError` / same subtest name (`:46-83`) — a directory as the scrollback (opens cleanly, first `io.Copy` read fails EISDIR), asserting the error is an `*os.PathError` returned verbatim, that it is *not* wrapped in `open scrollback`, and that `ExecShell` was NOT called.
  - Both would fail if their branch were altered: replacing `cmd/state_hydrate.go:122`'s wrapping with a bare `return err` breaks the message assertion; wrapping `:137` breaks the "not `open scrollback`" assertion; returning nil from either breaks the non-nil check; and removing the `HandleFileMissing != nil` guard so the handler is always called panics on a nil func. That is the standing proof the nil-`HandleTimeout` sibling (`cmd/state_hydrate_timeout_log_test.go:114-141`) already had.
  - The directory-as-scrollback shape is the suite's established idiom for open-succeeds-then-read-fails (`cmd/state_hydrate_test.go:1937-1971`), and `errors.AsType[T]` is the repo-wide idiom (`main.go:63`, `cmd/state_hydrate_test.go:1893`, and others) on go 1.26.
- Notes:
  - No over-testing: three assertions per case, each pinning a distinct property (error identity, wrapping/non-wrapping, no exec). No duplication with the handler-present cases at `cmd/state_hydrate_test.go:1866-1971`, which exercise the opposite branch.
  - Lane and isolation rules hold: unit lane, no build tag needed (no portal binary, no daemon, no real tmux — `hydrateCfg` wires `tmux.NewClient(commandertest.Quiet())`, and neither case reaches the marker-unset call). No `t.Parallel()`, per CLAUDE.md.
  - The plan's Tests list also named `"it carries an explicit nil hook store"`. No test by that name exists, and none is warranted: the criterion behind it ("no fixture carries a load-bearing nil implicitly") is a property of the fixture's own declaration, satisfied at `cmd/state_hydrate_timeout_log_test.go:52-58`, and the case carrying it (`TestHydrateTimeoutLog_SignalTimeoutPrecedesExecINFO`) is itself the test that depends on that nil. Nothing is untested as a result, so this is not reported as a finding.

CODE QUALITY:
- Project conventions: Followed. Unit-lane placement, no `t.Parallel()`, `t.Run("it …")` subtest naming matching the surrounding suites, `commandertest.Quiet()` rather than a hand-rolled fake, `logtest`-backed capture untouched, imports all used in both edited files (`bytes`/`commandertest` still live at `cmd/state_hydrate_timeout_log_test.go:82` and `:124` after the `tmux` import was dropped).
- SOLID principles: Good. The absent-handler decision is a separate, single-purpose function (`clearAbsentHandler`) rather than more branching inside the builder, and the enum names the one thing it decides.
- Complexity: Low. One three-arm switch with a fail-closed `default`.
- Modern idioms: Yes — typed `uint8` enum with `iota`, generic `errors.AsType[*os.PathError]`, `errors.Is` traversal rather than string matching for the identity check.
- Readability: Good. Every non-obvious choice carries its reason: why the stand-ins are assigned aside rather than back into `opts` (`cmd/state_hydrate_test.go:1009-1010`), why the zero value means "both wired" (`:952-955`), why a directory is the copy-failure shape (`cmd/state_hydrate_file_missing_fallthrough_test.go:50-51`), and why the timeout case's nil store matters (`cmd/state_hydrate_timeout_log_test.go:55`).
- Comment accuracy: The comments hold against the code. `clearAbsentHandler`'s "runs after the stand-in defaults so the named handler ends up nil rather than loud" matches the call order at `:1019-1031`; "a case cannot both name a handler and declare it absent" matches the two `t.Fatal` guards. No process-artifact references (no task ids, phases or spec sections) anywhere in the changed text.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
