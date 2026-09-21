// Package resumemode holds the closed eager/lazy vocabulary a restored
// registration resolves against: the two on-disk spellings, the recogniser
// that admits those words and nothing else, the shipped install-wide default
// and the resolution of a registration's mode against the install's. It
// depends on the standard library alone, so the hooks store and the prefs
// store can each reach the vocabulary without an edge between them.
package resumemode

// Mode is a resume mode. It is an int kind rather than a string kind because
// the three-state override needs a representable "names no mode" that no
// on-disk value spells, and because an invented literal should not compile.
type Mode int

const (
	// Unset names no mode. A registration holding it inherits the install's;
	// an install holding it falls back to Default.
	Unset Mode = iota
	// Eager fires the hook as restore finishes.
	Eager
	// Lazy holds the hook until the user answers the waiting panel.
	Lazy
)

const (
	eagerSpelling = "eager"
	lazySpelling  = "lazy"
)

// Default is the install-wide mode a registration resolves to when neither it
// nor the install names one.
const Default = Lazy

// String returns the on-disk spelling, and the empty string for Unset or any
// value outside the vocabulary — an unset mode renders as absent rather than
// impersonating one of the two.
func (m Mode) String() string {
	switch m {
	case Eager:
		return eagerSpelling
	case Lazy:
		return lazySpelling
	default:
		return ""
	}
}

// Parse recognises the two on-disk spellings exactly: nothing is trimmed,
// nothing is case-folded and no prefix matches. Every other input, the empty
// string included, answers (Unset, false) — what a caller does with that
// refusal is the caller's policy.
func Parse(s string) (Mode, bool) {
	switch s {
	case eagerSpelling:
		return Eager, true
	case lazySpelling:
		return Lazy, true
	default:
		return Unset, false
	}
}

// Resolve answers the mode a registration restores under: its own if it names
// one, otherwise the install's, otherwise Default. It never answers Unset.
func Resolve(registration, install Mode) Mode {
	if registration != Unset {
		return registration
	}
	if install != Unset {
		return install
	}
	return Default
}
