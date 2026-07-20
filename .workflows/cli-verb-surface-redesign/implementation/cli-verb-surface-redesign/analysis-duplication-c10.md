AGENT: duplication
FINDINGS:
- FINDING: doctor down-server short-circuit repeated across three runtime checks
  SEVERITY: low
  FILES: cmd/doctor.go:483-487 (checkDaemonAlive), cmd/doctor.go:511-515 (checkSaverUp), cmd/doctor.go:542-546 (checkHooksRegistered)
  DESCRIPTION: The three runtime health checks each open with the identical
    down-server guard — `const name = "..."` followed by
    `if !serverUp { return checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning} }`.
    This is a clean Rule-of-Three instance: the same "runtime not running" result
    shape is constructed verbatim in three sibling functions. The shared detail
    string is already single-sourced as the `doctorRuntimeNotRunning` constant,
    which signals the intent to consolidate the whole result, not just its text.
    A future change to how a down server is reported (e.g. switching from checkFail
    to a distinct status, or adjusting the marker) would have to be applied in three
    places in lockstep. Small blocks (3 lines each), so impact is modest.
  RECOMMENDATION: Extract a `runtimeDownResult(name string) checkResult` helper in
    cmd/doctor.go that returns `checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning}`,
    and call it from the `!serverUp` arm of all three checks. Purely a consolidation
    of existing code — no behaviour change.

- FINDING: gone-flag banner + map seeding duplicated between live handler and capture-harness option
  SEVERITY: low
  FILES: internal/tui/burst_preflight_abort.go:43-51 (handlePreflightAbort), internal/tui/burst_preflight_abort.go:77-83 (WithInitialGoneFlagged)
  DESCRIPTION: Both sites set the abort banner via `spawn.GoneMessage(names)`, then
    allocate `m.goneFlagged = make(map[string]struct{}, len(names))`, populate it in a
    `for name := range names { m.goneFlagged[name] = struct{}{} }` loop, and call
    `refreshSessionDelegate()`. The only divergence is that the live handler also
    prunes the selection (`delete(m.selectedSessions, name)`) inside the loop; the
    harness seeder omits it. The banner + map-build + refresh sequence is otherwise
    identical. This is a near-duplicate of the gone-flag seeding logic; a copy edit to
    the banner assignment or the map construction would need to land in both. One of
    the two sites is a capture-harness-only Option (WithInitialGoneFlagged), so impact
    is limited to keeping the harness in sync with the live path.
  RECOMMENDATION: Extract a small `(m *Model) seedGoneFlags(names []string)` that sets
    `abortBannerText`, builds `goneFlagged`, and refreshes the delegate; have
    WithInitialGoneFlagged call it directly and handlePreflightAbort call it and then
    apply the selection prune separately. Consolidates existing code only.

- FINDING: completion prefix-filter logic duplicated between session-name and alias-key completers
  SEVERITY: low
  FILES: cmd/completion.go:40-48 (completeSessionNames), cmd/completion.go:81-89 (completeAliasKeys)
  DESCRIPTION: The two completer functions are structurally identical: iterate a
    candidate slice from a package-level source seam, keep entries where
    `strings.HasPrefix(candidate, toComplete)`, and return
    `(matches, cobra.ShellCompDirectiveNoFileComp)`. They differ only in the source
    function (completionSessionNames vs completionAliasKeys). Note this is only two
    instances — the project's own DRY standard (code-quality.md) says to avoid
    extraction for code used once or twice and to extract after three — so this sits
    just under the Rule-of-Three bar and is reported as a borderline observation, not a
    clear defect. If a third Portal-owned namespace ever gains prefix completion, this
    becomes a firm extraction candidate.
  RECOMMENDATION: Optional. If/when a third completer appears, extract a
    `prefixFilterCompletions(candidates []string, toComplete string) ([]string, cobra.ShellCompDirective)`
    helper both call. Until then, leaving the two copies is consistent with the stated
    two-instance DRY posture.
SUMMARY: The implementation is unusually well consolidated — nearly every cross-file
  spawn/burst/resolve/store pattern is already single-sourced through shared helpers
  (internal/spawn message.go / logemit.go / classify.go / split.go / command.go /
  ackid.go, cmd runHookStaleCleanup, storelog.EmitCleanStaleSummary, project/hooks
  StaleEntries/StaleKeys predicates), and the two burst orchestrations are deliberately
  distinct with documented divergences rather than accidental copies. Only three
  low-severity residuals remain, all small (a 3x down-server guard in doctor, a
  gone-flag seeding near-duplicate, and a 2-instance completion prefix-filter);
  none is high-impact.
