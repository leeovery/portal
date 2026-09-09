TASK: resume-hooks-silently-lost-8-21 — Four Callee-Name Unwrappers Survive Beside The Shared CalleeName (tick-67188d, phase 8, severity: duplication)

ACCEPTANCE CRITERIA:
- [ ] `sourceguardtest.CalleeName` is the only Ident/Selector callee unwrapper in the tree.
- [ ] The tmux guard's vocabulary check is a predicate over the shared unwrap.
- [ ] All four guards report the same findings as before on the real tree and on their staged probes.

STATUS: issues_found

SPEC CONTEXT: This is a phase-8 implementation-analysis consolidation task, so its authority is its own body rather than the resume-hooks specification (per the shared verifier context: phases 6–9 are consolidation/quality tasks the implementation phase generated). The surrounding project convention that bears on it is CLAUDE.md's `sourceguardtest` row: that package is "the Go-source scanning primitives the repo's ~20 unit-lane source guards share", with `CalleeName` named as `ForEachFuncCall`'s companion unwrapper — "the identifier of a bare call or the selected name of a method or qualified call". The task's outcome sentence is "One callee-name unwrapper in the tree; a guard states its own vocabulary and nothing else."

IMPLEMENTATION:
- Status: Implemented for the four named sites; the task's exclusivity criterion is not achieved (a fifth duplicate was missed by the enumeration).
- Location: commit 7159680b, four files:
  - cmd/state_daemon_lock_pid_ordering_test.go:207 — `ifStmtContainsCallTo` now `if sourceguardtest.CalleeName(call) == name`, local two-arm switch deleted, import added at line 12.
  - internal/theme/slug_collapse_guard_test.go:47 — `switch sourceguardtest.CalleeName(call)`, local `calledName` deleted (no references remain: grep for `calledName` across the tree returns none).
  - internal/tmux/target_composition_guard_test.go:899-905 — `isExactTargetCall` is now `return exactTargetHelpers[sourceguardtest.CalleeName(call)]`, i.e. a set-membership predicate over the shared unwrap.
  - internal/tui/restore_source_guard_test.go:179-181 — `callsRestoreHelper` is now `return sourceguardtest.CalleeName(call) == restoreHelperName`.
- Notes:
  - Behavioural equivalence holds at all four sites. The one non-identical detail is the cmd guard's dropped `fun.Sel != nil` check: `go/parser` always populates `SelectorExpr.Sel` (a `*ast.Ident`, `_` under error recovery) and these guards are routed through `sourceguardtest.ParsePackageSources`, which is fatal on an unparseable file, so no input reaches the dereference with a nil `Sel`. Not a finding.
  - The non-Ident/non-Selector callee (a called function literal, a generic instantiation) reported `false`/no-match before and now reports `CalleeName == ""`. Every name compared against is a non-empty constant (`writePIDFileIdent = "WritePIDFile"`, `restoreHelperName = "RestoreTerminalBackground"`, the theme guard's `"ResolveSetting"`/`"Slug"` cases), and `exactTargetHelpers` (internal/tmux/target_composition_guard_test.go:100-106) holds no empty key, so the empty string matches nothing at any of the four sites.
  - Do-item 3 respected: the receiver-qualified matchers were left alone. I re-enumerated every `call.Fun` type inspection in the tree. The remaining two-arm switches at cmd/open_theme_nomination_test.go:157, cmd/prefs_translation_test.go:247, cmd/doctor_spawn_seams_guard_test.go:24, cmd/theme_fatal_test.go:99 and internal/theme/resolve_test.go:574 are receiver-qualified matchers or qualified-path renderers (`pkg.Name + "." + Sel.Name`), which `CalleeName` cannot express — correctly excluded. `internal/portaltest/teardown_guard_coverage_test.go:283-289`'s `localCallName` is an Ident-only half deliberately paired with `selectorName` for the same qualified/local discrimination — also correctly excluded.
  - The one genuine miss is internal/capture/swap_harness_test.go:226-235 — see FINDINGS.

TESTS:
- Status: Adequate.
- Coverage: `CalleeName` carries its own three-case unit suite (internal/sourceguardtest/calleename_test.go:10-26 — bare identifier, selector, and the neither-shape empty-string case), which is what the four collapsed call sites now lean on. Each guard keeps its own fixtures, predicates and failure verdicts unchanged; the four suites are the regression evidence for the substitution.
- Notes: No new tests were warranted and none were added — correct for a behaviour-preserving substitution onto an already-tested helper. No over-testing: nothing duplicates the `calleename_test.go` cases at the four call sites.

CODE QUALITY:
- Project conventions: Followed. All four files stay in the unit lane (no build tags added), `sourceguardtest` remains stdlib-plus-`harnesstest`/`portalbintest` (no new edges — the four consumers depend on it, not the reverse), and the guards keep their scanning primitives routed through the shared package as CLAUDE.md's `sourceguardtest` row prescribes.
- SOLID principles: Good — one declaration of the unwrap rule, each guard left stating only its own vocabulary.
- Complexity: Low. Net −29 lines; the tmux guard's `isExactTargetCall` drops from a four-branch switch to a single map lookup.
- Modern idioms: Yes.
- Readability: Good. `internal/theme/slug_collapse_guard_test.go:47`'s `switch sourceguardtest.CalleeName(call)` reads better than the old `switch calledName(call.Fun)` because the switch subject is now the call rather than its callee expression.
- Comment accuracy: The retained comment at cmd/state_daemon_lock_pid_ordering_test.go:198-199 ("searches init, cond and body for a call to a bare ident with the given name or a selector ending in it") still describes the new predicate exactly. internal/tmux/target_composition_guard_test.go:759-761's comment about reading the callee through `CalleeName` is true of the code beneath it.
- Issues: None in the delivered diff.

BLOCKING ISSUES:
- Acceptance criterion 1 ("`sourceguardtest.CalleeName` is the only Ident/Selector callee unwrapper in the tree") is unmet: a fifth duplicate of the same unwrap survives at internal/capture/swap_harness_test.go:226-235. The task's stated outcome — one callee-name unwrapper in the tree — is not achieved, and the task body's claim that "these four are the complete candidate set" is false. See the finding below for the remedy.

FINDINGS:
- [in-scope] [contained] internal/capture/swap_harness_test.go:226 — `countCalls` folds the Ident/Selector callee unwrap into its name match with the same two-arm switch the task collapsed elsewhere (`case *ast.SelectorExpr: if fn.Sel.Name == name` / `case *ast.Ident: if fn.Name == name`), so it is a fifth copy of the rule, not a receiver-qualified matcher — nothing in it qualifies the receiver. It predates this task (present at commit 7159680b via the theming-system work), so the task's enumeration missed it rather than the task creating it. Fix: replace lines 226-235 with `if sourceguardtest.CalleeName(call) == name { count++ }`; the file already imports `github.com/leeovery/portal/internal/sourceguardtest` at line 12 (it is used elsewhere in the file), so no import edit is needed and none becomes unused. The substitution is exactly equivalent for this call site: `name` is always a non-empty literal at the three call sites (`countCalls(body, "ModelAt")`, `countCalls(body, "Build")`, `countCalls(body, "ApplyTheme")` at lines 194-200), so the empty string `CalleeName` returns for any other callee shape matches nothing, and the guard's own assertions observe the counts. — FAILS: acceptance criterion 1 is false as written — the unwrap rule is still stated in two places, and a done task now records the opposite, so the next contributor changing what "the name a call names" means (adding a generic-instantiation `*ast.IndexExpr` arm, say) will change `CalleeName` and leave this guard silently reading the old rule, having been told by the record that there was nothing else to change.
