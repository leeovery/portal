AGENT: architecture
FINDINGS:
- FINDING: Stale field-role contract left on the ClientActivity type doc after the gate inversion
  SEVERITY: low
  FILES: internal/spawn/detect_inside.go:9-13
  DESCRIPTION: The change correctly inverted the locality gate and fully rewrote
    the detectInsideTmux function docstring so client_activity is now described as
    the cross-client primary winner-selection signal. But the ClientActivity type
    doc at the top of the same (rewritten) file still describes the Activity field
    as "the local-only tiebreak" — the exact contract phrase the fix falsified.
    Activity is no longer a local-only disambiguator; it selects the triggering
    winner across ALL clients (local and remote alike). This is precisely the
    residual the spec's "Owned Behaviour Change" section named must-not-remain
    ("Do not leave the old contract text in place describing behaviour the code no
    longer has"; §66 flags "used ONLY to disambiguate among host-local clients" as
    "the exact inversion of the new rule"). The type-definition contract is the
    seam a future reader consults first; leaving it describing the pre-fix role
    invites re-introduction of filter-then-tiebreak reasoning. No runtime effect —
    doc-contract consistency only, hence low. NOTE: the source-of-truth mirror
    type tmux.ClientInfo (internal/tmux/clients.go:11-12) carries the identical
    now-falsified phrase ("Activity is the local-only tiebreak used to choose
    among 2+ host-local clients"), but that file is outside this plan's scope and
    was not changed — flagged here only as corroborating drift, not as in-scope work.
  RECOMMENDATION: Update the ClientActivity type doc (detect_inside.go:11) so the
    Activity field's described role matches the new contract — e.g. "its
    last-activity timestamp (the cross-client winner-selection signal — the
    most-active client is the burst's trigger)". Keep it consistent with the
    already-correct function docstring below it.
SUMMARY: The gate inversion is architecturally sound — the clientLister/walker/reader seam and the detectInsideTmux(session, ...) signature are preserved intact, selectTriggeringClient is a clean single-responsibility extraction, and the new winner-walk-transient outcome folds correctly through Detect()'s existing NULL+WARN path so all callers inherit the fix with no downstream change. The only issue is a low-severity documentation-contract residue: the ClientActivity type doc still labels Activity a "local-only tiebreak," the very phrase the fix inverted. The deliberately-manual end-to-end verification is spec-sanctioned, not a test gap.
