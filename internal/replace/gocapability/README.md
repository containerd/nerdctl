# ./gocapability

`./gocapability` is a replacement for https://github.com/syndtr/gocapability providing just
what is currently required to fulfill:
- https://github.com/cncf-tags/container-device-interface
- via https://github.com/opencontainers/runtime-tools
  (originally added as a dependency in the early days of opencontainers by RH Patel:
https://github.com/opencontainers/runtime-tools/commit/b462307c920f3a3e4f9a294ebf716f160a08ed44#diff-daddc66424f674dc57b411e63fce1f1bd231a27a77e39d25c781b4a8ca6f6062)

The original repository unfortunately makes unnecessary use of `init()`, resulting
in systematically attempting to open and read `/proc/sys/kernel/cap_last_cap`
even (especially) when not required.

This issue affects any project linking it (runc, containerd, etc).

It has been reported a year ago without reaction from the maintainer
(https://github.com/syndtr/gocapability/issues/26), and the project
looks very much abandoned at this point (maintainer seem to have all but vanished into crypto circa 2022).

While the performance impact is likely marginal, the security impact, less so:
for `gomodjail` to work, `gocapability` would have to be marked as unconfined,
which is made even more problematic precisely because it seems abandoned
(hence more likely to be taken over).

As far as nerdctl is concerned, the only thing that we actually need from it is the static list of linux capabilities
and consts, which this is providing.
