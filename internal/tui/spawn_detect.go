package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
)

// TerminalDetector resolves the host terminal's identity for the picker's async
// detection lifecycle. It is the 1-method seam (the picker-side counterpart of
// cmd's TerminalDetector, cmd/spawn_seams.go) that lets the model be driven with a
// fabricated detector — no real tmux, ps, or defaults reads. Production wiring
// passes a *spawn.Detector built once at TUI construction over the shared
// *tmux.Client.
type TerminalDetector interface {
	Detect() spawn.Identity
}

// terminalDetectedMsg carries the resolved host-terminal identity back onto the
// event loop after the async Detect() completes on Bubble Tea's command
// goroutine. Its arm resolves the identity to a Resolution via the injected
// config-aware resolve seam and caches both, so the (later) picker burst and
// banner read the resolution without re-detecting or re-resolving.
type terminalDetectedMsg struct {
	identity spawn.Identity
}

// WithTerminalDetector wires the async host-terminal detection seam. Nil-tolerant:
// a nil detector leaves detection unwired (matching the offline capture harness),
// so maybeDispatchDetectionCmd is a permanent no-op and no terminalDetectedMsg is
// ever produced.
func WithTerminalDetector(d TerminalDetector) Option {
	return func(m *Model) {
		m.detector = d
	}
}

// WithResolve wires the config-aware identity→adapter/resolution seam. It is the
// SAME resolver the multi-target open burst uses (config override → native →
// unsupported), loaded once from terminals.json at TUI construction. The picker never re-injects
// it: the terminalDetectedMsg arm caches the Resolution and the later burst reuses
// the cached value.
func WithResolve(fn spawn.AdapterResolver) Option {
	return func(m *Model) {
		m.resolve = fn
	}
}

// WithInitialDetection seeds the §6 host-terminal detection cache at construction
// with the given identity already resolved — the capture-harness entry point for
// the otherwise async detection lifecycle (production reaches PageSessions and
// dispatches Detect(), never this option). It marks detection resolved, caches the
// identity, AND resolves the ADAPTER + Resolution by running the identity through the
// zero-config spawn.ResolveAdapter — caching BOTH halves of that single resolve (like
// the terminalDetectedMsg arm) keeps detectAdapter and detectResolution in lockstep
// so a later dispatchBurst never disagrees with the gate. Seeding the Resolution (not
// just IsNull) is load-bearing so DetectUnsupported() is true for a non-NULL
// recognised-but-undriven terminal (e.g. com.apple.Terminal) and the proactive §6.2
// banner renders from the first frame. A nil identity is a no-op so omitting the
// option leaves detection unresolved.
func WithInitialDetection(id *spawn.Identity) Option {
	return func(m *Model) {
		if id == nil {
			return
		}
		adapter, resolution := spawn.ResolveAdapter(*id)
		m.detectIdentity = *id
		m.detectAdapter = adapter
		m.detectResolution = resolution
		m.detectResolved = true
	}
}

// maybeDispatchDetectionCmd returns the async detection command exactly once,
// guarded by the detectDispatched latch. It returns nil — dispatching nothing —
// when detection is unwired (nil detector), already dispatched, or the model is
// not on PageSessions. Otherwise it sets the latch and returns a command that runs
// Detect() on Bubble Tea's command goroutine, so detection NEVER blocks Update and
// is NEVER part of the §2.6 first-paint appearance gate.
//
// Pointer receiver: it mutates the detectDispatched latch, and the mutation must
// persist onto the model Update returns. Callers invoke it on the addressable
// Update-local value (Go auto-addresses).
func (m *Model) maybeDispatchDetectionCmd() tea.Cmd {
	if m.detector == nil || m.detectDispatched || m.activePage != PageSessions {
		return nil
	}
	m.detectDispatched = true
	detector := m.detector
	return func() tea.Msg {
		return terminalDetectedMsg{identity: detector.Detect()}
	}
}

// DetectDispatched reports whether the async detection command has been
// dispatched (the once-only latch), for testing.
func (m Model) DetectDispatched() bool { return m.detectDispatched }

// DetectResolved reports whether the detected identity has been resolved and
// cached (the terminalDetectedMsg arm ran), for testing.
func (m Model) DetectResolved() bool { return m.detectResolved }

// DetectedIdentity returns the cached detected host-terminal identity, for
// testing. Zero (NULL) until DetectResolved is true.
func (m Model) DetectedIdentity() spawn.Identity { return m.detectIdentity }

// DetectedResolution returns the cached resolution classification of the detected
// identity, for testing. Empty until DetectResolved is true.
func (m Model) DetectedResolution() spawn.Resolution { return m.detectResolution }

// DetectUnsupported is the single "this terminal cannot spawn host windows"
// predicate: true only once detection has resolved AND the resolution is
// unsupported. It is TRUE for a NULL remote/mosh identity AND a non-NULL
// recognised-but-undriven identity (e.g. com.apple.Terminal), so IsNull() alone
// is NOT the test — a recognised-but-undriven terminal is non-NULL yet resolves
// unsupported. FALSE for a native or config-driven identity, and FALSE while
// detection is still in flight.
func (m Model) DetectUnsupported() bool {
	return m.detectResolved && m.detectResolution == spawn.ResolutionUnsupported
}
