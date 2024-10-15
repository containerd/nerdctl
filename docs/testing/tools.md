# Nerdctl testing tools

## Preamble

The integration test suite in nerdctl is meant to apply to both nerdctl and docker,
and further support additional test properties to target specific contexts (ipv6, kube).

Basic _usage_ is covered in the [testing docs](README.md).

This here covers how to write tests, leveraging nerdctl `pkg/testutil/test`
which has been specifically developed to take care of repetitive tasks,
protect the developer against unintended side effects across tests, and generally
encourage clear testing structure with good debug-ability and a relatively simple API for
most cases.

## Using `test.Case`

Starting from scratch, the simplest, basic structure of a new test is:

```go
package main

import (
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMyThing(t *testing.T) {
	// Declare your test
	myTest := nerdtest.Setup()
	// This is going to run `nerdctl info` (or `docker info`)
	mytest.Command = test.Command("info")
    // Verify the command exits with 0, and stdout contains the word `Kernel`
    myTest.Expected = test.Expects(0, nil, test.Contains("Kernel"))
	// Run it
	myTest.Run(t)
}
```

## Expectations

There are a handful of helpers for "expectations".

You already saw two (`test.Expects` and `test.Contains`):

First, `test.Expects(exitCode int, errors []error, outputCompare Comparator)`, which is
convenient to quickly describe what you expect overall.

`exitCode` is obvious (note that passing -1 as an exit code will just verify the commands does fail without comparing the code).

`errors` is a slice of go `error`, that allows you to compare what is seen on stderr
with existing errors (for example: `errdefs.ErrNotFound`), or more generally
any string you want to match.

`outputCompare` can be either your own comparison function, or
one of the comparison helper.

Secondly, `test.Contains` - which is a `Comparator`.

### Comparators

Besides `test.Contains(string)`, there are a few more:
- `test.DoesNotContain(string)`
- `test.Equals(string)`
- `test.Match(*regexp.Regexp)`
- `test.All(comparators ...Comparator)`, which allows you to bundle together a bunch of other comparators

The following example shows how to implement your own custom `Comparator`
(this is actually the `Equals` comparator).

```go
package whatever

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func MyComparator(compare string) test.Comparator {
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Assert(t, stdout == compare, info)
	}
}
```

Note that you have access to an opaque `info` string.
It contains relevant debugging information in case your comparator is going to fail,
and you should make sure it is displayed.

### Advanced expectations

You may want to have expectations that contain a certain piece of data
that is being used in the command or at other stages of your test (Setup).

For example, creating a container with a certain name, you might want to verify
that this name is then visible in the list of containers.

To achieve that, you should write your own `Manager`, leveraging test `Data`.

Here is an example, where we are using `data.Get("sometestdata")`.

```go
package main

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMyThing(t *testing.T) {
	nerdtest.Setup()

	// Declare your test
	myTest := &test.Case{
		Data:        test.WithData("sometestdata", "blah"),
		Command:     test.Command("info"),
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: 1,
				Errors: []error{
					errors.New("foobla"),
					errdefs.ErrNotFound,
				},
				Output: func(stdout string, info string, t *testing.T) {
					assert.Assert(t, stdout == data.Get("sometestdata"), info)
				},
			}
		},
	}

	myTest.Run(t)
}
```

## On `Data`

`Data` is provided to allow storing mutable key-value information that pertain to the test.

While it can be provided through `test.WithData(key string, value string)`,
inside the testcase definition, it can also be dynamically manipulated inside `Setup`, or `Command`.

Note that `Data` additionally exposes the following functions:
- `Identifier(words ...string)` which returns a unique identifier associated with the _current_ test (or subtest)
- `TempDir()` which returns the private, temporary directory associated with the test

... along with the `Get(key)` and `Set(key, value)` methods.

Note that Data is copied down to subtests, which is convenient to pass "down"
information relevant to a bunch of subtests (eg: like a registry IP).

## On Config

`Config` is similar to `Data`, although it is meant specifically for predefined
keys that impact the base behavior of the binary you are testing.

You can initiate your config using `test.WithConfig(key, value)`, and you can
manipulate it further using `helpers.Read` and`helpers.Write`.

Currently, the following keys are defined:
- `DockerConfig` allowing to set custom content for the `$DOCKER_CONFIG/config.json` file
- `Namespace` (default to `nerdctl-test` if unspecified, but see "mode private")
- `NerdctlToml` to set custom content for the `$NERDCTL_TOML` file
- `HostsDir` to specify the value of the arg `--hosts-dir`
- `DataRoot` to specify the value of the arg `--data-root`
- `Debug` to enable debug (works for both nerdctl and docker)

Note that config defined on the test case is copied over for subtests.

## Commands

For simple cases, `test.Command(args ...string)` is the way to go.

It will execute the binary to test (nerdctl or docker), with the provided arguments,
and will by default get cwd inside the temporary directory associated with the test.

### Environment

You can attach custom environment variables for your test in the `Env` property of your
test.

These will be automatically added to the environment for your command, and also
your setup and cleanup routines (see below).

If you would like to override the environment specifically for a command, but not for
others (eg: in `Setup` or `Cleanup`), you can do so with custom commands (see below).

Note that environment as defined statically in the test will be copied over for subtests.

### Working directory

By default, the working directory of the command will be set to the temporary directory
of the test.

This behavior can be overridden using custom commands.

### Custom Executor

Custom `Executor`s allow you to manipulate test `Data`, override important aspects
of the command to execute (`Env`, `WorkingDir`), or otherwise give you full control
on what the command does.

You just need to implement an `Executor`:

```go
package main

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMyThing(t *testing.T) {
	nerdtest.Setup()

	// Declare your test
	myTest := &test.Case{
		Data:        test.WithData("sometestdata", "blah"),
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--name", data.Get("sometestdata"))
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: 1,
				Errors: []error{
					errors.New("foobla"),
					errdefs.ErrNotFound,
				},
				Output: func(stdout string, info string, t *testing.T) {
					assert.Assert(t, stdout == data.Get("sometestdata"), info)
				},
			}
		},
	}

	myTest.Run(t)
}
```

Note that inside your `Executor` you do have access to the full palette of command options,
including:
- `Background(timeout time.Duration)` which allows you to background a command execution
- `WithWrapper(binary string, args ...string)` which allows you to "wrap" your command with another binary
- `WithStdin(io.Reader)` which allows you to pass a reader to the command stdin
- `WithCwd(string)` which allows you to specify the working directory (default to the test temp directory)
- `Clone()` which returns a copy of the command, with env, cwd, etc

and also `WithBinary` and `WithArgs`.

### On `helpers`

Inside a custom `Executor`, `Manager`, or `Butler`, you have access to a collection of
`helpers` to simplify command execution:

- `helpers.Ensure(args ...string)` will run a command and ensure it exits successfully
- `helpers.Fail(args ...string)` will run a command and ensure it fails
- `helpers.Anyhow(args ...string)` will run a command but does not care if it succeeds or fails
- `helpers.Capture(args ...string)` will run a command, ensure it is successful, and return the output
- `helpers.Command(args ...string)` will return a command that can then be tested against expectations
- `helpers.Custom(binary string, args ...string)` will do the same for any arbitrary command (not limited to nerdctl)
- `helpers.T()` which returns the appropriate `*testing.T` for your context

## Setup and Cleanup

Tests routinely require a set of actions to be performed _before_ you can run the
command you want to test.
A setup routine will get executed before your `Command`, and have access to and can
manipulate your test `Data` and `Config`.

Conversely, you very likely want to clean things up once your test is done.
While temporary directories are cleaned for you with no action needed on your part,
the app you are testing might have stateful data you may want to remove.
Note that a `Cleanup` routine will get executed twice - after your `Command` has run
its course evidently - but also, pre-emptively, before your `Setup`, so that possible leftovers from
previous runs are taken care of.

```go
package main

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMyThing(t *testing.T) {
	nerdtest.Setup()

	// Declare your test
	myTest := &test.Case{
		Data:        test.WithData("sometestdata", "blah"),
		Setup: func(data *test.Data, helpers test.Helpers){
			helpers.Ensure("volume", "create", "foo")
			helpers.Ensure("volume", "create", "bar")
		},
		Cleanup: func(data *test.Data, helpers test.Helpers){
			helpers.Anyhow("volume", "rm", "foo")
			helpers.Anyhow("volume", "rm", "bar")
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--name", data.Identifier()+data.Get("sometestdata"))
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: 1,
				Errors: []error{
					errors.New("foobla"),
					errdefs.ErrNotFound,
				},
				Output: func(stdout string, info string, t *testing.T) {
					assert.Assert(t, stdout == data.Get("sometestdata"), info)
				},
			}
		},
	}

	myTest.Run(t)
}
```

## Subtests

Subtests are just regular tests, attached to the `SubTests` slice of a test.

Note that a subtest will inherit its parent `Data`, `Config` and `Env`, in the state they are at
after the parent test has run its `Setup` and `Command` routines (but before `Cleanup`).
This does _not_ apply to `Identifier()` and `TempDir()`, which are unique to the subtest.

Also note that a test does not have to have a `Command`.
This is a convenient pattern if you just need a common `Setup` for a bunch of subtests.

## Parallelism

All tests (and subtests) are assumed to be parallelizable.

You can force a specific `test.Case` to not be run in parallel though,
by setting its `NoParallel` property to `true`.

Note that if you want better isolation, it is usually better to use the requirement
`nerdtest.Private` instead of `NoParallel` (see below).

## Requirements

`test.Case` has a `Require` property that allow enforcing specific, per-test requirements.

A `Requirement` is expected to make you `Skip` tests when the environment does not match
expectations.

Here are a few:
```go
test.Windows // a test runs only on Windows (or Not(Windows))
test.Linux // a test runs only on Linux
test.Darwin // a test runs only on Darwin
test.OS(name string) // a test runs only on the OS `name`
test.Binary(name string) // a test requires the bin `name` to be in the PATH
test.Not(req Requirement) // a test runs only if the opposite of the requirement `req` is fulfilled
test.Require(req ...Requirement) // a test runs only if all requirements are fulfilled

nerdtest.Docker // a test only run on Docker - normally used with test.Not(nerdtest.Docker)
nerdtest.Soci // a test requires the soci snapshotter
nerdtest.Stargz // a test requires the stargz snapshotter
nerdtest.Rootless // a test requires Rootless
nerdtest.Rootful // a test requires Rootful
nerdtest.Build // a test requires buildkit
nerdtest.CGroup // a test requires cgroup
nerdtest.NerdctlNeedsFixing // indicates that a test cannot be run on nerdctl yet as a fix is required
nerdtest.BrokenTest // indicates that a test needs to be fixed and has been restricted to run only in certain cases
nerdtest.OnlyIPv6 // a test is meant to run solely in the ipv6 environment
nerdtest.OnlyKubernetes // a test is meant to run solely in the Kubernetes environment
nerdtest.IsFlaky // indicates that a test will fail in a flaky way - this may be the test fault, or more likely something racy in nerdctl
nerdtest.Private // see below
```

### About `nerdtest.Private`

While all requirements above are self-descriptive or obvious, `nerdtest.Private` is  a
special case.

If set, it will run tests inside a dedicated namespace that is private to the test.
Note that subtests by default are going to be set in that same namespace, unless they
ask for private as well, or they reset the `Namespace` config key.

If the target is Docker - which does not support namespaces - asking for `Private`
will disable parallelization.

The purpose of private is to provide a truly clean-room environment for tests
that are going to have side effects on others, or that do require an exclusive, pristine
environment.

Using private is generally preferable to disabling parallelization, as doing the latter
would slow down the run and won't have the same isolation guarantees about the environment.

## Advanced command customization

Testing any non-trivial binary likely assume a good amount of custom code
to set up the right default behavior wrt environment, flags, etc.

To do that, you can pass a `test.Testable` implementation to the `test.Customize` method.

It basically lets you define your own `CustomizableCommand`, along with a hook to deal with
ambient requirements that is run after `test.Require` and before `test.Setup`.

`CustomizableCommand` are typically embedding a `test.GenericCommand` and overriding both the
`Run` and `Clone` methods.

Check the `nerdtest` implementation for details.

## Utilities

TBD