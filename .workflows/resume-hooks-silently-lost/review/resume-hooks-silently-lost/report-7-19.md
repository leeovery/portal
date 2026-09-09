TASK: resume-hooks-silently-lost-7-19 — "The hook rm Exit Suite Restates Four Subtests And Carries Two Latent Traps" (tick-e1b5be)

ACCEPTANCE CRITERIA:
- No route is staged twice in the file: the four standalone subtests no longer re-stage what the table stages.
- Every failing route still ends with a byte-identity assertion, from the table.
- Both tables drive their rows through one `runRmCase`, with the row struct and preamble declared once.
- The tmux-call counters are built per row, and a second `--pane-key` row cannot share another row's pair.
- `:306`'s name matches the axis its rows vary.
- `:30` compares the error text exactly.
- Coverage is unchanged: every route, message and exit code asserted before is still asserted.

STATUS: issues_found (0 blocking; 1 non-blocking finding)

SPEC CONTEXT:
The suite under consolidation pins §4.2/§4.3 of the specification: `hook rm` exits 0 **iff** it removed an
entry, so removing nothing is non-zero every way it happens — tmux's own words for a gone pane, Portal's own
words for a live pane carrying no token (decided before the store is consulted), and `no resume hook
registered for <key>` for a resolved token or a `--pane-key` naming no entry. The `--pane-key` pass-through
"performs no validation of any kind and touches tmux not at all". The spec's unit-lane acceptance rows
(specification.md:496, :506) require, for these routes, a non-zero exit, `hooks.json` left byte-identical,
and — on the `--pane-key` path — no tmux read at all. Removal mints nothing, stamps nothing and touches no
dirty flag.

IMPLEMENTATION:
- Status: Implemented (all seven acceptance criteria met)
- Location: cmd/hooks_rm_exit_test.go (whole file); driver at :78-108, row struct at :18-34, seam chooser at
  :60-75, its own guard test at :110-156. Delivered by commit 6cb37396; later commits (c7f7bd50, a01ef932,
  a5732ee3) extended the same driver.
- Notes:
  - Criterion 1: the four standalone subtests (:159, :194, :208, :227) now stage an empty/absent hooks.json
    (`hooksFileInTempDir(t, nil)`) and keep only their message-text assertions; every seed-plus-byte-identity
    pair they used to carry is gone.
  - Criterion 2: all five failing routes are rows of the byte-identity table (:302-344), each ending in
    `assertHooksFileUnchanged` at :349. The empty-key-entry seed that the "consults the store for nothing"
    subtest used to carry inline survives as the third row's seed (:323-326) with its reason in the comment.
  - Criterion 3: three tables (:297, :354, :395) plus two rows in cmd/hooks_write_lock_test.go:194 and :211
    all drive through the single `runRmCase`; the row struct and the eight-line preamble are declared once.
  - Criterion 4: `rmPaneSeams` (:60) constructs `paneKeyPathSeams()` per row inside the driver, and
    `runRmCase` guards the pair it built (:103-105). Nothing is hoisted per parent subtest, so a second
    `--pane-key` row cannot count against another row's calls. The chooser additionally refuses a row naming
    both a pane key and a resolver (:64-66) and returns a nil (not typed-nil) seam for a row naming neither,
    both pinned by TestRmCaseRows.
  - Criterion 5: "it touches no dirty flag on either path" (:395) gained the missing `--pane-key removal` row
    (:412-418), so the name now matches the axis its rows vary.
  - Criterion 6: :171 is `err.Error() != stderr`, an exact comparison, matching the newer sibling at :189.
  - Criterion 7: coverage is intact — each claim moved rather than vanished. The gone-pane byte-identity,
    the empty-key-entry non-match, the resolved-token byte-identity and the `--pane-key` byte-identity are
    all owned by table rows; the message texts stay with the standalone subtests. Two assertions were added
    rather than lost (`out == ""` at :222, and the `%42`-is-not-the-key check at :266).

TESTS:
- Status: Adequate (the file is itself the test; its own driver is unit-tested)
- Coverage: All five failing routes, both succeeding routes, the mint/stamp claim on three routes, the
  dirty-flag claim on three routes, and the error-classification claim. `TestRmCaseRows` (:110) covers the
  driver's own two decision branches — the contradictory-row refusal and the nil-seam fallback — through
  `harnesstest.Recorder`, so the helper's failure paths are observed rather than assumed.
- Notes:
  - Every table row's expected exit is enforced by the driver's `tt.wantErr != (err != nil)` check (:100),
    which is stricter than the pre-task `if err == nil` in the byte-identity table.
  - No over-testing: the routes that appear in more than one place each assert a distinct axis (message,
    byte-identity, mint/stamp, dirty flag, error classification), which is exactly the split the task asked
    for.
  - Lane: unit-lane, no tmux server, no binary build, no daemon — correct per CLAUDE.md. `hooksFileInTempDir`
    points `PORTAL_HOOKS_FILE` at a temp file and every row injects `HooksDeps`, so the cmd `TestMain` tmux
    poison is never reached. No `t.Parallel()`.

CODE QUALITY:
- Project conventions: Followed. Seams staged through `withHooksDeps` (never assigned directly), helpers
  reached from `internal/hookstest`/`internal/harnesstest` rather than hand-rolled, named seed constants used
  throughout instead of literal keys, `t.Helper()` on both helpers.
- SOLID principles: Good. `rmPaneSeams` holds the one seam decision, `runRmCase` the one drive; splitting the
  seam choice out is what makes the refusal path testable at all.
- Complexity: Low. One four-arm switch, one linear driver.
- Modern idioms: Yes. Table-driven subtests, typed row struct, `errors.AsType`.
- Readability: Good. Every non-obvious choice carries its reason (the typed-nil trap at :53-57, the
  empty-key seed at :318-319, the `%42` pane id at :251-253).
- Issues: one comment overclaims — see FINDINGS.

BLOCKING ISSUES:
- None.

FINDINGS:
- [in-scope] [contained] cmd/hooks_rm_exit_test.go:231 — the comment "The poisoned pair doubles as the
  assertion here: a body that reached the pane would surface the seam's error in place of this message" holds
  for the resolver half only: `resolveCurrentPaneKey` returns the resolver's error, but `hook rm` has no code
  path that calls the stamper at all, so a stamp issued on the `--pane-key` path would not surface anywhere in
  this subtest. This is also the only one of the five `paneKeyPathSeams()` call sites that does not pair with
  `assertNoPaneTmuxCalls` (:68 pairs at :104, :281 at :294, cmd/hooks_pane_token_test.go:164 at :175,
  cmd/hooks_test.go:578 at :597) — the call the task's commit removed. Fix: either narrow the comment to name
  the resolver as the half the message assertion catches and point at the byte-identity table's
  "--pane-key naming no entry" row (:337-343, guarded at :104) as the owner of the both-seams claim, or
  restore the one-line `assertNoPaneTmuxCalls(t, resolver, stamper)` here. — FAILS: the rule stated at
  cmd/hookkey_vocabulary_test.go:180-182 ("poisoning one seam proves nothing about the other, so every such
  case guards both") is false at this call site, so a contributor who copies this subtest as the pattern for a
  new `--pane-key` case inherits a fixture that watches one seam while its comment claims it watches the pane.
