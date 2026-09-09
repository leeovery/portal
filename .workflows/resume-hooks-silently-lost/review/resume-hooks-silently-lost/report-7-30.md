TASK: resume-hooks-silently-lost-7-30 — `session.NewPaneToken` is a vestigial forwarder that keeps pane identity in the session package

ACCEPTANCE CRITERIA:
- `internal/session/panetoken.go` no longer exists and nothing references `session.NewPaneToken`.
- `cmd/hooks.go` does not import `internal/session`.
- `HooksDeps.TokenMinter` is `nanoid.Generator` and defaults to `nanoid.NewGenerator()`.
- `hook set` mints and stamps exactly as before: a pane with no token gets one, a pane already carrying one keeps it and no `set-option` is issued.
- A minting failure still ends the command non-zero with nothing written to `hooks.json`.
- Both lanes pass.

STATUS: complete

SPEC CONTEXT: The specification's §3.2/§9 arrangement originally put the pane-token mint in `internal/session` (`NewPaneToken`, specification.md:74) forwarding to the shared generator the shape predicate reads. Corrigendum 2026-08-30 moved the vocabulary to the stdlib-only `internal/nanoid` leaf with `internal/session/panetoken.go` forwarding to it; **corrigendum 2026-09-01 (specification.md:560) records this task's own outcome as authoritative**: "the forwarder was deleted and `cmd/hooks.go` reaches the leaf directly, and the leaf now holds **two** unexported widths — a general-purpose `width` behind `NewGenerator` and `paneTokenWidth` behind `NewPaneTokenGenerator`, which is the width `IsTokenShaped` reads … A source guard pins the mint to the pane-token generator, since both widths are 6 today and the swap would otherwise be invisible." The safety property underneath is `hooks.json`'s on-disk key-recognition contract: a key minted at a width `IsTokenShaped` does not read becomes unjudgeable, and the staleness rule then retains it forever.

IMPLEMENTATION:
- Status: Implemented (with one deliberate, recorded divergence from the criteria's literal wording — see Notes)
- Location:
  - `cmd/hooks.go:46` — `TokenMinter nanoid.Generator` (was `session.IDGenerator`).
  - `cmd/hooks.go:99-101` — `seams.TokenMinter = nanoid.NewPaneTokenGenerator()` (was `session.NewPaneToken`).
  - `cmd/hooks.go:3-13` — import block: `internal/nanoid` present, `internal/session` absent; no `session.` selector survives anywhere in `cmd/hooks.go` or the `cmd/hooks_*` test files.
  - `internal/session/panetoken.go` and `internal/session/panetoken_test.go` — deleted in commit `67ffc1b7`; `internal/session/` now holds create/naming/dirresolve/prepare/quickstart only.
  - `cmd/hooks_pane_token_width_guard_test.go:17-52` — the re-homed source guard (`TestHookSeams_MintsTokensAtThePaneTokenWidth`).
  - `CLAUDE.md` — the `session` row's `panetoken.go` sentence removed, and the Resume-hooks paragraph re-pointed at `nanoid.NewPaneTokenGenerator`; both claims hold against the tree.
- Notes:
  - **AC3's literal `nanoid.NewGenerator()` is not what landed, and should not have been.** Task 7-5 (commit `3bd37a7f`, which precedes this one) split the leaf into a general-purpose `width` and a `paneTokenWidth` and moved the mint onto `NewPaneTokenGenerator`; the task body for 7-30 was authored against the pre-7-5 tree, where "the wrapper adds no width" was true. Both widths are `6` (`internal/nanoid/nanoid.go:22,58`), so `NewGenerator()` would compile and every runtime assertion would still pass — while silently re-coupling `hooks.json`'s persisted-key contract to the width the leaf's own comment declares free to move. Defaulting to `NewPaneTokenGenerator()` is the substance of "mints exactly as before"; the divergence is from stale wording, is recorded in the commit message and in the spec's 2026-09-01 corrigendum, and is a strict improvement. Not a finding.
  - The pre-existing negative guard lived inside the deleted `internal/session/panetoken_test.go`, so deleting the file the task named would have deleted the only protection against exactly that swap. The commit re-homes it as a **positive** assertion in `cmd` (must be `NewPaneTokenGenerator`, not merely must-not-be `NewGenerator`), which also catches a third generator. The guard is fail-closed: `found == false` — the mint moving out of `cmd`'s production sources, or becoming a composite-literal field rather than an assignment — is a `t.Fatalf`, so it cannot pass having stopped looking.
  - Behavioural equivalence of the swap: `session.NewPaneToken` was `nanoid.NewPaneTokenGenerator()()`; the seam now holds the returned `Generator` and calls it. `generatorOfWidth` (`internal/nanoid/nanoid.go:39-50`) closes over nothing but `n`, so constructing once per `hookSeams()` versus once per mint is indistinguishable.
  - Type compatibility of every injection site checked: the four test injections (`cmd/hooks_seams_test.go:47`, `cmd/hooks_rm_exit_test.go:96`, `cmd/hooks_pane_token_test.go:143,194`, `cmd/hooks_write_lock_test.go:126`) are all untyped `func() (string, error)` literals, assignable to the named `nanoid.Generator` without change.
  - Repo-wide grep for `NewPaneToken` (all `.go` files, tag-blind so both lanes are covered) returns only `NewPaneTokenGenerator` occurrences; `panetoken` survives only in `.workflows/` records and the superseded spec body. `session.IDGenerator` remains, used solely by the session-creation pipeline it belongs to (`internal/session/naming.go:14`, `create.go:56,61`, `quickstart.go:19,24`, `prepare.go:23`).

TESTS:
- Status: Adequate
- Coverage: The task prescribes no new behavioural tests — the mint is unchanged in width, charset and behaviour — and names four existing subjects that must stay green with their subjects intact. All four survive in substance (the wording in the tree differs from the task body, which was written from an earlier snapshot):
  - "it mints and stamps a token for a pane carrying none" → `cmd/hooks_pane_token_test.go:16` ("it stamps and writes under a freshly minted token"), which asserts exactly one `set-option`, the target, the option name `state.PortalPaneIDOption`, and the `hooks.json` entry landing under the stamped value.
  - "it reuses the token a pane already carries and issues no set-option" → `cmd/hooks_pane_token_test.go:50`, asserting a zero `set-option` count and the entry under the pre-existing token.
  - "it exits non-zero and writes nothing when the mint fails" → `cmd/hooks_pane_token_test.go:135` ("it never writes an empty key"): mint returns an error, no `set-option` is issued, and `hooks.json` is never created. AC5 is directly covered.
  - "it recognises the minted token as token-shaped" → two places, at different layers: `cmd/hooks_pane_token_test.go:45` on the stamped value, and `cmd/hooks_seams_test.go:34` on the **production** (uninjected) minter — which is where the width contract is actually observed at runtime.
- Notes:
  - `cmd/hooks_seams_test.go:34` is a strengthening the commit made in passing: the old assertion was `token != ""`, which a general-purpose mint would satisfy. The new one (`nanoid.IsTokenShaped`) fails if the production default ever mints at a width the predicate does not read.
  - The coverage deleted with `internal/session/panetoken_test.go` is not lost: both runtime properties it asserted are held by `internal/nanoid/nanoid_test.go:99` ("it recognises every token the pane-token mint produces", 200 iterations) and `:123` ("it mints a distinct token per call", 200 iterations), and its structural guard is superseded by the stronger positive guard in `cmd`.
  - Not over-tested. The AST guard and the `IsTokenShaped` runtime assertions overlap in subject but not in failure mode: while both widths are 6, swapping the constructor is invisible to every runtime assertion, and only the guard sees it. Conversely the guard cannot see a width change inside the leaf, which the nanoid suite does.
  - No test execution was performed (reading only, per the reviewer's remit). "Both lanes pass" is judged by reading: the change is type-compatible at every site, leaves no dangling reference, and the guard is untagged (unit lane) with no tmux, daemon or binary-build dependency, so it violates no lane rule.

CODE QUALITY:
- Project conventions: Followed. The guard is untagged and stdlib+`sourceguardtest` only, so it runs in the fast lane alongside the repo's ~20 other source guards; it routes through `sourceguardtest.ParsePackageSources` (the single parse step) rather than re-authoring the enumerate-parse loop. `internal/nanoid` stays the single home of the id vocabulary, and `internal/session` no longer holds anything about panes — which is what the architecture table now says.
- SOLID principles: Good. The seam is typed at the leaf that owns the concept rather than at a package that merely aliased it; `cmd/hooks.go` now depends on `internal/nanoid` (what it uses) instead of `internal/session` (a package scoped to session creation).
- Complexity: Low. The production change is two lines plus an import swap; the guard is a flat AST walk with one unwrapping helper.
- Modern idioms: Yes.
- Readability: Good. `generatorName` (`cmd/hooks_pane_token_width_guard_test.go:56`) is a single-purpose unwrapper with an accurate doc comment, and there is no name collision with any other helper in `package cmd`.
- Comment accuracy: Every claim in the changed code holds. The guard's header comment ("The two widths are equal today, so nothing observable separates the generators") is true — `width = 6` at `internal/nanoid/nanoid.go:22`, `paneTokenWidth = 6` at `:58` — and it is the reason the guard exists rather than a restatement of the code.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
