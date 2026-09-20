# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. The task store holds a de-specified copy of 31 of the plan's 39 tasks

**Severity**: Important
**Plan Reference**: 31 tasks across Phases 1–6 — `lazy-resume-on-attach-1-5`, `-2-3`, `-2-4`, `-2-5`, `-3-1` … `-3-6`, `-4-1` … `-4-7`, `-5-1` … `-5-6`, `-6-1` … `-6-8` (the remaining eight — `-1-1`, `-1-2`, `-1-3`, `-1-4`, `-1-6`, `-1-7`, `-2-1`, `-2-2` — differ only in the backticks around their test names)
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
The implementer never sees the task text that was approved. The implementation flow reads each task out of the tick store through the format adapter (`tick show`) and normalises *that* into the executor's brief; the `phase-{N}-tasks.md` files are not among the executor's inputs. For 31 of the 39 tasks the stored copy is a prose paraphrase of the approved body with roughly 370 code spans removed — exact identifiers, flag spellings, byte values and platform constants replaced by descriptions of them.

The cost is not uniform. In most tasks the lost identifier is recoverable from the surrounding sentence or from the codebase (`indicatorSlotWidth` → "the indicator slot width", `Theme.StatePositive` → "the positive state token"). In a handful it is not, and the worst is task 5-4, the one task whose calls were researched precisely because they are not guessable:

- approved: "darwin issues `unix.IoctlSetPointerInt(fd, unix.TIOCFLUSH, freadOnly)` over a file-local `freadOnly = 0x1`; linux issues `unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)`"
- stored: "darwin issues the terminal-flush ioctl with the read-side bit over a file-local constant; linux issues the terminal-flush ioctl with the input-queue selector"

An implementer handed the second writes `unix.IoctlSetInt(fd, unix.TIOCFLUSH, …)` — the obvious reading, and wrong on darwin, where `TIOCFLUSH` takes a pointer and the read-side bit is a value `x/sys/unix` does not export at all. The same shape recurs in task 4-2, where the key dispatcher's byte values (`\r`, `\n`, `0x03`, `0x04`, `0x1a`, `0x1b`) are stored as "carriage return and newline", "the three control keys" and "the escape byte"; in task 5-3, where the flag spellings the hand-off argv must carry (`--screen discard`, `--report`, `--pane-key`) are stored as "the discard screen" and "the drop flag"; and in task 4-3, where the hook's exec shape `/bin/sh -c "<command>; exec <$SHELL>"` — the shape that makes a restored pane close on the first `exit` — is stored without the argv.

Per task, the number of code spans present in the approved body and absent from the stored copy:

| Task | tick id | lost spans | Task | tick id | lost spans |
|---|---|---|---|---|---|
| 1-5 | tick-923e20 | 2 | 5-1 | tick-8294a1 | 15 |
| 2-3 | tick-ea97fe | 3 | 5-2 | tick-bc0022 | 11 |
| 2-4 | tick-b3514f | 1 | 5-3 | tick-1b69d7 | 25 |
| 2-5 | tick-13b0aa | 0 (prose only) | 5-4 | tick-02e519 | 19 |
| 3-1 | tick-567651 | 11 | 5-5 | tick-7f0ea9 | 24 |
| 3-2 | tick-7ec142 | 10 | 5-6 | tick-6cb404 | 17 |
| 3-3 | tick-8b70d3 | 5 | 6-1 | tick-5d531b | 9 |
| 3-4 | tick-cbfffa | 8 | 6-2 | tick-9761fd | 10 |
| 3-5 | tick-ce5f67 | 6 | 6-3 | tick-5f2afd | 12 |
| 3-6 | tick-161f58 | 5 | 6-4 | tick-70ca46 | 12 |
| 4-1 | tick-0ce3a8 | 11 | 6-5 | tick-84b607 | 8 |
| 4-2 | tick-bafa83 | 17 | 6-6 | tick-dffb33 | 17 |
| 4-3 | tick-9078b4 | 14 | 6-7 | tick-23983c | 11 |
| 4-4 | tick-11a36d | 9 | 6-8 | tick-ff28c4 | 20 |
| 4-5 | tick-71b703 | 29 | | | |
| 4-6 | tick-c15167 | 14 | | | |
| 4-7 | tick-104e82 | 16 | | | |

Nothing is wrong with the plan's content: the `phase-{N}-tasks.md` bodies are correct, complete and — verified against the working tree for this review — accurate in every identifier, file path, module requirement and design-frame reference they name. The defect is that the copy the implementer is handed is not that body.

**Proposal**:
Rewrite each task's tick description to the body its phase detail file already holds, verbatim. The detail files are the approved text of record — they are what the author agent wrote and what the user saw at each authoring gate — and they are unchanged by this finding, so the fix is a copy in one direction with nothing to compose.

For each of the 39 internal ids in `task_map`, set the tick task's description to the content under the heading `## {internal_id}` in `.workflows/lazy-resume-on-attach/planning/lazy-resume-on-attach/phase-{N}-tasks.md`, excluding that heading and the `### Task N.M: …` line beneath it, and including every field from `**Problem**:` through `**Spec Reference**:`. Preserve backticks, byte literals (`\r`, `\x1b`, `0x03`), flag spellings and qualified identifiers exactly as the file holds them — those are the content, not formatting. Re-sync all 39 rather than the 31: the other eight differ only in the backticks around their test names, and a uniform copy costs nothing and leaves the store equal to the record everywhere.

Sequence the copy per the tick format's `updating.md` amendment procedure, reading the current description with `tick show <tick-id> --json` before each write, and verify each afterwards.

The per-task content is named by source rather than inlined here: the 39 bodies total roughly 150 KB, they already exist in the repository at a path this finding states exactly, and reproducing them would add a second copy to keep in step without adding a word of information. Nothing about the replacement is left to interpretation — the source file, the section within it and the boundaries of the extract are all stated.

**Current**:
The tick description for each id listed above, as `tick show <tick-id> --json` returns it today.

**Proposed Text**:
For each internal id, the body under `## {internal_id}` in its phase detail file, verbatim:

- `lazy-resume-on-attach-1-1` → `phase-1-tasks.md`, section `## lazy-resume-on-attach-1-1`
- `lazy-resume-on-attach-1-2` → `phase-1-tasks.md`, section `## lazy-resume-on-attach-1-2`
- `lazy-resume-on-attach-1-3` → `phase-1-tasks.md`, section `## lazy-resume-on-attach-1-3`
- `lazy-resume-on-attach-1-4` → `phase-1-tasks.md`, section `## lazy-resume-on-attach-1-4`
- `lazy-resume-on-attach-1-5` → `phase-1-tasks.md`, section `## lazy-resume-on-attach-1-5`
- `lazy-resume-on-attach-1-6` → `phase-1-tasks.md`, section `## lazy-resume-on-attach-1-6`
- `lazy-resume-on-attach-1-7` → `phase-1-tasks.md`, section `## lazy-resume-on-attach-1-7`
- `lazy-resume-on-attach-2-1` → `phase-2-tasks.md`, section `## lazy-resume-on-attach-2-1`
- `lazy-resume-on-attach-2-2` → `phase-2-tasks.md`, section `## lazy-resume-on-attach-2-2`
- `lazy-resume-on-attach-2-3` → `phase-2-tasks.md`, section `## lazy-resume-on-attach-2-3`
- `lazy-resume-on-attach-2-4` → `phase-2-tasks.md`, section `## lazy-resume-on-attach-2-4`
- `lazy-resume-on-attach-2-5` → `phase-2-tasks.md`, section `## lazy-resume-on-attach-2-5`
- `lazy-resume-on-attach-3-1` → `phase-3-tasks.md`, section `## lazy-resume-on-attach-3-1`
- `lazy-resume-on-attach-3-2` → `phase-3-tasks.md`, section `## lazy-resume-on-attach-3-2`
- `lazy-resume-on-attach-3-3` → `phase-3-tasks.md`, section `## lazy-resume-on-attach-3-3`
- `lazy-resume-on-attach-3-4` → `phase-3-tasks.md`, section `## lazy-resume-on-attach-3-4`
- `lazy-resume-on-attach-3-5` → `phase-3-tasks.md`, section `## lazy-resume-on-attach-3-5`
- `lazy-resume-on-attach-3-6` → `phase-3-tasks.md`, section `## lazy-resume-on-attach-3-6`
- `lazy-resume-on-attach-4-1` → `phase-4-tasks.md`, section `## lazy-resume-on-attach-4-1`
- `lazy-resume-on-attach-4-2` → `phase-4-tasks.md`, section `## lazy-resume-on-attach-4-2`
- `lazy-resume-on-attach-4-3` → `phase-4-tasks.md`, section `## lazy-resume-on-attach-4-3`
- `lazy-resume-on-attach-4-4` → `phase-4-tasks.md`, section `## lazy-resume-on-attach-4-4`
- `lazy-resume-on-attach-4-5` → `phase-4-tasks.md`, section `## lazy-resume-on-attach-4-5`
- `lazy-resume-on-attach-4-6` → `phase-4-tasks.md`, section `## lazy-resume-on-attach-4-6`
- `lazy-resume-on-attach-4-7` → `phase-4-tasks.md`, section `## lazy-resume-on-attach-4-7`
- `lazy-resume-on-attach-5-1` → `phase-5-tasks.md`, section `## lazy-resume-on-attach-5-1`
- `lazy-resume-on-attach-5-2` → `phase-5-tasks.md`, section `## lazy-resume-on-attach-5-2`
- `lazy-resume-on-attach-5-3` → `phase-5-tasks.md`, section `## lazy-resume-on-attach-5-3`
- `lazy-resume-on-attach-5-4` → `phase-5-tasks.md`, section `## lazy-resume-on-attach-5-4`
- `lazy-resume-on-attach-5-5` → `phase-5-tasks.md`, section `## lazy-resume-on-attach-5-5`
- `lazy-resume-on-attach-5-6` → `phase-5-tasks.md`, section `## lazy-resume-on-attach-5-6`
- `lazy-resume-on-attach-6-1` → `phase-6-tasks.md`, section `## lazy-resume-on-attach-6-1`
- `lazy-resume-on-attach-6-2` → `phase-6-tasks.md`, section `## lazy-resume-on-attach-6-2`
- `lazy-resume-on-attach-6-3` → `phase-6-tasks.md`, section `## lazy-resume-on-attach-6-3`
- `lazy-resume-on-attach-6-4` → `phase-6-tasks.md`, section `## lazy-resume-on-attach-6-4`
- `lazy-resume-on-attach-6-5` → `phase-6-tasks.md`, section `## lazy-resume-on-attach-6-5`
- `lazy-resume-on-attach-6-6` → `phase-6-tasks.md`, section `## lazy-resume-on-attach-6-6`
- `lazy-resume-on-attach-6-7` → `phase-6-tasks.md`, section `## lazy-resume-on-attach-6-7`
- `lazy-resume-on-attach-6-8` → `phase-6-tasks.md`, section `## lazy-resume-on-attach-6-8`

All paths are relative to `.workflows/lazy-resume-on-attach/planning/lazy-resume-on-attach/`.

**Resolution**: Pending
**Notes**:
