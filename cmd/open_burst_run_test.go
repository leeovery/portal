package cmd

// Tests for the multi-target open burst body (Task 3-6): runOpenBurstWithDeps with
// an injected OpenBurstDeps. They drive the FIRST-trigger net-N dispatch directly
// (no cobra) with fabricated seams — a fake Detector/Resolve, a recording
// Connector, a spawntest fake adapter/ack-channel wired into a real Burster on a
// manual clock, and a recording LocalMint — so the whole detect → resolve → spawn
// N−1 → self-connect-last flow runs with zero real tmux, osascript, or process
// handoff. MUST NOT use t.Parallel (package cmd mutates package-level state).

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/spf13/cobra"
)

// openBurstEvents is a shared ordered event recorder proving the N−1 external
// spawns precede the trigger self-connect.
type openBurstEvents struct {
	seq []string
}

// recordingAdapter appends "spawn" to the shared event log on each OpenWindow,
// then delegates to the inner FakeAdapter (which records argv + writes the ack
// marker for a confirmed window).
type recordingAdapter struct {
	events *openBurstEvents
	inner  *spawntest.FakeAdapter
}

func (a *recordingAdapter) OpenWindow(command []string) spawn.Result {
	a.events.seq = append(a.events.seq, "spawn")
	return a.inner.OpenWindow(command)
}

// recordingConnector records every self-connect target AND appends "connect:<name>"
// to the shared event log, standing in for the inside/outside-tmux connector.
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

// mintCall captures one LocalMint invocation.
type mintCall struct {
	dir     string
	command []string
}

// recordingMint records every local-mint invocation AND appends "mint:<dir>" to
// the shared event log, standing in for openPath's trigger-is-mint local connect.
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

// openBurstDepsForTest assembles a fully-injected OpenBurstDeps for the burst body:
// the fabricated detector/resolver, the recording connector + local mint, and the
// fixed executable/PATH composition seams. Ack + NewBurster are wired separately by
// withOpenBurster on the paths that reach the burster.
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

// withOpenBurster wires a fake ack channel + manual clock into deps and the inner
// fake adapter so a burst-reaching test drives the whole spawn → confirm flow with
// zero real time, tmux, or osascript: deps.Ack and the adapter's Ack share ack, and
// deps.NewBurster builds a real Burster on the manual clock with a deterministic id
// generator.
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

// surfacesFromArgv reconstructs the ordered external surfaces the burster spawned
// from the recorded OpenWindow argvs (composeOpenArgv is deterministic:
// `--session <name>` for an attach surface, `--path <dir>` for a mint surface). It
// lets a test assert the burster received surfaces[1:] in order, kinds included.
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

// argvForTarget returns the first recorded argv whose flag element (e.g. --path /
// --session) is immediately followed by value, or nil if none matches. It lets a
// test pick out a specific spawned window's argv from the recorded calls.
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
	// surfaces = [a, b, c] → trigger a (first), external [b, c] in order.
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
	// The N−1 external OpenWindow calls must ALL precede the trigger self-connect.
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
	// A mint trigger self-connects via LocalMint(cmd, dir, command) — the command
	// is threaded verbatim (single unit, no word-splitting).
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
	// The external attach window still spawned.
	if len(inner.Calls) != 1 {
		t.Errorf("OpenWindow calls = %d, want 1 (the external e1)", len(inner.Calls))
	}
}

func TestRunOpenBurst_Command_RidesMintExternalsOnly(t *testing.T) {
	// A mixed external set with a command: the external MINT window carries the
	// `-- claude` passthrough tail, the external ATTACH window does not. The
	// attach trigger connects bare (its own target carries no command).
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

	// The attach trigger self-connects bare.
	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Errorf("self-connect targets = %#v, want exactly [trig]", conn.calls)
	}
	if len(mint.calls) != 0 {
		t.Errorf("LocalMint called %d times, want 0 for an attach trigger", len(mint.calls))
	}
}

func TestRunOpenBurst_AllAttachWithCommand_UsageError(t *testing.T) {
	// A multi-target ALL-ATTACH set carrying a command has nowhere to run it (no mint
	// surface), so it is a usage error (exit 2) — the multi-target arity of the
	// single-target attach-command guard. Nothing opens, nothing self-connects.
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)

	// Spy: neither the burster nor any detection may run on the guard path.
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
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("error = %T (%v), want *UsageError (exit 2)", err, err)
	}
	if want := "a command (-e/--) can only run in a newly-created session, not an existing one"; err.Error() != want {
		t.Errorf("error = %q, want %q (Task 2-6's message)", err.Error(), want)
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
	// surfaces = [a, a, b] → trigger a, external [a, b]; the duplicate is honored,
	// never collapsed.
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

func TestRunOpenBurst_UnsupportedTerminal_AtomicNoop(t *testing.T) {
	// A recognised-but-undriven terminal (Apple Terminal → unsupported) at N≥2 is an
	// atomic no-op: nothing opens, the trigger does NOT half-connect, and the error
	// names the identity. The burster is never constructed or run.
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	id := appleTerminalIdentity()
	deps := openBurstDepsForTest(id, spawn.ResolutionUnsupported, adapter, conn, mint.mint)

	// Spy: the burster must never be built on the unsupported path.
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

	if err == nil {
		t.Fatal("expected an atomic no-op error for the unsupported terminal, got nil")
	}
	if want := spawn.UnsupportedNoopMessage(id); err.Error() != want {
		t.Errorf("error = %q, want %q (names the detected identity)", err.Error(), want)
	}
	if bursterBuilt {
		t.Error("NewBurster must not be called on the unsupported no-op path")
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

func TestRunOpenBurst_PreSpawnBursterError_TriggerNotConnected(t *testing.T) {
	// A pre-spawn Burster error (the executable fails to resolve before any window
	// opens) aborts before the self-connect: the error propagates and the trigger
	// is NOT connected.
	events := &openBurstEvents{}
	inner := &spawntest.FakeAdapter{}
	adapter := &recordingAdapter{events: events, inner: inner}
	conn := &recordingConnector{events: events}
	mint := &recordingMint{events: events}
	ack := &spawntest.FakeAckChannel{}
	clock := &manualClock{}
	deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
	// A failing ExePath makes Burster.Run abort at the top, before any OpenWindow.
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
