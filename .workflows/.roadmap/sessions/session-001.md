# Roadmap Session 001

Date: 2026-11-06

## Imports (as of session)

- imports/portal-control-plane-plan.md
- imports/001-herdr-prior-art-detection-layers-state-model-agent-shaped-api.md
- imports/001-herdr-prior-art-sidebar-needs-attention-blocked-notifications.md

## Map State at Start

1 horizon · 3 items — 3 waiting

## Exploration

### How Portal is used today

Portal starts and resumes sessions and nothing more: the `x` picker lists
sessions to resume and jump between. Its value is cross-device continuity —
every session runs on the Mac upstairs, and Lee hops into the same session
from the Mac (Ghostty), the iPad, or the phone (Rootshell, connecting with
tssh). Reboot resume — turn the computer off, come back, every session
returns — works well and Lee calls it a godsend. Rootshell is now Lee's
mobile terminal and the Ghostty desktop setup is settled: there is no desire
to build a mobile terminal of Portal's own. Portal is mid-migration from tmux
to zmx (separate work, in its own worktree); nothing from this conversation
starts until that ships.

### What Lee wants Portal to become

A central place for managing sessions that is Lee's own and identical on
every device. Whether Lee enters a session from the phone, the iPad or the
Mac, it is the same system and the same surface — not one tool on the desktop
and Rootshell's features on the phone. Other products offer this kind of
visibility; Lee wants his own so it can be made exactly right for him.

More agent-first: pressing `x` should give a top-level overview of what is
going on — which agents are open, which are working, which need Lee's
attention.

Capabilities named:

- See every session.
- Hide or snooze some sessions.
- Order sessions by recency.
- Order sessions by who needs Lee's attention.
- A redesign: a fresher, more relevant look.

The driving situation: after a workday with a lot running upstairs, Lee comes
down and picks up the iPad or phone. Sometimes the goal is just to hop back
into the sessions that were mid-work and check on them. With 30 sessions in
the picker Lee cannot remember which project needs attention or which one he
was working on, and only some of the many running sessions actually need him.

Lee's stated starting point: talk about use first — how Lee actually works
across devices — to derive what the UI and functionality need to be, and how
they would be reached.

### SitRep and Portal

SitRep (sibling repository, `~/Code/sitrep`) is Lee's board for Claude Code
sessions: each session's status, its asks (what only Lee can do or decide),
notes and parked items. It has a Swift Mac app with a notch at the screen's
edge and a Claude Code panel where asks are answered; a click jumps to the
Ghostty window a session is in.

Options Lee floated, without choosing: bring some of SitRep into Portal, bring
all of it, merge the two products, or keep them separate. Lee is starting to
think Portal could take on what SitRep has, but is torn. A form question rides
on it: Portal may not want to be a desktop app (SitRep is Swift so it can draw
the notch) — or the two belong together. During the zmx work the working split
had been that SitRep covers agent awareness and Portal's in-session surface is
navigation and control only; Lee is reopening that split here.

### Where agent state could come from

Lee recalls a recently released OSC sequence (attributed to Mitchell
Hashimoto's side) that lets a terminal report what an agent is doing; Claude
Code reportedly emits it, and Ghostty may not support it yet. Whether it is
relevant here is unknown, and the sequence itself is not yet identified.
Other candidate sources on the record: SitRep's board, and the per-agent
hooks in the imported June plan.

### Prior work carried in

The June agent-first epic (`agent-first-portal`, created 2026-06-26) was built
from an imported plan framing Portal as an agent-aware control plane over tmux
with its own mobile app, sold as Free plus Pro. It mapped seven topics: agent
state captured on the host; surfacing it; an in-session menu replacing the
Alt+M tmux menu; Portal owning resume registration; a mobile client;
phone-to-Mac transport; productisation. Nothing in it was decided — the
agent-state discussion opened with seven pending subtopics and the surfacing
discussion is an empty stub.

Overtaken since: keeping tmux as the substrate (the zmx move reverses it); the
mobile client and Blink (Rootshell replaces both); the Free/Pro productisation
(Portal is Lee's own tool). Still standing: the agent-state spine (Working /
Waiting / Idle / Unknown, captured from per-agent hooks) and its surfacing (a
state mark on every picker row, a needs-attention view with waiting sessions
on top, an aggregate count in the status line, notifications) — close to what
Lee describes now. The in-session menu and resume-registration ideas also
exist as separate inbox ideas.

Decided: this direction does not rework the June epic in place. Its plan and
the two herdr prior-art notes come into the roadmap as imports, and the June
epic is closed.

The tagging-and-filtering epic (in progress: project-link and tagging
discussions complete, filtering mid-way, command-line filters unstarted) —
Lee wondered whether this work supersedes it, or whether it could be pulled
in. Claude's read, not yet taken up by Lee: it does not compete — tags say
what a session is about, attention and recency say what needs Lee now. The
touch points are display: a redesigned picker has to carry tagging's filter
chips, and the waiting `picker-row-redesign` item (folder beside the name, the
right-hand status strip) covers the same ground as the fresher look.

### Altitude

The conversation sits at product level: the SitRep question decides what
Portal is — one product, two, or one borrowing from the other — before any
single piece of work can be scoped, and Lee wants to talk through use before
anything is committed.

Open threads: how Lee uses sessions across the Mac, iPad and phone; SitRep —
merge, borrow or separate; Portal's form (terminal surface, desktop app, or
both); where agent state comes from; what hiding and snoozing mean; the new
look.

## Edits

- Imported: portal-control-plane-plan.md
- Imported: 001-herdr-prior-art-detection-layers-state-model-agent-shaped-api.md
- Imported: 001-herdr-prior-art-sidebar-needs-attention-blocked-notifications.md

## Items Sorted

(none)

## Conclusion

(none)
