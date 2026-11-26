# Improve os.Stat Error Handling in migrateConfigFile

The `migrateConfigFile` helper in `cmd/config.go` checks whether the target file already exists at the new path using `os.Stat`. If the stat call returns `nil` error (file exists), migration is skipped to avoid overwriting. However, the current logic doesn't distinguish between "file exists" and "stat failed for another reason" — for example, permission denied on the parent directory.

In that edge case, the migration would be silently skipped even though moving the file might have succeeded. The function treats any non-error stat result as "file present, don't touch it" without considering that a stat failure could mean something other than "file not found."

This surfaced during review of the config-dir-wrong-path-macos bugfix. It was assessed as acceptable given the best-effort migration contract — migration failures are non-fatal by design, and the scenario (permission denied on a parent of the new config path while the path itself doesn't exist) is extremely unlikely in practice. But it's worth noting as a potential refinement.

The specific location is the `os.Stat(newPath)` check in the `migrateConfigFile` helper in `cmd/config.go`. A more precise approach would check for `os.IsNotExist(err)` explicitly rather than treating any error as "file doesn't exist, proceed with migration." This would make the guard clause's intent clearer and handle the theoretical edge case correctly.
