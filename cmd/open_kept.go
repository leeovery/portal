package cmd

import (
	"fmt"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

// keptRestoreFailedCopy is the one line of a kept session open couldn't bring
// back: the session, then why. Its entry stays kept, for the next restore or
// another try.
const keptRestoreFailedCopy = "Portal couldn't restore %s — kept for the next restore: %w"

var bringBackFunc = bringBack

// openOr opens result as openResolved does, but for a kept session, which is
// brought back first, or, judged again under bootstrap.lock and kept no
// longer, left for instead, which opens the target as if it had never been
// kept.
func openOr(cmd *cobra.Command, result resolver.QueryResult, command []string, instead func() error) error {
	if kept, ok := result.(*resolver.KeptResult); ok {
		return openKeptOr(cmd, kept.Name, command, instead)
	}
	return openResolved(cmd, result, command)
}

// openKept opens the kept session named name, as a pre-flight judged it:
// brought back, or, kept no longer under bootstrap.lock, the live session of
// that name the session pin finds, if any.
func openKept(cmd *cobra.Command, name string, command []string) error {
	return openKeptOr(cmd, name, command, func() error {
		qr, err := buildQueryResolver(cmd)
		if err != nil {
			return err
		}
		return openPinnedBy(cmd, qr.WithKept(nil), (*resolver.QueryResolver).ResolveSessionPin, name, command)
	})
}

// openKeptOr brings back the kept session named name and attaches it, or,
// when it is kept no longer, runs instead. A kept session is an attach
// target, so a command, which is mint-scoped, is refused before anything is
// brought back.
func openKeptOr(cmd *cobra.Command, name string, command []string, instead func() error) error {
	if len(command) > 0 {
		return NewUsageError(commandAttachOnlyMessage)
	}
	brought, err := bringBackFunc(cmd, name)
	switch {
	case err != nil:
		return fmt.Errorf(keptRestoreFailedCopy, name, err) //nolint:staticcheck // user-facing message per spec
	case brought == restore.NoLongerKept:
		return instead()
	}
	return openSessionFunc(cmd, name)
}

// bringBack brings back the kept session named name through restore's own
// path, under bootstrap.lock, taken as a bootstrap takes it, within its bound
// and with its notice on the command's stderr, so it never races a
// bootstrap's restore. It raises no restoring flag: the session's own
// hydrating flag keeps the saver from capturing it or repaying its entry, and
// a kept entry, owed or from before this boot, is one no saver drops. Raised
// by a run that died, restoring would stand the saver down until the next
// bootstrap, which a warm command never runs.
func bringBack(cmd *cobra.Command, name string) (restore.Brought, error) {
	dir, err := state.Dir()
	if err != nil {
		return 0, err
	}
	restorer, err := keptRestorer(dir)
	if err != nil {
		return 0, err
	}
	ctx := bootstrap.WithProgressEmitter(cmd.Context(), waitNotice(cmd.ErrOrStderr()))
	release, err := bootstrap.Locked{Lock: bootstrapLock(dir), Logger: bootstrapLogger}.Hold(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	return restorer.BringBack(cmd.Context(), name)
}

// keptRestorer is the restorer over the state directory dir that brings back
// a kept session in the backend that creates, running bootstrapExe as its
// helper, as the bootstrap's restore runs it.
func keptRestorer(dir string) (*restore.Restorer, error) {
	b, err := attachingBackend()
	if err != nil {
		return nil, err
	}
	exe, err := bootstrapExe()
	if err != nil {
		return nil, fmt.Errorf("find Portal's own executable: %w", err)
	}
	return restore.New(restore.Config{Backend: b, StateDir: dir, Exe: exe, Logger: restoreLogger})
}
