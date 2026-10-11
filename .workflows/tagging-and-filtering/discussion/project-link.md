# Discussion: Project Link

## Context

Tags in Portal hang off projects, and sessions are tagged only through their project. Every tag filter and project filter this epic adds is therefore only as good as the link between a session and its project: a session linked to the wrong project, or to none, silently drops out of the filters the user relies on. This topic settles which project a session belongs to, and keeps that link reliable through worktrees, unregistered folders, renamed or moved folders, nested projects, and folder-name case.

### Inherited position from discovery

Discovery reached these with the user; they carry forward as working ground, open to movement only where this discussion surfaces something that contradicts them:

- **A session's project is the git repository the session was started in** — the root of that repository.
- **A worktree belongs to the repository it was made from**, not to the worktree folder. A worktree's own git root is the worktree folder itself, so "roll up to the git root" taken literally lands on the worktree; the rule names the repository the worktree belongs to. The project is decided when the session is created, while the folder still exists, and kept — once a worktree is deleted git can no longer be asked which repository it came from.
- **The user's workspace pattern falls out of the rule with no special case.** `~/Code/fabric/folio` holds three independent repositories (api, app, sdk) and is itself a git repository carrying the workspace setup, docs and workflows. Sessions are started at the folio level and link to folio. A session deliberately started inside `folio/api` links to `folio/api`, a registered project of its own — accepted.
- **The folder-name case mismatch is fixed inside this epic**, not as a separate bugfix: the epic reworks how a session links to its project anyway.
- **The reboot link loss is a separate bugfix** (inbox: `session-project-link-lost-on-reboot`). Portal records a session's origin folder only when it creates the session; save and restore do not carry it, so after a reboot every restored session is grouped by a guess from its active pane's current folder (34 of 38 live sessions were on the guess on 2026-10-01). That bugfix lands before this epic's implementation, not before its discussions. It also covers the common worktree case seen live: the three live worktree sessions were all started at their repository's root and later moved into a worktree (all three worktree folders since deleted), so their recorded origin would have kept them under the parent project — it is the post-reboot guess that sends them to Unknown.

### Rejected paths from discovery

- Treating the reboot link loss as part of this epic — split out because Portal already promises the link holds and a reboot breaks that promise; a straightforward fix that repairs the By Project view immediately, while worktrees and unregistered folders are design questions filtering has to answer.
- Treating the goosemates and flowx sessions as a bug. The goosemates session was created on 2026-09-26 in `~/Code/homeos`, where Portal saved a project "homeos"; the folder was later renamed (HomeOS → Goosemates) along with the session, the saved project then pointed at a folder that no longer existed, and stale-project housekeeping removed it (one entry removed 2026-09-28). Normal behaviour. `~/Code/flowx` is a separate repository from the registered `~/Code/fabric/flowx` (not a symlink) and predates the retained logs.

### Open questions carried in

- Whether Portal saves a folder as a project when it finds a session whose folder has none (`~/Code/flowx`, `~/Code/goosemates` today).
- A way to link an Unknown session to a project, or save its folder as one, from the picker. Tagging a session's project from the sessions list needs it for any Unknown session, and it is also the repair for a renamed folder.
- What happens to a project and its tags when its folder is renamed or moved. Housekeeping removes the record silently — both the daemon's throttled prune and `portal doctor --fix` — and its summary line records only a count, so neither the project nor its tags can be recovered from the log. Tolerable while tags only feed a grouping view; once tags drive filtering, a folder rename silently drops sessions out of filters.
- Whether a project takes in the projects nested inside it. 34 of the 62 saved projects sit inside another project's folder (folio holds folio/api and folio/sdk; docman holds docman/docman and docman/sdk-php; finder holds finder/finder-api; flowx holds flowx/flowx-api; fabric holds most of the fabric projects). It decides whether a folio filter or By Project group takes in sessions started in folio/api — given the workspace pattern, likely wanted. A project is also saved at `~/code` (the Code folder itself), which under any nesting rule would contain nearly every project.
- Folder-name case. On this case-insensitive Mac, `~/code/papercode` and `~/Code/papercode` are the same folder, but Portal compares project folders with their case as written (path canonicalisation leaves case untouched — checked in discovery). The papercode project is saved as `~/code/papercode` and the `~/code` project duplicates `~/Code`. A session whose folder is spelled the other way would not match its project, and opening one folder under both spellings can save it as two projects with separate tags. Not yet confirmed against a live mislinked session.
- A session's folder and its project become two different things. Today one recorded folder serves both as the project link and as the folder shown and searched beside a session in `x /term`. A session started in a worktree would have the worktree as its folder and the parent repository as its project.
- The slow switch to By Project / By Tag comes from looking up each session's folder one at a time; project and tag filtering take the same path. Most of it disappears once the reboot fix lands and restored sessions carry their recorded link again.

### References

- [Discovery brief — Project Link](../discovery/briefs/project-link.md)
- [Discovery session 001](../discovery/sessions/session-001.md)

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent. The Discussion Map (which subtopics exist and their states) lives in the manifest, not this file.*

---

## Session Folder And Project

### Context

Today Portal records one folder on each session at creation, and it does several jobs at once: it decides which project the session groups under (and so its tags), it is the folder shown on the session's row, it is what the picker's text filter and `x /term` match against, and its name generates the session's name (`{folder name}-{nanoid}`). Discovery's worktree rule pulls those apart. A session started in `~/Code/portal/.claude/worktrees/tag-filters` belongs to the portal project, but if the one recorded folder became `~/Code/portal`, its row would show `~/Code/portal`, typing `tag-filters` in the filter would find nothing, and the session would be named `portal-…` — indistinguishable from the main-checkout session beside it under the same heading. Session names are not a durable cue either: sessions are routinely renamed.

Today a session started in a worktree saves the worktree folder as a project of its own (creation resolves to the worktree's own git root), which housekeeping prunes — tags and all — once the worktree is deleted. No saved project is a worktree today:

`while read p; do g=$(git -C "$p" rev-parse --path-format=absolute --git-dir 2>/dev/null); c=$(git -C "$p" rev-parse --path-format=absolute --git-common-dir 2>/dev/null); [ -n "$g" ] && [ "$g" != "$c" ] && echo "$p"; done < <(jq -r '.projects[].path' ~/.config/portal/projects.json)` → no output (2026-10-11)

### Options Considered

**Record both** — the folder the session was started in, and the project it belongs to.
**Record the project only** — the row shows the project folder; a worktree session is found and told apart by name alone.

### Journey

Recording the project alone regresses what a worktree session shows and is found by today, and leans on the session name, which the user renames freely. Recording both changes nothing visible for any session not started in a worktree, where folder and project are the same folder. The reboot bugfix restores the one folder Portal records today; the project is new here, and must survive a reboot on its own account because a deleted worktree's project cannot be re-derived — so carrying it through save and restore belongs to this epic.

### Decision

**Settled by derivation** — not discussed. Determined by discovery's worktree rule plus no regression to what a worktree session shows and is found by today.

A session records two things at creation: **the folder it was started in** and **the project it belongs to**. The folder keeps every job it has today apart from grouping — the session row, the picker's text filter, `x /term`, and the generated session name. The project drives the By Project heading, the session's tags, and every tag or project filter. Save and restore carry both. For every session not started in a worktree the two are the same folder. Creation saves the session's project as a saved project — for a worktree session the repository the worktree was made from, even one never opened directly — and no longer saves the worktree folder as a project of its own. *(Amended 2026-10-11 — this left creation's save target implicit; discovery's worktree rule and Session Folder And Project make the thing creation saves the session's project, which for a worktree is the repository it was made from.)*

Sibling check: open-with-forced-filter (specification) — the recorded folder alone is shown on the row and matched by `x /term` and the text filter; consistent, the session's folder keeps exactly that role.

---

## Unrecorded Sessions

### Context

Once the reboot bugfix lands, every session Portal creates keeps the folder it was started in across reboots. Two populations still carry no record: sessions created some other way (tmux's own `new-session`, a script, another tool), and the sessions running when the fix ships that already lost their record at an earlier reboot — the fix cannot recover an origin that was never saved. For those, Portal guesses today from the folder the session's active pane currently sits in, resolved to its git root. The guess is grouping-only: the `open-with-forced-filter` specification established that a derived folder never lands in the recorded one and never feeds the session row's folder, the text filter or `x /term`, so a regroup cannot make a session findable by a path it was not findable by a moment earlier.

Measured against the live install on 2026-10-11 (read-only): 30 non-hidden sessions carry no recorded folder; the guess puts 21 under a plausible project and 9 elsewhere —

- `laravel-workflow-PGR3jA` — pane in `~`, so it files under the home-folder project and takes that project's tags.
- `ws-folio-dev` — pane in `folio/app`, not a project → Unknown.
- Two worktree sessions (agentic-workflows' `mailbox-and-inbox`, switchboard's `capture-harness`) → Unknown, because the guess resolves a worktree to the worktree folder.
- Four sessions in `~/Code/flowx` and one in `~/Code/goosemates` → Unknown, no project at either folder.

`tmux list-sessions -F '#{session_name}|#{@portal-dir}' | while IFS='|' read n d; do case "$n" in _portal-*) continue;; esac; [ -n "$d" ] && continue; p=$(tmux display-message -p -t "=$n:" '#{pane_current_path}'); r=$(git -C "$p" rev-parse --show-toplevel 2>/dev/null || echo "$p"); echo "$n -> $r"; done` → 30 rows (session names compared against the guessed folder by hand; renamed sessions judged on the folder alone)

### Options Considered

**1. Keep guessing, live** (today's behaviour, with the worktree rule applied to the guess)
- Pros: right for most sessions with no record; a session whose pane wanders off files under wherever the pane went and returns when the pane does — the guess corrects itself; no one-time chore at ship.
- Cons: silent when wrong — a wrongly guessed session appears in a filter it doesn't belong to, or drops out of one it does.

**2. Guess once, then keep it** — the first guess becomes the session's record.
- Pros: stops drift; the guess never runs again for that session.
- Cons: locks in whatever folder the pane happened to be in at that moment. Portal deliberately avoids this today for exactly that reason (`internal/tui/model.go`: freezing a drifted cwd would permanently mis-group the session).

**3. No guess** — sessions without a record sit in Unknown until the user links them.
- Pros: never wrong.
- Cons: all 30 current unrecorded sessions start in Unknown and are missing from every tag and project filter until linked one by one.

All three assume the picker gains a way to link a session to a project (Linking From Picker), which turns any guess into a record the user chose.

### Journey

The trade turned on filters. With filters, being missing is as much a failure as being misplaced: option 3 trades 9 misplaced-or-Unknown sessions for 30 missing from every filter until a manual pass. Between 1 and 2, the live guess self-corrects when the pane returns and the frozen one does not, so 2 buys stability at the price of making a transient wrong answer permanent. Applying the worktree rule to the guess moves the two worktree sessions from Unknown to their parent project while their worktree still exists.

### Decision

**Option 1 — keep the live guess for sessions with no record, with the worktree rule applied to it.** A guess resolves the pane's folder to the repository it belongs to (a worktree to the repository it was made from), exactly as creation does. The guess stays a project-link value only — heading, tags, and tag/project filters — and never becomes the session's folder: the row, the text filter and `x /term` keep matching only what was recorded, as `open-with-forced-filter` decided. A link the user records from the picker replaces the guess for that session. Accepted trade-off: a wrong guess is silent.

Sibling check: open-with-forced-filter (specification) — a derived folder never lands in the recorded one and is read by grouping alone; consistent, with tag and project filtering reading it as grouping does.

---

## Unregistered Folders

### Context

Every session Portal creates saves its project on the spot (creation upserts it — for a worktree session the repository it was made from, never the worktree folder) *(Amended 2026-10-11 — this left creation's save target implicit; discovery's worktree rule and Session Folder And Project make the thing creation saves the session's project, which for a worktree is the repository it was made from.)*, so a session whose project is not a saved project arises only two ways: the session's recorded project was deleted (the projects page's delete, or a hand edit — housekeeping only prunes projects whose folder is gone), or the session has no record and its live guess lands in a folder that was never a project. *(Amended 2026-10-11 — this was worded around the session's folder; Session Folder And Project records the session's project apart from its folder, so the test is the session's project: its recorded project, or for a session with no record its live guess.)* Live examples on 2026-10-11: four sessions guessed into `~/Code/flowx` (a separate repository from the saved `~/Code/fabric/flowx`), one into `~/Code/goosemates` (the renamed `~/Code/homeos`, whose project housekeeping removed), and `ws-folio-dev` guessed into `~/Code/fabric/folio/app` (a repository inside folio, never saved as a project). The open question discovery left: whether Portal saves such a folder as a project when it finds one.

### Options Considered

**Save it automatically** — the session then groups under a new project at that folder.
**Never save automatically** — the session sits in Unknown until the user saves or links it from the picker.

### Journey

Each source makes automatic saving wrong in its own way. For a recorded folder, a missing project is the user's own deletion, and recreating it would undo them. For a guessed folder, saving it as a project converts a guess chosen *because* it corrects itself into a durable record that does not: `ws-folio-dev`'s pane sitting in `folio/app` would mint a `folio/app` project and keep the session filed there after the pane moved back. The goosemates case, where automatic saving would have helped, is served by the picker's explicit save.

### Decision

**Settled by derivation** — not discussed. Determined by the live-guess decision's own rationale (Unrecorded Sessions).

**Portal never saves a folder as a project on its own when a session's project is not a saved project.** Such a session stays in Unknown until the user saves that folder as a project or links the session to an existing one from the picker (Linking From Picker). *(Amended 2026-10-11 — this was worded around the session's folder; Session Folder And Project records the session's project apart from its folder, so the test is the session's project: its recorded project, or for a session with no record its live guess.)* Creation still saves a new session's project when the session is created, as today; for a worktree session that project is now the repository it was made from (Session Folder And Project). *(Amended 2026-10-11 — this left creation's save target implicit; discovery's worktree rule and Session Folder And Project make the thing creation saves the session's project, which for a worktree is the repository it was made from.)*

Sibling check: no overlap found.

---

## Linking From Picker

### Context

With automatic saving ruled out and wrong guesses accepted as silent, the picker needs a way for the user to say which project a session belongs to: it is the repair for a wrong guess (`laravel-workflow-PGR3jA` filed under the home-folder project), the way out of Unknown (`ws-folio-dev`, the `~/Code/flowx` and `~/Code/goosemates` sessions), and the prerequisite for tagging an Unknown session's project from the sessions list (the tagging topic's sessions-list tagging needs a project to put the tag on).

### Options Considered

The shape was proposed and stood without objection: an action available on **every** session row — guessed, recorded or Unknown — opening a chooser of the saved projects, with **"save `<folder>` as a project"** at its top when the session's project is not a saved project. The folder offered there is that project value itself — the session's recorded project, or for an unrecorded session its guess, already resolved by the repository rule (a worktree to the repository it was made from) — never a re-resolution of the session's own folder; a session linked to a project since deleted is offered that project's folder. A session with neither, an unrecorded one whose guess resolved nothing (its pane's folder gone or unreadable), gets no save entry and its chooser lists only the saved projects. *(Amended 2026-10-11 — this was worded around the session's folder; Session Folder And Project records the session's project apart from its folder, so the test is the session's project: its recorded project, or for a session with no record its live guess.)* Picking records the session's project; the folder the session shows on its row and is searched by is left as it was — an unrecorded session stays without one.

The fork was what a link applies to, worked on the four `~/Code/flowx` sessions linked to the saved `fabric/flowx` project:

**This session only**
- Pros: matches the repair's grain — a wrong guess is about one session; adds no second membership rule beside the repository rule.
- Cons: the other three stay Unknown until each is linked; the next session started in `~/Code/flowx` saves that folder as a project of its own, per the repository rule.

**That whole folder** — Portal remembers that `~/Code/flowx` belongs to `fabric/flowx`; all four move at once and so do future sessions started there.
- Pros: one action for many sessions.
- Cons: a second way of deciding which project a folder belongs to, beside the repository rule — the same shape nested projects might want (`folio/api` belonging to folio), so deciding it here would settle that subtopic sideways.

### Journey

The flowx case itself argued against folder-level linking: `~/Code/flowx` and `~/Code/fabric/flowx` are different things that only look alike (the user: "they are different (kind of)") — `~/Code/flowx` is its own git repository, `~/Code/fabric/flowx` a plain folder holding `flowx-api` — so a folder-level rule taught by one link would have folded future work in a separate repository into the wrong project. Saving a folder as a project already covers the many-sessions case where it matters: saving `~/Code/goosemates` fixes every other session guessed into that folder at once, because their guess now lands on a project.

The closing review found the chooser's save entry would offer a vanished folder once a recorded session's untagged project had been removed after a rename: replaying the homeos history on a recorded session, the entry would offer `~/Code/homeos`, and nothing would offer `~/Code/goosemates` until a fresh session there had saved it. A tagged project never reaches this — it is kept as missing and re-pointed from the projects page (Renamed Or Moved Folders). The repair chosen leans on the shell having followed the rename; the alternative, dropping the save entry, left a detour through a fresh session.

### Decision

**A link applies to the one session it is made on.** Every session row offers the action; the chooser lists the saved projects and, when the session's project is not a saved project, offers saving that project's folder as one. *(Amended 2026-10-11 — this was worded around the session's folder; Session Folder And Project records the session's project apart from its folder, so the test is the session's project: its recorded project, or for a session with no record its live guess.)* Picking records the session's project — durable across reboots like any recorded project (Session Folder And Project) — and replaces any guess for that session; the session's folder is untouched. Future sessions started in the same folder follow the repository rule as before. If nested projects later needs folder-level membership, linking can grow into it then.

When the folder the save entry would offer no longer exists — the session's project sat at a folder since renamed or moved, and housekeeping removed it because it carried no tags — the save entry offers instead the folder the session's active pane is in now, resolved by the repository rule exactly as a guess is. After a rename the shell has followed the folder to its new name, so one pick repairs the session; the folder is shown before anything is saved, so a pane that has wandered elsewhere offers a folder the user can see is wrong and ignore. (resolves review-001 F4)

Sibling check: tagging (discovery brief) — sessions-list tagging relies on this chooser to reach a project and says nothing about a vanished folder; no overlap.

Sibling check: tagging (discovery brief) — sessions-list tagging needs Unknown sessions to reach a project, which this action supplies; the key it takes and how the tagging flow invokes it are the tagging topic's.

---

## Renamed Or Moved Folders

### Context

Housekeeping removes a saved project whose folder no longer exists — on the daemon's throttled idle tick, when the picker runs it, and under `portal doctor --fix` — and takes the project's tags with it. Its log line is a summary count; the per-project detail is written at DEBUG, below the production level, so neither the project nor its tags can be recovered from the log. A folder renamed or moved looks exactly like a folder deleted for good: from a missing folder alone housekeeping cannot tell `homeos` (renamed to goosemates, still in use) from a spike folder the user is finished with. Tolerable while tags only fed a grouping view; once tags drive filtering, a folder rename silently drops every session of that project out of its filters.

Measured from the retained logs (2026-09-11 → 2026-10-11): housekeeping removed 13 projects in 7 runs, from both the daemon and the picker.

`grep -h "projects: clean-stale" ~/.config/portal/state/portal.log.* | grep -o 'entries=[0-9]*'` → 7 lines, entries summing to 13

The log does not name them; matching the INFO project-creation lines in the same logs against today's `projects.json` identifies 6 — one rename (`~/Code/homeos`), one worktree (`dex-engine/.claude/worktrees/fixes-0-2-3`), and four folders since deleted (`fumi-hooklab`, `harness-spike`, `portal-spike`, `dex/business`):

`comm -23 <(grep -h "projects: set" ~/.config/portal/state/portal.log.* | grep -o 'path=[^ ]*' | sed 's/path=//' | sort -u) <(jq -r '.projects[].path' ~/.config/portal/projects.json | sort)` → those 6 paths

The worktree case disappears under Session Folder And Project: worktrees no longer become projects.

### Options Considered

**1. As today, but logged** — any missing project is removed; each removal is logged with its path, name and tags so it can be rebuilt by hand.
- Cons: the repair is manual reconstruction from a log; sessions drop out of filters at the next prune.

**2. Keep tagged projects** — a tagged project whose folder disappears is marked missing on the projects page instead of being removed, where the user points it at its new folder or deletes it; untagged projects are still removed as today.
- Pros: the user's own work is never deleted silently; the projects list still clears itself of deleted experiments, which carry nothing the user made.
- Cons: a tagged project whose folder was deleted for good waits for the user to delete it.

**3. Keep every project** — nothing is removed on its own; missing projects sit flagged until the user deals with them.
- Pros: also preserves a project's name and sessions' grouping when it carried no tags.
- Cons: the projects list accumulates every deleted folder.

**4. Detect the move automatically** — e.g. match a missing project to a new folder with the same git history.
- Cons: two clones of one repository share that history, and plain folders (`fabric/flowx`, `papercode`) have no history to match. Rejected.

### Journey

Under 2 or 3, sessions recorded under the missing project keep grouping under it and stay in its filters until the user acts, because their recorded project still names it. Pointing the project at its new folder brings its tags and its sessions along; if a session has already been started in the new folder, it saved a second project there, so pointing the old project at that folder merges the two. The deciding line between 2 and 3 was what counts as the user's own work: tags are, and once filters depend on them, removing them silently is data loss — the same stance the hooks store takes (a key the staleness rule cannot judge is retained). An untagged project holds only a folder name and a last-used time; removing those keeps the projects list clear of the deleted experiments that made up four of the six traceable removals.

### Decision

#### 2026-10-11 — revised
*Trigger: settled call — a kept missing project needs a place outside the projects page where it is reported.*

**Settled by derivation** — not discussed. Determined by the keep-tagged rule above and doctor's role as the health catalog.

The entries below stand, and `portal doctor` lists each missing tagged project as an info line — info lines never fail its exit code — while `portal doctor --fix` leaves them in place, following the same keep-tagged rule as every other housekeeping path.

#### 2026-10-11 — revised
*Trigger: settled call — re-pointing and merging left the project's name unspecified.*

**Settled by derivation** — not discussed. Determined by how project names are generated today (a project's default name is its folder's name) and the merge the decision already carries.

The entries below stand, and a re-pointed project takes its new folder's name unless its name was changed by hand: a name still equal to the old folder's name was never chosen by the user and follows the folder (`homeos` pointed at `~/Code/goosemates` becomes `goosemates`), while a hand-set name stays. Pointed at a folder that is already a project, the two merge into one project at that folder with their tags combined, named by the same rule.

#### 2026-10-11 — revised
*Trigger: settled call — a session started at or under a renamed folder keeps showing a path that no longer exists while its shell has followed the rename.*

**Settled by derivation** — not discussed. Determined by the session folder's job of showing and finding where the session actually is (Session Folder And Project).

The entry below stands, and pointing a project at a new folder also rewrites the recorded folder of every session started at or under the old folder to the same place under the new one, so the session row and `x /term` show and match a folder that exists.

#### 2026-10-11

**Option 2 — tagged projects are never removed by housekeeping.** When a tagged project's folder disappears, the project is kept and marked missing on the projects page, where the user points it at a new folder or deletes it. Sessions recorded under it keep grouping under it and stay in its filters meanwhile. Pointing it at a new folder carries its tags and sessions; pointing it at a folder that is already a project merges the two. Untagged projects whose folder disappears are removed as today. The rule is the same on every housekeeping path — the daemon's prune, the picker's, and `portal doctor --fix`.

---

## Folder Name Case

### Context

On this case-insensitive Mac, `~/code` and `~/Code` are one folder, but Portal compares project folders by their spelling as written: its path canonicalisation resolves symlinks and leaves case untouched, and the system's own path resolution does the same.

`python3 -I -c "import os; print(os.path.realpath('/Users/leeovery/code'))"` → `/Users/leeovery/code` (the folder is spelled `Code` on disk: `ls ~ | grep -ix code` → `Code`)

Two saved projects carry a spelling the disk does not — `~/code` and `~/code/papercode` — and one live session is recorded at `/Users/leeovery/code` (`code-OQWkFt · sitrep`). No two saved projects collide when compared without case today:

`jq -r '.projects[].path|ascii_downcase' ~/.config/portal/projects.json | sort | uniq -d` → no output (2026-10-11)

A session whose folder is spelled the other way from its project would not match it, and opening one folder under both spellings saves it as two projects with separate tags. The retained log also shows a project named `Code` at `/Users/leeovery/Code` removed from the projects page on 2026-09-30, while the `~/code` project — the same folder — survived.

### Journey

Discovery already decided the mismatch is fixed inside this epic. The only key that names a folder once whatever it is typed as is the spelling on disk; on a case-sensitive filesystem that is the spelling as written, so the same rule holds there unchanged.

### Decision

**Settled by derivation** — not discussed. Determined by discovery's decision to fix the case mismatch here and on-disk identity being the only defensible key.

**Two spellings of one folder are one project.** Saved projects and recorded session folders are compared as the folder is spelled on disk, and saved projects written another way are re-spelled — `~/code` becomes `~/Code`, `~/code/papercode` becomes `~/Code/papercode`. Should two saved projects ever turn out to be the same folder, they merge with their tags combined. The re-spelled `~/code` project sits at the same folder as the `Code` project deleted on 2026-09-30; whether a project belongs at the Code folder at all is Nested Projects'.

---

## Nested Projects

### Context

Many saved projects sit inside another project's folder, which raises whether a project — its By Project heading and a project filter — takes in sessions started in the projects nested inside it. The case discovery named: sessions are started at the folio level, but a session started in `folio/api` gets its own heading, "api" — one of three "api" headings beside `evvi/api` and `mach2/api` — and a folio project filter misses it. Two saved projects sit at folders that contain nearly every other project: the home folder (`~`, tagged personal) and the Code folder (saved as `~/code`, re-spelled `~/Code` under Folder Name Case).

Measured 2026-10-11 (read-only). Excluding the home and Code folder projects, 12 saved projects contain others; only two are git repositories (`fabric/folio` with `api` and `sdk`; `fabric/finder` with `finder-api`), the other ten plain folders (`acom`, `acom/mach2`, `curated`, `detaillab`, `dex`, `evvi`, `fabric`, `fabric/docman`, `fabric/flowv1`, `fabric/flowx`). Under every parent, all nested projects carry the parent's tag but three: `acom/mach2` (untagged under acom), `fabric/flowx/flowx-api` and `fabric/packages/laravel-workflow` (both personal under fabric).

`python3 -I -c "import json,os,subprocess as s; ps={p['path']:p for p in json.load(open(os.path.expanduser('~/.config/portal/projects.json')))['projects']}; ex={os.path.expanduser('~'),os.path.expanduser('~/code')}; [print('GIT' if s.run(['git','-C',q,'rev-parse','--show-toplevel'],capture_output=True,text=True).stdout.strip()==q else 'PLAIN', q, ps[q].get('tags'), [(k[len(q)+1:],ps[k].get('tags')) for k in sorted(ps) if k.startswith(q+'/')]) for q in sorted(ps) if q not in ex and any(k.startswith(q+'/') for k in ps)]"` → 12 rows, 2 GIT

### Options Considered

**1. Projects stand alone; tags group them** — a project filter shows sessions started at that project's folder; a family (folio, api, sdk) is a tag put on its members and filtered on.
- Pros: one way of saying "these projects belong together", the one already in use — the user's tags already form the families at the broad level; a project filter means exactly that folder; the home and Code folder projects need no special case.
- Cons: a finer family (folio's) needs a tag added to its members.

**2. Repositories take in what's inside them** — a project that is itself a git repository includes the projects inside it (folio takes in api and sdk, finder takes in finder-api); plain-folder projects never do, which stops the home and Code folder projects swallowing everything. Under By Project, folio/api's sessions would show under folio, their own folder on the row. Today: 2 parents taking in 3 projects.
- Cons: whether a project takes in its neighbours turns on whether its folder happens to be a git repository, which nothing in the picker shows — folio absorbs its children and docman does not, with no visible reason.

**3. A per-project switch** — the project's edit screen gains "takes in nested projects", on for folio, off for the home folder.
- Cons: a second way to group projects beside tags, for the 3 projects that would use it.

### Journey

Discovery leaned toward rollup ("given the workspace pattern, likely wanted"), but the measurement showed the families the user cares about are already expressed as tags at the broad level, and only two of the twelve containing projects are repositories at all — so a repository-based rollup would serve folio and finder while leaving the plain-folder families (docman, mach2, dex) flat for reasons the picker cannot show. Whether a new nested project picks up its enclosing project's tags automatically is the tagging topic's open question, and it is what would make a family tag cost nothing to maintain.

### Decision

**Option 1 — projects stand alone; families are tags.** No project takes in the projects nested inside it: a session belongs to the project it was started in (Session Folder And Project), appears under that project's heading in By Project, and matches a project filter for that project only. A group of related projects is expressed by tagging its members. The home folder and Code folder projects are ordinary projects for the sessions started there.

Sibling check: tagging (discovery brief) — whether a new project inherits the tags of the project whose folder encloses it is open there; this decision leaves families to tags and anticipates that question rather than deciding it. Filtering (discovery brief) — narrowing to a project is that topic's surface; consistent, a project filter matches that project's sessions alone.

---

## Lookup Cost

### Context

Switching the picker to By Project or By Tag is slow today (~0.5s reported) because the folder of each session with no record is looked up one session at a time — the active pane's folder, then its git root. Discovery expected most of it to disappear with the reboot fix, but under Unrecorded Sessions the live guess keeps running for every session that has no record, including the 30 running now whose origin was never saved, and tag and project filters take the same path as grouping.

Measured on the live install 2026-10-11 (read-only), for the 30 unrecorded sessions:

`S=$(tmux list-sessions -F '#{session_name}|#{@portal-dir}' | awk -F'|' '$2=="" && $1 !~ /^_portal/ {print $1}'); time (echo "$S" | while read n; do tmux display-message -p -t "=$n:" '#{pane_current_path}' >/dev/null; done)` → 0.201s

`P=$(tmux list-panes -a -F '#{pane_active}|#{pane_current_path}' | awk -F'|' '$1==1{print $2}' | head -30); time (echo "$P" | while read p; do git -C "$p" rev-parse --show-toplevel >/dev/null 2>&1; done)` → 0.199s

`time (tmux list-panes -a -F '#{session_name}|#{pane_active}|#{pane_current_path}' >/dev/null)` → 0.007s

### Journey

The cost scales with the number of guessed sessions, and the live-guess decision keeps that number at 30 until those sessions are killed; filters are interactive, so the delay would recur on every filter change, not only on a view switch.

### Decision

**Settled by derivation** — not discussed. Determined by the live-guess decision (Unrecorded Sessions) and filters being interactive.

**Switching to a grouped view, or applying a tag or project filter, shows no delay however many sessions are guessed.** Guessed folders are read for all sessions at once, never one session at a time.

---
