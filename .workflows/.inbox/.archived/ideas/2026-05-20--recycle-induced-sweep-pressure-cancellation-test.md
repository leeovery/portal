# Add recycle-induced sweep pressure cancellation test

Spec §Defect 2 documents a "self-amplifying property": the kill-respawn path itself emits `session-closed` and `session-created` hooks, both of which fire `save.requested` events on the surviving daemon, pushing it into a back-to-back sweep regime. This widens the cancel-to-exit window precisely on the recycle path the barrier is meant to defend. Change 2's ctx-aware loop must remain interruptible under this pressure.

Task 2-5 implementation flagged the sweep-pressure test as optional and did not land it. The current `TestDaemon_MidTickSIGHUP_ExitsWithinBoundedWindow` proves cancellation works under a single-tick scenario; it does not prove cancellation survives the self-amplifying back-to-back-`save.requested` regime that the spec explicitly calls out.

Design call needed:
- How to drive the pressure — looping `os.WriteFile(state.SaveRequested(dir), ...)` from a goroutine during the tick window is the obvious approach, but cadence and total count need a measurement.
- What to assert — daemon still exits within the anchored threshold despite the pressure, or some tighter bound that proves cancellation isn't deferred by the queued saves.

Source: review of saver-kill-respawn-loop-leaks-daemons (Task 2-5 non-blocking note #10).
