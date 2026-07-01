TASK: 1-5 — Retire the four stale hooks.json-ownership doc-comments in internal/tmux/tmux.go (PaneTarget, PaneTargetExact, StructuralKeyFormat, ListAllPanes); transfer the stability invariant to HookKey / HookKeyFormat.

ACCEPTANCE CRITERIA:
- None of PaneTarget/PaneTargetExact/StructuralKeyFormat/ListAllPanes doc-comments claim to be the canonical hooks.json key or lookup, and none asserts that changing that formatter invalidates hooks.json.
- PaneTarget/PaneTargetExact docs still describe name-based -t targeting and the exact-match prefix guidance.
- StructuralKeyFormat/ListAllPanes docs still describe structural-key enumeration for non-hook structural use (@portal-skeleton-* markers, cleanup-path agreement) and the error-propagating contract.
- The stability invariant is documented on both HookKey and HookKeyFormat.
- The four functions' bodies/signatures are unchanged (documentation-only diff).
- cmd/bootstrap/stale_marker_cleanup.go and cmd/state_daemon.go are untouched.
- go build succeeds; full go test ./... green.

STATUS: Complete

SPEC CONTEXT:
Spec (Hook-Key Derivation -> Deliverable — retire the stale doc-comments) names this an explicit deliverable and frames it as a load-bearing invariant TRANSFER, not a rename. PaneTarget stays exactly as-is as the canonical name-based -t target formatter (Decoupling from tmux.PaneTarget); the hook key becomes a separate concern owned by HookKey / HookKeyFormat. The four doc-comments today falsely assert name-based formatters ARE the canonical hooks.json key; leaving them would invite a future caller back into name-based keying — re-establishing the exact drift this fix removes. StructuralKeyFormat / ListAllPanes retain their non-hook structural role (@portal-skeleton-* markers via SanitizePaneKey, sessions.json delta/merge matching).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go
  - PaneTarget (566-581): describes it as the single canonical name-based session:window.pane -t target formatter; retains "callers issuing -t MUST use PaneTargetExact" guidance; adds an explicit "This formatter is NOT the hook key ... do not reintroduce name-based keying here" clause cross-referencing HookKey/HookKeyFormat. No hooks.json ownership / invalidation claim remains.
  - PaneTargetExact (583-599): describes the "=" exact-match prefix and exact-match -t target resolution; the "do not mix the two" prefix guidance is re-phrased in terms of -t target resolution (wrong pane), not hooks.json. No "PaneTarget remains the canonical hook-key formatter" claim remains.
  - StructuralKeyFormat (818-829): describes it as the load-bearing join key between live-pane enumeration and @portal-skeleton-* marker names; retains the "two cleanup paths (stale-marker cleanup and orphan-FIFO sweep) agree" invariant and the "drift here would desync the cleanup paths" statement, minus the hook-lookup claim; adds "This is a name-based structural key, NOT the hook key" cross-reference. The format literal is unchanged.
  - ListAllPanes (851-875, incl. ResolveStructuralKey cross-reference): describes structural-key enumeration for non-hook structural use (@portal-skeleton-* marker matching, sessions.json delta/merge), explicitly "not for hook-key lookup (see HookKey / HookKeyFormat)"; retains the error-propagating / discriminating contract. No "used as the lookup key in hooks.json" or "intersect with persisted hook entries" framing remains.
  - HookKey (601-623): carries the transferred invariant verbatim — "This format is load-bearing: it is stable across releases — changing it silently invalidates every entry in hooks.json" — with an explicit note that this is the invariant formerly on PaneTarget.
  - HookKeyFormat (831-849): carries the transferred invariant — "this format is load-bearing and MUST stay stable across releases: changing it silently invalidates every hooks.json entry".
- Notes: The ResolveStructuralKey doc-comment (320-322), named in the spec as part of ListAllPanes' scope, is clean and makes no hooks.json claim — correct. A codebase-wide grep for the old stale phrasings ("canonical hooks.json", "hook lookup table", "used as the lookup key in hooks.json", "remains the canonical hook-key formatter", "persisted hook entries in hooks.json") returns zero matches across internal/ and cmd/. Residual "hooks.json" mentions in tmux.go are confined to HookKey / HookKeyFormat / ListAllPaneHookKeys / the transfer-annotation clauses — all legitimate.

TESTS:
- Status: Adequate (no new tests — correct for a documentation-only change)
- Coverage: The four function bodies/signatures are unchanged and remain protected by existing behavioral tests: TestPaneTarget (tmux_test.go:2862) asserts PaneTarget's output format; ListPanes/ListAllPanes/ResolveStructuralKey tests (1462/1710/1735) assert StructuralKeyFormat is the format string passed to tmux. These are behavioral assertions, not comment-content guards, so they neither require nor received a lockstep update.
- Notes: No doc-guard/grep-based test asserting comment content exists in the tmux package, so none needed updating (task's conditional instruction satisfied by absence). Adding a comment-content assertion test would be over-testing for a documentation-only change and was correctly not done.

CODE QUALITY:
- Project conventions: Followed — comments follow golang-documentation style (full-sentence GoDoc starting with the identifier); cross-references to HookKey/HookKeyFormat are precise and the invariant transfer is annotated in-source.
- SOLID principles: N/A (documentation-only; no logic changed).
- Complexity: N/A — function bodies untouched (PaneTarget/PaneTargetExact one-line fmt.Sprintf; ListAllPanes delegates to ListAllPanesWithFormat(StructuralKeyFormat)).
- Modern idioms: N/A.
- Readability: Good — each comment now states the formatter's true remaining purpose, explicitly disclaims hook-key ownership, and points to the owning primitive, which structurally discourages the name-based-keying regression the spec warns about.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
