# Analysis Report: lazy-resume-on-attach (Cycle 2)

## Stats

- Total findings: 2
- Deduplicated findings: 2
- Proposed tasks: 1

## Summary
Standards found the implementation conforms to the specification and its corrigenda, and raised two comment corrections. Architecture raised one of the same corrections. Duplication raised one finding: the rule that hands a pane to its hook or its bare shell is written out at five sites. The spec requires a lazily-answered pane to match an eager one, and each site's suite pins only its own argv. That finding clears the floor and is staged as one proposal. Two sub-claims inside it do not clear the floor and are discarded below. Architecture's structural finding is low severity, stands alone and is discarded.

## Comment Corrections

- cmd/state_resume_wait.go:103-104 — Ctrl-Z stops the foreground process rather than killing it, and the spec's 2026-09-28 corrigendum changed the wording to "signal". Standards and architecture both raised this. Architecture's version also drops "three". This entry keeps it, because the corrected spec text in §4.3 still says "the three that would ordinarily signal a foreground process". Applying both entries would conflict.
  OLD: // answer the panel, and the three keys that would kill a foreground process are
// bytes like any other under raw mode. No signal is declined: tmux tearing the
  NEW: // answer the panel, and the three keys that would signal a foreground process are
// bytes like any other under raw mode. No signal is declined: tmux tearing the
- internal/capture/resume_surfaces.go:141-143 — the comment claims a surface "joins every enumerating guard". That claim is about tests, and it goes false as soon as a guard is renamed or changes how it enumerates.
  OLD: // the contrast swatch and the resume surfaces. Declared once so a surface added
// later joins every enumerating guard at this edit rather than fataling each of
// them on a name it cannot resolve.
  NEW: // the contrast swatch and the resume surfaces.

## Discarded Findings
- The waiter's answers each restore the tty and re-enable signals by hand (a sub-claim of the duplication finding). Discarded because the divergence is not silent. `TestResumeWait_SignalGenerationOnHandingThePaneOn` (cmd/state_resume_signals_test.go:163) runs Enter and the confirmed discard as separate subtests. Each asserts exactly one enable, after the raw restore and before the exec. Dropping or reordering the step in either answer fails that answer's own subtest. Also, the recovery tail restores a different tty state (fully cooked, per the settled p7 direction), so the pair is not one rule shared across the hand-offs.
- The parked chain's backstop spells `resolveShell`'s fallback as `${SHELL:-/bin/sh}` (a sub-claim of the duplication finding). Discarded as below the floor. The copies could only diverge on a pane whose `$SHELL` is unset, and only on a route reached when the baked binary has gone from its path. Even then the user still lands at a shell. The divergence is not consequential.
- The hydrate helper's resume decision is a mutable pointer each tail must finish, with a nil branch only tests reach (architecture, low). Discarded: it is low severity and does not cluster with any other finding. `runHydrate` always sets the Decision (cmd/state_hydrate.go:112). All three production tails run `markPendingThenUnsetSkeletonMarker`: the replay, `handleHydrateTimeout` and `handleHydrateFileMissing`, the last two wired in production at :434-435. So both outcomes it names need a future edit before they can happen.
