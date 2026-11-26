## Discovery Session 001

Date: 2026-07-27
Work unit: theming-system

### Description (as of session)

Make Portal's Modern Vivid colour-token layer user-themeable — load token
values from config rather than code, ship additional built-in themes, add a
live-preview in-app theme selector, and settle the border token names before
they become the public contract.

### Seed

- seeds/2026-06-17-user-overridable-theme-system.md (inbox:idea)
- seeds/2026-06-21-border-token-role-names.md (inbox:idea)

### Imports

(none)

### Map State at Start

(n/a — single-topic work)

### Exploration

The work arrived as two inbox ideas picked up together: the deferred
user-overridable theme system (external theme file, more built-in themes plus a
selector, contrast-floor validation for user colours, and a documented token
vocabulary) and the rethink of the two border design-token names
(`border.separator` / `border.footer`), which are named after their first
use-site rather than their intrinsic weight. The second idea explicitly flags
that the rename matters most *before* the theme system ships, because the role
token names become the public contract a user themes against.

Lee framed these as a single piece of work from the outset — mostly the theming
system, with the rename folded in — and added a new requirement: ship at least
one additional theme so Portal launches with genuine options rather than a
single built-in.

On how a user actually changes theme, Lee's picture is config-driven at the
base, but with a strong preference for selecting a theme *inside* the app and
seeing the display change immediately. He identified a concrete obstacle
himself: Portal's existing modals blank the whole screen to the canvas, so a
modal-based theme picker would leave nothing visible to preview against. His
proposed answer is a new chrome shape — a TUI equivalent of a web "slide over",
occupying a strip of the right-hand side, where arrowing down the theme list
re-renders the main Portal view live behind it. Whether live re-theming is
technically achievable was raised and deliberately parked for the phase that
should answer it rather than guessed at here.

Custom theme authoring, by contrast, is explicitly *not* a UI concern in Lee's
view — that's a configuration file. Format is undecided and open: YAML, JSON,
TOML, or possibly just a flat key/value list, with a preference expressed for
whatever is least verbose and least prescriptive rather than a heavyweight
schema.

The shape was tested against an epic reading — the theme layer, the live-preview
selector, and the token rename each carry their own decisions, and the selector
is a new interaction pattern rather than a colour change. Lee pushed back: the
token layer already exists and every renderer already reads tokens, so the work
is changing where token values come from and adding the surface to switch them,
not building a theming system from nothing. It would spec as one document even
if run as an epic. That reasoning held — the pieces resolve to a single
deliverable a user experiences as "Portal has themes" — and the work was
committed as a feature.

### Edits

(none)

### Topics Identified

(none)

### Conclusion

Routed to research.
