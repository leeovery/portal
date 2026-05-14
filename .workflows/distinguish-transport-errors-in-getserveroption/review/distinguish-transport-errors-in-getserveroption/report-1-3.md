# Review Report: Task 1-3 — Add optionAbsentStderrPatterns slice and rewrite GetServerOption to discriminate via errors.As

STATUS: Complete
FINDINGS_COUNT: 0 blocking issues
SUMMARY: Task 1-3 fully implemented and tested — `optionAbsentStderrPatterns` slice and the rewritten `GetServerOption` discriminator faithfully match the spec, with comprehensive same-package and external-package test coverage.

## Acceptance Criteria
- `optionAbsentStderrPatterns` exists as unexported `[]string` in `internal/tmux` with exactly the three case-sensitive patterns.
- `GetServerOption` returns `(TrimSpace(output), nil)` on success.
- Returns `("", ErrOptionNotFound)` iff `err` unwraps via `errors.As` to a `*CommandError` whose `Stderr` contains one of the patterns.
- Propagates original wrapped error for (a) `*CommandError` with empty `Stderr`, (b) unmatched stderr, (c) non-`*CommandError` errors.
- `TryGetServerOption` returns `("", false, non-nil err)` for non-absent failures — dead branch now live.
- No new exported symbols beyond Task 1-1.
- Production sweep performed; `go test ./...` passes.

## Status: Complete

## Spec Context
Spec "Design: Discrimination in GetServerOption" mandates extraction of wrapped `*CommandError` via `errors.As` and substring-match of its `Stderr` against an unexported package-level slice. Only on match does it collapse to `ErrOptionNotFound`; everything else propagates the original wrapped error unchanged. Matching is case-sensitive `strings.Contains`; verbatim `Stderr` storage (including trailing whitespace) is tolerated. Pattern membership: `"invalid option:"`, `"unknown option:"`, `"ambiguous option:"` — the third empirically derived from `show-option -sv ""` on Darwin 25.3.0.

## Implementation
- Status: Implemented
- Locations:
  - `internal/tmux/tmux.go:23-27` — `optionAbsentStderrPatterns` declaration with the three patterns and explanatory comment.
  - `internal/tmux/tmux.go:331-354` — `GetServerOption` rewrite with full contract docstring.
- Notes:
  - Slice contents and ordering exactly match spec: `["invalid option:", "unknown option:", "ambiguous option:"]`.
  - Implementation matches spec's recommended structure verbatim — short-circuit on first match, no regex, no normalisation.
  - Fallthrough correct: when `errors.As` returns false or no pattern matches, original `err` returned unchanged on line 353, preserving the `*CommandError` chain.
  - `GetServerOption` docstring (lines 331-339) goes beyond the minimum task scope and covers the post-fix contract clearly — overlaps task 1-5 deliverable but does not break anything.
  - `TryGetServerOption` body (tmux.go:370-379) correctly left unchanged; previously-dead branch (lines 375-377) is now live.
  - Production-code sweep confirmed: only `internal/tmux/tmux.go` and `internal/state/markers.go:146` reference these symbols in production. Matches the spec's audited surface.

## Tests
- Status: Adequate
- Coverage:
  - **Reshape** at `internal/tmux/tmux_test.go:924-939` — `TestGetServerOption` "option does not exist" subtest now uses `&tmux.CommandError{Stderr: "unknown option: @portal-active-%3", Err: errors.New("exit status 1")}` instead of bare `errors.New`. Old code passed by accident; new code passes because stderr genuinely matches.
  - **Transport error**: `tmux_test.go:947-988` — `TestGetServerOption_TransportError` table-driven over `socket_connect_failure` and `lost_server` stderr shapes; asserts `!errors.Is(err, ErrOptionNotFound)`, `errors.As` recovers `*CommandError`, recovered Stderr matches verbatim.
  - **Non-exit error**: `tmux_test.go:994-1017` — `TestGetServerOption_NonExitErrorPropagates` uses `*CommandError{Stderr: "", Err: errors.New("exec: \"tmux\": not found")}`.
  - **Try-wrapper propagation**: `tmux_test.go:1772-1798` — `TestTryGetServerOption_PropagatesTransportError` covers the previously-unreachable `err != nil` branch through the public surface.
  - **Discriminator-set + slice pinning** (same-package, internal): `internal/tmux/option_discriminator_internal_test.go:38-109` — `TestGetServerOption_DiscriminatorSet` iterates `optionAbsentStderrPatterns` directly so future pattern additions auto-extend coverage; includes negative `unrelated_stderr_does_not_match` subtest; adds `slice_contents_pinned` subtest asserting exact membership.
  - `TestTryGetServerOption` (`tmux_test.go:1745-1762`) "found=false when not found" also updated to use `*CommandError` shape — strict over-scope but sensibly bundled.
- Notes:
  - `option_discriminator_internal_test.go` deliberately lives in package `tmux` to read the unexported slice — matches spec guidance ("same-package, white-box").
  - `slice_contents_pinned` is slightly beyond spec minimum but defensible as anti-drift insurance.
  - No `t.Parallel()` usage — adheres to CLAUDE.md project policy.
  - `internalMockCommander` (option_discriminator_internal_test.go:14-25) duplicates `MockCommander`'s pinhole shape because the external package's `MockCommander` is unreachable from package `tmux` without an import cycle; documented at lines 8-13.
  - All assertions are behavioural (`errors.Is`, `errors.As`, `strings.Contains`) — never against exact `.Error()` strings.

## Code Quality
- Project conventions: Followed. Go 1.21+ idioms; `errors.As` over type assertion; full docstrings on exported types/functions; no `t.Parallel()`.
- SOLID: Good. `GetServerOption` has single responsibility; pattern slice open for one-line extension without modifying discriminator logic.
- Complexity: Low. ~12 lines; cyclomatic ~4 (success / errors.As / loop / fallthrough). Linear iteration over three entries.
- Modern idioms: Yes. `errors.As`, `strings.Contains`, range-over-slice. No regex.
- Readability: Good. Docstring names failure modes explicitly and points at `errors.As`. Slice comment cross-references the unit-test file.
- Issues: None blocking.

## Blocking Issues
- None.

## Non-Blocking Notes
- [idea] The `GetServerOption` docstring (`tmux.go:331-339`) substantively delivers task 1-5's "Site 2" deliverable. If task 1-5 is reviewed independently, this overlap should be acknowledged so the reviewer does not double-count or attempt to rewrite what already exists.
- [idea] The `slice_contents_pinned` subtest goes slightly beyond the spec's minimum (per-entry positive + one negative). It is defensible as anti-drift insurance but a strict reviewer could classify it as over-testing. Net assessment: keep — silent membership drift would be a contract-faithful regression.
- [quickfix] The slice comment (`tmux.go:21`) hardcodes the filename `option_discriminator_internal_test.go`. If the file is renamed, the comment goes stale. Optional: rephrase to "the same-package internal test file" without the specific filename.
