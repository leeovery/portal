### Whether x --tag and x --project see sessions whose project link is a guess
*From: project-link · discussion · 2026-10-11*

The project-link discussion decided that a session with no recorded project — one Portal did not create, or one running since before the session-to-project link survived reboots (30 such sessions on the live install on 2026-10-11) — keeps a live guess of its project: the folder its active pane is in now, resolved to the repository it belongs to (a worktree to the repository it was made from). That guess feeds the By Project heading, the session's tags, and the picker's tag and project filters. It never becomes the session's recorded folder: the session row, the picker's text filter and `x /term` match only what was recorded, as the open-with-forced-filter specification decided.

That leaves the command line a choice the picker does not have to make. If `x --tag work` / `x --project folio` read the guess, they pay a per-session pane read (plus a git lookup per distinct folder) on the shell path — the read open-with-forced-filter refused for `/term` because that path has to feel instant, and because on a cold machine the guess has nowhere to come from before the session list is read. If they match recorded links only, an unrecorded session that shows under a tag in the picker drops out of the same tag on the command line, so the two surfaces disagree.

Measured on 2026-10-11: reading the 30 unrecorded sessions' pane folders one at a time took ~0.2s plus ~0.2s of git-root lookups; one listing of every pane took 7ms. The project-link discussion decided that grouped views and filters in the picker read guessed folders for all sessions at once, never one at a time, so the batched cost is the relevant one.

The ask: whether the command-line tag and project filters read guessed project links, and if not, how the disagreement with the picker is presented (if at all).
