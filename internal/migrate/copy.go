package migrate

// Every line portal migrate shows, its refusals included.

// Refusals: a run stops on each, having migrated nothing. stoppedCopy words
// any other failure that stops a run.
const (
	stoppedCopy            = "portal migrate stopped: %v. Once that's fixed, run portal migrate again: it picks up where this run stopped."
	lockedCopy             = "Another portal migrate is running. Wait for it to finish."
	unusableIndexCopy      = "Portal can't migrate: %s can't be read: %v"
	bothStatesCopy         = "Portal can't migrate: %s is still the tmux version's state, but %s is there already from an earlier migration. Nothing was changed. Move one of them aside, then run portal migrate again."
	stagingBesideStateCopy = "Portal can't finish migrating: %s holds the migrated state, but %s has been written since the migration began. Nothing was changed. Keep the one you want as the state dir, move the other aside, then run portal migrate again."
	inTheWayCopy           = "Portal can't finish migrating: %s holds %s, which the migration didn't put there. Nothing was changed. Move it aside, then run portal migrate again; the migrated state is waiting in %s."
	tmuxHooksLeftCopy      = "Portal couldn't remove its tmux hooks, so nothing was migrated: %v. Run portal migrate again."
	tmuxSaverLeftCopy      = "Portal couldn't stop the tmux version's saver (_portal-saver), so nothing was migrated: %v. Run portal migrate again."
	daemonRunningCopy      = "The tmux version's save daemon (pid %d) is still running %s after its saver was stopped%s, so nothing was migrated. Run portal migrate again once it has exited."
	daemonUnreadCopy       = ", or Portal can't tell it has exited (%v)"
	hooksMalformedCopy     = "Portal can't migrate: hooks.json at %s doesn't parse, so nothing was migrated: %v. Fix it, then run portal migrate again."
	unlistedCopy           = "Portal can't migrate: zmx's sessions can't be listed, so nothing was migrated: %v"
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
	unrestoredCopy      = " Couldn't restore %s; kept for the next restore."
	waitsCopy           = "Each one with a resume hook waits on its resume panel the first time you open it."
	noRestoreCopy       = "No restore was owed: the sessions are restored already."
	unfinishedCopy      = "Couldn't restore the sessions yet; the next portal command tries again."
	tmuxDirCopy         = "The tmux version's state is kept at %s."
	backupCopy          = "hooks.json's backup is at %s."
	sessionsHeading     = "Sessions (name ← tmux name):"
	sessionLine         = "  %s ← %s"
	paneSuffix          = " (window %d, pane %d)"
	generatedHeading    = "Generated names:"
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
