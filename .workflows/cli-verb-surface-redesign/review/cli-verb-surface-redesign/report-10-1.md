TASK: cli-verb-surface-redesign-10-1 — Refresh the stale `open` command help text to describe the redesigned verb

ACCEPTANCE CRITERIA:
- Help-metadata only (Use/Short/Long) — do NOT touch RunE/Args/flag registration/dispatch/resolution
- `portal open --help` and bare `portal --help` both name session-name attach, the -s/-p/-z/-a pins, -f/--filter, -e/-- command scoping, and multi-target opening — no single-path `destination` implication
- Copy stays consistent with the accurate per-flag strings and the spec Command Surface row
- Spec dictates no golden string (match intent, not exact copy)
- Guard test asserts openCmd.Short/Long mentions the redesigned capabilities; any test asserting the literal Use/Short strings is updated
- Full unit+integration suite stays green

STATUS: Complete

SPEC CONTEXT: The spec (§ "portal open — Grammar & Target Resolution", § "Flags & Command Passthrough", and the Command Surface Summary row at :411) defines `open` as the single public session verb: no-args → TUI picker; a bare target resolves through the precedence chain (exact session name → path → alias → zoxide) with attach-vs-mint outcomes; four domain pins -s/-p/-z/-a skip the chain; -f/--filter opens the picker pre-filtered (mutually exclusive with targets/pins); -e/-- runs a mint-scoped command; 2+ targets open N surfaces (this terminal + N−1 host windows). Spec explicitly dictates matching intent, not a golden string.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open.go:151-176 (openCmd Use/Short/Long); guard test cmd/retired_surface_test.go:168-222
- Notes: Verified via `git show b3e8c1c0` the change is strictly help-metadata: Use `"open [targets…] [-- cmd args…]"`, a rewritten Short, and a new multi-paragraph Long. The `Args: cobra.ArbitraryArgs` line differs only in alignment whitespace (value unchanged); RunE, flag registration (init at :996-1004), dispatch, and resolution are untouched — the no-touch constraint holds.
  - Use no longer implies a single `[destination]`.
  - Short = "Open portals to one or more targets, or launch the interactive picker" — names multi-target + picker, drops the stale "start a session at a path".
  - Long names every required capability: no-args picker, precedence chain (exact session name → path → alias → zoxide), attach vs mint outcomes, all four pins (-s/-p/-a/-z), -f/--filter with its mutual-exclusivity note, -e/--exec and the `--` separator (mint-scoped), and multi-target opening ("this terminal becomes the first surface and the remaining N−1 open in host-terminal windows").
  - Consistency with the per-flag registration strings (open.go:997-1002) confirmed line-by-line (e.g. -s "attach … never mints", -p "dir must exist", -a "alias key or key glob", -f "skips resolution"); the Long abbreviates the -z not-installed nuance and the parenthetical domain labels, which the spec's "match intent, not exact copy" directive permits.
  - Note on bare `portal --help`: that path renders only each subcommand's Short (not Long), so the full pin/-e enumeration is inherent to `portal open --help`; the redesigned Short still removes the single-destination implication and names the multi-target + picker surface, satisfying the criterion's intent.

TESTS:
- Status: Adequate
- Coverage: cmd/retired_surface_test.go TestOpenHelpMetadata_DescribesRedesignedVerb (added by this task) asserts: Use excludes "destination"; Short excludes "at a path" and names a target/portal surface + "picker"; Long (lowercased) contains picker/session/attach/mint/-s/-p/-a/-z/-f/-e/--/precedence and describes multi-target via window|surface. Traced each keyword to the live copy — all present, so the suite is green.
- Notes: The guard pins intent (keyword presence) rather than a golden string, matching the spec directive and avoiding churn on accurate copy edits — correctly not over-tested. No other test asserts the literal open Use/Short; grep for the old strings ("interactive session picker" / "start a session" / "at a path" / "choose a destination" / "[destination]") found only this test's absence-assertions and the new copy, so nothing stale was left asserting the pre-redesign wording. The test would fail if the help re-staled (e.g. reverting Short to "at a path" or dropping any pin from Long).

CODE QUALITY:
- Project conventions: Followed — Cobra Use/Short/Long metadata pattern, em-dash/arrow glyphs consistent with the codebase house style; no dispatch or flag changes.
- SOLID principles: N/A (declarative metadata + a string-keyword guard test).
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good — the Long is well-structured (no-args → bare-target chain → pins block → -f → command → multi-target), accurate, and consistent with the per-flag strings.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The Long deliberately abbreviates the -z "explicit error if zoxide not installed" nuance and the per-flag domain-label parentheticals; this is sanctioned by the spec's "match intent, not exact copy" directive and the acceptance's per-flag-string-consistency note, so it is not a finding.)
