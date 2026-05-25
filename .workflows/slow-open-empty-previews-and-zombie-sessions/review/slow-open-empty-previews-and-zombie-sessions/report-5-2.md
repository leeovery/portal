TASK: 5-2 — Extract saverMembershipProbe seam and add tmux.SaverPanePID helper

STATUS: Complete (with structural refinement documented)

SPEC CONTEXT: Component D per-tick self-check — `has-session` → `list-panes -F '#{pane_pid}'` compared to `os.Getpid()`. Any tmux error collapses to "absent". Phase 2 shipped `tmux.ErrNoSuchSession`; daemon classifies via `errors.Is`.

IMPLEMENTATION:
- Status: Implemented (with deliberate documented surface change)
- Locations:
  - `internal/tmux/saver_pane_pid.go:48` — unexported `saverPanePID` rich-classification primitive (success / wrapped `ErrNoSuchSession` / `ErrEmptyPaneList` / `ErrPanePIDParse` / generic exec error)
  - `internal/tmux/saver_pane_pid.go:91` — exported `SaverPanePIDOrAbsent(c, sessionName) (pid, present, err)` — sole exported entry point centralizing `ErrNoSuchSession || ErrEmptyPaneList → (0, false, nil)` collapse for both Component D and bootstrap step 4
  - `internal/tmux/export_test.go:38` — `var SaverPanePID = saverPanePID` test re-export
  - `internal/tmux/errors.go:48,61` — new `ErrEmptyPaneList`, `ErrPanePIDParse` sentinels
  - `cmd/state_daemon.go:80` — `var saverMembershipProbe = defaultSaverMembershipProbe`
  - `cmd/state_daemon.go:102-111` — `defaultSaverMembershipProbe`
- Deviation: plan called for single exported `SaverPanePID(*Client, string) (int, error)` consumed directly. Implementation exports `SaverPanePIDOrAbsent(*Client, string) (int, bool, error)` and keeps rich form unexported. Spec's "treat any error as absent" rule encoded once in `SaverPanePIDOrAbsent` instead of being re-derived at each call site. Phase 4 orphan-sweep adapter and Component D probe share collapse policy by construction. Justified in-source at lines 13-16, 70-90. All AC bullets satisfied

TESTS:
- Status: Adequate
- Coverage:
  - `internal/tmux/saver_pane_pid_test.go` — all six plan-required shapes with `=`-prefix argv pinning
  - `cmd/state_daemon_test.go:791-842` — four cases: !HasSession (asserts list-panes NOT invoked — short-circuit), SaverPanePID error, pid match, pid mismatch
  - `cmd/state_daemon_test.go:849-861` — observable-behaviour guard for production wiring
- `membershipFakeCommander` keeps argv-shape pinning
- Whitespace-only-stdout case is extra coverage (not over-tested)

CODE QUALITY:
- Project conventions: Followed; `=`-prefix; `Commander` mock; package-level seam pattern matches existing
- SOLID: Good; sole-exported-entry-point enforces Open/Closed at package boundary
- Complexity: Low; `saverPanePID` ~parse-and-classify; `SaverPanePIDOrAbsent` 6-line wrapper
- Modern idioms: multi-`%w` wrapping; `errors.Is` at collapse layer
- Readability: Excellent; godoc cites spec section + rationale for unexported rich-sentinel form

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan specified exported `SaverPanePID(*Client, string) (int, error)`; shipped surface is `SaverPanePIDOrAbsent`. Well-motivated, documented; one-line note in planning memo explaining divergence would aid plan-vs-implementation traceability
