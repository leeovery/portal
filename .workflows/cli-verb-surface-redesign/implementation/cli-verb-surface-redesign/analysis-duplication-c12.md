AGENT: duplication
FINDINGS:
- FINDING: Prefix-match completer body authored twice
  SEVERITY: low
  FILES: cmd/completion.go:40-48 (completeSessionNames), cmd/completion.go:81-89 (completeAliasKeys)
  DESCRIPTION: The two shell-completion helpers introduced by the redesign's Tab
    Completion feature are structurally identical: each iterates a candidate-source
    function, keeps the entries prefix-matching `toComplete`, and returns
    `(matches, cobra.ShellCompDirectiveNoFileComp)`. The only difference is the
    candidate source (completionSessionNames vs completionAliasKeys). This is the
    same ~8-line loop written independently in two places. It is minor in size, but
    it is a genuine near-duplicate against this codebase's own fastidious
    single-sourcing standard — the surrounding open/burst code goes out of its way
    to single-source even one-line strings (singleMissError, commandAttachOnlyMessage,
    GoneMessage, runtimeDownResult), so leaving the prefix-filter loop copied twice
    is inconsistent with the established local convention and invites drift (e.g. a
    future change to the directive or the match rule landing in only one).
  RECOMMENDATION: Extract a single unexported helper, e.g.
    `prefixCompleteNoFileComp(candidates []string, toComplete string) ([]string, cobra.ShellCompDirective)`,
    that owns the prefix-filter loop and the ShellCompDirectiveNoFileComp return.
    Have both completeSessionNames and completeAliasKeys call it with their
    respective source slice. No behaviour change; consolidates the completion match
    rule to one site.

- FINDING: "No session found: %s" user-facing message authored in two packages
  SEVERITY: low
  FILES: cmd/kill.go:39, internal/resolver/query.go:326 (ResolveSessionPin)
  DESCRIPTION: The exact user-facing miss string `fmt.Errorf("No session found: %s", …)`
    (both carrying the same `//nolint:staticcheck` house-style directive) is written
    independently in the kill command's HasSession validation and in the resolver's
    session-pin miss path. They emerged separately — kill predates the redesign; the
    -s/--session pin miss is new and is specced to be byte-identical to the retired
    `attach`. Within internal/resolver the sibling misses are already single-sourced
    (unknownAliasError for "No alias found", and "No zoxide match" inline), so this is
    the one session-miss wording that lives in two authoring sites and can silently
    diverge on a copy edit. Impact is small: a single format string, and the two sites
    sit across the cmd↔resolver package boundary, so consolidation is only worth it if
    the shared-wording contract matters more than the boundary cost.
  RECOMMENDATION: If the "byte-identical session-miss wording" contract is worth
    enforcing, hoist the string to one authoring site both sites reference — e.g. an
    exported resolver helper (mirroring unknownAliasError) that kill.go also calls, or
    a shared cmd-level constant. Otherwise, leave as-is and record it as an accepted
    two-site string; do not add a new abstraction solely for a one-liner.

SUMMARY: The redesign surface is heavily consolidated — the two burst orchestrations
  (cmd/open_burst_run.go vs internal/tui) already delegate every shared count-semantic,
  message renderer, log shape, split, and pre-flight primitive to internal/spawn, and
  the doctor/store stale-detection predicates are single-sourced per package; the
  deliberate per-caller divergences (SplitTriggerFirst vs SplitNetN, stderr vs flash)
  are documented design, not accidental duplication. Only two low-severity residual
  near-duplicates remain: the shell-completion prefix-filter loop copied across two
  completers, and the "No session found" string authored in both cmd/kill.go and the
  resolver. Both are optional consolidations, not high-impact.
