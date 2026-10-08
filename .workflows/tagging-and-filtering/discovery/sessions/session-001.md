# Discovery Session 001

Date: 2026-10-08
Work unit: tagging-and-filtering

## Description (as of session)

Improve tagging, filtering and the related display surfaces in the picker — tag suggestions while typing, bulk tagging, tag management, filtering the session list by tags and by project, and settling what project a session belongs to.

## Seed

(none)

## Imports

(none)

## Map State at Start

(empty — first session)

## Exploration

The user opened wanting to improve the picker's filtering and display surfaces and its tagging. Their concrete asks: when tagging, show a dropdown of the tags already in use; bulk tagging; filtering the list to sessions/projects carrying a chosen tag or tags; possibly filtering by project; and a brainstorm of anything else in the area.

The brainstorm laid out three areas, all of which the user accepted ("I love all that — we are in a good place to do all of these now"):

- Tagging: suggestions while typing a tag (existing tags across all projects, narrowing as you type); bulk tagging (mark several projects, add or remove a tag on all at once); tag management in one place (rename or delete a tag everywhere, see how many projects carry each); tagging a session's project from the sessions list without going to the projects edit screen; per-session tags (a tag on a session its directory does not carry). Per-session tags, a command-line tag flag, grouped filtering and tag exclusion were all explicitly deferred when tagging first shipped (session-tagging-and-grouping), so this picks up existing threads.
- Filtering: narrow to one or more tags with an any/all choice; narrow to a project; exclude a tag ("everything except archive"); keep the group headings while filtering (today typing a filter flattens the list); filter from the command line (e.g. `x --tag work` opening the picker pre-narrowed, or opening every tagged session at once).
- Display: show the active filter as chips in the header so it is visible what is narrowing the list.

The user raised a concern that some sessions are not linked to a project. A read-only look at the live install (38 sessions, projects.json) found two distinct things:

1. The session-to-project link is lost on every reboot — the session's recorded origin directory is written only at creation and is not carried through save/restore, so after a reboot every restored session falls back to guessing from its active pane's current folder. 34 of 38 live sessions were running on the guess. This was split out of the epic as a prerequisite bugfix and captured to the inbox (session-project-link-lost-on-reboot), since everything this epic builds keys off that link.
2. Sessions that legitimately land in Unknown because their folder is not a known project: three sessions working inside git worktrees (two under agentic-workflows/.claude/worktrees, one switchboard worktree) resolve to the worktree folder rather than the parent repo's project; five sit in folders never registered as projects (four in ~/Code/flowx where the saved project is ~/Code/fabric/flowx, one in ~/Code/goosemates). Whether a worktree counts as its parent repo's project, and whether an unregistered folder should become a project automatically, are design questions kept inside this epic — filtering by project has to answer them.

The picker row rework (directory beside each session name, the right-hand status strip) is already a waiting roadmap item and the user confirmed it stays separate; display work tied to filtering (filter chips) stays in this epic.

Epic vs separate features: chosen as an epic because the decisions are interdependent — per-session tags changes what tag suggestions offer, what bulk tagging writes and what a tag filter matches; what a worktree belongs to changes what filtering by project means. Topics still ship independently and in any order.

Two inbox bugs sit on the screens this work touches and were offered as candidate topics, left undecided by the user and still in the inbox: narrow-width overflow (the section header, where filter chips would go, and the edit screen, where tag suggestions would appear) and the keymap footer clipping on short terminals (filter chips add a row to that same vertical budget). The known slowness switching to By Project / By Tag (one folder lookup per session) is on the path project/tag filtering takes; most of it disappears once the reboot fix lands and restored sessions carry their real link again.

Ordering against the reboot bugfix: the epic's discovery, discussions and specs are conversation only, so the bugfix needs to land before this epic's implementation, not before its discussions. The bugfix can run in parallel from workflow start.

Decided: no per-session tags. Sessions are ephemeral — they can last a minute or a month — so tagging them is not useful. Tags stay on the project (the directory), and every session is tagged indirectly through its project: a Portal session is always linked to the portal project, which carries the user's "personal" tag. This also removes the knock-on that per-session tags would have had on the reboot bugfix (a session-level tag would need restoring across a reboot too); the bugfix stays scoped to the project link. Rejected path: project tags as a base with session tags layered on top (effective tags = project's plus the session's own), proposed as a way to tag what a session is for and to tag sessions whose folder is not a project — rejected on the ephemerality argument.

Decided: a session's project is the git root of where the session was started, and a worktree rolls up to the project it belongs to. The user's workspace pattern falls out of this naturally: ~/Code/fabric/folio holds three independent repositories (api, app, sdk) and is itself a git repository (it carries workspace setup, docs and the workflows); the user always starts sessions at the folio level, so those sessions link to folio. A session deliberately started inside folio/api resolves to folio/api, which is registered as a project of its own — accepted.

Checked against the live install: all three worktree sessions sit in worktree folders that have since been deleted (agentic-workflows' pull-request-per-taak and refactor worktrees, switchboard's refusals-and-removal). These sessions were started at their repo's root and later moved into a worktree, so their recorded origin would have kept them under the parent project; it is the post-reboot guess from the pane's current folder that sends them to Unknown. The reboot bugfix therefore covers the common worktree case. Two precisions for the remaining case: a worktree's own git root is the worktree folder, not the parent repo, so "roll up to the git root" taken literally lands on the worktree — the rule has to be "the repository the worktree belongs to"; and once a worktree is deleted git can no longer be asked, so the project has to be decided while the session's folder still exists (at creation) and kept.

Open: sessions in folders that are not registered projects. ~/Code/flowx is a separate repository from the registered ~/Code/fabric/flowx (not a symlink), and ~/Code/goosemates is a repository with no project record. The flowx-MSy3gd, flowx-x0DHXP and goosemates-2DAjC4 session names carry Portal's generated suffix, so Portal created them, and Portal registers a project when it creates a session — yet neither folder is registered. Cause unknown; possibly a second bug. Whether Portal should register a session's folder as a project when it finds none is not yet decided.

Correction, from Portal's own log: the goosemates gap is not a bug. The session was created on 2026-09-26 in ~/Code/homeos, and Portal saved a project "homeos" at that path. The user later renamed the folder (HomeOS became Goosemates) and the session with it. The saved project then pointed at a folder that no longer existed, and Portal's stale-project housekeeping removes such records — it ran on 2026-09-28 and removed one entry (the summary line does not name it, but the timing fits). The session kept running with no project. ~/Code/flowx predates the oldest retained log (2026-09-08), so its history cannot be read; it is a separate repository from the registered ~/Code/fabric/flowx. No bug to log.

What the goosemates case surfaces for this epic: renaming or moving a project's folder silently drops the project and every tag on it — housekeeping removes the record, and its summary line records only a count, so neither the project nor its tags can be recovered from the log. Tags carry more weight once this epic makes them the basis of filtering, so what happens to a project and its tags when its folder moves or disappears is an impacted area.

The user framed the rest — how the filters behave, how they combine with what exists, persistence, bulk tagging's location, the command-line behaviour — as conversations for the discussion phase, not discovery. The remaining discovery job is an impact sweep: what else this work touches, so the topics cover every base.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
