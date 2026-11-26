## Discovery Session 001

Date: 2026-07-27
Work unit: projects-filter-input-reskin

### Description (as of session)

Reskin the Projects page live filter input to Modern Vivid (orange prompt/query/cursor), mirroring the Sessions filter-input restyle.

### Seed

- seeds/2026-06-25-projects-filter-input-reskin.md (inbox:quickfix)

### Imports

(none)

### Map State at Start

(n/a — single-topic work)

### Exploration

Surfaced from the inbox during a triage pass over stale/ambiguous items. Before selecting it we confirmed provenance: the item belongs to the *current* skin, not a superseded one — "spectrum-tui-design" (the completed feature) and "Modern Vivid" (the design it delivered) are the same reskin, so this is a genuine unfinished edge of the live design rather than obsolete polish.

The defect: the Projects page's live `/` filter input never received the Modern Vivid reskin. Pressing `/` on Projects shows the default `bubbles/list` `Filter:` prompt and query/cursor colours, whereas the Sessions filter carries the §7 MV treatment — accent.orange `/ ` prompt, orange query text, orange block cursor. Root cause confirmed while shaping: `styleFilterInput()` in `internal/tui/model.go` (~line 1398) only restyles `m.sessionList.FilterInput` and never touches `m.projectList.FilterInput`.

Scope was pinned to the input-active state only: mirror the existing session-list restyle onto the project list's `FilterInput` — set the prompt to `filterPromptPrefix`, set `Focused.Prompt`/`Focused.Text` foreground to accent.orange, set `Cursor.Color` to orange with blink off — covering both the coloured and NO_COLOR branches exactly as the session list does. The rest of the Projects filter path is already reskinned (locked-query header via `renderFilterQueryHeader`, contextual footers via `renderFilteringFooter` / `renderProjectsFilterAppliedFooter`) and is out of scope. No test currently asserts the project filter input prompt, so coverage for the restyle would be net-new.

The user confirmed nothing else rides along. This is a small, known, mechanical styling change with no behaviour to debate and nothing to diagnose — a clean quick-fix, routing to scoping.

### Edits

(none)

### Topics Identified

(none)

### Conclusion

Routed to scoping.
