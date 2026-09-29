## Attempt 1

ISSUES:
- `internal/tui/pagepreview_waiting_pane_test.go:89` (with the file staged at `:95`). The subtest "a waiting pane whose token is not token-shaped" passes with the `nanoid.IsTokenShaped` check at `internal/tui/preview_adapter.go:28` removed.
  - With the check gone, `PendingToken = "../"+token` becomes `scrollback/pane-../<token>.bin`, a path under a `pane-..` directory that does not exist. I confirmed this path with `filepath.Join`.
  - That read returns `(nil, nil)` and falls back to the positional file. The staged `scrollback/pane-<token>.bin` can never be reached from this token.
  - So the check the adapter's comment relies on for path safety is not tested. A token like `/../../x` does leave the scrollback directory: it resolves to `<stateDir>/x.bin`.
  - The executor's report says this malformed-token case is covered; it is not.
  FIX: Give the table a per-case staged token and stage a file this token actually reaches.
  - For the malformed case, use a token that fails `IsTokenShaped` but still names a real file, e.g. `token[:len(token)-1]` (one character short, alphabet only).
  - Call `writeTokenBinFile(t, stateDir, <that token>, []byte("token transcript\n"))` for that case, keeping the positional file as now.
  - Without the check the adapter would then read "token transcript", so the `"positional transcript"` assertion tests the check.
  - Keep the real `token` staged for the other two cases, whose outcome does not change.
  ALTERNATIVE: Also add a directory-escape case with `PendingToken = "/../../" + token` and a file staged at `filepath.Join(stateDir, token+".bin")`. This tests the escape the adapter comment names; the width-short case alone tests only the shape rule. I recommend adding both, since each is cheap.
  CONFIDENCE: high

NOTES:
- No test covers the adapter's decision to return a token-file read error rather than fall back (`preview_adapter.go:30-32`). If it were simplified to `if bytes != nil`, an unreadable token file would fall back to the positional file. For a moved pane that file can hold a different pane's capture, which the preview would then show. The trigger (a permission failure on a 0600 file the daemon wrote as the same user) is rare, so I have not made this blocking. A test using `themetest.DenyRead` on the token file would take a few lines.
- Adding a waiting pane to the hermetic lifecycle fixture (`pagepreview_hermetic_test.go:23`) is harmless. That test checks call counts only, and its reader records `PaneKey` alone, so the waiting pane's `PendingToken` is never checked there. `TestPreviewWaitingPane_ReaderIsHandedTheFocusedPanesTokenOnlyWhileItWaits` checks it instead.
- The row parser now needs exactly 5 fields (`internal/tmux/tmux.go:646-648`). A hand-set `@portal-pane-id` value containing `\x1f` would make the preview refuse to open for that whole session. This is a hand-edit case, not raised.
- Nothing beyond the enumerated inputs arrived with the dispatch.
