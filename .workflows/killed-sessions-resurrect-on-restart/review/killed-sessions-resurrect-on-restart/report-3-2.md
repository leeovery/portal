TASK: killed-sessions-resurrect-on-restart-3-2 — Refresh `buildHydrateCommand` doc comment and confirm `RespawnPane` interface signature is unchanged (still single command-string)

ACCEPTANCE CRITERIA:
- buildHydrateCommand's doc comment no longer references `sh -c`, `exec $SHELL`.
- New doc comment states the bare form, the `respawn-pane -k` atomic-replace contract, and absence of parked parent process.
- RespawnPane signature remains `(target, command string) error`.
- No production code outside buildHydrateCommand modified.

STATUS: Complete

SPEC CONTEXT: Spec § "Argument Quoting" — bare-form, single command-string argument to RespawnPane; signature unchanged. § "Inner Hook-Firing Wrapper Is Untouched" — only outer wrapper dropped. Doc-only task; behaviour shipped by 3-1 (finalised by 8-1).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/restore/session.go:408-436 — refreshed doc comment + bare-form function.
  - /Users/leeovery/Code/portal/internal/tmux/tmux.go:569-583 — RespawnPane signature unchanged.
- Notes:
  - Doc comment opens with bare-form contract + respawn-pane -k atomic replace rationale.
  - References spec Fix 3 (Defect D); explains unreachable trailer, parked parent, double-exit bug.
  - Documents apostrophe-input safety caveat post-8-1.
  - Grep for `sh -c|exec \$SHELL` against session.go returns no matches.
  - RespawnPane signature at tmux.go:577: `func (c *Client) RespawnPane(target, command string) error`. No []string anywhere.

TESTS:
- Status: Adequate (no new tests required)
- Coverage: TestSessionRestorer_HydrateCommandFormat (session_test.go:568-594) asserts bare-string emitted shape. TestRespawnPane cases exercise unchanged interface.

CODE QUALITY:
- Project conventions: Followed.
- Complexity: Low. Body is one fmt.Sprintf.
- Readability: Good. Structured (what / why-removed / safety-caveat), spec-referenced.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Doc comment conveys "absence of parked parent" only via past-tense rationale. A present-tense statement on the post-change invariant would more cleanly satisfy the AC bullet.
