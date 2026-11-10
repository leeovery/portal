# Discussion: Filtering

## Context

The sessions list can be narrowed today only by the `/` text filter, which matches each session's name and its recorded folder, and it can be arranged by the `s` grouping cycle — Flat, By Project, By Tag — whose choice is remembered between launches. Discovery accepted a set of new filters for the picker: narrowing the sessions list to one or more tags with a choice between matching any of them and matching all of them; narrowing to a project; excluding a tag ("everything except archive"); keeping the group headings while filtering, since today typing a filter flattens the list; and showing the active filter as chips in the header, so it is visible what is narrowing the list. This topic settles how those filters behave and how they sit beside what exists.

`grep -n 'func (i SessionItem) FilterValue' -A2 internal/tui/session_item.go` → returns `resolver.SearchFields(i.Session.Name, i.Session.Dir)`: the session's name and its recorded folder (abbreviated under `~`), the recorded folder only when one exists (2026-10-26). *(Amended 2026-11-10 — since the move to zmx a session's name never changes and `r` sets a label instead; the filter matches the label when it differs from the name, then the name, then the recorded folder: `sed -n '/^func SearchFields/,/^}/p' internal/resolver/search.go` → label (when set and not the name), name, abbreviated dir.)*

### Inherited position from discovery

Discovery reached these with the user; they carry forward as working ground, open to movement only where this discussion surfaces something that contradicts them:

- **In scope, all accepted:** tag filters with an any/all choice, a project filter, tag exclusion, group headings kept while filtering, and the active filter shown as chips in the header.
- **A tag filter matches through projects.** Tags live on projects only, and a session carries its project's tags.
- **The picker row rework stays a separate roadmap item** — each session's folder beside its name, the right-hand status strip. Display work tied to filtering, such as the chips, belongs here.

### History in the original tagging work

The session-tagging-and-grouping specification deferred two of these explicitly:

- **Live-grouped filtering** — keeping group headings while the `/` filter is active was the user's first preference then, deferred because the list widget re-ranks matches into a relevance-sorted flat list, so keeping groups would mean Portal owning the filter outright (its own input state, matching, re-grouping per keystroke, cursor and pagination), with a large interaction matrix (an active filter plus a view switch, the preview, an external kill refreshing the list). The payoff was judged small and transient, since filtering is usually "find one session fast". Nothing in that work locked it out.
- **Tag exclusion** — deferred as a power feature, acceptable because the `/` filter covered basic narrowing; to be revisited if a concrete "tag I never want to see" pain appeared.

That specification also fixed the text filter's scope as name-based, with the tag dimension served by the By Tag view rather than by the filter. Since then the filter has come to match the recorded folder too, and group headings have become real, non-selectable rows of the list whose empty filter text makes them vanish the moment a query is typed — the mechanism by which the list flattens today.

### Ground settled by sibling discussions

From project-link:

- A session records the folder it was started in and, separately, the project it belongs to; a worktree session's project is the repository the worktree was made from. The folder keeps every job it had apart from grouping — the session row, the `/` text filter, `x /term` — and the project drives the By Project heading, the session's tags, and every tag or project filter.
- A session with no record keeps a live guess of its project from its active pane's folder *(amended 2026-11-10 — under zmx, from the git root of the session's working folder as the saver last saw it; and since zmx-era Portal records every session it mints in its own index, carried through restore, a session with no record is now only one started outside Portal: `jq '[.sessions[]|select((.name|startswith("_")|not) and ((.dir//"")==""))]|length' ~/.config/portal/state/sessions.json` → 0 of 31)*; the guess feeds headings, tags and the tag and project filters only, and is silent when wrong. Being missing from a filter was weighed as just as much a failure as being misplaced in one.
- Projects stand alone: a project filter matches the sessions of that project only, never those of projects nested inside it. A family of projects (folio with folio/api and folio/sdk) is a tag put on its members.
- A tagged project whose folder disappears is kept and marked missing; sessions recorded under it keep grouping under it and stay in its filters.
- Switching to a grouped view, or applying a tag or project filter, shows no delay however many sessions are guessed.
- Every session row offers `l`, linking the session to a project; a link applies to that one session.

From tagging:

- Tag identity ignores case everywhere a tag is compared — explicitly including tag filters — and a tag has one casing on disk.
- The sessions page's keys become: Enter, `/`, Space, `s`, `n`, `r`, `k`, `q`, `x`, `t` (tagging a session's project, replacing the theme key among the footer's core keys), `m`, `?`, plus `c` (theme) and `l` (link) in help only. Footer core keys on the sessions page today: Enter, `/`, Space, `s`, `x`, `t`, `m`, `?` (`internal/tui/keymap.go`).
- New surfaces use the existing colour tokens, keep every state glyph- or word-backed under `NO_COLOR`, and are designed in Paper, in the existing Portal design file, starting from an existing design.
- The projects page gains a Tags list with each tag's project count.

### Open questions carried in

- How the new filters sit beside the `/` text filter and the `s` grouping cycle — one more control stacking on them, or merged with them — including how a tag filter relates to the By Tag view.
- Whether a filter persists between launches, as the grouping mode does.
- Whether the projects list filters too, or only the sessions list — the user said "projects / sessions".
- Whether typing a tag name in the `/` text filter matches that tag's sessions.
- Sessions with no project drop out of any tag or project filter, so filtering is only as reliable as the session-to-project link and the separate reboot bugfix. *(Amended 2026-11-10 — the reboot link loss was a tmux-era bug: zmx-era Portal carries each session's recorded folder through restore in its session index.)*
- Keys and footer space for the filter controls.
- Filter chips add a row to the vertical space where the keymap footer already disappears on short terminals (ten rows or fewer), and the section header where chips would go overflows at very narrow widths. Both are inbox bugs discovery left out of the epic.
- Designing the chips and filter controls in the current look, including the no-colour mode.

Settled elsewhere and not reopened here: whether a project filter takes in nested projects (project-link — it does not), and the lookup cost of tag and project filtering (project-link — no delay).

### References

- [Discovery brief — Filtering](../discovery/briefs/filtering.md)
- [Discovery session 001](../discovery/sessions/session-001.md)
- [Discussion — Project Link](project-link.md)
- [Discussion — Tagging](tagging.md)
- [Specification — session-tagging-and-grouping](../../session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md) (Non-Goals; Filter Composition)

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent. The Discussion Map (which subtopics exist and their states) lives in the manifest, not this file.*

---

## Filter Model

### Context

The new tag, project and exclusion filters need a place beside the two controls the sessions list already has: the `/` text filter and the `s` grouping cycle. The worked case: an afternoon where only work should show, across 33 live sessions (`tmux list-sessions -F '#{session_name}' | grep -vc '^_portal'` → 33, 2026-10-26; *amended 2026-11-10 — a tmux command; under zmx, `jq '[.sessions[]|select(.name|startswith("_")|not)]|length' ~/.config/portal/state/sessions.json` → 31*). Today that means switching to By Tag and scrolling to the `fabric` heading, or typing `/fabric` — which catches sessions whose recorded folder sits under `~/Code/fabric`, an approximation of the tag rather than the tag. Either way, typing `/folio` to find one session then drops the headings and re-ranks the list flat.

The framing that drove the choice: two different jobs. **Search** (`/`) is "find one session now" — transient: type, pick, gone. **Scope** is "this is my working set for a while" — sticky: it should survive a search, a view switch, perhaps a relaunch.

### Options Considered

**A. Scope apart from search** — the tag/project/exclusion filter is set on its own, stays put, and shows as chips in the header; `/` searches within it and clearing a search leaves the scope alone; `s` arranges whatever is in scope.
- Pros: each control keeps one lifetime; the scope persists naturally if persistence is wanted; a command-line way to open the picker pre-narrowed has an obvious thing to set.
- Cons: a new control to learn and to find a key for.

**B. One box with a syntax** — `/` accepts tokens such as `t:fabric -t:personal folio` beside free text; chips are the parsed tokens.
- Pros: one control; fast to type.
- Cons: the scope lives and dies with the search box — clearing a search clears the scope; persisting it means opening the picker with text already in the box.

**C. Filter through the grouped views** — in By Tag, hide or isolate tag groups; in By Project, isolate a project.
- Pros: no new control.
- Cons: the filter is tied to a view — switching to Flat loses or hides it; "match all" has no group to map onto, since a session with two tags shows under both headings.

```text
 Sessions 9 — by project    fabric · −personal          / to filter
   folio ··· 3
 ▌ folio-a1b2
   folio-x9y8
   finder ··· 2
   …
```

(Illustration of A only; the chips' look and placement are Filter Chips'.)

### Journey

The deciding point was lifetime: under B a transient search wipes out a lasting choice, and under C the choice depends on which view is showing. The question put to the user was whether they reach for filtering mostly as "find one session" — which would have favoured B's single box. They agreed with A.

### Decision

**The tag and project filter is a scope, separate from the `/` search and from the `s` grouping cycle.** Three independent controls:

- **Scope** — which sessions are in the list at all: tags and projects, included or excluded (Project Filter). It stays in place until the user changes it, and is shown as chips in the header.
- **Search** (`/`) — finds sessions within the scope, as today; clearing the search returns to the scoped list, never to the unscoped one.
- **Arrangement** (`s`) — Flat, By Project or By Tag over whatever is in scope; switching views leaves the scope as it is.

How the scope is set is Setting The Filter's; how long it lives is Filter Persistence's.

Sibling check: command-line-filters (discovery brief) — whether `x --tag work` opens the picker narrowed is open there; a scope separate from the search box gives that flag something to set without pre-filling `/`, as `-f` does. No overlap.

---

## Setting The Filter

### Context

Filter Model made the scope a control of its own; this is the surface the user sets it on. The picker already has one precedent for a side panel over a live page: the theme panel is a slide-over at the right edge, a left border only, opaque over the page, with the page beside it not re-laid out, and it refuses to open below a minimum width (`internal/tui/theme_panel_render.go`, `theme_panel_geometry.go`). Tagging's tag checklist set the other precedent this surface leans on: every tick is written at once, with no save step, and Esc closes.

### Options Considered

**A filter panel, as a slide-over** — opened by a key, at the right edge like the theme panel, with the sessions list visible and updating beside it.

```text
 Sessions 9 — by project         │ Filter                   any · all
   folio ··· 3                   │ TAGS
 ▌ folio-a1b2                    │ ▌ + fabric               14
   folio-x9y8                    │     acom                  4
   finder ··· 2                  │   − personal             12
   …                             │     curated               2
                                 │ PROJECTS
                                 │     folio                 3
                                 │     portal                2
```

- Pros: every change is seen in the list as it is made; one place holds the whole scope, including any/all and exclusions.

**Gestures on the list** — a key narrows to the highlighted session's project, another hides its tag.
- Pros: fast for "just this project".
- Cons: any/all cannot be built from gestures, and each gesture needs its own undo. Could be added later as a shortcut beside the panel.

**A centred modal**, like the edit screen.
- Cons: it covers the list, so the effect of a change is invisible until it closes.

### Journey

The slide-over won on seeing the effect: the scope is a choice about what the list shows, so the list should be on screen while it is made. The theme panel already established the shape in this picker, and the tag checklist the apply-at-once rule. The user agreed with the slide-over as the main control.

### Decision

**The scope is set in a filter panel that slides in at the right edge, over the sessions list, which stays visible and updates as the scope changes.**

- **What it lists**: the tags carried by live sessions, then the projects with live sessions, each with how many live sessions it holds. A tag or project with no live session is not listed, since choosing it could only empty the list — except one already in the scope, which stays listed so it can be switched off.
- **Rows**: a tag row is off, included (`+`) or excluded (`−`). Project rows take the same three states, and picks combine as Project Filter and Any All And Exclude decide.
- **Behaviour**: each change applies to the list the moment it is made, with no save step; typing narrows the panel's rows; Esc closes the panel and leaves the scope in place.

The panel's keys are Keys And Footer's; its look, and the chips the scope shows in the header, are Filter Chips'.

Sibling check: tagging (discussion) — the tag checklist writes each tick at once and closes on Esc; the panel follows the same rule for the scope. Theming-system (specification) — the theme panel's slide-over shape is reused, not changed. No overlap.

---

## Project Filter

### Context

Raised by the background review (review-001 F7): the scope was described as "tags included or excluded, a project", and nothing said whether several projects can be picked, how a project pick combines with tag picks, whether a project can be excluded, which projects are offered, or how same-named projects are told apart — the install has three `api` projects, two `docman` and two `leeovery` (project-link, Linking From Picker). Which projects are offered was already settled by Setting The Filter: those with live sessions, plus any already in the scope.

### Options Considered

The fork was how a tag pick and a project pick combine, worked on `fabric` included under TAGS and `portal` — a personal project — picked under PROJECTS because it is the one personal thing on the go that afternoon:

**Either** — the 14 fabric sessions plus the 2 portal ones.
- Pros: the afternoon the user wanted: work, plus one named exception.

**Both** — sessions tagged `fabric` that are also in `portal`.
- Cons: none, since portal is tagged personal. In general a project either carries the tag — "both" then gives that project alone, which picking the project alone already gives — or it does not, and "both" gives nothing.

### Journey

"Both" was degenerate rather than merely weaker: a project's tags are fixed at any moment, so intersecting a project with a tag can only return the project or nothing. The remaining questions each had one defensible shape: a session belongs to exactly one project, so several picked projects can only mean any of them; excluding a project costs nothing beyond what excluding a tag costs and keeps every row in the panel behaving alike ("everything except the four flowx sessions" is one row); and the link chooser already tells same-named projects apart by folder. Proposed as one shape and accepted by the user.

### Decision

**Project picks add to tag picks.** (resolves review-001 F7)

- A project pick and a tag pick combine as "either": a session is in when it matches the tag picks or is in a picked project.
- Several projects can be picked, and a session is in when it is in any of them.
- A project row can be excluded as a tag row can; every row in the panel takes the same three states.
- Where two listed projects share a name, the panel shows each one's folder, as the link chooser does. How a chip names such a project is Filter Chips'.

How exclusions and inclusions settle against each other on one session is Any All And Exclude's.

Sibling check: project-link — Nested Projects decided a project filter matches that project's sessions alone; consistent, a picked project takes in no project nested inside it. Linking From Picker tells same-named projects apart by their folder; the panel does the same. Command-line-filters (discovery brief) — whether a project can be named on the command line the same way is open there; no overlap.

---

## Any All And Exclude

### Context

Discovery accepted narrowing to one or more tags "with a choice between matching any of them and matching all of them", and excluding a tag. Raised by the background review (review-001 F6): any/all, and some exclusion outcomes, make a difference only when a session's project carries more than one tag, and none does on the install —

`jq '[.projects[]|select((.tags//[])|length>1)]|length' ~/.config/portal/projects.json` → 0; `jq -r '.projects[].tags[]?' ~/.config/portal/projects.json | sort | uniq -c | sort -rn` → 27 personal, 22 fabric, 5 acom, 3 curated, 2 detaillab (2026-10-27)

— so "all" of two different tags would return nothing today. Two-tag projects become likely once families are tags (project-link's Nested Projects: folio, folio/api and folio/sdk carrying a `folio` tag on top of `fabric`). Open were: the concrete case "all" serves; whether any/all applies to excluded tags; which wins when one session matches both an inclusion and an exclusion; and which headings By Tag shows under a scope.

### Options Considered

**Keep "match all"** — the panel carries an any/all switch; "fabric + api" would show only fabric's API projects.
- Pros: serves tags along two independent lines — who a project is for (`fabric`, `acom`) and what kind it is (`api`, `infra`, `frontend`).
- Cons: along a single line of tags it never helps — "fabric and folio" gives exactly what `folio` alone gives, since the family sits inside the client.

**Drop "match all"** — every included tag or project widens the list; no switch.
- Pros: the panel and its chips carry no mode; the one combination with a real case today ("fabric or acom") is the only one there is.
- Cons: a second line of tags, if it ever appears, has no "both" until the switch is added.

### Journey

Three of the four questions had one defensible shape and were proposed as such: "except archive and personal" never means "only when it carries both", so exclusions hide on any excluded tag; "except" reads as the last word, so an exclusion beats an inclusion on the same session; and Filter Model made the arrangement independent of the scope, so By Tag shows each in-scope session under all its tags, as it does unscoped. A session tagged `fabric` and `folio` under a `fabric` scope therefore also appears under `folio` — every session under that heading is in scope, so the heading is accurate rather than a leak.

The live question was whether "all" earns its place. Asked whether they tag, or would, along a second line, the user saw some value in a client line crossed with a kind line (infra, backend, frontend) but judged it unlikely: their tags name clients, project families and personal work, which run along one line rather than crossing. Along one line "all" is redundant with the narrower tag. What tipped it: adding the switch later is purely additive, so dropping it now loses nothing if a second line ever appears.

### Decision

**Every inclusion widens the list; exclusions hide; exclusion has the last word. There is no "match all".** (resolves review-001 F6)

- With nothing included, the list holds every session; with anything included, it holds the sessions that carry any included tag or sit in any included project.
- An excluded tag or project hides every session carrying that tag or sitting in that project, whatever else the scope includes.
- By Tag arranges the in-scope sessions under all their tags, exactly as it does with no scope.

This moves discovery's accepted any/all choice: the "all" half is dropped because the user's tags run along a single line, and it can be added later as a switch on the panel without changing anything decided here.

Sibling check: command-line-filters (discovery brief) — whether the any/all and exclude choices carry over to the command line is open there; with "all" dropped there is only "any" to carry. The brief anticipates this decision; nothing owed. Tagging — Tag Case decided tag filters compare tags ignoring case; consistent.

---

## Sessions Without A Project

### Context

Discovery carried in that sessions with no project drop out of any tag or project filter, so filtering is only as reliable as the session-to-project link. Since then project-link decided how that link is made and repaired — sessions with no record keep a live guess of their project, `l` links any session to a project, and the reboot bugfix restores the recorded link before this epic's implementation *(amended 2026-11-10 — no bugfix is owed under zmx, which carries each session's recorded folder through restore)* — so the reliability concern itself is settled ground. Raised by the background review (review-001 F3): the carried-in wording holds only for inclusions. Under an exclusion such as "everything except personal", a session with no project and a session whose project carries no tag both stay in the list, since neither carries `personal` — which is the right reading of "except" under Any All And Exclude. What was left open was whether the user can scope to these sessions directly.

The grouped views already gather them: By Project puts sessions whose project is not a saved project under **Unknown**, and By Tag puts sessions carrying no tag — whether their project has none or they have no project at all — under **Untagged** (`sed -n '/^func resolveSessionTags/,/^}/p' internal/tui/grouping.go` → returns no tags when the session has no folder or its folder matches no saved project, 2026-10-27).

### Options Considered

**A row for each in the panel** — "no project" under PROJECTS and "no tags" under TAGS, each with its count, each included or excluded like any other row.
- Pros: these are the sessions `l` (link to a project) and `t` (tag a session's project) exist to repair; including "no project" gives the list of sessions waiting to be linked, and excluding it hides the clutter.

**No rows for them**
- Cons: finding the sessions to repair means switching to a grouped view and scrolling to its catch-all heading.

### Journey

The deciding point was that these two groups are where the repair work comes from: the link and tag actions decided by project-link and tagging act on exactly the sessions the catch-all headings hold. The user agreed.

### Decision

**The filter panel lists "no project" under PROJECTS and "no tags" under TAGS**, each with its live-session count, each taking the same three states as any other row. (resolves review-001 F3)

- "no project" holds the sessions the By Project view puts under Unknown — those whose project, recorded or guessed, is not a saved project, or who have none.
- "no tags" holds the sessions the By Tag view puts under Untagged — those carrying no tag, a session with no project included.
- Like any row, each is listed only while it holds a live session or is already in the scope (Setting The Filter).
- They combine like any other pick (Any All And Exclude): including "no project" adds those sessions; excluding "no tags" hides every untagged session.

Sibling check: project-link — Unrecorded Sessions and Linking From Picker decided the live guess and the `l` action; "no project" lists the sessions that action repairs and changes neither. Tagging — Sessions List Tagging routes any session whose project is not a saved project through the link chooser before tagging; consistent, "no project" is that same set. No overlap.

---

## What The Filter Hides

### Context

Raised by the background review (review-001 F2): the picker's screens were written for a list with no scope, and three of them mislead once one is set.

- **A search that finds nothing inside the scope.** Filtered to `fabric`, the user types `/goose` to find goosemates, tagged personal. The screen reads `No sessions match "goose"` with "⌫ to widen the search · esc to clear the filter" (`grep -n 'noMatchesHint' internal/tui/filtering_no_matches.go` → `"⌫ to widen the search · esc to clear the filter"`, 2026-10-28). Under Filter Model, Esc clears the search only and lands back on the fabric sessions — goosemates still hidden, and the "find one session fast" job fails without a word.
- **A scope that matches nothing.** The last session carrying an included tag is killed, or an exclusion removes everything. The list is empty and the empty state reads "No sessions yet" with "Press n to start one in the current directory · x for projects" (`grep -n 'emptySessions' internal/tui/empty_states.go` → `"No sessions yet"`, `"Press n to start one in the current directory · x for projects"`, 2026-10-28), while 33 sessions are running.
- **The header count.** Nothing said whether the count beside "Sessions" counts the scoped list alone or also tells the user how many are hidden — on landing back in the picker after a detach, with the scope still set, it is the first clue that something is.

### Options Considered

The rule proposed: **the list never hides sessions without saying so** — each of the three screens says the scope is in play and how to get out of it.

The alternative weighed for the no-match case: **let a search look past the scope when nothing inside it matches.**
- Cons: the user stops being able to tell what the scope is doing — the same search would return scoped results one moment and unscoped ones the next.

### Journey

The deciding point was the failure being silent: under a scope, every one of these screens would point the user the wrong way or tell them something false, while the session they wanted sat one key away. The chips say what the scope is; the count is what says how much it hides. Counting matches outside the scope on the no-match screen was preferred over a bare "searching inside the filter" note because it tells the user whether clearing the scope would help at all. The user accepted the rule.

### Decision

**A scope never hides sessions silently.** (resolves review-001 F2)

- **The header count shows what is hidden.** With a scope set, the count reads as the sessions shown out of the sessions running ("9 of 33"); with no scope, it is the plain count as today.
- **A search that finds nothing inside the scope but something outside it says so** — how many sessions outside the scope match (`No sessions match "goose" here · 1 outside the filter`, as an illustration) and the key that clears the scope. A search that matches nothing anywhere keeps today's no-match screen.
- **A scope that matches nothing has its own empty state**, saying nothing is in the filter and how to change or clear it. "No sessions yet" stays for a picker with no sessions at all.
- A search never looks past the scope on its own.

Wording and look are the Paper design's; the key that clears the scope is Keys And Footer's.

Sibling check: spectrum-tui-design (specification) — the section header carries a count, the no-match screen reads `No sessions match "<query>"` with "⌫ to widen the search · esc to clear the filter", and the empty sessions state reads "No sessions yet"; that is what ships today, so no correction is owed now — this epic's specification carries these as changes to existing behaviour under a scope. Its rule that no match-count is shown while typing a search is untouched: the outside-the-filter count appears only on the no-match screen.

---

## Scope Follows Changes

### Context

Raised by the background review (review-001 F5): a scope can name a tag that changes under it, within one sitting. Filtered to `work`, the user presses `x` for the projects page, opens the Tags list, and renames `work` to `client`, merges it into `fabric`, or deletes it — tagging decided all three (Tag Management: a rename changes the tag on every project, renaming into an existing tag merges the two, `d` removes a tag everywhere, and a tag exists only while a project carries it). Back on the sessions page, a scope still holding the old name would show a chip for a tag that no longer exists; as an inclusion it would empty the list, as an exclusion it would bring back the sessions it was hiding with nothing said. A project in the scope meets the same through project-link's re-pointing and merging (Renamed Or Moved Folders).

### Options Considered

**The scope follows the change.**
- Pros: the user's own rename, merge or delete never quietly breaks their scope.

**The scope keeps the old name.**
- Cons: a rename the user made elsewhere in Portal silently breaks the scope.

### Journey

Every one of these changes is made inside Portal by the user, so Portal knows what became of the tag and can carry the scope along. The one change it cannot see is a hand edit of the projects file; for that, Setting The Filter already keeps a pick that holds no live session listed in the panel, so it can be switched off. Proposed as one shape; the user accepted it.

### Decision

**A scope follows its tags and projects through every change Portal makes to them.** (resolves review-001 F5)

- A renamed tag stays in the scope under its new name; a tag merged into another becomes that tag in the scope; a deleted tag leaves the scope.
- A project re-pointed at a new folder stays in the scope; two projects merged become the one in the scope.
- The scope is never saved between runs (Filter Persistence), so there is no saved copy to update.
- A change Portal does not make — a hand edit of the projects file — is not followed: the stale pick stays listed in the panel with no sessions, as any pick already in the scope does, so it can be switched off.

Tag Case already makes the scope compare tags ignoring case, so the casing rewrite on write changes nothing here.

Sibling check: tagging — Tag Management's rename, merge and delete are the changes followed; consistent. Project-link — Renamed Or Moved Folders' re-pointing and merging are the project changes followed; consistent. No overlap.

---

## Filter Persistence

### Context

Discovery left open whether a filter persists between launches, as the grouping mode does. Raised alongside it by the background review (review-001 F4): a scope that persisted would meet the entry points that open the picker already searching — `x /term` searches every live session and attaches outright on exactly one match, `x -f` opens the picker pre-filtered — so the shell and the picker could disagree about what matched, and the planned `x --tag` would have to replace or stack on a saved scope.

Portal's picker now outlives a single pick. Since the move from tmux to zmx, an attach runs inside Portal: detaching, the kill key (which returns on the kill prompt) and a take all reopen the picker in the same run, which goes on until it is quit (`grep -n 'func reopenPicker' cmd/open_attach.go` → `cmd/open_attach.go:221`, its doc comment: "opens the plain picker an attach's end asked for"; 2026-11-10). The reopened picker is plain: no search text, no command.

### Options Considered

**Saved between runs** — the scope is written beside the grouping mode and every `x` opens with it.
- Cons: every fresh picker starts narrowed by a choice made in another window, hours earlier; a shell search would have to decide whether to look past it.

**Lives for the run** — the scope survives every return to the picker within one run; a new `x` starts clean.
- Pros: the working loop — filter to fabric, attach, work, detach, pick the next — keeps its scope; nothing is saved, so every new picker starts as it does today.

**Clears on every return** — each reopened picker starts unscoped.
- Cons: the scope would last one visit, the search's lifetime, and would have to be set again after every session.

### Journey

The first proposal was to save the scope, on the ground that the picker exits on every pick, so an unsaved scope would last one visit — the search's lifetime, which Filter Model had separated it from. That ground was the tmux-era picker and no longer holds: since the zmx move, detach returns to the picker in the same run. The user pushed back on saving — every picker should start clean, as now — and the corrected picture gave a middle path: the scope lives exactly as long as the run. The user agreed: having filtered, attached and then detached or killed, they expect to land back on the filter, one Esc away from clearing it as the search is today; a new picker in a new window starts with none. With nothing saved, the shell-search conflict dissolves: `x /term` and `x -f` start a fresh run, which has no scope.

### Decision

**The scope lives for the run and is never saved.** (resolves review-001 F4)

- Every `x` — a new run, a shell search included — opens with no scope, as today.
- Within a run, the scope survives every return to the picker: after a detach, after the kill key's return, after a take. The search text still clears on return, as today.
- `x /term` and `x -f` search every live session as they do today; with nothing saved there is no scope for them to look past.

The user expects one Esc to clear the scope, as one Esc clears the search today; how Esc divides between an active search and the scope is Keys And Footer's.

Sibling check: command-line-filters (discovery brief) — whether `x --tag` opens the picker narrowed, and how it sits beside `-f` and `x /term`, is open there; with no saved scope, a scope set from the command line can only be that run's. The brief anticipates this; nothing owed. Open-with-forced-filter (specification) — the search form's matching and its direct attach on one match are untouched.

---

## Summary

### Key Insights

### Open Threads

### Current State
