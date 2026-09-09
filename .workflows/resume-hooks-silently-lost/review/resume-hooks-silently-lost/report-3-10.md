TASK: resume-hooks-silently-lost-3-10 — tmux.PaneTarget's Comment States The Rule The Package Actually Follows (tick-104a61)

ACCEPTANCE CRITERIA:
- `PaneTarget`'s doc comment states when exactness matters rather than prohibiting what the package does
- No non-comment line changes anywhere
- `go test ./...` and `go test -tags integration -p 1 ./...` pass, unchanged

STATUS: complete

SPEC CONTEXT:
The specification (`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md`) says nothing about the tmux target-exactness vocabulary — its ten corrigenda cover stand-down reasons, the `internal/nanoid` leaf, the two lock bounds, `StaleKeys`, and the lane rule. This task is a phase-3 implementation-analysis consolidation whose authority is its own body: the pre-existing `PaneTarget` doc carried a blanket prohibition ("Anything issuing a tmux `-t` flag must use PaneTargetExact") that the package's own code contradicted — `internal/restore/session.go` built its `liveTarget` with `PaneTarget` and spent it on `respawn-pane -k -t` and `set-option -p -t`, and `internal/tmuxtest/stamp.go` did the same across five fixtures. The true hazard, already stated accurately by the exact-target sibling, is *session-level* prefix matching: `-t foo` silently resolving to a live `foo-2` once `foo` is gone. The work is a pure comment edit with the call sites explicitly ruled out of scope.

IMPLEMENTATION:
- Status: Implemented
- Location: commit `3c1d9424`, `internal/tmux/tmux.go` only. Current text: `internal/tmux/tmux.go:403-409` (PaneTarget doc), `:414-416` (PaneTargetExact doc).
- Notes:
  - The new doc states the condition rather than the prohibition — "for display, for name-based keys, and for a `-t` flag whose session is known to be live under that exact name", then names the hazard and its bound: `=` pins the session half only, so a renumbered coordinate is caught by neither form. That is exactly the substance the task directed be taken from the exact-target sibling.
  - The sibling at `:414-416` was corrected in the same pass ("the `=` … prefix a `-t` needs when its session may be gone. See PaneTarget."), which is what stops the edit from falsifying the doc two lines below it.
  - Comment-only is verified, not assumed: `git show 3c1d9424 -- internal/tmux/tmux.go` changes eleven lines, all of them `//` lines, in two doc blocks. The commit's other files (`.tick/tasks.jsonl`, the manifest, the fix-tracking record) are workflow artifacts, not source.
  - The prohibition survives nowhere in source. Its only remaining occurrences in the tree are the `.workflows/` consolidation records that raised it.
  - Later phases moved past this task without invalidating it: `internal/restore/session.go:140` and `cmd/state_daemon.go:278` now compose `PaneTargetExact`, and `internal/tmux/tmux.go:825`/`:839` keep `PaneTarget` for the error-message string beside a `PaneTargetExact` argv. Production therefore issues no `-t` from `PaneTarget` today, while integration fixtures still do (e.g. `internal/restore/integration_full_test.go:157`, `cmd/bootstrap/reboot_roundtrip_test.go:407`) — which the doc's stated condition covers, and which `internal/tmux/target_composition_guard_test.go` does not police because it scans non-test sources only. The doc's phrasing is also the repo-wide convention rather than a local invention: `windowTarget` at `internal/tmux/tmux.go:480-483` says the same thing, and CLAUDE.md's architecture table restates it for both bare forms.

TESTS:
- Status: Adequate (none required)
- Coverage: The task specifies no tests, correctly — a doc-comment edit changes no behaviour. The rendering of both forms is already pinned independently: `internal/tmux/tmux_test.go:2282` (`TestPaneTarget`) and `internal/tmux/target_type_test.go:15-38`, whose second subtest asserts through a `string`-typed helper so `PaneTarget` returning a `Target` would fail to compile.
- Notes: Judged by reading, the edit cannot move any assertion — no identifier, signature or expression changed.

CODE QUALITY:
- Project conventions: Followed. Comment-only, so the lane rules, logging vocabulary and isolation invariants are untouched.
- SOLID principles: N/A
- Complexity: Low (unchanged)
- Modern idioms: N/A
- Readability: Good. The doc now says what the reader needs at a `PaneTarget` call site — the condition under which the bare form is safe, and the precise limit of what `=` buys — instead of a rule that would have sent them auditing five fixtures and two production lines.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None
