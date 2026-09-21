package capture

import (
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// No Text: a modified key is not printable, and a Text-bearing message would
// also match rune arms it must not reach.
func keyPageDown() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}
}

// No Text, for the same reason as keyPageDown: these are not printable, and a
// Text-bearing message would also reach the rune arms they must not.
func keyEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}

// keyLineStart is the text input's own line-start binding.
func keyLineStart() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
}

// ModelAt drives the model to its captured state with no tea program running.
// BootstrapCompleteMsg and LoadingMinElapsedMsg are deliberately never sent:
// loading fixtures stay parked precisely because neither arrives.
func (f *Fixture) ModelAt(th theme.Theme, w, h int) tui.Model {
	var model tea.Model = tui.Build(f.Deps(th))
	w, h = f.renderSize(w, h)
	model, _ = model.Update(tea.WindowSizeMsg{Width: w, Height: h})

	sessions, err := f.Lister.ListSessions()
	model, _ = model.Update(tui.SessionsMsg{Sessions: sessions, Err: err})

	projects, err := f.projectStore.List()
	model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: projects, Err: err})

	for _, event := range f.loadingEvents {
		model, _ = model.Update(event)
	}
	if f.fatalEvent.FailedStep > 0 {
		model, _ = model.Update(f.fatalEvent)
	}
	for _, key := range f.captureKeys {
		model, _ = model.Update(key)
	}
	return model.(tui.Model)
}

// renderSize substitutes whichever dimension the fixture declares for the
// caller's, so every consumer renders a geometry fixture at the size that
// produces its frame without a branch of its own.
func (f *Fixture) renderSize(w, h int) (int, int) {
	return substituteDeclaredSize(f.width, f.height, w, h)
}

// The one substitution rule, shared with the standalone surfaces: a declared
// dimension replaces the caller's, and a zero one takes it.
func substituteDeclaredSize(declaredW, declaredH, w, h int) (int, int) {
	if declaredW > 0 {
		w = declaredW
	}
	if declaredH > 0 {
		h = declaredH
	}
	return w, h
}

// PinRenderSize rewrites a resize to the fixture's declared render size, for a
// live consumer whose window size is the terminal's rather than its own choice.
// Any other message passes through.
func (f *Fixture) PinRenderSize(msg tea.Msg) tea.Msg {
	resize, ok := msg.(tea.WindowSizeMsg)
	if !ok {
		return msg
	}
	resize.Width, resize.Height = f.renderSize(resize.Width, resize.Height)
	return resize
}

// RenderSwapRender renders the fixture under theme a, swaps it live to theme b
// through the production Model.ApplyTheme, and renders again. One model,
// deliberately: cached styles are assigned at construction, so two models would
// each render correctly while live swap was broken.
func (f *Fixture) RenderSwapRender(a, b theme.Theme, w, h int) (before, after string) {
	m := f.ModelAt(a, w, h)
	before = m.View().Content
	m.ApplyTheme(b)
	return before, m.View().Content
}

// Colourless reports whether the fixture renders under the NO_COLOR carve-out,
// read off its own Deps so the exclusion stays structural rather than a
// hand-maintained name list.
func (f *Fixture) Colourless() bool {
	return f.Deps(theme.Theme{}).NoColor
}
