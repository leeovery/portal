# Discussion: Tagging

## Context

Tags in Portal live on projects (a saved folder), and every session is tagged through the project it belongs to. Today the only way to put a tag on a project is the projects page's edit screen: open the projects page, pick the project, open edit, move to the TAGS field and type the tag in full as a chip. Nothing suggests the tags already in use, nothing tags several projects at once, there is no place that shows every tag or how many projects carry it, and nothing renames or deletes a tag across projects. A session's project cannot be tagged from the sessions list. Once this epic makes tags the basis of filtering, the cost of getting tags onto projects — and keeping them consistent — matters more than it did while tags only fed the By Tag grouping view.

Measured on the live install 2026-10-26 (read-only): 66 saved projects, 5 tags in use, no project carrying more than one tag, 7 projects untagged.

`jq -r '.projects[].tags[]?' ~/.config/portal/projects.json | sort | uniq -c | sort -rn` → 27 personal, 22 fabric, 5 acom, 3 curated, 2 detaillab

`jq '[.projects[]|select((.tags//[])|length>1)]|length' ~/.config/portal/projects.json` → 0

`jq -r '.projects[]|select((.tags//[])|length==0)|.path' ~/.config/portal/projects.json` → 7 rows: `~/Code/switchboard`, `~/Code/acom/mach2`, `~/code`, `~/.config/kit`, `~/Code/packages`, `~/Code/kit`, `~/Code/terrace`

### Inherited position from discovery

Discovery reached these with the user; they carry forward as working ground, open to movement only where this discussion surfaces something that contradicts them:

- **Tags stay on projects only.** No per-session tags: sessions are ephemeral — one can last a minute or a month — so tagging them is not useful. Every session is tagged indirectly through its project.
- **In scope, all accepted by the user:** suggestions while typing a tag, drawn from the tags already in use across all projects and narrowing as you type; bulk tagging — mark several projects and add or remove a tag on all of them at once; tag management in one place — rename or delete a tag everywhere, and see how many projects carry each; tagging a session's project from the sessions list, without going to the projects edit screen.
- **Tags are case-sensitive by deliberate choice** — "Work" and "work" are two tags (`project.NormaliseTag` trims whitespace and preserves case).

### Rejected paths from discovery

- Per-session tags layered on project tags (a session's tags being its project's plus its own), proposed to tag what a session is for and to tag sessions whose folder is not a project. Rejected on the ephemerality argument; it would also have widened the reboot bugfix to restore session-level tags.

### Ground settled by the project-link discussion

- A session records its project apart from the folder it started in; a worktree session's project is the repository the worktree was made from. Sessions with no record keep a live guess of their project, which is grouping-and-filtering only and silent when wrong.
- Every session row offers an action that links the session to a project: a chooser that is the projects list as the projects page draws it, narrowed by typing, with the session's current project marked (labelled as a guess when it is one), and "save `<folder>` as a project" at the top when the session's project is not a saved project. A link applies to the one session it is made on. Project-link left the key this action takes, and how sessions-list tagging invokes it, to this topic.
- Projects stand alone: no project takes in the projects nested inside it, and a family of projects (folio with folio/api and folio/sdk) is expressed by tagging its members. Project-link anticipated this topic's inheritance question as what would make a family tag cost nothing to maintain.
- Tagged projects are never removed by housekeeping when their folder disappears; they are kept, marked missing, and re-pointed or deleted by the user. Re-pointing a project at a folder that is already a project merges the two with their tags combined, as does two spellings of one folder turning out to be one project.

### Open questions carried in

- Where bulk tagging happens. Marking several items exists only on the sessions list today (`m`, multi-select), where it opens several sessions at once.
- Whether a new project inherits the tags of the project whose folder encloses it. Measured 2026-10-26 by nearest enclosing saved project (folder compared without case): 36 projects sit directly under a tagged project, and 31 carry exactly its tags; the five that do not are `~/.config/kit` and `~/code` (untagged under the home-folder project, tagged personal), `acom/mach2` (untagged under acom), and `fabric/flowx/flowx-api` and `fabric/packages/laravel-workflow` (personal under fabric). Four untagged top-level projects (`~/Code/kit`, `packages`, `switchboard`, `terrace`) sit directly under the untagged `~/code` project, which itself sits under the home folder.

  `python3 -I -c "import json,os; ps={p['path']:p for p in json.load(open(os.path.expanduser('~/.config/portal/projects.json')))['projects']}; enc=lambda q: max((k for k in ps if k!=q and (q.lower()+'/').startswith(k.lower()+'/')), key=len, default=None); r=[(q,e) for q in ps for e in [enc(q)] if e and ps[e].get('tags')]; print(len(r), sum(sorted(ps[q].get('tags') or [])==sorted(ps[e]['tags']) for q,e in r))"` → `36 31`
- Tag case once suggestions and merging put near-duplicates side by side, and renaming a tag into one that already exists — a merge that has to be handled.
- Which key tagging takes: `t` opens the theme panel on both pages. The sessions page already uses Enter, Space, `/`, `s`, `n`, `r`, `k`, `q`, `x`, `t`, `m` and `?`; the projects page Enter, `x`, `e`, `/`, `t`, `d`, `n`, `q`, Esc and `?`. New actions compete for footer space. The link action's key is this topic's too.
- Tagging an Unknown session's project from the sessions list has nowhere to put the tag until the session is linked to a project; the link action now exists (project-link), and this topic decides how tagging invokes it.
- Each new surface needs designing in the current look, including the no-colour mode: the suggestion list, tag management, bulk tagging.
- The narrow-width overflow bug (inbox) touches the edit screen where tag suggestions would appear; discovery left it in the inbox, not in the epic.

### References

- [Discovery brief — Tagging](../discovery/briefs/tagging.md)
- [Discovery session 001](../discovery/sessions/session-001.md)
- [Discussion — Project Link](project-link.md)

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent. The Discussion Map (which subtopics exist and their states) lives in the manifest, not this file.*

---

## Tag Case

### Context

Today a tag is stored and shown exactly as typed, and two tags differing only in case are two tags: `Work` and `work` give two By Tag headings and two separate sets of projects (`project.NormaliseTag` trims and preserves case; every comparison goes through it). Suggestions put such near-duplicates side by side, tag management would show them as two rows with separate counts, a tag filter on one misses projects carrying the other, and renaming a tag into one that already exists has to be handled as a merge. How tag identity treats case decides all of these.

The original tagging work (session-tagging-and-grouping specification) made the canonical form lower-cased: `Work`, `WORK` and `work` were one tag, stored and shown as `work`. A later fix changed it — commit `e0e41a13d` (2026-06-08), "preserve tag casing": `NormaliseTag` now trims only, "so a tag is stored and displayed exactly as typed ("Personal" no longer renders "personal"). Matching/dedup/grouping are consequently case-sensitive". The aim of that change was display casing; case-sensitive identity followed from it rather than being chosen for itself. Discovery recorded the current behaviour as case-sensitive "by deliberate choice".

On the live install no tag carries a capital and no two tags differ only by case (2026-10-26):

`jq -r '.projects[].tags[]?' ~/.config/portal/projects.json | sort -u | awk '{print tolower($0)}' | sort | uniq -d | wc -l` → 0

`jq -r '.projects[].tags[]?' ~/.config/portal/projects.json | sort -u | grep -c '[A-Z]'` → 0

### Options Considered

**Case-sensitive (today)** — `Work` typed where `work` exists becomes a second tag.
- Cons: two By Tag headings, a filter on one misses the other, suggestions and tag management show both; the repair is a rename that merges.

**One tag whatever the case, keeping the casing it was first written with** — typing `Work` matches the existing `work`; the suggestion highlights it and adding it adds `work`.
- Pros: keeps what the June fix was for (the user's casing is shown as written) and removes its side effect; two tags differing only in case cannot arise.
- Cons: a tag's casing is set by whichever project used it first; changing it is a deliberate rename.

**Lower-case everything** (the original rule) — not reopened: the June fix rejected it for erasing the user's casing.

### Journey

The history reframed discovery's open question: the deliberate choice was display casing, not case-sensitive identity. Suggestions make the question sharper than it is today — once tags are picked from a list, a stray capital is the main way a duplicate gets made, and under case-sensitive identity it would silently split a tag in two (two headings, a filter that misses half the projects). Nothing on the install needs merging at ship. The test that would have kept case-sensitivity — a real case where `API` and `api` should mean different things — has no instance.

### Decision

#### 2026-10-26 — revised
*Trigger: review finding (review-001 F5) — the tie-break in the entry below rests on a project's last-used time, which moves every time a session is created in it, and a majority can flip when a tag is removed elsewhere; either way the displayed casing could change with no rename.*

The casing is settled once and written, not worked out on every read. Whenever Portal writes the projects file — a merge, a rename, a tag added or removed anywhere — any tag carried in more than one casing is rewritten to its winning casing on every project that carries it: the casing carried by more projects, a tie going to the casing that sorts first. From then on the tag has one spelling on disk, so its heading, chips and suggestions never change casing on their own, and project-link's merges never leave two casings behind. Between a hand edit that introduces a second casing and the next write, the display uses the same rule. The last-used tie-break in the entry below is replaced. Accepted cost: Portal rewrites casing a hand edit put on projects the user did not touch — two casings of one tag is never a state the user chose.

#### 2026-10-26 — revised
*Trigger: settled call — case-insensitive identity left unsaid which casing shows when two casings of one tag are already on disk.*

**Settled by derivation** — not discussed. Determined by the case-insensitive identity below and the paths that can still put two casings on disk: a hand edit of `projects.json`, and project-link's merges (re-pointing a project at a folder that is already a project, and two spellings of one folder becoming one project), which combine two projects' tags.

The entry below stands. Where two casings of one tag are already on disk — `Work` on one project, `work` on another — they are one tag, shown in the casing carried by more projects; a tie keeps the casing of the most recently used project. Any project that gains the tag afterwards takes that casing.

#### 2026-10-26

**One tag whatever the case, shown in the casing it was first written with.** Tag identity ignores case everywhere a tag is compared — adding a tag to a project, suggestions, grouping, tag management, and tag filters. Typing a tag that matches an existing one in another case adds the existing tag, in its existing casing. Changing a tag's casing everywhere (`work` → `Work`) is an ordinary rename in tag management, never a merge. The user's casing is still shown as written — the June fix's aim stands; only its side effect is dropped.

Sibling check: filtering, command-line-filters (discovery briefs) — neither says how a tag filter or `x --tag` treats case; both compare tags, so they inherit this identity. No overlap.

---

## Tag Suggestions

### Context

Adding a tag today means typing it in full into the edit screen's TAGS field (Enter on the `+ add` slot opens a text box; Enter commits it as a chip; Esc discards). Nothing shows which tags already exist, so reusing a tag depends on remembering its exact spelling — and a misremembered spelling silently makes a new tag. Discovery accepted suggestions drawn from the tags already in use across all projects, narrowing as you type. While a tag is being typed, ↑ and ↓ do nothing in that box today (they move between fields only when no text box is open), so they are free to move through a suggestion list.

### Options Considered

The list's content and behaviour were proposed as one shape and stood: it opens as soon as a tag is being typed — before any text — showing tags in use on other projects that this project does not already carry, each with how many projects carry it, most-used first; typing narrows it, matching anywhere in the tag and ignoring case (Tag Case); ↑↓ move through it; it serves editing an existing chip as well as adding one, so replacing one tag with another on a project is a pick, not a retype.

The fork was what Enter does with `fab` typed while `fabric` is listed:

**Enter takes the highlighted suggestion** — the best match is highlighted from the start; creating a new tag means moving to a `+ new tag "fab"` row, present only when the typed text is not already a tag.
- Pros: the easy keystroke reuses a tag; a near-duplicate takes a deliberate move.
- Cons: one extra ↓ to create a tag that happens to be a prefix of an existing one.

**Enter adds what was typed** — nothing is highlighted until ↓.
- Pros: creating a tag is a keystroke shorter.
- Cons: `fab` + Enter creates `fab` beside `fabric` — the duplicate trap Tag Case closed for capitals, reopened for prefixes.

### Journey

Reuse is the common case by a wide margin — five tags on the install after months of use — so the default keystroke belongs to it. The user then pushed back on the list showing every tag: with a hundred tags it would run off the bottom of the screen. The list is a scrollable dropdown of bounded height that fits the space available, scrolling for the rest; its look is to be spiked in Paper, in the existing Portal design file, starting from one of the existing designs.

The background review (review-001 F1) found two cases where "the best match is highlighted from the start" writes a tag the user never chose. On a blank box every tag matches, so the most-used tag (`personal`) would sit highlighted and a bare Enter — today a no-op that adds nothing — would add it. A chip edit opens with the chip's text in the box, and the chip's own tag, being on the project, would be left out of the list: editing `api` with `api-docs` on another project would highlight `api-docs`, and Enter with nothing changed would swap the chip. Today, clearing a chip's text and pressing Enter deletes the chip. The rule adopted: Enter on a box the user has not typed into never writes a tag they did not pick. The alternative — keeping the instant pick on a blank box — saves one ↓ when browsing and would need a different key than Enter for closing without adding.

The review (review-001 F3) also found the list blind to tags the project already carries: typing `api` on a project carrying `api` shows no row for it and no create row, so Enter adds whatever else contains the text (`api-docs`) or, with no other match, does nothing unexplained; today the duplicate is dropped silently. A chip edit that changes only the casing (`work` → `Work`) still matches `work` under Tag Case, so the capital silently does not take. The answer generalises the chip-edit rule just adopted: a carried tag matching the typed text heads the list, marked as already on the project. Offering a Tags-list rename from a case-only chip edit was weighed and declined — a keystroke on one project silently recasing the tag on every project that carries it is the bigger surprise.

The review (review-001 F2) noted "best match" was undefined: typing `a` matches all five tags, and most-used order puts `personal` (27 projects) on top while `acom`, the only tag starting with `a`, sits fourth — so `a` + Enter would add `personal`. The `fab`/`fabric` example had a single match and settled nothing. People type the start of the word they mean; ranking by use alone turns a short prefix into a guess at the user's favourite tag.

The review (review-001 F4) raised two edges of the bounded, scrolling list. With a short new tag that appears inside many existing ones (`e2e` among tags containing `e`), a create row placed last would scroll out of sight below the matches, so creating would look impossible. And TAGS is the edit screen's last field: directly below the box are only the modal's footer and frame, so a dropdown fitted into the remaining space could get a row or none on a short terminal. Both were settled as product rules rather than left to the Paper spike, which still owns the look: the way to create a tag is never hidden, and the dropdown never collapses to nothing. Pinning costs one row of list height whenever the create row shows.

### Decision

**Suggestions open with the tag box, and Enter takes the highlighted suggestion.** While a tag is being typed — added or edited — a dropdown below the box lists the tags in use on other projects that this project does not carry, each with its project count, most-used first, narrowed as the user types by a case-insensitive match anywhere in the tag. Once there is typed text, the best match is highlighted and Enter adds it; ↑↓ move the highlight. On a blank box the list shows with nothing highlighted: Enter adds nothing and closes the box, as today, and ↓ steps into the list. Editing an existing chip opens the box with the chip's text, as today, and the chip's own tag heads the list, highlighted and marked as the current value, so Enter with nothing changed leaves the chip as it was; clearing the text and pressing Enter deletes the chip, as today. (resolves review-001 F1) The same rule covers any typed text that matches, ignoring case, a tag this project already carries: that tag heads the list in its stored casing, highlighted and marked as already on the project, with other matches below it; Enter on it adds nothing and closes the box. A casing change typed into one chip therefore shows the tag's real casing and changes nothing; changing a tag's casing is a rename in the Tags list (Tag Management), because it changes the tag on every project. (resolves review-001 F3) With typed text, tags that start with it rank above tags that only contain it, most-used first within each group, and the top row is the best match: `a` highlights `acom`, `per` highlights `personal`, and `ail` still lists `detaillab`. (resolves review-001 F2) When the typed text is not already a tag, a `+ new tag "…"` row closes the list and is the way to create one; when nothing matches, it is the only row and is highlighted, so Enter creates the tag. The dropdown has a bounded height that fits the space available and scrolls beyond it — it never runs off the screen however many tags exist. The `+ new tag "…"` row is pinned at the dropdown's foot, outside the scrolling part, so it is visible whenever it exists; only the matches scroll. The dropdown floats over whatever lies below the box — the modal's footer, the page behind — rather than being fitted inside the modal; when the screen below the box is too short for a few rows plus the create row, it opens above the box instead. (resolves review-001 F4)

---

## Tag Management

### Context

Tags have no home of their own: a tag exists only as the union of what projects carry (no registry — the session-tagging-and-grouping design), and the only way to see which projects carry a tag is the By Tag view of the sessions list, which shows only projects with a live session. Renaming a tag means editing every project that carries it one by one; deleting a tag likewise. Discovery accepted tag management in one place: rename or delete a tag everywhere, and see how many projects carry each. Tag Case already settled that renaming into an existing tag (ignoring case) is a merge, and that a case-only rename is a rename.

### Options Considered

Proposed as one surface with bulk tagging (see Bulk Tagging), and it stood: a **Tags list** opened by a key on the projects page — one row per tag with its project count; `r` renames a tag everywhere, a rename into an existing tag being a merge confirmed with the counts; `d` deletes a tag from every project, confirmed with the count; Enter opens the tag's project checklist (Bulk Tagging); a `+ new tag` row starts a new tag.

```text
Tags                          acom — 5 projects
  personal   27                 [x] acom
  fabric     22                 [x] acom/mach2/api
▸ acom        5     ⏎ ──▶       [ ] acom/mach2
  curated     3                 [ ] switchboard
  detaillab   2                 …
  + new tag
```

### Journey

The projects page is the natural parent: tags live on projects, and the page already owns the edit screen where single-project tagging happens. Management and bulk tagging collapsed into one surface once membership was seen as the thing both act on — the tag's row says how many, its checklist says which, and changing either is the same act.

### Decision

#### 2026-10-26 — revised
*Trigger: review finding (review-001 F10) — written before the Tags list was designed, it asked where tag management is reached from and how rename, delete and counts behave; the entries below answer all but whether the sessions page reaches the Tags list too.*

**Settled by derivation** — not discussed. Determined by tags living on projects, the sessions page having its own tags editor for a session's project (Sessions List Tagging), and `x` already reaching the projects page in one key (review-001 F10).

The entries below stand, and the Tags list is reached from the projects page only; the sessions page gains no key for it.

#### 2026-10-26 — revised
*Trigger: review finding (review-001 F7) — deleting a tag everywhere was covered by the entry below, but unticking a missing project's last tag in a checklist, and `x` on its last chip in the edit screen or the sessions-list tag editor, would also leave it untagged, and the next housekeeping pass would remove it with nothing said.*

**Settled by derivation** — not discussed. Determined by project-link's Renamed Or Moved Folders (a missing project is kept only because it carries a tag — the user's work — and removal is housekeeping's, the same on every path) and the entry below's rule that the loss is shown before it happens (review-001 F7).

The entries below stand, and every way of removing a missing project's last tag says so first. Unticking its last tag in a checklist, or `x` on its last chip in the edit screen or the sessions-list tag editor, asks in one line before removing: the project's folder is gone, and with no tags left Portal will clear the project. Housekeeping still does the clearing, at its next pass — no tag surface removes a project itself, so removal stays on the one path project-link defined. Renames and merges never take a tag off a project and ask nothing.

#### 2026-10-26 — revised
*Trigger: settled call — tagged projects whose folder has gone carry tags, and removing their last tag hands them to housekeeping.*

**Settled by derivation** — not discussed. Determined by project-link's Renamed Or Moved Folders: tagged projects whose folder disappears are kept and marked missing; untagged ones are removed on every housekeeping path.

The entries below stand, and missing projects count toward each tag they carry and appear in its checklist marked missing. Removing a missing project's last tag leaves it to housekeeping like any untagged missing project, so the delete confirmation states how many of the projects losing the tag are missing and will be cleared with it.

#### 2026-10-26 — revised
*Trigger: settled call — the Tags list needed an order.*

**Settled by derivation** — not discussed. Determined by Tag Suggestions, which orders tags by how many projects carry them.

The entry below stands, and the Tags list is ordered most-used first, ties alphabetical — the order the suggestions dropdown uses, so a tag sits in the same place on both surfaces.

#### 2026-10-26

**A Tags list off the projects page manages tags.** A key on the projects page (Keys And Footer) opens it: every tag in use, each with how many projects carry it. `r` renames a tag on every project that carries it; renaming into a tag that already exists, ignoring case, merges the two after a confirmation showing the counts involved, and a project carrying both ends up with the one; a rename that changes only the casing is a plain rename (Tag Case). `d` removes a tag from every project after a confirmation showing how many projects carry it. Enter on a tag opens its project checklist (Bulk Tagging). A `+ new tag` row asks for a name — offering the existing tags as the suggestions dropdown does, so a name that already exists lands on that tag — and opens its empty checklist; a new tag with nothing ticked when the checklist closes leaves nothing behind, since a tag exists only while a project carries it.

---

## Bulk Tagging

### Context

Discovery accepted bulk tagging — add or remove a tag on several projects at once — and left open where it happens: marking several items exists only on the sessions list (`m`, multi-select), where it opens several sessions at once. The families project-link left to tags (folio with folio/api and folio/sdk) are the motivating case: tagging a family is one tag across several projects.

### Options Considered

**Tag first, then tick projects** — inside the Tags list, a tag's checklist shows every saved project, ticked where it carries the tag; Space ticks and unticks; typing filters the list.
- Pros: bulk add and bulk remove in one place, with the tag's current membership visible while it changes; reuses the Tags list rather than adding a second mode to the projects page.
- Cons: the checklist holds every saved project (66 on the install), so finding the ones wanted relies on typing to filter.

**Mark projects first, then pick a tag** — multi-select on the projects page, like `m` on the sessions list, then an add-tag or remove-tag action on the marked set.
- Pros: suits "these three new projects all get `personal`".
- Cons: needs separate add and remove paths, and shows nothing of who already carries the tag.

### Journey

The mark-first case is served tag-first at the same cost — open `personal`, tick three rows — while the reverse does not hold: mark-first cannot show a tag's membership. Single-project tagging stays in the edit screen, so a projects-page multi-select would add a mode for nothing the checklist lacks.

### Decision

#### 2026-10-26 — revised
*Trigger: review finding (review-001 F8) — written before bulk tagging was placed, it listed clashes by location: marked sessions on the sessions list sharing a project or carrying guessed links, suggestions having no single project to exclude tags from, and marked projects that already carry (or lack) the tag.*

**Settled by derivation** — not discussed. Determined by the entries below, which place bulk tagging in a tag's checklist: it works on one tag at a time, so suggestions never span several projects, and each row shows that project's current state, so partial overlap is visible rather than resolved behind the user's back (review-001 F8).

The entries below stand, and the sessions list's multi-select keeps its one job, opening several sessions at once; it does not tag. A session's project is tagged from the sessions list one session at a time (Sessions List Tagging), so the guessed-link rule never meets a set of marked sessions.

#### 2026-10-26 — revised
*Trigger: settled call — the checklist needed an order.*

**Settled by derivation** — not discussed. Determined by project-link's Linking From Picker, whose chooser reuses the projects list exactly as the projects page draws it.

The entries below stand, and the checklist lists the projects in the projects page's order; a tick or untick leaves the row where it is, so the highlight never jumps.

#### 2026-10-26 — revised
*Trigger: settled call — the checklist needed a save model.*

**Settled by derivation** — not discussed. Determined by the edit screen's existing behaviour: each chip is saved the moment it is added or removed, and closing the screen never discards.

The entry below stands, and each tick or untick in a tag's checklist is written at once, with no save step; Esc closes the checklist.

#### 2026-10-26

**Bulk tagging is the tag's project checklist in the Tags list; the projects page gains no multi-select.** Enter on a tag opens a checklist of every saved project, ticked where it carries the tag; Space ticks or unticks the highlighted project, adding or removing the tag on it; typing narrows the list. A new tag (Tag Management's `+ new tag`) opens on an empty checklist.

---

## Tag Inheritance

### Context

New projects arrive untagged: Portal saves a session's project silently when the session is created (or, under project-link, when the user saves a folder from the link chooser), and nothing tags it. The user's tags follow the folder layout, and project-link decided that a family of projects is expressed by tagging its members rather than by folder containment — anticipating inheritance as what would make a family tag cost nothing to maintain. Discovery raised inheritance and did not take it up. Measured 2026-10-26 by nearest enclosing saved project (see Context above): 36 projects sit directly under a tagged project, 31 carry exactly its tags; the five that do not are `~/.config/kit` and `~/code` (untagged under the home-folder project, tagged personal), `acom/mach2` (untagged under acom), and `fabric/flowx/flowx-api` and `fabric/packages/laravel-workflow` (personal under fabric). Four untagged top-level projects (`~/Code/kit`, `packages`, `switchboard`, `terrace`) sit directly under the untagged Code-folder project.

The background review (review-001 F9) listed the angles open: what happens when the nearest enclosing project is untagged, which saves inherit, whether a later change to a parent reaches projects already inside it, whether the user ever sees inherited tags, and what inheritance does to housekeeping — a tagged project is kept when its folder disappears, so a nested spike that inherits a tag lingers as missing instead of clearing itself. Of the deleted folders housekeeping removed in the retained logs, `dex/business` sits under `dex` (tagged personal) and would have inherited; `~/Code/harness-spike` and `~/Code/portal-spike` sit directly under the untagged Code-folder project and would not:

`grep -hE 'harness-spike|portal-spike' ~/.config/portal/state/portal.log.* | grep -oE '/Users/leeovery/[^ "]*(harness-spike|portal-spike)' | sort -u` → `/Users/leeovery/Code/harness-spike`, `/Users/leeovery/Code/portal-spike` (2026-10-26)

### Options Considered

**No inheritance (today)** — every new project starts untagged.
- Cons: every family member is tagged by hand, and an untagged project silently drops out of the family's tag filter until it is.

**Copy at save, from the nearest enclosing project** — a newly saved project takes the tags its nearest enclosing saved project carries at that moment.
- Pros: right for 31 of 36 nested projects on the install; `acom/mach2` would have been tagged acom with no action; the exceptions are fixed once by removing a tag.
- Cons: a nested spike that inherits a tag is kept as missing when its folder is deleted, rather than clearing itself.

**Live inheritance** — a project's tags include whatever its enclosing project carries now.
- Cons: tags appear and vanish on projects the user never touched; the measured exceptions (personal projects inside the fabric tree) would need an override mechanism; it reintroduces folder containment as a grouping rule, which project-link rejected.

**Walk up past an untagged enclosing project to the nearest tagged one.**
- Cons: every new project under the Code folder would take the home folder's `personal`, though the Code-folder project itself — the nearest statement — carries nothing; a new top-level work project would land personal.

### Journey

The user left this call to Claude. The measurement carried the yes: the folder layout predicts the user's own tagging for 31 of 36 nested projects, and the five misses are cheaper to fix by removing one tag than the hits are to add by hand. Copy-at-save over live inheritance followed from project-link's standing decision that projects stand alone — a tag on a project is the project's, whatever happens to its parent later. Nearest-only followed from the home folder: walking up would make `personal` the default for nearly everything. The housekeeping cost was weighed and accepted: on the retained logs one of the traceable deleted folders (`dex/business`) would have lingered, and a lingering missing project is listed on the projects page and by `portal doctor`, and cleared by deleting it or removing its tag (with Tag Management's confirmation).

### Decision

**Settled by derivation** — not discussed. Determined by the nested-project measurement (31 of 36 match their nearest enclosing project), project-link's Nested Projects (projects stand alone; families are tags) and Renamed Or Moved Folders (tagged projects are kept when their folder disappears) (review-001 F9).

**A newly saved project copies the tags of its nearest enclosing saved project.** When Portal saves a project that did not exist — at session creation, or from the link chooser's "save `<folder>` as a project" — it gives the new project the tags carried at that moment by the deepest saved project whose folder contains it, folders compared by their on-disk spelling (project-link's Folder Name Case). When that project carries no tags, nothing is inherited; Portal never looks further up. The copy is made once: a later change to the enclosing project's tags never reaches projects already saved, and the inherited tags are the new project's own, edited and removed like any other. Re-pointing a missing project at a new folder and merging two projects keep and combine their own tags and inherit nothing. Projects that exist when this ships are not touched. Nothing announces the inheritance — creation's save is silent today, and the tags show wherever tags show (By Tag, the Tags list counts, the edit screen). Inherited tags are tags for housekeeping: a nested project that inherited one is kept, marked missing, when its folder disappears.

Sibling check: project-link — Nested Projects left this question to tagging and anticipated it; Unregistered Folders keeps Portal from saving projects on its own, so inheritance runs only on the saves the user's actions make. Consistent.

---

## Keys And Footer

### Context

Three new entry points need keys: tagging a session's project and linking a session to a project (both on the sessions page; project-link left the link action's key to this topic), and opening the Tags list (projects page). The obvious key for tags, `t`, opens the theme panel on both pages (theming-system specification). Taken on the sessions page: Enter, `/`, Space, `s`, `n`, `r`, `k`, `q`, `x`, `t`, `m`, `?`; on the projects page: Enter, `x`, `e`, `/`, `t`, `d`, `n`, `q`, Esc, `?` (`internal/tui/keymap.go`). The footer shows the keys marked core and competes for width. Under `NO_COLOR` the theme key is unavailable — filtered out of the footer and help at the call site — since there is no colour to theme.

`grep -c '\`t\`' README.md docs/theming.md` → README.md 4, docs/theming.md 2 (2026-10-26): the theme key is documented in both.

### Options Considered

**Tags take `t`; theme moves to `c` (colours)** — theme leaves the footer for help only, so the footer keeps its width.
- Pros: the strong mnemonic goes to the more frequent action — a theme is set once and rarely revisited; tagging recurs.
- Cons: the user relearns the theme key.

**Theme keeps `t`; tags take `g`.**
- Cons: a weak mnemonic for the more frequent action.

`#`, the natural tag symbol, was ruled out: on a UK Mac layout it is ⌥3.

### Journey

Frequency decided it: the key with the strong mnemonic belongs to the action used more often, and the theme panel's only cost of moving is the user's own muscle memory. The link action is rare and takes `l`, kept out of the footer.

### Decision

#### 2026-10-26 — revised
*Trigger: settled call — the sessions-list tags editor needed its keys.*

**Settled by derivation** — not discussed. Determined by Sessions List Tagging: the editor is the edit screen's TAGS field on its own.

The entries below stand, and the sessions-list tags editor uses exactly the TAGS field's keys: ←→ move between chips, ⏎ on `+ add` starts a tag and on a chip edits it, `x` removes the highlighted chip, Esc closes.

#### 2026-10-26 — revised
*Trigger: settled call — a tag's checklist needed its keys.*

**Settled by derivation** — not discussed. Determined by the checklist being a modal list with no letter actions — so typing can narrow it directly, as project-link's link chooser is narrowed — the checkbox convention for Space, and Esc's behaviour in Portal's filtered lists.

The entries below stand, and in a tag's checklist typing narrows the list, ␣ ticks or unticks the highlighted project, ↑↓ move, and Esc clears a typed filter first and otherwise returns to the Tags list.

#### 2026-10-26 — revised
*Trigger: settled call — the Tags list needed its keys.*

**Settled by derivation** — not discussed. Determined by the pages the Tags list sits beside: `d` deletes on the projects page, `r` renames on the sessions page, `/` filters every Portal list, and Esc steps back from the edit screen.

The entry below stands, and in the Tags list ↑↓ move, ⏎ opens the highlighted tag's checklist (on the `+ new tag` row, starts a new tag), `r` renames, `d` deletes, `/` filters, and Esc returns to the projects page.

#### 2026-10-26

**`t` opens tagging on both pages; the theme panel moves to `c`.** On the sessions page `t` opens the tags-only editor for the highlighted session's project — through the link chooser first when its project is not a saved project (Sessions List Tagging); on the projects page `t` opens the Tags list (Tag Management). `t` takes the theme key's place among the footer's core keys on both pages, and `c` opens the theme panel from help only. The `NO_COLOR` filter that hides the theme key now applies to `c`; `t` stays available there, since tagging needs no colour. The link-to-project action takes `l` on the sessions page, listed in help, not in the footer. README and `docs/theming.md` name `t` as the theme key and change with it.

Sibling check: theming-system (specification) — "Key: `t`" opens the theme panel on Sessions and Projects; that is what ships today, so no correction is owed now — this epic's specification carries the move to `c` as a change to existing behaviour. Project-link — left the link action's key to this topic; consistent.

---

## Surface Design

### Context

Discovery noted that each new surface needs designing in the current look, including the no-colour mode. The decisions above add these surfaces: the suggestions dropdown (under the tag box on the edit screen and in the sessions-list editor — counts per row, a pinned `+ new tag` row, the current-value and already-on-this-project markers, floating over what lies below and opening upward when short of room); the sessions-list tags editor (titled with the project's name and folder); the Tags list (tags with counts, a `+ new tag` row); a tag's checklist (ticks, the missing marker, a typed filter); and the confirmations — rename-into-existing merge with counts, delete with its count and how many projects are missing, and removing a missing project's last tag. The link chooser adds nothing visual: project-link made it the projects list as the projects page draws it.

### Journey

The user set the direction while deciding Tag Suggestions: the look is spiked in Paper, in the existing Portal design file, starting from one of the existing designs. The behaviour of each surface is fixed here; Paper fixes how it looks.

### Decision

#### 2026-10-26 — revised
*Trigger: settled call — the new surfaces need colours.*

**Settled by derivation** — not discussed. Determined by Portal's closed colour vocabulary: every colour in the picker is one of 19 theme tokens, whose names are a public contract for drop-in theme files, and a token is added only for a genuinely distinct reusable role.

The entries below stand, and the new surfaces use the existing tokens — their rows, highlights, markers, borders and confirmations are roles the existing tokens already serve. No token is added, so no theme file has to change.

#### 2026-10-26 — revised
*Trigger: settled call — the new surfaces carry states that the no-colour mode must still show.*

**Settled by derivation** — not discussed. Determined by Portal's standing TUI rule that under `NO_COLOR` every state stays glyph- or letter-backed, never colour-only.

The entry below stands, and every state the new surfaces carry — the highlighted suggestion, ticked and unticked projects, the current-value and already-on-this-project markers, missing projects, the guess label — is shown by a glyph or word as well as any colour, so the no-colour mode loses nothing.

#### 2026-10-26

**Each new surface is designed in Paper, in the existing Portal design file, starting from an existing design.** The surfaces are those listed above. Behaviour — what each surface shows, its markers, its scrolling and placement rules, its keys — is fixed by this discussion; the look is the Paper design's.

---

## Sessions List Tagging

### Context

Discovery accepted tagging a session's project from the sessions list, "without going to the projects edit screen" — today that means leaving for the projects page, finding the project, and opening its edit screen. Which sessions reach the tag editor directly and which go through the link chooser first is decided below (Tagging a session's project from the sessions list when its project link is a guess, widened to every session whose project is not a saved project). What remained open was what the tag editor is: the guessed-link decision says "the tag editor then opens on that project" without saying what the user lands in, whether it removes as well as adds, where Esc returns, and how the user sees which project they are tagging — the Flat view shows nothing of a session's project link.

### Options Considered

**The edit screen's TAGS field on its own, over the sessions list** — the project's tag chips, `+ add` with the suggestions dropdown, `x` to remove a chip; titled with the project's name and folder; Esc returns to the same session row.
- Pros: the TAGS field's behaviour exactly, so nothing new to learn; the title shows which project is being tagged.

**The full projects edit screen, over the sessions list** — name, aliases and tags.
- Cons: brings renaming and aliases into a moment that is only about tags; discovery's wording reads as ruling the edit screen out here.

**A single add-a-tag box.**
- Cons: cannot remove a tag, and shows nothing of what the project already carries.

### Journey

Raised by the background review (review-001 F11). The deciding points were discovery's own wording — tagging from the sessions list without the edit screen — and visibility: since the Flat view shows nothing of which project a session belongs to, the editor's title is where the user sees what they are about to tag. The user accepted the tags-only editor.

### Decision

**Tagging from the sessions list opens a tags-only editor over the list.** It is the edit screen's TAGS field on its own — the project's chips, `+ add` with the suggestions dropdown (Tag Suggestions), `x` to remove the highlighted chip — titled with the project's name and folder, so the user sees which project they are tagging. Changes save as they are made, as on the edit screen. Esc closes it and returns to the same session row. A session whose project is not a saved project reaches it through the link chooser first (below); Esc in the chooser returns to the session row with nothing changed. The key that opens it, and the link action's own key, are Keys And Footer's.

Sibling check: project-link — Linking From Picker decided the chooser and left how sessions-list tagging invokes it to this topic; consistent. Session-tagging-and-grouping (specification) — "Tags are edited from the projects page only, never from a session row", deferred there as an indirect "edit this session's project"; this epic takes up that deferral, as discovery scoped it.

---

## Tagging a session's project from the sessions list when its project link is a guess

### Context

*From: project-link · discussion · 2026-10-11*

The project-link discussion decided that a session with no recorded project keeps a live guess of its project — the folder its active pane is in now, resolved to the repository it belongs to — for its heading, its tags and the tag and project filters. A wrong guess was accepted as silent because it corrects itself: when the pane moves back, the guess follows. The picker also gains a "link to project" action on every session row, which records the session's project by the user's choice and replaces any guess; its chooser is the projects list as the projects page draws it, with the session's current project marked and labelled as a guess when it is one.

Tagging a session's project from the sessions list (this topic's flow) writes a tag onto the session's project. When that project is a guess, the tag lands durably on whatever project the guess chose. Live example on 2026-10-11: `laravel-workflow-PGR3jA`'s pane sits in `~`, so its guess is the home-folder project; tagging it from the sessions list would tag the home-folder project — and with it every session the guess files there — while the session itself belongs to `~/Code/fabric/packages/laravel-workflow`. Unlike the guess, the tag does not correct itself when the pane moves.

Options in sight from project-link: treat a guessed link like a recorded one; route the tag action through the link chooser first when the link is a guess (the user confirms the project, which records it, then tags); or refuse with a pointer to linking. How the sessions-list tagging flow invokes the link action, and which key it takes, were already left to this topic.

The ask: whether sessions-list tagging treats a session whose project is a guess the same as one whose project is recorded.

### Options Considered

**Treat a guess like a recorded link** — the tag goes straight onto the guessed project.
- Cons: a wrong guess puts a durable tag on an unrelated project, and through it on every other session filed there, with nothing shown.

**Chooser first when the link is a guess** — the tag action opens the link chooser with the guessed project highlighted; confirming records the link, then the tag editor opens.
- Pros: a correct guess costs one Enter and leaves the session with a recorded link it keeps; a wrong guess is caught at the moment a tag would make it durable.
- Cons: one extra step on every guessed session tagged.

**Refuse, pointing at the link action.**
- Cons: a detour that reaches the same chooser with nothing gained over opening it directly.

### Journey

The asymmetry decided it: the guess is tolerable because it corrects itself, and a tag does not — so the moment a tag would be written is exactly when a guess has to become a choice. Opening the chooser with the guess highlighted makes the right-guess case nearly free, and turns a guessed session into a recorded one as a side effect, which is the outcome project-link wants for every repair. Refusing was strictly worse than the chooser: the same destination behind an extra hop.

### Decision

#### 2026-10-26 — revised
*Trigger: review finding — "a recorded project goes straight to the tag editor" breaks for a session whose recorded project the user has since deleted (the tag would be written to a project that does not exist), and no rule covered a session with no guess at all (its pane's folder gone or unreadable).*

**Settled by derivation** — not discussed. Determined by the carried-in point that an Unknown session has nowhere to put a tag until it is linked, the rejection of refusing in the entry below (the same destination behind an extra hop), and project-link's Linking From Picker, whose chooser already serves exactly these sessions — "save `<folder>` as a project" for a recorded-but-deleted project or an unsaved guess, the saved projects alone when there is neither (review-001 F6).

The entry below stands, widened: **any session whose project is not a saved project goes through the link chooser before anything is tagged** — a guessed project, a guess that lands on a folder that is not a saved project, a recorded project since deleted, and no project at all. The tag editor then opens on the project picked or saved in the chooser. Only a session whose recorded project is still a saved project goes straight to the tag editor — including a tagged project kept despite its missing folder.

#### 2026-10-26

**A guessed link goes through the link chooser before anything is tagged.** Tagging from the sessions list on a session whose project is a guess opens the link-to-project chooser with the guessed project highlighted; picking a project (the guess or another) records the session's link, and the tag editor then opens on that project. A session whose project is recorded goes straight to the tag editor. Tagging never writes onto a guessed project.

Sibling check: project-link — Linking From Picker decided the chooser (the projects list, the current project marked and labelled as a guess) and that a picked link replaces the guess for that session; it left how the sessions-list tagging flow invokes it to this topic. Consistent; this is that invocation.

---

## Summary

### Key Insights

1. The default keystroke belongs to reuse, never to creation. Five tags after months of use means picking an existing tag is the common case: Enter takes the highlighted suggestion, a new tag takes a deliberate move, case never splits a tag, and Enter on a box the user has not typed into writes nothing.
2. A tag is the moment a guess must become a choice. A guessed project link corrects itself when the pane moves; a tag written onto it would not — so every session without a saved project passes through the link chooser before anything is tagged, and comes out with a recorded link.
3. Membership is the thing management and bulk tagging both act on. One Tags list — a count per tag, a checklist per tag — replaced a separate multi-select mode, and shows who carries a tag while it changes.
4. Removing a tag can remove a project. Project-link keeps a missing project only while it carries a tag, so every tag-removal path that can leave a missing project bare says so before it acts.
5. The folder layout predicts the user's tagging: 31 of 36 nested projects carry exactly their nearest enclosing project's tags, which makes copy-at-save inheritance right far more often than wrong.

### Open Threads

- The narrow-width overflow bug (inbox) touches the edit screen, where the suggestions dropdown now floats; discovery left it out of the epic.
- The theming-system specification and README/`docs/theming.md` describe `t` as the theme key; the move to `c` is this epic's change, carried by its specification.

### Current State

- Resolved: tag identity ignores case and a tag has one casing on disk; suggestions with ranking, blank-box, chip-edit, carried-tag, scrolling and placement rules; a Tags list off the projects page with rename-merge, delete and per-tag checklists for bulk tagging; a tags-only editor from the sessions list, reached through the link chooser for any session without a saved project; copy-at-save inheritance from the nearest enclosing project; confirmations before a missing project loses its last tag; `t` for tagging, `c` for theme, `l` for link; every new surface designed in Paper.
- Uncertain: inheritance is the call closest to taste — the user left it to derivation; a nested spike that inherits a tag lingers as a missing project after its folder is deleted.
