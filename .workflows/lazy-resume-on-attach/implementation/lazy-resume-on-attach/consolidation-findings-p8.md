# Consolidation Findings: lazy-resume-on-attach (Phase 8)

## Findings

None. The phase's one task routed all five hook-or-shell hand-offs through `handOffToHookOrShell`. The superseded `execShellAndExit` is deleted outright with no remaining references, and the parked chain stays on `execHandOff`, as planned. No duplication, drift, accretion or dead code survives the bar.

## Comment Corrections

- cmd/state_resume_chain.go:150-152 — the stated reason is gone. The phase moved the eager tail off `lookup.Found` and onto the command itself, so `handOffToHookOrShell`, the only production caller of `hookExecArgs`, now refuses an empty command by its own branch. What the normalisation still protects is the `Found`-gated wait decision (`cmd/state_hydrate.go:198`): without it, a commandless registration would park a pane on a panel.
  OLD: // resumeRegistrationOrLog answers the zero value for a registration carrying no
// command as it does for a miss, so no caller can hand hookExecArgs an empty
// command and compose sh -c "; exec $SHELL".
  NEW: // resumeRegistrationOrLog answers the zero value for a registration carrying no
// command as it does for a miss, so a pane with nothing to run never waits.

- cmd/state_resume_registration_test.go:34-37 — same stale reason for keeping the branch. Once `handOffToHookOrShell` branches on the empty command, that branch is no longer what stops `sh -c "; exec $SHELL"`.
  OLD: 	// The production lookup answers a stored value carrying no command with the
	// zero result, so only a fake can hand this rule a registration found with
	// an empty command. The branch is kept so no caller can compose
	// sh -c "; exec $SHELL" if that ever stops being true.
  NEW: 	// The production lookup answers a stored value carrying no command with the
	// zero result, so only a fake can hand this rule a registration found with
	// an empty command. The branch is kept so a pane with nothing to run never
	// waits if that ever stops being true.
