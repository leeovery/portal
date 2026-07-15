AGENT: duplication
FINDINGS: none
SUMMARY: No significant duplication detected across the implementation files. This
is a quick-fix whose only new production code is a 7-line mark-on-entry block in a
single site (internal/tui/model.go handleMultiSelectToggle, ~lines 3511-3518); the
2-line "insert Session.Name into the set" it shares with the toggle branch below
(~3532-3536) is intentionally identity-aligned and well under the Rule-of-Three
extraction threshold. The only genuinely new test code — the enterMultiSelectEmpty
helper (internal/tui/multi_select_keymap_test.go:57) — is itself a consolidation
that replaces an inline "enter + toggle-off" idiom at ~13 call sites and reduces
duplication rather than adding it (single definition, no rival helper). The
remaining "enter + markRow loop" repetition across the burst_*_test.go setups is
pre-existing structure from the restore-host-terminal-windows work unit; this
implementation only swapped the entry call on those lines and did not create the
pattern, so it falls outside plan scope. The changed test files are otherwise
assertion flips (count 0 -> count 1) with no repeated blocks worth extracting.
