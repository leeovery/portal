package spawn

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
)

var spawnLogger = log.For("spawn")

const (
	msgDetectionResolved   = "detection resolved host terminal"
	msgDetectionNullBundle = "detection resolved no host-local terminal"
	msgDetectionUnsure     = "detection could not tell the host terminal"
	msgDetectionTransient  = "detection transient failure"
)

const (
	routeInside       = "inside-session"
	routeInsideRunner = "inside-session runner walk"
	routeInsideRaw    = "inside-session raw client walk"
	routeOutside      = "outside-session env/self walk"
)

// detectCeiling bounds what detection inside a session asks of zmx and of
// the runner registry, each of which has its own, shorter, ceilings beneath.
const detectCeiling = 5 * time.Second

// SessionReader is what detection reads of the backend: the session this
// process runs in, and the tracked environment of the client leading it.
type SessionReader interface {
	Current() string
	LeaderEnv(ctx context.Context, name string) (backend.ClientEnv, error)
}

// RunnerLocator finds a session's leading attach runner by the token its
// client carries: runnerreg.Registry.
type RunnerLocator interface {
	ConfirmSession(ctx context.Context, token, session string) (pid int, err error)
}

// ClientLister finds the zmx attach processes of a session no runner leads,
// failing with backend.ErrUnreadableClient when a process that might be one
// can't be read: the zmx driver.
type ClientLister interface {
	AttachClients(session string) ([]backend.ClientProcess, error)
}

type Detector struct {
	sessions SessionReader
	runners  RunnerLocator
	clients  ClientLister
	getenv   func(string) string
	selfPID  int
	walker   ProcessWalker
	reader   BundleReader
	logger   *slog.Logger
}

func NewDetector(sessions SessionReader, runners RunnerLocator, clients ClientLister) *Detector {
	return &Detector{
		sessions: sessions,
		runners:  runners,
		clients:  clients,
		getenv:   os.Getenv,
		selfPID:  os.Getpid(),
		walker:   realProcessWalker{},
		reader:   realBundleReader{},
		logger:   spawnLogger,
	}
}

// Detect emits exactly one spawn-component record. A transient failure, and a
// session whose leading terminal can't be told, fold to the unsure NULL
// identity — the same unsupported no-op path as a clean NULL, worded apart —
// the first with a WARN, the second with an INFO of its own saying why.
func (d *Detector) Detect() Identity {
	id, route, err := d.resolve()

	switch {
	case errors.Is(err, ErrDetectTransient):
		d.logger.Warn(msgDetectionTransient, "detail", err.Error())
		return Identity{Unsure: true}
	case err != nil:
		d.logger.Info(msgDetectionUnsure, "detail", route+": "+err.Error())
		return Identity{Unsure: true}
	case !id.IsNull():
		d.logger.Info(msgDetectionResolved, "terminal", id.Name, "bundle_id", id.BundleID, "detail", route)
		return id
	default:
		d.logger.Info(msgDetectionNullBundle, "detail", route)
		return Identity{}
	}
}

// resolve's errors either satisfy errors.Is(err, ErrDetectTransient) or
// errors.Is(err, errUncertain).
func (d *Detector) resolve() (Identity, string, error) {
	session := d.sessions.Current()
	if session == "" {
		id, err := detectOutsideSession(d.getenv, d.selfPID, d.walker, d.reader)
		return id, routeOutside, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), detectCeiling)
	defer cancel()
	pid, route, err := d.leaderPID(ctx, session)
	if err != nil {
		return Identity{}, route, err
	}
	id, err := walkToBundle(pid, d.walker, d.reader)
	return id, route, err
}
