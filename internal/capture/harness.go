package capture

import (
	"cmp"

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
	w, h = f.renderSize(w, h)
	return f.modelAt(f.Deps(th), w, h)
}

// drawing is the fixture's golden frame source, kept at its frame size with any
// declared render size substituted, so the stem names the size it is drawn at.
func (f *Fixture) drawing() Drawing {
	w, h := f.renderSize(cmp.Or(f.frame.Width, harnessSize.Width), cmp.Or(f.frame.Height, harnessSize.Height))
	return Drawing{name: f.name, size: Size{Width: w, Height: h}, draw: f.draw}
}

// colourless joins the fixture's own NO_COLOR, so the environment can take
// colour away but never give it to a colourless fixture.
func (f *Fixture) draw(th theme.Theme, colourless bool, size Size) (tea.View, Size) {
	deps := f.Deps(th)
	deps.NoColor = deps.NoColor || colourless
	w, h := f.renderSize(size.Width, size.Height)
	return f.modelAt(deps, w, h).View(), Size{Width: w, Height: h}
}

// modelAt takes the size as rendered: any declared size already substituted.
func (f *Fixture) modelAt(deps tui.Deps, w, h int) tui.Model {
	var model tea.Model = tui.Build(deps)
	model, _ = model.Update(tea.WindowSizeMsg{Width: w, Height: h})

	sessions, err := f.Lister.ListSessions()
	model, _ = model.Update(tui.SessionsMsg{Sessions: sessions, Pending: pendingSessionsOf(deps.PendingReader), Err: err})

	projects, err := f.projectStore.List()
	model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: projects, Err: err})

	for _, event := range f.loadingEvents {
		model, _ = model.Update(event)
	}
	if f.fatalEvent.FailedStep > 0 {
		model, _ = model.Update(f.fatalEvent)
	}
	var cmd tea.Cmd
	for _, key := range f.captureKeys {
		model, cmd = model.Update(key)
	}
	if f.landsLastKeyRead && cmd != nil {
		model, _ = model.Update(cmd())
	}
	return model.(tui.Model)
}

// Read through the reader the model was built with, so the rows' pending state
// comes off the same seam production's session fetch takes.
func pendingSessionsOf(r tui.PendingResumeReader) map[string]struct{} {
	if r == nil {
		return nil
	}
	pending, err := r.ListPendingResumeSessions()
	if err != nil {
		return nil
	}
	return pending
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
