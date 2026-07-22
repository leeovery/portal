package cmd

// Tests for the multi-target open burst body (Task 3-6): runOpenBurstWithDeps with
// an injected OpenBurstDeps. They drive the FIRST-trigger net-N dispatch directly
// (no cobra) with fabricated seams — a fake Detector/Resolve, a recording
// Connector, a spawntest fake adapter/ack-channel wired into a real Burster on a
// manual clock, and a recording LocalMint — so the whole detect → resolve → spawn
// N−1 → self-connect-last flow runs with zero real tmux, osascript, or process
// handoff. MUST NOT use t.Parallel (package cmd mutates package-level state).

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

// Fixed burst-composition inputs: an injected ExePath and PATH so each recorded
// OpenWindow argv is a deterministic, exact env-self-sufficient `open` command
// with no dependence on the running binary or the developer's PATH.
const (
	spawnPipelineExe  = "/opt/portal/bin/portal"
	spawnPipelinePATH = "/opt/homebrew/bin:/usr/bin:/bin"
)

// ghosttyIdentity is the fixed supported host-terminal identity the burst tests
// detect (a real native adapter would resolve for it in production).
func ghosttyIdentity() spawn.Identity {
	return spawn.Identity{Name: "Ghostty", BundleID: "com.mitchellh.ghostty"}
}

// appleTerminalIdentity is a recognised-but-undriven host terminal: it has a real
// friendly name and bundle id (so it is NOT the NULL identity), yet no native
// adapter drives it, so the resolver classifies it unsupported. The N≥2
// atomic-no-op gate must name it in the one-line message.
func appleTerminalIdentity() spawn.Identity {
	return spawn.NewIdentity("com.apple.Terminal", "Apple Terminal")
}

// manualClock is the deterministic fake clock the burster-reaching tests drive:
// now reads the current instant, sleep advances it. No real time passes, so no
// real time.Sleep is ever invoked.
type manualClock struct{ t time.Time }

func (c *manualClock) now() time.Time        { return c.t }
func (c *manualClock) sleep(d time.Duration) { c.t = c.t.Add(d) }

// seqIDGen returns a deterministic id generator yielding "id1", "id2", … — the
// first call is the batch id, each later call a per-window token. Option-safe
// (alphanumeric) so NewSpawnID accepts them and distinct so no ids collide.
func seqIDGen() func() (string, error) {
	var n int
	return func() (string, error) {
		n++
		return fmt.Sprintf("id%d", n), nil
	}
}

// cleanOrderConnector is a SessionConnector that, on each Connect, snapshots how
// many batches the shared ack channel has cleaned so far — letting a test prove
// Clean(batch) ran BEFORE the trigger self-connect on the success path.
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

func TestRunOpenBurst_UnsupportedTerminal_CopyIsPlainLanguage(t *testing.T) {
	// Explicit byte-literal copy regression pinning the rewritten plain-language
	// unsupported no-op strings on the CLI open-burst surface (spec §5 copy set,
	// §7 CLI copy assertions). TestRunOpenBurst_UnsupportedTerminal_AtomicNoop
	// asserts want := spawn.UnsupportedNoopMessage(id) — it SELF-REFERENCES the
	// shared renderer and so silently tracks any wording change. Each `want` here is
	// a hardcoded literal (NOT spawn.UnsupportedNoopMessage(id)), so this test FAILS
	// if the copy drifts from spec §5. It drives the same N≥2 unsupported atomic
	// no-op path for BOTH message shapes: the named terminal AND the NULL/remote
	// identity — closing the gap the named-only existing test leaves uncovered.
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
			events := &openBurstEvents{}
			inner := &spawntest.FakeAdapter{}
			adapter := &recordingAdapter{events: events, inner: inner}
			conn := &recordingConnector{events: events}
			mint := &recordingMint{events: events}
			deps := openBurstDepsForTest(tt.id, spawn.ResolutionUnsupported, adapter, conn, mint.mint)

			// Spy: the burster must never be built on the unsupported no-op path.
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
			// Byte-literal want — NOT spawn.UnsupportedNoopMessage(tt.id) — so a copy
			// drift from spec §5 is caught here rather than silently tracked.
			if err.Error() != tt.want {
				t.Errorf("error = %q, want the plain-language literal %q", err.Error(), tt.want)
			}
			// Atomic no-op is preserved: nothing built, nothing opened, no half-connect.
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
		})
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

// --- Task 3-8: leave-what-opened partial failure + per-window ack timeout + log ---

// burstDelayingAck mirrors internal/spawn's burst_test.go delayingAck at the cmd
// layer: Write records a token with a reveal instant of now()+delay, Collect
// returns only tokens whose reveal instant has arrived, and Clean records swept
// batches. Driving it with the shared manualClock lets a cmd-level burst test prove
// the per-window ack timer starts at each window's OWN spawn (a delay >= Poll and <
// Timeout forces the poll loop to cross a timeout check, which an immediate-reveal
// FakeAckChannel cannot). It satisfies spawn.AckChannelFull (Collect+Clean) and
// spawn.AckWriter (Write).
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

// ackWritingAdapter is a minimal spawn.Adapter that, on each OpenWindow, parses the
// --ack <batch>:<token> pair out of the composed argv and — when that window's
// confirm flag is true (a nil confirm slice, and any index beyond it, confirms) —
// writes the token to an AckWriter, simulating the spawned window's marker write.
// It always reports the adapter open itself as successful, so a suppressed write
// times the window out. It is the cmd-side counterpart of burst_test.go's
// writingAdapter, used only where the burster's Ack is a burstDelayingAck (whose
// concrete type the spawntest.FakeAdapter cannot target).
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
	// External e1 spawn-fails, e2 confirms. Leave-what-opened: e2 stays open, e1 is
	// neither retried nor torn down, and — the open-specific divergence from the
	// picker's burst — the trigger STILL connects and NO partial-failure error is
	// returned. The failed window rides a best-effort stderr summary + portal.log only.
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

	// Both externals were attempted (no early stop, no teardown).
	if len(inner.Calls) != 2 {
		t.Errorf("OpenWindow calls = %d, want 2 (e1 failed, e2 opened; both attempted)", len(inner.Calls))
	}
	// The trigger connects regardless of e1's failure.
	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Errorf("self-connect targets = %#v, want exactly [trig] (trigger connects independent of external failures)", conn.calls)
	}
	// Best-effort stderr partial-failure summary names the failed window; e2 opened,
	// so othersOpened is true.
	want := spawn.PartialFailureMessage([]string{"e1"}, true)
	if got := strings.TrimSpace(buf.String()); got != want {
		t.Errorf("stderr = %q, want the partial-failure summary %q", got, want)
	}
}

func TestRunOpenBurst_TriggerOwnConnectFails_PropagatesError(t *testing.T) {
	// The trigger's OWN connect failing (its attach session vanished between
	// pre-flight and connect) is the SOLE case the trigger is skipped: connectTrigger
	// returns that error and runOpenBurstWithDeps propagates it (outside tmux Portal
	// returns to the shell). Externals all succeed, isolating the own-target failure
	// as the cause.
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
	// The batch summary is emitted just BEFORE the trigger self-connect (a
	// successful outside-tmux attach exec-replaces this process and never returns),
	// so it optimistically counts the trigger's self-attach in `opened`. On the rare
	// connect-failure path the process survives, so a corrective WARN must record
	// that the trigger did NOT attach — otherwise the durable portal.log is left
	// claiming an `opened N/N` that includes a trigger which never landed.
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

	var warns, summaries []logtest.Record
	for _, rec := range sink.Records() {
		switch {
		case rec.Level == slog.LevelWarn && rec.Msg == "trigger did not attach":
			warns = append(warns, rec)
		case rec.Level == slog.LevelInfo && strings.HasPrefix(rec.Msg, "opened"):
			summaries = append(summaries, rec)
		}
	}
	if len(warns) != 1 {
		t.Fatalf("corrective WARN records = %d, want exactly 1; body:\n%s", len(warns), sink.Body())
	}
	w := warns[0]
	if got := w.AttrString(t, "session"); got != "trig" {
		t.Errorf("corrective WARN session attr = %q, want %q (the trigger that did not attach)", got, "trig")
	}
	if got := w.AttrString(t, "detail"); !strings.Contains(got, connErr.Error()) {
		t.Errorf("corrective WARN detail attr = %q, want it to carry the connect error %q", got, connErr.Error())
	}
	// The batch summary above still emitted its optimistic `opened` count — it MUST,
	// since a successful attach never returns to emit it. The corrective WARN is what
	// keeps the durable log honest despite that count.
	if len(summaries) != 1 {
		t.Fatalf("opened batch summaries = %d, want exactly 1; body:\n%s", len(summaries), sink.Body())
	}
}

func TestRunOpenBurst_TriggerMintOwnConnectFails_PropagatesError(t *testing.T) {
	// The mint-trigger analogue: LocalMint failing (the trigger's own local mint
	// errors) propagates as the command's error, even though the external window
	// opened fine.
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
	// Full success: each external window's outcome is recorded in portal.log via the
	// shared logemit helpers — one DEBUG per window carrying its ack, plus one INFO
	// `opened N/N` batch summary (opened = 2 confirmed externals + the trigger; total
	// = len(surfaces) = 3).
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

	var summaries, windows []logtest.Record
	for _, rec := range sink.Records() {
		switch {
		case rec.Level == slog.LevelInfo && strings.HasPrefix(rec.Msg, "opened"):
			summaries = append(summaries, rec)
		case rec.Level == slog.LevelDebug && rec.HasAttr("ack"):
			windows = append(windows, rec)
		}
	}
	if len(summaries) != 1 {
		t.Fatalf("INFO opened summaries = %d, want exactly 1; body:\n%s", len(summaries), sink.Body())
	}
	s := summaries[0]
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
	// A permission wall on an EXTERNAL window stops the burst (later windows never
	// spawn), surfaces the driver's guidance exactly ONCE (LogPermission + one
	// best-effort stderr line, NO batch summary), and — per trigger-independence —
	// STILL connects the trigger with NO error returned.
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

	// Burst stopped at the permission wall (e2): e1,e2 attempted, e3 never spawned.
	if len(inner.Calls) != 2 {
		t.Errorf("OpenWindow calls = %d, want 2 (burst stops at the permission wall; e3 never spawned)", len(inner.Calls))
	}
	// The trigger STILL connects (permission on an external window doesn't cost it its landing).
	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Errorf("self-connect targets = %#v, want exactly [trig] (trigger connects despite the external permission wall)", conn.calls)
	}
	// Exactly one LogPermission INFO and NO batch summary (the burst stopped).
	var perms, summaries []logtest.Record
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
	// Guidance surfaced exactly once on stderr, verbatim, on a single line.
	if got := strings.TrimSpace(buf.String()); got != guidance {
		t.Errorf("stderr = %q, want exactly the driver guidance %q (shown once)", got, guidance)
	}
	// Driver-quarantine: the opaque AppleEvent detail never reaches the stderr line.
	if strings.Contains(buf.String(), "-1743") {
		t.Errorf("stderr %q leaks the opaque driver detail; it must ride the log only", buf.String())
	}
}

func TestRunOpenBurst_MarkersCleanedBeforeSelfConnect(t *testing.T) {
	// The batch @portal-spawn-* markers are cleaned on every post-burst path BEFORE
	// the trigger's self-connect handoff (a point of no return outside tmux). The
	// cleanOrderConnector snapshots how many batches were cleaned at Connect time.
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
	// External e1 never acks (times out, consuming a FULL ~8s budget); e2 acks
	// late-but-in-time. The per-window timer starts at e2's OWN spawn, so e1's spent
	// budget does not eat e2's — e2 still confirms even though the global clock is
	// already past a whole Timeout by e2's spawn. Mirrors internal/spawn's
	// burst_test.go per-window clock proof, driven through the open burst; a delay >=
	// Poll and < Timeout forces the poll loop across a timeout check (an
	// immediate-reveal ack could not distinguish per-window from a global timer).
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

	// The global clock is already past a full Timeout by e2's spawn (e1 consumed it),
	// so e2 confirming proves its budget is judged from its own spawn, not a global timer.
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
	// The trigger still connects.
	if !slices.Equal(conn.calls, []string{"trig"}) {
		t.Errorf("self-connect targets = %#v, want exactly [trig]", conn.calls)
	}
}
