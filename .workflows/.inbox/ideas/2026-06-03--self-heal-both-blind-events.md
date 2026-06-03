# Parameterise the self-heal real-tmux test over both blind events

`TestRegisterPortalHooks_SelfHealsKDeepStackLeavingUserHookIntact`
(`internal/tmux/hooks_register_realtmux_test.go`) exercises K-deep stack
collapse only on `pane-focus-out`. `window-layout-changed` — the other event
the global read is blind to — has its depth-collapse covered only indirectly
(by `TestRegisterPortalHooks_NoGrowthAcrossBootstraps`, which asserts depth-1
no-growth across N runs, not collapse of a pre-seeded K-deep stack).

Optionally parameterise the self-heal test over both blind events for symmetry,
so a K-deep stack on `window-layout-changed` is directly proven to collapse to
one with a co-resident user hook intact. Not required by any acceptance
criterion — the convergence loop is event-independent — but it would remove the
asymmetry in the regression coverage.

Source: review of state-notify-cascade-on-binary-upgrade/state-notify-cascade-on-binary-upgrade (recommendation #3)
