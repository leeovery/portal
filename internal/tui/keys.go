package tui

import (
	tea "charm.land/bubbletea/v2"
)

// Bubble Tea v2 key-matching helpers.
//
// In v1, a key event was a struct (tea.KeyMsg) carrying a Type (e.g.
// tea.KeyEsc, tea.KeyRunes) and a Runes []rune slice; the cmd-layer handlers
// matched on msg.Type and string(msg.Runes). In v2, tea.KeyMsg is an interface
// and the concrete press event is tea.KeyPressMsg, whose embedded tea.Key
// carries Code (a rune — the named key like tea.KeyEnter or a printable rune
// like 'a') and Text (the printable characters, empty for special keys).
//
// These helpers preserve the v1 matching SEMANTICS one-to-one so the migration
// is parity-only: keyIsCode is the v1 `msg.Type == tea.KeyX` test for named
// keys, isRuneKey is the v1 `msg.Type == tea.KeyRunes && string(msg.Runes) ==
// ch` test for a single printable rune, and keyIsCtrlC / keyIsCtrlU / keyIsCtrlD
// are the v1 dedicated ctrl-combo key types (no longer distinct Types in v2 —
// they are a Code + ModCtrl pairing).

// keyIsCode reports whether the key press is the named key with the given code
// (e.g. tea.KeyEnter, tea.KeyEscape, tea.KeyTab). It is the v2 replacement for
// the v1 `msg.Type == tea.KeyX` comparison for special/named keys.
func keyIsCode(msg tea.KeyPressMsg, code rune) bool {
	return msg.Code == code && msg.Mod == 0
}

// isRuneKey reports whether msg is a single printable-rune key press matching
// the given character. v2 equivalent of the v1
// `msg.Type == tea.KeyRunes && string(msg.Runes) == ch`. Text carries the
// printable characters (empty for special keys), so a one-character Text equal
// to ch — with no modifiers — is the exact v1 single-rune match.
func isRuneKey(msg tea.KeyPressMsg, ch string) bool {
	return msg.Mod == 0 && msg.Text == ch
}

// keyIsCtrlC reports whether the key press is Ctrl+C — the v2 replacement for
// the v1 `msg.Type == tea.KeyCtrlC`. v2 carries ctrl combos as a base Code plus
// the ModCtrl modifier rather than a dedicated key Type.
func keyIsCtrlC(msg tea.KeyPressMsg) bool {
	return msg.Code == 'c' && msg.Mod.Contains(tea.ModCtrl)
}
