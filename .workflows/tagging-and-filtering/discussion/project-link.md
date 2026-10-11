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
