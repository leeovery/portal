# Plan: Session Tagging and Grouping

## Phases

### Phase 1: Tag data model & session→directory resolution
status: draft

**Goal**: Establish the feature's foundation — add a normalised `tags []string` field to the `Project` record, and build the `@portal-dir` session→directory resolution mechanism (creation-time stamp plus lazy stamp-on-render fallback) that maps each live session back to its directory and thus its tags.

**Why this order**: Nothing can be grouped until a live session resolves to a directory and that directory's tags can be read. This is the single piece of genuinely new mechanism the feature needs; every rendering phase consumes it. It integrates with the existing `internal/project` store (reusing `AtomicWrite` / `configFilePath`) and the `session.PrepareSession` creation pipeline, verifying existing behaviour is unaffected (missing `tags` decodes to empty — no migration).

**Acceptance**:
- [ ] The `Project` record carries a `tags []string` field; a `projects.json` lacking the field decodes to nil/empty (the zero-tag state) with no migration step
- [ ] Tag value handling enforces the complete v1 rule set: leading/trailing whitespace trimmed, canonical form lower-cased, empty/whitespace-only rejected as a no-op, per-project deduped as a set
- [ ] The same canonical form (trim + lower-case) is applied wherever a tag is compared (per-project dedup, cross-project union, grouping key)
- [ ] `@portal-dir = <resolvedDir>` is stamped via the session user-option at creation using the git-root already computed in `PrepareSession`; the stamp survives rename and pane `cd`
- [ ] A live session with no `@portal-dir` is resolved from its active pane's `current_path` → git-root and re-stamped best-effort; a stamp-write failure never drops the session (the derived value is used for the current render)
- [ ] A pane with no enclosing git repository (no derivable git-root) yields no stamp and is re-attempted each render
- [ ] The render-time lookup key (stamped value and fallback-derived git-root) matches stored `Project.Path` exactly, normalised for symlinks, trailing slash, and `~` expansion

### Phase 2: Grouped render — By Project & By Tag
status: draft

**Goal**: Render the live session list in grouped form using the render-layer item model: every `bubbles/list` item remains a session instance, group headings are injected at group-key boundaries as visual separators, the By-Tag mode materialises a multi-tag session as one `(session, tag)` item per tag, and unresolved/untagged sessions collect in the pinned **Unknown** (By Project) / **Untagged** (By Tag) buckets.

**Why this order**: Builds directly on Phase 1's resolution and tag data. It delivers the core user-visible payoff — **By Project** is valuable with zero setup — and proves the render-layer approach (headers never as list items) before the toggle and persistence are layered on. Grouping is exercised here against the renderers directly; the `s` cycle that selects between them arrives in Phase 3.

**Acceptance**:
- [ ] By Project renders one item per session under its project `name` heading, items pre-sorted by heading then session name, headings alphabetical
- [ ] By Tag renders one item per `(session, tag)` pair under each tag heading the session's directory carries; a zero-tag session contributes one item in **Untagged**
- [ ] Two distinct directories sharing a `name` form two separate By-Project groups (key is canonical path, not name); `work` / `Work` / `WORK` collapse into a single By-Tag heading
- [ ] Group headers are dimmed, non-selectable, and carry a count of the rows rendered beneath them; the cursor, initial position, and `g`/`G` land only on session instances, never a header
- [ ] Catch-all buckets (**Unknown** / **Untagged**) are pinned last, carry a count, and are suppressed when their membership is zero; an unresolvable or deleted-project session falls to Unknown (By Project) / Untagged (By Tag) and is never dropped
- [ ] Selecting any instance of a session attaches the same underlying session (duplicate By-Tag instances are views of one session); the picker is not routed through `lipgloss/tree`

### Phase 3: Mode toggle, persistence & empty/filter states
status: draft

**Goal**: Wire the `s` key as a single unconditional cycle (Flat → By Project → By Tag → Flat), persist the last-used mode in a new `prefs.json`, surface the mode in the title and the `s switch view` footer hint, and handle the degenerate states: the By-Tag "No tags yet" signpost and flatten-on-filter.

**Why this order**: The grouped renderers from Phase 2 must exist before there is anything to cycle between. This phase completes the interactive shell — turning two renderers and a flat list into a togglable, persistent, filter-aware view.

**Acceptance**:
- [ ] `s` in browse mode cycles Flat → By Project → By Tag → Flat unconditionally (any session count including zero, any tag count) and writes the new mode to `prefs.json` on each press
- [ ] The title reflects the mode (`Sessions` / `Sessions — by project` / `Sessions — by tag`) via `SessionListTitle()`; the footer adds only the `s switch view` hint
- [ ] While the `/` filter input is focused, `s` is a literal filter character and does not cycle the mode
- [ ] `prefs.json` resolves through `configFilePath` (env override → XDG → `~/.config`), participates in `migrateConfigFile`, and persists `{"session_list_mode": "flat"|"by-project"|"by-tag"}` via `AtomicWrite`
- [ ] First-ever launch (no `prefs.json`) opens in Flat; after toggling to By Tag and reopening, it opens in By Tag; a missing/empty/corrupt/unrecognised file falls back to Flat with no hard error
- [ ] By Tag with zero tags anywhere renders the plain session list with an explicit "No tags yet" signpost (not a silent flatten), and the cycle still lands on By Tag
- [ ] An active filter flattens the grouped view to matching sessions (headers step aside, filtering behaviour otherwise unchanged); clearing the filter restores the grouped view

### Phase 4: Tag management in the projects edit modal
status: draft

**Goal**: Add a **Tags** field to the existing projects edit modal that behaves exactly like the alias field (type + Enter to add, highlight + `x` to remove), extend the modal's Tab handler from a binary toggle to a three-way Name → Aliases → Tags cycle, and dispatch a sessions-list re-group refresh on the projects-edit → sessions-page transition so edits are visible on return.

**Why this order**: Last because the By-Tag grouping built in Phases 2–3 can be validated against seeded `projects.json` records without an editing UI. This phase makes tags assignable end-to-end through the only v1 surface (projects page, TUI only), completing the feature. It extends the existing modal handler (`model.go:1391-1449`) and mirrors the existing preview-dismiss refresh contract.

**Acceptance**:
- [ ] A Tags field is added to the projects edit modal, placed visually after Aliases (last); Tab cycles Name → Aliases → Tags → wrap to Name
- [ ] With the Tags field focused and non-empty input, Enter adds the normalised tag (field-scoped add, not modal confirm); modal confirm/save is unchanged for existing fields
- [ ] Highlighting a tag entry and pressing `x` removes it; pressing Enter on blank/whitespace-only input adds nothing; adding a tag the project already carries (post-normalisation) is a no-op
- [ ] An empty tags field shows a clear "no tags" empty state rather than a blank
- [ ] The projects-edit → sessions-page transition dispatches a sessions-list refresh that re-reads project records and re-groups, so an added/removed tag is reflected on the next render
- [ ] Tags are editable from the projects page only (never a session row) and there is no `portal tags …` CLI
