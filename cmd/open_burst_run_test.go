package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/spf13/cobra"
)

type openBurstEvents struct {
	seq []string
}

type recordingAdapter struct {
	events *openBurstEvents
	inner  *spawntest.FakeAdapter
}

func (a *recordingAdapter) OpenWindow(command []string) spawn.Result {
	a.events.seq = append(a.events.seq, "spawn")
	return a.inner.OpenWindow(command)
}

type recordingConnector struct {
	events *openBurstEvents
	calls  []string
	err    error
}

func (c *recordingConnector) Connect(name string) error {
	c.events.seq = append(c.events.seq, "connect:"+name)
	c.calls = append(c.calls, name)
	return c.err
}

type mintCall struct {
	dir     string
	command []string
}

type recordingMint struct {
	events *openBurstEvents
	calls  []mintCall
	err    error
}

func (m *recordingMint) mint(_ *cobra.Command, dir string, command []string) error {
	m.events.seq = append(m.events.seq, "mint:"+dir)
	m.calls = append(m.calls, mintCall{dir: dir, command: command})
	return m.err
}

// Injected so every recorded argv is deterministic, independent of the
// running binary and the developer's PATH.
const (
	spawnPipelineExe  = "/opt/portal/bin/portal"
	spawnPipelinePATH = "/opt/homebrew/bin:/usr/bin:/bin"
)

func ghosttyIdentity() spawn.Identity {
	return spawn.Identity{Name: "Ghostty", BundleID: "com.mitchellh.ghostty"}
}

// Recognised but undriven: a real name and bundle id, so not the NULL
// identity, yet no native adapter resolves for it.
func appleTerminalIdentity() spawn.Identity {
	return spawn.NewIdentity("com.apple.Terminal", "Apple Terminal")
}

type manualClock struct{ t time.Time }

func (c *manualClock) now() time.Time        { return c.t }
func (c *manualClock) sleep(d time.Duration) { c.t = c.t.Add(d) }

// Ids must be option-safe (alphanumeric) for NewSpawnID to accept them.
func seqIDGen() func() (string, error) {
	var n int
	return func() (string, error) {
		n++
		return fmt.Sprintf("id%d", n), nil
	}
}

type cleanOrderConnector struct {
	ack           *spawntest.FakeAckChannel
	calls         []string
	cleanedBefore []int
}

func (c *cleanOrderConnector) Connect(name string) error {
	c.calls = append(c.calls, name)
	c.cleanedBefore = append(c.cleanedBefore, len(c.ack.Cleaned))
	return nil
}

func openBurstDepsForTest(id spawn.Identity, resolution spawn.Resolution, adapter spawn.Adapter, conn SessionConnector, mint func(*cobra.Command, string, []string) error) *OpenBurstDeps {
	return &OpenBurstDeps{
		Detector:  fakeTerminalDetector{id: id},
		Resolve:   func(spawn.Identity) (spawn.Adapter, spawn.Resolution) { return adapter, resolution },
		Connector: conn,
		ExePath:   func() (string, error) { return spawnPipelineExe, nil },
		Getenv:    func(string) string { return spawnPipelinePATH },
		Ack:       &spawntest.FakeAckChannel{},
		Logger:    nil, // spawn.LogUnsupported is nil-tolerant (log.OrDiscard).
		LocalMint: mint,
	}
}

func withOpenBurster(deps *OpenBurstDeps, inner *spawntest.FakeAdapter, ack *spawntest.FakeAckChannel, clock *manualClock) {
	deps.Ack = ack
	inner.Ack = ack
	deps.NewBurster = func(a spawn.Adapter) *spawn.Burster {
		return &spawn.Burster{
			Adapter: a,
			Ack:     ack,
			Exe:     deps.ExePath,
			Getenv:  deps.Getenv,
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second,
			Poll:    75 * time.Millisecond,
			Now:     clock.now,
			Sleep:   clock.sleep,
		}
	}
}

func surfacesFromArgv(calls [][]string) []spawn.Surface {
	var out []spawn.Surface
	for _, argv := range calls {
		for i := 0; i+1 < len(argv); i++ {
			switch argv[i] {
			case "--session":
				out = append(out, spawn.Surface{Kind: spawn.SurfaceAttach, Value: argv[i+1]})
			case "--path":
				out = append(out, spawn.Surface{Kind: spawn.SurfaceMint, Value: argv[i+1]})
			}
		}
	}
	return out
}

func argvForTarget(calls [][]string, flag, value string) []string {
	for _, argv := range calls {
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] == flag && argv[i+1] == value {
				return argv
			}
		}
	}
	return nil
}

func TestRunOpenBurst_TriggerFirst_ExternalRestInOrder(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "a"},
		{Kind: spawn.SurfaceAttach, Value: "b"},
		{Kind: spawn.SurfaceAttach, Value: "c"},
	}
	if err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantExternal := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "b"},
		{Kind: spawn.SurfaceAttach, Value: "c"},
	}
	if got := surfacesFromArgv(inner.Calls); !slices.Equal(got, wantExternal) {
		t.Errorf("burster external surfaces = %#v, want %#v (surfaces[1:] in order)", got, wantExternal)
	}
	if !slices.Equal(conn.calls, []string{"a"}) {
		t.Errorf("self-connect targets = %#v, want exactly [a] (the first surface)", conn.calls)
	}
	if len(mint.calls) != 0 {
		t.Errorf("LocalMint called %d times, want 0 for an attach trigger", len(mint.calls))
	}
}

func TestRunOpenBurst_TriggerConnectsLast(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
		{Kind: spawn.SurfaceAttach, Value: "e2"},
	}
	if err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"spawn", "spawn", "connect:trig"}
	if !slices.Equal(events.seq, want) {
		t.Errorf("event order = %#v, want %#v (both spawns precede the trigger self-connect)", events.seq, want)
	}
}

func TestRunOpenBurst_TriggerAttach_RoutesToConnector(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
	}
	if err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Errorf("Connector.Connect targets = %#v, want exactly [trig]", conn.calls)
	}
	if len(mint.calls) != 0 {
		t.Errorf("LocalMint called %d times, want 0 for an attach trigger", len(mint.calls))
	}
}

func TestRunOpenBurst_TriggerMint_RoutesToLocalMint(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	withOpenBurster(deps, inner, ack, clock)

	command := []string{"npm run dev"}
	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceMint, Value: "/repo/api"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
	}
	if err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, command, deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mint.calls) != 1 {
		t.Fatalf("LocalMint called %d times, want exactly 1 for a mint trigger", len(mint.calls))
	}
	if mint.calls[0].dir != "/repo/api" {
		t.Errorf("LocalMint dir = %q, want %q", mint.calls[0].dir, "/repo/api")
	}
	if !slices.Equal(mint.calls[0].command, command) {
		t.Errorf("LocalMint command = %#v, want %#v (threaded verbatim)", mint.calls[0].command, command)
	}
	if len(conn.calls) != 0 {
		t.Errorf("Connector.Connect targets = %#v, want none for a mint trigger", conn.calls)
	}
	if len(inner.Calls) != 1 {
		t.Errorf("OpenWindow calls = %d, want 1 (the external e1)", len(inner.Calls))
	}
}

func TestRunOpenBurst_Command_RidesMintExternalsOnly(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	withOpenBurster(deps, inner, ack, clock)

	command := []string{"claude"}
	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},    // trigger: connects bare
		{Kind: spawn.SurfaceMint, Value: "/repo/new"}, // external mint: carries `-- claude`
		{Kind: spawn.SurfaceAttach, Value: "api"},     // external attach: no command
	}
	if err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, command, deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(inner.Calls) != 2 {
		t.Fatalf("OpenWindow calls = %d, want 2 (the two externals)", len(inner.Calls))
	}
	mintArgv := argvForTarget(inner.Calls, "--path", "/repo/new")
	if mintArgv == nil {
		t.Fatalf("no external mint window argv for /repo/new; calls = %#v", inner.Calls)
	}
	dashIdx := slices.Index(mintArgv, "--")
	if dashIdx < 0 {
		t.Fatalf("external mint window argv missing `--`; argv = %#v", mintArgv)
	}
	if rest := mintArgv[dashIdx+1:]; !slices.Equal(rest, command) {
		t.Errorf("external mint post-`--` argv = %#v, want %#v verbatim", rest, command)
	}

	attachArgv := argvForTarget(inner.Calls, "--session", "api")
	if attachArgv == nil {
		t.Fatalf("no external attach window argv for api; calls = %#v", inner.Calls)
	}
	if slices.Contains(attachArgv, "--") {
		t.Errorf("external attach window argv carries the command; argv = %#v", attachArgv)
	}

	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Errorf("self-connect targets = %#v, want exactly [trig]", conn.calls)
	}
	if len(mint.calls) != 0 {
		t.Errorf("LocalMint called %d times, want 0 for an attach trigger", len(mint.calls))
	}
}

func TestRunOpenBurst_AllAttachWithCommand_UsageError(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)

	bursterBuilt := false
	deps.NewBurster = func(spawn.Adapter) *spawn.Burster {
		bursterBuilt = true
		return nil
	}

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "api"},
		{Kind: spawn.SurfaceAttach, Value: "web"},
	}
	err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, []string{"claude"}, deps)

	if err == nil {
		t.Fatal("expected a usage error for an all-attach set carrying a command, got nil")
	}
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Fatalf("error = %T (%v), want *UsageError (exit 2)", err, err)
	}
	if want := "a command (-e/--) can only run in a newly-created session, not an existing one"; err.Error() != want {
		t.Errorf("error = %q, want %q verbatim", err.Error(), want)
	}
	if bursterBuilt {
		t.Error("NewBurster must not be built on the zero-mint-command usage-error path")
	}
	if len(inner.Calls) != 0 {
		t.Errorf("OpenWindow called %d times, want 0 (nothing opens)", len(inner.Calls))
	}
	if len(conn.calls) != 0 {
		t.Errorf("Connector.Connect targets = %#v, want none (nothing self-connects)", conn.calls)
	}
	if len(mint.calls) != 0 {
		t.Errorf("LocalMint called %d times, want 0 (nothing self-connects)", len(mint.calls))
	}
}

func TestRunOpenBurst_DuplicatesHonored_NoDedup(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "a"},
		{Kind: spawn.SurfaceAttach, Value: "a"},
		{Kind: spawn.SurfaceAttach, Value: "b"},
	}
	if err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantExternal := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "a"},
		{Kind: spawn.SurfaceAttach, Value: "b"},
	}
	if got := surfacesFromArgv(inner.Calls); !slices.Equal(got, wantExternal) {
		t.Errorf("burster external surfaces = %#v, want %#v (duplicate honored, no dedup)", got, wantExternal)
	}
	if !slices.Equal(conn.calls, []string{"a"}) {
		t.Errorf("self-connect targets = %#v, want exactly [a]", conn.calls)
	}
}

func runUnsupportedOpenBurstNoOp(t *testing.T, id spawn.Identity) (*spawntest.FakeAdapter, *recordingConnector, *recordingMint, bool, error) {
	t.Helper()
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	deps := openBurstDepsForTest(id, spawn.ResolutionUnsupported, adapter, conn, mint.mint)

	bursterBuilt := false
	deps.NewBurster = func(spawn.Adapter) *spawn.Burster {
		bursterBuilt = true
		return nil
	}

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "a"},
		{Kind: spawn.SurfaceAttach, Value: "b"},
	}
	err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps)
	return inner, conn, mint, bursterBuilt, err
}

func assertOpenBurstAtomicNoOp(t *testing.T, err error, inner *spawntest.FakeAdapter, conn *recordingConnector, mint *recordingMint, bursterBuilt bool) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an atomic no-op error for the unsupported terminal, got nil")
	}
	if bursterBuilt {
		t.Error("NewBurster must not be built on the unsupported no-op path")
	}
	if len(inner.Calls) != 0 {
		t.Errorf("OpenWindow called %d times, want 0 (nothing opens)", len(inner.Calls))
	}
	if len(conn.calls) != 0 {
		t.Errorf("Connector.Connect targets = %#v, want none (trigger must NOT half-connect)", conn.calls)
	}
	if len(mint.calls) != 0 {
		t.Errorf("LocalMint called %d times, want 0 (trigger must NOT half-connect)", len(mint.calls))
	}
}

func TestRunOpenBurst_UnsupportedTerminal_AtomicNoop(t *testing.T) {
	id := appleTerminalIdentity()
	inner, conn, mint, bursterBuilt, err := runUnsupportedOpenBurstNoOp(t, id)

	assertOpenBurstAtomicNoOp(t, err, inner, conn, mint, bursterBuilt)
	if want := spawn.UnsupportedNoopMessage(id); err.Error() != want {
		t.Errorf("error = %q, want %q (names the detected identity)", err.Error(), want)
	}
}

func TestRunOpenBurst_UnsupportedTerminal_CopyIsPlainLanguage(t *testing.T) {
	tests := []struct {
		name string
		id   spawn.Identity
		want string
	}{
		{
			name: "named unsupported terminal",
			id:   appleTerminalIdentity(), // spawn.NewIdentity("com.apple.Terminal", "Apple Terminal")
			want: "can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened",
		},
		{
			name: "null remote identity",
			id:   spawn.Identity{}, // empty BundleID → IsNull() true
			want: "can't open new windows over a remote connection — nothing opened",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner, conn, mint, bursterBuilt, err := runUnsupportedOpenBurstNoOp(t, tt.id)

			assertOpenBurstAtomicNoOp(t, err, inner, conn, mint, bursterBuilt)
			// Byte-literal want, deliberately not the shared renderer — routing it
			// through spawn.UnsupportedNoopMessage would track a copy drift silently.
			if err.Error() != tt.want {
				t.Errorf("error = %q, want the plain-language literal %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRunOpenBurst_PreSpawnBursterError_TriggerNotConnected(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	exeErr := errors.New("executable unresolvable")
	deps.ExePath = func() (string, error) { return "", exeErr }
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "a"},
		{Kind: spawn.SurfaceAttach, Value: "b"},
	}
	err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps)

	if !errors.Is(err, exeErr) {
		t.Fatalf("error = %v, want the pre-spawn executable error", err)
	}
	if len(inner.Calls) != 0 {
		t.Errorf("OpenWindow called %d times, want 0 (pre-spawn abort)", len(inner.Calls))
	}
	if len(conn.calls) != 0 {
		t.Errorf("self-connect targets = %#v, want none (trigger not connected on pre-spawn abort)", conn.calls)
	}
	if len(mint.calls) != 0 {
		t.Errorf("LocalMint called %d times, want 0 (trigger not connected on pre-spawn abort)", len(mint.calls))
	}
}

// Reveals each written token only after a delay, so a burst test can cross a
// timeout check in the poll loop — which an immediate-reveal FakeAckChannel
// cannot force.
type burstDelayingAck struct {
	now      func() time.Time
	delay    time.Duration
	revealAt map[string]map[string]time.Time
	cleaned  []string
}

func newBurstDelayingAck(now func() time.Time, delay time.Duration) *burstDelayingAck {
	return &burstDelayingAck{now: now, delay: delay, revealAt: map[string]map[string]time.Time{}}
}

func (d *burstDelayingAck) Write(batch, token string) error {
	set := d.revealAt[batch]
	if set == nil {
		set = map[string]time.Time{}
		d.revealAt[batch] = set
	}
	set[token] = d.now().Add(d.delay)
	return nil
}

func (d *burstDelayingAck) Collect(batch string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	for token, revealAt := range d.revealAt[batch] {
		if !d.now().Before(revealAt) { // now >= revealAt
			out[token] = struct{}{}
		}
	}
	return out, nil
}

func (d *burstDelayingAck) Clean(batch string) error {
	d.cleaned = append(d.cleaned, batch)
	delete(d.revealAt, batch)
	return nil
}

// Parses the --ack pair out of the composed argv and writes the token,
// simulating the spawned window's marker write; a suppressed write times that
// window out. Needed because FakeAdapter cannot target burstDelayingAck.
type ackWritingAdapter struct {
	ack     spawn.AckWriter
	confirm []bool
	calls   int
}

func (a *ackWritingAdapter) OpenWindow(command []string) spawn.Result {
	i := a.calls
	a.calls++
	if a.confirmed(i) {
		for j := 0; j+1 < len(command); j++ {
			if command[j] == "--ack" {
				if batch, token, ok := spawn.ParseSpawnAckFlag(command[j+1]); ok {
					_ = a.ack.Write(batch, token)
				}
			}
		}
	}
	return spawn.Success("ok")
}

func (a *ackWritingAdapter) confirmed(i int) bool {
	if i < len(a.confirm) {
		return a.confirm[i]
	}
	return true
}

func TestRunOpenBurst_PartialFailure_LeavesOthersOpen_StillConnectsTrigger(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{
		Results: []spawn.Result{spawn.SpawnFailed("osascript exited 1: -1743"), spawn.Success("ok")},
	}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	logger, _ := newCaptureLoggerForComponent(t, "spawn")
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	deps.Logger = logger
	withOpenBurster(deps, inner, ack, clock)

	buf := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetErr(buf)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
		{Kind: spawn.SurfaceAttach, Value: "e2"},
	}
	if err := runOpenBurstWithDeps(cmd, surfaces, nil, deps); err != nil {
		t.Fatalf("partial failure must NOT return an error (trigger-independence); got %v", err)
	}

	if len(inner.Calls) != 2 {
		t.Errorf("OpenWindow calls = %d, want 2 (e1 failed, e2 opened; both attempted)", len(inner.Calls))
	}
	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Errorf("self-connect targets = %#v, want exactly [trig] (trigger connects independent of external failures)", conn.calls)
	}
	want := spawn.PartialFailureMessage([]string{"e1"}, true)
	if got := strings.TrimSpace(buf.String()); got != want {
		t.Errorf("stderr = %q, want the partial-failure summary %q", got, want)
	}
}

func TestRunOpenBurst_TriggerOwnConnectFails_PropagatesError(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	connErr := errors.New("attach session vanished")
	conn := &recordingConnector{events: events, err: connErr}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
	}
	err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps)

	if !errors.Is(err, connErr) {
		t.Fatalf("error = %v, want the trigger's own connect error (skipped only if its own target fails)", err)
	}
}

func TestRunOpenBurst_TriggerConnectFails_EmitsCorrectiveWarn(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	connErr := errors.New("attach session vanished")
	conn := &recordingConnector{events: events, err: connErr}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	logger, sink := newCaptureLoggerForComponent(t, "spawn")
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	deps.Logger = logger
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
	}
	err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps)

	if !errors.Is(err, connErr) {
		t.Fatalf("error = %v, want the trigger's own connect error propagated", err)
	}

	var warns, summaries logtest.Records
	for _, rec := range sink.Records() {
		switch {
		case rec.Level == slog.LevelWarn && rec.Msg == "trigger did not attach":
			warns = append(warns, rec)
		case rec.Level == slog.LevelInfo && strings.HasPrefix(rec.Msg, "opened"):
			summaries = append(summaries, rec)
		}
	}
	w := warns.Only(t, "corrective WARN record")
	if got := w.AttrString(t, "session"); got != "trig" {
		t.Errorf("corrective WARN session attr = %q, want %q (the trigger that did not attach)", got, "trig")
	}
	if got := w.AttrString(t, "detail"); !strings.Contains(got, connErr.Error()) {
		t.Errorf("corrective WARN detail attr = %q, want it to carry the connect error %q", got, connErr.Error())
	}
	_ = summaries.Only(t, "opened batch summary")
}

// An exit status is an attach that landed and has since ended, as a signal
// ends one: the trigger did attach, so no correction follows the summary.
func TestRunOpenBurst_TriggerAttachThatEndsWithAStatus_EmitsNoCorrection(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	ended := &ExitCodeError{Code: 129}
	conn := &recordingConnector{events: events, err: ended}
	mint := &recordingMint{events: events}
	logger, sink := newCaptureLoggerForComponent(t, "spawn")
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	deps.Logger = logger
	withOpenBurster(deps, inner, &spawntest.FakeAckChannel{}, &manualClock{})

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
	}
	err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps)

	if !errors.Is(err, ended) {
		t.Fatalf("error = %v, want the attach's exit status", err)
	}
	if warns := sink.Records().WithMessage("trigger did not attach"); len(warns) != 0 {
		t.Errorf("corrective WARN emitted for an attach that landed: %+v", warns)
	}
}

func TestRunOpenBurst_TriggerMintOwnConnectFails_PropagatesError(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mintErr := errors.New("git root resolution failed")
	mint := &recordingMint{events: events, err: mintErr}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceMint, Value: "/repo/api"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
	}
	err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps)

	if !errors.Is(err, mintErr) {
		t.Fatalf("error = %v, want the trigger's own local-mint error", err)
	}
}

func TestRunOpenBurst_RecordsOutcomesInLog(t *testing.T) {
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	logger, sink := newCaptureLoggerForComponent(t, "spawn")
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	deps.Logger = logger
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
		{Kind: spawn.SurfaceAttach, Value: "e2"},
	}
	if err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var summaries, windows logtest.Records
	for _, rec := range sink.Records() {
		switch {
		case rec.Level == slog.LevelInfo && strings.HasPrefix(rec.Msg, "opened"):
			summaries = append(summaries, rec)
		case rec.Level == slog.LevelDebug && rec.HasAttr("ack"):
			windows = append(windows, rec)
		}
	}
	s := summaries.Only(t, "INFO opened summary")
	if s.Msg != "opened 3/3" {
		t.Errorf("summary msg = %q, want %q (2 externals + trigger / len(surfaces))", s.Msg, "opened 3/3")
	}
	if got := s.AttrString(t, "resolution"); got != "native" {
		t.Errorf("resolution attr = %q, want %q", got, "native")
	}
	if got := s.AttrString(t, "terminal"); got != "Ghostty" {
		t.Errorf("terminal attr = %q, want %q", got, "Ghostty")
	}
	if got := s.AttrString(t, "bundle_id"); got != "com.mitchellh.ghostty" {
		t.Errorf("bundle_id attr = %q, want %q", got, "com.mitchellh.ghostty")
	}
	if got := s.IntAttr(t, "opened"); got != 3 {
		t.Errorf("opened attr = %d, want 3", got)
	}
	if got := s.IntAttr(t, "total"); got != 3 {
		t.Errorf("total attr = %d, want 3 (len(surfaces), N including the trigger)", got)
	}
	if got := s.AttrString(t, "batch"); got == "" {
		t.Errorf("batch attr = %q, want a non-empty batch id", got)
	}
	if len(windows) != 2 {
		t.Fatalf("per-window DEBUG lines with an ack attr = %d, want 2; body:\n%s", len(windows), sink.Body())
	}
	for _, w := range windows {
		if got := w.AttrString(t, "ack"); got != "confirmed" {
			t.Errorf("per-window ack attr = %q, want %q", got, "confirmed")
		}
	}
}

func TestRunOpenBurst_PermissionRequired_GuidanceOnce_StillConnectsTrigger(t *testing.T) {
	const guidance = "grant Automation for Ghostty, then try again"
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{
		Results: []spawn.Result{spawn.Success("ok"), spawn.PermissionRequired("evt -1743", guidance)},
	}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	logger, sink := newCaptureLoggerForComponent(t, "spawn")
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	deps.Logger = logger
	withOpenBurster(deps, inner, ack, clock)

	buf := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetErr(buf)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
		{Kind: spawn.SurfaceAttach, Value: "e2"},
		{Kind: spawn.SurfaceAttach, Value: "e3"},
	}
	if err := runOpenBurstWithDeps(cmd, surfaces, nil, deps); err != nil {
		t.Fatalf("a permission wall on an external window must NOT return an error (trigger-independence); got %v", err)
	}

	if len(inner.Calls) != 2 {
		t.Errorf("OpenWindow calls = %d, want 2 (burst stops at the permission wall; e3 never spawned)", len(inner.Calls))
	}
	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Errorf("self-connect targets = %#v, want exactly [trig] (trigger connects despite the external permission wall)", conn.calls)
	}
	var perms, summaries logtest.Records
	for _, rec := range sink.Records() {
		switch {
		case rec.Level == slog.LevelInfo && rec.Msg == "permission required — nothing self-attached":
			perms = append(perms, rec)
		case rec.Level == slog.LevelInfo && strings.HasPrefix(rec.Msg, "opened"):
			summaries = append(summaries, rec)
		}
	}
	if len(perms) != 1 {
		t.Errorf("permission INFO records = %d, want exactly 1; body:\n%s", len(perms), sink.Body())
	}
	if len(summaries) != 0 {
		t.Errorf("generic opened-summary records = %d, want 0 (permission path skips the batch summary); body:\n%s", len(summaries), sink.Body())
	}
	if got := strings.TrimSpace(buf.String()); got != guidance {
		t.Errorf("stderr = %q, want exactly the driver guidance %q (shown once)", got, guidance)
	}
	if strings.Contains(buf.String(), "-1743") {
		t.Errorf("stderr %q leaks the opaque driver detail; it must ride the log only", buf.String())
	}
}

func TestRunOpenBurst_MarkersCleanedBeforeSelfConnect(t *testing.T) {
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: &openBurstEvents{}, inner: inner}
	ack := &spawntest.FakeAckChannel{}
	conn := &cleanOrderConnector{ack: ack}
	mint := &recordingMint{events: &openBurstEvents{}}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	withOpenBurster(deps, inner, ack, clock)

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
	}
	if err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Fatalf("self-connect targets = %#v, want exactly [trig]", conn.calls)
	}
	if len(conn.cleanedBefore) != 1 || conn.cleanedBefore[0] != 1 {
		t.Errorf("batches cleaned before the self-connect = %#v, want [1] (Clean precedes Connect)", conn.cleanedBefore)
	}
}

func TestRunOpenBurst_PerWindowAckTimeout_TimedFromOwnSpawn(t *testing.T) {
	// e1 never acks and consumes a full timeout budget before e2 spawns, so the
	// timer must start at each window's own spawn. The delay must be >= Poll and
	// < Timeout to force the loop across a timeout check.
	clock := &manualClock{}
	dack := newBurstDelayingAck(clock.now, 200*time.Millisecond)
	adapter := &ackWritingAdapter{ack: dack, confirm: []bool{false, true}}
	conn := &recordingConnector{events: &openBurstEvents{}}
	mint := &recordingMint{events: &openBurstEvents{}}
	logger, sink := newCaptureLoggerForComponent(t, "spawn")
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	deps.Logger = logger
	deps.Ack = dack
	deps.NewBurster = func(a spawn.Adapter) *spawn.Burster {
		return &spawn.Burster{
			Adapter: a,
			Ack:     dack,
			Exe:     deps.ExePath,
			Getenv:  deps.Getenv,
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second,
			Poll:    75 * time.Millisecond,
			Now:     clock.now,
			Sleep:   clock.sleep,
		}
	}

	surfaces := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "trig"},
		{Kind: spawn.SurfaceAttach, Value: "e1"},
		{Kind: spawn.SurfaceAttach, Value: "e2"},
	}
	if err := runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if elapsed := clock.t.Sub(time.Time{}); elapsed < 8*time.Second {
		t.Fatalf("clock advanced only %v, want >= Timeout so the per-window proof is meaningful", elapsed)
	}

	var e1Timeout, e2Confirmed bool
	for _, rec := range sink.Records() {
		if !rec.HasAttr("session") || !rec.HasAttr("ack") {
			continue
		}
		switch rec.AttrString(t, "session") {
		case "e1":
			e1Timeout = rec.AttrString(t, "ack") == "timeout"
		case "e2":
			e2Confirmed = rec.AttrString(t, "ack") == "confirmed"
		}
	}
	if !e1Timeout {
		t.Errorf("want e1 recorded ack=timeout; body:\n%s", sink.Body())
	}
	if !e2Confirmed {
		t.Errorf("want e2 recorded ack=confirmed (judged from its own spawn); body:\n%s", sink.Body())
	}
	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Errorf("self-connect targets = %#v, want exactly [trig]", conn.calls)
	}
}
