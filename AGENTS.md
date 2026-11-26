# AGENTS.md

## Preparing changes

Before contributing, read [Contributing to nerdctl](README.md#contributing-to-nerdctl) and the [org-wide contribution guide](https://github.com/containerd/project/blob/main/CONTRIBUTING.md).

For AI-assisted work, follow the org guide's [Coding Agent Usage](https://github.com/containerd/project/blob/main/CONTRIBUTING.md#coding-agent-usage) policy. Never list an AI tool in `Signed-off-by` or `Co-authored-by`; use an `Assisted-by` trailer instead.

- nerdctl aims at Docker CLI compatibility. Before adding or changing a command or flag, check the behavior of the corresponding `docker` command, and update [Command reference](docs/command-reference.md) in the same change.
- CLI flag parsing lives in `cmd/nerdctl/`; the implementation lives in `pkg/cmd/`, with option types in `pkg/api/types/`. Keep business logic out of `cmd/nerdctl/`.
- New features of uncertain shape should start as [experimental](docs/experimental.md).
- Keep existing [configuration files](docs/config.md) and [on-disk state](docs/dir.md) working.

## Security findings

Before evaluating a suspected vulnerability or drafting a security report, read the [containerd security policy](https://github.com/containerd/project/blob/main/SECURITY.md).
