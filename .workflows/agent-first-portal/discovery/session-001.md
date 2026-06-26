# Discovery Session 001

Date: 2026-06-26
Work unit: agent-first-portal

## Description (as of session)

Reposition Portal as an agent-first tool — internal multi-agent session
resume, an in-tmux pop-up overlay, a mobile companion app, and push
notifications for remote agent handoff.

## Seed

(none)

## Imports

(none)

## Map State at Start

(empty — first session)

## Exploration

The user wants to deliberately reverse Portal's standing design stance.
Portal has been agent-agnostic on purpose — its resume mechanism is
generic and its UI/messaging never names Claude (or any tool). The user
now wants to lean *into* agents, making Portal agent-first. They were
explicit that this is an intentional foundational shift, not a tweak.

Two stated motivations: (1) adoption — "agent-first" is a popular framing
right now and likely draws more users; (2) honesty — the user's own
practically-exclusive use of Portal is as a Claude Code session manager /
remote-handoff tool (a tmux wrapper), so the agnostic framing understates
the real use case. Multi-agent, not Claude-only, was emphasised.

Several distinct capability seeds surfaced from one description, and the
user's own framing ("a lot of moving parts") signalled multi-topic scope.
The seeds cluster into a local in-tmux story and a remote story:

- Internal multi-agent resume — fold the agent-session resume system into
  Portal so it's managed for the user, rather than relying on an
  externally-registered hook the user wires up themselves. Spans multiple
  agents, not just Claude Code.
- Portal-as-overlay — a Portal pop-up accessible from inside tmux while an
  agent runs, for actions like detach/quit and renaming sessions/windows/
  panes. Would replace the user's current Alt+M tmux overlay tool. The
  user does not want Portal to reimplement all of tmux — just to surface
  the relevant Portal/tmux actions as a clean overlay.
- Mobile companion app — a terminal app wrapping the Ghostty lib that
  communicates with Portal cleanly, for remote use.
- Push / lock-screen notifications — alerts to the phone when an agent
  needs the user's input or attention.
- Agent-first repositioning — the connective spine: shift Portal's
  product thesis and messaging from agnostic to agent-first.
- (Uncertain) Portal surfaced from inside the agent itself — the user
  floated this and flagged it as "not sure if that's a good idea."

The work-type read converged on an epic — several independently-shippable
features sharing one vision — and the user confirmed it. Topic synthesis
and per-topic routing are deferred to the discovery session loop.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
