package restore

import (
	"context"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

// JudgeByBoot has r judge each entry by a boot at booted, in place of the one
// state's claim rule reads from the kernel, so a unit test holds on any host.
func JudgeByBoot(r *Restorer, booted time.Time) {
	r.claimRule = func(string) sessionindex.Rule { return sessionindex.NewRule(booted, nil) }
}

// SleepWith has r wait through sleep in place of a timer, a wait whose
// context has ended returning its cause once sleep returns.
func SleepWith(r *Restorer, sleep func(time.Duration)) {
	r.wait = func(ctx context.Context, d time.Duration) error {
		sleep(d)
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		return nil
	}
}

// ClockWith has r read the time from now, which its retries' budget is
// measured on.
func ClockWith(r *Restorer, now func() time.Time) { r.now = now }
