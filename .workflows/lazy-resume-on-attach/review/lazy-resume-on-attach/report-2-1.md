TASK: The Pending Marker: One Name, Set, Read and Clear (lazy-resume-on-attach-2-1, tick-ecf7fe)

ACCEPTANCE CRITERIA:
- `state.ResumePendingOption` is the only spelling of `@portal-resume-pending` in the tree — no format string, option argument or test restates the literal.
- `SetResumePendingMarker` issues exactly one `SetPaneOption` carrying that constant and the value `1` against the target it was handed, and no unset; `UnsetResumePendingMarker` issues exactly one `UnsetPaneOption` carrying that constant, and no set; both return the writer's error unchanged.
- `tmux.UnsetPaneOption` composes `set-option -pu -t <target> <name>` and takes `tmux.Target`, so a hand-composed string argument does not compile, and `internal/tmux/target_composition_guard_test.go` passes with the method in place.
- Against a real tmux pane: a set marker reads back as `1` through `#{@portal-resume-pending}`; after the unset it reads back empty; a second unset of the now-absent option succeeds; an unset against a target naming no live pane fails.
- `ResumePendingSet` reports true for `1` and for any other non-empty value, and false for the empty string.
- `internal/state` still compiles without importing `internal/tmux` — the seam is generic over `~string`, not over the client's concrete target type.

STATUS: complete

SPEC CONTEXT: The waiting pane's capture freeze must be held by the pane rather than by its positional address, so the spec picks the pane user-option `@portal-resume-pending`, set to `1`, read by presence (the section on how the freeze is held by the pane, and the section on explicitly marking a pending pane). A pane option dies with its pane, so it has no staleness case and needs no sweep. Bootstrap's server-option sweep cannot see it. Later phases set it before the mid-restore marker clears, clear it from the waiter, and read it from capture, doctor and the picker. This task supplies only the vocabulary: the name, the presence rule, the set/unset helpers and the missing tmux remover.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/markers.go:31 — `ResumePendingOption = "@portal-resume-pending"`, beside `PortalPaneIDOption` (:26)
  - internal/state/markers.go:52-55 — `PaneOptionWriter[T ~string]` with `SetPaneOption(target T, name, value string) error` / `UnsetPaneOption(target T, name string) error`
  - internal/state/markers.go:105-107 — `SetResumePendingMarker` writes `ResumePendingOption` = "1"
  - internal/state/markers.go:111-113 — `UnsetResumePendingMarker` removes `ResumePendingOption`
  - internal/state/markers.go:117-119 — `ResumePendingSet(optionValue) = optionValue != ""`
  - internal/tmux/tmux.go:329-335 — `UnsetPaneOption(target Target, name string)` runs `set-option -pu -t string(target) name` and wraps a failure as `failed to unset pane option %s on %s: %w`, the same shape `SetPaneOption` (tmux.go:319-325) uses
  - CLAUDE.md — the `state` row's marker-helpers clause names the pane-scoped `@portal-resume-pending`, `ResumePendingOption`, the set/unset helpers over `PaneOptionWriter[T ~string]`, and the `ResumePendingSet` presence rule, beside `@portal-restoring`. The `tmux` row's options clause adds `UnsetPaneOption` after `SetPaneOption` as the `set-option -pu -t <pane> <name>` remover, and its list of methods handed an already-composed target now includes `UnsetPaneOption`.
- Notes:
  - Criterion 1: a search of every `.go` file finds the literal only at internal/state/markers.go:31. Every other site composes the name from `state.ResumePendingOption`: tmuxtest/stamp.go:39,47, the hydrate backstop's `set-option -pu` argv at cmd/state_hydrate.go:293, and the test format reads. CLAUDE.md and .tick/tasks.jsonl mention the literal in prose and task data. Neither is a format string, option argument or test.
  - Criterion 3: `UnsetPaneOption` declares `Target`. The guard accepts any parameter declared with `Target`/`tmux.Target` as already vouched for (`targetTypeNames`, internal/tmux/target_composition_guard_test.go:113-116), and this argv has the same `string(target)` shape as `SetPaneOption`, which the guard already accepts.
  - Criterion 6: markers.go imports only `strings` and `internal/tmuxout`. `internal/tmux` imports `internal/state` (tmux.go:249), so a reverse edge could not compile at all.
  - Production callers already fit the seam with the concrete client: cmd/state_hydrate.go:375, cmd/state_resume_wait.go:483, cmd/state_resume_recover.go:92. So the generic parameterisation is shown to reach `*tmux.Client`, not only the test mock.
  - Comments in the changed code match the code and the recorded tmux measurements.

TESTS:
- Status: Adequate
- Coverage:
  - internal/state/markers_test.go:399-427 — `paneWriterMock` is modelled on `writerMock`. It records target, name and value over a local `paneTarget` string kind, which also shows the seam is generic over a `~string` type other than `tmux.Target`.
  - markers_test.go:430-442 / :462-474 — set and unset each issue exactly one call, and whole-struct equality checks target, `state.ResumePendingOption` and value "1".
  - markers_test.go:444-450 / :476-482 — writer-error propagation from each helper, checked with `errors.Is` against a sentinel.
  - markers_test.go:452-458 / :484-490 — set issues no unset, and unset issues no set.
  - markers_test.go:493-509 — presence table over `1`, `0`, `yes`, `""`.
  - internal/tmux/pane_option_test.go:63-94 — argv pinned as `set-option -pu -t %3 <ResumePendingOption>` through `commandertest.New` (which returns `*commandertest.Scripted`), plus the wrapped failure carrying pane, option name and tmux's words.
  - internal/tmux/pane_option_realtmux_test.go:31-77 — unit lane, no build tag. `seedRealTmuxServer` calls `tmuxtest.SkipIfNoTmux` and uses a disposable `tmuxtest.New` socket. The file covers set → `#{...}` format read "1" → unset → empty read; two consecutive unsets of a never-set marker, both succeeding; and an unset against `%99999` failing.
- Notes: The argv test is in `pane_option_test.go` rather than `tmux_test.go`, and the "second unset" runs in its own subtest on a never-set pane instead of at the tail of the set/unset round trip. Neither loses anything: tmux reports an unset pane option and a never-set one identically, and every named test is present. Each subtest asserts one property, and none is over-tested.

CODE QUALITY:
- Project conventions: Followed. Option names come from a single constant; `-t` takes the typed `Target`; the new seam sits in `internal/state` without importing tmux; the error wrap matches `SetPaneOption`'s shape; tests use the `commandertest`/`tmuxtest` fixtures and no `t.Parallel()`.
- SOLID principles: Good. The two-method interface is limited to what the helpers need.
- Complexity: Low
- Modern idioms: Yes. Generics over `~string` keep the seam in reach of the named `tmux.Target` type.
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "Against a real tmux pane: a set marker reads back as `1` through `#{@portal-resume-pending}`; after the unset it reads back empty; a second unset of the now-absent option succeeds; an unset against a target naming no live pane fails." — The real-tmux subtests that assert each clause exist and are correct on reading. Whether tmux behaves this way has to be observed by running `go test ./internal/tmux -run TestUnsetPaneOption_RealTmux` against the installed tmux.
