# Dependencies provisioning for integration testing

This folder provides a set of scripts useful (for the CI) to configure hosts for
the purpose of testing.

While this is agnostic and would (probably) work outside the context of GitHub Actions,
this is not the right way for people to install a functioning stack.
Use provided installation scripts instead (see user documentation).

## Contents

- `/version` allows retrieving latest (or experimental) versions of certain products (golang, containerd, etc)
- `/linux` allows updating in-place containerd, cni (future: buildkit)
- `/windows` allows install WinCNI, containerd
- `/kube` allows spinning-up a Kind cluster