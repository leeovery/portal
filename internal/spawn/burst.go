package spawn

import (
	"context"
	"time"

	"github.com/leeovery/portal/internal/nanoid"
)

// Per-window is deliberate: each timer starts at its own window's spawn, so the
// cumulative delay of earlier windows never eats a later window's budget.
const spawnAckTimeout = 8 * time.Second

const defaultAckPoll = 75 * time.Millisecond

type AckOutcome string

const (
	AckConfirmed AckOutcome = "confirmed"
	// AckTimeout — the window opened but its token never appeared in time; a
	// missing ack at timeout counts as a failed spawn.
	AckTimeout AckOutcome = "timeout"
	// AckFailed — the adapter reported no window opened, so nothing was awaited.
	AckFailed AckOutcome = "failed"
)

// WindowResult's Session holds the target's value: a session name for an attach
// surface, the literal dir for a mint surface.
type WindowResult struct {
	Session string
	Token   string
	Result  Result
	Ack     AckOutcome
}

// Burster is the N−1 external half of the spawn burst, opening each window
// sequentially. The Nth self-attach is the caller's concern.
type Burster struct {
	Adapter Adapter
	Ack     AckCollector
	Exe     ExecutableResolver
	Env     WindowEnv
	NewID   func() (string, error)
	Timeout time.Duration
	Poll    time.Duration
	Now     func() time.Time
	Sleep   func(time.Duration)
}

// NewBurster applies production defaults for the id generator, timeout, poll
// cadence and clock.
func NewBurster(adapter Adapter, ack AckCollector, exe ExecutableResolver, env WindowEnv) *Burster {
	return &Burster{
		Adapter: adapter,
		Ack:     ack,
		Exe:     exe,
		Env:     env,
		NewID:   nanoid.NewGenerator(),
		Timeout: spawnAckTimeout,
		Poll:    defaultAckPoll,
		Now:     time.Now,
		Sleep:   time.Sleep,
	}
}

// Run opens one external window per surface, in list order, returning the batch
// id and one WindowResult per window. It only spawns each window's `open` argv —
// a mint surface is minted inside its own window at exec time. command
// (nil-tolerant) rides every mint surface byte-identically and attach surfaces
// ignore it; progress (nil-tolerant) fires after each ack classification. The
// executable, the window environment and every id resolve up front, so a
// failure there aborts before any window opens. Cancelling ctx stops the burst and returns what it has collected
// with a nil error.
func (b *Burster) Run(ctx context.Context, external []Surface, command []string, progress func(done, total int)) (batch string, results []WindowResult, err error) {
	exePath, err := b.Exe()
	if err != nil {
		return "", nil, err
	}
	env, err := b.Env()
	if err != nil {
		return "", nil, err
	}

	batch, err = NewSpawnID(b.NewID)
	if err != nil {
		return "", nil, err
	}
	tokens := make([]string, len(external))
	for i := range external {
		token, terr := NewSpawnID(b.NewID)
		if terr != nil {
			return "", nil, terr
		}
		tokens[i] = token
	}

	results = make([]WindowResult, 0, len(external))
	for i, surface := range external {
		if ctx.Err() != nil {
			break
		}
		token := tokens[i]
		argv := composeOpenArgv(exePath, env, surface, batch, token, command)
		result := b.Adapter.OpenWindow(argv)

		ack := AckFailed
		if result.OK() {
			ack = awaitToken(ctx, b, batch, token)
		}
		results = append(results, WindowResult{Session: surface.Value, Token: token, Result: result, Ack: ack})

		if progress != nil {
			progress(i+1, len(external))
		}

		// Early stop: the macOS Automation grant is per-(source, target), so
		// every later window would hit the same permission wall.
		if result.Outcome == OutcomePermissionRequired {
			break
		}
	}
	return batch, results, nil
}

// A Collect error counts as "token not present yet": the loop is timer-bounded,
// so a persistently failing enumeration classifies as AckTimeout.
func awaitToken(ctx context.Context, b *Burster, batch, token string) AckOutcome {
	start := b.Now()
	for {
		if ctx.Err() != nil {
			return AckTimeout
		}
		if tokens, cerr := b.Ack.Collect(batch); cerr == nil {
			if _, ok := tokens[token]; ok {
				return AckConfirmed
			}
		}
		b.Sleep(b.Poll)
		if b.Now().Sub(start) >= b.Timeout {
			return AckTimeout
		}
	}
}
