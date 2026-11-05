package hooks

// Event names the moment a stored hook's command is run, and is the
// second-level key of hooks.json. The vocabulary is closed at this boundary:
// an entry filed under an event Portal never fires is a command nothing will
// ever run, so a caller may not invent one.
//
// It is a string kind rather than an enum over the wire values because the
// value is persisted state rather than a rendering of it — the map index and
// the on-disk key are the same string.
type Event string

// EventOnResume runs the command when a restored session hydrates.
const EventOnResume Event = "on-resume"

// String returns the persisted key.
func (e Event) String() string {
	return string(e)
}
