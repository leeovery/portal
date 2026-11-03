package cmd

import (
	"context"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/sessionindex"
)

var mintIndexLogger = log.For("sessions")

// indexingCreator mints sessions through the backend and records each in the
// session index. A minted session exists whatever the index holds, so a
// failed record warns and the mint still succeeds.
type indexingCreator struct {
	ctx     context.Context
	minter  *session.SessionCreator
	backend backend.Backend
	via     string
}

func newIndexingCreator(ctx context.Context, store session.ProjectStore, b backend.Backend, via string) indexingCreator {
	return indexingCreator{
		ctx:     ctx,
		minter:  session.NewSessionCreator(&resolverAdapter{}, store, b, nanoid.NewGenerator()),
		backend: b,
		via:     via,
	}
}

func (c indexingCreator) CreateFromDir(dir string, command []string) (string, error) {
	created, err := c.minter.Create(c.ctx, dir, command)
	if err != nil {
		return "", err
	}
	at := createdAt(c.ctx, c.backend, created.Name)
	err = updateSessionIndex(c.via, func(idx *sessionindex.Index) error {
		idx.Claim(created.Name, created.Dir, at)
		return nil
	})
	if err != nil {
		mintIndexLogger.Warn("unrecorded", "op", "unrecorded", "session", created.Name, "via", c.via, "error", err)
	}
	return created.Name, nil
}

// createdAt is when the backend reports making the session, zero when it
// reports no time: it lists the session as not answering, or has lost it.
func createdAt(ctx context.Context, b backend.Backend, name string) time.Time {
	sessions, err := b.List(ctx)
	if err != nil {
		return time.Time{}
	}
	for _, s := range sessions {
		if s.Name == name {
			return s.Created
		}
	}
	return time.Time{}
}
