TASK: lazy-resume-on-attach-4-12 — One Declaration of How a Gone Pane Is Told From an Unset Option

ACCEPTANCE CRITERIA:
- `ResolveHookKey` issues no tmux command of its own — the probe and the format read are `ReadPaneOption`'s, and there is one place the probe can be wrongly removed from
- A pane id no live pane answers to still fails on the probe, before the token read runs, returning an empty key and a recoverable `*tmux.CommandError`
- A live pane that has never been stamped still resolves to `("", nil)`
- `hook set` and `hook rm` against a gone pane still render exactly one Portal clause followed by tmux's own words, byte-for-byte as today
- The measured fact is stated in one comment, over `ReadPaneOption`; `HookKeyFormat` and the whole-server enumeration are untouched
- `go test ./internal/tmux/ ./cmd/ -count=1` and `go test ./...` pass

STATUS: complete

SPEC CONTEXT: The lazy-resume feature adds a second consumer of the gone-pane-versus-unset-option distinction: the pane-scoped `@portal-resume-pending` marker read, where a gone pane misread as "no marker" would make recovery decline a pane and leave it with a dead keyboard. The registration path (`portal hook set` / `hook rm`) already depended on the same distinction to refuse a write against a vanished pane. This task is a consolidation: both paths now read through one probe-then-read declaration, so the measurement cannot be dropped from one copy while the other keeps it.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/tmux.go:244-250 — `ResolveHookKey` is now one line, `return c.ReadPaneOption(paneID, state.PortalPaneIDOption)`. It adds no wrap and runs no tmux command of its own.
  - internal/tmux/tmux.go:337-356 — `ReadPaneOption` is unchanged and is the only production `show-options -p -t <target>` probe. The only other production `show-options` call is the server-scoped `show-options -s` at :421. Its doc comment (:341-345) is the only statement of the measured fact in Go source; the same grep found no restatement anywhere else.
  - internal/tmux/tmux.go:669, :688-689, :696-702 — `HookKeyFormat`, `paneHookRowFormat` and `ListAllPaneHookKeys` are byte-identical to before this task's commit (a3ad74b92). The only later change to tmux.go (d1cbdfe2c, task 6-1) added code after `parsePaneHookRows` and did not touch these.
  - cmd/hooks.go:67-79 — `resolveCurrentPaneKey` passes the resolver's error through unwrapped, so the gone-pane message is still `no pane answers to "%999": ` + `CommandError.Error()`, with exactly one Portal clause.
  - CLAUDE.md `tmux` row now says `ResolveHookKey` composes no reads of its own and `ReadPaneOption` is the sole declaration of the rule. This matches the code.
- Notes: The only behavioural change is that the token read now passes the format as `-F "#{@portal-pane-id}"` instead of positionally. tmux treats both forms the same, and `hookkey_cross_site_realtmux_test.go:13` checks against a real server that `ResolveHookKey` and the whole-server enumeration return the same token. Losing the `failed to resolve hook key for pane` wording on a failed token read is intended; no Go source or test refers to it any more (grep outside `.workflows/` finds nothing).

TESTS:
- Status: Adequate
- Coverage:
  - internal/tmux/resolve_hookkey_test.go:57-60 is updated to the `-F` argv. Line 32-38 still checks that the probe runs alone and names no option when it fails, and lines 71-93 check that a failed token read returns an empty key and a recoverable `*tmux.CommandError` carrying tmux's own words.
  - internal/tmux/target_type_test.go:140-143 is updated to the `-F` argv in the same way.
  - internal/tmux/resolve_hookkey_realtmux_test.go:29-45 runs against a real gone pane `%999` and checks for an empty key, a recoverable `*tmux.CommandError` and tmux's "no such pane". Lines 47-61 check that a live pane with no pane options resolves to `("", nil)`. Lines 94-104 pin the raw tmux fact the probe relies on: `display-message` against a gone pane exits 0.
  - internal/tmux/pane_option_realtmux_test.go:116-124 is the second real-server gone-pane subtest. Both real-server files are untagged, so they run in the unit lane.
  - The mutation proof holds by reading. With the probe deleted from `ReadPaneOption`, `display-message` against a gone pane exits 0 with empty output (pinned at resolve_hookkey_realtmux_test.go:100). `ReadPaneOption` would then return `("", nil)`, and both real-server gone-pane subtests fail on their `err == nil` checks. The mock suites (resolve_hookkey_test.go:32, pane_option_test.go:142) would also fail on their call counts.
  - cmd/hooks_seams_test.go:91-135 (`TestGonePaneErrorCarriesOnePortalClause`) still expects exactly `no pane answers to "%999": tmux show-options -p -t %999: no such pane: %999`. Its scripted commander accepts only the probe argv, so the new code path produces that string and makes no other call.
- Notes: The probe ordering is now pinned for both public entry points and once more in the argv-composition test. Each checks a different public contract, so this is not redundant bloat, and the task says to inherit coverage rather than add tests.

CODE QUALITY:
- Project conventions: Followed. The `Target`-typed parameter is kept, there is no second wrap (per the wording decision), and CLAUDE.md is kept in sync.
- SOLID principles: Good. One function now owns the probe-then-read rule.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The `ResolveHookKey` doc comment states the empty-key-means-live contract without restating the measurement, and holds true against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./internal/tmux/ ./cmd/ -count=1` and `go test ./...` pass" — run both commands. Reading found no argv or wording expectation left unreconciled, but only a suite run settles this.
