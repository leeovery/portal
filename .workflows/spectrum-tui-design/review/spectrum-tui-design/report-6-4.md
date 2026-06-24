TASK: spectrum-tui-design-6-4 — Remove the stale post-detection documentation and the dead dark-pinned cursorStyle var

ACCEPTANCE CRITERIA:
- theme.go package doc + Token.Color() comment no longer claim detection "lands in 1-7" / hard-pins dark; they accurately describe the canvasMode → ColorFor(mode) flow.
- The package-level cursorStyle var and its comment are removed from session_item.go.
- The edit_modal.go:170 shadowing local cursorStyle is untouched and still compiles.
- go build succeeds; internal/tui passes with no unused-variable / unreferenced-symbol errors.
- No behavioural change to any rendered output in either mode.

STATUS: Complete

SPEC CONTEXT:
Spec §2.6 (light/dark detection & canvas selection) and §2.9 (MV token table) describe the SHIPPED detection flow: OSC 11 query → BackgroundColorMsg → light/dark, gated by detect-or-timeout, with an `appearance: auto|light|dark` pref override; NO_COLOR skips detection. The resolved mode flows from the appearance gate into the model's canvasMode and is carried by the delegates as their Mode; every renderer resolves each token per-mode via Token.ColorFor(mode). Detection HAS landed (appearance_gate.go + model.canvasMode + syncResolvedMode), so any comment claiming it is still pending or that resolution hard-pins DARK is stale. This task is the Phase-6 analysis-cycle cleanup of two such artifacts; acceptance is removal/correction landing with no behavioural change.

IMPLEMENTATION:
- Status: Implemented (verified via commit 88e95441 and current file state)
- Location:
  - internal/tui/theme/theme.go:1-12 — package doc rewritten: "Each token carries BOTH a Light and a Dark variant. The resolved appearance flows from the §2.6 appearance gate into the model's canvasMode, which the delegates carry as their Mode; renderers resolve each token per mode via ColorFor(mode)." The stale "Detection (§2.6) lands in 1-7; until then ColorFor(Dark) is what every Color() call resolves to" / "Resolution currently defaults to the DARK variant" phrasing is gone.
  - internal/tui/theme/theme.go (Token.Color() comment) — at the time of this task (commit 88e95441) the Token.Color() method still existed and its comment was correctly rewritten to "Color is a dark-pinned convenience that always resolves the DARK variant … the live renderers resolve per mode via ColorFor(mode)". The whole Color() method was LATER removed by task 9-2 (commit 3ad3c624) — so the method no longer exists; this is correct for 6-4's scope as it stood.
  - internal/tui/session_item.go:13-19 — the package-level `cursorStyle = lipgloss.NewStyle().Foreground(theme.MV.AccentViolet.Color())` var and its "Retained for the projects page…" comment are deleted. The remaining `var ( nameBase … )` block (session_item.go:15-20) is intact; lipgloss + theme imports remain heavily used (30 references), so no orphaned import.
  - internal/tui/edit_modal.go:167-176 — the function-local `cursorStyle := lipgloss.NewStyle().Reverse(true)` shadowing local is untouched and resolves per-mode via AccentOrange.ColorFor(mode) / Canvas.ColorFor(mode). It is unrelated to the removed package-level var.
  - Verified project_item.go (the var's former justification) resolves the selection accent per-mode end-to-end: ProjectDelegate.rowToken → rowTokenStyle → AccentViolet.ColorFor(d.Mode) (project_item.go:86-88, 141; session_item.go:309-318). It never read the removed var, so removal is behaviour-neutral.
- Notes: No production reference to the package-level cursorStyle remains anywhere in internal/tui (grep confirms only the edit_modal.go local). The three scoped acceptance items are all satisfied.

TESTS:
- Status: Adequate (compilation is the primary and correct guard for this change class)
- Coverage: This is a documentation + dead-code-removal task with zero behavioural surface. The build itself is the guard: removing an unreferenced package-level var and editing comments cannot regress runtime behaviour. The brief confirms `go build` + full `go test ./...` are GREEN. No doc-accuracy guard test exists in internal/tui/theme/ (only theme_test.go and contrast_test.go), so none could regress; per the task's own Tests note, no new test is required.
- Notes: A doc-accuracy guard test would be the only thing that could have caught the new dangling Color() reference in the package doc (see NON-BLOCKING NOTES); none exists, which is acceptable for this task but is why the dangling reference slipped through the later 9-2 removal.

CODE QUALITY:
- Project conventions: Followed. Dead-code removal aligns with the YAGNI / code-style guidance cited in the task; the var was flagged unused by golangci-lint and its removal clears that finding. No raw hex / ANSI literal reintroduced.
- SOLID principles: Good — removing the last would-be render-site dark-pinned .Color() call eliminates a latent single-responsibility/consistency hazard (a per-mode resolver with one stray hard-pinned escape hatch).
- Complexity: Low — pure deletion + comment edit.
- Modern idioms: Yes.
- Readability: Good — the rewritten package doc accurately narrates the shipped flow.
- Issues: One staleness now exists in a file this task owns, introduced by the LATER task 9-2's incomplete cleanup (not by 6-4): the package doc at theme.go:10-11 still reads "The dark-pinned Color() convenience survives only for the handful of not-yet-mode-resolved call sites" — but task 9-2 (commit 3ad3c624) deleted the Token.Color() method entirely. The referenced method no longer exists and the claim is now false (there are zero not-yet-mode-resolved call sites; Color() was the only dark-pin and it is gone). Non-blocking and out of 6-4's original scope, but it is a live dangling reference in theme.go.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/theme/theme.go:10-11 — The package doc still says "The dark-pinned Color() convenience survives only for the handful of not-yet-mode-resolved call sites." Task 9-2 removed the Token.Color() method (commit 3ad3c624), so this references a method that no longer exists. Delete that sentence (the flow description in lines 6-10 stands on its own; every renderer now resolves via ColorFor(mode)). Doc-only, zero logic impact. (Introduced by 9-2's incomplete cleanup, surfaced here because 6-4 owns this doc.)
- [do-now] internal/tui/theme/theme.go:20-21 — The Mode-type doc comment still reads "Until OSC 11 detection lands (1-7), the resolver defaults to Dark." Detection has shipped (§2.6 appearance gate); reword to state Dark is the no-answer fallback the resolver defaults to, dropping "Until … lands (1-7)". Out of 6-4's scope (the sibling 1-3/1-4/1-9 verifiers flagged the cluster) but co-located with this task's edits.
- [do-now] internal/tui/theme/theme.go:30, 36, 118 — Residual "filled by task 1-4 … wired to detection by task 1-7" / "Light is a placeholder filled by task 1-4" phrasing. Tasks 1-4 and 1-7 have shipped (Light variants are pinned at lines 134-183; the resolver is wired), so these read as still-pending. Reword to past/shipped tense or drop the task-number references. Out of 6-4's scope; noted for the broader theme.go doc sweep the sibling verifiers opened.
