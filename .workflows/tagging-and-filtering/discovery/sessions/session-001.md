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

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
