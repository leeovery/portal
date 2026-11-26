## Discovery Session 001

Date: 2026-08-15
Work unit: split-oversized-go-files

### Description (as of session)

Set Portal's own file-organisation standard, record it in CLAUDE.md, and split the oversized Go files along their concern seams — standard and sweep landing in the same release.

### Seed

- seeds/2026-08-09-split-oversized-go-files.md (inbox:idea)

### Imports

(none)

### Map State at Start

(n/a — single-topic work)

### Exploration

The seed opened on Portal's hub files: `internal/tui/model.go` as the TUI's everything-file, with `model_test.go`, `internal/tmux/portal_saver_test.go`, `cmd/open_test.go` and `internal/tmux/tmux_test.go` all past three thousand lines. Its argument was that the size is an active cost rather than an aesthetic complaint, evidenced by the theming-system feature: `model.go` was the single most-read artifact of that implementation at 327 reads across 85 distinct subagents, and hub-file gravity concentrates merge risk against any future parallelism. Go makes the mechanical half free — files inside a package split with no import, caller or test changes — so the work is choosing the seams, with the existing `theme_panel.go` / `theme_panel_commit.go` / `theme_row.go` siblings as the working pattern.

One of the seed's headline figures was checked against the tree at session time and has moved: `model.go` was 5,468 lines when the idea was logged on 2026-08-09 and is 3,448 now, the drop coming from the comment-strip sweep rather than any split; `model_test.go` is 7,116 against the seed's 7,766. The file is still much the largest thing in its package, and no concern split has happened — the only new `internal/tui` files since are the theming-system's own panel set. The seed's argument survives; its numbers do not.

The user's framing extended the seed in two directions. First, the goal is to align with a recognised convention rather than pick an arbitrary threshold, on the grounds that the current sizes serve neither a human nor an agent reader. Second, the outcome should not stop at a refactor: standards go into CLAUDE.md so the discipline holds for future work, with the trimming and refactoring done as far as is practical, all in the same release. The user was explicitly unsure about a size discipline as such while judging it worth having.

A correction to the premise was surfaced during shaping and accepted into the framing rather than resolved: Go has no file-size convention to align with. The standard library ships multi-thousand-line files without apology (`net/http/server.go` at roughly 3,900 lines, `go/types/expr.go` comparable). What Go does conventionalise is one concern per file within a package — a cohesion rule, not a line count. The standard this work sets will therefore be Portal's own, argued from the measured agent-read cost rather than borrowed from the language, and the honest target is files a reader can hold rather than conformance to an external number. Where exactly that lands — a hard ceiling, a soft guideline, a cohesion rule with size as a symptom — was left open for the next phase.

Shape signals converged on a single coherent piece of work: the splits are the same seam-choosing exercise repeated over a handful of files rather than independent concerns needing their own cycles, so no topic multiplication. The work-as-standard reading was considered and rejected because the standard alone would terminate at specification, and the user requires the refactor to land in the same release — which needs the work to reach implementation.

### Edits

(none)

### Topics Identified

(none)

### Conclusion

Routed to discussion.
