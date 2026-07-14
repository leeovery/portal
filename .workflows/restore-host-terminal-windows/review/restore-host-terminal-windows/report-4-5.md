TASK: restore-host-terminal-windows-4-5 — Config script recipe adapter

ACCEPTANCE CRITERIA:
- A leading ~ in the script path is expanded via resolver.ExpandTilde before the file is stat'd and executed.
- OpenWindow execs the resolved script path with the composed command string as positional arg 1 (a single positional argument), asserted via the recorded final argv.
- A missing script path -> newScriptRecipeAdapter returns (nil, false) after one spawn WARN naming the entry key (resolver falls through to native).
- A non-executable script (no exec bit) -> (nil, false) after one spawn WARN.
- A clean exit -> Success; a non-zero exit -> SpawnFailed (opaque Detail); an exec error -> SpawnFailed; never PermissionRequired.
- The integration-tagged test execs a real shebang script and confirms success / non-zero mappings + positional arg 1; the unit lane execs no real script.

STATUS: Complete

SPEC CONTEXT:
Spec §"Recipe execution contract" (line 373-374): script recipes receive {command} as $1; Portal expands a leading ~ in the script path and executes the file directly (it must carry its own exec bit + shebang); a missing/non-executable script is an invalid entry (skipped + WARN, falling through to native). §374 + §"Architectural boundary" (399-405): a non-zero exit of the recipe process maps to spawn-failed; permission-required is native-adapter-only — config recipes never produce it (output resembling a permission signal, e.g. -1743/-1712, folds to spawn-failed). §381 covers the earlier structural exactly-one-of check (Task 4.2); this task's missing/non-executable check is a distinct resolution-time filesystem gate.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/configadapter.go:101-154 (newScriptRecipeAdapter + scriptRecipeAdapter). Reuses resolver.ExpandTilde (internal/resolver/path.go:60), recipeRunner seam + mapRecipeResult + renderCommandString (configadapter.go:21-96, recipe.go:94).
- Notes:
  - ~ expansion via resolver.ExpandTilde before os.Stat/exec — correct, single-source-of-truth reuse; resolver does not import spawn (verified: no import cycle).
  - Missing script (os.Stat err) -> one WARN ("script %q not found: %v") naming key + ok=false. Correct.
  - Directory OR no-exec-bit (info.IsDir() || Perm()&0o111==0) -> one WARN ("script %q is not executable") + ok=false. Mode-bit test is root-safe (not an access(2) probe) — correctly reasoned in the doc comment.
  - OpenWindow builds final = [scriptPath, renderCommandString(command)] — script path is argv[0], composed command is the single positional arg[1] delivered directly (no {command} token), execs the file directly so shebang+exec-bit apply. Matches spec.
  - mapRecipeResult (shared) has no permission branch — permission-required is structurally unreachable. Matches §374/§399-405.
  - TOCTOU between resolve-time stat and OpenWindow exec is benign: a file removed/changed after the gate folds to a non-exit exec error -> SpawnFailed via mapRecipeResult. Robust by design.

TESTS:
- Status: Adequate
- Coverage:
  - Unit (internal/spawn/configadapter_script_test.go): ~ expansion + argv[0] resolution (asserts recorded final argv == [expandedPath, commandString]); positional-arg-1 delivery (argv length 2, argv[1]==renderCommandString); missing script -> (nil,false)+exactly one spawn WARN naming key; non-executable 0o644 file -> (nil,false)+one WARN naming key; result mapping (clean->Success, non-zero->SpawnFailed carrying opaque body, exec-error->SpawnFailed); never-permission matrix of 5 cases incl. embedded -1743/-1712 asserting Outcome != PermissionRequired AND Guidance == "".
  - Integration (internal/spawn/configadapter_script_integration_test.go, //go:build integration): real shebang script via production execRecipeRunner + newScriptRecipeAdapter — clean exit -> Success with $1 recorded to a sibling file and read back == renderCommandString; non-zero (exit 3) real script -> SpawnFailed. Real stat/exec-bit gate exercised against a real 0o755 file. Correctly off the unit lane; no tmux/daemon/portal binary.
  - Tests would fail if the feature broke: dropping ~ expansion breaks argv[0] assertion; dropping the stat gate breaks the missing/non-executable subtests; adding a permission branch breaks the never-permission matrix.
- Notes:
  - Minor gap: the info.IsDir() rejection branch is not directly exercised. The 0o644 subtest covers Perm()&0o111==0 but not IsDir(); a directory path (typical 0o755) would pass the exec-bit half and be caught only by IsDir(). If IsDir() were removed, a dir would slip through to a scriptRecipeAdapter and fold to SpawnFailed at exec rather than falling through to native at resolve — the spec-intended resolve-time reject would silently regress with green tests. See non-blocking note.
  - The 5-case never-permission matrix overlaps mapRecipeResult's own direct matrix in the argv sibling test (configadapter_argv_test.go). Justified here: it verifies the script adapter's OpenWindow wiring adds no permission branch, and Guidance-empty is a script-adapter-level assertion. Not over-tested.

CODE QUALITY:
- Project conventions: Followed. WARN uses detectLogger = log.For("spawn"); the key+reason ride the opaque "detail" attr (closed spawn attr set has no entry-key attr) — same "terminals.json entry rejected" message as the structural gate (recipe.go:83), consistent. DI via the recipeRunner seam; compile-time Adapter assertions present.
- SOLID principles: Good. Constructor owns the resolution-time validity gate; adapter owns only exec+map; runner behind a 1-method seam; mapping/detail/render primitives shared, not duplicated.
- Complexity: Low. Two straight-line guards + a two-element argv build.
- Modern idioms: Yes. Perm()&0o111 mode-bit test (root-safe, no access probe), os.Stat symlink-follow, slices.Equal in tests.
- Readability: Good. Doc comments explain the direct-exec rationale, the root-safety of the mode-bit check, and the separation from the structural gate.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/configadapter_script_test.go — add a subtest that passes a directory path (e.g. t.TempDir()) to newScriptRecipeAdapter and asserts (nil, false) + exactly one spawn WARN, so the info.IsDir() rejection branch (configadapter.go:123) is directly covered rather than only the Perm()&0o111==0 half.
