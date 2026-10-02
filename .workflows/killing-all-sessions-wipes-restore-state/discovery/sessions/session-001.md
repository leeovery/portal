# Discovery Session 001

Date: 2026-10-02
Work unit: killing-all-sessions-wipes-restore-state

## Description (as of session)

Rebooting the Mac, killing the tmux server, or killing every session while Portal is running must not empty the saved restore state or delete saved scrollback — the user should be able to do any of these normally without a special Portal shutdown step first.

## Seed

- seeds/2026-08-16-killing-all-sessions-wipes-restore-state.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

The work originates from the inbox bug of the same name, which before promotion was enriched from a project memory recording the tested safe-reboot procedure. The seed describes two routes to the same loss: (1) the daemon's capture tick, once the user's sessions are gone but the server survives hosting Portal's internal sessions, reads "no sessions" as authoritative, commits an empty index over sessions.json and its housekeeping pass deletes every saved scrollback file; (2) the session-closed hook commits synchronously on every session close, so a teardown that ends sessions in sequence would commit a progressively shrinking index and delete each closed session's scrollback as it goes. Route 2 is from a code read only — whether a real reboot or `tmux kill-server` actually ends sessions in sequence while hooks still run is unverified. The only verified-safe reboot today is backup → `portal uninstall` → confirm the final flush → reboot without running `x` → `x` after login.

The user set the target in their own words: a normal reboot of the computer must be safe, and killing tmux directly must be safe — they need to work normally without having to think about safely shutting Portal down before killing tmux or rebooting the Mac. That target spans all three triggers (reboot, kill-server, killing every session) and both routes, rather than only the empty-capture case.

The central tension surfaced for investigation, not resolved here: the per-session save exists so a session the user deliberately kills does not come back on the next restore, so the fix has to distinguish "the user closed this session on purpose" from "tmux itself is going away" — while legitimately closing every session must still eventually persist as empty. The scrollback deletion is the irreversible half and the part to guard most carefully.

Confirmed as a bugfix: restore across a reboot is meant to work already and silently fails under ordinary actions.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
