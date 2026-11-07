package sourceguardtest

// TestOnlyPackages are the module's test-only packages, by import path: the
// helpers the suites share, which the portal binary must never link. A new
// one joins this list, and with it every guard that reads it.
var TestOnlyPackages = []string{
	"github.com/leeovery/portal/internal/backendtest",
	"github.com/leeovery/portal/internal/capture",
	"github.com/leeovery/portal/internal/harnesstest",
	"github.com/leeovery/portal/internal/hookstest",
	"github.com/leeovery/portal/internal/logtest",
	"github.com/leeovery/portal/internal/portalbintest",
	"github.com/leeovery/portal/internal/portaltest",
	"github.com/leeovery/portal/internal/proctest",
	"github.com/leeovery/portal/internal/sourceguardtest",
	"github.com/leeovery/portal/internal/spawntest",
	"github.com/leeovery/portal/internal/themetest",
	"github.com/leeovery/portal/internal/tmuxtest",
	"github.com/leeovery/portal/internal/zmxtest",
}
