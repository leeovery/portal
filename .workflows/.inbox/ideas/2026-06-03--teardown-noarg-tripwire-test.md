# Add an executable negative test for the teardown no-arg-global-read tripwire

The no-arg-global-read `t.Fatalf` guard (in `perEventDispatchWithFaults`,
`internal/tmux/hooks_register_test.go:128-131`) is now shared by the teardown
dispatch path, so a teardown regression that reverts to the blind no-arg
`show-hooks -g` read fails loudly. However, this is a *structural* tripwire only
— it fires solely on regression, and no test deliberately exercises it on the
teardown path. AC #2 of task 4-2 ("a simulated teardown regression to no-arg
`show-hooks -g` fails the teardown test loudly") is therefore satisfied
structurally but not by an observable assertion.

Optionally add a tiny sub-test that invokes the consolidated teardown dispatcher
with `("show-hooks","-g")` under a recovered / synthetic `*testing.T` and
asserts the fatal path fires — making the guard's coverage executable proof
rather than an inherited invariant.

Source: review of state-notify-cascade-on-binary-upgrade/state-notify-cascade-on-binary-upgrade (recommendation #4)
