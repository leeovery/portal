TASK: cli-verb-surface-redesign-7-3 — Collapse the dual source of truth for open's value-taking flags in the ordered-target argv scan

ACCEPTANCE CRITERIA (from plan row 7-3):
- Two acceptable paths — (a) thread *cobra.Command/*pflag.FlagSet to derive arity, OR (b) add a VisitAll drift-guard test.
- A future value-taking flag's value must not be misclassified as a bare positional target (covered by test).
- Existing left-to-right multi-target ordering unchanged.

STATUS: Complete

SPEC CONTEXT:
Spec § "Argv parsing contract (target ordering)" (specification.md:184-194): Cobra stays the source of truth for flag validation/binding; a raw os.Args scan recovers only target ORDER, and "it classifies each token by the same flag set cobra knows, so the two never disagree." The task's whole point is to make that "same flag set" guarantee structurally enforced rather than a hand-maintained coincidence. orderedOpenTargets classifies each argv token via the static openTargetPins map; a value-taking flag registered on openCmd but missing from the map would be treated as arity-0, misrouting its value into a bare positional target (and potentially flipping a single-target open into a multi-target burst).

IMPLEMENTATION:
- Status: Implemented (path (b) — drift-guard test; the map is retained but guarded)
- Location:
  - cmd/open_targets.go:14-35 — openTargetPins unchanged in content; a doc comment added (lines 21-26) declaring the map a hand-maintained mirror of openCmd's live flag set and naming the guard test.
  - cmd/open_targets_guard_test.go:26-40 — valueTakingFlagMissingPins predicate (bool / NoOptDefVal → arity-0 → nil; else require --long and, when present, -short).
  - cmd/open_targets_guard_test.go:82-88 — TestOpenTargetPinsCoverValueTakingFlags: walks openCmd.Flags().VisitAll and fails on any value-taking flag absent from openTargetPins.
  - orderedOpenTargets (open_targets.go:48-92) production logic is UNCHANGED (comment-only diff), so ordering behaviour is preserved by construction.
- Notes: The chosen path (b) is one of the two the plan explicitly sanctions. The predicate's arity model exactly mirrors orderedOpenTargets' "skip WITHOUT consuming a following value" fallback: pflag treats bool and NoOptDefVal (optional-value / count) flags as arity-0, and both are correctly excluded from the required set. Verified the live flag set is the 7 value-taking flags exec/filter/session/path/alias/zoxide/ack (open.go:997-1003) — all StringP/String, all present in openTargetPins in both long and short forms (ack has no shorthand, so only --ack is required and present). Confirmed no persistent flags exist anywhere in cmd (root.go has none), so VisitAll sees exactly openCmd's 7 local flags; the guard would additionally auto-cover any future root persistent value-taking flag (a bonus, not a gap).

TESTS:
- Status: Adequate
- Coverage:
  - TestValueTakingFlagMissingPins_DetectsDrift — crafts a value-taking "zzz"/"Z" flag absent from the map; asserts BOTH forms reported missing (proves the guard catches the exact misrouting-drift the task targets). Uses a crafted FlagSet because a pflag.FlagSet cannot cleanly un-register a flag from the real openCmd — a sound, documented choice.
  - TestValueTakingFlagMissingPins_SkipsAndCovers — bool skipped, NoOptDefVal (optional-value) skipped, and a fully-covered flag (session) produces no false positive. Covers the three arity/false-positive edges.
  - TestOpenTargetPinsCoverValueTakingFlags — the live guard against openCmd's real flag set; passes for the current 7 flags, fails the moment a value-taking flag is added without a matching pin entry.
  - Existing cmd/open_targets_test.go TestOrderedOpenTargets (unchanged) covers left-to-right ordering, equals/space forms, exec/filter/ack exclusion, `--` termination, repeats/no-dedup, unknown-flag-consumes-nothing, and trailing-pin panic-safety — the "ordering unchanged" criterion.
- Notes: The map-presence ⟺ value-consumption coupling the guard relies on is itself behaviourally pinned in open_targets_test.go (known flag "-e claude ~/new" → only ~/new; unknown "--dry-run blog" → dry-run consumes nothing), so the two files together close the loop end-to-end. Not over-tested: three focused predicate/guard tests, each a distinct facet, plus the untouched ordering suite. There is no direct test that orderedOpenTargets actually misroutes an unmapped value-taking flag's value — but that is inherent to path (b): the guard forbids that state from ever existing, so it cannot be simultaneously guarded and demonstrated against the real map. Not a gap.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel (package cmd; noted in the file comment per CLAUDE.md). No package-level state, no cobra Execute, no tmux — a pure unit + a read-only VisitAll walk, so it stays in the fast unit lane (no integration tag needed). Mirrors the existing keymap_dispatch_guard_test.go drift-guard idiom the plan pointed at.
- SOLID principles: Good. Extracting valueTakingFlagMissingPins as a shared predicate (single responsibility) makes the guard's decision logic unit-testable without mutating openCmd — the right seam.
- Complexity: Low. Predicate is two guard-clauses + two presence checks; the live guard is a one-line VisitAll closure.
- Modern idioms: Yes. slices.Equal for the expected-slice assertion; pflag introspection via Value.Type()/NoOptDefVal/Shorthand is the idiomatic arity check (pflag itself compares Value.Type() == "bool").
- Readability: Good. Both the map comment and the test-file header explain the drift risk and the mechanism precisely; the live-guard failure message is actionable (names the flag, the missing key, and the fix).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The guard verifies arity-presence, not that a new entry's mapped domain string is a valid resolver domain — but a wrong domain cannot misroute a value into a positional and would surface in the pin-dispatch tests, so it is out of scope for this task and not an actionable finding here.)
