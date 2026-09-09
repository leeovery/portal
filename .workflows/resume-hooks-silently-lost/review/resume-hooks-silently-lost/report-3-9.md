TASK: resume-hooks-silently-lost-3-9 — One Name For Reading A Pane Token In Fixtures (tick-31679f)

ACCEPTANCE CRITERIA:
- `internal/tmuxtest` exposes one pane-token read beside the write, taking a pane target
- `readPaneToken` and the inline `display-message` read are both gone
- The three raw-tmux assertions in `internal/tmux/resolve_hookkey_realtmux_test.go` and the `livePaneRows` enumeration are untouched
- No assertion changes its verdict
- `go test ./...` passes; `go test -tags integration -p 1 ./...` passes

STATUS: complete

SPEC CONTEXT:
This is an analysis-cycle consolidation task appended to phase 3 (the specified phase-3 tasks are 3-1..3-5 in
`.workflows/resume-hooks-silently-lost/planning/resume-hooks-silently-lost/phase-3-tasks.md`; 3-6..3-9 are ad hoc
tasks generated during implementation, so the task body is the authority, not the specification). The surrounding
phase-3 work carries the pane token across the reboot gap: `Pane.PortalPaneID` is captured into `sessions.json`
and re-stamped onto the paired live pane at restore. The fixtures that verify that durability all need to read
`@portal-pane-id` back off a live pane, which is the read this task gives one home. Purely test-scaffolding work;
no production code is touched.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmuxtest/stamp.go:18-33` — `(*Socket).ReadPaneToken`, sited directly beside `StampPaneToken`
    (`:11-16`), implemented as the `show-options -p -t <target> -v @portal-pane-id` read, `""` on non-zero exit.
  - `internal/restore/rename_reboot_hook_integration_test.go:78` and
    `internal/restore/rename_reboot_durability_integration_test.go:59` — both re-pointed at the helper.
  - `cmd/state_daemon_hook_cleanup_integration_test.go:101` — the inline `display-message` read replaced.
  - `internal/restore/rename_reboot_shared_test.go` — `readPaneToken` deleted; the file's imports are down to
    `path/filepath`, `testing`, `internal/hooks`, `internal/state`, all still used (`:5-11`).
- Notes:
  - Delivered by commit `f4e259ea`. Its diff touches exactly the four Go files above plus the task/manifest
    records — `internal/tmux/resolve_hookkey_realtmux_test.go` and
    `cmd/noncontiguous_window_reboot_integration_test.go` are absent from the commit, so the two "leave alone"
    constraints hold. Both are still in their protected form today: the raw assertions at
    `internal/tmux/resolve_hookkey_realtmux_test.go:80-87` and `:97-103` still drive `TryRun` directly and assert
    on exit status and the `invalid option` message text, and `livePaneRows`
    (`cmd/noncontiguous_window_reboot_integration_test.go:415-420`) is still the single
    `ListAllPanesWithFormat` whole-server enumeration with its rationale comment at `:408-414`.
  - Two deliberate divergences from the Do list, both sound. (1) The Do list said to pass
    `tmux.PaneTarget(name, 0, 0)` at the restore sites; the commit did exactly that, and a later task (`9-17`,
    commit `3ccac0d7`, "a pinned target is a type, not a naming convention") moved them to
    `tmux.PaneTargetExact(renameNewName, 0, 0)`. That is strictly stronger — it pins the session half against
    tmux's prefix match — and the callers' assertions are unchanged. (2) The same later task retyped the helper's
    parameter from `string` to `tmux.Target`, which is what "taking a pane target" now means in this tree; the
    criterion is met in substance and more strongly than as written.
  - A third fixture-level pane-token read survives deliberately: `readHookKey`
    (`internal/tmux/hookkey_format_realtmux_test.go:11-15`) reads through `tmux.HookKeyFormat` with
    `display-message`, which is the production mechanism under test in that file — folding it into
    `ReadPaneToken` would stop it testing anything. This is recorded in the work unit's manifest bank against
    task 3-9, so a later consolidation sweep will not mistake it for a fourth copy. Correct call.
  - `ReadPaneToken` collapses "option unset" and "target no pane answers to" into the same `""` (both make
    `show-options` exit non-zero). That conflation is carried over verbatim from `readPaneToken`, as the task
    directed, and cannot mislead any current caller: all three compare against a known non-empty token, so a
    bogus target fails the assertion rather than silently answering with a neighbour's token as
    `display-message` would. Recorded as an observation, not a finding.
  - Repo-wide check: no other `show-options -p … @portal-pane-id` read and no other inline pane-token
    `display-message` read remains outside `internal/tmuxtest` and the two protected sites.

TESTS:
- Status: Adequate
- Coverage: No new test, as the task specifies, and none is warranted — the helper is a fixture read exercised by
  three integration callers (`internal/restore` rename-reboot hook + durability suites,
  `cmd/state_daemon_hook_cleanup_integration_test.go`), each of which compares its result against an expected
  token and fails loudly on `""`. A regression in the helper cannot pass silently at any of them.
- Notes: No assertion moved and none changed its verdict. The restore sites' pre-change call
  (`readPaneToken(t, ts, renameNewName)`) composed the identical argv against the identical target; the `cmd`
  site swapped `display-message`'s value for `show-options`'s on a pane stamped one line earlier, where both
  yield the same token. `internal/tmuxtest`'s own suite covers `WaitForSession` and `SendKeys` only, matching the
  pre-existing treatment of `StampPaneToken` — the pair stays symmetric.

CODE QUALITY:
- Project conventions: Followed. The helper lives in `internal/tmuxtest` (test-only package, never imported by
  production), takes `*testing.T` first like its `StampPaneToken` sibling, calls `t.Helper()`, and composes
  `@portal-pane-id` from `state.PortalPaneIDOption` rather than restating the literal. The file stays untagged,
  which is required — its callers span both lanes.
- SOLID principles: Good. One method, one job; the read and the write are now a named pair on one type.
- Complexity: Low — a single command, one error branch.
- Modern idioms: Yes.
- Readability: Good. The doc comment carries the rationale the task asked for (why `show-options` and not
  `display-message`) and states a fact about tmux that was measured elsewhere in this work unit rather than
  restating the code.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
