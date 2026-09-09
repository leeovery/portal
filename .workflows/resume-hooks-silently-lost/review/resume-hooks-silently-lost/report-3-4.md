TASK: resume-hooks-silently-lost-3-4 — "The @portal-id Machinery Goes" (tick-938b62)

ACCEPTANCE CRITERIA:
1. `grep -rn "PortalIDOption\|@portal-id\|PortalID" internal cmd --include="*.go"` returns nothing, test and non-test alike
2. `CreateFromDir` issues exactly one `SetSessionOption` call, for `@portal-dir`, and still returns a generated session name
3. `QuickStart`'s `ExecArgs` chain is `new-session -d … ; set-option … @portal-dir … ; attach-session …` with no `@portal-id` link, `@portal-dir` still preceding `attach-session`
4. `tmux.HookKey` does not exist; nothing in the repository calls it
5. `portal state migrate-rename` does not resolve, is absent from `stateChildCommands`, and the state-surface tests enumerate five hidden children
6. `migrateRenameSubstring` and its comment are unchanged, and `portal uninstall` still reaps a legacy `session-renamed` hook through `teardownFingerprints`
7. No replacement literal-binding guard is added; both `@portal-id` guards are gone and neither is re-pointed
8. CLAUDE.md holds zero `@portal-id` / `PortalID` / `portal_id`, still holds the other five `@portal-*` literals, and its `state` row describes `Pane.PortalPaneID`, the per-pane capture column and the restore re-stamp
9. CLAUDE.md's "Resume hooks" section makes no saved-window/pane-indices claim and describes the baked key as the pane's saved token
10. `go build ./...`, `go test ./...`, `go test -tags integration -p 1 ./...` all pass

STATUS: complete

SPEC CONTEXT:
Spec §7.1–§7.4 govern this task. §7.1 makes the deciding argument supersession rather than dead weight: a token-only hook key (§3.1) carries no session identity, so renames are irrelevant by construction and `@portal-id` — which existed solely so a rename could not orphan a hook — is subsumed, not merely orphaned. §7.2 is the removal table; the rows this task owns are `internal/session/create.go` (the const + the `CreateFromDir` stamp), `internal/session/quickstart.go` (the `set-option` link in the chained `ExecArgs`), `internal/tmux/tmux.go` (`HookKey`) and `cmd/state_migrate_rename.go` (whole file). §7.3 pins the one reference that stays: `migrateRenameSubstring`, because `portal uninstall` must keep reaping the inert `session-renamed` hooks older binaries installed — a job outliving the command. §7.4 requires replacement and removal to ship in one release. §9.4 requires both `@portal-id` literal-binding guards to be deleted rather than re-pointed, the hazard being answered by construction (one literal home) instead of assertion. §3.3 drives the CLAUDE.md rewrite. No corrigendum touches this task.

IMPLEMENTATION:
- Status: Implemented (commit 33eb7dc9, 12 files, +80/−780)
- Location:
  - `internal/session/create.go:11-13` — only `PortalDirOption` remains; the `PortalIDOption` const and its doc comment are gone.
  - `internal/session/create.go:83-86` — one `SetSessionOption` call, `PortalDirOption`; the swallowed-stamp comment was narrowed from "Both stamps" to "The stamp", and its identity-fallback clause dropped.
  - `internal/session/create.go:74` — `sc.gen` survives, consumed by `PrepareSession`; the `IDGenerator` seam is intact and `CreateFromDir` still returns the generated name (`:88`).
  - `internal/session/quickstart.go:59-69` — the chain is `new-session -d … ; set-option -t … @portal-dir … ; attach-session -t …`; the `idToken, idGenErr := qs.gen()` line and the `@portal-id` link are gone. `qs.gen` survives via `PrepareSession` (`:54`).
  - `internal/session/quickstart.go:38-41` — the load-bearing detached-create ordering comment is rewritten rather than deleted, its subject narrowed to `@portal-dir` alone.
  - `internal/tmux/tmux.go` — `HookKey` and its doc comment removed (previously between `PaneTargetExact` and the target-composition block); `internal/tmux/hookkey_test.go` deleted whole (it held `TestHookKey` and `TestHookKey_DistinctSuffixesUnderOneID`, both callers).
  - `cmd/state_migrate_rename.go` and `cmd/state_migrate_rename_test.go` deleted; `stateCmd` now has exactly five `AddCommand` registrations (`cmd/state_daemon.go:446`, `cmd/state_commit_now.go:169`, `cmd/state_hydrate.go:301`, `cmd/state_notify.go:35`, `cmd/state_signal_hydrate.go:84`).
  - `internal/tmux/hooks_register.go:42-45,62-68` — `migrateRenameSubstring` and its comment are byte-identical from `33eb7dc9^` to HEAD (`git diff 33eb7dc9^ HEAD -- internal/tmux/hooks_register.go` is empty). The teardown chain is live: `cmd/uninstall.go:28,36` → `tmux.UnregisterPortalHooks` → `internal/tmux/hooks_unregister.go:14` (`portalCommandSubstrings = teardownFingerprints()`).
  - `CLAUDE.md` — the `tmux`, `state` and `session` rows and the "Resume hooks" section rewritten by hand.
- Notes:
  - Criterion 1 verified: `grep -rn "PortalIDOption\|@portal-id\|PortalID" internal cmd --include="*.go"` returns no lines. The only surviving `portal_id` in Go source is `internal/state/schema_test.go:142`, a legacy JSON payload in `TestDecodeIndex_DecodesPreUpgradePayloadToEmptyPortalPaneID`; it is the tolerant-decode coverage §7.2 calls for and is outside all three grep patterns.
  - Criterion 4 verified: no `func HookKey` and no `tmux.HookKey` call site anywhere; `HookKeyFormat` / `ResolveHookKey` / `ListAllPaneHookKeys` are the surviving, distinct primitives.
  - Criterion 5 verified: `rootCmd.Find([]string{"state","migrate-rename"})` cannot resolve (nothing registers it); `stateChildCommands` (`cmd/state_test.go:217-223`) holds five entries and every `migrate-rename` occurrence at `cmd/state_test.go:87,112,145,278,294` is gone — the remaining `state_test.go` references (`:275-283`) are the retirement assertion itself.
  - Criterion 7 verified: `cmd/portal_id_binding_guard_test.go` and `internal/state/portal_id_literal_guard_test.go` are both absent, and the commit adds no file at all (12 changed, 0 created).
  - Criterion 8/9 verified: CLAUDE.md matches none of `@portal-id`, `PortalID`, `portal_id`, and retains `@portal-dir` (3), `@portal-pane-id` (4), `@portal-skeleton-` (6), `@portal-restoring` (5), `@portal-spawn-` (4). The `state` row now names `Pane.PortalPaneID` (`json:"portal_pane_id"`, additive/tolerant-decode/no `SchemaVersion` bump), the trailing `#{@portal-pane-id}` `captureFormat` column at unchanged arity lifted per pane, and restore's per-pane re-stamp. The "saved window/pane indices … base-index drift" sentence — the one the task warned the grep cannot find — is gone, replaced by "The baked key is the pane's own saved token, so a lookup stays addressable however tmux renumbers windows and panes on restore."
  - Deleting `cmd/state_migrate_rename.go` left no orphaned helper: its callees (`loadHookStore`, `hooksLogger`, the store's load/save) either retain other production callers or were themselves consolidated by later phases.
  - Criterion 10 not executed (test execution is out of this review's remit). Judged by reading: every symbol the deletions removed has zero remaining references across tagged and untagged sources alike, and both deleted test files' contents were self-contained.

TESTS:
- Status: Adequate
- Coverage: every named test in the task exists and would fail if the behaviour it names regressed.
  - `internal/session/create_test.go:605` "it stamps only @portal-dir at session creation" — asserts `len(setOptionCalls) == 1` and compares the single call against `setOptionCall{Session, PortalDirOption, dir}`. A re-introduced second stamp fails on the count; a wrong option name fails on the value compare.
  - `internal/session/create_test.go:645` "it still generates a session name from the injected generator" — pins the returned name to `filepath.Base(dir)+"-z9y8x7"`, so the seam's survival is observed rather than assumed.
  - `internal/session/create_test.go:629` "it omits the stamp when the directory is unavailable" — the retained degradation case (task text said "omits the stamp link"; same substance, one stamp now).
  - `internal/session/quickstart_test.go:90` "it emits no legacy session-id link in the QuickStart ExecArgs" — full-chain `reflect.DeepEqual` against the exact 17-element argv, which is the strongest available form of "no extra link".
  - `internal/session/quickstart_test.go:115` "it keeps @portal-dir before attach-session in the ExecArgs chain" — the retained ordering assertion.
  - `cmd/state_test.go:275-283` "it does not resolve portal state migrate-rename" — the `err == nil && found.Name() == "migrate-rename"` form is correct for cobra: a resolving command would make both conjuncts true and fail; a missing one resolves to `stateCmd`, whose `Name()` is `state`.
  - `cmd/state_test.go:235-244` "it registers exactly five hidden state children", plus `:246-256`, which cross-checks `stateCmd.Commands()` against the same list so a phantom registration cannot hide.
  - The legacy teardown fingerprint is covered by `internal/tmux/hooks_event_parity_test.go:46-56` (asserts `MigrateRenameSubstring` is in `PortalTeardownFingerprints()` and *not* in the install/converge union) and `internal/tmux/hooks_unregister_test.go:108-120,334-347` (a `session-renamed[1] … portal state migrate-rename` body is reaped). Under the task's own name ("it keeps the legacy session-renamed teardown fingerprint") no new test was written, which is right — the file it protects is unchanged and the coverage already existed.
  - Retired tests map 1:1 onto deleted behaviour: the five `@portal-id` stamp subtests in `create_test.go` and the four in `quickstart_test.go`. The `@portal-dir` counterparts of each (stamp value, stamp-failure tolerance, prepared-dir source, no-stamp-on-`NewSession`-failure) all survive at `create_test.go:505,533,556,584`.
- Notes: `quickstart_test.go` carries three full-chain `reflect.DeepEqual` assertions over the same argv (`:35`, `:90`, `:207`) and duplicates the set-option-before-attach ordering (`:80-84` and `:129-133`) and the `-A` check (`:85-87` and `:150-152`). The redundancy is real but predates this task — the two subtests it authored replaced two `@portal-id` subtests one-for-one at the same positions, and both were named in the plan. Not raised as a finding. Minor: `indexOfSubseq(result.ExecArgs, []string{session.PortalDirOption})` at `:129` passes a one-element "subsequence" where `indexOf` says the same thing.

CODE QUALITY:
- Project conventions: Followed. Lane placement untouched (no test moved lanes, no binary built). No new log component, attr or call-site emission. The `internal/session` package keeps its no-log-component posture, and the swallowed-stamp comment still states why. CLAUDE.md was updated in the same commit as the code it describes, per the repo's own binding-document rule.
- SOLID principles: Good. The `IDGenerator` seam is retained for the one responsibility it still has (name generation) rather than deleted along with its second, now-dead use — the narrower reading of the change.
- Complexity: Low. `CreateFromDir` loses a branch; `QuickStart.Run` loses a conditional append and the error-carrying `idGenErr` it existed to absorb, so the chain is now a single unconditional `append`.
- Modern idioms: Yes. No opportunities missed.
- Readability: Good. Both rewritten comments narrow their subject rather than being deleted, and both hold true against the code beneath them: `create.go:83-85` claims one best-effort swallowed stamp and a derived-directory fallback (which `internal/session/dirresolve.go` provides); `quickstart.go:38-41` claims the detached create exists to give `@portal-dir` a stamp point that `new-session -A` would skip, which the argv at `:59-69` bears out. Neither carries a task id, phase or spec reference.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
