package cmd

import (
	"context"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/session"
)

// indexingCreator mints sessions through the backend and claims each one's
// entry in the session index. A minted session exists whatever the index
// holds, so a failed claim warns and the mint still succeeds; a session that
// reported no creation is recorded by no claim, and the saver claims it once
// it reports one.
type indexingCreator struct {
	ctx    context.Context
	minter *session.SessionCreator
	names  func(string) error
	via    string
}

func newIndexingCreator(ctx context.Context, store session.ProjectStore, b backend.Backend, via string) indexingCreator {
	return indexingCreator{
		ctx:    ctx,
		minter: session.NewSessionCreator(&resolverAdapter{}, store, b, nanoid.NewGenerator()),
		names:  b.ValidateName,
		via:    via,
	}
}

func (c indexingCreator) CreateFromDir(dir string, command []string) (string, error) {
	created, err := c.minter.Create(c.ctx, dir, command)
	if err != nil {
		return "", err
	}
	err = claimSession(c.via, c.names, claimed{Name: created.Name, Dir: created.Dir, Created: created.At, PID: created.PID}, nil, nil)
	// The store warns of a failed write itself; one before it, taking the
	// lock, reading the index or a claim the rule refuses, is the caller's to
	// report.
	if err != nil && !fileutil.IsWriteFailure(err) {
		indexLogger.Warn("unrecorded", "op", "unrecorded", "session", created.Name, "via", c.via, "error", err)
	}
	return created.Name, nil
}
