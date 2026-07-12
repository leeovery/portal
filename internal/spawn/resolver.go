package spawn

// Resolution classifies how ResolveAdapter mapped a host-terminal Identity to an
// Adapter (or to no adapter). Its string values are exactly the closed
// `resolution` log-attr vocabulary (config | native | unsupported), so the spawn
// pipeline logs a Resolution directly with no translation.
type Resolution string

const (
	// ResolutionConfig — the adapter came from a terminals.json config override.
	// This is the Phase-4 tier; Phase 2 never returns it.
	ResolutionConfig Resolution = "config"
	// ResolutionNative — the adapter is a built-in native driver matched by
	// bundle-id family.
	ResolutionNative Resolution = "native"
	// ResolutionUnsupported — no adapter is available for this identity (a NULL
	// identity, a known-but-undriven terminal, or a passthrough/unknown one).
	ResolutionUnsupported Resolution = "unsupported"
)

// nativeAdapter is one entry in the ordered native-adapter registry: a bundle-id
// family glob and the constructor that builds its driver on a family hit.
type nativeAdapter struct {
	family string
	build  func() Adapter
}

// nativeAdapters is the ordered native-adapter registry, consulted in order
// after any config override. Phase 2 ships exactly one entry: Ghostty, keyed by
// the family glob so both com.mitchellh.ghostty and any channel-suffixed variant
// resolve.
var nativeAdapters = []nativeAdapter{
	{
		family: "com.mitchellh.ghostty*",
		build:  func() Adapter { return newGhosttyAdapter() },
	},
}

// ResolveAdapter maps a host-terminal Identity to the Adapter that opens windows
// for it, plus the Resolution describing how the mapping was made. Precedence is
// config override (Phase 4) → native adapter → unsupported: a NULL identity, a
// known-but-undriven terminal, and any passthrough/unknown identity all resolve
// to (nil, ResolutionUnsupported).
//
// It is pure — no logging and no I/O. The spawn pipeline (Task 2.6) logs the
// returned Resolution, and constructing a native adapter never touches tmux or
// osascript; only OpenWindow does.
func ResolveAdapter(id Identity) (Adapter, Resolution) {
	if id.IsNull() {
		return nil, ResolutionUnsupported
	}

	// Phase-4 placeholder: a terminals.json config-override lookup runs here
	// first, ahead of the native registry (config → native → unsupported). No
	// config read exists in Phase 2.

	for _, entry := range nativeAdapters {
		if MatchesFamily(id.BundleID, entry.family) {
			return entry.build(), ResolutionNative
		}
	}

	return nil, ResolutionUnsupported
}
