TASK: cli-verb-surface-redesign-3-2 — Raw `os.Args` scan → ordered domain-tagged target-set union (no dedup)

ACCEPTANCE CRITERIA:
- `-s api` / `-s=api` / `--session=api` value attributed to the pin, never a positional
- `-e <cmd>` value and everything after `--` excluded from the target list
- Pins repeat and interleave with positionals; left-to-right order preserved; no dedup
- Value pins never bundled (`-sf`-style)
- Cobra stays the flag-validation source of truth (scan is a pure classifier, not a validator)
- Single-target arity yields a one-element list

STATUS: Complete

SPEC CONTEXT:
Spec §"Argv parsing contract (target ordering)" (specification.md L184-194) governs this task. Cobra owns flag validation, value binding, `-f` exclusivity, and unknown-flag rejection; a raw `os.Args` scan is layered on top solely to recover true left-to-right target order and repeats that cobra's parsed buckets collapse (StringP overwrites repeated same-flag values; positionals/flags are split). Both value forms (space + equals) must attribute the value to the pin; `-e`/`--`/excluded-flag values are never targets; no dedup (spec §"No dedup — duplicates are honored as intent", L179-182). Plan Phase 3 AC bullet 1 (L83) restates the same contract. The scan "classifies each token by the same flag set cobra knows, so the two never disagree" (L194).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open_targets.go (Target struct L9-12; openTargetPins map L27-35; orderedOpenTargets L48-92). Consumed at cmd/open.go:230 via orderedOpenTargets(openOwnArgs()); openOwnArgs at cmd/open_burst.go:43-51.
- Notes:
  - Equals form handled by strings.Cut on FIRST '=' (L69) — `-s=api` / `--session=api` → name+inline value; space form leaves value empty and consumes the next token (L82-85). Value attributed to the pin, never emitted as a bare positional. Correct.
  - Excluded flags (`-e`/`--exec`, `-f`/`--filter`, `--ack`) map to the empty domain (L32-34): their value is still consumed off the argv (so it can't be misread as a positional) but no Target is emitted (L87-89). Matches spec exactly.
  - `--` terminates the scan via early break (L55-57) — post-`--` command-passthrough tokens are never targets. Correct.
  - No dedup and order preservation are inherent: a single left-to-right append loop with no collapsing (L50-90). Repeats and pin/positional interleave preserved.
  - Unknown flag-like tokens (booleans, `--help`, unmodelled flags) are skipped WITHOUT consuming a following token (L72-77) — correct arity handling that avoids swallowing a following positional.
  - Lone `-` treated as a bare positional (L62) — defensive, graceful.
  - Purity/authority: scan runs in RunE, after cobra's parse (documented L43-47); it never rejects a token. Cobra remains flag-validation authority. Correct per contract.
  - Single-target arity: falls out naturally — one token → one-element slice.
  - "Value pins never bundled" is honored as a design CONTRACT rather than enforced: a bundled `-sf` token is not in openTargetPins, so it is classified unknown and skipped (no value consumed). This is the only hairline where the raw scan could diverge from cobra (pflag would parse `-sf` as `-s` value `f`), but the spec explicitly excludes bundled value pins from the contract (L191), so it is out of scope by design — not a defect.

TESTS:
- Status: Adequate (well-balanced; comprehensive without bloat)
- Coverage: cmd/open_targets_test.go (TestOrderedOpenTargets, 22 table cases) + cmd/open_targets_guard_test.go (3 drift-guard tests).
  Behavioural cases cover: space-form pin + positionals with order; equals form short/long; all four pins (s/p/z/a) across space + equals forms; `-e`/`--exec` value excluded (space + equals + long); `--` tail excluded; `-f`/`--filter` excluded (space + equals); `--ack` excluded (space + equals); repeated pin no-dedup; mixed interleave order; single bare + single pin arity; empty args → nil; unknown flag skipped without consuming; two trailing-value-pin panic-safety edges.
  Guard tests: valueTakingFlagMissingPins detects a value-taking flag missing from the map (both forms), skips bool/optional-value/covered flags, and the live TestOpenTargetPinsCoverValueTakingFlags walks openCmd's real flag set so any future value-taking flag added without a pins entry fails loudly. This is a strong structural defense against the exact misroute-value-as-positional failure mode.
- Notes:
  - Tests would fail if the feature broke (e.g. dropping the equals-Cut, mis-consuming excluded values, or deduping) — they assert exact Target slices via slices.Equal.
  - The trailing value-pin-with-no-token cases are explicitly documented as panic-safety only (cobra rejects such input upstream since the scan runs post-parse) — the empty-Value emission is pinned as the current correct shape, not a real production path. Reasonable.
  - Minor gap (non-blocking): no case with an excluded flag's value sitting BETWEEN two positionals (e.g. `blog -e claude api`); logically identical to the covered `-e claude ~/new` head case, so low value.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel (both test files note the package-cmd rule). Small pure function, table-driven tests, seam-based consumption (openRawArgs/openOwnArgs overridable). Idiomatic Go (strings.Cut, HasPrefix).
- SOLID principles: Good. Single responsibility — orderedOpenTargets is a pure classifier; validation stays in cobra; the pins map is the single extension point. openTargetPins is a deliberately decoupled mirror with a live drift guard closing the coupling risk.
- Complexity: Low. One linear pass, clear branch structure, each branch commented with intent.
- Modern idioms: Yes. strings.Cut for the equals split; map-driven dispatch.
- Readability: Good. Doc comments are thorough and accurately describe the contract, the cobra relationship, the excluded-flag semantics, and the drift-guard obligation.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/open_targets_guard_test.go:82 — the live drift guard is forward-only (walks openCmd's flags to ensure each value-taking flag is in openTargetPins). Add a reverse assertion that every non-excluded openTargetPins key maps to a live openCmd flag, so a stale entry left after a future flag removal is caught. Low urgency (cobra rejects unknown flags before the scan runs, so a stale entry is harmless today).
- [do-now] cmd/open_targets.go:27 — the openTargetPins doc block covers the long/short mirror obligation but not the "no bundled `-sf` value pins" contract; add a one-line note that bundled value shorthands are out of contract (spec §Argv parsing) and are deliberately classified as unknown-and-skipped, so a future reader doesn't mistake the `-sf` divergence from cobra for a bug.
- [quickfix] cmd/open_targets_test.go:147 — add a case placing an excluded flag's value between two positionals (e.g. `["blog","-e","claude","api"]` → two bare targets) to pin that mid-list `-e` value consumption doesn't shift positional order; currently only head-position `-e` is exercised.
