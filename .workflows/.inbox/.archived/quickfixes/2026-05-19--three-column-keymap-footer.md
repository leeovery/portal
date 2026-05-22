# Three-column keymap footer on the sessions and projects lists

In the TUI the bottom-of-screen keymap footer on both the sessions list and the projects list currently renders as two columns. On the sessions list the left column is short — Up, Down, Home, End — and the right column carries most of the entries (filter, quit, and the rest, roughly eight lines). On the projects list the right column is similar in length but the left column is taller, most likely because the projects list paginates across multiple pages while the sessions list usually fits on a single page. That pagination difference is just an aside; it is not part of this change.

The change is purely layout. Restructure the footer to render as three columns instead of two. The number of entries per column should be a fixed constant chosen during implementation — somewhere around five looks about right, but the exact value is for the implementer to pick. It must not be dynamic. The third column being a little shorter than the other two if entries don't divide evenly is fine and expected.

No semantic grouping is required. The bindings have no meaningful hierarchy or category from the user's point of view, so the entries should simply be split evenly across the three columns in their current order. Apply the same three-column layout to both the sessions list and the projects list so the two screens stay visually consistent with each other.

All of the bindings being rendered already exist and continue to behave exactly as they do today — this is a footer-rendering layout change only, with no behavioural changes to any keys. The relevant rendering lives in `internal/tui/`, on the sessions-page and projects-page views.
