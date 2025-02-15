# Tigron test framework

>  no-one likes you, `if [ $? -eq 0 ]`

A modern testing framework for command-line applications.

## TL;DR

TBD

## Documentation

TBD

For now, see [nerdctl-specific testing documentation](https://github.com/containerd/nerdctl/blob/main/docs/testing/tools.md).

## Motivation and goals

Testing (go) binaries is a journey fraught with many pitfalls.

While some tooling exist (the venerable [bats](https://github.com/bats-core/bats-core), or the solid work going
on at [gotestyourself](https://github.com/gotestyourself)), they either focus on relatively low-level testing
primitives (`assert`, `exec`), or do not integrate well into the natural go environment
(`bats` requires you to write shell scripts), and routinely require additional third-party tools for advanced scenarios
(hello `unbuffer`), and for the developer to write a large set of "helpers".

Projects and companies thus routinely end-up growing in-house tooling, that generally suffer from a number of
rampant issues: lack of structure and expressiveness, helpers spaghetti, unclear test lifecycle (specifically
cleanup), resource leakage and cross test interaction, ultimately encouraging bad test design leading to degraded
and un-scalable situations (flakyness being of course the number 1 scourge).

Tigron was developed specifically to address these issues, based on the experience testing nerdctl, a large cli
with a lot of integration tests.

Tigron does not replace `gotest.tools`, nor `gotestsum`. In fact, it leverages and encourages use of these where
appropriate.

Tigron ambition is to provide a ready-to-use, clean, simple, go-native framework meant specifically to
write tests for cli binaries, encouraging good test design and a stronger basis to build tests suite.
It also comes with a set of helpers to accomodate most advanced scenarios (command backgrounding, stdin manipulation,
support for pseudo ttys, environment filtering, etc.)

## Hack

### Initial setup

Clone, then:

```
./hack/dev-setup-linux.sh
# Or
# ./hack/dev-setup-macos.sh

make install-dev-tools
```

### Work

```
# Update dependencies
make up
```

```
# Re-order imports, gofmt, go mod tidy, etc
make fix
```

```
# Ensure linters are happy
make lint
```

```
# Run tests
make test
```
