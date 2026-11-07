package migrate

// Every line portal migrate shows, its refusals included.

// Refusals: a run stops on each, having migrated nothing. stoppedCopy words
// any other failure that stops a run.
const (
	stoppedCopy            = "portal migrate stopped: %v.%s Once that's fixed, run portal migrate again%s"
	lockedCopy             = "Another portal migrate is running. Wait for it to finish."
	unusableIndexCopy      = "Portal can't migrate: %s can't be read: %v"
	bothStatesCopy         = "Portal can't migrate: %[1]s is still the tmux version's state, but %[2]s is there already from an earlier migration. Nothing was changed. Move %[1]s aside, and keep %[2]s, from the earlier migration."
	stagingBesideStateCopy = "Portal can't finish migrating: %s holds the migrated state, but %s has been written since the migration began. Nothing was changed. Keep the one you want as the state dir, move the other aside, then run portal migrate again."
	inTheWayCopy           = "Portal can't finish migrating: %s holds %s, which the migration didn't put there. Nothing was changed. Move it aside, then run portal migrate again; the migrated state is waiting in %s."
	migratedStateGoneCopy  = "Portal can't migrate: %[2]s is there from an earlier migration, but %[1]s holds none of the state it migrated. Nothing was changed. Put that state back in %[1]s if you have it; otherwise the next portal command starts with no saved sessions."
	tmuxHooksLeftCopy      = "Portal couldn't remove its tmux hooks, so nothing was migrated: %v. Run portal migrate again."
	tmuxSaverLeftCopy      = "Portal couldn't stop the tmux version's saver (_portal-saver), so nothing was migrated: %v. Run portal migrate again."
	daemonRunningCopy      = "The tmux version's save daemon (pid %d) is still running %s after its saver was stopped%s, so nothing was migrated. Run portal migrate again once it has exited."
	daemonUnstoppedCopy    = "The tmux version's save daemon (pid %d) is running with no saver Portal could stop%s, so nothing was migrated. Run portal migrate again once it has exited."
	daemonUnreadCopy       = ", or Portal can't tell it has exited (%v)"
	hooksMalformedCopy     = "Portal can't migrate: %s doesn't parse, so nothing was migrated: %v.%s Fix it, then run portal migrate again%s"
	unlistedCopy           = "Portal can't migrate: zmx's sessions can't be listed: %v. Nothing was changed."
	hooksChangedCopy       = "Portal can't migrate: %[1]s has changed since an earlier portal migrate backed it up to %[2]s, and migrating rewrites it from that backup, which would lose every hook registered since. Nothing was changed. Keep the hooks you want in %[1]s, move %[2]s aside, then run portal migrate again: it backs up %[1]s afresh."
)

// What stoppedCopy and hooksMalformedCopy add of how far the run they word
// got: whether it stopped the tmux side, then what a re-run does.
const (
	tmuxOffCopy    = " Portal's tmux hooks are off and its tmux saver is stopped, so tmux isn't saving your sessions."
	againCopy      = "."
	startsOverCopy = ": it starts over."
	finishesCopy   = ": it finishes the migration."
)

// How a session's name differs from its tmux session's name, and why,
// each told in the report beside the session.
const (
	replacedOneCopy   = "%s isn't allowed in a session name, so it became '-'"
	replacedManyCopy  = "%s and %s aren't allowed in a session name, so each became '-'"
	leadDroppedCopy   = "a session name can't start with '-', so it was dropped"
	cutCopy           = "too long, so it was cut to %d characters"
	takenCopy         = "%s was taken, so it has a new id"
	numberTooLongCopy = "%s is too long, so it has a new id"
	noPartCopy        = "no part of it is allowed in a session name, so it has a generated name"
	stillRefusedCopy  = "%v, so it has a generated name"
)

// The waiting line: told once, as a run starts to wait for the tmux
// version's daemon to exit.
const waitingCopy = "Waiting for the tmux version's save daemon (pid %d) to save and exit…"

// The report.
const (
	noStateCopy         = "Nothing to migrate: there's no saved state at %s."
	notTmuxCopy         = "Nothing to migrate: the saved state at %s is this version's already."
	migratedCopy        = "Migrated %s to %s."
	noSessionsCopy      = "The tmux version saved no sessions to migrate."
	skippedCountCopy    = " Skipped %s."
	hooksMovedCopy      = "Moved %s."
	hooksRemovedCopy    = " Removed %d, kept in the backup."
	finishedCopy        = "Finished a migration cut short: the migrated state is in place at %s."
	alreadyCopy         = "Already migrated."
	restoredCopy        = "Restored %s."
	stoppedAfterCopy    = "Restored %s, then stopped on the error below."
	stoppedBeforeCopy   = "Couldn't restore the sessions: the error below says why."
	unreadRestoreCopy   = "Couldn't restore the sessions: the warning above says why."
	allRestoredCopy     = "Every saved session is restored."
	leftUnreadCopy      = "Portal can't tell which sessions are left to restore: %v"
	waitsCopy           = "Each migrated session with a resume hook waits on its resume panel the first time you open it."
	leftHeading         = "%s left to restore. x <name> brings one back now; otherwise they come back at the next restore after a reboot:"
	leftStoppedHeading  = "%s left to restore. Once the error below is fixed, x <name> brings one back; otherwise they come back at the next restore after a reboot:"
	leftLine            = "  %s"
	tmuxDirCopy         = "The tmux version's state is kept at %s."
	backupCopy          = "hooks.json's backup is at %s."
	sessionsHeading     = "Sessions (tmux name → name):"
	sessionLine         = "  %s → %s"
	paneSuffix          = " (window %d, pane %d)"
	changedHeading      = "Names made compatible (tmux name → name):"
	changedLine         = "  %s → %s: %s"
	changesSeparator    = "; "
	labelsHeading       = "Labels dropped:"
	skippedHeading      = "Skipped:"
	transcriptsHeading  = "Restored without scrollback:"
	movedHeading        = "Resume hooks moved:"
	movedLine           = "  %s → %s"
	removedHeading      = "Resume hooks removed (kept in the backup):"
	whyLine             = "  %s: %s"
	transcriptGoneCopy  = "its scrollback file, %s, is gone from the tmux version's state"
	paneAddressKeyCopy  = "a pane address, which hasn't fired since pane tokens"
	paneIDKeyCopy       = "a tmux pane id, which hasn't fired since pane tokens"
	unsavedTokenKeyCopy = "the token of a pane the tmux version didn't save"
	otherKeyCopy        = "not a key Portal wrote"
)
