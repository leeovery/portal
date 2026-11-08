package tui

import (
	tea "charm.land/bubbletea/v2"
)

func keyIsCode(msg tea.KeyPressMsg, code rune) bool {
	return msg.Code == code && msg.Mod == 0
}

func isRuneKey(msg tea.KeyPressMsg, ch string) bool {
	return msg.Mod == 0 && msg.Text == ch
}

func keyIsCtrlC(msg tea.KeyPressMsg) bool {
	return msg.Code == 'c' && msg.Mod.Contains(tea.ModCtrl)
}

// lockMods are the lock keys' states, which a terminal may report on a key
// but which no one presses with it.
const lockMods = tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock

// keypadEnterAsReturn reads the keypad's Enter as Return, so it acts wherever
// Return does with no binding spelling it twice. Bubble Tea always asks for
// the kitty keyboard's disambiguation, under which the keypad's Enter arrives
// as a key of its own (CSI 57414 u), as it does in application keypad mode
// (SS3 M), and Return never carries a lock's state.
func keypadEnterAsReturn(msg tea.Msg) tea.Msg {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || key.Code != tea.KeyKpEnter {
		return msg
	}
	key.Code = tea.KeyEnter
	key.Mod &^= lockMods
	return key
}
