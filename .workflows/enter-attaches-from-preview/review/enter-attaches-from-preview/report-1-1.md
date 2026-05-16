TASK: enter-attaches-from-preview-1-1 — Add SelectWindow method to tmux.Client

ACCEPTANCE CRITERIA:
- Client.SelectWindow(session, window) issues `tmux select-window -t <session>:<window>` exactly once via injected Commander.
- nil Commander error => nil return.
- non-nil error => wrapped error containing "failed to select-window" and target string; unwrappable to *tmux.CommandError via errors.As.
- No `=` exact-match prefix added by this method (target is `<session>:<window>`, not `=<session>:<window>`).

STATUS: Complete

SPEC CONTEXT: Spec § Pre-select + attach sequence > step 2 requires `tmux select-window -t <session>:<window_index>` as a discrete step in the four-call attach pipeline; best-effort failure semantics (log+swallow) live with the caller. Spec § Exact-match target syntax requires the `=` prefix uniformly across has-session / select-window / select-pane / switch-client / attach-session (task 1-2's responsibility, applied on top of 1-1).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go:847-870 (SelectWindow); peer of SelectPane at 872+.
- Notes: Composes `bareTarget = "<session>:<window>"`, prepends `=` for the tmux argv target, dispatches `c.cmd.Run("select-window", "-t", target)`, wraps errors via `fmt.Errorf("failed to select-window %s: %w", bareTarget, err)`. Godoc covers exact-match rationale, best-effort caller contract, and the bare-target choice for error context. The `=` prefix is present here — matching the planned post-task-1-2 state (plan explicitly notes 1-2 wraps 1-1's target). Strictly, task 1-1's own AC item 4 ("No exact-match prefix is added by this method") describes the pre-1-2 intermediate state; the landed commit reflects the post-1-2 merged end state, which is the correct end behaviour. No drift.

TESTS:
- Status: Adequate
- Coverage: internal/tmux/tmux_test.go:2262-2357 — TestSelectWindow with six subtests:
  - composed `=work:2` target argv exact assertion
  - exactly-one Commander.Run call assertion
  - nil error on zero exit
  - wrapped error contains "failed to select-window" and bare target "work:0"
  - errors.As recovery of *tmux.CommandError preserving Stderr
  - exact-match `=` prefix present on the target argv
- Notes: All planned tests present, mirroring TestSelectPane shape. Error-message assertion uses bare target ("work:99"), correctly matching the implementation's choice to keep `=` out of human-readable error text. Focused, not bloated.

CODE QUALITY:
- Project conventions: Followed — uses Commander seam, mirrors SelectPane / SelectLayout shape, fmt.Errorf %w wrap matches package idiom, no Commander bypass.
- SOLID: Good — single-responsibility wrapper.
- Complexity: Low — straight-line.
- Modern idioms: Yes — %w wrap, errors.As-compatible chain.
- Readability: Good — godoc clearly states exact-match rationale, best-effort caller contract, and bare-vs-prefixed target distinction.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The exact-match `=` prefix is hardcoded inline at five sites (HasSession, SelectWindow, SelectPane, SwitchClient, AttachConnector). A single helper (e.g. `exactTarget(session string) string` or extending a PaneTarget-style builder) would centralise the policy. Already discussed in task 1-2's `Do` block; flagged here for completeness only — not a regression of this task.
