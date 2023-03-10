# Maintainers' guide

## Maintainer list

- Core: https://github.com/containerd/project/blob/main/MAINTAINERS
- Non-core: [`MAINTAINERS`](./MAINTAINERS)

## Governance

See https://github.com/containerd/project/blob/main/GOVERNANCE.md

## Creating a release

Eligibility to be a release manager:
- MUST be an active Commiter (Core or Non-core)
- MUST have the GPG fingerprint listed in [`MAINTAINERS`](./MAINTAINERS)
- MUST upload the GPG public key to `https://github.com/USERNAME.gpg`
- MUST protect the GPG key with a passphrase or a hardware token.

Release steps:
- Open a PR to keep the dependencies up-to-date.
  Update `go.mod` for Go dependencies (usually Dependabot automatically updates them).
  Update `Dockerfile` and relevant files under `Dockerfile.d` for `nerdctl-full` dependencies.
- Open an issue to propose making a new release.
  The proposal should be public, with an exception for vulnerability fixes.
  If this is the first time for you to take a role of release management,
  you SHOULD make a beta (or alpha, RC) release as an exercise before releasing GA.
- Make sure that all the merged PRs are associated with the correct [Milestone](https://github.com/containerd/nerdctl/milestones).
- Run `git tag --sign vX.Y.Z-beta.W` .
- Run `git push UPSTREAM vX.Y.Z-beta.W` .
- Wait for the `Release` action on GitHub Actions to complete. A draft release will appear in https://github.com/containerd/nerdctl/releases .
- Download `SHA256SUMS` from the draft release, and confirm that it corresponds to the hashes printed in the build logs on the `Release` action.
- Sign `SHA256SUMS` with `gpg --detach-sign -a SHA256SUMS` to produce `SHA256SUMS.asc`, and upload it to the draft release.
- Add release notes in the draft release, to explain the changes and show appreciation to the contributors.
  Make sure to fulfill the `Release manager: [ADD YOUR NAME HERE] (@[ADD YOUR GITHUB ID HERE])` line with your name.
  e.g., `Release manager: Akihiro Suda (@AkihiroSuda)` .
- Click the `Set as a pre-release` checkbox if this release is a beta (or alpha, RC).
- Click the `Publish release` button.
- Close the [Milestone](https://github.com/containerd/nerdctl/milestones).
