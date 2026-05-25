TASK: 3-5 — Integration test for clean bootstrap end-state and lock-loser persistence

STATUS: Issues Found (non-blocking) — Complete with documented spec deviation

SPEC CONTEXT: Component F acceptance bullets 1–3 require (a) zero `"no such session: _portal-saver"` log entries on clean bootstrap, (b) `destroy-unattached=off` + pane process == `portal state daemon`, (c) lock-loser daemon exit does NOT destroy the session.

IMPLEMENTATION:
- Status: Implemented with one documented deviation
- Location:
  - `internal/tmux/portal_saver_endstate_integration_test.go:128` — `CleanBootstrap_EndState`
  - `internal/tmux/portal_saver_endstate_integration_test.go:279` — `LockLoser_NoNoSuchSessionLogNoise` (renamed from spec's `LockLoser_SessionPersists`)
  - `internal/tmux/portal_saver_endstate_integration_test.go:447` — `EnvironmentInheritanceAcrossRespawn` (task 3-6, colocated)
- DEVIATION: Test #2 renamed because on tmux 3.6b without `remain-on-exit on`, session DOES disappear when lock-loser daemon exits — even with `destroy-unattached=off`. Test recasts the F invariant as "absence of no-such-session log noise during the lock-loser cascade". Header documents the deviation at length. Spec text NOT updated; spec/impl mismatch exists for future readers.
- Negative-case limitation disclosed (lines 268-278): literal revert of 3-2's reorder is timing-dependent on tmux 3.6b/darwin/arm64

TESTS:
- Status: Adequate (with deviation caveats)
- Coverage:
  - HasSession (158); destroy-unattached=off via show-options (168-171); pane pid → ps -o args= containing `portal state daemon` with placeholder-absence cross-check (180-212)
  - `portal.log` scan via `assertNoNoSuchSessionEntries` (221, 379, 625-640)
  - Lock-loser: seeded competing daemon + post-bootstrap log scan + bootstrap return-value cascade-substring detector (398-409) — strongest negative-case probe
  - Tmux server kept alive via `ptl-keepalive` dummy (300-304)
  - `portaltest.IsolateStateForTest` used (139, 283)
- 2500 ms `lockLoserCascadeWindow` flat `time.Sleep` (not poll) — justified by absence-of-substring nature
- `assertNoNoSuchSessionEntries` reads `portal.log` only; comment notes short-lived tests never rotate

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; helper-style polling via `tmuxtest.PollUntil`
- SOLID: Good; each test pins one observable; helpers single-responsibility
- Complexity: Low; linear pre-condition / action / assertion
- Modern idioms: `envValue` struct cleanly handles `tmux show-environment`'s `-NAME` unset marker
- Readability: Good; exhaustive headers explain rationale and race-window limitations

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] **Spec ↔ implementation mismatch (notable)**: spec acceptance bullet 3 literally asserts "session persists after the daemon exits", but implementation (driven by observed tmux 3.6b behaviour) asserts "no log-noise cascade". Either amend spec to record actual tmux behaviour and renamed assertion, OR add `remain-on-exit on` so literal spec assertion holds. Needs discussion
- [idea] Negative-case detection of literal 3-2 revert is timing-dependent; strengthen via injected daemon-exit delay or rely on unit-level argv-recorder in 3-2's test
- [quickfix] `lockLoserCascadeWindow` flat 2500 ms sleep could be tuned smaller if daemon-exit latency is bounded
- [idea] `EnvironmentInheritanceAcrossRespawn` (task 3-6) colocated in same file, mixing 3-5 and 3-6 scope (allowed per spec "or sibling file")
