TASK: resume-hooks-silently-lost-8-43 — `--pane-key`'s Help Text And README Narrow The Flag Below The Contract It Was Given (tick-b439ca)

ACCEPTANCE CRITERIA:
- The flag help describes a verbatim hook key, not a pane token.
- README says the same, and names the retention escape hatch.
- No validation is added — a non-token key still passes through unchanged.
- Exit-code behaviour is untouched: removing nothing is still non-zero.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 implementation-analysis task, so its authority is its own body rather than the
specification; the specification's §1.2 change C (the shape-aware stale sweep) is the design the
copy has to describe. The retention rule it produced — a persisted key the staleness rule cannot
judge (the legacy `<session>:<window>.<pane>` shape) is retained forever, because reaping it would
be a guess and the entry holds a user-authored `on-resume` command with no other copy — leaves
`portal hook rm --pane-key <key>` as the CLI's only removal route for such an entry. CLAUDE.md
states that ("the only sanctioned removals are the user's own `portal hook rm --pane-key <key>` and
a hand edit"); before this task the CLI's own help and the README described the flag as taking a
*pane token*, which is precisely the one thing a retained legacy key is not.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/hooks.go:310 — flag usage is now "Hook key of the entry to remove, taken verbatim — any key,
    including an old-format one (defaults to the current pane's token)". It describes a verbatim
    hook key, names the old-format case, and keeps the default behaviour accurate (an empty flag
    falls through to `resolveCurrentPaneKey`, cmd/hooks.go:272-283).
  - README.md:191 — "`hook rm` defaults to the current pane's token but accepts `--pane-key` to
    remove the entry under any hook key, taken verbatim (including panes that no longer exist, and
    the old-format `<session>:<window>.<pane>` keys the stale-entry sweep retains forever rather
    than guessing at — hand removal through this flag is the sanctioned route for those)". The
    escape hatch is named explicitly.
  - README.md:202 — the repeated claim in the example block now reads "remove the entry under any
    hook key, verbatim (works outside tmux)". The parenthetical is true: the `--pane-key` branch
    (cmd/hooks.go:270-271) never reaches `requireTmuxPane`.
  - cmd/hooks.go:264-283 — no validation was added. `paneKey` is assigned straight to `hookKey` with
    no shape check, no `nanoid.IsTokenShaped` call, and no non-empty guard beyond the branch test.
  - cmd/hooks.go:292-298 — exit-code behaviour untouched: the verdict comes from `store.Remove`'s
    `removed` return, and a false return produces `no resume hook registered for <key>`, i.e.
    non-zero.
- Notes: I considered and rejected one adjacent observation as a nitpick rather than a finding —
  `hooksRmCmd.Short` (cmd/hooks.go:261) still reads "Remove a resume hook for the current pane",
  which is narrower than the flag's contract. It is not a false statement about the default
  invocation, its remedy would be pure help text, and a user reading `portal hook rm --help` sees
  the corrected flag line on the same screen, so the task's stated outcome holds regardless. The
  Do list scoped the change to the flag usage and the README, and it did exactly that.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/hooks_test.go:623-633 ("--pane-key help describes a verbatim hook key") pins the usage
    string byte-for-byte against `hooksRmCmd.Flags().Lookup("pane-key").Usage`. The literal in the
    test matches cmd/hooks.go:310 exactly, em dash included, so a silent reversion of the wording
    fails here — this is the assertion that makes the criterion observable rather than assumed.
  - cmd/hooks_rm_exit_test.go:274-295 ("it exits 0 and removes on the --pane-key path") drives the
    flag with `hookstest.UnjudgeableSeedA`, which is the legacy `unjudgeable-session-0:0.0` shape
    (internal/hookstest/hooks.go:159-161, :187) — a non-token key — asserts the entry is gone and
    the exit is zero, and asserts via `assertNoPaneTmuxCalls` against the poisoned pane pair that
    no pane read or stamp happened. That is the task's "removes an entry keyed on a non-token hook
    key via --pane-key" pin, and it is also the pass-through-validates-nothing pin: had validation
    been added, this row would fail.
  - cmd/hooks_rm_exit_test.go:227-244 ("it exits non-zero when --pane-key names no entry") covers
    the named-key-matches-no-entry exit, asserting the exact message.
  - cmd/hooks_rm_exit_test.go:302-352 rows include "--pane-key naming no entry", proving hooks.json
    is left byte-identical on that failing route; :354-393 and :395-433 prove the flag mints,
    stamps and touches no dirty flag.
  - cmd/hooks_test.go:571-598 covers the flag's outside-tmux use (`TMUX_PANE` empty) that
    README.md:202 now claims.
- Notes: No new test was warranted beyond the usage-string pin — the behavioural criteria (3 and 4)
  are pre-existing behaviour the task explicitly required to be left alone, and the suite already
  pins both. Nothing here is redundant: the usage pin, the non-token removal and the no-entry exit
  each observe a different criterion.

CODE QUALITY:
- Project conventions: Followed. The change is a string literal and two prose sentences; no seam,
  log component, attr key or lane rule is touched, and no new import appears.
- SOLID principles: N/A — no structural change.
- Complexity: Low (unchanged).
- Modern idioms: N/A.
- Readability: Good. The help text is long for a flag line but reads as one sentence, and the
  README sentence carries the reason (why the sweep retains those keys) alongside the instruction,
  which is what makes the escape hatch discoverable rather than merely stated.
- Issues: None. I checked the whole README for other statements of the same claim — README.md:191
  and :202 are the only two places the flag is described, and both now match the contract.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
