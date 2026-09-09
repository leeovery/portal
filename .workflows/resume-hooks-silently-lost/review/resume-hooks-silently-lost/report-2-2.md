TASK: resume-hooks-silently-lost-2-2 — Registration Writes The Pane's Own Token

ACCEPTANCE CRITERIA:
- [x] `hook set` in a live unstamped pane mints a token, issues exactly one `set-option -p` carrying `state.PortalPaneIDOption`, and writes exactly one `hooks.json` entry keyed by that same token
- [x] The written key satisfies the token-shape predicate and equals the value handed to the stamper
- [x] `hook set` on a pane already carrying a token writes under that token and issues **no** `set-option` — no second token is minted
- [x] A failed stamp ends the command non-zero and leaves `hooks.json` byte-identical (absent stays absent)
- [x] A stamp that succeeds followed by a failed write leaves the pane's token in place — no unstamp, no rollback — and a retry reuses it
- [x] `hook set` never writes an empty key: a mint failure is a non-zero exit with nothing written
- [x] `@portal-pane-id` is composed from `state.PortalPaneIDOption` at both the format and the stamp; the literal is restated nowhere
- [x] `hook` remains bootstrap-exempt — no tmux server is started by any of these calls
- [x] `hook rm --pane-key <key>` still issues no tmux call at all and is behaviourally unchanged
- [x] CLAUDE.md describes the hook key as the pane token

STATUS: complete

SPEC CONTEXT:
§2.1 declares the durable pane identity as the tmux pane user-option `@portal-pane-id`, named **once** in `internal/state` and composed from that constant everywhere else (the literal being written in exactly one place is what retires the binding guards rather than re-pointing them, §9.4). §2.2 makes stamping lazy at `hook set` — read first, mint only on empty read-back, a pane already carrying a token keeps it. §3.1 fixes the key as the token alone, no session component and no coordinates, the composite `<portal-id>:<pane-token>` having been considered and rejected because `move-pane -t <other-session>` reintroduces drift. §4.1 fixes the order — probe existence, read the token, mint+stamp an unstamped pane, write, touch `save.requested` — with steps 4 and 5 unreorderable and step 4's failure ending the command; the mirror state (stamp landed, write failed) is left exactly as it is, **no rollback**, because unstamping races a concurrent registration that already read the token. §4.3 keeps `--pane-key` a literal pass-through touching tmux not at all.

Two corrigenda bear on this task and are honoured by the delivered code: the generator and predicate live in `internal/nanoid` rather than `internal/session` (2026-08-30), and the `internal/session/panetoken.go` forwarder the plan's Do-list named was later deleted, `cmd/hooks.go` reaching the leaf directly, with the leaf split into a general-purpose `width` and a `paneTokenWidth` (2026-09-01). The plan text naming `internal/session` is therefore superseded by the record, not diverged from.

IMPLEMENTATION:
- Status: Implemented (moved past the plan's letter in ways the corrigenda and later phases record)
- Location:
  - `internal/state/markers.go:26` — `PortalPaneIDOption = "@portal-pane-id"`, the sole production home of the literal
  - `internal/tmux/tmux.go:631` — `const HookKeyFormat = "#{" + state.PortalPaneIDOption + "}"`, composed by constant concatenation
  - `internal/tmux/tmux.go:232` — `ResolveHookKey`, the option-less `show-options -p` existence probe followed by the `display-message` read of that format
  - `internal/tmux/tmux.go:314` — `SetPaneOption(target Target, name, value string) error` running `set-option -p -t <target> <name> <value>`, option name from the caller, no `-g`/`-s`
  - `cmd/hooks.go:30` / `:45` / `:46` — the `PaneOptionSetter` one-method seam, the `PaneStamper` field and the `TokenMinter` field
  - `cmd/hooks.go:67` — `resolveCurrentPaneKey` returning `(hookKey, paneID, err)` so the caller stamps the pane it read
  - `cmd/hooks.go:83` — `hookSeams`, filling each unset seam with its production default
  - `cmd/hooks.go:108` — `stampPaneToken`, mint then `SetPaneOption(paneID, state.PortalPaneIDOption, token)`, tmux's error passed back unaltered
  - `cmd/hooks.go:190` — `hooksSetCmd.RunE`: resolve → mint+stamp iff empty → `store.Set` → best-effort `save.requested` touch, in that order
  - `internal/nanoid/nanoid.go:35`/`:58`/`:65` — `NewPaneTokenGenerator`, `paneTokenWidth`, `IsTokenShaped` reading the same width
- Notes:
  - The literal `@portal-pane-id` appears in exactly one production file (`internal/state/markers.go:26`); the other Go occurrences are three test files pinning it deliberately, and one guard fixture that models a violation. The binding guard `cmd/portal_id_binding_guard_test.go` is deleted rather than re-pointed, as §9.4 prescribes.
  - No rollback and no unstamp exist on any path — `hooksSetCmd.RunE` returns the store error and does nothing else.
  - `hook` remains in `skipTmuxCheck` (`cmd/root.go:28`); the stamp is a pane write on a server `$TMUX_PANE` already implies.
  - The `--pane-key` branch reaches neither `resolveCurrentPaneKey` nor the stamper, so it takes no tmux call.

TESTS:
- Status: Adequate
- Coverage: every acceptance criterion has a test that would fail if the behaviour broke.
  - `cmd/hooks_pane_token_test.go` carries all seven of the task's named cmd-level cases under their prescribed titles: freshly-minted stamp-and-write (one `set-option`, correct target and option name, entry under the stamped value, value token-shaped), token reuse with zero `set-option` calls, nothing written on a failed stamp with the error text pinned to tmux's own words, stamp-before-write pinned by an `onCall` hook that stats the file at stamp time, the stamp standing after a denied write, and the empty-key case driven by a failing `TokenMinter`. `TestHooksSetRefusesAnUnresolvablePane` additionally pins zero mints and zero stamps when the probe fails.
  - The retry half of "a retry reuses it" is covered where it can actually be observed — `cmd/hooks_write_lock_test.go:114` drives a real second run through the `stampedPane` fake and asserts mint count 1, stamp count 1 and one entry under the token the failed attempt stamped.
  - `internal/tmux/pane_option_test.go` pins the `set-option -p` argv exactly, the scope (no `-g`/`-s`), and the failure wrap.
  - Real-tmux coverage: `internal/tmux/resolve_hookkey_realtmux_test.go` resolves a stamped pane to its token, a live unstamped pane to `""`, a gone pane to a `*tmux.CommandError`, and pins the raw tmux facts the two-read discrimination rests on; `internal/tmux/hookkey_format_realtmux_test.go` proves the stamp is per-pane not per-session (split sibling and other-window pane both read empty); `internal/tmux/hookkey_cross_site_realtmux_test.go` pins `ResolveHookKey(pane)` equal to that pane's enumeration row `Token`.
  - `cmd/hooks_seams_test.go` pins each seam's production default, including that the default minter produces a value `IsTokenShaped` accepts; `cmd/hooks_pane_token_width_guard_test.go` is a source guard failing a `TokenMinter` defaulted to the general-purpose generator, which is what keeps the two equal-width generators from silently swapping.
  - `cmd/root_test.go:303` runs `hook set`/`rm`/`list` (and the `hooks` alias rows) through `PersistentPreRunE` with a recording orchestrator, pinning bootstrap-exemption.
- Notes: The `--pane-key` case poisons **both** pane seams and asserts both call counts, so a regression that reached either one fails loudly rather than passing on the un-poisoned half. No test asserts on implementation detail beyond the argv contract, which is the subject.

CODE QUALITY:
- Project conventions: Followed. The seam family follows the `*Deps` + `withXDeps` convention and is staged through `withHooksDeps`, so `cmd/seam_guard_test.go` covers it. All new tests are unit-lane; the real-tmux ones are per-test `-L` socket client tests, which CLAUDE.md admits to the fast lane, and none builds or spawns a binary.
- SOLID principles: Good. `PaneOptionSetter` is a one-method interface declared at the consumer; the compile-time `_ PaneOptionSetter = (*tmux.Client)(nil)` assertion sits beside its siblings.
- Complexity: Low. `hooksSetCmd.RunE` is a straight-line sequence with one conditional; `stampPaneToken` is six lines.
- Modern idioms: Yes — `%w` wrapping throughout, `errors.AsType` in the tests, a `Generator` func type rather than a one-method interface for the mint.
- Readability: Good. The stamp-before-write ordering carries the one-line rationale the task asked for (`cmd/hooks.go:206-207`), and it states the consequence rather than restating the code.
- Issues: None. Every comment on the changed code holds against it: the `HookKeyFormat` doc's "un-stamped pane yields an empty key" is what the real-tmux suite measures; `ResolveHookKey`'s doc describes the two reads the body performs and the reason the probe names no option; `stampPaneToken`'s "returning tmux's own error unaltered" is true of that function (`return "", err`), and the single Portal clause the user sees is added inside `SetPaneOption`, which `TestGonePaneErrorCarriesOnePortalClause` pins on the sibling path.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
