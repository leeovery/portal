TASK: Retarget stale source comments that cite redesign-deleted files (cmd/state_cleanup.go, attach.go) — internal ID cli-verb-surface-redesign-12-1

ACCEPTANCE CRITERIA (from plan row 12-1):
- Comment-only change (no code/signature/behaviour edit in the three files)
- Four sites retargeted:
  - internal/tmux/hooks_unregister.go:14-15 → point at cmd/uninstall.go (buildUninstallDeps)
  - internal/tmux/hooks_unregister.go:95-96 → point at cmd/uninstall.go (buildUninstallDeps)
  - internal/tmux/tmux.go:382 → point at cmd/uninstall.go (killSaver); internal/tmux/portal_saver.go kept in the KillSession caller list
  - internal/resolver/query.go:307-308 → reworded to a house-style/byte-compat justification naming no deleted file
- //nolint:staticcheck directive and "No session found: %s" string byte-for-byte unchanged
- grep -rn "state_cleanup.go\|attach.go" internal/ returns nothing from these sites
- No new tests; existing UnregisterPortalHooks / KillSession / resolver "No session found" miss suites stay green
- go build ./... succeeds and golangci-lint run clean on the three touched files

STATUS: Complete

SPEC CONTEXT: The cli-verb-surface-redesign retired the `portal attach` command (folded into `portal open --session`) and `portal state cleanup` (folded into `portal uninstall`), deleting cmd/attach.go and cmd/state_cleanup.go. Surviving source comments that still cited those deleted files were stale forensic references. This Cycle-6 analysis chore retargets them to the surviving consumers (cmd/uninstall.go) or rewords them to avoid naming a deleted file, keeping the audit trail accurate. Pure comment maintenance — no behaviour is in scope.

IMPLEMENTATION:
- Status: Implemented (all four sites retargeted correctly; comment-only)
- Location / per-site verification:
  - internal/tmux/hooks_unregister.go:12-18 (bootstrapLogger doc) — now reads "consumed as a function value by cmd/uninstall.go's buildUninstallDeps, which defaults the Unregister seam to tmux.UnregisterPortalHooks". Correct surviving target; no deleted-file name.
  - internal/tmux/hooks_unregister.go:96-99 (UnregisterPortalHooks doc) — now reads "consumed as a function value by cmd/uninstall.go's buildUninstallDeps (which defaults the Unregister seam to tmux.UnregisterPortalHooks)". Correct surviving target; no deleted-file name.
  - internal/tmux/tmux.go:381-385 (KillSession doc) — now reads "including the internal _portal-saver callers (cmd/uninstall.go's killSaver, internal/tmux/portal_saver.go), which gain the prefix harmlessly". Retargeted to cmd/uninstall.go's killSaver; internal/tmux/portal_saver.go correctly RETAINED in the caller list (it is a real surviving caller, not a deleted file). Correct.
  - internal/resolver/query.go:303-310 — reworded to a planner-decision / house-style / byte-compat justification: "the VERBATIM string the retired attach command used, so `open --session` is byte-identical to the former `attach` on the miss path (planner decision)... staticcheck ST1005 silenced per house style; its verbatim text is preserved for byte-compat with the former attach miss path." Names the retired attach *command* conceptually but does NOT name the deleted file attach.go. Satisfies "naming no deleted file".
- Byte-compat invariants (query.go:310): the executable line is unchanged —
  `return nil, fmt.Errorf("No session found: %s", query) //nolint:staticcheck // user-facing message per spec`
  Both the //nolint:staticcheck directive and the "No session found: %s" format string are byte-for-byte intact.
- Deleted-file reference sweep: grep -rn "state_cleanup.go\|attach.go" over internal/ returns a single hit — internal/tui/preview_attach.go:66 — which is a coincidental substring match on the legitimately-existing file preview_attach.go (self-reference), NOT the deleted cmd/attach.go, and is not one of the four in-scope sites. All four retargeted sites are clean of deleted-file names.
- Notes: Change is confined to doc comments in the three named files; no signature, control flow, directive, or string literal was touched. Consistent with the "comment-only" acceptance.

TESTS:
- Status: Adequate (no new tests expected or added — comment-only chore)
- Coverage: Behaviour of UnregisterPortalHooks, KillSession, and the resolver "No session found" miss path is unchanged, so the existing suites remain the coverage of record; the byte-compat "No session found: %s" string is guarded by the existing resolver miss tests. A comment edit cannot regress runtime behaviour.
- Notes: None. Adding tests here would be over-testing.

CODE QUALITY:
- Project conventions: Followed. Comments retarget to real surviving consumers and use precise identifiers (cmd/uninstall.go's buildUninstallDeps / killSaver), matching the codebase's forensic-comment style.
- SOLID principles: N/A (no code change).
- Complexity: Low (unchanged).
- Modern idioms: N/A.
- Readability: Good — the retargeted comments accurately describe the current call graph, improving the audit trail vs the stale pre-edit references.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
