# tmutil-wildcard

Keeps git clones out of Time Machine backups. `tmutil addexclusion` has no
wildcard support, so this tool walks one or more root folders, finds every
directory that holds a `.git` entry, and adds a sticky exclusion for it. A
launch agent repeats that hourly, so new clones are excluded before the next
backup picks them up.

## How it works

- A clone root is any directory containing `.git` (a directory for a normal
  clone, a file for a worktree or submodule checkout). The walk stops there,
  so nested repositories are covered by the parent's exclusion.
- Each clone is checked with `tmutil isexcluded`; only clones reported as
  included get `tmutil addexclusion <path>`.
- The exclusion is the sticky kind (an xattr on the folder, no `-p`). It needs
  no root, moves with the folder, and disappears when the folder is deleted.
  A re-cloned folder is picked up on the next run.
- Symlinks are not followed. Unreadable directories are logged and skipped.

## Install

Requires Go and [Task](https://taskfile.dev).

```sh
task install
```

This installs the binary to `$(go env GOPATH)/bin/tmutil-wildcard`, renders
`launchd/com.erikfrerejean.tmutil-wildcard.plist` with your home directory into
`~/Library/LaunchAgents/`, and loads it. The agent runs at load and then every
hour against `~/WeDevelop`. Output goes to `~/Library/Logs/tmutil-wildcard.log`;
only newly excluded paths and failures are logged.

```sh
task run        # one foreground run against ~/WeDevelop
task test       # go vet + go test
task uninstall  # unload the agent and remove the binary
```

To watch a different folder, change the root argument in the plist template
and re-run `task install`, or run the binary directly:

```sh
tmutil-wildcard ~/Code ~/Other
```

## Undoing an exclusion

```sh
tmutil removeexclusion <path>
```
