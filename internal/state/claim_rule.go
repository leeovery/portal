package state

import (
	"sync/atomic"
	"time"

	"github.com/leeovery/portal/internal/bootid"
	"github.com/leeovery/portal/internal/sessionindex"
)

// bootTolerance is how long after the boot time read an entry's creation still
// counts as from before the boot. The boot time is worked out against the wall
// clock, which can step after boot, and zmx keeps creations to the second. An
// entry misjudged as from before the boot is adopted, keeping what it holds;
// misjudged the other way it would be replaced, losing it.
const bootTolerance = 10 * time.Second

var bootTime = bootid.BootTime

// warnedNoBootTime holds the warning to one per process: every claim and every
// listing builds the rule, and a boot time that can't be read fails each time.
var warnedNoBootTime atomic.Bool

// ClaimRule is the one rule every claim of a session in the state directory
// dir judges an entry from another creation by: what makes an entry the
// earlier session's, so replaced, rather than one Portal created or restored
// that the live session takes over. Code outside this package builds no other.
//
//   - An entry created before this boot, give or take bootTolerance, was saved
//     for restore.
//   - An entry whose session is being restored, its hydrating or
//     resume-pending flag raised, is adopted. A flag that can't be looked for
//     counts as raised.
//
// A boot time that can't be read gives the zero Rule, under which every claim
// adopts, losing nothing.
func ClaimRule(dir string) sessionindex.Rule {
	booted, err := bootTime()
	if err != nil {
		if !warnedNoBootTime.Swap(true) {
			sessionsLogger.Warn("claim-rule", "op", "claim-rule", "detail", "no boot time to judge entries by, so every claim adopts", "error", err)
		}
		return sessionindex.Rule{}
	}
	return sessionindex.NewRule(booted.Add(bootTolerance), func(name string) bool { return Flagged(dir, name) })
}

// Flagged reports whether the session's hydrating or resume-pending flag is
// raised in the state directory dir, a flag that can't be looked for counting
// as raised: Portal is restoring the session, and what its screen shows is a
// replay or the resume panel, not the session's own.
func Flagged(dir, name string) bool {
	for _, raised := range []func(dir, name string) (bool, error){Hydrating.Exists, ResumePending.Exists} {
		if up, err := raised(dir, name); up || err != nil {
			return true
		}
	}
	return false
}
