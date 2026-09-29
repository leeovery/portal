# Consolidation Findings: lazy-resume-on-attach (Phase 11)

## Findings

None.

## Comment Corrections

- internal/state/capture.go:230-234 — the refusal does not stop one pane being handed another's history: in the phase's own renumbered layout (Y tokened, saved at work:3.0 and now at work:2.0; X tokenless, now at work:3.0), the refused X still commits a record naming `scrollback/work__3.0.bin`, its live-address positional path. That is Y's saved file, and it still holds Y's bytes after `CaptureAndRefile` links Y onto `pane-<token>.bin`. Measured in a scratch copy, with the same result before and after this phase. The clause claims a protection the rule does not give. The token half of the clause is true and stays.
  OLD: // indexPrevPanes reads prev in canonical order, so a token held by more than
// one record resolves to the first of them. A record whose token a live pane
// carries is left out of byAddress: it belongs to the pane answering to that
// token, so a pane at its old address must never take it — which would commit
// one token on two records and hand one pane another's history.
  NEW: // indexPrevPanes reads prev in canonical order, so a token held by more than
// one record resolves to the first of them. A record whose token a live pane
// carries is left out of byAddress: it belongs to the pane answering to that
// token, so a pane at its old address must never take it — which would commit
// one token on two records.
