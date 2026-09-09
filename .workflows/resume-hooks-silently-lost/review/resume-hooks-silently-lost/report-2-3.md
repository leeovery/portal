TASK: resume-hooks-silently-lost-2-3 — Registration Refuses A Pane That Is Not There
(give `ResolveHookKey` a tmux-native `show-options -p` existence probe, naming no option, ahead of its token read)

ACCEPTANCE CRITERIA:
1. `ResolveHookKey` against a pane id no pane answers to returns a non-nil error and an empty key, from the probe, before the token read runs
2. `ResolveHookKey` against a live pane carrying no pane options at all returns `("", nil)`
3. `ResolveHookKey` against a stamped pane returns its token
4. The probe names no option; the token's value comes from the `display-message` read
5. `hook set` on an unresolvable `$TMUX_PANE` exits non-zero, writes nothing to `hooks.json`, mints nothing and stamps nothing
6. The error surfaced to the user carries tmux's own words, and no production code inspects that text
7. The returned error keeps `*tmux.CommandError` recoverable via `errors.As`
8. Real-tmux coverage pins the raw facts the probe rests on (`set-option -p -t %999 …` non-zero; `display-message -p -t %999 '<format>'` exit 0)

STATUS: complete

SPEC CONTEXT:
§4.1 (specification.md:176-206) states the write-time rule: `portal hook set` refuses a `$TMUX_PANE` naming no live pane, exits non-zero and writes nothing, and it is verified tmux-natively rather than by a shape heuristic, because `display-message -p` against a bogus target exits 0. The section fixes the six-step order (`requireTmuxPane` → probe → token read → mint+stamp → write → touch `save.requested`), states that naming the option in the probe collapses the discrimination (`invalid option`, exit 1, indistinguishable from `no such pane`), and states that Portal never parses tmux's message text — exit status is the whole signal and tmux's words pass through unaltered. §4.2 notes the `$TMUX_PANE` removal path inherits the same guard (its wording is Phase 4's). §9's coverage table requires the discrimination to be pinned against a real server through Portal's own argv, with the raw tmux facts asserted alongside. No corrigendum touches this task.

IMPLEMENTATION:
- Status: Implemented (matches the spec's step order and rationale; later phases retyped the parameter to `tmux.Target`, which is consistent with the record)
- Location:
  - `internal/tmux/tmux.go:226-245` — rewritten doc comment describing the two reads and what each answers (226-231); the probe `show-options -p -t <target>` with no option name, returning `("", wrapped)` on non-zero exit before the token read (236-238); the unchanged `display-message` token read (240-243)
  - `internal/tmux/tmux.go:233-235` — the required one-line-plus comment stating the probe must name no option and why (same exit status as a missing pane)
  - `cmd/hooks.go:67-79` — `resolveCurrentPaneKey` propagates the resolver's error verbatim (its doc comment at 63-66 says why it adds no second clause)
  - `cmd/hooks.go:194-224` — `hook set` returns on the resolve error before the flag read, the mint/stamp (207-211) and the store write (213-220)
  - `CLAUDE.md:60` — the `ResolveHookKey` clause now names the two-call resolution, the no-option rule and the "no production code parses tmux's message text on this path" property
- Notes:
  - Only production call site of `ResolveHookKey` is `cmd/hooks.go:73` (verified by repo-wide grep; all other references are tests), so the probe's blast radius is exactly the two CLI verbs the spec intends.
  - The wrap is `fmt.Errorf("no pane answers to %q: %w", …)` over the `*CommandError` that `runCommand` (`internal/tmux/tmux.go:52-62`) produces via `WrapCommandError`, so `errors.As` recovery and tmux's stderr both survive; `(*CommandError).Error` (`internal/tmux/command_error.go:24-54`) renders argv, exit status and stderr.
  - The clause "no pane answers to" is asserted from an exit status that cannot prove that specific cause (a dead server renders the same phrase). This is the shape §4.1 prescribes — exit status is the whole signal — and tmux's own words follow in the same string, so it is recorded as an observation, not a finding.
  - No `strings.Contains`/text classification runs on this path. The one stderr-matching helper in the package (`optionAbsentStderrPatterns`, `internal/tmux/tmux.go:18-22`) is reached only from the server-option getters, not from `ResolveHookKey`.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/tmux/resolve_hookkey_realtmux_test.go:29-45` — bogus `%999` against a **live** server: non-zero, empty key, `errors.As(*tmux.CommandError)`, tmux's "no such pane" preserved (AC 1, 6, 7)
  - `:47-61` — a live pane asserted to carry no pane options at all resolves to `("", nil)` (AC 2)
  - `:63-75` — a stamped pane resolves to its token (AC 3)
  - `:77-92` — the negative control: `show-options -p -t <live pane> @portal-pane-id` fails with `invalid option`, and the same live pane still resolves through Portal's own call (AC 4)
  - `:94-104` — the raw facts: `set-option -p` against `%999` non-zero, `display-message -p` against it exit 0 (AC 8)
  - `internal/tmux/resolve_hookkey_test.go:15-38` — fake `Commander`: the probe's failure short-circuits, exactly one call recorded, argv asserted as `show-options -p -t %999` with no option name (AC 1, 4)
  - `:40-68` — the two-call sequence and argv on the passing path
  - `:70-92` — the `display-message` read-failure branch (the pane-closes-between-the-two-reads race): empty key, `errors.As`, stderr preserved
  - `internal/tmux/target_type_test.go:132-147` — byte-identical argv for both reads through the typed target
  - `cmd/hooks_test.go:431-454` — `hook set` with an injected `KeyResolver` failure: non-zero, tmux's words unaltered, `errors.As`, `hooks.json` never created (AC 5, 6, 7)
  - `cmd/hooks_pane_token_test.go:182-207` — mint count 0 and zero `set-option` calls when the resolve fails (AC 5)
  - `cmd/hooks_seams_test.go:91-135` — both verbs driven through a real `tmux.NewClient` over a scripted commander that refuses only the probe argv; pins the whole rendered message (`no pane answers to "%999": tmux show-options -p -t %999: no such pane: %999`) — one Portal clause plus tmux's words, and any other argv fails the test, which pins the probe itself without a server
- Notes:
  - Lane placement is correct: the real-tmux file is a per-test `-L` socket client test with no daemon, no built binary and no build tag — the carve-out CLAUDE.md sanctions for the unit lane. The `cmd` propagation tests use the `withHooksDeps` staging helper (no direct `hooksDeps` assignment, so `cmd/seam_guard_test.go` stays satisfied) and never touch a real server.
  - The re-point the task asked for landed: the old `TestResolveHookKey_ReadFailureWrapsError` and its "tmux tolerates a bogus `-t` target, so killing the server is the only way" comment are gone, and the premise they rested on is now asserted positively in `:94-104`. The read-failure branch they used to cover was re-covered by `resolve_hookkey_test.go:70-92` rather than dropped.
  - Not over-tested: the argv appears in three assertions (probe short-circuit, passing sequence, typed-target identity), but each has a distinct subject — ordering, composition, and target-type spend — and none is a redundant restatement of another.
  - Tests were assessed by reading; none were executed.

CODE QUALITY:
- Project conventions: Followed. Target typing, the `Commander` seam, the `*CommandError` wrap contract, the `*Deps` staging helpers and the unit/integration lane rule are all respected; the probe adds no exported surface, as the task required.
- SOLID principles: Good — the discrimination stays inside the one method that owns it, so both CLI verbs inherit it through `resolveCurrentPaneKey` with no duplicated rule.
- Complexity: Low — one added guard clause and one early return.
- Modern idioms: Yes (`errors.AsType` in the tests, `%w` wrapping, typed target spent at the argv boundary).
- Readability: Good. The doc comment states what each read answers and that an empty key means resolved-but-unstamped; the inline comment states the non-obvious rule (name no option) that the code cannot express.
- Comment accuracy: Verified. The claim that naming the option is rejected with the same exit status as a missing pane is asserted against a real server at `internal/tmux/resolve_hookkey_realtmux_test.go:77-92`, and the empty-key claim at `internal/tmux/tmux.go:229-231` is asserted at `:47-61`.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
