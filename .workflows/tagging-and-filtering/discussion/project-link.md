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
