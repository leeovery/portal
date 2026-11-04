package restore

import (
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

// JudgeByBoot has r judge each entry by a boot at booted, in place of the one
// state's claim rule reads from the kernel, so a unit test holds on any host.
func JudgeByBoot(r *Restorer, booted time.Time) {
	r.claimRule = func(string) sessionindex.Rule { return sessionindex.NewRule(booted, nil) }
}

// SleepWith has r wait through sleep in place of time.Sleep.
func SleepWith(r *Restorer, sleep func(time.Duration)) { r.sleep = sleep }
