TASK: session-rename-orphans-resume-hook-2-3 (tick-37c4c2) — Add tmux.ListAllPaneHookKeys enumeration (list-panes -a with HookKeyFormat)

ACCEPTANCE CRITERIA:
- ListAllPaneHookKeys exists and delegates to ListAllPanesWithFormat(HookKeyFormat) + parsePaneOutput (uses HookKeyFormat, not StructuralKeyFormat).
- Real-tmux stamped sessions: two sessions stamped with distinct @portal-id -> returned slice contains each session's <id>:w.p (id prefix).
- Real-tmux un-stamped: appears as <name>:w.p.
- Multi-window/multi-pane stamped: distinct <id>:w.p suffixes (all sharing one id).
- list-panes error propagates: underlying list-panes -a failure -> (nil, err) wrapped, NOT empty slice.
- Empty output -> non-nil empty slice ([]string{}).
- ListAllPanes, StructuralKeyFormat, ListAllPanesWithFormat unchanged.
- Real-tmux test NO build tag, skips cleanly via SkipIfNoTmux.
- go build succeeds; go test ./internal/tmux/... passes (skips real-tmux where absent).

STATUS: Complete

SPEC CONTEXT:
Per the specification's "Hook-Key Derivation → Stage 2 Stale cleanup live keys" and "Decoupling / new primitives", stale hook cleanup must build its live-key set from the immutable @portal-id (rename-immune) rather than the name-based structural key. ListAllPanes() hardcodes StructuralKeyFormat and is still consumed by out-of-scope name-based callers (skeleton-marker cleanup, daemon), so its format must stay StructuralKeyFormat. A NEW hook-key-specific enumeration is required; the AllPaneLister wiring switch is deferred to Task 2-4. The discriminating error contract ((nil, err) on tmux failure, NOT empty) is load-bearing because treating a failure as "no live panes" would mass-orphan every hooks.json entry.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go:877-907 (method + doc-comment), placed immediately after ListAllPanes (869-875).
- Notes:
  - Body is a byte-for-byte mirror of ListAllPanes save for the format constant: `raw, err := c.ListAllPanesWithFormat(HookKeyFormat); if err != nil { return nil, err }; return parsePaneOutput(raw), nil`. Uses HookKeyFormat (tmux.go:849), NOT StructuralKeyFormat. Correct.
  - Inherits (nil, err) on failure and []string{} on empty via ListAllPanesWithFormat + parsePaneOutput (parsePaneOutput at tmux.go:540 returns non-nil []string{} on empty input). Correct.
  - Doc-comment (877-900) covers all mandated points: canonical live hook-key enumeration for stale cleanup; stamped -> <id>:w.p, un-stamped -> <name>:w.p; per-pane-row resolution for mixed populations; the discriminating (nil, err) vs empty contract with the mass-orphan rationale; and the explicit SEPARATELY-from-ListAllPanes justification naming the non-hook structural callers (skeleton-marker cleanup, daemon).
  - Unchanged shapes verified: ListAllPanes (869-875) still delegates to ListAllPanesWithFormat(StructuralKeyFormat); ListAllPanesWithFormat (794-800) unchanged (list-panes -a -F <format>); StructuralKeyFormat (829) unchanged; parsePaneOutput (540-555) unchanged. No drift.

TESTS:
- Status: Adequate
- Location: internal/tmux/list_all_pane_hookkeys_realtmux_test.go (package tmux_test, no build tag, SkipIfNoTmux-gated, no t.Parallel()).
- Coverage:
  - TestListAllPaneHookKeys_StampedSession — stamps @portal-id=tok123, asserts "tok123:0.0" present (id branch). Covers stamped criterion.
  - TestListAllPaneHookKeys_UnstampedSession — no stamp, asserts "<name>:0.0" present (name branch). Covers un-stamped criterion.
  - TestListAllPaneHookKeys_MultiWindowMultiPane — one stamped id (tokMulti), split + new-window, asserts {tokMulti:0.0, tokMulti:0.1, tokMulti:1.0} all present. Covers distinct w.p suffixes under one shared id.
  - TestListAllPaneHookKeys_MixedStampedAndUnstamped — stamped (tokMix:0.0) + un-stamped (<name>:0.0) in one server, asserts per-session prefixes resolve independently. Covers mixed population.
  - TestListAllPaneHookKeys_EmptyOutputReturnsNonNilEmptySlice — pure mock (MockCommander{Output:""}) through the public method; asserts err==nil, non-nil, len 0. Correctly pins the inherited empty-output contract end to end without a fabricated live-empty read. MockCommander.Run returns Output for any args (incl. list-panes), so the path is genuinely exercised.
  - TestListAllPaneHookKeys_ListPanesFailurePropagates — ts.KillServer() then enumerate; asserts err!=nil, keys==nil, and errors.As recovers *tmux.CommandError. The socketCommander wraps exec failures via tmux.WrapCommandError, so the *CommandError assertion is valid. Correctly pins the discriminating (nil, err) contract.
- Notes:
  - Test helper symbols all resolve: portalIDLiteral (hookkey_test.go:17, == "@portal-id"), MockCommander (tmux_test.go:14, Run returns Output for any args), tmux.NewClient (tmux.go:107), ts.KillServer (tmuxtest/socket.go:104).
  - The base-index-0 assumption (":0.0" suffixes) is documented in the file header and correct for the tmuxtest harness (-f /dev/null -> base-index 0).
  - Focused, no redundant assertions, no over-mocking. Each test maps 1:1 to a distinct acceptance criterion. Tests assert observable behaviour (resolved key strings, contract shapes), not implementation internals. Balanced — neither under- nor over-tested.

CODE QUALITY:
- Project conventions: Followed. Error wrapping is inherited from ListAllPanesWithFormat's fmt.Errorf("...: %w") so errors.Is/As reach the sentinel chain (matches the codebase's discriminating-error convention). Tests carry no t.Parallel() (mandatory in this repo). Real-tmux guard has no build tag and is SkipIfNoTmux-gated, matching the sibling guards in the package.
- SOLID: Good. Single responsibility (one enumeration variant); the deliberate non-DRY duplication of the 4-line body over ListAllPanes is justified in-source (the two methods must diverge on format and serve different caller sets) — correct call, not premature abstraction.
- Complexity: Low (linear, single error branch).
- Modern idioms: Yes (slices.Contains in tests, errors.As for the recoverable-error assertion).
- Readability: Good. Doc-comment is thorough and self-documenting; the SEPARATELY-from-ListAllPanes rationale prevents a future "just repoint the shared method" regression.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
