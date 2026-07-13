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

// Resolver maps a host-terminal Identity to the Adapter that opens windows for
// it, applying the full precedence config override → native adapter →
// unsupported. It holds the loaded terminals.json config (the escape-hatch tier)
// and the recipe runner every config-built adapter is wired with; the native
// tier and the unsupported fall-through need no state.
type Resolver struct {
	// Config is the loaded terminals.json escape hatch. An empty (or nil) config
	// means the config tier never matches, so resolution reduces to the Phase 2
	// native → unsupported behaviour.
	Config TerminalsConfig
	// runner is the exec seam threaded into every config-built argv/script
	// adapter. NewResolver defaults it to the production execRecipeRunner;
	// white-box tests swap it for a fake so no real process is ever spawned.
	runner recipeRunner
}

// NewResolver returns a config-aware Resolver over cfg, wired to the production
// recipe runner. cmd/spawn.go builds one from the loaded terminals.json and uses
// its Resolve method as the default resolve seam.
func NewResolver(cfg TerminalsConfig) *Resolver {
	return &Resolver{Config: cfg, runner: &execRecipeRunner{}}
}

// Resolve maps a host-terminal Identity to the Adapter that opens windows for it,
// plus the Resolution describing how the mapping was made. Precedence is config
// override → native adapter → unsupported:
//
//  1. A NULL identity (remote/mosh / no host-local terminal) resolves to
//     unsupported WITHOUT consulting the config tier — even a `*` catch-all
//     config entry must not hijack a NULL identity into a config adapter.
//  2. The config tier (terminals.json) is tried first: its single most-specific
//     matching entry, when it holds a valid recipe, builds a config adapter.
//  3. The native registry is tried next, matched by bundle-id family.
//  4. Everything else — a known-but-undriven terminal, any passthrough/unknown
//     identity — resolves to (nil, ResolutionUnsupported).
//
// A config entry that matches but is invalid (structurally, or a missing/non-exec
// script) is skipped and resolution falls through to the NATIVE tier — never to a
// less-specific config entry (matchConfig already returns the single winner).
//
// Building an adapter never touches tmux or osascript; only OpenWindow does.
func (r *Resolver) Resolve(id Identity) (Adapter, Resolution) {
	if id.IsNull() {
		return nil, ResolutionUnsupported
	}

	if adapter, ok := r.resolveConfig(id); ok {
		return adapter, ResolutionConfig
	}

	for _, entry := range nativeAdapters {
		if MatchesFamily(id.BundleID, entry.family) {
			return entry.build(), ResolutionNative
		}
	}

	return nil, ResolutionUnsupported
}

// resolveConfig tries the terminals.json config tier for id: it matches the
// single most-specific entry, validates its recipe, and builds the matching
// argv/script adapter. It returns ok=false — so Resolve falls through to native —
// on no match, an entry with no `open` capability, a structurally-invalid recipe
// (Task 4.2 already emitted the WARN), or a missing/non-exec script (Task 4.5's
// constructor emits the WARN and returns false).
func (r *Resolver) resolveConfig(id Identity) (Adapter, bool) {
	key, entry, ok := matchConfig(r.Config, id)
	if !ok {
		return nil, false
	}

	recipe, kind, valid := validRecipeForEntry(key, entry)
	if !valid {
		return nil, false
	}

	switch kind {
	case RecipeArgv:
		return &argvRecipeAdapter{template: recipe.Argv, runner: r.runner}, true
	case RecipeScript:
		return newScriptRecipeAdapter(key, recipe.Script, r.runner)
	default:
		return nil, false
	}
}

// ResolveAdapter is a thin zero-config wrapper over Resolver.Resolve, preserving
// the Phase 1/2 free-function entry point: an empty config means resolveConfig
// never matches, so behaviour reduces to native → unsupported.
func ResolveAdapter(id Identity) (Adapter, Resolution) {
	return NewResolver(TerminalsConfig{}).Resolve(id)
}
