package capture

import (
	"slices"

	"github.com/leeovery/portal/internal/prefs"
)

// scenarios are every scenario, at the frames' size: the README's demos. Each
// one's comment names the keys its tape, in demo/, presses as its cues come.
func scenarios() []Scenario {
	return []Scenario{
		{
			Name: "portal-cold", Size: harnessSize, world: coldWorld,
			boot: coldBoot(), restoredAt: offset(coldCompletes), histories: demoHistories,
		},
		{Name: "portal-tour", Size: harnessSize, world: tourWorld, histories: demoHistories},
		{Name: "themes", Size: harnessSize, world: tourWorld, histories: demoHistories, themes: previewingThemes},
	}
}

// coldCompletes is when portal-cold's bootstrap completes, its restore landed.
const coldCompletes = 2.9

// coldBoot is portal-cold's bootstrap, as a cold start after a reboot streams
// it: zmx checked at 0.25s; the latch removed, the sweep run and the saver
// started at 0.5, 0.65 and 0.8; the 12 sessions restored one every 0.15s from
// 0.9, 8 of 12 at 1.95, as the loading frame draws it; the restore done at 2.7
// and the latch written at 2.8; and complete at 2.9, the picker then on its
// restored sessions. demo/portal-cold.tape waits for Checking zmx, presses ↓↓
// at 4.5, ␣ at 5.5 to preview kiln-relayx-explore's replayed scrollback, and ↓
// at 8.5 to loom-p84cXa's, and hides at 10.5.
func coldBoot() []bootCue {
	return slices.Concat(
		[]bootCue{checks(0.25), saverStep(0.5, 2), saverStep(0.65, 3), saverStep(0.8, 4)},
		restores(0.9, 0.15, 12),
		[]bootCue{restoreDone(2.7), latchWritten(2.8), completes(coldCompletes)},
	)
}

// coldWorld is the flat sessions page's twelve sessions as a reboot leaves
// them, no client attached to any, three of them waiting to resume once
// restored, so the restored page shows their dots.
func coldWorld() *Fixture {
	fx := sessionsFlatFixture()
	for i := range fx.Lister.sessions {
		fx.Lister.sessions[i].Attached = false
	}
	fx.pendingSessions = []string{"lumen-sync-engine", "orchard-pipeline-codify", "quill-Ue9fLr"}
	return fx
}

// tourWorld is the by-tag page's world, its projects tagged work, client and
// personal and lumen untagged, opened flat, so s steps through every mode.
// demo/portal-tour.tape presses s at 1.5, 3.5 and 5.5, by project, by tag and
// flat again; / at 7, typing a fuzzy term after it; esc at 9.5; ↓↓ at 10.5; ␣
// at 11.5 to preview, ↓ at 13.5 to the next session's, ␣ at 15 back; x at 16 to
// Projects and x at 18 back; and hides at 19.5, a row selected. Over the same
// world, demo/themes.tape presses t to open the theme panel, arrows through
// the six built-ins, each previewed as the cursor reaches it, and closes it
// with esc: Enter would commit, through no persister.
func tourWorld() *Fixture {
	fx := sessionsByTagFixture()
	fx.initialMode = prefs.ModeFlat
	return fx
}

// demoHistories are the scenarios' sessions' scrollback, by name, each its
// project's: generic terminal output, naming no tool.
var demoHistories = map[string]string{
	"orchard-pipeline-code-based": orchardHistory,
	"orchard-pipeline-codify":     orchardHistory,
	"kiln-relayx-explore":         kilnHistory,
	"kiln-aws-migration":          kilnHistory,
	"lumen webhooks and watchers": lumenHistory,
	"lumen-sync-engine":           lumenHistory,
	"tessera-proxy-Vb3kQw":        tesseraHistory,
	"atelier-web-m4TzpD":          atelierHistory,
	"relay-v1-api-Hq7wNe":         relayHistory,
	"relayx-3RJDWA":               relayxHistory,
	"loom-p84cXa":                 loomHistory,
	"quill-Ue9fLr":                quillHistory,
}

const (
	orchardHistory = "$ make pipeline\n" +
		"==> fetching sources\n" +
		"==> transforming 1,284 records\n" +
		"==> loading into the warehouse\n" +
		"pipeline finished in 12.6s\n" +
		"$ git status\n" +
		"On branch main\n" +
		"Your branch is up to date with 'origin/main'.\n" +
		"nothing to commit, working tree clean\n"
	kilnHistory = "$ terraform plan -out=migration.plan\n" +
		"Plan: 4 to add, 2 to change, 0 to destroy.\n" +
		"$ terraform apply migration.plan\n" +
		"aws_s3_bucket.kiln_assets: Creating...\n" +
		"aws_s3_bucket.kiln_assets: Creation complete after 3s\n" +
		"aws_iam_role.kiln_deploy: Modifying...\n" +
		"aws_iam_role.kiln_deploy: Modifications complete after 1s\n" +
		"Apply complete! Resources: 4 added, 2 changed, 0 destroyed.\n"
	lumenHistory = "$ npm test\n" +
		" PASS  src/webhooks/dispatch.test.ts\n" +
		" PASS  src/watchers/debounce.test.ts\n" +
		" PASS  src/sync/engine.test.ts\n" +
		"Tests: 42 passed, 42 total\n" +
		"Time:  3.81 s\n" +
		"$ git push origin sync-engine\n" +
		"   4f2a9c1..b7d03e8  sync-engine -> sync-engine\n"
	tesseraHistory = "$ go test ./...\n" +
		"ok  \ttessera/proxy\t0.412s\n" +
		"ok  \ttessera/proxy/route\t0.188s\n" +
		"ok  \ttessera/proxy/tls\t0.093s\n" +
		"$ ./bin/tessera-proxy --check\n" +
		"config ok · 3 routes · listening on :8080\n"
	atelierHistory = "$ npm run dev\n" +
		"  ready in 612 ms\n" +
		"  Local:   http://localhost:5173/\n" +
		"  Network: use --host to expose\n" +
		"[hmr] updated /src/gallery/Grid.svelte\n" +
		"[hmr] updated /src/gallery/Lightbox.svelte\n"
	relayHistory = "$ curl -s localhost:8080/v1/health\n" +
		"{\"status\":\"ok\",\"version\":\"1.14.2\"}\n" +
		"$ make migrate\n" +
		"applying 0042_add_rate_limits.sql ... done\n" +
		"applying 0043_index_tokens.sql ... done\n" +
		"2 migrations applied\n"
	relayxHistory = "$ cargo build --release\n" +
		"   Compiling relayx v0.3.1 (/home/user/code/relayx)\n" +
		"    Finished release [optimized] target(s) in 41.07s\n" +
		"$ ./target/release/relayx --bench\n" +
		"throughput   18,240 msg/s\n" +
		"p99 latency  3.2 ms\n"
	loomHistory = "$ python -m pytest -q\n" +
		"........................................ [ 83%]\n" +
		"........                                 [100%]\n" +
		"48 passed in 2.31s\n" +
		"$ git log --oneline -3\n" +
		"9e1c4b2 weave: retry failed shuttles\n" +
		"71fa0d8 weave: batch thread writes\n" +
		"c03b5e6 docs: describe the loom file format\n"
	quillHistory = "$ bundle exec rake build\n" +
		"quill 2.4.0 built to pkg/quill-2.4.0.gem.\n" +
		"$ ls pkg\n" +
		"quill-2.4.0.gem\n"
)
