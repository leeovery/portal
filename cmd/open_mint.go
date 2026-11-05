package cmd

import (
	"context"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/session"
)

// indexingCreator mints sessions through the backend and claims each one's
// entry in the session index. A minted session exists whatever the index
// holds, so a failed claim warns and the mint still succeeds. One whose
// Create read back no creation is asked for once more; one that still reports
// none is recorded by no claim, and the saver claims it once it reports one.
type indexingCreator struct {
	ctx    context.Context
	minter *session.SessionCreator
	b      backend.Backend
	via    string
	// wait is the wait before that second ask: backend.Pause but in tests.
	wait func(ctx context.Context, d time.Duration) error
}

func newIndexingCreator(ctx context.Context, store session.ProjectStore, b backend.Backend, via string) indexingCreator {
	return indexingCreator{
		ctx:    ctx,
		minter: session.NewSessionCreator(&resolverAdapter{}, store, b, nanoid.NewGenerator()),
		b:      b,
		via:    via,
		wait:   backend.Pause,
	}
}

func (c indexingCreator) CreateFromDir(dir string, command []string) (string, error) {
	created, err := c.minter.Create(c.ctx, dir, command)
	if err != nil {
		return "", err
	}
	live := claimed{Name: created.Name, Dir: created.Dir, Created: created.At, PID: created.PID}
	if live.Created.IsZero() {
		live = c.askAgain(live)
	}
	err = claimSession(c.via, c.b.ValidateName, live, nil, nil)
	// The store warns of a failed write itself; one before it, taking the
	// lock, reading the index or a claim the rule refuses, is the caller's to
	// report.
	if err != nil && !fileutil.IsWriteFailure(err) {
		indexLogger.Warn("unrecorded", "op", "unrecorded", "session", created.Name, "via", c.via, "error", err)
	}
	return created.Name, nil
}

// askAgain fills in the creation of the minted session live from a second
// ask, as restore asks again of a Create that read back none: under load the
// first read can outlast its ceiling. A session the second ask finds with no
// creation is left with none.
func (c indexingCreator) askAgain(live claimed) claimed {
	if s, err := backend.AskAgain(c.ctx, c.b, live.Name, c.wait); err == nil {
		live.Created, live.PID = s.Created, s.PID
	}
	return live
}
