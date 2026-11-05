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

// settleKept is what to open for result: a kept session is brought back
// first and opened as the session it then is, or, when it is kept no longer,
// judged again under bootstrap.lock, what again resolves as if it had never
// been kept. A kept session is an attach target, so a command, which is
// mint-scoped, is refused before anything is brought back. Any other result is
// opened as it is.
func settleKept(cmd *cobra.Command, result resolver.QueryResult, command []string, again func() (resolver.QueryResult, error)) (resolver.QueryResult, error) {
	kept, ok := result.(*resolver.KeptResult)
	if !ok {
		return result, nil
	}
	if len(command) > 0 {
		return nil, NewUsageError(commandAttachOnlyMessage)
	}
	brought, err := bringBackFunc(cmd, kept.Name)
	switch {
	case err != nil:
		return nil, fmt.Errorf(keptRestoreFailedCopy, kept.Name, err) //nolint:staticcheck // user-facing message per spec
	case brought == restore.NoLongerKept:
		return again()
	}
	return &resolver.SessionResult{Name: kept.Name, Domain: resolver.DomainKept}, nil
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
