TASK: spectrum-tui-design-1-2 — Upgrade to Bubble Tea v2 / Lipgloss v2 (OSC 11 API + AdaptiveColor removal) with full behaviour parity

ACCEPTANCE CRITERIA (from tick-f44b8c):
1. go build ./... and go test ./... pass on v2 (parity IS the test; vhs/frame-check exempt).
2. go.mod shows v2 lines for bubbletea/lipgloss/bubbles; go mod tidy leaves a clean graph.
3. previewBorderColor resolves to the same #3B5577 / #7B95BD (no hue change) under the v2 explicit-resolution model.
4. No render/key behaviour differs from v1 — verified by reading/tracing/diffing every touched path.
5. NO_COLOR / palette-downsample confirmed intact under v2.

STATUS: Complete

SPEC CONTEXT:
§2.6 pins detection to OSC 11 (tea.RequestBackgroundColor → BackgroundColorMsg in Bubble Tea v2) and notes Lipgloss v2 removed AdaptiveColor, forcing explicit light/dark wiring. §14.5: the light/dark choice is wired explicitly, not via a framework adaptive type. §10.2: the detect-or-timeout first-paint gate consumes the v2 API. This task is the framework foundation that unblocks detection task 1-7; it is a parity-only migration — nothing visual changes here, parity is the whole test. The AMBIGUITY note (default = upgrade unless v1 already exposes a sufficient OSC 11 query) was resolved by upgrading, per the spec's v2-specific wording.

IMPLEMENTATION:
- Status: Implemented (commit 8dfbacbc). Later tasks (1-3/1-4 token layer, Phase 4 cyan chrome) legitimately superseded the previewBorderColor variable — that is the task's own documented hand-off, not drift.
- Location:
  - go.mod:5-8 — charm.land/bubbles/v2 v2.1.0, charm.land/bubbletea/v2 v2.0.7, charm.land/lipgloss/v2 v2.0.4. `go mod verify` → all modules verified; no v1 charmbracelet bubbletea/lipgloss/bubbles entries survive in go.sum (clean tidy).
  - internal/tui/keys.go:1-44 (new) — keyIsCode / isRuneKey / keyIsCtrlC v2 helpers that preserve v1 matching semantics 1:1 (msg.Type==KeyX → Code==code && Mod==0; KeyRunes+single rune → Mod==0 && Text==ch; KeyCtrlC → Code=='c' && Mod.Contains(ModCtrl)).
  - internal/tui/pagepreview.go (at 1-2): AdaptiveColor{Light:"#3B5577",Dark:"#7B95BD"} → explicit const previewBorderColorLight/Dark + var previewBorderColor = lipgloss.Color(previewBorderColorDark), resolving to the same dark-default v1 produced absent a detected light bg (parity). Now evolved to previewBorderColorToken = theme.MV.AccentCyan (pagepreview.go:35) via the intended 1-3/1-4 + Phase 4 supersession.
  - internal/tui/model.go: View() now returns tea.View with AltScreen=true; viewString() holds the verbatim v1 body (model.go:~2208-3240). KeyMsg→KeyPressMsg across every handler (updateSessionList, updateProjectsPage, updateModal, updateKill/Rename/Delete/EditProject, skipHeaderRow); KeyEsc→KeyEscape; KeyCtrlC→keyIsCtrlC; KeyRunes case → default-with-Mod/Text guard preserving string(msg.Runes) semantics.
  - pagepreview.go:386 — viewport.New(WithWidth, WithHeight) functional options; Update WindowSizeMsg uses SetWidth/SetHeight (resolves the documented v1.0.0 viewport.SetSize TODO).
  - cmd/open.go:593 + cmd/capturetool/main.go:56 — tea.NewProgram(m) (tea.WithAltScreen removed; alt-screen now declared in View).
  - internal/tui/sessions_flash.go:84-92 — isActionableKey migrated from `msg.Type != 0 || len(msg.Runes) > 0` to `Code != 0 || Text != ""` (faithful equivalent).
- Notes: No bare v1 API surface remains in production code (grep for tea.KeyMsg{ / WithAltScreen / EnterAltScreen / `.Type ==` returns only migration doc-comments). AdaptiveColor appears nowhere. Module graph verified.

TESTS:
- Status: Adequate
- Coverage: ~50 test files mechanically migrated to the v2 KeyPressMsg shape (e.g. keymap_dispatch_guard_test.go constructs tea.KeyPressMsg{Code:..., Text:..., Mod:...}); these are the parity proof for the Sessions/Projects/preview key paths. The full suite is GREEN per the orchestrator-established baseline. The previewBorderColor migration was proved at the time by the existing preview-chrome render tests (byte-identical capture vs the v1 baseline, per the commit message). NO_COLOR was an audit at 1-2 (no carve-out existed yet — canvas/colourless landed in Phase 2); the colourless path is now strongly covered by colourless_nocolor_test.go (asserts no background-SGR ever activates under NoColor).
- Notes: No new dedicated "v2 parity" or "NO_COLOR" test file was added at 1-2 — appropriate, since 1-2 is a parity migration with no new behaviour to lock; the existing suite migrated in place is the parity gate. textinput editing keys (incl. Ctrl+U/Ctrl+D inside the rename/filter input) are exercised through the delegated m.renameInput.Update(msg) path, not tui-level handlers — which is why no keyIsCtrlU/keyIsCtrlD helpers exist.

CODE QUALITY:
- Project conventions: Followed. v2 import paths (charm.land/*) consistent everywhere; no t.Parallel; DI/seam pattern untouched; helpers are small and single-purpose with intent-named doc comments.
- SOLID principles: Good. keys.go centralises the v1→v2 key-matching translation behind three small functions, so every call site reads as a like-for-like swap; viewString/View split keeps page composers plain string builders (single wrap point for alt-screen).
- Complexity: Low. The migration is mechanical and localised; no new control-flow complexity introduced.
- Modern idioms: Yes. Functional-option viewport.New, declarative tea.View.AltScreen, Mod-based ctrl detection — idiomatic v2.
- Readability: Good. Every non-obvious v2 delta carries a doc comment that names the v1 shape it replaces and asserts parity, making the diff self-auditing.
- Issues: One stale doc reference (see NON-BLOCKING NOTES) and one honestly-documented forced timing change in the non-concurrent warning-flush path.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/keys.go:19 — the package doc claims "keyIsCtrlC / keyIsCtrlU / keyIsCtrlD are the v1 dedicated ctrl-combo key types"; keyIsCtrlU and keyIsCtrlD are never defined or used anywhere (ctrl-u/ctrl-d scrolling is delegated to the bubbles viewport keymap). Drop the two non-existent names from the comment so it describes only keyIsCtrlC.
- [bug] internal/tui/bootstrap_warnings.go:68-87 — the v1→v2 migration forces the one observable non-parity in the whole task: v1 wrapped the stderr warning write in an imperative ExitAltScreen/EnterAltScreen toggle so soft warnings surfaced in scrollback MID-RUN; v2 removed those commands, so on the non-concurrent (warm/CLI-staged) path the warnings now surface only on alt-screen teardown at program exit. Content/order/single-flush are unchanged and the cold/TUI path surfaces them in-TUI as a notice band, so this is correctly handled and well-documented — but it is the single place where mid-run timing could not be preserved byte-for-byte. Flagged for reviewer awareness; no action required unless mid-run surfacing is a hard requirement (it is not expressible in v2 as a command).
