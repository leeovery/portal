# Plan: Session Tagging and Grouping

## Phases

### Phase 1: Tag data model & session→directory resolution
status: approved
approved_at: 2026-06-07

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

#### Tasks
status: approved
approved_at: 2026-06-07

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-tagging-and-grouping-1-1 | Add `tags []string` field to Project record | missing tags field decodes to nil/empty (no migration), existing records round-trip unchanged, null vs [] in JSON |
| session-tagging-and-grouping-1-2 | Tag value normalisation helper (trim + lower-case + reject empty) | leading/trailing whitespace trimmed, whitespace-only rejected, mixed/upper case collapses, empty string rejected, internal whitespace preserved |
| session-tagging-and-grouping-1-3 | Per-project tag set add/remove (normalised, deduped, persisted) | duplicate-after-normalisation no-op, removing absent tag no-op, blank/whitespace add rejected, project path not found |
| session-tagging-and-grouping-1-4 | Canonical directory path key for dir→project lookup | symlinked path, trailing slash, ~ home expansion, path not a known project, relative path |
| session-tagging-and-grouping-1-5 | Stamp `@portal-dir` at session creation | stamp survives rename (rides session object, not name), QuickStart exec-handoff path, SetSessionOption failure non-fatal |
| session-tagging-and-grouping-1-6 | Expose `@portal-dir` via ListSessions (Session.Dir) | empty/absent @portal-dir parses to empty Dir, format-field count change, pipe character in value |
| session-tagging-and-grouping-1-7 | Lazy active-pane → git-root directory resolution | pane with no enclosing git repo (no git-root), session killed mid-resolve, active pane only (not all panes) |
| session-tagging-and-grouping-1-8 | Best-effort lazy re-stamp of derived `@portal-dir` | SetSessionOption failure swallowed and re-attempted next render, git-root derivation failure yields no stamp, derived value used for current render regardless of write outcome |

### Phase 2: Grouped render — By Project & By Tag
status: approved
approved_at: 2026-06-07

**Goal**: Render the live session list in grouped form using the render-layer item model: every `bubbles/list` item remains a session instance, group headings are injected at group-key boundaries as visual separators, the By-Tag mode materialises a multi-tag session as one `(session, tag)` item per tag, and unresolved/untagged sessions collect in the pinned **Unknown** (By Project) / **Untagged** (By Tag) buckets.

**Why this order**: Builds directly on Phase 1's resolution and tag data. It delivers the core user-visible payoff — **By Project** is valuable with zero setup — and proves the render-layer approach (headers never as list items) before the toggle and persistence are layered on. Grouping is exercised here against the renderers directly; the `s` cycle that selects between them arrives in Phase 3.

**Acceptance**:
- [ ] By Project renders one item per session under its project `name` heading, items pre-sorted by heading then session name, headings alphabetical
- [ ] By Tag renders one item per `(session, tag)` pair under each tag heading the session's directory carries; a zero-tag session contributes one item in **Untagged**
- [ ] Two distinct directories sharing a `name` form two separate By-Project groups (key is canonical path, not name); `work` / `Work` / `WORK` collapse into a single By-Tag heading
- [ ] Group headers are dimmed, non-selectable, and carry a count of the rows rendered beneath them; the cursor, initial position, and `g`/`G` land only on session instances, never a header
- [ ] Catch-all buckets (**Unknown** / **Untagged**) are pinned last, carry a count, and are suppressed when their membership is zero; an unresolvable or deleted-project session falls to Unknown (By Project) / Untagged (By Tag) and is never dropped
- [ ] Selecting any instance of a session attaches the same underlying session (duplicate By-Tag instances are views of one session); the picker is not routed through `lipgloss/tree`

#### Tasks
status: approved
approved_at: 2026-06-07

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-tagging-and-grouping-2-1 | Extend SessionItem with group metadata (group key, heading label, optional tag) | zero-value flat item carries no group metadata, FilterValue still returns session name, multiple instances share one underlying Session |
| session-tagging-and-grouping-2-2 | By Project grouping builder (session → dir key → project name heading, pre-sorted) | two distinct dirs sharing a project name form two groups (key is canonical path not name), Session.Dir empty, stamped dir with no matching project record, zero live sessions |
| session-tagging-and-grouping-2-3 | By Tag grouping builder (one item per (session, tag), pre-sorted) | multi-tag session yields N items, work/Work/WORK collapse to one heading, zero-tag session yields exactly one item, session whose dir has no project record |
| session-tagging-and-grouping-2-4 | Pinned catch-all buckets (Unknown / Untagged) with empty-suppression | empty bucket suppressed, unresolvable dir (no git-root) → Unknown, deleted-project session → Unknown (By Project) / Untagged (By Tag), bucket pinned last after alphabetical headings, session never dropped |
| session-tagging-and-grouping-2-5 | Header-injecting counted delegate render (dimmed heading + row count at group boundary) | count reflects rows beneath heading (By-Tag sum exceeds live session count), leading header before first item, flat items inject no header, last group's count |
| session-tagging-and-grouping-2-6 | Cursor, initial position and g/G land only on session instances; any instance attaches same session | initial cursor on first session instance, g/G land on session not header, duplicate By-Tag instances resolve to one session, selectedSessionItem returns underlying session |

### Phase 3: Mode toggle, persistence & empty/filter states
status: approved
approved_at: 2026-06-07

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

#### Tasks
status: approved
approved_at: 2026-06-07

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-tagging-and-grouping-3-1 | prefs.json store: read/write session_list_mode with tolerant decode | missing file → Flat, empty file → Flat, corrupt/unparseable JSON → Flat, unrecognised mode value → Flat, valid by-tag/by-project round-trip, AtomicWrite temp+rename |
| session-tagging-and-grouping-3-2 | Resolve prefs.json via configFilePath + migrateConfigFile | per-file env-var override wins, XDG_CONFIG_HOME path, ~/.config fallback, migrate from old macOS path only when new absent, never overwrite existing |
| session-tagging-and-grouping-3-3 | Mode-aware session re-render core on the model (dispatches to Phase 2 builders) | zero live sessions per mode, mode-unchanged idempotent, correct builder per mode, SessionsMsg refresh preserves active mode |
| session-tagging-and-grouping-3-4 | `s` cycle key handler (Flat → By Project → By Tag → Flat) | cycle wraps By Tag → Flat, unconditional on zero sessions, unconditional on zero tags, `s` literal while filter focused, persist once per press, persist failure non-fatal |
| session-tagging-and-grouping-3-5 | Mode-aware title via SessionListTitle() | inside-tmux current-session title interaction, updates on mode change, updates on SessionsMsg refresh, Flat title unchanged from baseline |
| session-tagging-and-grouping-3-6 | Footer `s switch view` hint on sessions page | absent on projects page, footer column layout unbroken, present at all session counts |
| session-tagging-and-grouping-3-7 | By-Tag zero-tags "No tags yet" signpost | zero tags anywhere → signpost, degrade-with-message not silent flatten, reopen persisted by-tag with zero tags shows signpost, one `s` advances to Flat, tags-exist-all-sessions-tagged does not trigger signpost |
| session-tagging-and-grouping-3-8 | Flatten-on-filter and restore-grouping-on-clear | filter active flattens, headers absent while filtering, clear restores grouping, Flat-mode filter unchanged, re-group respects current mode on clear, FilterApplied vs Filtering transitions |
| session-tagging-and-grouping-3-9 | Wire prefs-backed initial mode + persister into TUI construction (open.go Option) | first-ever launch opens Flat, persisted by-tag opens By Tag, corrupt prefs opens Flat, persister writes on toggle end-to-end, nil persister tolerated in tests |

### Phase 4: Tag management in the projects edit modal
status: approved
approved_at: 2026-06-07

**Goal**: Add a **Tags** field to the existing projects edit modal that behaves exactly like the alias field (type + Enter to add, highlight + `x` to remove), extend the modal's Tab handler from a binary toggle to a three-way Name → Aliases → Tags cycle, and dispatch a sessions-list re-group refresh on the projects-edit → sessions-page transition so edits are visible on return.

**Why this order**: Last because the By-Tag grouping built in Phases 2–3 can be validated against seeded `projects.json` records without an editing UI. This phase makes tags assignable end-to-end through the only v1 surface (projects page, TUI only), completing the feature. It extends the existing modal handler (`model.go:1391-1449`) and mirrors the existing preview-dismiss refresh contract.

**Acceptance**:
- [ ] A Tags field is added to the projects edit modal, placed visually after Aliases (last); Tab cycles Name → Aliases → Tags → wrap to Name
- [ ] With the Tags field focused and non-empty input, Enter adds the normalised tag (field-scoped add, not modal confirm); modal confirm/save is unchanged for existing fields
- [ ] Highlighting a tag entry and pressing `x` removes it; pressing Enter on blank/whitespace-only input adds nothing; adding a tag the project already carries (post-normalisation) is a no-op
- [ ] An empty tags field shows a clear "no tags" empty state rather than a blank
- [ ] The projects-edit → sessions-page transition dispatches a sessions-list refresh that re-reads project records and re-groups, so an added/removed tag is reflected on the next render
- [ ] Tags are editable from the projects page only (never a session row) and there is no `portal tags …` CLI

#### Tasks
status: draft

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-tagging-and-grouping-4-1 | Add Tags field modal state + load editProject.Tags on modal open | project with nil/empty tags, existing tags preserved on open, modal re-open resets prior tag buffer |
| session-tagging-and-grouping-4-2 | Three-way Tab field cycle (Name → Aliases → Tags → wrap to Name) | wrap Tags → Name, focus order Name→Aliases→Tags, tag cursor initialised on entering Tags field |
| session-tagging-and-grouping-4-3 | Add-tag-on-Enter (field-scoped add, blank/dup no-op) | blank/whitespace-only Enter no-op, "  Work " stored as "work", duplicate-after-normalisation no-op, Enter while Name/Aliases focused still confirms, Tags-focused empty-input Enter is no-op not confirm |
| session-tagging-and-grouping-4-4 | Remove-highlighted-tag via `x` + Tags field text/Backspace/Up/Down keys | `x` on Add input is not a removal, cursor clamp after removing last entry, Backspace only affects new-tag input, Up/Down bounded to entries+Add row |
| session-tagging-and-grouping-4-5 | Persist tag additions/removals on confirm via ProjectEditor AddTag/RemoveTag seam | persist failure sets editError and aborts close, no-op when tags unchanged, additions and removals in one confirm, normalisation/dedup owned by Phase 1 store |
| session-tagging-and-grouping-4-6 | Render Tags block after Aliases with "no tags" empty state | empty tags shows clear empty state not blank, focus indicator only on focused field, highlighted-entry marker, Add-input row always rendered |
| session-tagging-and-grouping-4-7 | Dispatch re-group sessions refresh on projects-edit → sessions-page transition | refresh respects active grouping mode, nil SessionLister tolerated, no refresh when not transitioning, command-pending guard preserved |
