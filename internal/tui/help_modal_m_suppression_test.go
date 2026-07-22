package tui

// persistent-no-host-terminal-banner-2-3 — Help-Modal m-Suppression at the
// Sessions call site (spec §4 / §7).
//
// These white-box (package tui) tests pin the §4 call-site filter: the `m`
// (multi-select) entry is dropped from the descriptor slice fed to the Sessions
// `?` help modal IFF `DetectUnsupported() && !multiSelectMode` — exactly "`m`
// appears in help iff `m` is functional". sessionsKeymap() itself stays a pure
// static constant (the filter lives at the call site via m.sessionsHelpKeymap()),
// so keymap_dispatch_guard_test — which probes the static descriptor with
// detection unwired — stays green. The condensed Sessions footer never lists `m`
// (non-Core) under any resolution.
//
// No t.Parallel: consistent with the rest of the tui test surface (package-level
// mocks + shared canvas helpers).

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tui/theme"
)

// multiSelectHelpLabel is the HelpAction label the `?` help body renders for the
// multi-select (`m`) row (sessionsKeymap()). Kept in one place so the render-level
// assertions cannot drift from the descriptor.
const multiSelectHelpLabel = "Multi-select mode"

// keymapHasMKey reports whether the descriptor slice carries the `m` multi-select
// entry (keyed off keymapEntry.Key, a string glyph).
func keymapHasMKey(entries []keymapEntry) bool {
	for _, e := range entries {
		if e.Key == "m" {
			return true
		}
	}
	return false
}

// TestSessionsHelpKeymap_UnsupportedNotInMultiSelect_OmitsM covers §7 case (a):
// on a resolved-unsupported terminal (both the named com.apple.Terminal shape and
// the NULL/remote spawn.Identity{} shape), NOT in multi-select, the descriptor fed
// to the help modal omits `m` and the rendered help body omits the multi-select
// label. The predicate is DetectUnsupported() — identity-blind — so both shapes
// suppress `m`.
func TestSessionsHelpKeymap_UnsupportedNotInMultiSelect_OmitsM(t *testing.T) {
	tests := []struct {
		name     string
		identity spawn.Identity
	}{
		{"named undriven (com.apple.Terminal)", appleTerminalIdentity()},
		{"NULL remote/mosh (empty identity)", spawn.Identity{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := unsupportedResolvedModel(t, tc.identity)
			if !m.DetectUnsupported() {
				t.Fatalf("precondition: %s must resolve unsupported", tc.name)
			}
			if m.multiSelectMode {
				t.Fatal("precondition: must not be in multi-select mode")
			}

			if keymapHasMKey(m.sessionsHelpKeymap()) {
				t.Error("sessionsHelpKeymap() must OMIT the m entry when unsupported and not in multi-select")
			}

			body := ansi.Strip(helpModalBody(m.sessionsHelpKeymap(), theme.Dark, false))
			if strings.Contains(body, multiSelectHelpLabel) {
				t.Errorf("rendered help body must omit %q when m is blocked:\n%s", multiSelectHelpLabel, body)
			}
		})
	}
}

// TestSessionsHelpKeymap_Supported_ListsM covers §7 case (b): on a supported
// terminal (ghostty → native), DetectUnsupported() is false so the filter is
// inert — sessionsHelpKeymap() lists `m` and the rendered help body carries the
// multi-select label.
func TestSessionsHelpKeymap_Supported_ListsM(t *testing.T) {
	m := unsupportedResolvedModel(t, ghosttyIdentity())
	if m.DetectUnsupported() {
		t.Fatal("precondition: ghostty must resolve native (supported)")
	}

	if !keymapHasMKey(m.sessionsHelpKeymap()) {
		t.Error("sessionsHelpKeymap() must LIST the m entry on a supported terminal (filter inert)")
	}

	body := ansi.Strip(helpModalBody(m.sessionsHelpKeymap(), theme.Dark, false))
	if !strings.Contains(body, multiSelectHelpLabel) {
		t.Errorf("rendered help body must list %q on a supported terminal:\n%s", multiSelectHelpLabel, body)
	}
}

// TestSessionsHelpKeymap_UnsupportedInMultiSelect_ListsM covers §7 case (c): the
// A1 in-flight-entered state — detection resolves unsupported WHILE multi-select is
// already open. Here `m` is a live row-toggle, so the help never hides it. The
// !multiSelectMode leg of the predicate makes the filter inert. Both the named and
// NULL shapes list `m` in this state.
func TestSessionsHelpKeymap_UnsupportedInMultiSelect_ListsM(t *testing.T) {
	tests := []struct {
		name     string
		identity spawn.Identity
	}{
		{"named undriven (com.apple.Terminal)", appleTerminalIdentity()},
		{"NULL remote/mosh (empty identity)", spawn.Identity{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := unsupportedResolvedModel(t, tc.identity)
			if !m.DetectUnsupported() {
				t.Fatalf("precondition: %s must resolve unsupported", tc.name)
			}
			// A1: multi-select was entered during the async in-flight window and is not
			// ejected when detection later resolves unsupported.
			m.multiSelectMode = true

			if !keymapHasMKey(m.sessionsHelpKeymap()) {
				t.Error("sessionsHelpKeymap() must LIST m while in multi-select mode — the help never hides a working row-toggle")
			}

			body := ansi.Strip(helpModalBody(m.sessionsHelpKeymap(), theme.Dark, false))
			if !strings.Contains(body, multiSelectHelpLabel) {
				t.Errorf("rendered help body must list %q while in multi-select mode:\n%s", multiSelectHelpLabel, body)
			}
		})
	}
}

// TestSessionsFooter_NeverListsMultiSelect guards §4 "footer unchanged": `m` is a
// non-Core descriptor entry, so the condensed Sessions footer never lists it —
// under either a supported or an unsupported resolution.
func TestSessionsFooter_NeverListsMultiSelect(t *testing.T) {
	tests := []struct {
		name     string
		identity spawn.Identity
	}{
		{"supported (ghostty → native)", ghosttyIdentity()},
		{"named undriven (com.apple.Terminal)", appleTerminalIdentity()},
		{"NULL remote/mosh (empty identity)", spawn.Identity{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := unsupportedResolvedModel(t, tc.identity)

			footer := ansi.Strip(renderSessionsFooter(sectionHeaderWidth, m.canvasMode, m.colourless))
			if strings.Contains(strings.ToLower(footer), "multi-select") {
				t.Errorf("the condensed Sessions footer must never list m (non-Core):\n%s", footer)
			}
		})
	}
}

// TestSessionsKeymap_StaticConstantUnaffectedByFilter guards §4's core constraint:
// the filter lives only in the call-site copy — sessionsKeymap() itself remains a
// pure static constant that always lists `m`, so the descriptor↔dispatch guard
// (which probes the static descriptor with detection unwired) stays green.
func TestSessionsKeymap_StaticConstantUnaffectedByFilter(t *testing.T) {
	if !keymapHasMKey(sessionsKeymap()) {
		t.Error("sessionsKeymap() must remain a pure static constant that always lists m — the filter belongs at the call site only")
	}
}
