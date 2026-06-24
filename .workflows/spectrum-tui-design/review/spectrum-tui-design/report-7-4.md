TASK: spectrum-tui-design-7-4 — Close residual enforcement and DRY gaps in the reskin (colour guard, command-pending footer, separator constants)

ACCEPTANCE CRITERIA:
- centralisedColourSites (or the guard's glob) covers every production token-referencing render file in internal/tui; the guard passes.
- The command-pending footer is sourced from a keymapEntry/filterFooterEntry slice with no inline enter→⏎ string rewrite; its rendered output is unchanged.
- editFooterSep is gone; the edit modal and main footer share the single footerEntrySeparator constant.
- The helpColumnGap / modalFooterGap pair is left as-is.
- go build succeeds and go test ./... passes.

STATUS: Complete

SPEC CONTEXT:
- §2.9 (MV token table, closed vocabulary) pins the rule: "Every renderer references a token — no literal hex at call sites (this is what makes §2.8 theming work)." The closed-vocabulary list says "every rendered colour is one of these tokens; no literal hex outside the token layer." The colour-literal guard is this rule made executable.
- §3.4 (condensed footer) defines the dot-rhythm footer separated by the " · " run, shared across footer variants.
- §11.4 (command-pending banner) defines the swapped Projects footer copy `enter run here · n run in cwd · esc cancel` + the retained `? help` anchor — the descriptor source the command-pending path must read from.

IMPLEMENTATION:
- Status: Implemented (all three gaps closed; the colour-guard claim is genuinely exhaustive)
- Location:
  - Colour guard: internal/tui/colour_literal_guard_test.go:25-43 (centralisedColourSites now a `filepath.Glob("*.go")` over the package dir, excluding _test.go; the theme/ subpackage is excluded by globbing the package dir only, not recursing). TestNoRawColourLiteralAtCentralisedSites:51-92 parses each globbed file and flags any lipgloss.Color(...) called with a raw string/int literal.
  - Command-pending footer: internal/tui/keymap.go:153-159 (commandPendingKeymap() — the §11.4 descriptor with the `enter`/HelpKey `⏎` declarative encoding); internal/tui/footer.go:99-109 (commandPendingFooterEntries maps the descriptor through helpKeyGlyph — the SAME glyph resolver the help path uses — into the filterFooterEntry shape). The former commandPendingHelpKeys() raw []key.Binding + inline `if glyph == "enter" { glyph = "⏎" }` rewrite is fully removed (grep: zero references to commandPendingHelpKeys anywhere).
  - Separator constant: editFooterSep is deleted (grep confirms no production reference; only a test-comment mention survives). edit_modal.go:247 and edit_modal.go:503 now reference the shared footerEntrySeparator (footer.go:45).
  - helpColumnGap / modalFooterGap left untouched (not consolidated, per the explicit "leave as-is").
- Notes:
  - Colour-guard exhaustiveness verified independently: the ONLY subdirectory under internal/tui is theme/ (the single sanctioned home for raw colour values), so a package-dir-only glob covers every render file outside theme/. All eight files the task named as the Phase 3-5 blind spot (destructive_confirm.go, kill_modal.go, delete_modal.go, pagepreview.go, loading_view.go, notice_band.go, sessions_flash.go, empty_states.go) are present in the glob set. A whole-tree grep for lipgloss.Color() with a raw literal outside theme/ returns nothing — no live violation, so the guard passes today and now traps future regressions in the newly-covered files.
  - The glob choice is the stronger of the two task-offered options (enumerate vs glob): it is self-maintaining, so no future renderer can be silently omitted. Drift risk eliminated rather than merely patched.
  - "internal/capture, as applicable" resolves to not-applicable: internal/capture constructs no lipgloss.Color at all and references no theme tokens, so it needs no guard coverage.
  - The command-pending footer continues to route through renderFilterFooter (the shared two-row machinery), so the `? help` anchor + two-row structure stay byte-consistent with the standard / filter footers — only the entries differ.

TESTS:
- Status: Adequate
- Coverage:
  - Colour guard: TestNoRawColourLiteralAtCentralisedSites runs a subtest per globbed file (t.Run(name, ...)) and AST-parses each, asserting no lipgloss.Color(literal). The guard passing on the current tree IS the proof that the newly-covered files (pagepreview.go et al.) carry no live raw-hex. A len==0 guard (line 39-41) fails loudly if the glob ever matches nothing (catches a mis-pathed run).
  - Command-pending footer: TestCommandPendingFooter_ByteExact (command_pending_band_test.go:363-386) pins the FULL-ANSI rendered output across Dark / Light / NO_COLOR — a single byte drift in any cell (glyph, colour, separator, spacer) fails. This is the byte-identical proof the descriptor-vocabulary refactor preserved output. TestCommandPendingKeymap_Copy:339-356 pins the descriptor (enter/⏎/run here, n/run in cwd, esc/cancel) so the declarative HelpKey encoding is locked. TestCommandPendingFooter_SwappedCopy:254-273 asserts the §11.4 copy is present and the standard-footer copy (quit / new session / new in cwd) does not leak.
  - Separator constant: TestEditModalFooterRow_ByteExact (edit_modal_test.go:719-739) pins the full-ANSI edit-modal footer for both the navigate-name and editing-in-place states, proving the editFooterSep → footerEntrySeparator switch is byte-identical (both were the same " · " in text.detail). edit_modal_test.go:470-472 additionally references footerEntrySeparator directly in a gap-width assertion.
- Notes:
  - Test balance is good — no over-testing. The byte-exact goldens are the right granularity for a "render byte-identically" acceptance criterion; the descriptor-copy and swapped-copy tests cover the structural/behavioural layer without redundancy. Dispatch parity (TestCommandPending_DispatchParity) confirms the reskin did not touch key handling.
  - The colour guard does not have a separate test asserting the glob explicitly enumerates the previously-blind files — but a glob is self-evidently exhaustive over the package dir, so an enumeration assertion would only re-encode the hand-list the glob was introduced to retire. Correct call.

CODE QUALITY:
- Project conventions: Followed. Tests carry no t.Parallel() (the package mandate). The guard test lives in package tui_test (external) and uses go/ast — idiomatic. The descriptor-driven footer matches the established §12.1 keymap-descriptor-as-single-source-of-truth pattern.
- SOLID principles: Good. commandPendingFooterEntries reuses helpKeyGlyph (the one glyph resolver), so the footer and help paths cannot drift — a clean single-responsibility / DRY win. The separator consolidation removes a genuine same-role duplicate.
- Complexity: Low. The glob loop is trivial; commandPendingFooterEntries is a flat map over the descriptor.
- Modern idioms: Yes. filepath.Glob + go/ast inspection is the right tool for a structural lint-style guard.
- Readability: Good. Docstrings are thorough and explain WHY (the blind-spot rationale, the byte-identical contract, the deliberate non-consolidation of helpColumnGap/modalFooterGap).
- Issues: One stale docstring (non-blocking, see below).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/help_modal.go:190 — the helpKeyGlyph docstring states "The footer NEVER calls this — it always reads Key directly." This is now inaccurate: commandPendingFooterEntries (footer.go:104) calls helpKeyGlyph. Reword to scope the claim to the condensed footers (e.g. "The condensed sessions/projects footer never calls this — it reads Key directly; the command-pending footer reuses this resolver to share the `enter`→`⏎` encoding."). Documentation-only, single file, zero logic impact.
- [do-now] internal/tui/colour_literal_guard_test.go:18 — the function/file docstring references "the §2.8 'closed vocabulary, no literal hex at call sites' rule" while the migration spec pins the closed-vocabulary/no-literal-hex rule under §2.9 (the MV token table); §2.8 is the theme-readiness section. Align the section reference to §2.9 (or "§2.8/§2.9") for accuracy. Comment-only.
