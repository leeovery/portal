TASK: resume-hooks-silently-lost-6-14 — Single-Source The --pane-key No-Tmux-Call Poison Fixture (tick-a7e6ad)

ACCEPTANCE CRITERIA:
- One declaration of the poison fixture and one of its assertion.
- All seven `--pane-key` cases guard both seams.
- The poison error message is identical at every site.

STATUS: complete

SPEC CONTEXT:
Spec §4.3 ("`--pane-key` stays a literal pass-through") states the flag "performs **no validation of any kind** and touches tmux not at all"; §2 restates it in the bootstrap-exemption clause ("the `--pane-key` path (§4.3) touches tmux not at all"), and §4.2 relies on it ("This is what lets the `--pane-key` path carry the same rule while reading nothing of its own"). The verification table names it twice — "`hook rm --pane-key <a seeded key>` still removes it and exits 0 with no tmux read at all" and the byte-identical row. So "no tmux call on this path" is a specified behaviour, and the task's premise — that a site poisoning only one of the two pane seams does not prove it — is sound.

The production body backs the premise: `cmd/hooks.go:270-273` takes `hookKey = paneKey` verbatim when the flag is set and only otherwise calls `resolveCurrentPaneKey` (`cmd/hooks.go:73`, the sole `KeyResolver.ResolveHookKey` call on this command). `SetPaneOption` is reached only from `stampPaneToken` (`cmd/hooks.go:116`), which `hook rm` never calls. Both seams are therefore genuinely unreachable on this path, and poisoning them is a real guard rather than a tautology.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Fixture + assertion (single declaration): `cmd/hookkey_vocabulary_test.go:175-195` — the two package-level poison errors (`:176-177`), `paneKeyPathSeams()` (`:183-185`) returning the armed `*mockKeyResolver` / `*recordingPaneStamper` pair, and `assertNoPaneTmuxCalls` (`:187-195`) asserting both `resolver.calls == 0` and `len(stamper.calls) == 0`. Placed beside the other hook-key fakes as the Do list directed.
  - Converted sites, current HEAD: `cmd/hooks_rm_exit_test.go:68` + `:104` (the `rmPaneSeams` / `runRmCase` driver, which builds the pair for every `paneKeyPath: true` row and guards it — covering the table rows at `:337-343`, `:371-377`, `:412-418`), `cmd/hooks_rm_exit_test.go:233`, `cmd/hooks_rm_exit_test.go:281`+`:294`, `cmd/hooks_test.go:578`+`:597`, `cmd/hooks_pane_token_test.go:164`+`:175`, and `cmd/hooks_write_lock_test.go:208-223` (driven through `runRmCase`, so the guard rides along).
- Notes:
  - Scope was exceeded in the right direction. The task named six sites across three files (while saying "seven"); commit 47461f90 converted eight, adding the two `cmd/hooks_test.go` sites that carried a *different* message ("resolver must not be called when --pane-key is set") and poisoned only the resolver. Folding those in is what actually delivers criterion 3.
  - Three sites gained the stamper guard they previously lacked (`cmd/hooks_rm_exit_test.go` old `:144` and `:199`, `cmd/hooks_test.go` old `:733`/`:789`), and the Do list's contingency — "if one fails, that is a real gap" — did not trigger: the production body reaches neither seam, so all conversions hold.
  - `cmd/hooks_write_lock_test.go:236-237`'s `errors.New`/`fmt.Errorf` drift named in the task is gone; both messages now exist exactly once, as `errors.New` package vars.
  - Current HEAD differs from the task's own commit because phases 7-8 restructured these suites (`rmCase`/`runRmCase`/`rmPaneSeams` at `cmd/hooks_rm_exit_test.go:18-108`). That restructure *strengthened* this task's outcome: the poisoned pair is now built per row (`cmd/hooks_rm_exit_test.go:51-52,68`) rather than once per table, a contradictory row is refused (`:64-66`), and the guard fires from one place for every `--pane-key` row. One site (`cmd/hooks_rm_exit_test.go:227-244`) had its explicit `assertNoPaneTmuxCalls` call removed by task 7-19 in favour of the injected pair alone — its route is still guarded in full by the table row at `:337-343`, and that change belongs to 7-19's change-set, not this one.

TESTS:
- Status: Adequate
- Coverage: The task's deliverable *is* test code, so adequacy is judged as whether the guard proves the specified behaviour. It does: every `--pane-key` invocation in `cmd` (enumerated — `cmd/hooks_test.go:584`, `cmd/hooks_rm_exit_test.go:236,286,341,376,417`, `cmd/hooks_pane_token_test.go:170`, `cmd/hooks_write_lock_test.go:216`; eight in total) is driven against the poisoned pair, and all but `cmd/hooks_rm_exit_test.go:236` also run the paired count assertion. The routes covered span the whole `--pane-key` surface the spec names: successful removal, a key naming no entry, an unjudgeable old-format key, a held-lock timeout, and the mints-nothing / no-dirty-flag invariants.
- Notes:
  - Would the guard fail if the behaviour broke? Yes on both seams. A resolver call increments `mockKeyResolver.calls` (`cmd/hookkey_vocabulary_test.go:127`) and returns the armed error, which `resolveCurrentPaneKey` propagates; a stamp appends to `recordingPaneStamper.calls` (`:153`). Either trips `assertNoPaneTmuxCalls`, and at the one site without it the substituted error text fails the message assertion.
  - No over-testing introduced. The one overlap — `cmd/hooks_rm_exit_test.go:387` re-checking `len(got.stamper.calls) != 0` after `runRmCase` already guarded the poisoned row — is the subtest's own subject applied uniformly to all three rows ("hook rm neither stamps nor unstamps"), not a second copy of the poison assertion.
  - The Do list's "temporary edit must fail all seven" check is an implementation-time verification with nothing committed, so it is unverifiable here by design; the structural argument above stands in its place.

CODE QUALITY:
- Project conventions: Followed. Test-only helper in a `_test.go` file in `package cmd`; `t.Helper()` is set (`cmd/hookkey_vocabulary_test.go:188`); `t.Errorf` rather than `t.Fatalf` so both seams report; unit lane, no build tag, no `t.Parallel()`; seams staged through `withHooksDeps` at every converted site, per the `*Deps` injection rule. `*testing.T` (rather than `harnesstest.TestingT`) is correct here — the shared subset is reserved for helpers whose own failure path is under test, which this one's is not.
- SOLID principles: Good. The fixture and its assertion are split into two functions with one job each, exactly as the task specified; the "poisoning one seam proves nothing about the other" rationale is carried in the comment rather than left implicit.
- Complexity: Low. Two straight-line functions, no branching beyond the two independent checks.
- Modern idioms: Yes. `errors.New` for constant-text sentinels (the `fmt.Errorf`-with-no-verbs the old sites used is what `err113`/vet-adjacent linting discourages); named multi-return for the pair.
- Readability: Good. The stamper failure message interpolates `%+v` of the recorded calls (`:193`), so a violation names the target and option it wrote rather than only a count.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
