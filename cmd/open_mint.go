package cmd

import (
	"context"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/sessionindex"
)

// indexingCreator mints sessions through the backend and claims each one's
// entry in the session index. A minted session exists whatever the index
// holds, so a failed claim warns and the mint still succeeds.
type indexingCreator struct {
	ctx    context.Context
	minter *session.SessionCreator
	via    string
}

func newIndexingCreator(ctx context.Context, store session.ProjectStore, b backend.Backend, via string) indexingCreator {
	return indexingCreator{
		ctx:    ctx,
		minter: session.NewSessionCreator(&resolverAdapter{}, store, b, nanoid.NewGenerator()),
		via:    via,
	}
}

func (c indexingCreator) CreateFromDir(dir string, command []string) (string, error) {
	created, err := c.minter.Create(c.ctx, dir, command)
	if err != nil {
		return "", err
	}
	err = updateSessionIndex(c.via, func(idx *sessionindex.Index) error {
		idx.Claim(created.Name, created.Dir, created.At, created.PID)
		return nil
	})
	// The store warns of a failed write itself; one before it, taking the
	// lock or reading the index, is the caller's to report.
	if err != nil && !fileutil.IsWriteFailure(err) {
		indexLogger.Warn("unrecorded", "op", "unrecorded", "session", created.Name, "via", c.via, "error", err)
	}
	return created.Name, nil
}
