package state

import (
	"errors"
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

// BootTimeEnv names the boot time, in RFC 3339, that an integration build's
// ClaimRule judges by in place of the kernel's: a test can't reboot, so it
// moves the boot past the entries it saved. Nothing in a production build
// reads it.
const BootTimeEnv = "PORTAL_TEST_BOOT_TIME"

var bootTime = sandboxBootTime(bootid.BootTime)

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
//   - A session whose hydrating flag is raised, or can't be looked for, has a
//     restore its helper hasn't finished: no claim repays its entry, and a
//     claim taking it over leaves it owed.
//   - A name names refuses is refused. names is the backend's Create rule, so
//     no entry is kept that restore could never make again. A nil names
//     refuses every name: a caller that claims gives the backend's, and one
//     that only looks entries up, as LookupLive does, needs none.
//
// A boot time that can't be read leaves the rule judging no boot, under which
// every claim adopts, losing nothing; it reads the flags and the names all
// the same.
func ClaimRule(dir string, names func(name string) error) sessionindex.Rule {
	return claimRule(dir, names, warnNoBootTime)
}

// SilentClaimRule is ClaimRule for a reader that claims nothing and reports
// what it finds, as doctor does: a boot time it can't read goes unlogged.
func SilentClaimRule(dir string, names func(name string) error) sessionindex.Rule {
	return claimRule(dir, names, func(error) {})
}

func claimRule(dir string, names func(name string) error, noBootTime func(error)) sessionindex.Rule {
	if names == nil {
		names = refuseEveryName
	}
	return judgedByBoot(dir, noBootTime).
		ReadingHydrating(func(name string) bool { return Hydrating.Raised(dir, name) }).
		RefusingNames(names)
}

// judgedByBoot is the rule's judgement of entries by the boot: the zero Rule
// when the boot time can't be read, which it hands noBootTime.
func judgedByBoot(dir string, noBootTime func(error)) sessionindex.Rule {
	booted, err := bootTime()
	if err != nil {
		noBootTime(err)
		return sessionindex.Rule{}
	}
	return sessionindex.NewRule(booted.Add(bootTolerance), func(name string) bool { return Flagged(dir, name) })
}

func warnNoBootTime(err error) {
	if !warnedNoBootTime.Swap(true) {
		sessionsLogger.Warn("claim-rule", "op", "claim-rule", "detail", "no boot time to judge entries by, so every claim adopts", "error", err)
	}
}

var errNoNameRule = errors.New("no name rule to judge it by")

func refuseEveryName(string) error { return errNoNameRule }

// Flagged reports whether the session's hydrating or resume-pending flag is
// raised in the state directory dir, a flag that can't be looked for counting
// as raised: Portal is restoring the session, and what its screen shows is a
// replay or the resume panel, not the session's own.
func Flagged(dir, name string) bool {
	return Hydrating.Raised(dir, name) || ResumePending.Raised(dir, name)
}
