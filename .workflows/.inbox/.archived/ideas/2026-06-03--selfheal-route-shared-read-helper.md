# Route the residual self-heal hand-rolled read loop through the shared helper

`TestRegisterPortalHooks_SelfHealsKDeepStackLeavingUserHookIntact`
(`internal/tmux/hooks_register_realtmux_test.go:~334-351`) still hand-rolls the
read-per-event → `ParseShowHooks` → fingerprint-filter loop (building a
`[]tmux.HookEntry` on `notifyFingerprint`, then checking `len == 1` and
`portal[0].Command`). Since it only inspects the surviving command string, it is
cleanly expressible via the canonical primitive added in task 3-2:

    cmds := portalEntryCommandsForEvent(t, client, event, notifyFingerprint)
    // then len(cmds) == 1 / cmds[0] == expectedNotifyCommand

This is a residual, not a regression — it was outside task 3-2's named
consolidation targets, and the primitive's `[]string` return was only added by
that task. Routing it through would complete the single-source-of-truth intent
for the test-side read→parse→match body.

Source: review of state-notify-cascade-on-binary-upgrade/state-notify-cascade-on-binary-upgrade (recommendation #5)
